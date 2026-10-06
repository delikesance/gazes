package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/gazes/gazes/internal/kv"
	"github.com/gazes/gazes/internal/metadata"
)

// mirrorEvery paces the catalog import: one AniList request per period across the whole fleet,
// about half of the 24/min budget, the rest stays with visitors (the mirror runs as Background).
const mirrorEvery = 5 * time.Second

// startMirror copies the AniList library to disk and keeps it fresh (see metadata.MirrorStep).
// The elected instance does one step per period; a throttle pauses it for the cooldown.
func (s *Server) startMirror() {
	go func() {
		time.Sleep(20 * time.Second)
		for {
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			wait := mirrorEvery
			if s.kv.Elect(ctx, "mirror", mirrorEvery) {
				before, _ := s.catalogService.MirrorProgress()
				progress, err := s.catalogService.MirrorStep(ctx)
				var limited *metadata.RateLimitError
				switch {
				case errors.Is(err, metadata.ErrMirrorDisabled):
					cancel()
					return
				case errors.As(err, &limited):
					wait = limited.RetryAfter + time.Second
				case err != nil:
					s.logger.Warn("catalog.mirror_failed", "err", err)
					wait = 30 * time.Second
				case progress.Complete && !before.Complete:
					s.logger.Info("catalog.mirror_complete", "imported", progress.Imported)
				case !progress.Complete && progress.Imported/1000 != before.Imported/1000:
					s.logger.Info("catalog.mirror_progress", "imported", progress.Imported, "after_id", progress.AfterID)
				}
				if progress.Complete && err == nil {
					wait = time.Minute // idle until the daily refresh is due
				}
			}
			cancel()
			time.Sleep(wait)
		}
	}()
}

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
	ctx = kv.Background(kv.WithRenewWithin(ctx, warmRenewWithin))
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
	out := s.cacheDiagnostics(r.Context())
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(out)
}

// cacheDiagnostics builds the /api/v1/diagnostics/cache body (also read by the admin panel).
func (s *Server) cacheDiagnostics(ctx context.Context) map[string]any {
	out := map[string]any{"redis": "disabled"}
	if s.kv != nil {
		ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
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
	return out
}
