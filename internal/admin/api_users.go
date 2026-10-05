package admin

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

// Users and growth read endpoints (admin design pages "Utilisateurs" and "Croissance").
//
// Privacy: these handlers never read email_enc, email_idx or pass_hash, and never return a
// token_hash. The pseudo is returned to admin sessions only (a human), never to a token.
//
// Activity: a user is "active" on a period when they have at least one watch_sessions row
// whose started_at falls in it (same definition as the metrics rollup). Every window is in
// UTC. Windows named "7 d", "14 d", "30 d" are rolling instants before now.
//
// TODO(rollup): the per-user aggregates below (segments, directory, funnel) scan
// watch_sessions through the (user_id, started_at) index for every user; a per-user rollup
// table would make them O(users) instead of O(sessions).

const (
	usersDay         = int64(86400)
	usersMaxQueryLen = 64
)

// User segments, in display order. A user belongs to exactly one, first match wins:
//
//	new            account created less than 7 days ago
//	never_watched  at least 7 days old, no session at all
//	dormant        last session more than 30 days ago
//	at_risk        last session 14 to 30 days ago and at least 4 sessions in the 30 days
//	               preceding that silence (they were regular)
//	power          averaging more than 10 h of watching per week over the last 30 days
//	               (or since creation for accounts younger than 30 days)
//	regular        everyone else with sessions (includes casual viewers)
var usersSegments = []struct{ key, label string }{
	{"new", "Nouveaux (< 7 j)"},
	{"regular", "Réguliers"},
	{"power", "Power users (> 10 h/semaine)"},
	{"dormant", "Dormants (> 30 j)"},
	{"at_risk", "À risque (aucune séance depuis 14 j+)"},
	{"never_watched", "Jamais regardé"},
}

func usersSegmentValid(k string) bool {
	for _, s := range usersSegments {
		if s.key == k {
			return true
		}
	}
	return false
}

// usersStatsCTE builds the per-user aggregate CTE "sg" (one row per user with its segment).
// where is a fixed SQL fragment chosen by the caller (never client text); every client value
// goes through named parameters.
func usersStatsCTE(where string) string {
	return `WITH st AS (
	SELECT u.id AS id, u.pseudo AS pseudo, u.created_at AS created_at,
		COUNT(w.session_id) AS n,
		MIN(w.started_at) AS first_at,
		MAX(w.started_at) AS last_at,
		COALESCE(SUM(MAX(w.watched_seconds, 0)), 0) AS secs,
		COALESCE(SUM(CASE WHEN w.started_at >= :d30 THEN MAX(w.watched_seconds, 0) END), 0) AS secs30,
		COALESCE(SUM(CASE WHEN w.started_at >= :d7 THEN 1 END), 0) AS n7,
		COALESCE(SUM(CASE WHEN w.started_at >= :d14 AND w.started_at < :d7 THEN 1 END), 0) AS n7p,
		COALESCE(SUM(CASE WHEN w.started_at >= :d30 THEN 1 END), 0) AS n30,
		COALESCE(SUM(CASE WHEN w.started_at >= :d60 AND w.started_at < :d30 THEN 1 END), 0) AS n30p,
		COALESCE(SUM(CASE WHEN w.started_at >= :d44 AND w.started_at < :d14 THEN 1 END), 0) AS nprior,
		MAX(CASE WHEN w.started_at < :d30 THEN w.started_at END) AS lb30
	FROM users u LEFT JOIN watch_sessions w ON w.user_id = u.id
	` + where + `
	GROUP BY u.id
), sg AS (
	SELECT st.*, CASE
		WHEN created_at >= :d7 THEN 'new'
		WHEN n = 0 THEN 'never_watched'
		WHEN last_at < :d30 THEN 'dormant'
		WHEN last_at < :d14 AND nprior >= 4 THEN 'at_risk'
		WHEN secs30 > 36000.0 * (MIN(:now - created_at, 2592000) / 604800.0) THEN 'power'
		ELSE 'regular' END AS segment
	FROM st
)`
}

func usersWindowArgs(now time.Time) []any {
	n := now.UTC().Unix()
	return []any{
		sql.Named("now", n),
		sql.Named("d7", n-7*usersDay),
		sql.Named("d14", n-14*usersDay),
		sql.Named("d30", n-30*usersDay),
		sql.Named("d44", n-44*usersDay),
		sql.Named("d60", n-60*usersDay),
	}
}

