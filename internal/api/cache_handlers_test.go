package api

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/gazes/gazes/internal/kv"
	"github.com/gazes/gazes/internal/metadata"
	"github.com/redis/go-redis/v9"
)

func TestCacheDiagnosticsReportsRedisAndCounters(t *testing.T) {
	mr := miniredis.RunT(t)
	c := kv.New(redis.NewClient(&redis.Options{Addr: mr.Addr()}), "test")
	catalog := metadata.NewAnimeCatalogService(nil)
	catalog.SetRedis(c)
	s := &Server{kv: c, catalogService: catalog}
	rr := httptest.NewRecorder()
	s.HandleCacheDiagnostics(rr, httptest.NewRequest("GET", "/api/v1/diagnostics/cache", nil))
	var out struct {
		Redis  map[string]any `json:"redis"`
		Stats  map[string]any `json:"stats"`
		Cooled *float64       `json:"anilist_cooldown_ms"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Redis["status"] != "ok" || out.Stats["l1_hits"] == nil || out.Cooled == nil || *out.Cooled != 0 {
		t.Fatalf("unexpected diagnostics: %s", rr.Body.String())
	}
}
