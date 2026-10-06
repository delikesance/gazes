package admin

import (
	"context"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

// mountViews registers the admin read endpoints of this file's agent (views = overview/views/catalog).
// Only that agent edits this file (and its tests).
func (s *Service) mountViews(r chi.Router) {
	r.With(s.AuthCached(ScopeMetricsRead, ResponseCacheTTL)).Get("/overview", s.handleOverview)
	r.With(s.AuthCached(ScopeMetricsRead, ResponseCacheTTL)).Get("/views", s.handleViews)
	r.With(s.AuthCached(ScopeMetricsRead, ResponseCacheTTL)).Get("/catalog", s.handleCatalog)
}

var viewsWeekdayNames = [7]string{"lun", "mar", "mer", "jeu", "ven", "sam", "dim"}

// viewsWeekday is the weekday of a YYYY-MM-DD day, Monday = 0 .. Sunday = 6.
func viewsWeekday(day string) int {
	t, err := time.Parse(dayLayout, day)
	if err != nil {
		return 0
	}
	return (int(t.Weekday()) + 6) % 7
}

// viewsDays lists the UTC days of [from, to], both inclusive.
func viewsDays(from, to string) []string {
	f, err1 := time.Parse(dayLayout, from)
	t, err2 := time.Parse(dayLayout, to)
	if err1 != nil || err2 != nil {
		return nil
	}
	var out []string
	for d := f; !d.After(t); d = d.AddDate(0, 0, 1) {
		out = append(out, d.Format(dayLayout))
	}
	return out
}

func viewsPct(num, den float64) float64 {
	if den == 0 {
		return 0
	}
	return num / den * 100
}

func viewsRound(v float64, dec int) float64 {
	p := math.Pow10(dec)
	return math.Round(v*p) / p
}

type viewsKPI struct {
	Value    float64  `json:"value"`
	Previous float64  `json:"previous"`
	DeltaPct *float64 `json:"delta_pct"`
}

func viewsMakeKPI(cur, prev float64) viewsKPI {
	return viewsKPI{Value: viewsRound(cur, 2), Previous: viewsRound(prev, 2), DeltaPct: delta(cur, prev)}
}

type viewsDailyRow struct {
	sessions, seconds, active, newUsers, completed, total int64
}

// viewsLoadDaily reads metrics_daily for [from, to] keyed by day.
func (s *Service) viewsLoadDaily(ctx context.Context, from, to string) (map[string]viewsDailyRow, error) {
	rows, err := s.adminDB().QueryContext(ctx, `SELECT day, sessions, watch_seconds, active_users, new_users, completed_sessions, total_users
		FROM metrics_daily WHERE day >= ? AND day <= ?`, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]viewsDailyRow{}
	for rows.Next() {
		var d string
		var r viewsDailyRow
		if err := rows.Scan(&d, &r.sessions, &r.seconds, &r.active, &r.newUsers, &r.completed, &r.total); err != nil {
			return nil, err
		}
		out[d] = r
	}
	return out, rows.Err()
}

func viewsSum(m map[string]viewsDailyRow, days []string) (r viewsDailyRow) {
	for _, d := range days {
		x := m[d]
		r.sessions += x.sessions
		r.seconds += x.seconds
		r.active += x.active
		r.newUsers += x.newUsers
		r.completed += x.completed
	}
	return r
}

// viewsTotalUsersAt is the cumulative user count at the end of day (carried forward from the
// last known rollup day when the day itself has no row).
func (s *Service) viewsTotalUsersAt(ctx context.Context, day string) (int64, error) {
	var n int64
	err := s.adminDB().QueryRowContext(ctx, `SELECT COALESCE((SELECT total_users FROM metrics_daily WHERE day <= ? ORDER BY day DESC LIMIT 1), 0)`, day).Scan(&n)
	return n, err
}

type viewsHourRow struct {
	day      string
	hour     int
	sessions int64
}

func (s *Service) viewsLoadHourly(ctx context.Context, from, to string) ([]viewsHourRow, error) {
	rows, err := s.adminDB().QueryContext(ctx, `SELECT day, hour, sessions FROM metrics_hourly WHERE day >= ? AND day <= ?`, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []viewsHourRow
	for rows.Next() {
		var h viewsHourRow
		if err := rows.Scan(&h.day, &h.hour, &h.sessions); err != nil {
			return nil, err
		}
		if h.hour >= 0 && h.hour < 24 {
			out = append(out, h)
		}
	}
	return out, rows.Err()
}

func viewsServerError(w http.ResponseWriter) {
	writeAPIError(w, http.StatusInternalServerError, "server_error", "could not compute the metrics")
}

// handleOverview feeds the "Vue d'ensemble" page of the admin design: KPI cards, daily
// series, top anime, weekday x hour heatmap and the peak slot. Reads the rollups only.
//
// GET /api/v1/admin/overview?period=7|30|90
func (s *Service) handleOverview(w http.ResponseWriter, r *http.Request) {
	p, err := parsePeriod(r, s.now())
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "bad_period", err.Error())
		return
	}
	ctx := r.Context()
	daily, err := s.viewsLoadDaily(ctx, p.PrevFrom, p.To)
	if err != nil {
		viewsServerError(w)
		return
	}
	curDays, prevDays := viewsDays(p.From, p.To), viewsDays(p.PrevFrom, p.PrevTo)
	cur, prev := viewsSum(daily, curDays), viewsSum(daily, prevDays)
	totalCur, err1 := s.viewsTotalUsersAt(ctx, p.To)
	totalPrev, err2 := s.viewsTotalUsersAt(ctx, p.PrevTo)
	if err1 != nil || err2 != nil {
		viewsServerError(w)
		return
	}
	n := float64(p.Days)
	kpis := map[string]viewsKPI{
		"total_users":      viewsMakeKPI(float64(totalCur), float64(totalPrev)),
		"new_users":        viewsMakeKPI(float64(cur.newUsers), float64(prev.newUsers)),
		"sessions":         viewsMakeKPI(float64(cur.sessions), float64(prev.sessions)),
		"watch_hours":      viewsMakeKPI(float64(cur.seconds)/3600, float64(prev.seconds)/3600),
		"active_users_avg": viewsMakeKPI(float64(cur.active)/n, float64(prev.active)/n),
		"completion_pct":   viewsMakeKPI(viewsPct(float64(cur.completed), float64(cur.sessions)), viewsPct(float64(prev.completed), float64(prev.sessions))),
	}

	type dayPoint struct {
		Day         string `json:"day"`
		Sessions    int64  `json:"sessions"`
		NewUsers    int64  `json:"new_users"`
		ActiveUsers int64  `json:"active_users"`
	}
	series := make([]dayPoint, 0, len(curDays))
	for _, d := range curDays {
		x := daily[d]
		series = append(series, dayPoint{d, x.sessions, x.newUsers, x.active})
	}

	top, err := s.viewsTopAnime(ctx, p, cur.sessions)
	if err != nil {
		viewsServerError(w)
		return
	}

	hours, err := s.viewsLoadHourly(ctx, p.From, p.To)
	if err != nil {
		viewsServerError(w)
		return
	}
	var raw [7][24]int64
	var maxCell int64
	for _, h := range hours {
		wd := viewsWeekday(h.day)
		raw[wd][h.hour] += h.sessions
		maxCell = max(maxCell, raw[wd][h.hour])
	}
	norm := make([][]float64, 7)
	rawOut := make([][]int64, 7)
	peakWD, peakH := -1, -1
	for wd := range raw {
		norm[wd] = make([]float64, 24)
		rawOut[wd] = raw[wd][:]
		for h := range raw[wd] {
			if maxCell > 0 {
				norm[wd][h] = viewsRound(float64(raw[wd][h])/float64(maxCell), 4)
			}
			if maxCell > 0 && raw[wd][h] == maxCell && peakWD < 0 {
				peakWD, peakH = wd, h
			}
		}
	}
	var peak any
	if peakWD >= 0 {
		peak = map[string]any{"weekday": peakWD, "weekday_name": viewsWeekdayNames[peakWD], "hour": peakH, "sessions": maxCell}
	}

	writeData(w, p, map[string]any{
		"kpis":      kpis,
		"series":    series,
		"top_anime": top,
		"heatmap": map[string]any{
			"weekday_start": "monday", "hours_tz": "UTC", "weekdays": viewsWeekdayNames,
			"normalized": norm, "raw": rawOut,
		},
		"peak": peak,
	})
}

