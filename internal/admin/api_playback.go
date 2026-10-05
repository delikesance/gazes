package admin

import (
	"context"
	"net/http"
	"sort"
	"time"

	"github.com/gazes/gazes/internal/diagnostics"
	"github.com/go-chi/chi/v5"
)

// mountPlayback registers the admin endpoints of the playback group: player health, playback
// errors and sources (page "Lecteur et flux"), costs (page "Business") and issues (page
// "Claude et MCP"). Only the playback agent edits this file and api_playback_*.go.
func (s *Service) mountPlayback(r chi.Router) {
	diag := s.Auth(ScopeDiagnosticsRead)
	r.With(diag).Get("/playback/health", s.handlePlaybackHealth)
	r.With(diag).Get("/playback/errors", s.handlePlaybackErrors)
	r.With(diag).Get("/playback/errors/summary", s.handlePlaybackErrorsSummary)
	r.With(diag).Get("/playback/sources", s.handlePlaybackSources)
	r.With(s.Auth(ScopeMetricsRead)).Get("/costs", s.handleCosts)

	r.With(diag).Get("/issues", s.handleIssuesList)
	r.With(diag).Get("/issues/{id}", s.handleIssueGet)
	ops := s.Auth(ScopeOpsWrite)
	opsW := r.With(ops, s.refuseTokenWhenSuspended)
	opsW.Post("/issues", s.handleIssueCreate)
	opsW.Patch("/issues/{id}", s.handleIssueUpdate)
}

// pbMaxSince bounds ?since= on the error list: nothing older than the longest period.
const pbMaxSince = 90 * 24 * time.Hour

// pbRange is the [from, to) unix-second window of the days from..to (inclusive UTC days).
func pbRange(from, to string) (int64, int64, error) {
	f, err := time.Parse(dayLayout, from)
	if err != nil {
		return 0, 0, err
	}
	t, err := time.Parse(dayLayout, to)
	if err != nil {
		return 0, 0, err
	}
	return f.Unix(), t.AddDate(0, 0, 1).Unix(), nil
}

// pbKPI is the {"value","previous","delta_pct"} shape.
type pbKPI struct {
	Value    float64  `json:"value"`
	Previous float64  `json:"previous"`
	DeltaPct *float64 `json:"delta_pct"`
}

func pbNewKPI(cur, prev float64) pbKPI {
	return pbKPI{Value: cur, Previous: prev, DeltaPct: delta(cur, prev)}
}

// pbCauses is the static "probable cause + file to inspect" table of the error summary.
var pbCauses = map[diagnostics.ErrorCode]struct {
	Cause string
	Files []string
}{
	diagnostics.StreamTimeout:  {"Aucun pair ne répond ou le démarrage dépasse le délai : torrent peu seedé, pairs injoignables ou trackers morts.", []string{"internal/torrent", "internal/playback/manager.go"}},
	diagnostics.RemuxFailed:    {"ffmpeg quitte en erreur pendant le remux : conteneur ou codec inattendu, flux source tronqué ou binaire ffmpeg absent.", []string{"internal/stream"}},
	diagnostics.SourceDead:     {"Tous les fournisseurs de sources ont échoué : indexeur ou Sonarr/Radarr injoignable, clé API invalide ou limite de débit.", []string{"internal/indexer"}},
	diagnostics.SourceTimeout:  {"Un fournisseur de sources ne répond pas dans le délai imparti : indexeur lent ou saturé.", []string{"internal/indexer"}},
	diagnostics.KVUnavailable:  {"Redis est injoignable : réseau, instance arrêtée ou espace de noms mal configuré.", []string{"internal/kv"}},
	diagnostics.SubtitleFailed: {"L'extraction des sous-titres par ffmpeg a échoué : piste absente ou source illisible.", []string{"internal/api/subtitle_jobs.go", "internal/api/subtitle_handlers.go"}},
	diagnostics.Unknown:        {"Erreur de lecture non classée : consulter les journaux de diagnostic.", []string{"internal/diagnostics"}},
}

func pbCodeOrder(counts map[string]int64) []string {
	var out []string
	seen := map[string]bool{}
	for _, c := range diagnostics.ErrorCodes() {
		out = append(out, string(c))
		seen[string(c)] = true
	}
	var extra []string
	for c := range counts {
		if !seen[c] {
			extra = append(extra, c)
		}
	}
	sort.Strings(extra)
	return append(out, extra...)
}

