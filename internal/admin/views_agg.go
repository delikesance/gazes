package admin

import (
	"context"
	"database/sql"
	"encoding/json"
	"math"
	"time"
)

// viewsAcc accumulates what /views derives from watch_sessions: session length buckets, the
// retention curve and the per-episode drop stats. One instance serves a whole period (direct scan)
// or one day (rollup row); merging day rows gives the same result as scanning the period, because
// every figure is a sum, a count, a latest-title pick or a list of minutes.
type viewsAcc struct {
	buckets           [5]int64
	nSess             int64
	totalMin          float64
	present, eligible [10]int64
	drops             map[[3]int64]*viewsDropAgg
}

func newViewsAcc() *viewsAcc { return &viewsAcc{drops: map[[3]int64]*viewsDropAgg{}} }

// add folds in one watch session.
func (a *viewsAcc) add(anime, season, started int64, ep int, title string, startPos, endPos, watched, dur float64, comp int) {
	if watched < 0 {
		watched = 0
	}
	m := watched / 60
	a.nSess++
	a.totalMin += m
	switch {
	case m < 5:
		a.buckets[0]++
	case m < 15:
		a.buckets[1]++
	case m < 30:
		a.buckets[2]++
	case m <= 60:
		a.buckets[3]++
	default:
		a.buckets[4]++
	}
	if dur <= 0 {
		return
	}
	sf := math.Min(math.Max(startPos/dur, 0), 1)
	ef := math.Min(math.Max(endPos/dur, 0), 1)
	for k := 0; k < 10; k++ {
		lowEdge := float64(k) / 10
		if sf <= lowEdge {
			a.eligible[k]++
			if ef > lowEdge {
				a.present[k]++
			}
		}
	}
	if anime > 0 && sf < 0.25 {
		key := [3]int64{anime, season, int64(ep)}
		d := a.drops[key]
		if d == nil {
			d = &viewsDropAgg{anime: anime, season: season, ep: int64(ep)}
			a.drops[key] = d
		}
		d.sessions++
		if title != "" && started >= d.titleAt {
			d.title, d.titleAt = title, started
		}
		if comp == 0 && ef < 0.25 {
			d.abandoned++
			d.minutes = append(d.minutes, endPos/60)
		}
	}
}

// merge folds another accumulator (a day row) into this one.
func (a *viewsAcc) merge(o *viewsAcc) {
	for i := range a.buckets {
		a.buckets[i] += o.buckets[i]
	}
	a.nSess += o.nSess
	a.totalMin += o.totalMin
	for i := range a.present {
		a.present[i] += o.present[i]
		a.eligible[i] += o.eligible[i]
	}
	for key, d := range o.drops {
		x := a.drops[key]
		if x == nil {
			cp := *d
			cp.minutes = append([]float64(nil), d.minutes...)
			a.drops[key] = &cp
			continue
		}
		x.sessions += d.sessions
		x.abandoned += d.abandoned
		x.minutes = append(x.minutes, d.minutes...)
		if d.title != "" && d.titleAt >= x.titleAt {
			x.title, x.titleAt = d.title, d.titleAt
		}
	}
}

// scanViews aggregates the sessions started in [lo, hi) straight from watch_sessions.
func (s *Service) scanViews(ctx context.Context, lo, hi int64) (*viewsAcc, error) {
	rows, err := s.accountsDB().QueryContext(ctx, `SELECT anime_id, season_id, episode, title, started_at, start_position, end_position, watched_seconds, duration, completed
		FROM watch_sessions WHERE started_at >= ? AND started_at < ?`, lo, hi)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	acc := newViewsAcc()
	for rows.Next() {
		var anime, season, started int64
		var ep int
		var title string
		var startPos, endPos, watched, dur float64
		var comp int
		if err := rows.Scan(&anime, &season, &ep, &title, &started, &startPos, &endPos, &watched, &dur, &comp); err != nil {
			return nil, err
		}
		acc.add(anime, season, started, ep, title, startPos, endPos, watched, dur, comp)
	}
	return acc, rows.Err()
}

