package stream

import (
	"context"
	"io"
	"net/http"
)

// PipelineOptions defines configuration for the on-the-fly streaming pipeline.
type PipelineOptions struct {
	VideoCodec      string
	AudioCodec      string
	NoAudio         bool
	AudioTrackIndex int
	ForceRemux      bool
	Quality         string
	TimeOffset      float64
	InputURL        string
}

// Pipeline handles on-the-fly container remuxing (MKV -> fMP4) and HTTP range serving.
type Pipeline interface {
	// ServeHTTP serves the torrent stream via HTTP Range or piped remuxer.
	ServeHTTP(w http.ResponseWriter, r *http.Request, reader io.ReadSeekCloser, filename string, size int64, opts PipelineOptions) error

	// RemuxToPipe streams remuxed fMP4 to the destination writer.
	RemuxToPipe(ctx context.Context, src io.Reader, dst io.Writer, opts PipelineOptions) error
}
