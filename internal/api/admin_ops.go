package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gazes/gazes/internal/admin"
	"github.com/gazes/gazes/internal/diagnostics"
	"github.com/gazes/gazes/internal/indexer"
	"github.com/go-chi/chi/v5"
)

// Background source resolutions started by warm_cache: at most opsWarmSlots at once, each bounded
// like a viewer's request.
const (
	opsWarmSlots   = 2
	opsWarmTimeout = sourcesWorkBudget + 20*time.Second
)

// wireAdminOps gives the runtime admin actions their effects (internal/admin/ops_runtime.go).
func (s *Server) wireAdminOps() {
	h := admin.OpsHooks{
		EpisodeSources: s.cachedEpisodeSources,
		WarmEpisode:    s.warmEpisode,
		PurgeCache:     s.purgeCache,
	}
	if mp, ok := s.indexer.(*indexer.MultiProvider); ok && mp != nil {
		h.Sources = mp
	}
	if s.library != nil {
		h.Library = s.library
	}
	s.opsWarm = make(chan struct{}, opsWarmSlots)
	s.admin.SetOpsHooks(h)
}

// cachedEpisodeSources reports the cached findings of one episode, as the player asks for them.
func (s *Server) cachedEpisodeSources(ctx context.Context, seasonID, ep int) (int, bool) {
	res, ok := s.sources().cache.Peek(ctx, sourceCacheKey(s.cfg != nil && s.cfg.ArrAuthoritative, seasonID, ep, false))
	if !ok || res == nil {
		return 0, false
	}
	return len(res.Sources), true
}

// warmEpisode resolves one episode's sources in the background through the same handler a viewer
// hits, so the findings land in the shared cache under the key the player reads.
func (s *Server) warmEpisode(seasonID, ep int, refresh bool) bool {
	select {
	case s.opsWarm <- struct{}{}:
	default:
		return false
	}
	go func() {
		defer func() { <-s.opsWarm }()
		ctx, cancel := context.WithTimeout(context.Background(), opsWarmTimeout)
		defer cancel()
		if refresh {
			s.sources().cache.Invalidate(ctx, sourceCacheKey(s.cfg != nil && s.cfg.ArrAuthoritative, seasonID, ep, false))
		}
		id, num := strconv.Itoa(seasonID), strconv.Itoa(ep)
		rc := chi.NewRouteContext()
		rc.URLParams.Add("id", id)
		rc.URLParams.Add("season", id)
		rc.URLParams.Add("ep", num)
		ctx = context.WithValue(ctx, chi.RouteCtxKey, rc)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("/api/v1/catalog/anime/%s/seasons/%s/episodes/%s/sources", id, id, num), nil)
		if err != nil {
			return
		}
		rec := &statusRecorder{header: http.Header{}}
		s.HandleSeasonSources(rec, req)
		level := slog.LevelInfo
		if rec.status >= http.StatusBadRequest {
			level = slog.LevelWarn
		}
		diagnostics.Log(ctx, level, "admin.warm_cache", "season_id", seasonID, "episode", ep, "status", rec.status)
	}()
	return true
}

// purgeCache empties one purge_cache scope.
func (s *Server) purgeCache(ctx context.Context, scope string) (int, error) {
	switch scope {
	case "episode_sources":
		return s.sources().cache.Purge(ctx)
	case "indexer_results":
		if mp, ok := s.indexer.(*indexer.MultiProvider); ok && mp != nil {
			return mp.PurgeCache(ctx)
		}
		return 0, nil
	}
	return 0, fmt.Errorf("unknown cache scope %q", scope)
}

// statusRecorder is a ResponseWriter that keeps only the status of a background request.
type statusRecorder struct {
	header http.Header
	status int
}

func (r *statusRecorder) Header() http.Header { return r.header }
func (r *statusRecorder) WriteHeader(code int) {
	if r.status == 0 {
		r.status = code
	}
}
func (r *statusRecorder) Write(b []byte) (int, error) {
	r.WriteHeader(http.StatusOK)
	return len(b), nil
}
