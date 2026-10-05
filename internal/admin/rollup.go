package admin

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/url"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// DefaultRollupInterval is the period of the background rollup loop.
const DefaultRollupInterval = 10 * time.Minute

// MaxRollupDays bounds a RollupRange / Backfill.
const MaxRollupDays = 400

const dayLayout = "2006-01-02"

// Rollup aggregates watch_sessions and users from the accounts database into
// the admin database (metrics_daily, metrics_hourly, metrics_anime_daily).
//
// Read-only choice: the accounts database is opened as a SECOND, separate
// connection pool with `file:...?mode=ro` (SQLite URI, enforced by the engine)
// instead of ATTACH on the admin pool, because ATTACH is per-connection and
// would leak into pooled connections; aggregation across the two databases is
// done in Go. Any write attempt on it fails with "readonly database".
type Rollup struct {
	admin    *sql.DB
	accounts *sql.DB
	now      func() time.Time
	mu       sync.Mutex // prevents two overlapping passes
}

// NewRollup builds a Rollup writing into s and reading the accounts database
// at accountsPath (accounts.sqlite, which must already exist).
func NewRollup(s *Store, accountsPath string) (*Rollup, error) {
	abs, err := filepath.Abs(accountsPath)
	if err != nil {
		return nil, err
	}
	u := url.URL{Scheme: "file", Path: abs, RawQuery: "mode=ro&_busy_timeout=5000"}
	db, err := sql.Open("sqlite3", u.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(2)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return &Rollup{admin: s.db, accounts: db, now: time.Now}, nil
}

// Close closes the read-only accounts connection (not the admin store).
func (r *Rollup) Close() error { return r.accounts.Close() }

type dayAgg struct {
	sessions, seconds, completed int64
	secF                         float64
	users                        map[int64]struct{}
	hours                        [24]int64
	newUsers                     int64
}

type animeAgg struct {
	title      string
	titleAt    int64
	sessions   int64
	secF       float64
	completed  int64
	newViewers int64
}

// RollupRange recomputes every UTC day in [fromDay, toDay] (YYYY-MM-DD,
// inclusive, at most MaxRollupDays). Idempotent: each day's rows are replaced
// in one transaction.
func (r *Rollup) RollupRange(ctx context.Context, fromDay, toDay string) error {
	from, err := time.Parse(dayLayout, fromDay)
	if err != nil {
		return fmt.Errorf("rollup: invalid fromDay %q: %w", fromDay, err)
	}
	to, err := time.Parse(dayLayout, toDay)
	if err != nil {
		return fmt.Errorf("rollup: invalid toDay %q: %w", toDay, err)
	}
	if to.Before(from) {
		return errors.New("rollup: toDay before fromDay")
	}
	nDays := int(to.Sub(from).Hours()/24) + 1
	if nDays > MaxRollupDays {
		return fmt.Errorf("rollup: range of %d days exceeds %d", nDays, MaxRollupDays)
	}
	lo, hi := from.Unix(), to.AddDate(0, 0, 1).Unix() // [lo, hi)

	days := make(map[string]*dayAgg, nDays)
	animes := make(map[string]map[int64]*animeAgg)
	for i := 0; i < nDays; i++ {
		d := from.AddDate(0, 0, i).Format(dayLayout)
		days[d] = &dayAgg{users: map[int64]struct{}{}}
		animes[d] = map[int64]*animeAgg{}
	}
	dayOf := func(ts int64) string { return time.Unix(ts, 0).UTC().Format(dayLayout) }

	rows, err := r.accounts.QueryContext(ctx, `SELECT user_id, anime_id, title, started_at, watched_seconds, completed
		FROM watch_sessions WHERE started_at >= ? AND started_at < ?`, lo, hi)
	if err != nil {
		return err
	}
	for rows.Next() {
		var uid, aid, started int64
		var title string
		var secs float64
		var comp int64
		if err := rows.Scan(&uid, &aid, &title, &started, &secs, &comp); err != nil {
			rows.Close()
			return err
		}
		if secs < 0 {
			secs = 0
		}
		d := dayOf(started)
		a := days[d]
		a.sessions++
		a.secF += secs
		a.users[uid] = struct{}{}
		a.hours[time.Unix(started, 0).UTC().Hour()]++
		if comp != 0 {
			a.completed++
		}
		if aid > 0 { // anime_id 0 = unknown, kept out of the per-anime table
			an := animes[d][aid]
			if an == nil {
				an = &animeAgg{}
				animes[d][aid] = an
			}
			an.sessions++
			an.secF += secs
			if comp != 0 {
				an.completed++
			}
			if title != "" && started >= an.titleAt {
				an.title, an.titleAt = title, started
			}
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}

	// new_viewers: first-ever session of (user, anime) falls in the range.
	rows, err = r.accounts.QueryContext(ctx, `SELECT anime_id, MIN(started_at) AS first FROM watch_sessions
		WHERE anime_id > 0 GROUP BY user_id, anime_id HAVING first >= ? AND first < ?`, lo, hi)
	if err != nil {
		return err
	}
	for rows.Next() {
		var aid, first int64
		if err := rows.Scan(&aid, &first); err != nil {
			rows.Close()
			return err
		}
		if an := animes[dayOf(first)][aid]; an != nil {
			an.newViewers++
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}

	// new_users per day.
	rows, err = r.accounts.QueryContext(ctx, `SELECT created_at FROM users WHERE created_at >= ? AND created_at < ?`, lo, hi)
	if err != nil {
		return err
	}
	for rows.Next() {
		var c int64
		if err := rows.Scan(&c); err != nil {
			rows.Close()
			return err
		}
		days[dayOf(c)].newUsers++
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}

	// total_users at end of day: users before `lo` plus cumulative new users.
	var base int64
	if err := r.accounts.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE created_at < ?`, lo).Scan(&base); err != nil {
		return err
	}

	computed := r.now().Unix()
	total := base
	for i := 0; i < nDays; i++ {
		d := from.AddDate(0, 0, i).Format(dayLayout)
		a := days[d]
		total += a.newUsers
		if err := r.writeDay(ctx, d, a, animes[d], total, computed); err != nil {
			return err
		}
	}
	return nil
}

func (r *Rollup) writeDay(ctx context.Context, day string, a *dayAgg, an map[int64]*animeAgg, total, computed int64) error {
	tx, err := r.admin.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, t := range []string{"metrics_daily", "metrics_hourly", "metrics_anime_daily"} {
		if _, err := tx.ExecContext(ctx, `DELETE FROM `+t+` WHERE day = ?`, day); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO metrics_daily(day, sessions, watch_seconds, active_users, new_users, completed_sessions, total_users, computed_at)
		VALUES(?,?,?,?,?,?,?,?)`, day, a.sessions, int64(a.secF+0.5), len(a.users), a.newUsers, a.completed, total, computed); err != nil {
		return err
	}
	for h, n := range a.hours {
		if n == 0 {
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO metrics_hourly(day, hour, sessions) VALUES(?,?,?)`, day, h, n); err != nil {
			return err
		}
	}
	ids := make([]int64, 0, len(an))
	for id := range an {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		x := an[id]
		if _, err := tx.ExecContext(ctx, `INSERT INTO metrics_anime_daily(day, anime_id, title, sessions, watch_seconds, completed_sessions, new_viewers)
			VALUES(?,?,?,?,?,?,?)`, day, id, x.title, x.sessions, int64(x.secF+0.5), x.completed, x.newViewers); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// RollupRecent recomputes yesterday and today (UTC) relative to now.
func (r *Rollup) RollupRecent(ctx context.Context, now time.Time) error {
	n := now.UTC()
	return r.RollupRange(ctx, n.AddDate(0, 0, -1).Format(dayLayout), n.Format(dayLayout))
}

// Backfill recomputes the last `days` days up to and including today (UTC).
// days is clamped to [1, MaxRollupDays].
func (r *Rollup) Backfill(ctx context.Context, now time.Time, days int) error {
	if days < 1 {
		days = 1
	}
	if days > MaxRollupDays {
		days = MaxRollupDays
	}
	n := now.UTC()
	return r.RollupRange(ctx, n.AddDate(0, 0, -(days-1)).Format(dayLayout), n.Format(dayLayout))
}

// Run calls RollupRecent immediately, then every interval (default 10 minutes
// when interval <= 0) until ctx is done. Errors and panics are logged and the
// loop continues; passes never overlap.
//
// No cross-instance lock: M2 will wire a Redis leader election, like
// startWarmer in internal/api; a single replica is enough for v1.
func (r *Rollup) Run(ctx context.Context, interval time.Duration, now func() time.Time) {
	if interval <= 0 {
		interval = DefaultRollupInterval
	}
	if now == nil {
		now = time.Now
	}
	pass := func() {
		if !r.mu.TryLock() {
			return
		}
		defer r.mu.Unlock()
		defer func() {
			if p := recover(); p != nil {
				log.Printf("admin rollup: panic: %v", p)
			}
		}()
		if err := r.RollupRecent(ctx, now()); err != nil && ctx.Err() == nil {
			log.Printf("admin rollup: %v", err)
		}
	}
	pass()
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			pass()
		}
	}
}
