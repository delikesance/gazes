package admin

import (
	"encoding/json"
	"net/http"
	"time"
)

// statusWindow is how far back the public status looks, and statusMinErrors how many failures
// it takes before a quiet site is called degraded (a lone failure is noise).
const (
	statusWindow    = 15 * time.Minute
	statusMinErrors = 5
)

// PublicStatus answers GET /api/v1/status without authentication. It reveals one word about
// playback health and the scheduled maintenance, nothing about users, sources or volumes.
func (s *Service) PublicStatus(w http.ResponseWriter, r *http.Request) {
	since := s.now().Add(-statusWindow).Unix()
	var errs, starts int64
	e1 := s.adminDB().QueryRowContext(r.Context(), `SELECT COUNT(*) FROM playback_errors WHERE ts >= ?`, since).Scan(&errs)
	e2 := s.adminDB().QueryRowContext(r.Context(), `SELECT COUNT(*) FROM playback_startups WHERE ts >= ?`, since).Scan(&starts)
	status := "ok"
	if e1 != nil || e2 != nil {
		status = "unknown"
	} else if errs >= statusMinErrors && errs >= starts {
		status = "degraded"
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=30")
	out := map[string]any{"playback": status}
	if m, ok := s.Maintenance(r.Context()); ok {
		out["maintenance"] = m
	}
	_ = json.NewEncoder(w).Encode(out)
}