type viewsTopAnime struct {
	AnimeID  int64    `json:"anime_id"`
	Title    string   `json:"title"`
	Sessions int64    `json:"sessions"`
	SharePct float64  `json:"share_pct"`
	DeltaPct *float64 `json:"delta_pct"`
}

func (s *Service) viewsTopAnime(ctx context.Context, p Period, totalSessions int64) ([]viewsTopAnime, error) {
	rows, err := s.adminDB().QueryContext(ctx, `SELECT a.anime_id, SUM(a.sessions) AS n,
			COALESCE((SELECT t.title FROM metrics_anime_daily t WHERE t.anime_id = a.anime_id AND t.title <> '' AND t.day >= ? AND t.day <= ? ORDER BY t.day DESC LIMIT 1), '')
		FROM metrics_anime_daily a WHERE a.day >= ? AND a.day <= ?
		GROUP BY a.anime_id ORDER BY n DESC, a.anime_id ASC LIMIT 7`, p.From, p.To, p.From, p.To)
	if err != nil {
		return nil, err
	}
	out := []viewsTopAnime{}
	for rows.Next() {
		var t viewsTopAnime
		if err := rows.Scan(&t.AnimeID, &t.Sessions, &t.Title); err != nil {
			rows.Close()
			return nil, err
		}
		t.SharePct = viewsRound(viewsPct(float64(t.Sessions), float64(totalSessions)), 2)
		out = append(out, t)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		var prev int64
		if err := s.adminDB().QueryRowContext(ctx, `SELECT COALESCE(SUM(sessions), 0) FROM metrics_anime_daily WHERE anime_id = ? AND day >= ? AND day <= ?`,
			out[i].AnimeID, p.PrevFrom, p.PrevTo).Scan(&prev); err != nil {
			return nil, err
		}
		out[i].DeltaPct = delta(float64(out[i].Sessions), float64(prev))
	}
	return out, nil
}

