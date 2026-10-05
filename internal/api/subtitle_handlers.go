package api

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/gazes/gazes/internal/admin"
	"github.com/gazes/gazes/internal/diagnostics"
	"math"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"time"
)

// HandleSubtitles extracts textual subtitles as WebVTT or styled ASS, or raw PGS (sup).
func (s *Server) HandleSubtitles(w http.ResponseWriter, r *http.Request) {
	ih := r.URL.Query().Get("ih")
	if ih == "" {
		http.Error(w, "missing infohash parameter 'ih'", http.StatusBadRequest)
		return
	}

	fileIdx := 0
	if idxStr := r.URL.Query().Get("file_idx"); idxStr != "" {
		if idx, err := strconv.Atoi(idxStr); err == nil && idx >= 0 {
			fileIdx = idx
		}
	}

	trackIdx := 0
	if trackStr := r.URL.Query().Get("track_idx"); trackStr != "" {
		if idx, err := strconv.Atoi(trackStr); err == nil && idx >= 0 {
			trackIdx = idx
		}
	}

	port := 8090
	if s.cfg != nil && s.cfg.Port > 0 {
		port = s.cfg.Port
	}
	inputURL := fmt.Sprintf("http://127.0.0.1:%d/api/v1/stream/raw?ih=%s&file_idx=%d", port, ih, fileIdx)

	format := r.URL.Query().Get("format")
	if format == "" {
		format = "webvtt"
	}
	if format != "webvtt" && format != "ass" && format != "sup" {
		http.Error(w, "unsupported subtitle format", http.StatusBadRequest)
		return
	}
	// Read only the requested playback window. Input-side -t also stops at the
	// boundary when subtitle packets are sparse; output-side -t alone may not.
	start, duration := 0.0, 120.0
	origin := 0.0
	absoluteTimeline := r.URL.Query().Has("timeline_origin")
	if absoluteTimeline {
		value, err := strconv.ParseFloat(r.URL.Query().Get("timeline_origin"), 64)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || math.Abs(value) > 604800 {
			http.Error(w, "invalid timeline origin", 400)
			return
		}
		origin = value
	}
	windowed := r.URL.Query().Has("start") || r.URL.Query().Has("duration")
	if windowed {
		for _, field := range []struct {
			name  string
			value *float64
		}{{"start", &start}, {"duration", &duration}} {
			if raw := r.URL.Query().Get(field.name); raw != "" {
				value, err := strconv.ParseFloat(raw, 64)
				if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
					http.Error(w, "invalid subtitle window", http.StatusBadRequest)
					return
				}
				*field.value = value
			}
		}
		if start < 0 || start > 604800 || duration < 1 || duration > 120 {
			http.Error(w, "invalid subtitle window", http.StatusBadRequest)
			return
		}
	}
	contentType := "text/vtt; charset=utf-8"
	switch format {
	case "ass":
		contentType = "text/x-ssa; charset=utf-8"
	case "sup":
		// Bitmap tracks (PGS) are copied as-is, never converted: the browser renders them.
		contentType = "application/octet-stream"
	}
	w.Header().Set("Cache-Control", "public, max-age=86400")

	codec := format
	if format == "sup" {
		codec = "copy"
	}

	args := []string{
		"-hide_banner",
		"-loglevel", "error",
		"-probesize", "1000000",
		"-analyzeduration", "1000000",
	}
	if windowed {
		// Retain episode timestamps for both ASS and PGS, including after seeks.
		args = append(args, "-copyts")
		if !absoluteTimeline {
			args = append(args, "-start_at_zero")
		}
		args = append(args, "-ss", strconv.FormatFloat(start+origin, 'f', -1, 64), "-t", strconv.FormatFloat(duration, 'f', -1, 64))
	}
	args = append(args,
		"-i", inputURL,
		"-map", fmt.Sprintf("0:s:%d", trackIdx),
		"-c:s", codec,
	)
	if windowed {
		// Some text decoders return buffered cues beyond the input limit.
		args = append(args, "-to", strconv.FormatFloat(start+duration+origin, 'f', -1, 64))
	}
	if absoluteTimeline {
		args = append(args, "-output_ts_offset", strconv.FormatFloat(-origin, 'f', -1, 64))
	}
	args = append(args, "-f", format, "pipe:1")

	// Identical requests share one extraction. It outlives a request that times out, so the
	// client's retry collects the result instead of reading the swarm again from scratch.
	key := fmt.Sprintf("%s|%d|%d|%s|%v|%g|%g|%v|%g", ih, fileIdx, trackIdx, format, windowed, start, duration, absoluteTimeline, origin)
	job := s.subtitles.join(key, diagnostics.Logger(r.Context(), s.logger), args, findFFmpegBin())
	timer := time.NewTimer(subtitleWaitTimeout)
	defer timer.Stop()
	select {
	case <-job.done:
		s.subtitles.leave(job, false)
	case <-r.Context().Done():
		s.subtitles.leave(job, true)
		return
	case <-timer.C:
		s.subtitles.leave(job, false)
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Retry-After", "1")
		http.Error(w, "Les sous-titres ne sont pas encore disponibles. Réessayez.", http.StatusGatewayTimeout)
		return
	}
	if job.err != nil {
		if r.Context().Err() != nil {
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		if errors.Is(job.err, context.DeadlineExceeded) || errors.Is(job.err, context.Canceled) {
			w.Header().Set("Retry-After", "1")
			http.Error(w, "Les sous-titres ne sont pas encore disponibles. Réessayez.", http.StatusGatewayTimeout)
			return
		}
		s.recordPlaybackError(r.Context(), admin.PlaybackError{Code: diagnostics.SubtitleFailed, InfoHash: ih, Message: job.err.Error()})
		http.Error(w, "Impossible d’extraire les sous-titres.", http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", contentType)
	http.ServeContent(w, r, "subtitles", time.Time{}, bytes.NewReader(job.data))
}

func findFFmpegBin() string {
	if p := os.Getenv("FFMPEG_PATH"); p != "" {
		if _, err := exec.LookPath(p); err == nil {
			return p
		}
	}
	if p, err := exec.LookPath("ffmpeg"); err == nil {
		return p
	}
	candidates := []string{
		"/usr/local/bin/ffmpeg",
		"/usr/bin/ffmpeg",
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return "ffmpeg"
}
