package admin

import (
	"context"
	"database/sql"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"time"
)

// catAcc accumulates what /catalog derives from watch_sessions, before the ?format= filter, so a
// day's rows can be summed and filtered exactly like a scan of the whole period.
type catAcc struct {
	formats map[string]int64          // sessions per format key (ignores the filter)
	animes  map[catAnimeKey]*catAnime // per anime and format key
	audio   map[[2]string]int64       // {format key, language}
	sub     map[[2]string]int64
	genres  map[[2]string]*catGenre // {format key, genre}
}

type catAnimeKey struct {
	anime int64
	key   string
}

type catAnime struct {
	sessions, completed int64
	secs                float64
	title               string
	titleAt, fmtAt      int64
}

type catGenre struct{ sessions, completed int64 }

func newCatAcc() *catAcc {
	return &catAcc{formats: map[string]int64{}, animes: map[catAnimeKey]*catAnime{}, audio: map[[2]string]int64{}, sub: map[[2]string]int64{}, genres: map[[2]string]*catGenre{}}
}

func catLang(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "" {
		return "unknown"
	}
	return v
}

// add folds in one watch session.
func (a *catAcc) add(aid int64, title, genresJSON, fmtRaw string, started int64, secs float64, comp int, audioLang, subLang string) {
	key := viewsFormatKey(fmtRaw)
	a.formats[key]++
	if secs < 0 {
		secs = 0
	}
	a.audio[[2]string{key, catLang(audioLang)}]++
	a.sub[[2]string{key, catLang(subLang)}]++
	var gl []string
	if json.Unmarshal([]byte(genresJSON), &gl) == nil {
		seen := map[string]bool{}
		for _, g := range gl {
			g = strings.TrimSpace(g)
			if g == "" || seen[g] {
				continue
			}
			seen[g] = true
			k := [2]string{key, g}
			x := a.genres[k]
			if x == nil {
				x = &catGenre{}
				a.genres[k] = x
			}
			x.sessions++
			if comp != 0 {
				x.completed++
			}
		}
	}
	if aid <= 0 {
		return
	}
	k := catAnimeKey{aid, key}
	x := a.animes[k]
	if x == nil {
		x = &catAnime{}
		a.animes[k] = x
	}
	x.sessions++
	x.secs += secs
	if comp != 0 {
		x.completed++
	}
	if title != "" && started >= x.titleAt {
		x.title, x.titleAt = title, started
	}
	if started >= x.fmtAt {
		x.fmtAt = started
	}
}

// merge folds another accumulator (a day row) into this one.
func (a *catAcc) merge(o *catAcc) {
	for k, n := range o.formats {
		a.formats[k] += n
	}
	for k, n := range o.audio {
		a.audio[k] += n
	}
	for k, n := range o.sub {
		a.sub[k] += n
	}
	for k, g := range o.genres {
		x := a.genres[k]
		if x == nil {
			x = &catGenre{}
			a.genres[k] = x
		}
		x.sessions += g.sessions
		x.completed += g.completed
	}
	for k, v := range o.animes {
		x := a.animes[k]
		if x == nil {
			cp := *v
			a.animes[k] = &cp
			continue
		}
		x.sessions += v.sessions
		x.completed += v.completed
		x.secs += v.secs
		if v.title != "" && v.titleAt >= x.titleAt {
			x.title, x.titleAt = v.title, v.titleAt
		}
		if v.fmtAt > x.fmtAt {
			x.fmtAt = v.fmtAt
		}
	}
}

// catView is an accumulator seen through one ?format= filter.
type catView struct {
	animes     map[int64]*viewsAnimeAgg
	formats    map[string]int64
	audio, sub map[string]int64
	genres     map[string]*catGenre
	total      int64
}

func (a *catAcc) view(format string) catView {
	v := catView{animes: map[int64]*viewsAnimeAgg{}, formats: a.formats, audio: map[string]int64{}, sub: map[string]int64{}, genres: map[string]*catGenre{}}
	for key, n := range a.formats {
		if viewsFormatMatches(format, key) {
			v.total += n
		}
	}
	for k, n := range a.audio {
		if viewsFormatMatches(format, k[0]) {
			v.audio[k[1]] += n
		}
	}
	for k, n := range a.sub {
		if viewsFormatMatches(format, k[0]) {
			v.sub[k[1]] += n
		}
	}
	for k, g := range a.genres {
		if !viewsFormatMatches(format, k[0]) {
			continue
		}
		x := v.genres[k[1]]
		if x == nil {
			x = &catGenre{}
			v.genres[k[1]] = x
		}
		x.sessions += g.sessions
		x.completed += g.completed
	}
	// Ties between two format keys are settled by key, so the answer never depends on map order.
	keys := make([]catAnimeKey, 0, len(a.animes))
	for k := range a.animes {
		if viewsFormatMatches(format, k.key) {
			keys = append(keys, k)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].anime != keys[j].anime {
			return keys[i].anime < keys[j].anime
		}
		return keys[i].key < keys[j].key
	})
	for _, k := range keys {
		g := a.animes[k]
		x := v.animes[k.anime]
		if x == nil {
			x = &viewsAnimeAgg{}
			v.animes[k.anime] = x
		}
		x.sessions += g.sessions
		x.secs += g.secs
		x.completed += g.completed
		if g.title != "" && g.titleAt >= x.titleAt {
			x.title, x.titleAt = g.title, g.titleAt
		}
		if g.fmtAt >= x.fmtAt {
			x.format, x.fmtAt = k.key, g.fmtAt
		}
	}
	return v
}

