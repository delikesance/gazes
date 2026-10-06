package metadata

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

const malFixture = `{"data":{"Page":{"media":[
 {"id":10,"idMal":100,"isAdult":false,"title":{"romaji":"Ten","english":"Ten EN"}},
 {"id":12,"idMal":120,"isAdult":true,"title":{"romaji":"Adult"}},
 {"id":10,"idMal":100,"isAdult":false,"title":{"romaji":"Ten"}}
]}}}`

func TestAniListByMalIDsMapsDedupesAndDropsAdult(t *testing.T) {
	tr := &listTransport{status: 200, body: malFixture}
	svc := NewAnimeCatalogService(&http.Client{Transport: tr})
	got, err := svc.AniListByMalIDs(context.Background(), []int{100, 120, 100}, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Entries) != 1 || got.Entries[0].ID != 10 || got.Entries[0].Title != "Ten EN" {
		t.Fatalf("entries: %+v", got.Entries)
	}
	if !strings.Contains(tr.seen, "idMal_in") {
		t.Fatalf("the MAL ids must be looked up in one batched query: %s", tr.seen)
	}
}

func TestAniListByMalIDsBatchesAndCaps(t *testing.T) {
	tr := &countTransport{body: `{"data":{"Page":{"media":[]}}}`}
	svc := NewAnimeCatalogService(&http.Client{Transport: tr})
	ids := make([]int, 130)
	for i := range ids {
		ids[i] = i + 1
	}
	got, err := svc.AniListByMalIDs(context.Background(), ids, 120)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Truncated {
		t.Fatal("more ids than the cap must be reported as truncated")
	}
	if tr.calls != 3 {
		t.Fatalf("120 ids = 3 requests of 50, got %d", tr.calls)
	}
}

func TestAniListByMalIDsIgnoresInvalidIDsWithoutARequest(t *testing.T) {
	tr := &countTransport{body: `{}`}
	svc := NewAnimeCatalogService(&http.Client{Transport: tr})
	got, err := svc.AniListByMalIDs(context.Background(), []int{0, -4}, 100)
	if err != nil || len(got.Entries) != 0 || tr.calls != 0 {
		t.Fatalf("%+v %v calls=%d", got, err, tr.calls)
	}
}

type countTransport struct {
	body  string
	calls int
}

func (t *countTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	t.calls++
	return (&listTransport{status: 200, body: t.body}).RoundTrip(r)
}
