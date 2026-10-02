package api

import (
	"fmt"
	"github.com/gazes/gazes/internal/diagnostics"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"time"
)

// HandleSubtitles extracts textual subtitles as WebVTT or styled ASS.
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
	if format != "webvtt" && format != "ass" {
		http.Error(w, "unsupported subtitle format", http.StatusBadRequest)
		return
	}
	contentType := "text/vtt; charset=utf-8"
	if format == "ass" {
		contentType = "text/x-ssa; charset=utf-8"
	}
	output, err := os.CreateTemp("", "gazes-subtitles-*")
	if err != nil {
		http.Error(w, "subtitle storage unavailable", http.StatusInternalServerError)
		return
	}
	defer os.Remove(output.Name())
	defer output.Close()
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cache-Control", "public, max-age=86400")

	// Timeout context for subtitle extraction
	ctx, cancel := contextWithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	ffmpegBin := findFFmpegBin()
	args := []string{
		"-hide_banner",
		"-loglevel", "error",
		"-probesize", "1000000",
		"-analyzeduration", "1000000",
		"-i", inputURL,
		"-map", fmt.Sprintf("0:s:%d", trackIdx),
		"-c:s", format,
		"-f", format,
		"pipe:1",
	}

	cmd := exec.CommandContext(ctx, ffmpegBin, args...)
	cmd.Stdout = output

	var stderrBuf diagnostics.LimitedBuffer
	cmd.Stderr = &stderrBuf

	if err := cmd.Run(); err != nil {
		if r.Context().Err() != nil {
			return
		}
		diagnostics.Logger(r.Context(), s.logger).Error("failed to extract subtitles", "err", err, "stderr", stderrBuf.String())
		w.Header().Set("Cache-Control", "no-store")
		http.Error(w, "Impossible d’extraire les sous-titres.", http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", contentType)
	http.ServeContent(w, r, "subtitles", time.Time{}, output)
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
