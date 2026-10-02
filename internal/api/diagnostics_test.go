package api

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gazes/gazes/internal/diagnostics"
)

func TestDiagnosticEventsValidationAndRate(t *testing.T) {
	s := &Server{}
	for _, tc := range []struct {
		body   string
		status int
	}{
		{`{"events":[{"event":"playback.failed","playback_session_id":"session-1","attempt_id":"attempt-1","anime_id":"101280","episode":"1","attributes":{"reason":"metadata timeout","error_code":"metadata_timeout"}}]}`, 204},
		{`{"events":[{"event":"arbitrary","playback_session_id":"s"}]}`, 400},
		{`{"events":[{"event":"playback.failed","playback_session_id":"s","attributes":{"apikey":"secret"}}]}`, 400},
		{`{"events":[{"event":"playback.failed","playback_session_id":"s","attributes":{"reason":{"nested":"bad"}}}]}`, 400},
		{`{"events":[]}`, 400},
		{`{"events":[]} {}`, 400},
		{strings.Repeat("x", 33<<10), 400},
	} {
		r := httptest.NewRequest("POST", "/api/v1/diagnostics/events", strings.NewReader(tc.body))
		w := httptest.NewRecorder()
		s.HandleDiagnosticEvents(w, r)
		if w.Code != tc.status {
			t.Fatalf("%s: %d", tc.body[:min(len(tc.body), 70)], w.Code)
		}
	}
	for range 120 {
		s.diagnosticRate.allow("test")
	}
	if s.diagnosticRate.allow("test") {
		t.Fatal("rate limit ignored")
	}
}
func TestDiagnosticRequestCorrelation(t *testing.T) {
	o := diagnostics.Defaults()
	o.Dir = t.TempDir()
	o.Output = io.Discard
	o.Errors = io.Discard
	store, _ := diagnostics.Open(o)
	old := slog.Default()
	slog.SetDefault(slog.New(store.Handler()))
	defer slog.SetDefault(old)
	s := &Server{}
	handler := s.diagnosticContext(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := diagnostics.Get(r.Context())
		if c.SessionID != "session" || c.AttemptID != "attempt" {
			t.Fatal("missing context")
		}
		diagnostics.Log(r.Context(), slog.LevelInfo, "test.correlated")
		w.WriteHeader(204)
	}))
	r := httptest.NewRequest("GET", "/api/v1/test?playback_session_id=session&attempt_id=attempt", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Header().Get("X-Request-ID") == "" || w.Header().Get("X-Playback-Session-ID") != "session" {
		t.Fatal("missing response IDs")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	store.Close(ctx)
	var out bytes.Buffer
	_, e := diagnostics.Read(ctx, o.Dir+"/events.sqlite", diagnostics.Filter{Session: "session"}, &out)
	if e != nil || !strings.Contains(out.String(), "test.correlated") {
		t.Fatalf("correlation not persisted: %v %s", e, out.String())
	}
}

func TestDiagnosticBatchKeepsValidEventsWhenOneIsRejected(t *testing.T) {
	s := &Server{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	body := `{"events":[
		{"event":"playback.failed","playback_session_id":"s","infohash":"` + strings.Repeat("a", 40) + `","attributes":{"error_code":"startup_timeout"}},
		{"event":"unknown.event","playback_session_id":"s"},
		{"event":"playback.subtitle_failed","playback_session_id":"s","attributes":{"error_code":"SUB_HTTP_502"}}]}`
	rr := httptest.NewRecorder()
	s.HandleDiagnosticEvents(rr, httptest.NewRequest("POST", "/api/v1/diagnostics/events", strings.NewReader(body)))
	if rr.Code != 204 {
		t.Fatalf("a batch with valid events must be accepted even when one event is unknown, got %d", rr.Code)
	}
	rr = httptest.NewRecorder()
	s.HandleDiagnosticEvents(rr, httptest.NewRequest("POST", "/api/v1/diagnostics/events", strings.NewReader(`{"events":[{"event":"unknown.event","playback_session_id":"s"}]}`)))
	if rr.Code != 400 {
		t.Fatalf("a batch with no valid event must still be refused, got %d", rr.Code)
	}
}
