package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/gazes/gazes/internal/config"
	"github.com/gazes/gazes/internal/indexer"
	"github.com/gazes/gazes/internal/library"
	"github.com/gazes/gazes/internal/metadata"
	"github.com/gazes/gazes/internal/torrent"
)

const libHash = "0123456789abcdef0123456789abcdef01234567"

// libEngine is a torrent engine with one loaded torrent: file 0 is a video, file 1 a subtitle.
type libEngine struct{}

type nopRSC struct{ io.ReadSeeker }

func (nopRSC) Close() error { return nil }

func (libEngine) AddTorrent(context.Context, string) (string, []torrent.FileInfo, error) {
	panic("the library must never add torrents")
}
func (libEngine) GetFileStream(context.Context, string, int) (io.ReadSeekCloser, *torrent.FileInfo, error) {
	return nopRSC{strings.NewReader("torrent")}, &torrent.FileInfo{Path: "t.mkv", Length: 7}, nil
}
func (libEngine) GetStats(string) (*torrent.SwarmStats, error) { return &torrent.SwarmStats{}, nil }
func (libEngine) Close() error                                 { return nil }
func (libEngine) Files(h string) ([]torrent.FileInfo, bool) {
	if h != libHash {
		return nil, false
	}
	return []torrent.FileInfo{{Index: 0, Path: "a.mkv", Length: 10, IsVideo: true}, {Index: 1, Path: "a.srt", Length: 1}}, true
}

type libFetcher struct{ size int64 }

func (libFetcher) GetFileStream(context.Context, string, int) (io.ReadSeekCloser, *torrent.FileInfo, error) {
	return nil, nil, io.EOF
}
func (libFetcher) DownloadFile(string, int) error                   { return nil }
func (libFetcher) ReleaseFile(string, int)                          {}
func (f libFetcher) FileProgress(string, int) (int64, int64, error) { return 0, f.size, nil }
func (libFetcher) VerifyFile(context.Context, string, int) error    { return nil }

// episodesTransport answers every AniList detail query with a 12-episode finished series.
type episodesTransport struct{}

func (episodesTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	var in struct {
		Variables struct {
			ID int `json:"id"`
		} `json:"variables"`
	}
	json.NewDecoder(r.Body).Decode(&in)
	media := map[string]any{"id": in.Variables.ID, "title": map[string]any{"english": "Example"}, "format": "TV", "status": "FINISHED", "episodes": 12,
		"startDate": map[string]int{"year": 2020, "month": 1, "day": 1}}
	body, _ := json.Marshal(map[string]any{"data": map[string]any{"Media": media}})
	return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(body)), Header: make(http.Header)}, nil
}

type libEnv struct {
	s       *Server
	pool    string
	disk    string
	handler http.Handler
}

// newLibEnv opens a real library on temp dirs, optionally pre-filled by seed through the index.
func newLibEnv(t *testing.T, loggedIn bool, seed func(st *library.Store, diskID, diskDir string)) *libEnv {
	t.Helper()
	return newSizedLibEnv(t, loggedIn, 10, seed)
}

const (
	vostfrHash   = "fedcba9876543210fedcba9876543210fedcba98"
	unloadedHash = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
)

// cacheSources plants the findings HandleSeasonSources would have cached for (season, ep).
func (e *libEnv) cacheSources(season, ep int, srcs ...indexer.EpisodeSource) {
	e.s.sources().cache.Put(context.Background(), fmt.Sprintf("discovery-v3|%d|%d", season, ep),
		&indexer.EpisodeSourcesResponse{Sources: srcs}, time.Hour)
}

func libSource(hash string, tag indexer.LanguageTag) indexer.EpisodeSource {
	return indexer.EpisodeSource{TorrentItem: indexer.TorrentItem{InfoHash: hash}, LanguageTag: tag}
}

func newSizedLibEnv(t *testing.T, loggedIn bool, size int64, seed func(st *library.Store, diskID, diskDir string)) *libEnv {
	t.Helper()
	pool, index := t.TempDir(), t.TempDir()
	diskDir := filepath.Join(pool, "d1")
	if err := os.Mkdir(diskDir, 0o755); err != nil {
		t.Fatal(err)
	}
	diskID, err := library.EnsureMarker(diskDir)
	if err != nil {
		t.Fatal(err)
	}
	if seed != nil {
		st, err := library.OpenStore(index)
		if err != nil {
			t.Fatal(err)
		}
		seed(st, diskID, diskDir)
		st.Close()
	}
	svc, err := library.Open(library.Options{PoolDir: pool, IndexDir: index, ReservePercent: 0, ReserveBytes: 0, Stall: time.Hour}, libEngine{}, libFetcher{size: size}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { svc.Close() })
	s := NewServer(&config.Config{}, nil, nil, svc.Engine(), nil, WithLibrary(svc),
		WithLibraryUser(func(*http.Request) (int64, bool) { return 1, loggedIn }))
	s.catalogService = metadata.NewAnimeCatalogService(&http.Client{Transport: episodesTransport{}})
	env := &libEnv{s: s, pool: pool, disk: diskID, handler: s.Router()}
	for ep := 1; ep <= 12; ep++ {
		env.cacheSources(5, ep, libSource(libHash, indexer.LangVF), libSource(strings.ToUpper(unloadedHash), indexer.LangMULTI), libSource(vostfrHash, indexer.LangVOSTFR))
	}
	return env
}

