package metadata

import (
	"context"
	"errors"
	"github.com/alicebob/miniredis/v2"
	"github.com/gazes/gazes/internal/kv"
	"github.com/redis/go-redis/v9"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
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

type limitedTransport struct {
	calls      atomic.Int32
	limitedFor int32 // first N calls answer 429
	retryAfter string
	okBody     string
}

func (t *limitedTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	n := t.calls.Add(1)
	if n <= t.limitedFor {
		return &http.Response{StatusCode: 429, Header: http.Header{"Retry-After": {t.retryAfter}}, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
	}
	return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(t.okBody)), Request: r}, nil
}

func TestScheduleRateLimitedWithoutCacheReturnsTypedError(t *testing.T) {
	transport := &limitedTransport{limitedFor: 100, retryAfter: "60", okBody: scheduleFixture}
	svc := NewAnimeCatalogService(&http.Client{Transport: transport})
	_, err := svc.GetSchedule(context.Background(), 0, 3600)
	var limited *RateLimitError
	if !errors.As(err, &limited) || limited.RetryAfter != 60*time.Second {
		t.Fatalf("want RateLimitError(60s), got %v", err)
	}
	// While blocked, AniList must not be called again.
	before := transport.calls.Load()
	if _, err := svc.GetSchedule(context.Background(), 0, 7200); !errors.As(err, &limited) || transport.calls.Load() != before {
		t.Fatalf("blocked service must not call AniList (calls %d -> %d, err %v)", before, transport.calls.Load(), err)
	}
}

func TestScheduleShortRateLimitIsRetried(t *testing.T) {
	transport := &limitedTransport{limitedFor: 1, retryAfter: "1", okBody: scheduleFixture}
	svc := NewAnimeCatalogService(&http.Client{Transport: transport})
	got, err := svc.GetSchedule(context.Background(), 0, 3600)
	if err != nil || len(got.Entries) != 2 || transport.calls.Load() != 2 {
		t.Fatalf("a 1 s pause must be waited out once: err=%v calls=%d", err, transport.calls.Load())
	}
}

func redisBacked(t *testing.T, mr *miniredis.Miniredis, transport http.RoundTripper) *AnimeCatalogService {
	t.Helper()
	svc := NewAnimeCatalogService(&http.Client{Transport: transport})
	svc.SetRedis(kv.New(redis.NewClient(&redis.Options{Addr: mr.Addr()}), "test"))
	return svc
}

func TestScheduleServesStaleWhenRefreshFails(t *testing.T) {
	mr := miniredis.RunT(t)
	svc := redisBacked(t, mr, &limitedTransport{limitedFor: 1000, retryAfter: "60", okBody: scheduleFixture})
	old := &ScheduleResponse{From: 0, To: 3600, Entries: []ScheduleEntry{{MediaID: 1, Title: "Old"}}}
	svc.scheduleC.Put(context.Background(), "0-3600", old, time.Millisecond) // already soft-stale
	time.Sleep(20 * time.Millisecond)
	got, err := svc.GetSchedule(context.Background(), 0, 3600)
	if err != nil || len(got.Entries) != 1 || got.Entries[0].Title != "Old" {
		t.Fatalf("a throttled AniList must not blank the calendar: err=%v got=%+v", err, got)
	}
}

func TestRateLimitCooldownIsSharedAcrossInstances(t *testing.T) {
	mr := miniredis.RunT(t)
	first := &limitedTransport{limitedFor: 1000, retryAfter: "60", okBody: scheduleFixture}
	second := &limitedTransport{limitedFor: 0, okBody: scheduleFixture}
	a, b := redisBacked(t, mr, first), redisBacked(t, mr, second)
	if _, err := a.GetSchedule(context.Background(), 0, 3600); err == nil {
		t.Fatal("instance A must report the throttle")
	}
	var limited *RateLimitError
	if _, err := b.GetSchedule(context.Background(), 0, 7200); !errors.As(err, &limited) {
		t.Fatalf("instance B must honour A's cooldown, got %v", err)
	}
	if second.calls.Load() != 0 {
		t.Fatalf("instance B must not call AniList during the shared cooldown, calls=%d", second.calls.Load())
	}
}

func TestScheduleIsFetchedOnceAcrossInstances(t *testing.T) {
	mr := miniredis.RunT(t)
	shared := &limitedTransport{okBody: scheduleFixture}
	a, b := redisBacked(t, mr, shared), redisBacked(t, mr, shared)
	for _, svc := range []*AnimeCatalogService{a, b, a, b} {
		if _, err := svc.GetSchedule(context.Background(), 0, 3600); err != nil {
			t.Fatal(err)
		}
	}
	if shared.calls.Load() != 1 {
		t.Fatalf("two instances must share one AniList call, calls=%d", shared.calls.Load())
	}
}