// scanCatalog aggregates the sessions started in [lo, hi) straight from watch_sessions.
func (s *Service) scanCatalog(ctx context.Context, lo, hi int64) (*catAcc, error) {
	rows, err := s.accountsDB().QueryContext(ctx, `SELECT anime_id, title, genres, format, started_at, watched_seconds, completed, audio_lang, sub_lang
		FROM watch_sessions WHERE started_at >= ? AND started_at < ?`, lo, hi)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	acc := newCatAcc()
	for rows.Next() {
		var aid, started int64
		var title, genresJSON, fmtRaw, al, sl string
		var secs float64
		var comp int
		if err := rows.Scan(&aid, &title, &genresJSON, &fmtRaw, &started, &secs, &comp, &al, &sl); err != nil {
			return nil, err
		}
		acc.add(aid, title, genresJSON, fmtRaw, started, secs, comp, al, sl)
	}
	return acc, rows.Err()
}

// viewsRollupCovers reports whether every day of [from, to] has been rolled up. The /views and
// /catalog rows of a day are written in one transaction, so one row per day is the marker.
func (s *Service) viewsRollupCovers(ctx context.Context, from, to string) (bool, error) {
	f, e1 := time.Parse(dayLayout, from)
	t, e2 := time.Parse(dayLayout, to)
	if e1 != nil || e2 != nil || t.Before(f) {
		return false, nil
	}
	var got int
	err := s.adminDB().QueryRowContext(ctx, `SELECT COUNT(*) FROM metrics_views_daily WHERE day >= ? AND day <= ?`, from, to).Scan(&got)
	return err == nil && got == int(t.Sub(f).Hours()/24)+1, err
}

// loadCatalogRollup merges the per-day rows of [from, to]; ok is false when a day is missing.
func (s *Service) loadCatalogRollup(ctx context.Context, from, to string) (*catAcc, bool, error) {
	covered, err := s.viewsRollupCovers(ctx, from, to)
	if err != nil || !covered {
		return nil, false, err
	}
	rows, err := s.adminDB().QueryContext(ctx, `SELECT kind, a, b, n, completed, secs, title, title_at, fmt_at FROM metrics_catalog_daily WHERE day >= ? AND day <= ?`, from, to)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	acc := newCatAcc()
	one := newCatAcc()
	for rows.Next() {
		var kind, a, b, title string
		var n, completed, titleAt, fmtAt int64
		var secs float64
		if err := rows.Scan(&kind, &a, &b, &n, &completed, &secs, &title, &titleAt, &fmtAt); err != nil {
			return nil, false, err
		}
		switch kind {
		case "format":
			one.formats[a] += n
		case "audio":
			one.audio[[2]string{a, b}] += n
		case "sub":
			one.sub[[2]string{a, b}] += n
		case "genre":
			one.genres[[2]string{a, b}] = &catGenre{n, completed}
		case "anime":
			id, err := strconv.ParseInt(a, 10, 64)
			if err != nil {
				return nil, false, nil
			}
			one.animes[catAnimeKey{id, b}] = &catAnime{sessions: n, completed: completed, secs: secs, title: title, titleAt: titleAt, fmtAt: fmtAt}
		}
		// Rows of one table are unique per (day, kind, a, b); merging after each row keeps days apart.
		acc.merge(one)
		one = newCatAcc()
	}
	return acc, true, rows.Err()
}

// writeCatalogDay replaces the day's /catalog aggregates inside the rollup transaction.
func writeCatalogDay(ctx context.Context, tx *sql.Tx, day string, a *catAcc) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM metrics_catalog_daily WHERE day = ?`, day); err != nil {
		return err
	}
	ins := func(kind, x, y string, n, completed int64, secs float64, title string, titleAt, fmtAt int64) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO metrics_catalog_daily(day, kind, a, b, n, completed, secs, title, title_at, fmt_at) VALUES(?,?,?,?,?,?,?,?,?,?)`,
			day, kind, x, y, n, completed, secs, title, titleAt, fmtAt)
		return err
	}
	for k, n := range a.formats {
		if err := ins("format", k, "", n, 0, 0, "", 0, 0); err != nil {
			return err
		}
	}
	for k, n := range a.audio {
		if err := ins("audio", k[0], k[1], n, 0, 0, "", 0, 0); err != nil {
			return err
		}
	}
	for k, n := range a.sub {
		if err := ins("sub", k[0], k[1], n, 0, 0, "", 0, 0); err != nil {
			return err
		}
	}
	for k, g := range a.genres {
		if err := ins("genre", k[0], k[1], g.sessions, g.completed, 0, "", 0, 0); err != nil {
			return err
		}
	}
	for k, x := range a.animes {
		if err := ins("anime", strconv.FormatInt(k.anime, 10), k.key, x.sessions, x.completed, x.secs, x.title, x.titleAt, x.fmtAt); err != nil {
			return err
		}
	}
	return nil
}
