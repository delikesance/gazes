package api

import (
	"errors"
	"fmt"
	"github.com/gazes/gazes/internal/admin"
	"github.com/gazes/gazes/internal/diagnostics"
	"net/http"
	"strconv"

	"github.com/gazes/gazes/internal/stream"
)

// HandleStream handles HTTP streaming and remuxing requests for a specific torrent file.
func (s *Server) HandleStream(w http.ResponseWriter, r *http.Request) {
	// ih is interpolated into the loopback URL ffmpeg reads: validate it like /stream/raw does.
	ih, fileIdx, ok := streamTarget(r)
	if !ok {
		http.Error(w, `{"error": "invalid infohash or file_idx"}`, http.StatusBadRequest)
		return
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
		Client:          s.clientKey(r),
	}

	if err := s.streamPipeline.ServeHTTP(w, r, reader, fileInfo.Path, fileInfo.Length, opts); err != nil {
		diagnostics.Logger(r.Context(), s.logger).Error("stream serving error", "err", err, "file", fileInfo.Path)
		if errors.Is(err, stream.ErrRemuxFailed) {
			s.recordPlaybackError(r.Context(), admin.PlaybackError{Code: diagnostics.RemuxFailed, InfoHash: ih, Message: err.Error()})
		}
	}
}

// HandleStreamRaw serves the raw torrent file stream supporting HTTP 206 Partial Content Range requests.
func (s *Server) HandleStreamRaw(w http.ResponseWriter, r *http.Request) {
	ih, fileIdx, ok := streamTarget(r)
	if !ok {
		http.Error(w, `{"error": "invalid infohash or file_idx"}`, http.StatusBadRequest)
		return
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

const (
	maxFileIndex  = 100000
	maxTrackIndex = 1000
)

// boundedIndex parses an optional non-negative index (absent = 0); ok is false for junk or values above max.
func boundedIndex(raw string, max int) (int, bool) {
	if raw == "" {
		return 0, true
	}
	n, err := strconv.Atoi(raw)
	return n, err == nil && n >= 0 && n <= max
}

// streamTarget reads the ih and file_idx query parameters. ih must be a 40-hex id: it is
// interpolated into the loopback URL ffmpeg reads from, so anything else could add parameters.
func streamTarget(r *http.Request) (ih string, fileIdx int, ok bool) {
	ih = r.URL.Query().Get("ih")
	fileIdx, idxOK := boundedIndex(r.URL.Query().Get("file_idx"), maxFileIndex)
	return ih, fileIdx, infoHashPattern.MatchString(ih) && idxOK
}