func (e *libEnv) do(method, url, body string, hdr ...string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, url, strings.NewReader(body))
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	rr := httptest.NewRecorder()
	e.handler.ServeHTTP(rr, req)
	return rr
}

func libBody(hash string, idx int) string {
	b, _ := json.Marshal(map[string]any{"info_hash": hash, "file_index": idx, "release_name": "R", "anime_id": 5, "title": "T"})
	return string(b)
}

func TestLibraryPostRequiresLogin(t *testing.T) {
	env := newLibEnv(t, false, nil)
	if rr := env.do("POST", "/api/v1/library/episodes/5/1/vf", libBody(libHash, 0)); rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rr.Code)
	}
}

func TestLibraryPostValidatesLangEpisodeAndFile(t *testing.T) {
	env := newLibEnv(t, true, nil)
	cases := []struct{ name, url, body string }{
		{"bad lang", "/api/v1/library/episodes/5/1/en", libBody(libHash, 0)},
		{"episode not in season", "/api/v1/library/episodes/5/99/vf", libBody(libHash, 0)},
		{"unknown torrent", "/api/v1/library/episodes/5/1/vf", libBody(unloadedHash, 0)},
		{"hash not in resolved sources", "/api/v1/library/episodes/5/1/vf", libBody(strings.Repeat("c", 40), 0)},
		{"vostfr source registered as vf", "/api/v1/library/episodes/5/1/vf", libBody(vostfrHash, 0)},
		{"vf source registered as vostfr", "/api/v1/library/episodes/5/1/vostfr", libBody(libHash, 0)},
		{"anime id missing", "/api/v1/library/episodes/5/1/vf", `{"info_hash":"` + libHash + `","file_index":0}`},
		{"not a video", "/api/v1/library/episodes/5/1/vf", libBody(libHash, 1)},
		{"file out of range", "/api/v1/library/episodes/5/1/vf", libBody(libHash, 7)},
		{"bad hash", "/api/v1/library/episodes/5/1/vf", libBody("zz", 0)},
	}
	for _, c := range cases {
		if rr := env.do("POST", c.url, c.body); rr.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s: status = %d body %s", c.name, rr.Code, rr.Body)
		}
	}
	if rr := env.do("POST", "/api/v1/library/episodes/5/1/vf", strings.Repeat("x", 5000)); rr.Code != http.StatusBadRequest {
		t.Errorf("oversized body: status = %d", rr.Code)
	}
}

