package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// searchUpstream answers a 24-result search page; every detail lookup costs a call and points to
// a further sequel, so a franchise walk per result would spend dozens of AniList calls.
func searchUpstream(calls *atomic.Int32) *http.Client {
	media := []string{
		// 10 and 11 are two seasons of one show; 12 is a movie set after 11.
		`{"id":11,"format":"TV","title":{"romaji":"Show 2nd Season"},"relations":{"edges":[{"relationType":"PREQUEL","node":{"id":10,"format":"TV"}}]}}`,
		`{"id":10,"format":"TV","title":{"romaji":"Show"},"coverImage":{"large":"show-poster"},"relations":{"edges":[{"relationType":"SEQUEL","node":{"id":11,"format":"TV"}}]}}`,
		`{"id":12,"format":"MOVIE","title":{"romaji":"Show Movie"},"relations":{"edges":[{"relationType":"PREQUEL","node":{"id":11,"format":"TV"}}]}}`,
		// 20 is related to something outside the page only.
		`{"id":20,"format":"TV","title":{"romaji":"Other"},"relations":{"edges":[{"relationType":"PREQUEL","node":{"id":999,"format":"TV"}}]}}`,
	}
	for id := 30; len(media) < 24; id++ {
		media = append(media, fmt.Sprintf(`{"id":%d,"format":"TV","title":{"romaji":"Filler %d"}}`, id, id))
	}
	page := `{"data":{"Page":{"pageInfo":{"total":24,"hasNextPage":false},"media":[` + strings.Join(media, ",") + `]}}}`
	return &http.Client{Transport: discoveryTransport(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		var body struct {
			Variables map[string]any `json:"variables"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		answer := page
		if id, ok := body.Variables["id"].(float64); ok {
			answer = fmt.Sprintf(`{"data":{"Media":{"id":%d,"format":"TV","title":{"romaji":"Detail %d"},"relations":{"edges":[{"relationType":"SEQUEL","node":{"id":%d,"format":"TV"}}]}}}}`, int(id), int(id), int(id)+1000)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(answer)), Header: make(http.Header)}, nil
	})}
}

func TestSearchSpendsOneUpstreamCall(t *testing.T) {
	var calls atomic.Int32
	service := NewAnimeCatalogService(searchUpstream(&calls))
	res, err := service.SearchCatalogFiltered(context.Background(), "show", nil, nil, 1, 24)
	if err != nil {
		t.Fatal(err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("search spent %d AniList calls, want 1", got)
	}
	if res.Partial || res.Warning != "" {
		t.Fatalf("a search answered from one page is not partial: %+v", res)
	}
	byMedia := map[int]AnimeCatalogItem{}
	for _, item := range res.Items {
		byMedia[item.MediaID] = item
	}
	// Seasons and the movie of one show collapse into one card leading to its first season,
	// which keeps the artwork of the most popular entry.
	first, ok := byMedia[11]
	if !ok || first.ID != 10 || first.MediaTitle != "Show 2nd Season" {
		t.Fatalf("related entries not grouped on the first season: %+v", res.Items[0])
	}
	if _, dup := byMedia[10]; dup {
		t.Fatal("season 1 shown twice")
	}
	if _, dup := byMedia[12]; dup {
		t.Fatal("movie of the same show shown as its own card")
	}
	if other := byMedia[20]; other.ID != 20 {
		t.Fatalf("relation outside the page must not change the card: %+v", other)
	}
	if len(res.Items) != 22 {
		t.Fatalf("want 22 cards, got %d", len(res.Items))
	}
}

func TestSearchUsesKnownFranchises(t *testing.T) {
	var calls atomic.Int32
	service := NewAnimeCatalogService(searchUpstream(&calls))
	service.franchiseC.Put(context.Background(), "20", &Franchise{ID: 7, Title: "Other Franchise", PosterImage: "franchise-poster", Complete: true}, time.Hour)
	service.franchiseC.Put(context.Background(), "10", &Franchise{ID: 10, Title: "Show Franchise", Complete: true}, time.Hour)
	res, err := service.SearchCatalogFiltered(context.Background(), "show", nil, nil, 1, 24)
	if err != nil {
		t.Fatal(err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("search spent %d AniList calls, want 1", got)
	}
	for _, item := range res.Items {
		switch item.MediaID {
		case 20:
			if item.ID != 7 || item.DisplayTitle != "Other Franchise" || item.PosterImage != "franchise-poster" {
				t.Fatalf("cached franchise ignored: %+v", item)
			}
		case 11:
			if item.ID != 10 || item.DisplayTitle != "Show Franchise" {
				t.Fatalf("group root franchise ignored: %+v", item)
			}
		}
	}
}

// Shaped on AniList's answers for "attack on titan" and "vinland": the first season has an OVA
// prequel, a film retells it (ALTERNATIVE) and a crossover special points to two unrelated series.
func TestSearchGroupsLikeFranchiseWalk(t *testing.T) {
	media := []string{
		`{"id":1,"format":"TV","startDate":{"year":2013},"title":{"romaji":"Titan"},"relations":{"edges":[{"relationType":"PREQUEL","node":{"id":2,"format":"OVA"}},{"relationType":"SEQUEL","node":{"id":3,"format":"TV"}}]}}`,
		`{"id":3,"format":"TV","startDate":{"year":2017},"title":{"romaji":"Titan Season 2"},"relations":{"edges":[{"relationType":"PREQUEL","node":{"id":1,"format":"TV"}}]}}`,
		`{"id":2,"format":"OVA","startDate":{"year":2014},"title":{"romaji":"Titan: No Regrets"},"relations":{"edges":[{"relationType":"SEQUEL","node":{"id":1,"format":"TV"}}]}}`,
		`{"id":4,"format":"MOVIE","startDate":{"year":2014},"title":{"romaji":"Titan Part I"},"relations":{"edges":[{"relationType":"ALTERNATIVE","node":{"id":1,"format":"TV"}}]}}`,
		`{"id":5,"format":"TV","startDate":{"year":2010},"title":{"romaji":"Unrelated"}}`,
		`{"id":6,"format":"SPECIAL","startDate":{"year":2020},"title":{"romaji":"Crossover"},"relations":{"edges":[{"relationType":"PARENT","node":{"id":3,"format":"TV"}},{"relationType":"PARENT","node":{"id":5,"format":"TV"}}]}}`,
		// An older parody attached to the show must not become its card (AniList's "vinland").
		`{"id":7,"format":"ONA","startDate":{"year":2009},"title":{"romaji":"Titan Parody"},"relations":{"edges":[{"relationType":"PARENT","node":{"id":1,"format":"TV"}}]}}`,
	}
	page := `{"data":{"Page":{"pageInfo":{"total":6},"media":[` + strings.Join(media, ",") + `]}}}`
	service := NewAnimeCatalogService(&http.Client{Transport: discoveryTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(page)), Header: make(http.Header)}, nil
	})})
	res, err := service.SearchCatalogFiltered(context.Background(), "titan", nil, nil, 1, 24)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, item := range res.Items {
		got = append(got, fmt.Sprintf("%d:%s", item.ID, item.DisplayTitle))
	}
	if want := []string{"1:Titan", "5:Unrelated"}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("cards %v, want %v", got, want)
	}
}

func TestPartialCatalogPagesAreKeptBriefly(t *testing.T) {
	p := catalogPolicy(time.Hour)
	if got := p.TTLFor(&CatalogResponse{}); got != time.Hour {
		t.Fatalf("complete page ttl %s", got)
	}
	if got := p.TTLFor(&CatalogResponse{Partial: true}); got != partialCatalogTTL {
		t.Fatalf("partial page ttl %s", got)
	}
}
