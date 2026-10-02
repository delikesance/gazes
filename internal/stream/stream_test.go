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

type nopCloser struct {
	io.ReadSeeker
}

func (n *nopCloser) Close() error { return nil }

func TestServeRange(t *testing.T) {
	content := []byte("0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ")
	reader := bytes.NewReader(content)

	t.Run("Full content request (200 OK)", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/stream.mp4", nil)
		rec := httptest.NewRecorder()

		err := stream.ServeRange(rec, req, reader, "test_video.mp4", int64(len(content)))
		if err != nil {
			t.Fatalf("ServeRange failed: %v", err)
		}

		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rec.Code)
		}
		if rec.Header().Get("Accept-Ranges") != "bytes" {
			t.Errorf("expected Accept-Ranges: bytes")
		}
		if rec.Header().Get("Content-Type") != "video/mp4" {
			t.Errorf("expected Content-Type: video/mp4, got %s", rec.Header().Get("Content-Type"))
		}
	})

	t.Run("Range request (206 Partial Content)", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/stream.mp4", nil)
		req.Header.Set("Range", "bytes=0-9")
		rec := httptest.NewRecorder()

		err := stream.ServeRange(rec, req, reader, "test_video.mp4", int64(len(content)))
		if err != nil {
			t.Fatalf("ServeRange failed: %v", err)
		}

		if rec.Code != http.StatusPartialContent {
			t.Errorf("expected status 206, got %d", rec.Code)
		}
		if rec.Body.String() != "0123456789" {
			t.Errorf("expected body '0123456789', got '%s'", rec.Body.String())
		}
		if rec.Header().Get("Content-Range") != "bytes 0-9/36" {
			t.Errorf("expected Content-Range 'bytes 0-9/36', got '%s'", rec.Header().Get("Content-Range"))
		}
	})
}

func TestPipelineManager_Dispatch(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	pipeline := stream.NewPipelineManager(logger)

	content := []byte("sample mp4 data")
	reader := &nopCloser{ReadSeeker: bytes.NewReader(content)}

	req := httptest.NewRequest(http.MethodGet, "/stream", nil)
	rec := httptest.NewRecorder()

	err := pipeline.ServeHTTP(rec, req, reader, "movie.mp4", int64(len(content)), stream.PipelineOptions{})
	if err != nil {
		t.Fatalf("ServeHTTP failed: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
}
