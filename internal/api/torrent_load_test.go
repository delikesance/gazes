package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/gazes/gazes/internal/torrent"
	"io"
	"net/http/httptest"
	"testing"
)

type packEngine struct{ streams int }

func (e *packEngine) AddTorrent(context.Context, string) (string, []torrent.FileInfo, error) {
	files := make([]torrent.FileInfo, 500)
	for i := range files {
		files[i] = torrent.FileInfo{Index: i, Path: fmt.Sprintf("Naruto_Shippuuden_%03d.mkv", i+1), Length: int64(i + 100), IsVideo: true}
	}
	return "pack", files, nil
}
func (e *packEngine) GetFileStream(context.Context, string, int) (io.ReadSeekCloser, *torrent.FileInfo, error) {
	e.streams++
	panic("metadata-only loading must not open video data")
}
func (e *packEngine) GetStats(string) (*torrent.SwarmStats, error) { return nil, nil }
func (e *packEngine) Close() error                                 { return nil }
func TestMetadataOnlyLoadDoesNotProbeLargestEpisode(t *testing.T) {
	engine := &packEngine{}
	server := &Server{torrentEngine: engine}
	rr := httptest.NewRecorder()
	server.HandleLoadTorrent(rr, httptest.NewRequest("POST", "/torrent/load", bytes.NewBufferString(`{"magnet":"magnet:?xt=urn:btih:test","metadata_only":true}`)))
	var response LoadTorrentResponse
	if rr.Code != 200 || json.Unmarshal(rr.Body.Bytes(), &response) != nil || len(response.Files) != 500 {
		t.Fatalf("%d %s", rr.Code, rr.Body.String())
	}
	if engine.streams != 0 {
		t.Fatal("unrelated video data requested")
	}
}
