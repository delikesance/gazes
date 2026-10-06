package stream

import (
	"context"
	"errors"
	"fmt"
	"github.com/gazes/gazes/internal/diagnostics"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"strings"
)

// ErrRemuxFailed marks an ffmpeg remux that exited with an error (not a client disconnect).
var ErrRemuxFailed = errors.New("ffmpeg remux failed")

// remuxSlots bounds concurrent ffmpeg remuxes: the route is public, each request spawns a process.
var remuxSlots = make(chan struct{}, 8)

// RemuxStream pipes a video stream through FFmpeg, re-wrapping into fragmented MP4 (fMP4) with AAC audio.
func RemuxStream(ctx context.Context, w http.ResponseWriter, src io.Reader, logger *slog.Logger, opts PipelineOptions) error {
	logger = diagnostics.Logger(ctx, logger)
	select {
	case remuxSlots <- struct{}{}:
		defer func() { <-remuxSlots }()
	default:
		w.Header().Set("Retry-After", "2")
		http.Error(w, "too many concurrent remuxes", http.StatusServiceUnavailable)
		return nil
	}
	// Set HTTP response headers for fragmented streaming
	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	w.Header().Set("Connection", "keep-alive")

	// If ResponseWriter supports Flusher, flush headers immediately to initiate browser player buffering
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	// Construct FFmpeg command arguments optimized for low latency:
	// - -probesize 1000000 & -analyzeduration 1000000: Minimize stream probe delay to <100ms
	// - flush_packets: Flush output without dropping packets while probing
	// - -i pipe:0: Read from stdin
	// - -map 0:v:0: Map first video track
	// - -map 0:a:0?: Map first audio track (if present)
	// - -c:v copy: Direct stream copy (ZERO re-encoding / ~0% CPU)
	// - Copy AAC; convert incompatible audio to stereo AAC.
	// - delay_moov: Preserve the common audio/video timeline across a seek.
	// - -f mp4 -movflags frag_keyframe+empty_moov+default_base_moof+delay_moov: Fragmented MP4 for live streaming
	// - -frag_duration 500000: 0.5s fragment output for near-instantaneous player startup
	// - pipe:1: Output to stdout
	args := []string{
		"-hide_banner",
		"-loglevel", "error",
		"-probesize", "1000000",
		"-analyzeduration", "1000000",
		"-fflags", "+flush_packets",
	}

	if opts.TimeOffset > 0 {
		args = append(args, "-ss", fmt.Sprintf("%.3f", opts.TimeOffset))
	}

	inputSrc := "pipe:0"
	if opts.InputURL != "" {
		inputSrc = opts.InputURL
	}

	audioMap := fmt.Sprintf("0:a:%d", opts.AudioTrackIndex)
	if opts.AudioTrackIndex < 0 {
		audioMap = "0:a:0"
	}

	args = append(args,
		"-i", inputSrc,
		"-map", "0:v:0",
		"-c:v", "copy",
	)
	if !opts.NoAudio {
		args = append(args, "-map", audioMap)
		if strings.EqualFold(opts.AudioCodec, "aac") {
			args = append(args, "-c:a", "copy")
		} else {
			// Accurate input seeking trims encoded audio to the requested time.
			args = append(args, "-c:a", "aac", "-b:a", "128k", "-ac", "2")
		}
	}
	args = append(args, "-sn", "-dn")
	if strings.EqualFold(opts.VideoCodec, "hevc") || strings.EqualFold(opts.VideoCodec, "h265") {
		args = append(args, "-tag:v", "hvc1")
	}
	args = append(args,
		"-threads", "0",
		"-max_muxing_queue_size", "1024",
		"-f", "mp4",
		"-movflags", "frag_keyframe+empty_moov+default_base_moof+delay_moov",
		"-avoid_negative_ts", "disabled",
		"-frag_duration", "1000000",
		"-flush_packets", "1",
		"pipe:1",
	)

	ffmpegBin := FFmpegPath()
	cmd := exec.CommandContext(ctx, ffmpegBin, args...)
	if inputSrc == "pipe:0" {
		cmd.Stdin = src
	}
	cmd.Stdout = w

	var stderrBuf diagnostics.LimitedBuffer
	cmd.Stderr = &stderrBuf

	logger.Info("starting on-the-fly ffmpeg remux pipeline", "input", inputSrc, "time_offset", opts.TimeOffset, "video_codec", opts.VideoCodec, "audio_codec", opts.AudioCodec, "video_mode", "copy", "audio_track", opts.AudioTrackIndex)

	if err := cmd.Run(); err != nil {
		// If context was cancelled (client disconnected / seeked), this is normal behavior
		if ctx.Err() != nil {
			logger.Debug("ffmpeg pipeline aborted by client disconnect")
			return nil
		}
		logger.Error("ffmpeg pipeline execution error", "err", err, "stderr", stderrBuf.String())
		return fmt.Errorf("%w: %w, stderr: %s", ErrRemuxFailed, err, stderrBuf.String())
	}

	logger.Debug("media.remux_completed")
	return nil
}

// FFmpegPath locates the ffmpeg binary: $FFMPEG_PATH, then PATH, then the usual install dirs.
func FFmpegPath() string {
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