// loadViewsRollup merges the per-day rows of [from, to]. ok is false unless every day of the
// period has a row, so a period the rollup has not reached yet falls back to the scan.
func (s *Service) loadViewsRollup(ctx context.Context, from, to string) (acc *viewsAcc, ok bool, err error) {
	f, e1 := time.Parse(dayLayout, from)
	t, e2 := time.Parse(dayLayout, to)
	if e1 != nil || e2 != nil || t.Before(f) {
		return nil, false, nil
	}
	want := int(t.Sub(f).Hours()/24) + 1
	rows, err := s.adminDB().QueryContext(ctx, `SELECT sessions, minutes, buckets, present, eligible FROM metrics_views_daily WHERE day >= ? AND day <= ?`, from, to)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	acc = newViewsAcc()
	got := 0
	for rows.Next() {
		var n int64
		var minutes float64
		var b, p, e string
		if err := rows.Scan(&n, &minutes, &b, &p, &e); err != nil {
			return nil, false, err
		}
		day := newViewsAcc()
		day.nSess, day.totalMin = n, minutes
		if json.Unmarshal([]byte(b), &day.buckets) != nil || json.Unmarshal([]byte(p), &day.present) != nil || json.Unmarshal([]byte(e), &day.eligible) != nil {
			return nil, false, nil
		}
		acc.merge(day)
		got++
	}
	if err := rows.Err(); err != nil || got != want {
		return nil, false, err
	}
	drows, err := s.adminDB().QueryContext(ctx, `SELECT anime_id, season_id, episode, title, title_at, sessions, abandoned, minutes FROM metrics_drop_daily WHERE day >= ? AND day <= ?`, from, to)
	if err != nil {
		return nil, false, err
	}
	defer drows.Close()
	for drows.Next() {
		d := &viewsDropAgg{}
		var mins string
		if err := drows.Scan(&d.anime, &d.season, &d.ep, &d.title, &d.titleAt, &d.sessions, &d.abandoned, &mins); err != nil {
			return nil, false, err
		}
		if json.Unmarshal([]byte(mins), &d.minutes) != nil {
			return nil, false, nil
		}
		one := newViewsAcc()
		one.drops[[3]int64{d.anime, d.season, d.ep}] = d
		acc.merge(one)
	}
	return acc, true, drows.Err()
}

// writeViewsDay replaces the day's /views aggregates inside the rollup transaction.
func writeViewsDay(ctx context.Context, tx *sql.Tx, day string, a *viewsAcc) error {
	for _, t := range []string{"metrics_views_daily", "metrics_drop_daily"} {
		if _, err := tx.ExecContext(ctx, `DELETE FROM `+t+` WHERE day = ?`, day); err != nil {
			return err
		}
	}
	b, _ := json.Marshal(a.buckets)
	p, _ := json.Marshal(a.present)
	e, _ := json.Marshal(a.eligible)
	if _, err := tx.ExecContext(ctx, `INSERT INTO metrics_views_daily(day, sessions, minutes, buckets, present, eligible) VALUES(?,?,?,?,?,?)`,
		day, a.nSess, a.totalMin, string(b), string(p), string(e)); err != nil {
		return err
	}
	for key, d := range a.drops {
		mins := d.minutes
		if mins == nil {
			mins = []float64{}
		}
		m, _ := json.Marshal(mins)
		if _, err := tx.ExecContext(ctx, `INSERT INTO metrics_drop_daily(day, anime_id, season_id, episode, title, title_at, sessions, abandoned, minutes) VALUES(?,?,?,?,?,?,?,?,?)`,
			day, key[0], key[1], key[2], d.title, d.titleAt, d.sessions, d.abandoned, string(m)); err != nil {
			return err
		}
	}
	return nil
}