func TestLibraryPostCreatesThenTouches(t *testing.T) {
	env := newLibEnv(t, true, nil)
	rr := env.do("POST", "/api/v1/library/episodes/5/1/vf", libBody(libHash, 0))
	if rr.Code != http.StatusCreated || !strings.Contains(rr.Body.String(), `"created":true`) || !strings.Contains(rr.Body.String(), `"state":"DOWNLOADING"`) {
		t.Fatalf("first: %d %s", rr.Code, rr.Body)
	}
	rr = env.do("POST", "/api/v1/library/episodes/5/1/vf", libBody(libHash, 0))
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"created":false`) {
		t.Fatalf("second: %d %s", rr.Code, rr.Body)
	}
}

func seedCopy(t *testing.T, ep int, state library.State, content string) func(*library.Store, string, string) {
	return func(st *library.Store, diskID, diskDir string) {
		rel := filepath.Join("5", "1-vf.mkv")
		if content != "" {
			os.MkdirAll(filepath.Join(diskDir, "5"), 0o755)
			if err := os.WriteFile(filepath.Join(diskDir, rel), []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		now := time.Now()
		err := st.Create(library.Entry{Key: library.Key{SeasonID: 5, Episode: ep, Lang: "vf"}, State: state, DiskID: diskID, RelPath: "5/1-vf.mkv",
			VideoCodec: "av1", SizeBytes: int64(len(content)), DurationMS: 1420000, AudioTracks: 2, SubtitleTracks: 3, CreatedAt: now, UpdatedAt: now, LastAccessAt: now})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestLibraryGetListsReadyCopiesOnly(t *testing.T) {
	env := newLibEnv(t, true, func(st *library.Store, d, dir string) {
		seedCopy(t, 2, library.StateDownloading, "")(st, d, dir)
		seedCopy(t, 1, library.StateAV1, "0123456789")(st, d, dir)
	})
	rr := env.do("GET", "/api/v1/library/episodes/5/2", "")
	if rr.Code != 200 || strings.TrimSpace(rr.Body.String()) != `{"copies":[]}` {
		t.Fatalf("downloading episode: %d %s", rr.Code, rr.Body)
	}
	rr = env.do("GET", "/api/v1/library/episodes/5/1", "")
	var out struct {
		Copies []map[string]any `json:"copies"`
	}
	if rr.Code != 200 || json.Unmarshal(rr.Body.Bytes(), &out) != nil || len(out.Copies) != 1 {
		t.Fatalf("ready episode: %d %s", rr.Code, rr.Body)
	}
	c := out.Copies[0]
	if c["lang"] != "vf" || c["state"] != "AV1" || c["video_codec"] != "av1" || c["duration_ms"] != float64(1420000) ||
		c["audio_tracks"] != float64(2) || c["subtitle_tracks"] != float64(3) || !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(c["stream_id"].(string)) {
		t.Fatalf("copy: %v", c)
	}
}

func TestStreamRawServesLibraryCopy(t *testing.T) {
	env := newLibEnv(t, true, seedCopy(t, 1, library.StateAV1, "0123456789"))
	rr := env.do("GET", "/api/v1/library/episodes/5/1", "")
	var out struct {
		Copies []struct {
			StreamID string `json:"stream_id"`
		} `json:"copies"`
	}
	if json.Unmarshal(rr.Body.Bytes(), &out) != nil || len(out.Copies) != 1 {
		t.Fatalf("list: %s", rr.Body)
	}
	rr = env.do("GET", "/api/v1/stream/raw?ih="+out.Copies[0].StreamID+"&file_idx=0", "", "Range", "bytes=2-5")
	if rr.Code != http.StatusPartialContent || rr.Body.String() != "2345" {
		t.Fatalf("range: %d %q", rr.Code, rr.Body.String())
	}
	// Plain torrent hashes still go to the torrent engine.
	rr = env.do("GET", "/api/v1/stream/raw?ih="+libHash+"&file_idx=0", "", "Range", "bytes=0-2")
	if rr.Code != http.StatusPartialContent || rr.Body.String() != "tor" {
		t.Fatalf("torrent: %d %q", rr.Code, rr.Body.String())
	}
}

func TestLibraryPostRequiresResolvedSource(t *testing.T) {
	env := newLibEnv(t, true, nil)
	rr := env.do("POST", "/api/v1/library/episodes/5/1/vf", libBody(strings.Repeat("c", 40), 0))
	if rr.Code != http.StatusUnprocessableEntity || !strings.Contains(rr.Body.String(), "source_not_resolved") {
		t.Fatalf("hash not in sources: %d %s", rr.Code, rr.Body)
	}
	// No cached findings at all: the handler never triggers a new resolve.
	env.s.sources().cache.Invalidate(context.Background(), "discovery-v3|5|3")
	rr = env.do("POST", "/api/v1/library/episodes/5/3/vf", libBody(libHash, 0))
	if rr.Code != http.StatusUnprocessableEntity || !strings.Contains(rr.Body.String(), "source_not_resolved") {
		t.Fatalf("cache miss: %d %s", rr.Code, rr.Body)
	}
	// MULTI maps to vf, and a hash matches regardless of case.
	env.cacheSources(5, 4, libSource(strings.ToUpper(libHash), indexer.LangMULTI))
	if rr = env.do("POST", "/api/v1/library/episodes/5/4/vf", libBody(libHash, 0)); rr.Code != http.StatusCreated {
		t.Fatalf("multi source: %d %s", rr.Code, rr.Body)
	}
	// VOSTFR sources register as vostfr.
	env.cacheSources(5, 5, libSource(libHash, indexer.LangVOSTFR))
	if rr = env.do("POST", "/api/v1/library/episodes/5/5/vostfr", libBody(libHash, 0)); rr.Code != http.StatusCreated {
		t.Fatalf("vostfr source: %d %s", rr.Code, rr.Body)
	}
}

func TestLibraryPostTooManyDownloadsIs429(t *testing.T) {
	env := newLibEnv(t, true, nil)
	for ep := 1; ep <= 4; ep++ {
		if rr := env.do("POST", fmt.Sprintf("/api/v1/library/episodes/5/%d/vf", ep), libBody(libHash, 0)); rr.Code != http.StatusCreated {
			t.Fatalf("episode %d: %d %s", ep, rr.Code, rr.Body)
		}
	}
	if rr := env.do("POST", "/api/v1/library/episodes/5/5/vf", libBody(libHash, 0)); rr.Code != http.StatusTooManyRequests {
		t.Fatalf("fifth download: %d %s", rr.Code, rr.Body)
	}
}

func TestLibraryPostNoSpaceIs507(t *testing.T) {
	env := newSizedLibEnv(t, true, 1<<60, nil)
	if rr := env.do("POST", "/api/v1/library/episodes/5/1/vf", libBody(libHash, 0)); rr.Code != http.StatusInsufficientStorage {
		t.Fatalf("status = %d %s", rr.Code, rr.Body)
	}
}

type failingTransport struct{}

func (failingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, io.ErrUnexpectedEOF
}

func TestLibraryPostCatalogErrorIs503(t *testing.T) {
	env := newLibEnv(t, true, nil)
	env.s.catalogService = metadata.NewAnimeCatalogService(&http.Client{Transport: failingTransport{}})
	if rr := env.do("POST", "/api/v1/library/episodes/5/1/vf", libBody(libHash, 0)); rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d %s", rr.Code, rr.Body)
	}
}
