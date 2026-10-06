package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func publicStatus(t *testing.T, e *pbEnv) (string, *httptest.ResponseRecorder) {
	t.Helper()
	rec := httptest.NewRecorder()
	e.svc.PublicStatus(rec, httptest.NewRequest("GET", "/api/v1/status", nil))
	var out map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out["playback"], rec
}

func TestPublicStatusFollowsRecentFailures(t *testing.T) {
	e := newPBEnv(t)
	now := e.svc.now()
	if got, rec := publicStatus(t, e); got != "ok" || rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != "public, max-age=30" {
		t.Fatalf("quiet site: %q %d %v", got, rec.Code, rec.Header())
	}
	for i := 0; i < 4; i++ {
		e.exec(t, `INSERT INTO playback_errors(ts, code) VALUES (?, 'X')`, now.Add(-time.Minute).Unix())
	}
	if got, _ := publicStatus(t, e); got != "ok" {
		t.Fatalf("a few failures are noise, got %q", got)
	}
	e.exec(t, `INSERT INTO playback_errors(ts, code) VALUES (?, 'X')`, now.Add(-time.Minute).Unix())
	if got, _ := publicStatus(t, e); got != "degraded" {
		t.Fatalf("failures with no successful start must degrade, got %q", got)
	}
	for i := 0; i < 20; i++ {
		e.exec(t, `INSERT INTO playback_startups(ts, ms) VALUES (?, 900)`, now.Add(-time.Minute).Unix())
	}
	if got, _ := publicStatus(t, e); got != "ok" {
		t.Fatalf("plenty of successful starts outweigh a few failures, got %q", got)
	}
	e.exec(t, `DELETE FROM playback_startups`)
	e.exec(t, `DELETE FROM playback_errors`)
	e.exec(t, `INSERT INTO playback_errors(ts, code) SELECT ?, 'OLD' FROM (SELECT 1 UNION SELECT 2 UNION SELECT 3 UNION SELECT 4 UNION SELECT 5 UNION SELECT 6)`, now.Add(-time.Hour).Unix())
	if got, _ := publicStatus(t, e); got != "ok" {
		t.Fatalf("old failures must not count, got %q", got)
	}
}
