package metadata

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type discoveryTransport func(*http.Request) (*http.Response, error)

func (f discoveryTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestDiscoveryPreservesFeaturedSeasonAfterCanonicalization(t *testing.T) {
	for _, complete := range []bool{true, false} {
		t.Run(map[bool]string{true: "complete", false: "partial"}[complete], func(t *testing.T) {
			service := &AnimeCatalogService{
				httpClient: &http.Client{Transport: discoveryTransport(func(*http.Request) (*http.Response, error) {
					return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"data":{"Page":{"pageInfo":{"hasNextPage":false},"media":[{"id":2,"title":{"romaji":"Example 2nd Season"},"coverImage":{"large":"season-poster"},"bannerImage":"season-banner"}]}}}`)), Header: make(http.Header)}, nil
				})},
				franchiseCache: map[int]cachedFranchise{2: {data: &Franchise{ID: 1, Title: "Example", PosterImage: "franchise-poster", Complete: complete}, expiresAt: time.Now().Add(time.Hour)}},
			}
			result, err := service.doGraphQLPageQuery(context.Background(), trendingQuery, map[string]interface{}{"page": 1, "perPage": 24})
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Items) != 1 {
				t.Fatalf("unexpected catalog: %+v", result)
			}
			item := result.Items[0]
			if item.MediaID != 2 || item.MediaPosterImage != "season-poster" || item.BannerImage != "season-banner" || item.TitleRomaji != "Example 2nd Season" {
				t.Fatalf("featured season lost: %+v", item)
			}
			if complete && (item.ID != 1 || item.PosterImage != "franchise-poster") {
				t.Fatalf("franchise navigation lost: %+v", item)
			}
			if !complete && (item.ID != 2 || !result.Partial) {
				t.Fatalf("partial result lost: %+v", result)
			}
		})
	}
}

func TestCatalogSeasonBoundaries(t *testing.T) {
	for _, tt := range []struct {
		date, season string
		year         int
	}{
		{"2026-01-01", "WINTER", 2026}, {"2026-03-31", "WINTER", 2026},
		{"2026-04-01", "SPRING", 2026}, {"2026-07-01", "SUMMER", 2026},
		{"2026-10-01", "FALL", 2026}, {"2027-01-01", "WINTER", 2027},
	} {
		now, _ := time.Parse("2006-01-02", tt.date)
		season, year := catalogSeason(now)
		if season != tt.season || year != tt.year {
			t.Errorf("%s: %s %d", tt.date, season, year)
		}
	}
}

func TestSeasonalCatalogKeepsDistinctSeasonsOfOneFranchise(t *testing.T) {
	service := NewAnimeCatalogService(&http.Client{Transport: discoveryTransport(func(r *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "seasonYear") || !strings.Contains(string(body), "MediaSeason") {
			t.Error("seasonal query must filter the release season and year")
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"data":{"Page":{"media":[{"id":2,"title":{"romaji":"Example Season 2"}},{"id":3,"title":{"romaji":"Example Season 3"}}]}}}`)), Header: make(http.Header)}, nil
	})})
	for _, id := range []int{2, 3} {
		service.franchiseCache[id] = cachedFranchise{data: &Franchise{ID: 1, Title: "Example", Complete: true}, expiresAt: time.Now().Add(time.Hour)}
	}
	result, err := service.GetCurrentSeason(context.Background(), 1, 24)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 2 || result.Items[0].MediaID != 2 || result.Items[1].MediaID != 3 {
		t.Fatalf("distinct seasons collapsed: %+v", result)
	}
	if result.Items[0].ID != 1 || result.Items[1].ID != 1 || result.Items[0].MediaTitle != "Example Season 2" {
		t.Fatalf("season navigation lost: %+v", result.Items)
	}
	season, year := catalogSeason(time.Now().UTC())
	if result.Season != season || result.SeasonYear != year {
		t.Fatal("missing seasonal identity")
	}
}

func TestAvailableEpisodesDistinguishesReleasedFromPlanned(t *testing.T) {
	for _, tt := range []struct {
		status            string
		total, next, want int
		known             bool
	}{
		{"NOT_YET_RELEASED", 12, 1, 0, true},
		{"RELEASING", 12, 4, 3, true},
		{"RELEASING", 12, 0, 0, false},
		{"FINISHED", 12, 0, 12, true},
		{"FINISHED", 0, 0, 0, false},
		{"RELEASING", 12, 20, 12, true},
	} {
		m := aniListMediaItem{Status: tt.status, Episodes: tt.total}
		if tt.next > 0 {
			m.NextAiringEpisode = &struct {
				Episode int `json:"episode"`
			}{Episode: tt.next}
		}
		got := availableEpisodeCount(&m)
		if (got != nil) != tt.known || (got != nil && *got != tt.want) {
			t.Errorf("%+v: got %v", tt, got)
		}
	}
}

func TestSeasonalReleaseOrdering(t *testing.T) {
	items := []AnimeCatalogItem{
		{ID: 1, StartDate: "0000-00-00"}, {ID: 2, StartDate: "2026-11-20"},
		{ID: 3, StartDate: "2026-10-03"}, {ID: 4, StartDate: "2026-10-00"},
		{ID: 5, StartDate: "2026-10-03"}, {ID: 6, StartDate: "2026-02-30"},
	}
	sortSeasonalReleases(items)
	want := []int{3, 5, 2, 1, 4, 6}
	for i, item := range items {
		if item.ID != want[i] {
			t.Fatalf("index %d: got %d, want %d", i, item.ID, want[i])
		}
	}
}
