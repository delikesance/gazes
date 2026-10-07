package admin

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Per-user-day activity and per-day concurrency peaks (metrics_user_daily, metrics_activity_daily),
// written by the rollup with the other metrics_* tables.
//
// Readers combine the rolled-up days with a scan of watch_sessions for the days the rollup cannot
// answer exactly: a day not rolled up yet, a day whose last pass ran before it was over (today),
// a day only partly inside the requested window, and a day containing a window boundary ("cut",
// e.g. now - 30 d). Every record then lies on one side of each cut, so a per-record test on its
// first session gives the same answer as a per-session test, and the figures match a full scan.

const actDay = int64(86400)

// actFloorDay is the UTC day index of a unix time (floor division, unlike SQL's truncation, which
// only differs before 1970).
func actFloorDay(ts int64) int64 {
	d := ts / actDay
	if ts%actDay < 0 {
		d--
	}
	return d
}

func actDayStr(d int64) string { return time.Unix(d*actDay, 0).UTC().Format(dayLayout) }

// userDayAgg is one user's activity on one day (rollup side).
type userDayAgg struct {
	sessions    int64
	secs        float64
	first, last int64
	eps         map[string]struct{}
}

func (a *userDayAgg) add(started int64, secs float64, season int64, ep int) {
	if a.sessions == 0 || started < a.first {
		a.first = started
	}
	if a.sessions == 0 || started > a.last {
		a.last = started
	}
	a.sessions++
	a.secs += max(secs, 0)
	if a.eps == nil {
		a.eps = map[string]struct{}{}
	}
	a.eps[actEpisodeKey(season, int64(ep))] = struct{}{}
}

// actEpisodeKey matches SQLite's `season_id || ':' || episode`.
func actEpisodeKey(season, ep int64) string {
	return strconv.FormatInt(season, 10) + ":" + strconv.FormatInt(ep, 10)
}

// peakDay is the concurrency summary of one day: peakOwn counts only the sessions started that
// day, peakFull also those started the day before (both measured at instants of that day), and
// maxEnd is the latest end (updated_at) of the sessions started that day.
type peakDay struct{ own, full, maxEnd int64 }

// actInterval is one session's [start, end] activity, end = MAX(updated_at, started_at).
type actInterval struct{ a, b int64 }

// actPeakIn is the highest number of intervals containing one instant of [tLo, tHi).
func actPeakIn(ivs []actInterval, tLo, tHi int64) int64 {
	type ev struct {
		t int64
		d int
	}
	evs := make([]ev, 0, 2*len(ivs))
	for _, iv := range ivs {
		if iv.a >= tHi || iv.b < tLo {
			continue
		}
		evs = append(evs, ev{max(iv.a, tLo), +1}, ev{iv.b + 1, -1})
	}
	// Ends sort before starts at the same instant (b+1 closes an interval inclusive of b).
	sort.Slice(evs, func(i, j int) bool {
		if evs[i].t != evs[j].t {
			return evs[i].t < evs[j].t
		}
		return evs[i].d < evs[j].d
	})
	var cur, peak int64
	for _, e := range evs {
		cur += int64(e.d)
		peak = max(peak, cur)
	}
	return peak
}

