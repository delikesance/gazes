package stream_test

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gazes/gazes/internal/stream"
)

func BenchmarkServeRange_SmallChunk(b *testing.B) {
	// Simulate 100MB video in memory
	data := make([]byte, 100*1024*1024)
	reader := bytes.NewReader(data)

	req := httptest.NewRequest(http.MethodGet, "/stream.mp4", nil)
	req.Header.Set("Range", "bytes=0-1048575") // 1MB chunk

	b.ResetTimer()
	b.SetBytes(1024 * 1024)

	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		_ = stream.ServeRange(rec, req, reader, "test.mp4", int64(len(data)))
	}
}

func BenchmarkServeRange_Parallel(b *testing.B) {
	data := make([]byte, 50*1024*1024)

	b.ResetTimer()
	b.SetBytes(512 * 1024)

	b.RunParallel(func(pb *testing.PB) {
		reader := bytes.NewReader(data)
		req := httptest.NewRequest(http.MethodGet, "/stream.mp4", nil)
		req.Header.Set("Range", "bytes=1000000-1524287") // 512KB chunk

		for pb.Next() {
			rec := httptest.NewRecorder()
			_ = stream.ServeRange(rec, req, reader, "test.mp4", int64(len(data)))
		}
	})
}

func TestPipelineManager_RemuxOptions(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	pipeline := stream.NewPipelineManager(logger)

	data := []byte("dummy mkv container payload")
	reader := &nopCloser{ReadSeeker: bytes.NewReader(data)}

	// Verify dispatch for MKV routes to Remux pipeline
	req := httptest.NewRequest(http.MethodGet, "/stream", nil)
	rec := httptest.NewRecorder()

	// Since ffmpeg will exit on invalid container, we expect ServeHTTP to invoke RemuxStream
	_ = pipeline.ServeHTTP(rec, req, reader, "sample.mkv", int64(len(data)), stream.PipelineOptions{ForceRemux: true})

	// Response header content type should be video/mp4 for fMP4 output
	if rec.Header().Get("Content-Type") != "video/mp4" {
		t.Errorf("expected Content-Type video/mp4, got %s", rec.Header().Get("Content-Type"))
	}
}

func TestMP4PlaybackOptionsRequireRemux(t *testing.T) {
	for _, tc := range []struct {
		name string
		opts stream.PipelineOptions
	}{
		{"seek", stream.PipelineOptions{TimeOffset: 12.5, VideoCodec: "h264"}},
		{"audio", stream.PipelineOptions{AudioTrackIndex: 1, VideoCodec: "h264"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pipeline := stream.NewPipelineManager(slog.New(slog.NewTextHandler(io.Discard, nil)))
			data := []byte("invalid mp4 fixture")
			rec := httptest.NewRecorder()
			_ = pipeline.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/stream", nil), &nopCloser{ReadSeeker: bytes.NewReader(data)}, "sample.mp4", int64(len(data)), tc.opts)
			if got := rec.Header().Get("Cache-Control"); got != "no-cache, no-store, must-revalidate" {
				t.Fatalf("playback options must dispatch to remux, Cache-Control = %q", got)
			}
		})
	}
}