// pbCountErrors is the number of recorded errors in [from, to).
func (s *Service) pbCountErrors(ctx context.Context, from, to int64) (int64, error) {
	var n int64
	err := s.adminDB().QueryRowContext(ctx, `SELECT COUNT(*) FROM playback_errors WHERE ts >= ? AND ts < ?`, from, to).Scan(&n)
	return n, err
}

// pbSumSessions is the number of watch sessions over the days from..to, from the daily rollup.
func (s *Service) pbSumSessions(ctx context.Context, from, to string) (sessions, watchSeconds int64, err error) {
	err = s.adminDB().QueryRowContext(ctx,
		`SELECT COALESCE(SUM(sessions),0), COALESCE(SUM(watch_seconds),0) FROM metrics_daily WHERE day >= ? AND day <= ?`, from, to).Scan(&sessions, &watchSeconds)
	return
}

func pbServerError(w http.ResponseWriter, err error) {
	_ = err // details stay in the logs: never echo SQL errors to the caller
	writeAPIError(w, http.StatusInternalServerError, "internal", "internal error")
}

// handlePlaybackHealth: GET /playback/health (diagnostics:read) — feeds the "Lecteur et flux" page.
// Active sessions, error rate, start-up latency (not measured), sources and cache.
func (s *Service) handlePlaybackHealth(w http.ResponseWriter, r *http.Request) {
	p, err := parsePeriod(r, s.now())
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "bad_period", err.Error())
		return
	}
	ctx := r.Context()
	from, to, _ := pbRange(p.From, p.To)
	pfrom, pto, _ := pbRange(p.PrevFrom, p.PrevTo)
	errs, e1 := s.pbCountErrors(ctx, from, to)
	perrs, e2 := s.pbCountErrors(ctx, pfrom, pto)
	sess, _, e3 := s.pbSumSessions(ctx, p.From, p.To)
	psess, _, e4 := s.pbSumSessions(ctx, p.PrevFrom, p.PrevTo)
	var failing int64
	e5 := s.adminDB().QueryRowContext(ctx,
		`SELECT COUNT(DISTINCT source) FROM playback_errors WHERE ts >= ? AND ts < ? AND source IS NOT NULL AND source <> ''`, from, to).Scan(&failing)
	for _, e := range []error{e1, e2, e3, e4, e5} {
		if e != nil {
			pbServerError(w, e)
			return
		}
	}

	deps := s.pbDeps()
	active := map[string]any{"value": nil, "measured": false}
	if deps.stats != nil {
		active = map[string]any{"value": deps.stats.ActiveSessions(), "measured": true}
	}
	rate := func(e, n int64) *float64 {
		if n == 0 {
			return nil
		}
		v := float64(e) / float64(n) * 100
		return &v
	}
	cur, prev := rate(errs, sess), rate(perrs, psess)
	var rateDelta *float64
	if cur != nil && prev != nil {
		rateDelta = delta(*cur, *prev)
	}
	var cache any
	cacheMeasured := false
	if deps.cache != nil {
		cache, cacheMeasured = deps.cache(ctx), true
	}
	writeData(w, p, map[string]any{
		"active_sessions": active,
		"error_rate": map[string]any{
			"value": cur, "previous": prev, "delta_pct": rateDelta,
			"errors": errs, "sessions": sess, "measured": true,
			"note": "erreurs enregistrées / séances ; seuls les sites instrumentés sont comptés",
		},
		"startup_ms": map[string]any{"p50": nil, "p95": nil, "measured": false},
		"sources":    map[string]any{"active": nil, "total": nil, "failing": failing, "measured": false},
		"cache":      map[string]any{"data": cache, "measured": cacheMeasured},
	})
}

