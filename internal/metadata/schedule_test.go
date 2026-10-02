package metadata

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

type scheduleTransport struct {
	calls atomic.Int32
	body  string
}

func (t *scheduleTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	t.calls.Add(1)
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(t.body)), Request: r}, nil
}

const scheduleFixture = `{"data":{"Page":{"pageInfo":{"hasNextPage":false},"airingSchedules":[
 {"airingAt":2000,"episode":2,"media":{"id":2,"title":{"romaji":"Second"},"coverImage":{"large":"b.jpg"},"genres":["Action"],"format":"TV","episodes":12,"isAdult":false,"countryOfOrigin":"JP","studios":{"nodes":[{"name":"Studio B"}]}}},
 {"airingAt":1000,"episode":5,"media":{"id":1,"title":{"english":"First","romaji":"Daisan"},"coverImage":{"extraLarge":"a.jpg"},"format":"TV","isAdult":false,"countryOfOrigin":"JP","studios":{"nodes":[]}}},
 {"airingAt":1500,"episode":1,"media":{"id":3,"title":{"romaji":"Adult"},"format":"TV","isAdult":true,"countryOfOrigin":"JP"}},
 {"airingAt":1600,"episode":1,"media":{"id":4,"title":{"romaji":"Donghua"},"format":"TV","isAdult":false,"countryOfOrigin":"CN"}},
 {"airingAt":1700,"episode":1,"media":{"id":5,"title":{"romaji":"Movie"},"format":"MOVIE","isAdult":false,"countryOfOrigin":"JP"}},
 {"airingAt":1800,"episode":1,"media":null}
]}}}`

func TestGetScheduleFiltersSortsAndCaches(t *testing.T) {
	transport := &scheduleTransport{body: scheduleFixture}
	svc := NewAnimeCatalogService(&http.Client{Transport: transport})
	got, err := svc.GetSchedule(context.Background(), 0, 3600)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Entries) != 2 {
		t.Fatalf("adult, non-JP, non-series and null media must be dropped: %+v", got.Entries)
	}
	first, second := got.Entries[0], got.Entries[1]
	if first.MediaID != 1 || first.Title != "First" || first.PosterImage != "a.jpg" || first.Episode != 5 {
		t.Fatalf("entries must be sorted by time with english title preferred: %+v", first)
	}
	if second.Title != "Second" || second.PosterImage != "b.jpg" || second.Studio != "Studio B" {
		t.Fatalf("fallbacks: %+v", second)
	}
	if _, err := svc.GetSchedule(context.Background(), 0, 3600); err != nil || transport.calls.Load() != 1 {
		t.Fatalf("second call must hit the cache, calls=%d", transport.calls.Load())
	}
}

func TestGetScheduleRejectsBadRanges(t *testing.T) {
	svc := NewAnimeCatalogService(&http.Client{Transport: &scheduleTransport{body: scheduleFixture}})
	for _, r := range [][2]int64{{10, 10}, {10, 5}, {0, 43 * 24 * 3600}} {
		if _, err := svc.GetSchedule(context.Background(), r[0], r[1]); err == nil {
			t.Fatalf("range %v must be rejected", r)
		}
	}
}
