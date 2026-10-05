package admin

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"
)

// Pagination bounds.
const (
	DefaultLimit = 50
	MaxLimit     = 200
)

// Period is the window selected by ?period=7|30|90. From..To are inclusive UTC days
// (To is the current day); PrevFrom..PrevTo is the immediately preceding window of the
// same length, for comparisons.
type Period struct {
	Days     int    `json:"days"`
	From     string `json:"from"`
	To       string `json:"to"`
	PrevFrom string `json:"prev_from"`
	PrevTo   string `json:"prev_to"`
}

// parsePeriod reads ?period= (7, 30 or 90; default 30). Anything else is an error: answer 400.
func parsePeriod(r *http.Request, now time.Time) (Period, error) {
	days := 30
	if v := r.URL.Query().Get("period"); v != "" {
		switch v {
		case "7":
			days = 7
		case "30":
			days = 30
		case "90":
			days = 90
		default:
			return Period{}, errors.New("period must be 7, 30 or 90")
		}
	}
	to := now.UTC().Truncate(24 * time.Hour)
	from := to.AddDate(0, 0, -(days - 1))
	prevTo := from.AddDate(0, 0, -1)
	prevFrom := prevTo.AddDate(0, 0, -(days - 1))
	return Period{Days: days, From: from.Format(dayLayout), To: to.Format(dayLayout),
		PrevFrom: prevFrom.Format(dayLayout), PrevTo: prevTo.Format(dayLayout)}, nil
}

// Page is a bounded pagination window.
type Page struct{ Limit, Offset int }

// parsePagination reads ?limit= (default 50, capped at 200, must be >= 1) and ?offset= (default 0, >= 0).
func parsePagination(r *http.Request) (Page, error) {
	p := Page{Limit: DefaultLimit}
	q := r.URL.Query()
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return Page{}, errors.New("limit must be a positive integer")
		}
		p.Limit = min(n, MaxLimit)
	}
	if v := q.Get("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return Page{}, errors.New("offset must be a non-negative integer")
		}
		p.Offset = n
	}
	return p, nil
}

// delta is the percentage change from prev to cur, or nil when prev == 0.
func delta(cur, prev float64) *float64 {
	if prev == 0 {
		return nil
	}
	d := (cur - prev) / prev * 100
	return &d
}

type envelope struct {
	GeneratedAt string  `json:"generated_at"`
	Period      *Period `json:"period,omitempty"`
	Data        any     `json:"data"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeData answers 200 with {"generated_at","period","data"}.
func writeData(w http.ResponseWriter, p Period, data any) {
	writeJSON(w, http.StatusOK, envelope{GeneratedAt: time.Now().UTC().Format(time.RFC3339), Period: &p, Data: data})
}

// writeDataNoPeriod is writeData for endpoints that have no period.
func writeDataNoPeriod(w http.ResponseWriter, data any) {
	writeJSON(w, http.StatusOK, envelope{GeneratedAt: time.Now().UTC().Format(time.RFC3339), Data: data})
}

// writeAPIError answers {"error":{"code","message"}}.
func writeAPIError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": msg}})
}
