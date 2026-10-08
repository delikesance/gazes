package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gazes/gazes/internal/api"
	"github.com/gazes/gazes/internal/config"
	"github.com/gazes/gazes/internal/indexer"
	"github.com/gazes/gazes/internal/stream"
	"github.com/gazes/gazes/internal/torrent"
)

type mockIndexer struct{}

func (m *mockIndexer) Name() string { return "mock" }
func (m *mockIndexer) Search(ctx context.Context, opts indexer.SearchOptions) ([]indexer.TorrentItem, error) {
	return []indexer.TorrentItem{
		{
			ID:          "123",
			Title:       "Test Anime Episode 1",
			InfoHash:    "0123456789abcdef0123456789abcdef01234567",
			MagnetURI:   "magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567",
			SizeBytes:   1073741824,
			SizeDisplay: "1.0 GiB",
			Seeders:     100,
			Leechers:    10,
			PublishDate: time.Now(),
		},
	}, nil
}

func (m *mockIndexer) GetLatest(ctx context.Context, category string, page int) ([]indexer.TorrentItem, error) {
	return m.Search(ctx, indexer.SearchOptions{Category: category, Page: page})
}

type nopCloser struct {
	io.ReadSeeker
}

func (n *nopCloser) Close() error { return nil }

type mockTorrentEngine struct{}

func (m *mockTorrentEngine) AddTorrent(ctx context.Context, magnetURI string) (string, []torrent.FileInfo, error) {
	return "0123456789abcdef0123456789abcdef01234567", []torrent.FileInfo{
		{Index: 0, Path: "Episode_01.mp4", Length: 1073741824, IsVideo: true, MimeType: "video/mp4"},
	}, nil
}

func (m *mockTorrentEngine) GetFileStream(ctx context.Context, infoHash string, fileIndex int) (io.ReadSeekCloser, *torrent.FileInfo, error) {
	reader := &nopCloser{ReadSeeker: bytes.NewReader([]byte("sample-video-stream-bytes"))}
	info := &torrent.FileInfo{
		Index:    0,
		Path:     "Episode_01.mp4",
		Length:   25,
		IsVideo:  true,
		MimeType: "video/mp4",
	}
	return reader, info, nil
}

func (m *mockTorrentEngine) GetStats(infoHash string) (*torrent.SwarmStats, error) {
	return &torrent.SwarmStats{
		InfoHash:      infoHash,
		Title:         "Episode_01.mp4",
		TotalBytes:    1073741824,
		ProgressPct:   45.5,
		ActiveSeeders: 25,
		DownloadRate:  1048576,
	}, nil
}

func (m *mockTorrentEngine) Close() error {
	return nil
}

func TestAPIRoutes(t *testing.T) {
	cfg := config.Load()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	pipeline := stream.NewPipelineManager(logger)
	srv := api.NewServer(cfg, logger, &mockIndexer{}, &mockTorrentEngine{}, pipeline)
	handler := srv.Router()

	t.Run("GET /healthz", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rec.Code)
		}

		var body map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("failed to decode json body: %v", err)
		}

		if body["status"] != "ok" {
			t.Errorf("expected status 'ok', got '%s'", body["status"])
		}
	})

	t.Run("GET /api/v1/health", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rec.Code)
		}
	})

	t.Run("GET /api/v1/search", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=test", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rec.Code)
		}
	})

	t.Run("POST /api/v1/torrent/load", func(t *testing.T) {
		reqBody := `{"magnet": "magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/torrent/load", bytes.NewBufferString(reqBody))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
		}

		var resp api.LoadTorrentResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode json: %v", err)
		}

		if resp.InfoHash != "0123456789abcdef0123456789abcdef01234567" {
			t.Errorf("unexpected infohash: %s", resp.InfoHash)
		}
	})

	t.Run("GET /api/v1/torrent/stats", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/torrent/stats?ih=0123456789abcdef0123456789abcdef01234567", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
		}

		var stats torrent.SwarmStats
		if err := json.Unmarshal(rec.Body.Bytes(), &stats); err != nil {
			t.Fatalf("failed to decode stats: %v", err)
		}

		if stats.ActiveSeeders != 25 {
			t.Errorf("expected 25 active seeders, got %d", stats.ActiveSeeders)
		}
	})

	t.Run("GET /api/v1/metadata", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/metadata?ih=0123456789abcdef0123456789abcdef01234567&file_idx=0", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("GET /api/v1/stream", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/stream?ih=0123456789abcdef0123456789abcdef01234567&file_idx=0", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rec.Code)
		}
		if rec.Header().Get("Content-Type") != "video/mp4" {
			t.Errorf("expected Content-Type: video/mp4, got %s", rec.Header().Get("Content-Type"))
		}
	})
}

func TestPlaybackConfigReportsEngine(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	for engine, want := range map[string]string{"": "legacy", "legacy": "legacy", "hls": "hls", "bogus": "legacy"} {
		srv := api.NewServer(&config.Config{PlaybackEngine: engine, CacheDir: t.TempDir()}, logger, &mockIndexer{}, &mockTorrentEngine{}, stream.NewPipelineManager(logger))
		rec := httptest.NewRecorder()
		srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/playback/config", nil))
		var body struct{ Engine string }
		if err := json.NewDecoder(rec.Body).Decode(&body); err != nil || body.Engine != want {
			t.Errorf("PlaybackEngine=%q: got %q (err %v), want %q", engine, body.Engine, err, want)
		}
	}
}