func (s *Service) mountUsers(r chi.Router) {
	r.With(s.AuthCached(ScopeMetricsRead, ResponseCacheTTL)).Get("/users/summary", s.handleUsersSummary)
	r.With(s.AuthCached(ScopeMetricsRead, ResponseCacheTTL)).Get("/users", s.handleUsersList)
	r.With(s.Auth(ScopeMetricsRead)).Get("/users/{id}", s.handleUserDetail)
	r.With(s.AuthCached(ScopeMetricsRead, ResponseCacheTTL)).Get("/growth", s.handleGrowth)
}

func usersViaToken(r *http.Request) bool {
	info, _ := r.Context().Value(authKey{}).(authInfo)
	return info.via == "token"
}

type usersKPI struct {
	Value    float64  `json:"value"`
	Previous float64  `json:"previous"`
	DeltaPct *float64 `json:"delta_pct"`
}

func usersKPIOf(cur, prev float64) usersKPI {
	return usersKPI{Value: cur, Previous: prev, DeltaPct: delta(cur, prev)}
}

func usersPct(n, d int64) float64 {
	if d <= 0 {
		return 0
	}
	return float64(n) / float64(d) * 100
}

func usersPctPtr(n, d int64) *float64 {
	if d <= 0 {
		return nil
	}
	v := usersPct(n, d)
	return &v
}

func usersRFC3339(ts int64) string { return time.Unix(ts, 0).UTC().Format(time.RFC3339) }

// handleUsersSummary feeds the "Utilisateurs" page: KPI row, segments, seniority and
// active sessions. GET /users/summary (metrics:read). "previous" of each KPI is the same
// measure one window earlier (now - 7 d / 30 d).
func (s *Service) handleUsersSummary(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	now := s.now().UTC()
	nowU := now.Unix()
	args := usersWindowArgs(now)
	db := s.accountsDB()

	var total, a7, a7p, a30, a30p, noSess, dormant, totalPrev, noSessPrev, dormantPrev int64
	err := db.QueryRowContext(ctx, usersStatsCTE("")+`
SELECT COUNT(*),
	COALESCE(SUM(n7 > 0), 0), COALESCE(SUM(n7p > 0), 0),
	COALESCE(SUM(n30 > 0), 0), COALESCE(SUM(n30p > 0), 0),
	COALESCE(SUM(n = 0), 0),
	COALESCE(SUM(n > 0 AND last_at < :d30), 0),
	COALESCE(SUM(created_at < :d30), 0),
	COALESCE(SUM(created_at < :d30 AND (first_at IS NULL OR first_at >= :d30)), 0),
	COALESCE(SUM(lb30 IS NOT NULL AND lb30 < :d60), 0)
FROM st`, args...).Scan(&total, &a7, &a7p, &a30, &a30p, &noSess, &dormant, &totalPrev, &noSessPrev, &dormantPrev)
	if err != nil {
		usersInternal(w, err)
		return
	}

	type segAgg struct {
		n    int64
		secs float64
	}
	segs := map[string]segAgg{}
	rows, err := db.QueryContext(ctx, usersStatsCTE("")+`
SELECT segment, COUNT(*), COALESCE(SUM(secs), 0) FROM sg GROUP BY segment`, args...)
	if err != nil {
		usersInternal(w, err)
		return
	}
	for rows.Next() {
		var k string
		var a segAgg
		if err := rows.Scan(&k, &a.n, &a.secs); err != nil {
			rows.Close()
			usersInternal(w, err)
			return
		}
		segs[k] = a
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		usersInternal(w, err)
		return
	}
	rows.Close()
	segOut := make([]map[string]any, 0, len(usersSegments))
	for _, sd := range usersSegments {
		a := segs[sd.key]
		avg := 0.0
		if a.n > 0 {
			avg = a.secs / 3600 / float64(a.n)
		}
		segOut = append(segOut, map[string]any{"key": sd.key, "label": sd.label, "users": a.n,
			"share_pct": usersPct(a.n, total), "avg_watch_hours": avg})
	}

	// Seniority = account age buckets.
	type bucket struct {
		key, label string
		lo, hi     int64 // age in days, lo <= age < hi (hi 0 = open)
	}
	buckets := []bucket{
		{"lt_7d", "< 7 j", 0, 7}, {"7_30d", "7-30 j", 7, 30}, {"30_90d", "30-90 j", 30, 90},
		{"90_180d", "90-180 j", 90, 180}, {"180_365d", "180-365 j", 180, 365}, {"gt_365d", "> 1 an", 365, 0},
	}
	senior := make([]map[string]any, 0, len(buckets))
	for _, b := range buckets {
		// Age in whole days; an account created in the future counts as age 0.
		q := `SELECT COUNT(*) FROM users WHERE MAX(:now - created_at, 0) >= :lo`
		a := []any{sql.Named("now", nowU), sql.Named("lo", b.lo*usersDay)}
		if b.hi > 0 {
			q += ` AND MAX(:now - created_at, 0) < :hi`
			a = append(a, sql.Named("hi", b.hi*usersDay))
		}
		var n int64
		if err := db.QueryRowContext(ctx, q, a...).Scan(&n); err != nil {
			usersInternal(w, err)
			return
		}
		senior = append(senior, map[string]any{"key": b.key, "label": b.label, "users": n, "share_pct": usersPct(n, total)})
	}

	// Active sessions: only counts, no token_hash and no user_id leave the query.
	var valid, expiring int64
	if err := db.QueryRowContext(ctx, `SELECT COALESCE(SUM(expires_at > :now), 0),
		COALESCE(SUM(expires_at > :now AND expires_at <= :soon), 0) FROM sessions`,
		sql.Named("now", nowU), sql.Named("soon", nowU+7*usersDay)).Scan(&valid, &expiring); err != nil {
		usersInternal(w, err)
		return
	}

	writeDataNoPeriod(w, map[string]any{
		"kpis": map[string]usersKPI{
			"total":      usersKPIOf(float64(total), float64(totalPrev)),
			"active_7d":  usersKPIOf(float64(a7), float64(a7p)),
			"active_30d": usersKPIOf(float64(a30), float64(a30p)),
			"dormant":    usersKPIOf(float64(dormant), float64(dormantPrev)),
			"no_session": usersKPIOf(float64(noSess), float64(noSessPrev)),
		},
		"segments":        segOut,
		"seniority":       senior,
		"active_sessions": map[string]int64{"valid": valid, "expiring_7d": expiring},
	})
}

