package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gazes/gazes/internal/config"
)

func TestCORSIsOffByDefaultAndRestrictedToConfiguredOrigins(t *testing.T) {
	get := func(cfg *config.Config, origin string) string {
		r := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		r.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		NewServer(cfg, nil, nil, nil, nil).Router().ServeHTTP(w, r)
		return w.Header().Get("Access-Control-Allow-Origin")
	}
	if got := get(&config.Config{}, "https://evil.example"); got != "" {
		t.Fatalf("CORS header without ENABLE_CORS: %q", got)
	}
	cfg := &config.Config{EnableCORS: true, CORSAllowedOrigins: []string{"https://app.example"}}
	if got := get(cfg, "https://app.example"); got != "https://app.example" {
		t.Fatalf("allowed origin not honoured: %q", got)
	}
	if got := get(cfg, "https://evil.example"); got != "" {
		t.Fatalf("unlisted origin allowed: %q", got)
	}
}
