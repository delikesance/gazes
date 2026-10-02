package api

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/gazes/gazes/internal/indexer"
	"github.com/gazes/gazes/internal/metadata"
	"github.com/go-chi/chi/v5"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

type catalogTransport struct{ naruto bool }

func (transport catalogTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	var input struct {
		Variables struct {
			ID int `json:"id"`
		} `json:"variables"`
	}
	json.NewDecoder(r.Body).Decode(&input)
	id := input.Variables.ID
	title := "Example"
	related := 2
	relation := "SEQUEL"
	episodes := 2
	if id == 2 {
		title = "Example Season 2"
		related = 1
		relation = "PREQUEL"
		episodes = 3
	}
	if transport.naruto {
		title = "Naruto"
		if id == 2 {
			title = "Naruto Shippuden"
		}
	}
	payload := map[string]any{"data": map[string]any{"Media": map[string]any{"id": id, "title": map[string]any{"english": title}, "format": "TV", "status": "FINISHED", "episodes": episodes, "startDate": map[string]int{"year": 2020 + id, "month": 1, "day": 1}, "relations": map[string]any{"edges": []any{map[string]any{"relationType": relation, "node": map[string]any{"id": related, "format": "TV", "title": map[string]string{"english": "Example"}}}}}}}}
	body, _ := json.Marshal(payload)
	return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(body)), Header: make(http.Header)}, nil
}

type catalogIndexer struct{}

type discoveryIndexer struct{ catalogIndexer }

func (discoveryIndexer) Search(_ context.Context, opts indexer.SearchOptions) ([]indexer.TorrentItem, error) {
	items := []indexer.TorrentItem{{InfoHash: "vf", Title: "Example S02 Complete VF", Seeders: 2}}
	if opts.Query == "Example S02E01" {
		items = append(items, indexer.TorrentItem{InfoHash: "sub", Title: "Example S02E01 VOSTFR", Seeders: 10})
	}
	return items, nil
}

func TestFullDiscoveryHasIndependentCacheAndIncludesAdditionalSources(t *testing.T) {
	s := &Server{catalogService: metadata.NewAnimeCatalogService(&http.Client{Transport: catalogTransport{}}), episodeResolver: indexer.NewEpisodeResolver(discoveryIndexer{})}
	router := chi.NewRouter()
	router.Get("/anime/{id}/seasons/{season}/episodes/{ep}/sources", s.HandleSeasonSources)
	for _, tc := range []struct {
		mode   string
		count  int
		status int
	}{{"", 1, 200}, {"full", 2, 200}, {"fast", 1, 200}, {"invalid", 0, 400}} {
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, httptest.NewRequest("GET", "/anime/1/seasons/2/episodes/1/sources?discovery="+tc.mode, nil))
		if rr.Code != tc.status {
			t.Fatalf("mode %s: %d %s", tc.mode, rr.Code, rr.Body.String())
		}
		if tc.status == 200 {
			var result indexer.EpisodeSourcesResponse
			json.Unmarshal(rr.Body.Bytes(), &result)
			if result.TotalSources != tc.count {
				t.Fatalf("mode %s: %+v", tc.mode, result)
			}
		}
	}
}

func (catalogIndexer) Name() string { return "fixture" }
func (catalogIndexer) GetLatest(context.Context, string, int) ([]indexer.TorrentItem, error) {
	return nil, nil
}
func (catalogIndexer) Search(context.Context, indexer.SearchOptions) ([]indexer.TorrentItem, error) {
	return []indexer.TorrentItem{{InfoHash: "test", Title: "Example S02E01 VOSTFR", Seeders: 5}}, nil
}
func TestSeasonEndpoints(t *testing.T) {
	s := &Server{catalogService: metadata.NewAnimeCatalogService(&http.Client{Transport: catalogTransport{}}), episodeResolver: indexer.NewEpisodeResolver(catalogIndexer{})}
	router := chi.NewRouter()
	router.Get("/anime/{id}/franchise", s.HandleFranchise)
	router.Get("/anime/{id}/seasons/{season}", s.HandleSeason)
	router.Get("/anime/{id}/seasons/{season}/episodes/{ep}/sources", s.HandleSeasonSources)
	router.Get("/anime/{id}/episodes/{ep}/sources", s.HandleEpisodeSources)
	for _, tt := range []struct {
		path   string
		status int
	}{
		{"/anime/2/franchise", 200},
		{"/anime/1/seasons/2", 200},
		{"/anime/1/seasons/999", 404},
		{"/anime/1/seasons/2/episodes/1/sources", 200},
		{"/anime/1/seasons/2/episodes/99/sources", 404},
		{"/anime/2/episodes/1/sources", 200},
		{"/anime/no/seasons/2", 400},
	} {
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, httptest.NewRequest("GET", tt.path, nil))
		if rr.Code != tt.status {
			t.Errorf("%s: %d %s", tt.path, rr.Code, rr.Body.String())
		}
		if tt.path == "/anime/1/seasons/2/episodes/1/sources" {
			var result indexer.EpisodeSourcesResponse
			json.Unmarshal(rr.Body.Bytes(), &result)
			if result.TotalSources != 1 {
				t.Fatalf("season source missing: %s", rr.Body.String())
			}
		}
	}
}

func TestRenamedSeasonEndpointUsesReleaseSeason(t *testing.T) {
	s := &Server{catalogService: metadata.NewAnimeCatalogService(&http.Client{Transport: catalogTransport{naruto: true}}), episodeResolver: indexer.NewEpisodeResolver(narutoIndexer{})}
	router := chi.NewRouter()
	router.Get("/anime/{id}/seasons/{season}/episodes/{ep}/sources", s.HandleSeasonSources)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, httptest.NewRequest("GET", "/anime/1/seasons/2/episodes/1/sources", nil))
	var result indexer.EpisodeSourcesResponse
	json.Unmarshal(rr.Body.Bytes(), &result)
	if rr.Code != 200 || len(result.Sources) != 1 || result.Sources[0].SeasonNumber != 1 {
		t.Fatalf("renamed season failed: %d %s", rr.Code, rr.Body.String())
	}
}

type narutoIndexer struct{}

func (narutoIndexer) Name() string { return "naruto" }
func (narutoIndexer) GetLatest(context.Context, string, int) ([]indexer.TorrentItem, error) {
	return nil, nil
}
func (narutoIndexer) Search(context.Context, indexer.SearchOptions) ([]indexer.TorrentItem, error) {
	return []indexer.TorrentItem{{InfoHash: "correct", Title: "Naruto Shippuden S01E01 VOSTFR", Seeders: 5}, {InfoHash: "wrong", Title: "Naruto S02E01 VF", Seeders: 100}}, nil
}