func usersInternal(w http.ResponseWriter, err error) {
	if errors.Is(err, context.Canceled) {
		return
	}
	writeAPIError(w, http.StatusInternalServerError, "internal", "query failed")
}

// usersSortColumns is the whitelist of ORDER BY expressions; the client only picks a key.
var usersSortColumns = map[string]string{
	"user_id":       "id",
	"created_at":    "created_at",
	"last_activity": "last_at",
	"sessions":      "n",
	"watch_hours":   "secs",
	"pseudo":        "pseudo COLLATE NOCASE",
}

func usersEscapeLike(q string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(q)
}

type usersTopAnime struct {
	AnimeID    int64   `json:"anime_id"`
	Title      string  `json:"title"`
	WatchHours float64 `json:"watch_hours"`
}

type usersRow struct {
	UserID       int64          `json:"user_id"`
	Pseudo       *string        `json:"pseudo,omitempty"` // admin sessions only
	CreatedAt    string         `json:"created_at"`
	LastActivity *string        `json:"last_activity"`
	Sessions     int64          `json:"sessions"`
	WatchHours   float64        `json:"watch_hours"`
	TopAnime     *usersTopAnime `json:"top_anime"`
	Segment      string         `json:"segment"`
}

// handleUsersList feeds the "Utilisateurs" page directory table.
// GET /users?q=&sort=&dir=&segment=&limit=&offset= (metrics:read). q searches the pseudo and
// is refused (403) for a token, as is sorting by pseudo, so a token cannot probe pseudos.
func (s *Service) handleUsersList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	page, err := parsePagination(r)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "bad_pagination", err.Error())
		return
	}
	q := r.URL.Query()
	viaToken := usersViaToken(r)

	search := strings.TrimSpace(q.Get("q"))
	if search != "" && viaToken {
		writeAPIError(w, http.StatusForbidden, "forbidden", "q is only available to admin sessions")
		return
	}
	if len([]rune(search)) > usersMaxQueryLen {
		writeAPIError(w, http.StatusBadRequest, "bad_q", "q is too long")
		return
	}
	sortKey := q.Get("sort")
	if sortKey == "" {
		sortKey = "created_at"
	}
	col, ok := usersSortColumns[sortKey]
	if !ok {
		writeAPIError(w, http.StatusBadRequest, "bad_sort", "sort must be one of user_id, created_at, last_activity, sessions, watch_hours, pseudo")
		return
	}
	if sortKey == "pseudo" && viaToken {
		writeAPIError(w, http.StatusForbidden, "forbidden", "sorting by pseudo is only available to admin sessions")
		return
	}
	dir := "DESC"
	switch strings.ToLower(q.Get("dir")) {
	case "":
	case "desc":
	case "asc":
		dir = "ASC"
	default:
		writeAPIError(w, http.StatusBadRequest, "bad_dir", "dir must be asc or desc")
		return
	}
	segment := q.Get("segment")
	if segment != "" && !usersSegmentValid(segment) {
		writeAPIError(w, http.StatusBadRequest, "bad_segment", "unknown segment")
		return
	}

	args := usersWindowArgs(s.now())
	where := ""
	if search != "" {
		where = `WHERE u.pseudo LIKE :like ESCAPE '\'`
		args = append(args, sql.Named("like", "%"+usersEscapeLike(search)+"%"))
	}
	args = append(args, sql.Named("seg", segment))
	filter := ` WHERE (:seg = '' OR segment = :seg)`
	db := s.accountsDB()

	var total int64
	if err := db.QueryRowContext(ctx, usersStatsCTE(where)+` SELECT COUNT(*) FROM sg`+filter, args...).Scan(&total); err != nil {
		usersInternal(w, err)
		return
	}
	// col and dir come from the whitelists above, never from the client.
	listArgs := append(append([]any{}, args...), sql.Named("limit", page.Limit), sql.Named("offset", page.Offset))
	rows, err := db.QueryContext(ctx, usersStatsCTE(where)+
		` SELECT id, pseudo, created_at, last_at, n, secs, segment FROM sg`+filter+
		` ORDER BY `+col+` `+dir+`, id ASC LIMIT :limit OFFSET :offset`, listArgs...)
	if err != nil {
		usersInternal(w, err)
		return
	}
	users := []usersRow{}
	idx := map[int64]int{}
	var ids []any
	for rows.Next() {
		var u usersRow
		var pseudo string
		var created int64
		var last sql.NullInt64
		var secs float64
		if err := rows.Scan(&u.UserID, &pseudo, &created, &last, &u.Sessions, &secs, &u.Segment); err != nil {
			rows.Close()
			usersInternal(w, err)
			return
		}
		if !viaToken {
			u.Pseudo = &pseudo
		}
		u.CreatedAt = usersRFC3339(created)
		if last.Valid {
			v := usersRFC3339(last.Int64)
			u.LastActivity = &v
		}
		u.WatchHours = secs / 3600
		idx[u.UserID] = len(users)
		ids = append(ids, u.UserID)
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		usersInternal(w, err)
		return
	}
	rows.Close()

	if len(ids) > 0 {
		// Most watched anime of the users on this page (at most MaxLimit users): one query.
		marks := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
		tr, err := db.QueryContext(ctx, `SELECT user_id, anime_id, MAX(title), SUM(MAX(watched_seconds, 0))
			FROM watch_sessions WHERE user_id IN (`+marks+`) AND anime_id > 0 GROUP BY user_id, anime_id`, ids...)
		if err != nil {
			usersInternal(w, err)
			return
		}
		for tr.Next() {
			var uid, aid int64
			var title string
			var secs float64
			if err := tr.Scan(&uid, &aid, &title, &secs); err != nil {
				tr.Close()
				usersInternal(w, err)
				return
			}
			u := &users[idx[uid]]
			if u.TopAnime == nil || secs > u.TopAnime.WatchHours*3600 || (secs == u.TopAnime.WatchHours*3600 && aid < u.TopAnime.AnimeID) {
				u.TopAnime = &usersTopAnime{AnimeID: aid, Title: title, WatchHours: secs / 3600}
			}
		}
		if err := tr.Err(); err != nil {
			tr.Close()
			usersInternal(w, err)
			return
		}
		tr.Close()
	}

	writeDataNoPeriod(w, map[string]any{"total": total, "limit": page.Limit, "offset": page.Offset, "users": users})
}