// viewsPeriodBounds is the [lo, hi) unix range of the period's UTC days.
func viewsPeriodBounds(from, to string) (int64, int64) {
	f, _ := time.Parse(dayLayout, from)
	t, _ := time.Parse(dayLayout, to)
	return f.Unix(), t.AddDate(0, 0, 1).Unix()
}

type viewsDropAgg struct {
	title               string
	titleAt             int64
	anime, season, ep   int64
	sessions, abandoned int64
	minutes             []float64
}

// handleViews feeds the "Visionnages" page of the admin design: sessions per day against the
// previous period, session length, completion by weekday, hours, time zones, resume
// behaviour, retention curve and the episodes viewers drop.
//
// GET /api/v1/admin/views?period=7|30|90
func (s *Service) handleViews(w http.ResponseWriter, r *http.Request) {
	p, err := parsePeriod(r, s.now())
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "bad_period", err.Error())
		return
	}
	ctx := r.Context()

	// Rollup-backed parts.
	daily, err := s.viewsLoadDaily(ctx, p.PrevFrom, p.To)
	if err != nil {
		viewsServerError(w)
		return
	}
	curDays, prevDays := viewsDays(p.From, p.To), viewsDays(p.PrevFrom, p.PrevTo)
	type pair struct {
		Day          string `json:"day"`
		Sessions     int64  `json:"sessions"`
		PrevDay      string `json:"prev_day"`
		PrevSessions int64  `json:"prev_sessions"`
	}
	sessSeries := make([]pair, 0, len(curDays))
	var wdSess, wdDone [7]int64
	for i, d := range curDays {
		sessSeries = append(sessSeries, pair{d, daily[d].sessions, prevDays[i], daily[prevDays[i]].sessions})
		wd := viewsWeekday(d)
		wdSess[wd] += daily[d].sessions
		wdDone[wd] += daily[d].completed
	}
	type wdRow struct {
		Weekday       int     `json:"weekday"`
		Name          string  `json:"name"`
		Sessions      int64   `json:"sessions"`
		Completed     int64   `json:"completed"`
		CompletionPct float64 `json:"completion_pct"`
	}
	completionByWD := make([]wdRow, 7)
	for i := range completionByWD {
		completionByWD[i] = wdRow{i, viewsWeekdayNames[i], wdSess[i], wdDone[i], viewsRound(viewsPct(float64(wdDone[i]), float64(wdSess[i])), 2)}
	}
	hours, err := s.viewsLoadHourly(ctx, p.From, p.To)
	if err != nil {
		viewsServerError(w)
		return
	}
	byHour := make([]map[string]int64, 24)
	for h := range byHour {
		byHour[h] = map[string]int64{"hour": int64(h), "sessions": 0}
	}
	for _, h := range hours {
		byHour[h.hour]["sessions"] += h.sessions
	}

	// Session length, retention curve and drop episodes come from the per-day rollup when every day
	// of the period is there, otherwise from a period-bounded scan of watch_sessions.
	lo, hi := viewsPeriodBounds(p.From, p.To)
	acc, haveRollup, err := s.loadViewsRollup(ctx, p.From, p.To)
	if err != nil {
		viewsServerError(w)
		return
	}
	if !haveRollup {
		if acc, err = s.scanViews(ctx, lo, hi); err != nil {
			viewsServerError(w)
			return
		}
	}
	bucketNames := []string{"<5", "5-15", "15-30", "30-60", ">60"}
	buckets, nSess, totalMin, present, eligible, drops := acc.buckets, acc.nSess, acc.totalMin, acc.present, acc.eligible, acc.drops
	type bucket struct {
		Range    string  `json:"range_minutes"`
		Sessions int64   `json:"sessions"`
		SharePct float64 `json:"share_pct"`
	}
	dist := make([]bucket, 5)
	for i := range dist {
		dist[i] = bucket{bucketNames[i], buckets[i], viewsRound(viewsPct(float64(buckets[i]), float64(nSess)), 2)}
	}
	avgMin := 0.0
	if nSess > 0 {
		avgMin = totalMin / float64(nSess)
	}

	type decile struct {
		Decile      int     `json:"decile"`
		FromPct     int     `json:"from_pct"`
		Present     int64   `json:"present"`
		Eligible    int64   `json:"eligible"`
		RetainedPct float64 `json:"retained_pct"`
	}
	curve := make([]decile, 10)
	for k := range curve {
		curve[k] = decile{k, k * 10, present[k], eligible[k], viewsRound(viewsPct(float64(present[k]), float64(eligible[k])), 2)}
	}
	type dropPoint struct {
		FromDecile int     `json:"from_decile"`
		ToDecile   int     `json:"to_decile"`
		DropPts    float64 `json:"drop_pts"`
	}
	var dps []dropPoint
	for k := 0; k < 9; k++ {
		if eligible[k] == 0 || eligible[k+1] == 0 {
			continue
		}
		if d := curve[k].RetainedPct - curve[k+1].RetainedPct; d > 0 {
			dps = append(dps, dropPoint{k, k + 1, viewsRound(d, 2)})
		}
	}
	sort.SliceStable(dps, func(i, j int) bool { return dps[i].DropPts > dps[j].DropPts })
	if len(dps) > 3 {
		dps = dps[:3]
	}
	if dps == nil {
		dps = []dropPoint{}
	}

	dropEps := viewsDropEpisodes(drops)

	tz, err := s.viewsTimezones(ctx, lo, hi, nSess)
	if err != nil {
		viewsServerError(w)
		return
	}
	epu, err := s.viewsEpisodesPerUser(ctx, lo, hi)
	if err != nil {
		viewsServerError(w)
		return
	}
	plo, phi := viewsPeriodBounds(p.PrevFrom, p.PrevTo)
	epuPrev, err := s.viewsEpisodesPerUser(ctx, plo, phi)
	if err != nil {
		viewsServerError(w)
		return
	}
	resume, first, err := s.viewsResume(ctx, lo, hi)
	if err != nil {
		viewsServerError(w)
		return
	}

	writeData(w, p, map[string]any{
		"sessions_series":          sessSeries,
		"avg_session_minutes":      viewsRound(avgMin, 2),
		"session_length_buckets":   dist,
		"completion_by_weekday":    completionByWD,
		"sessions_by_hour":         byHour,
		"by_timezone":              tz,
		"episodes_per_active_user": viewsMakeKPI(epu, epuPrev),
		"resume_vs_first": map[string]any{
			"resume": resume, "first": first,
			"resume_pct": viewsRound(viewsPct(float64(resume), float64(resume+first)), 2),
		},
		"retention_curve": curve,
		"drop_points":     dps,
		"drop_episodes":   dropEps,
	})
}