// handlePlaybackErrors: GET /playback/errors?code=&since=&limit=&offset= (diagnostics:read) —
// feeds the "Lecteur et flux" page (error table). Grouped by (code, anime, episode, source).
func (s *Service) handlePlaybackErrors(w http.ResponseWriter, r *http.Request) {
	pg, err := parsePagination(r)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "bad_pagination", err.Error())
		return
	}
	q := r.URL.Query()
	code := q.Get("code")
	if code != "" && !diagnostics.ErrorCode(code).Valid() {
		writeAPIError(w, http.StatusBadRequest, "bad_code", "unknown error code")
		return
	}
	now := s.now().UTC()
	since := now.Add(-pbMaxSince)
	if v := q.Get("since"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			t, err = time.Parse(dayLayout, v)
		}
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "bad_since", "since must be RFC3339 or YYYY-MM-DD")
			return
		}
		if t.After(since) {
			since = t
		}
	}
	ctx := r.Context()
	where := `ts >= ? AND (? = '' OR code = ?)`
	var total int64
	if err := s.adminDB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM (SELECT 1 FROM playback_errors WHERE `+where+` GROUP BY code, anime_id, episode, source)`,
		since.Unix(), code, code).Scan(&total); err != nil {
		pbServerError(w, err)
		return
	}
	rows, err := s.adminDB().QueryContext(ctx,
		`SELECT code, COALESCE(anime_id,0), COALESCE(episode,0), COALESCE(source,''), COUNT(*), MIN(ts), MAX(ts)
		   FROM playback_errors WHERE `+where+`
		  GROUP BY code, anime_id, episode, source
		  ORDER BY MAX(ts) DESC, code, anime_id, episode, source
		  LIMIT ? OFFSET ?`, since.Unix(), code, code, pg.Limit, pg.Offset)
	if err != nil {
		pbServerError(w, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var c, src string
		var anime, ep, n, first, last int64
		if err := rows.Scan(&c, &anime, &ep, &src, &n, &first, &last); err != nil {
			pbServerError(w, err)
			return
		}
		items = append(items, map[string]any{
			"code": c, "anime_id": pbNullable(anime), "episode": pbNullable(ep), "source": pbNullableStr(src),
			"occurrences": n,
			"first_seen":  time.Unix(first, 0).UTC().Format(time.RFC3339),
			"last_seen":   time.Unix(last, 0).UTC().Format(time.RFC3339),
		})
	}
	if err := rows.Err(); err != nil {
		pbServerError(w, err)
		return
	}
	writeDataNoPeriod(w, map[string]any{
		"since": since.Format(time.RFC3339), "items": items,
		"page": map[string]any{"limit": pg.Limit, "offset": pg.Offset, "total": total},
	})
}

func pbNullable(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}

func pbNullableStr(v string) any {
	if v == "" {
		return nil
	}
	return v
}

// handlePlaybackErrorsSummary: GET /playback/errors/summary?period= (diagnostics:read) — feeds
// the "Lecteur et flux" page (error cards, daily trend and probable causes).
func (s *Service) handlePlaybackErrorsSummary(w http.ResponseWriter, r *http.Request) {
	p, err := parsePeriod(r, s.now())
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "bad_period", err.Error())
		return
	}
	ctx := r.Context()
	from, to, _ := pbRange(p.From, p.To)
	pfrom, pto, _ := pbRange(p.PrevFrom, p.PrevTo)

	byCode := func(a, b int64) (map[string]int64, error) {
		out := map[string]int64{}
		rows, err := s.adminDB().QueryContext(ctx, `SELECT code, COUNT(*) FROM playback_errors WHERE ts >= ? AND ts < ? GROUP BY code`, a, b)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var c string
			var n int64
			if err := rows.Scan(&c, &n); err != nil {
				return nil, err
			}
			out[c] = n
		}
		return out, rows.Err()
	}
	cur, err1 := byCode(from, to)
	prev, err2 := byCode(pfrom, pto)
	if err1 != nil || err2 != nil {
		pbServerError(w, firstErr(err1, err2))
		return
	}
	merged := map[string]int64{}
	for k, v := range cur {
		merged[k] = v
	}
	for k := range prev {
		if _, ok := merged[k]; !ok {
			merged[k] = 0
		}
	}
	order := pbCodeOrder(merged)

	var total, ptotal int64
	codes := []map[string]any{}
	causes := []map[string]any{}
	for _, c := range order {
		total += cur[c]
		ptotal += prev[c]
		codes = append(codes, map[string]any{"code": c, "count": pbNewKPI(float64(cur[c]), float64(prev[c]))})
		if cur[c] > 0 {
			if pc, ok := pbCauses[diagnostics.ErrorCode(c)]; ok {
				causes = append(causes, map[string]any{"code": c, "count": cur[c], "probable_cause": pc.Cause, "files": pc.Files})
			} else {
				pc := pbCauses[diagnostics.Unknown]
				causes = append(causes, map[string]any{"code": c, "count": cur[c], "probable_cause": pc.Cause, "files": pc.Files})
			}
		}
	}

	// Daily trend, complete: every day of the period, zeros included.
	daily := map[string]map[string]int64{}
	rows, err := s.adminDB().QueryContext(ctx,
		`SELECT date(ts,'unixepoch'), code, COUNT(*) FROM playback_errors WHERE ts >= ? AND ts < ? GROUP BY 1, 2`, from, to)
	if err != nil {
		pbServerError(w, err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var d, c string
		var n int64
		if err := rows.Scan(&d, &c, &n); err != nil {
			pbServerError(w, err)
			return
		}
		if daily[d] == nil {
			daily[d] = map[string]int64{}
		}
		daily[d][c] = n
	}
	if err := rows.Err(); err != nil {
		pbServerError(w, err)
		return
	}
	series := []map[string]any{}
	day, _ := time.Parse(dayLayout, p.From)
	for i := 0; i < p.Days; i++ {
		d := day.AddDate(0, 0, i).Format(dayLayout)
		by := map[string]int64{}
		var t int64
		for _, c := range order {
			by[c] = daily[d][c]
			t += daily[d][c]
		}
		series = append(series, map[string]any{"day": d, "total": t, "by_code": by})
	}

	writeData(w, p, map[string]any{
		"total":           pbNewKPI(float64(total), float64(ptotal)),
		"by_code":         codes,
		"daily":           series,
		"probable_causes": causes,
	})
}

func firstErr(errs ...error) error {
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	return nil
}

// handlePlaybackSources: GET /playback/sources?period= (diagnostics:read) — feeds the "Lecteur et
// flux" page (sources table). Failures per source/tracker from playback_errors.source. The
// number of attempts per source is not measured, so there is no per-source failure rate: only
// the failure count and its share of all failures.
func (s *Service) handlePlaybackSources(w http.ResponseWriter, r *http.Request) {
	p, err := parsePeriod(r, s.now())
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "bad_period", err.Error())
		return
	}
	ctx := r.Context()
	from, to, _ := pbRange(p.From, p.To)
	pfrom, pto, _ := pbRange(p.PrevFrom, p.PrevTo)
	count := func(a, b int64) (map[string]int64, map[string]int64, error) {
		n, last := map[string]int64{}, map[string]int64{}
		rows, err := s.adminDB().QueryContext(ctx,
			`SELECT COALESCE(NULLIF(source,''),'unknown'), COUNT(*), MAX(ts) FROM playback_errors WHERE ts >= ? AND ts < ? GROUP BY 1`, a, b)
		if err != nil {
			return nil, nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var src string
			var c, l int64
			if err := rows.Scan(&src, &c, &l); err != nil {
				return nil, nil, err
			}
			n[src], last[src] = c, l
		}
		return n, last, rows.Err()
	}
	cur, last, err1 := count(from, to)
	prev, _, err2 := count(pfrom, pto)
	if err1 != nil || err2 != nil {
		pbServerError(w, firstErr(err1, err2))
		return
	}
	var total int64
	names := make([]string, 0, len(cur))
	for k, v := range cur {
		total += v
		names = append(names, k)
	}
	sort.Slice(names, func(i, j int) bool {
		if cur[names[i]] != cur[names[j]] {
			return cur[names[i]] > cur[names[j]]
		}
		return names[i] < names[j]
	})
	if len(names) > MaxLimit {
		names = names[:MaxLimit]
	}
	items := []map[string]any{}
	for _, n := range names {
		items = append(items, map[string]any{
			"source": n, "failures": pbNewKPI(float64(cur[n]), float64(prev[n])),
			"share_pct":    float64(cur[n]) / float64(total) * 100,
			"last_seen":    time.Unix(last[n], 0).UTC().Format(time.RFC3339),
			"failure_rate": nil, "measured": false,
		})
	}
	writeData(w, p, map[string]any{
		"sources":        items,
		"total_failures": total,
		"not_measured":   []string{"attempts_per_source"},
	})
}