// handleUserDetail feeds the "Utilisateurs" page detail panel.
// GET /users/{id} (metrics:read). Unknown or malformed id: 404. No email, ever.
func (s *Service) handleUserDetail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id < 1 {
		writeAPIError(w, http.StatusNotFound, "not_found", "user not found")
		return
	}
	db := s.accountsDB()
	args := append(usersWindowArgs(s.now()), sql.Named("uid", id))
	var pseudo, segment string
	var created int64
	err = db.QueryRowContext(ctx, usersStatsCTE(`WHERE u.id = :uid`)+` SELECT pseudo, created_at, segment FROM sg`, args...).Scan(&pseudo, &created, &segment)
	if errors.Is(err, sql.ErrNoRows) {
		writeAPIError(w, http.StatusNotFound, "not_found", "user not found")
		return
	}
	if err != nil {
		usersInternal(w, err)
		return
	}

	type sess struct {
		AnimeID        int64   `json:"anime_id"`
		Title          string  `json:"title"`
		Episode        int64   `json:"episode"`
		StartedAt      string  `json:"started_at"`
		WatchedSeconds float64 `json:"watched_seconds"`
		Duration       float64 `json:"duration"`
		Completed      bool    `json:"completed"`
	}
	recent := []sess{}
	rows, err := db.QueryContext(ctx, `SELECT anime_id, title, episode, started_at, watched_seconds, duration, completed
		FROM watch_sessions WHERE user_id = ? ORDER BY started_at DESC, session_id DESC LIMIT 10`, id)
	if err != nil {
		usersInternal(w, err)
		return
	}
	for rows.Next() {
		var x sess
		var started, comp int64
		if err := rows.Scan(&x.AnimeID, &x.Title, &x.Episode, &started, &x.WatchedSeconds, &x.Duration, &comp); err != nil {
			rows.Close()
			usersInternal(w, err)
			return
		}
		x.StartedAt, x.Completed = usersRFC3339(started), comp != 0
		recent = append(recent, x)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		usersInternal(w, err)
		return
	}
	rows.Close()

	type prog struct {
		AnimeID   int64   `json:"anime_id"`
		SeasonID  int64   `json:"season_id"`
		Title     string  `json:"title"`
		Episode   int64   `json:"episode"`
		Position  float64 `json:"position"`
		UpdatedAt string  `json:"updated_at"`
	}
	progress := []prog{}
	pr, err := db.QueryContext(ctx, `SELECT anime_id, season_id, title, episode, position, updated_at
		FROM progress WHERE user_id = ? AND completed = 0 ORDER BY updated_at DESC, season_id LIMIT 50`, id)
	if err != nil {
		usersInternal(w, err)
		return
	}
	for pr.Next() {
		var x prog
		var upd int64
		if err := pr.Scan(&x.AnimeID, &x.SeasonID, &x.Title, &x.Episode, &x.Position, &upd); err != nil {
			pr.Close()
			usersInternal(w, err)
			return
		}
		x.UpdatedAt = usersRFC3339(upd)
		progress = append(progress, x)
	}
	if err := pr.Err(); err != nil {
		pr.Close()
		usersInternal(w, err)
		return
	}
	pr.Close()

	out := map[string]any{"user_id": id, "created_at": usersRFC3339(created), "segment": segment,
		"recent_sessions": recent, "progress": progress}
	if !usersViaToken(r) {
		out["pseudo"] = pseudo
	}
	writeDataNoPeriod(w, out)
}