// viewsDropMinSessions is the sample below which an episode is not ranked as a drop episode.
const viewsDropMinSessions = 3

func viewsDropEpisodes(drops map[[3]int64]*viewsDropAgg) []map[string]any {
	list := make([]*viewsDropAgg, 0, len(drops))
	for _, d := range drops {
		if d.sessions >= viewsDropMinSessions && d.abandoned > 0 {
			list = append(list, d)
		}
	}
	pct := func(d *viewsDropAgg) float64 { return viewsPct(float64(d.abandoned), float64(d.sessions)) }
	sort.Slice(list, func(i, j int) bool {
		a, b := list[i], list[j]
		if pa, pb := pct(a), pct(b); pa != pb {
			return pa > pb
		}
		if a.sessions != b.sessions {
			return a.sessions > b.sessions
		}
		if a.anime != b.anime {
			return a.anime < b.anime
		}
		if a.season != b.season {
			return a.season < b.season
		}
		return a.ep < b.ep
	})
	if len(list) > 10 {
		list = list[:10]
	}
	out := make([]map[string]any, 0, len(list))
	for _, d := range list {
		sort.Float64s(d.minutes)
		med := d.minutes[len(d.minutes)/2]
		if len(d.minutes)%2 == 0 {
			med = (d.minutes[len(d.minutes)/2-1] + d.minutes[len(d.minutes)/2]) / 2
		}
		out = append(out, map[string]any{
			"anime_id": d.anime, "title": d.title, "season_id": d.season, "episode": d.ep,
			"sessions": d.sessions, "abandon_pct": viewsRound(pct(d), 2), "median_drop_minute": viewsRound(med, 2),
		})
	}
	return out
}

