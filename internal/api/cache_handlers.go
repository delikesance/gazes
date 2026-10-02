package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/gazes/gazes/internal/kv"
)

// warmEvery is how often the elected instance renews the home-page data; entries expiring within
// warmRenewWithin are renewed so no visitor waits on AniList.
const (
	warmEvery       = 9 * time.Minute
	warmRenewWithin = 12 * time.Minute
)

// startWarmer keeps the catalog data behind the home page fresh. Exactly one instance per period
// does the work (elected through Redis); the others only check the clock.
func (s *Server) startWarmer() {
	go func() {
		time.Sleep(5 * time.Second)
		for {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			if s.kv.Elect(ctx, "warm", warmEvery) {
				s.warmOnce(ctx)
			}
			cancel()
			time.Sleep(time.Minute)
		}
	}()
}

func (s *Server) warmOnce(ctx context.Context) {
	ctx = kv.WithRenewWithin(ctx, warmRenewWithin)
	if _, err := s.catalogService.GetPopular(ctx, 1, 12); err != nil {
		s.logger.Debug("cache.warm_failed", "what", "popular", "err", err)
	}
	if _, err := s.catalogService.GetCurrentSeason(ctx, 1, 24); err != nil {
		s.logger.Debug("cache.warm_failed", "what", "seasonal", "err", err)
	}
	if _, err := s.catalogService.GetTrending(ctx, 1, 20); err != nil {
		s.logger.Debug("cache.warm_failed", "what", "trending", "err", err)
	}
}

// HandleCacheDiagnostics reports cache efficiency and upstream pressure: hit ratios, stale answers
// served, lock waits, throttled upstream calls and the remaining shared AniList cooldown.
func (s *Server) HandleCacheDiagnostics(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{"redis": "disabled"}
	if s.kv != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		redis := map[string]any{"status": "ok"}
		if latency, keys, err := s.kv.Health(ctx); err != nil {
			redis["status"] = "unreachable"
		} else {
			redis["latency_ms"] = float64(latency.Microseconds()) / 1000
			redis["keys"] = keys
		}
		out["redis"] = redis
		out["stats"] = s.kv.Stats()
		out["anilist_cooldown_ms"] = s.catalogService.UpstreamCooldown(ctx).Milliseconds()
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(out)
}