// ---- growth ----------------------------------------------------------------------------

type growthCohortUser struct {
	created, n, eps, first int64
	d1, d7, d14, d30       bool
}

// handleGrowth feeds the "Croissance" page: DAU/WAU/MAU, stickiness, signups, funnel,
// retention cohorts, time to first session and churn. GET /growth?period=7|30|90 (metrics:read).
//
// Retention day definition: a user is "active at Jn" when they have a session on the UTC
// calendar day (signup day + n). A step/cell is only measurable once that day is over for
// every user concerned (signup day + n < today); otherwise it is excluded (funnel) or null
// (cohorts) rather than counted as a loss.
func (s *Service) handleGrowth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	now := s.now().UTC()
	p, err := parsePeriod(r, now)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "bad_period", err.Error())
		return
	}
	db := s.accountsDB()
	fromT, _ := time.Parse(dayLayout, p.From)
	toT, _ := time.Parse(dayLayout, p.To)
	prevToT, _ := time.Parse(dayLayout, p.PrevTo)
	fromDay, toDay, prevToDay := fromT.Unix()/usersDay, toT.Unix()/usersDay, prevToT.Unix()/usersDay
	dayStr := func(d int64) string { return time.Unix(d*usersDay, 0).UTC().Format(dayLayout) }

	// --- DAU / WAU / MAU: one aggregated pass over distinct (day, user) pairs, then sliding
	// windows in memory (no correlated subquery per day).
	// TODO(rollup): metrics_daily only stores a distinct count per day, which cannot give
	// WAU/MAU; a daily (day, user_id) rollup would replace this scan.
	byDay := map[int64][]int64{}
	rows, err := db.QueryContext(ctx, `SELECT DISTINCT started_at / 86400, user_id FROM watch_sessions
		WHERE started_at >= ? AND started_at < ?`, (fromDay-29)*usersDay, (toDay+1)*usersDay)
	if err != nil {
		usersInternal(w, err)
		return
	}
	for rows.Next() {
		var d, uid int64
		if err := rows.Scan(&d, &uid); err != nil {
			rows.Close()
			usersInternal(w, err)
			return
		}
		byDay[d] = append(byDay[d], uid)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		usersInternal(w, err)
		return
	}
	rows.Close()
	type actPoint struct {
		Day string `json:"day"`
		DAU int    `json:"dau"`
		WAU int    `json:"wau"`
		MAU int    `json:"mau"`
	}
	series := make([]actPoint, 0, p.Days)
	c7, c30 := map[int64]int{}, map[int64]int{}
	dec := func(m map[int64]int, uid int64) {
		if m[uid]--; m[uid] <= 0 {
			delete(m, uid)
		}
	}
	for d := fromDay - 29; d <= toDay; d++ {
		for _, u := range byDay[d] {
			c7[u]++
			c30[u]++
		}
		for _, u := range byDay[d-7] {
			dec(c7, u)
		}
		for _, u := range byDay[d-30] {
			dec(c30, u)
		}
		if d >= fromDay {
			series = append(series, actPoint{Day: dayStr(d), DAU: len(byDay[d]), WAU: len(c7), MAU: len(c30)})
		}
	}
	cur := series[len(series)-1]

	countActive := func(lo, hi int64) (int, error) {
		var n int
		err := db.QueryRowContext(ctx, `SELECT COUNT(DISTINCT user_id) FROM watch_sessions WHERE started_at >= ? AND started_at < ?`,
			lo*usersDay, (hi+1)*usersDay).Scan(&n)
		return n, err
	}
	var pDAU, pWAU, pMAU int
	for _, x := range []struct {
		dst *int
		lo  int64
	}{{&pDAU, prevToDay}, {&pWAU, prevToDay - 6}, {&pMAU, prevToDay - 29}} {
		if *x.dst, err = countActive(x.lo, prevToDay); err != nil {
			usersInternal(w, err)
			return
		}
	}
	stick, pStick := 0.0, 0.0
	if cur.MAU > 0 {
		stick = float64(cur.DAU) / float64(cur.MAU) * 100
	}
	if pMAU > 0 {
		pStick = float64(pDAU) / float64(pMAU) * 100
	}

	// --- signups per week and cumulative users.
	// TODO(rollup): users.created_at is not indexed; metrics_daily.new_users/total_users
	// hold the same figures once the rollup is complete.
	weekStart := func(d int64) int64 { // Monday of the week containing day d (1970-01-01 was a Thursday)
		return d - ((d+3)%7+7)%7
	}
	firstWeek, lastWeek := weekStart(fromDay), weekStart(toDay)
	signups := map[int64]int64{}
	perDay := map[int64]int64{}
	ur, err := db.QueryContext(ctx, `SELECT created_at / 86400, COUNT(*) FROM users WHERE created_at >= ? AND created_at < ? GROUP BY 1`,
		firstWeek*usersDay, (toDay+1)*usersDay)
	if err != nil {
		usersInternal(w, err)
		return
	}
	for ur.Next() {
		var d, n int64
		if err := ur.Scan(&d, &n); err != nil {
			ur.Close()
			usersInternal(w, err)
			return
		}
		signups[weekStart(d)] += n
		perDay[d] = n
	}
	if err := ur.Err(); err != nil {
		ur.Close()
		usersInternal(w, err)
		return
	}
	ur.Close()
	var base int64
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE created_at < ?`, fromDay*usersDay).Scan(&base); err != nil {
		usersInternal(w, err)
		return
	}
	weeks := []map[string]any{}
	for wk := firstWeek; wk <= lastWeek; wk += 7 {
		weeks = append(weeks, map[string]any{"week_start": dayStr(wk), "signups": signups[wk]})
	}
	cumul := make([]map[string]any, 0, p.Days)
	run := base
	for d := fromDay; d <= toDay; d++ {
		run += perDay[d]
		cumul = append(cumul, map[string]any{"day": dayStr(d), "users": run})
	}

	// --- cohort of accounts created in the period: funnel, retention, time to first session.
	cr, err := db.QueryContext(ctx, `SELECT u.id, u.created_at, COUNT(w.session_id),
		COUNT(DISTINCT w.season_id || ':' || w.episode), COALESCE(MIN(w.started_at), 0),
		COALESCE(SUM(CASE WHEN w.started_at / 86400 = u.created_at / 86400 + 1 THEN 1 END), 0),
		COALESCE(SUM(CASE WHEN w.started_at / 86400 = u.created_at / 86400 + 7 THEN 1 END), 0),
		COALESCE(SUM(CASE WHEN w.started_at / 86400 = u.created_at / 86400 + 14 THEN 1 END), 0),
		COALESCE(SUM(CASE WHEN w.started_at / 86400 = u.created_at / 86400 + 30 THEN 1 END), 0)
		FROM users u LEFT JOIN watch_sessions w ON w.user_id = u.id
		WHERE u.created_at >= ? AND u.created_at < ? GROUP BY u.id`, fromDay*usersDay, (toDay+1)*usersDay)
	if err != nil {
		usersInternal(w, err)
		return
	}
	var cohort []growthCohortUser
	for cr.Next() {
		var c growthCohortUser
		var id, a1, a7, a14, a30 int64
		if err := cr.Scan(&id, &c.created, &c.n, &c.eps, &c.first, &a1, &a7, &a14, &a30); err != nil {
			cr.Close()
			usersInternal(w, err)
			return
		}
		c.d1, c.d7, c.d14, c.d30 = a1 > 0, a7 > 0, a14 > 0, a30 > 0
		cohort = append(cohort, c)
	}
	if err := cr.Err(); err != nil {
		cr.Close()
		usersInternal(w, err)
		return
	}
	cr.Close()

	// today is the current (incomplete) UTC day: day n after signup is measurable once
	// signupDay + n < today.
	today := toDay
	measurable := func(c growthCohortUser, n int64) bool { return c.created/usersDay+n < today }

	// Funnel, nested: each step keeps only users who passed the previous ones.
	var nAll, nSess, nEps, el7, base7, ok7, el30, base30, ok30 int64
	for _, c := range cohort {
		nAll++
		if measurable(c, 7) {
			el7++
		}
		if measurable(c, 30) {
			el30++
		}
		if c.n == 0 {
			continue
		}
		nSess++
		if c.eps < 3 {
			continue
		}
		nEps++
		if measurable(c, 7) {
			base7++
			if c.d7 {
				ok7++
			}
			if measurable(c, 30) && c.d7 { // J30 keeps users who also reached J7
				base30++
				if c.d30 {
					ok30++
				}
			}
		}
	}
	steps := []map[string]any{
		{"key": "signup", "label": "Inscription", "users": nAll, "eligible": nAll, "excluded_too_recent": 0, "conversion_pct": nil},
		{"key": "first_session", "label": "Première séance", "users": nSess, "eligible": nAll, "excluded_too_recent": 0, "conversion_pct": usersPctPtr(nSess, nAll)},
		{"key": "three_episodes", "label": "3 épisodes distincts vus", "users": nEps, "eligible": nSess, "excluded_too_recent": 0, "conversion_pct": usersPctPtr(nEps, nSess)},
		{"key": "active_d7", "label": "Actif à J7", "users": ok7, "eligible": base7, "excluded_too_recent": nAll - el7, "conversion_pct": usersPctPtr(ok7, base7)},
		{"key": "active_d30", "label": "Actif à J30", "users": ok30, "eligible": base30, "excluded_too_recent": nAll - el30, "conversion_pct": usersPctPtr(ok30, base30)},
	}
	funnel := map[string]any{"cohort_size": nAll, "steps": steps,
		"note": "nested funnel; Jn = session on signup day + n; accounts too recent for a step are excluded from its denominator (excluded_too_recent)"}

	// Weekly cohorts.
	type coh struct{ size, d1, d7, d14, d30 int64 }
	cw := map[int64]*coh{}
	for _, c := range cohort {
		k := weekStart(c.created / usersDay)
		x := cw[k]
		if x == nil {
			x = &coh{}
			cw[k] = x
		}
		x.size++
		if c.d1 {
			x.d1++
		}
		if c.d7 {
			x.d7++
		}
		if c.d14 {
			x.d14++
		}
		if c.d30 {
			x.d30++
		}
	}
	cohorts := []map[string]any{}
	for wk := firstWeek; wk <= lastWeek; wk += 7 {
		x := cw[wk]
		row := map[string]any{"week_start": dayStr(wk), "users": int64(0), "d1": nil, "d7": nil, "d14": nil, "d30": nil}
		if x != nil {
			row["users"] = x.size
			// Whole week must be over for day n: Sunday of the cohort week + n < today.
			for _, cell := range []struct {
				key string
				n   int64
				cnt int64
			}{{"d1", 1, x.d1}, {"d7", 7, x.d7}, {"d14", 14, x.d14}, {"d30", 30, x.d30}} {
				if wk+6+cell.n < today {
					row[cell.key] = usersPct(cell.cnt, x.size)
				}
			}
		}
		cohorts = append(cohorts, row)
	}

	// Time to first session.
	ttf := [5]int64{}
	for _, c := range cohort {
		switch dt := c.first - c.created; {
		case c.n == 0:
			ttf[4]++
		case dt < 3600:
			ttf[0]++
		case dt < 86400:
			ttf[1]++
		case dt < 7*86400:
			ttf[2]++
		default:
			ttf[3]++
		}
	}
	ttfKeys := []struct{ key, label string }{{"lt_1h", "< 1 h"}, {"1h_24h", "1-24 h"}, {"1d_7d", "1-7 j"}, {"gt_7d", "> 7 j"}, {"never", "Jamais (aucune séance à ce jour)"}}
	ttfOut := make([]map[string]any, 0, 5)
	for i, k := range ttfKeys {
		ttfOut = append(ttfOut, map[string]any{"key": k.key, "label": k.label, "users": ttf[i], "share_pct": usersPct(ttf[i], nAll)})
	}

	// Churn: active in the previous 30 days window, inactive in the current one (rolling
	// windows ending now; independent of ?period=).
	nowU := now.Unix()
	curLo, prevLo := nowU-30*usersDay, nowU-60*usersDay
	chr, err := db.QueryContext(ctx, `SELECT MAX(started_at < ?), MAX(started_at >= ?) FROM watch_sessions
		WHERE started_at >= ? AND started_at <= ? GROUP BY user_id`, curLo, curLo, prevLo, nowU)
	if err != nil {
		usersInternal(w, err)
		return
	}
	var prevActive, churned int64
	for chr.Next() {
		var inPrev, inCur int64
		if err := chr.Scan(&inPrev, &inCur); err != nil {
			chr.Close()
			usersInternal(w, err)
			return
		}
		if inPrev > 0 {
			prevActive++
			if inCur == 0 {
				churned++
			}
		}
	}
	if err := chr.Err(); err != nil {
		chr.Close()
		usersInternal(w, err)
		return
	}
	chr.Close()

	writeData(w, p, map[string]any{
		"activity": map[string]usersKPI{
			"dau":            usersKPIOf(float64(cur.DAU), float64(pDAU)),
			"wau":            usersKPIOf(float64(cur.WAU), float64(pWAU)),
			"mau":            usersKPIOf(float64(cur.MAU), float64(pMAU)),
			"stickiness_pct": usersKPIOf(stick, pStick),
		},
		"activity_series":       series,
		"signups_per_week":      weeks,
		"cumulative_users":      cumul,
		"funnel":                funnel,
		"cohorts":               cohorts,
		"time_to_first_session": map[string]any{"cohort_size": nAll, "buckets": ttfOut},
		"churn": map[string]any{"previous_window_active": prevActive, "churned": churned,
			"churn_pct": usersPctPtr(churned, prevActive), "window_days": 30},
		"not_measured":        []string{"visitor_to_signup", "acquisition_sources"},
		"visitor_to_signup":   map[string]any{"measured": false, "value": nil},
		"acquisition_sources": map[string]any{"measured": false, "value": nil},
	})
}