// viewsTimezones groups the period's sessions by tz_offset (minutes east of UTC as reported
// by the player), top 8.
func (s *Service) viewsTimezones(ctx context.Context, lo, hi, total int64) ([]map[string]any, error) {
	rows, err := s.accountsDB().QueryContext(ctx, `SELECT tz_offset, COUNT(*) AS n FROM watch_sessions
		WHERE started_at >= ? AND started_at < ? GROUP BY tz_offset ORDER BY n DESC, tz_offset ASC LIMIT 8`, lo, hi)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var off, n int64
		if err := rows.Scan(&off, &n); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{"tz_offset": off, "sessions": n, "share_pct": viewsRound(viewsPct(float64(n), float64(total)), 2)})
	}
	return out, rows.Err()
}

// viewsEpisodesPerUser is distinct episodes watched divided by distinct active users.
func (s *Service) viewsEpisodesPerUser(ctx context.Context, lo, hi int64) (float64, error) {
	var eps, users int64
	err := s.accountsDB().QueryRowContext(ctx, `SELECT COUNT(*), COUNT(DISTINCT user_id) FROM
		(SELECT DISTINCT user_id, season_id, episode FROM watch_sessions WHERE started_at >= ? AND started_at < ?)`, lo, hi).Scan(&eps, &users)
	if err != nil || users == 0 {
		return 0, err
	}
	return float64(eps) / float64(users), nil
}

