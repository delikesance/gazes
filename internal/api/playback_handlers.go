package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"

	"github.com/gazes/gazes/internal/admin"
	"github.com/gazes/gazes/internal/diagnostics"
	"github.com/gazes/gazes/internal/loadstats"
	"github.com/gazes/gazes/internal/playback"
	"github.com/go-chi/chi/v5"
)

var playbackHash = regexp.MustCompile(`^[a-fA-F0-9]{40}$`)

func (s *Server) playbackManager() *playback.Manager {
	s.playbackOnce.Do(func() {
		dir := filepath.Join(s.cfg.CacheDir, "playback")
		os.MkdirAll(dir, 0700)
		s.playback = playback.New(s.torrentEngine, s.analyzer, s.logger, playback.Options{Directory: dir, RawURL: fmt.Sprintf("http://127.0.0.1:%d/api/v1/stream/raw", s.cfg.Port), MemoryBytes: s.cfg.PlaybackMemoryBytes, DiskBytes: s.cfg.PlaybackDiskBytes})
	})
	return s.playback
}
func (s *Server) ClosePlayback() {
	if s.playback != nil {
		s.playback.Close()
	}
}
func (s *Server) HandlePlaybackConfig(w http.ResponseWriter, r *http.Request) {
	engine := "legacy"
	if s.cfg != nil && s.cfg.PlaybackEngine == "hls" {
		engine = "hls"
	}
	w.Header().Set("Cache-Control", "no-store")
	writePlaybackJSON(w, map[string]string{"engine": engine})
}
func writePlaybackJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
func playbackError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": code})
}
func (s *Server) HandlePlaybackCreate(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Hash     string  `json:"info_hash"`
		File     int     `json:"file_index"`
		Audio    int     `json:"audio_track"`
		Position float64 `json:"position"`
		NoAV1    bool    `json:"no_av1"`
		Replaces string  `json:"replaces"` // the viewer's previous session (seek, audio switch), closed first
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048)).Decode(&input); err != nil || !playbackHash.MatchString(input.Hash) || input.File < 0 || input.Audio < 0 {
		playbackError(w, 400, "invalid_session")
		return
	}
	if _, ok := s.playbackManager().Get(input.Replaces); ok {
		s.playbackManager().Delete(input.Replaces)
	}
	// limit_concurrent_streams (admin): new sessions wait while the cap is reached, open ones play on.
	if limit, full := s.streamLimitReached(r.Context(), s.playbackManager().ActiveSessions); full {
		diagnostics.Logger(r.Context(), s.logger).Warn("playback.stream_limit", "limit", limit)
		w.Header().Set("Retry-After", "30")
		playbackError(w, http.StatusServiceUnavailable, "stream_limit_reached")
		return
	}
	session, err := s.playbackManager().Create(r.Context(), input.Hash, input.File, input.Audio, input.Position, input.NoAV1)
	if err != nil {
		code := "media_unavailable"
		status := 502
		if errors.Is(err, playback.ErrIndex) {
			code = "seek_index_unavailable"
			status = 422
		}
		diagnostics.Logger(r.Context(), s.logger).Warn("playback.session_failed", "error_code", code, "err", err)
		if errors.Is(err, context.DeadlineExceeded) { // no peer / start took longer than the manager's deadline
			s.recordPlaybackError(r.Context(), admin.PlaybackError{Code: diagnostics.StreamTimeout, InfoHash: input.Hash, Message: err.Error()})
		}
		playbackError(w, status, code)
		return
	}
	loadstats.Session(r.UserAgent(), input.NoAV1, session.NeedsTranscode())
	diagnostics.Logger(r.Context(), s.logger).Info("playback.session_created", "session_id", session.ID, "timeline_origin", session.Origin, "position", session.Position)
	writePlaybackJSON(w, session)
}

// streamLimitReached reports whether the admin stream cap (limit_concurrent_streams) is reached.
// Without the admin panel there is no cap.
func (s *Server) streamLimitReached(ctx context.Context, active func() int) (int, bool) {
	limit := s.admin.StreamLimit(ctx)
	return limit, limit > 0 && active() >= limit
}
func (s *Server) HandlePlaybackUpdate(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Position   float64 `json:"position"`
		Generation uint64  `json:"generation"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&input) != nil {
		playbackError(w, 400, "invalid_position")
		return
	}
	session, ok := s.playbackManager().Update(chi.URLParam(r, "session"), input.Position, input.Generation)
	if !ok {
		playbackError(w, 404, "session_unavailable")
		return
	}
	writePlaybackJSON(w, session)
}
func (s *Server) HandlePlaybackDelete(w http.ResponseWriter, r *http.Request) {
	s.playbackManager().Delete(chi.URLParam(r, "session"))
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) HandlePlaybackPlaylist(w http.ResponseWriter, r *http.Request) {
	session, ok := s.playbackManager().Get(chi.URLParam(r, "session"))
	if !ok {
		playbackError(w, 404, "session_unavailable")
		return
	}
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Set("Cache-Control", "no-store")
	fmt.Fprint(w, s.playbackManager().Playlist(session))
}
func (s *Server) HandlePlaybackMedia(w http.ResponseWriter, r *http.Request) {
	n, err := strconv.Atoi(chi.URLParam(r, "segment"))
	asset := chi.URLParam(r, "asset")
	if err != nil || n < 0 || (asset != "init.mp4" && asset != "media.m4s") {
		playbackError(w, 400, "invalid_segment")
		return
	}
	err = s.playbackManager().Media(r.Context(), chi.URLParam(r, "session"), n, asset == "init.mp4", w, r)
	if err != nil {
		if r.Context().Err() != nil {
			return
		}
		status := 503
		if errors.Is(err, os.ErrNotExist) {
			status = 404
		}
		w.Header().Set("Retry-After", "1")
		playbackError(w, status, "segment_unavailable")
		diagnostics.Logger(r.Context(), s.logger).Warn("playback.segment_failed", "segment", n, "err", err)
		s.recordSegmentError(r.Context(), err)
	}
}
