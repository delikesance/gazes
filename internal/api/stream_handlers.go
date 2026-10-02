package api

import (
	"fmt"
	"github.com/gazes/gazes/internal/diagnostics"
	"net/http"
	"strconv"

	"github.com/gazes/gazes/internal/stream"
)

// HandleStream handles HTTP streaming and remuxing requests for a specific torrent file.
func (s *Server) HandleStream(w http.ResponseWriter, r *http.Request) {
	ih := r.URL.Query().Get("ih")
	if ih == "" {
		http.Error(w, `{"error": "missing infohash parameter 'ih'"}`, http.StatusBadRequest)
		return
	}

	fileIdx := 0
	if idxStr := r.URL.Query().Get("file_idx"); idxStr != "" {
		if idx, err := strconv.Atoi(idxStr); err == nil && idx >= 0 {
			fileIdx = idx
		}
	}

	forceRemux := false
	if remuxStr := r.URL.Query().Get("remux"); remuxStr == "true" || remuxStr == "1" {
		forceRemux = true
	}

	reader, fileInfo, err := s.torrentEngine.GetFileStream(r.Context(), ih, fileIdx)
	if err != nil {
		diagnostics.Logger(r.Context(), s.logger).Error("failed to get torrent file stream", "err", err, "infohash", ih, "file_idx", fileIdx)
		http.Error(w, `{"error": "failed to open torrent stream"}`, http.StatusNotFound)
		return
	}

	var timeOffset float64
	if toStr := r.URL.Query().Get("time_offset"); toStr != "" {
		if to, err := strconv.ParseFloat(toStr, 64); err == nil && to >= 0 {
			timeOffset = to
		}
	}

	audioTrack := 0
	if atStr := r.URL.Query().Get("audio_track"); atStr != "" {
		if at, err := strconv.Atoi(atStr); err == nil && at >= 0 {
			audioTrack = at
		}
	}

	port := 8090
	if s.cfg != nil && s.cfg.Port > 0 {
		port = s.cfg.Port
	}
	inputURL := fmt.Sprintf("http://127.0.0.1:%d/api/v1/stream/raw?ih=%s&file_idx=%d", port, ih, fileIdx)

	opts := stream.PipelineOptions{
		AudioTrackIndex: audioTrack,
		ForceRemux:      forceRemux,
		TimeOffset:      timeOffset,
		InputURL:        inputURL,
	}

	if err := s.streamPipeline.ServeHTTP(w, r, reader, fileInfo.Path, fileInfo.Length, opts); err != nil {
		diagnostics.Logger(r.Context(), s.logger).Error("stream serving error", "err", err, "file", fileInfo.Path)
	}
}

// HandleStreamRaw serves the raw torrent file stream supporting HTTP 206 Partial Content Range requests.
func (s *Server) HandleStreamRaw(w http.ResponseWriter, r *http.Request) {
	ih := r.URL.Query().Get("ih")
	if ih == "" {
		http.Error(w, `{"error": "missing infohash parameter 'ih'"}`, http.StatusBadRequest)
		return
	}

	fileIdx := 0
	if idxStr := r.URL.Query().Get("file_idx"); idxStr != "" {
		if idx, err := strconv.Atoi(idxStr); err == nil && idx >= 0 {
			fileIdx = idx
		}
	}

	reader, fileInfo, err := s.torrentEngine.GetFileStream(r.Context(), ih, fileIdx)
	if err != nil {
		diagnostics.Logger(r.Context(), s.logger).Error("failed to get raw torrent stream", "err", err, "infohash", ih, "file_idx", fileIdx)
		http.Error(w, `{"error": "failed to open raw stream"}`, http.StatusNotFound)
		return
	}
	defer reader.Close()

	if err := stream.ServeRange(w, r, reader, fileInfo.Path, fileInfo.Length); err != nil {
		diagnostics.Logger(r.Context(), s.logger).Error("raw stream range error", "err", err, "file", fileInfo.Path)
	}
}
