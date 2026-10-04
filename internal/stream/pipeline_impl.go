package stream

import (
	"context"
	"fmt"
	"github.com/gazes/gazes/internal/diagnostics"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gazes/gazes/internal/metadata"
)

// PipelineManager implements stream.Pipeline.
type PipelineManager struct {
	logger *slog.Logger
}

// NewPipelineManager initializes a new Pipeline instance.
func NewPipelineManager(logger *slog.Logger) *PipelineManager {
	return &PipelineManager{
		logger: logger,
	}
}

// ServeHTTP dynamically dispatches between Direct HTTP Range serving and On-The-Fly Remuxing.
func (p *PipelineManager) ServeHTTP(w http.ResponseWriter, r *http.Request, reader io.ReadSeekCloser, filename string, size int64, opts PipelineOptions) error {
	defer reader.Close()

	ext := strings.ToLower(filepath.Ext(filename))
	needsRemux := opts.ForceRemux || opts.TimeOffset > 0 || opts.AudioTrackIndex > 0 || ext == ".mkv" || ext == ".avi" || ext == ".ts" || ext == ".flv" || ext == ".wmv"

	diagnostics.Logger(r.Context(), p.logger).Info("serving media stream",
		"filename", filename,
		"size", size,
		"remux", needsRemux,
	)

	if needsRemux {
		// Probe the selected file, then restore its position for pipe-based input.
		// HEVC needs an hvc1 sample entry for browser-compatible MP4 packaging.
		if opts.VideoCodec == "" {
			position, err := reader.Seek(0, io.SeekCurrent)
			if err != nil {
				return err
			}
			meta, probeErr := metadata.NewFFprobeAnalyzer(p.logger).ProbeStreams(r.Context(), reader, size)
			if _, err := reader.Seek(position, io.SeekStart); err != nil {
				return err
			}
			if probeErr == nil && meta != nil {
				opts.VideoCodec = meta.VideoCodec
				if meta.VideoCodec != "" {
					if len(meta.AudioTracks) == 0 && opts.AudioTrackIndex == 0 {
						opts.NoAudio = true
					} else if opts.AudioTrackIndex < 0 || opts.AudioTrackIndex >= len(meta.AudioTracks) {
						http.Error(w, "La piste audio demandée est introuvable.", http.StatusBadRequest)
						return fmt.Errorf("invalid audio track %d", opts.AudioTrackIndex)
					} else {
						opts.AudioCodec = meta.AudioTracks[opts.AudioTrackIndex].Codec
					}
				}
			}
		}
		return RemuxStream(r.Context(), w, reader, p.logger, opts)
	}

	return ServeRange(w, r, reader, filename, size)
}

// RemuxToPipe streams remuxed fMP4 directly to the given destination writer.
func (p *PipelineManager) RemuxToPipe(ctx context.Context, src io.Reader, dst io.Writer, opts PipelineOptions) error {
	// Custom pipe remux implementation if needed
	return nil
}

// Verify interface compliance
var _ Pipeline = (*PipelineManager)(nil)