// rollupPeaks computes the peakDay of every day of [lo, hi) (whole UTC days), reading the
// sessions started from the day before lo.
func (r *Rollup) rollupPeaks(ctx context.Context, lo, hi int64) (map[string]peakDay, error) {
	rows, err := r.accounts.QueryContext(ctx, `SELECT started_at, MAX(updated_at, started_at) FROM watch_sessions
		WHERE started_at >= ? AND started_at < ?`, lo-actDay, hi)
	if err != nil {
		return nil, err
	}
	byDay := map[int64][]actInterval{}
	for rows.Next() {
		var iv actInterval
		if err := rows.Scan(&iv.a, &iv.b); err != nil {
			rows.Close()
			return nil, err
		}
		d := actFloorDay(iv.a)
		byDay[d] = append(byDay[d], iv)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := map[string]peakDay{}
	for d := actFloorDay(lo); d < actFloorDay(hi); d++ {
		var p peakDay
		for _, iv := range byDay[d] {
			p.maxEnd = max(p.maxEnd, iv.b)
		}
		p.own = actPeakIn(byDay[d], d*actDay, (d+1)*actDay)
		p.full = actPeakIn(append(append([]actInterval{}, byDay[d-1]...), byDay[d]...), d*actDay, (d+1)*actDay)
		out[actDayStr(d)] = p
	}
	return out, nil
}

// writeActivityDay replaces one day's per-user rows and its coverage marker.
func writeActivityDay(ctx context.Context, tx *sql.Tx, day string, users map[int64]*userDayAgg, p peakDay, computed int64) error {
	for _, t := range []string{"metrics_user_daily", "metrics_activity_daily"} {
		if _, err := tx.ExecContext(ctx, `DELETE FROM `+t+` WHERE day = ?`, day); err != nil {
			return err
		}
	}
	ids := make([]int64, 0, len(users))
	for id := range users {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		u := users[id]
		eps := make([]string, 0, len(u.eps))
		for k := range u.eps {
			eps = append(eps, k)
		}
		sort.Strings(eps)
		b, _ := json.Marshal(eps)
		if _, err := tx.ExecContext(ctx, `INSERT INTO metrics_user_daily(day, user_id, sessions, watch_seconds, first_at, last_at, episodes)
			VALUES(?,?,?,?,?,?,?)`, day, id, u.sessions, u.secs, u.first, u.last, string(b)); err != nil {
			return err
		}
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO metrics_activity_daily(day, computed_at, peak_own, peak_full, max_end) VALUES(?,?,?,?,?)`,
		day, computed, p.own, p.full, p.maxEnd)
	return err
}

// actRec is the activity of one user on one day, or on the part of a day between two cuts.
type actRec struct {
	uid, day    int64 // day: UTC day index
	n           int64
	secs        float64
	first, last int64
	eps         []string // distinct "season:episode" keys, only when asked for
}

type actMarker struct {
	computed int64
	p        peakDay
}

// activityMarkers returns the coverage marker of every rolled-up day.
func (s *Service) activityMarkers(ctx context.Context) (map[int64]actMarker, error) {
	rows, err := s.adminDB().QueryContext(ctx, `SELECT day, computed_at, peak_own, peak_full, max_end FROM metrics_activity_daily`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]actMarker{}
	for rows.Next() {
		var day string
		var m actMarker
		if err := rows.Scan(&day, &m.computed, &m.p.own, &m.p.full, &m.p.maxEnd); err != nil {
			return nil, err
		}
		t, err := time.Parse(dayLayout, day)
		if err != nil {
			continue
		}
		out[actFloorDay(t.Unix())] = m
	}
	return out, rows.Err()
}

// actFinal reports whether day d's rollup ran after the day was over.
func actFinal(m actMarker, ok bool, d int64) bool { return ok && m.computed >= (d+1)*actDay }

// actUIDChunk bounds the IN (...) lists of a per-user filter.
const actUIDChunk = 500

// forUIDChunks calls fn with an SQL condition on col and its arguments: no condition when uids
// is nil, one call per chunk otherwise (none for an empty, non-nil list).
func forUIDChunks(uids []int64, col string, fn func(cond string, args []any) error) error {
	if uids == nil {
		return fn("", nil)
	}
	for i := 0; i < len(uids); i += actUIDChunk {
		part := uids[i:min(i+actUIDChunk, len(uids))]
		args := make([]any, len(part))
		for j, u := range part {
			args[j] = u
		}
		if err := fn(` AND `+col+` IN (`+strings.TrimSuffix(strings.Repeat("?,", len(part)), ",")+`)`, args); err != nil {
			return err
		}
	}
	return nil
}

// userActivity returns the activity of the sessions started in [lo, hi) (math.MinInt64 /
// math.MaxInt64 for an open bound), split at every cut, restricted to uids when non-nil, with
// the distinct episodes when eps is set. Records of one user may repeat a day (one per side of a
// cut); callers sum them.
func (s *Service) userActivity(ctx context.Context, lo, hi int64, cuts, uids []int64, eps bool) ([]actRec, error) {
	markers, err := s.activityMarkers(ctx)
	if err != nil {
		return nil, err
	}
	cutDay := map[int64]bool{}
	for _, c := range cuts {
		cutDay[actFloorDay(c)] = true
	}
	var rolled []int64
	for d, m := range markers {
		if actFinal(m, true, d) && !cutDay[d] && d*actDay >= lo && (d+1)*actDay <= hi {
			rolled = append(rolled, d)
		}
	}
	sort.Slice(rolled, func(i, j int) bool { return rolled[i] < rolled[j] })

	var out []actRec
	if len(rolled) > 0 {
		want := make(map[string]int64, len(rolled))
		for _, d := range rolled {
			want[actDayStr(d)] = d
		}
		q := `SELECT day, user_id, sessions, watch_seconds, first_at, last_at, episodes FROM metrics_user_daily WHERE day >= ? AND day <= ?`
		err := forUIDChunks(uids, "user_id", func(cond string, args []any) error {
			rows, err := s.adminDB().QueryContext(ctx, q+cond, append([]any{actDayStr(rolled[0]), actDayStr(rolled[len(rolled)-1])}, args...)...)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var day, epsJSON string
				var r actRec
				if err := rows.Scan(&day, &r.uid, &r.n, &r.secs, &r.first, &r.last, &epsJSON); err != nil {
					return err
				}
				d, ok := want[day]
				if !ok {
					continue
				}
				r.day = d
				if eps && json.Unmarshal([]byte(epsJSON), &r.eps) != nil {
					return fmt.Errorf("metrics_user_daily %s/%d: bad episodes", day, r.uid)
				}
				out = append(out, r)
			}
			return rows.Err()
		})
		if err != nil {
			return nil, err
		}
	}

	// Scan the gaps between rolled-up days.
	cur := lo
	scan := func(a, b int64) error {
		if a >= b {
			return nil
		}
		raw, err := s.scanUserActivity(ctx, a, b, cuts, uids, eps)
		out = append(out, raw...)
		return err
	}
	for _, d := range rolled {
		if err := scan(cur, d*actDay); err != nil {
			return nil, err
		}
		cur = (d + 1) * actDay
	}
	if err := scan(cur, hi); err != nil {
		return nil, err
	}
	return out, nil
}

// scanUserActivity reads watch_sessions started in [lo, hi), grouped by user, day and side of
// every cut.
func (s *Service) scanUserActivity(ctx context.Context, lo, hi int64, cuts, uids []int64, eps bool) ([]actRec, error) {
	side := "0"
	var sideArgs []any
	for _, c := range cuts {
		side += " + (started_at >= ?)"
		sideArgs = append(sideArgs, c)
	}
	epsCol := "''"
	if eps {
		epsCol = "COALESCE(group_concat(DISTINCT season_id || ':' || episode), '')"
	}
	var out []actRec
	err := forUIDChunks(uids, "user_id", func(cond string, args []any) error {
		q := `SELECT user_id, started_at / 86400 AS d, ` + side + ` AS side, COUNT(*), COALESCE(SUM(MAX(watched_seconds, 0)), 0),
			MIN(started_at), MAX(started_at), ` + epsCol + `
			FROM watch_sessions WHERE started_at >= ? AND started_at < ?` + cond + ` GROUP BY user_id, d, side`
		all := append(append(append([]any{}, sideArgs...), lo, hi), args...)
		rows, err := s.accountsDB().QueryContext(ctx, q, all...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r actRec
			var sd int64
			var epsList string
			if err := rows.Scan(&r.uid, &r.day, &sd, &r.n, &r.secs, &r.first, &r.last, &epsList); err != nil {
				return err
			}
			if eps && epsList != "" {
				r.eps = strings.Split(epsList, ",")
			}
			out = append(out, r)
		}
		return rows.Err()
	})
	return out, err
}

// actDistinctUsers counts the users with a record whose first session is in [lo, hi).
func actDistinctUsers(recs []actRec, lo, hi int64) int64 {
	seen := map[int64]struct{}{}
	for _, r := range recs {
		if r.first >= lo && r.first < hi {
			seen[r.uid] = struct{}{}
		}
	}
	return int64(len(seen))
}

// peakConcurrent is pbPeakConcurrent from the per-day peaks: the first day of the period counts
// only its own sessions (those started before the period are excluded), every later day also
// the sessions started the day before. That is exact as long as no session of the period lasts
// into a second day after its start; a period holding such a session falls back to the sweep.
// Days the rollup cannot answer exactly (not rolled up, rolled up before they were over) are
// swept from watch_sessions.
func (s *Service) peakConcurrent(r *http.Request, from, to int64) (peak int, truncated bool, err error) {
	ctx := r.Context()
	first, last := actFloorDay(from), actFloorDay(to)-1
	markers, err := s.activityMarkers(ctx)
	if err != nil {
		return 0, false, err
	}
	rolled := func(d int64) bool { m, ok := markers[d]; return actFinal(m, ok, d) }
	var rawDays []int64
	for d := first; d <= last; d++ {
		if !rolled(d) {
			rawDays = append(rawDays, d)
			continue
		}
		m := markers[d]
		if d+2 > last {
			continue
		}
		reach := (d + 2) * actDay
		// A session of d may reach d+2: seen in max_end, or still running when d was last rolled up.
		if m.p.maxEnd >= reach || (m.p.maxEnd >= m.computed-pbActiveWindow && m.computed < reach) {
			return s.pbPeakConcurrent(r, from, to)
		}
	}

	var best int64
	for d := first; d <= last; d++ {
		if !rolled(d) {
			continue
		}
		if d == first {
			best = max(best, markers[d].p.own)
		} else {
			best = max(best, markers[d].p.full)
		}
	}
	if len(rawDays) > 0 {
		// One scan from the day before the first raw day (never before the period).
		lo := max(from, (rawDays[0]-1)*actDay)
		rows, err := s.accountsDB().QueryContext(ctx,
			`SELECT started_at, MAX(updated_at, started_at) FROM watch_sessions WHERE started_at >= ? AND started_at < ?`, lo, to)
		if err != nil {
			return 0, false, err
		}
		byDay := map[int64][]actInterval{}
		for rows.Next() {
			var iv actInterval
			if err := rows.Scan(&iv.a, &iv.b); err != nil {
				rows.Close()
				return 0, false, err
			}
			d := actFloorDay(iv.a)
			if iv.b >= (d+2)*actDay && d+2 <= last {
				rows.Close()
				return s.pbPeakConcurrent(r, from, to)
			}
			byDay[d] = append(byDay[d], iv)
		}
		if err := rows.Close(); err != nil {
			return 0, false, err
		}
		if err := rows.Err(); err != nil {
			return 0, false, err
		}
		for _, d := range rawDays {
			ivs := byDay[d]
			if d > first {
				ivs = append(append([]actInterval{}, byDay[d-1]...), ivs...)
			}
			best = max(best, actPeakIn(ivs, d*actDay, (d+1)*actDay))
		}
	}
	if best > math.MaxInt32 {
		best = math.MaxInt32
	}
	return int(best), false, nil
}
