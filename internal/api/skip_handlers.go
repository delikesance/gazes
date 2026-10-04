package api

import (
	"encoding/json"
	"math"
	"net/http"
	"strconv"

	"github.com/gazes/gazes/internal/diagnostics"
	"github.com/gazes/gazes/internal/metadata"
	"github.com/go-chi/chi/v5"
)

// maxSkipDuration bounds the file duration the player may report (6 h).
const maxSkipDuration = 21600

// HandleSkipTimes serves the AniSkip opening/ending segments for a season episode. A valid request
// always gets 200: unknown season, missing MAL id or an AniSkip failure all yield an empty list.
func (s *Server) HandleSkipTimes(w http.ResponseWriter, r *http.Request) {
	season, e1 := strconv.Atoi(chi.URLParam(r, "season"))
	ep, e2 := strconv.Atoi(chi.URLParam(r, "ep"))
	duration, e3 := strconv.ParseFloat(r.URL.Query().Get("duration"), 64)
	if e1 != nil || e2 != nil || e3 != nil || season <= 0 || ep <= 0 ||
		math.IsNaN(duration) || math.IsInf(duration, 0) || duration <= 0 || duration > maxSkipDuration {
		http.Error(w, "invalid season, episode or duration", http.StatusBadRequest)
		return
	}
	segments := []metadata.SkipSegment{}
	cacheable := false
	log := diagnostics.Logger(r.Context(), s.logger)
	if item, err := s.catalogService.GetAnimeDetailsWithEpisodes(r.Context(), season); err != nil {
		log.Warn("skip times: season lookup failed", "err", err, "season", season)
	} else if item.MalID > 0 {
		if found, err := s.catalogService.SkipTimes(r.Context(), item.MalID, ep, duration); err != nil {
			log.Warn("skip times: aniskip failed", "err", err, "mal_id", item.MalID, "episode", ep)
		} else {
			if found != nil {
				segments = found
			}
			cacheable = true
		}
	}
	if cacheable {
		w.Header().Set("Cache-Control", "private, max-age=3600")
	} else {
		w.Header().Set("Cache-Control", "no-store")
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string][]metadata.SkipSegment{"segments": segments})
}
