package admin

import (
	"net/http"
	"sort"
)

// pbMaxPeakRows bounds the session scan behind the concurrent-peak estimate.
const pbMaxPeakRows = 200000

// pbComponent is one cost line: value null and measured false when its inputs are missing.
func pbComponent(v *float64, missing ...string) map[string]any {
	if v == nil {
		return map[string]any{"value": nil, "measured": false, "missing_inputs": missing}
	}
	return map[string]any{"value": *v, "measured": true}
}

// handleCosts: GET /costs?period= (metrics:read) — feeds the "Business" page (usage and costs).
// Usage is real (rollups + watch_sessions); money comes only from the optional cost inputs and is
// null with measured:false when they are absent.
func (s *Service) handleCosts(w http.ResponseWriter, r *http.Request) {
	p, err := parsePeriod(r, s.now())
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "bad_period", err.Error())
		return
	}
	ctx := r.Context()
	sess, secs, e1 := s.pbSumSessions(ctx, p.From, p.To)
	psess, psecs, e2 := s.pbSumSessions(ctx, p.PrevFrom, p.PrevTo)
	from, to, _ := pbRange(p.From, p.To)
	pfrom, pto, _ := pbRange(p.PrevFrom, p.PrevTo)
	activeUsers := func(a, b int64) (int64, error) {
		var n int64
		err := s.accountsDB().QueryRowContext(ctx,
			`SELECT COUNT(DISTINCT user_id) FROM watch_sessions WHERE started_at >= ? AND started_at < ?`, a, b).Scan(&n)
		return n, err
	}
	users, e3 := activeUsers(from, to)
	pusers, e4 := activeUsers(pfrom, pto)
	peak, truncated, e5 := s.pbPeakConcurrent(r, from, to)
	load, e6 := s.pbLoad(ctx, p.From, p.To)
	if err := firstErr(e1, e2, e3, e4, e5, e6); err != nil {
		pbServerError(w, err)
		return
	}
	hours, phours := float64(secs)/3600, float64(psecs)/3600

	c := s.pbDeps().costs
	prorata := float64(p.Days) / 30
	var server, bandwidth, storage *float64
	if c.ServerMonth != nil {
		v := *c.ServerMonth * prorata
		server = &v
	}
	if c.BandwidthPerGB != nil && c.GBPerWatchHour != nil {
		v := *c.BandwidthPerGB * *c.GBPerWatchHour * hours
		bandwidth = &v
	}
	// Storage cost needs the stored volume, which is not measured: it stays null.
	_ = c.StoragePerGBMonth

	var total *float64
	for _, v := range []*float64{server, bandwidth, storage} {
		if v != nil {
			t := *v
			if total != nil {
				t += *total
			}
			total = &t
		}
	}
	costs := map[string]any{
		"currency_unit":   "unit of the configured prices (no currency is assumed)",
		"server":          pbComponent(server, "GAZES_COST_SERVER_MONTH"),
		"bandwidth":       pbComponent(bandwidth, "GAZES_COST_BANDWIDTH_PER_GB", "GAZES_GB_PER_WATCH_HOUR"),
		"storage":         pbComponent(storage, "GAZES_COST_STORAGE_PER_GB_MONTH", "stored volume (not measured)"),
		"total":           map[string]any{"value": nil, "measured": false},
		"per_watch_hour":  map[string]any{"value": nil, "measured": false},
		"per_active_user": map[string]any{"value": nil, "measured": false},
		"server_prorata":  prorata,
	}
	if total != nil {
		costs["total"] = map[string]any{"value": *total, "measured": true, "partial": server == nil || bandwidth == nil || storage == nil}
		if hours > 0 {
			costs["per_watch_hour"] = map[string]any{"value": *total / hours, "measured": true}
		}
		if users > 0 {
			costs["per_active_user"] = map[string]any{"value": *total / float64(users), "measured": true}
		}
	}
	writeData(w, p, map[string]any{
		"usage": map[string]any{
			"watch_hours":  pbNewKPI(hours, phours),
			"sessions":     pbNewKPI(float64(sess), float64(psess)),
			"active_users": pbNewKPI(float64(users), float64(pusers)),
			"peak_concurrent_sessions": map[string]any{
				"value": peak, "estimated": true, "truncated": truncated,
				"method": "sweep over watch_sessions (started_at..updated_at)",
			},
		},
		"load":  load,
		"costs": costs,
	})
}

// pbPeakConcurrent estimates the highest number of simultaneous sessions in [from, to) by a
// sweep over [started_at, updated_at] intervals.
// TODO(rollup): a metrics_hourly peak column would avoid this bounded scan of watch_sessions.
func (s *Service) pbPeakConcurrent(r *http.Request, from, to int64) (peak int, truncated bool, err error) {
	rows, err := s.accountsDB().QueryContext(r.Context(),
		`SELECT started_at, MAX(updated_at, started_at) FROM watch_sessions WHERE started_at >= ? AND started_at < ? LIMIT ?`,
		from, to, pbMaxPeakRows+1)
	if err != nil {
		return 0, false, err
	}
	defer rows.Close()
	type ev struct {
		t int64
		d int
	}
	var evs []ev
	n := 0
	for rows.Next() {
		var a, b int64
		if err := rows.Scan(&a, &b); err != nil {
			return 0, false, err
		}
		n++
		if n > pbMaxPeakRows {
			truncated = true
			break
		}
		evs = append(evs, ev{a, +1}, ev{b + 1, -1})
	}
	if err := rows.Err(); err != nil {
		return 0, false, err
	}
	// Ends sort before starts at the same instant (b+1 closes an interval inclusive of b).
	sort.Slice(evs, func(i, j int) bool {
		if evs[i].t != evs[j].t {
			return evs[i].t < evs[j].t
		}
		return evs[i].d < evs[j].d
	})
	cur := 0
	for _, e := range evs {
		cur += e.d
		peak = max(peak, cur)
	}
	return peak, truncated, nil
}