// viewsResume counts sessions of the period whose episode the same user had already started
// earlier (resume) and the others (first).
func (s *Service) viewsResume(ctx context.Context, lo, hi int64) (resume, first int64, err error) {
	var total int64
	err = s.accountsDB().QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(SUM(EXISTS(
			SELECT 1 FROM watch_sessions p WHERE p.user_id = w.user_id AND p.season_id = w.season_id AND p.episode = w.episode
			AND (p.started_at < w.started_at OR (p.started_at = w.started_at AND p.session_id < w.session_id)))), 0)
		FROM watch_sessions w WHERE w.started_at >= ? AND w.started_at < ?`, lo, hi).Scan(&total, &resume)
	return resume, total - resume, err
}

type viewsAnimeAgg struct {
	title     string
	titleAt   int64
	format    string
	fmtAt     int64
	sessions  int64
	secs      float64
	completed int64
}

type viewsShare struct {
	Key      string  `json:"key"`
	Sessions int64   `json:"sessions"`
	SharePct float64 `json:"share_pct"`
}

func viewsShares(m map[string]int64, total int64, limit int) []viewsShare {
	out := make([]viewsShare, 0, len(m))
	for k, n := range m {
		out = append(out, viewsShare{k, n, viewsRound(viewsPct(float64(n), float64(total)), 2)})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Sessions != out[j].Sessions {
			return out[i].Sessions > out[j].Sessions
		}
		return out[i].Key < out[j].Key
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func viewsFormatKey(f string) string {
	f = strings.ToLower(strings.TrimSpace(f))
	if f == "" {
		return "unknown"
	}
	return f
}

// viewsFormatMatches maps the ?format= filter onto the free-form format stored on sessions.
func viewsFormatMatches(filter, key string) bool {
	switch filter {
	case "all":
		return true
	case "tv":
		return key == "tv" || key == "tv_short" || key == "tv short"
	case "movie":
		return key == "movie"
	case "ova":
		return key == "ova" || key == "ona" || key == "special"
	}
	return false
}

func viewsMedian(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	c := append([]float64(nil), v...)
	sort.Float64s(c)
	if len(c)%2 == 1 {
		return c[len(c)/2]
	}
	return (c[len(c)/2-1] + c[len(c)/2]) / 2
}

// handleCatalog feeds the "Catalogue" page of the admin design: the paginated anime table,
// the popularity/completion quadrant, genre, format and language shares, and new series vs
// back catalogue.
//
// GET /api/v1/admin/catalog?period=7|30|90&format=tv|movie|ova|all&limit=&offset=
func (s *Service) handleCatalog(w http.ResponseWriter, r *http.Request) {
	p, err := parsePeriod(r, s.now())
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "bad_period", err.Error())
		return
	}
	page, err := parsePagination(r)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "bad_pagination", err.Error())
		return
	}
	format := strings.ToLower(r.URL.Query().Get("format"))
	if format == "" {
		format = "all"
	}
	if format != "tv" && format != "movie" && format != "ova" && format != "all" {
		writeAPIError(w, http.StatusBadRequest, "bad_format", "format must be tv, movie, ova or all")
		return
	}
	ctx := r.Context()
	lo, hi := viewsPeriodBounds(p.From, p.To)

	// Per-anime, genre, format and language figures come from the per-day rollup when every day of
	// the period is there, otherwise from a period-bounded scan of watch_sessions.
	acc, haveRollup, err := s.loadCatalogRollup(ctx, p.From, p.To)
	if err != nil {
		viewsServerError(w)
		return
	}
	if !haveRollup {
		if acc, err = s.scanCatalog(ctx, lo, hi); err != nil {
			viewsServerError(w)
			return
		}
	}
	cv := acc.view(format)
	animes, formats, audio, sub, genres, total := cv.animes, cv.formats, cv.audio, cv.sub, cv.genres, cv.total

	// Rollup-backed per-anime numbers: new viewers and previous-period sessions.
	newViewers := map[int64]int64{}
	prevSessions := map[int64]int64{}
	rr, err := s.adminDB().QueryContext(ctx, `SELECT anime_id,
			SUM(CASE WHEN day >= ? AND day <= ? THEN new_viewers ELSE 0 END),
			SUM(CASE WHEN day >= ? AND day <= ? THEN sessions ELSE 0 END)
		FROM metrics_anime_daily WHERE day >= ? AND day <= ? GROUP BY anime_id`, p.From, p.To, p.PrevFrom, p.PrevTo, p.PrevFrom, p.To)
	if err != nil {
		viewsServerError(w)
		return
	}
	for rr.Next() {
		var id, nv, ps int64
		if err := rr.Scan(&id, &nv, &ps); err != nil {
			rr.Close()
			viewsServerError(w)
			return
		}
		newViewers[id], prevSessions[id] = nv, ps
	}
	rr.Close()
	if err := rr.Err(); err != nil {
		viewsServerError(w)
		return
	}

	type animeRow struct {
		AnimeID       int64    `json:"anime_id"`
		Title         string   `json:"title"`
		Format        string   `json:"format"`
		Sessions      int64    `json:"sessions"`
		WatchHours    float64  `json:"watch_hours"`
		CompletionPct float64  `json:"completion_pct"`
		NewViewers    int64    `json:"new_viewers"`
		DeltaPct      *float64 `json:"delta_pct"`
	}
	list := make([]animeRow, 0, len(animes))
	for id, a := range animes {
		list = append(list, animeRow{AnimeID: id, Title: a.title, Format: a.format, Sessions: a.sessions,
			WatchHours: viewsRound(a.secs/3600, 2), CompletionPct: viewsRound(viewsPct(float64(a.completed), float64(a.sessions)), 2),
			NewViewers: newViewers[id], DeltaPct: delta(float64(a.sessions), float64(prevSessions[id]))})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Sessions != list[j].Sessions {
			return list[i].Sessions > list[j].Sessions
		}
		return list[i].AnimeID < list[j].AnimeID
	})

	// Quadrant: thresholds are the medians over every matching anime (strictly above = high);
	// points are the top MaxLimit.
	sess := make([]float64, len(list))
	comps := make([]float64, len(list))
	for i, a := range list {
		sess[i], comps[i] = float64(a.Sessions), a.CompletionPct
	}
	medS, medC := viewsMedian(sess), viewsMedian(comps)
	type qPoint struct {
		AnimeID       int64   `json:"anime_id"`
		Title         string  `json:"title"`
		Sessions      int64   `json:"sessions"`
		CompletionPct float64 `json:"completion_pct"`
		Quadrant      string  `json:"quadrant"`
		QuadrantKey   string  `json:"quadrant_key"`
	}
	points := make([]qPoint, 0, min(len(list), MaxLimit))
	for _, a := range list[:min(len(list), MaxLimit)] {
		hiPop, hiComp := float64(a.Sessions) > medS, a.CompletionPct > medC
		q := qPoint{AnimeID: a.AnimeID, Title: a.Title, Sessions: a.Sessions, CompletionPct: a.CompletionPct}
		switch {
		case hiPop && hiComp:
			q.Quadrant, q.QuadrantKey = "valeurs sûres", "safe"
		case !hiPop && hiComp:
			q.Quadrant, q.QuadrantKey = "à pousser", "push"
		case hiPop && !hiComp:
			q.Quadrant, q.QuadrantKey = "à surveiller", "watch"
		default:
			q.Quadrant, q.QuadrantKey = "à retirer", "retire"
		}
		points = append(points, q)
	}

	// New series (first-ever session in the period) vs back catalogue, from the rollup history.
	var historyFrom string
	known := map[int64]bool{}
	hr, err := s.adminDB().QueryContext(ctx, `SELECT DISTINCT anime_id FROM metrics_anime_daily WHERE day < ?`, p.From)
	if err != nil {
		viewsServerError(w)
		return
	}
	for hr.Next() {
		var id int64
		if err := hr.Scan(&id); err != nil {
			hr.Close()
			viewsServerError(w)
			return
		}
		known[id] = true
	}
	hr.Close()
	if err := s.adminDB().QueryRowContext(ctx, `SELECT COALESCE(MIN(day), '') FROM metrics_anime_daily`).Scan(&historyFrom); err != nil {
		viewsServerError(w)
		return
	}
	var newSeries, newSess, catSeries, catSess int64
	for _, a := range list {
		if known[a.AnimeID] {
			catSeries++
			catSess += a.Sessions
		} else {
			newSeries++
			newSess += a.Sessions
		}
	}
	anySess := newSess + catSess

	type genreRow struct {
		Genre         string  `json:"genre"`
		Sessions      int64   `json:"sessions"`
		SharePct      float64 `json:"share_pct"`
		CompletionPct float64 `json:"completion_pct"`
	}
	grows := make([]genreRow, 0, len(genres))
	for g, a := range genres {
		grows = append(grows, genreRow{g, a.sessions, viewsRound(viewsPct(float64(a.sessions), float64(total)), 2),
			viewsRound(viewsPct(float64(a.completed), float64(a.sessions)), 2)})
	}
	sort.Slice(grows, func(i, j int) bool {
		if grows[i].Sessions != grows[j].Sessions {
			return grows[i].Sessions > grows[j].Sessions
		}
		return grows[i].Genre < grows[j].Genre
	})
	if len(grows) > 20 {
		grows = grows[:20]
	}

	start := min(page.Offset, len(list))
	end := min(start+page.Limit, len(list))
	writeData(w, p, map[string]any{
		"format":      format,
		"anime":       list[start:end],
		"anime_total": len(list),
		"limit":       page.Limit,
		"offset":      page.Offset,
		"quadrant": map[string]any{
			"median_sessions": medS, "median_completion_pct": medC, "points": points,
		},
		"genres":      grows,
		"formats":     viewsShares(formats, sumMap(formats), 20),
		"audio_langs": viewsShares(audio, total, 20),
		"sub_langs":   viewsShares(sub, total, 20),
		"new_vs_catalog": map[string]any{
			"new_series":   map[string]any{"series": newSeries, "sessions": newSess, "share_pct": viewsRound(viewsPct(float64(newSess), float64(anySess)), 2)},
			"catalog":      map[string]any{"series": catSeries, "sessions": catSess, "share_pct": viewsRound(viewsPct(float64(catSess), float64(anySess)), 2)},
			"history_from": historyFrom,
		},
	})
}

func sumMap(m map[string]int64) (n int64) {
	for _, v := range m {
		n += v
	}
	return n
}
