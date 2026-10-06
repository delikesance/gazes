package metadata

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gazes/gazes/internal/kv"
	"github.com/redis/go-redis/v9"
)

// fakeAniList serves a library of `total` anime (ids 1..total) through the three queries the
// mirror issues, and counts the calls by kind.
type fakeAniList struct {
	mu      sync.Mutex
	total   int
	updated map[int]int64 // id -> updatedAt
	calls   map[string]int
	title   func(id int) string
}

func (f *fakeAniList) RoundTrip(r *http.Request) (*http.Response, error) {
	var in struct {
		Query     string         `json:"query"`
		Variables map[string]any `json:"variables"`
	}
	raw, _ := io.ReadAll(r.Body)
	_ = json.Unmarshal(raw, &in)
	f.mu.Lock()
	defer f.mu.Unlock()
	num := func(k string) int { v, _ := in.Variables[k].(float64); return int(v) }
	media := func(id int) map[string]any {
		return map[string]any{"id": id, "format": "TV", "isAdult": id%2 == 0, "title": map[string]any{"romaji": f.title(id)}}
	}
	var page map[string]any
	switch {
	case strings.Contains(in.Query, "id_greater"):
		f.calls["import"]++
		var items []map[string]any
		for id := num("after") + 1; id <= f.total && len(items) < num("perPage"); id++ {
			items = append(items, media(id))
		}
		last := 0
		if len(items) > 0 {
			last = items[len(items)-1]["id"].(int)
		}
		page = map[string]any{"pageInfo": map[string]any{"hasNextPage": last < f.total}, "media": items}
	case strings.Contains(in.Query, "UPDATED_AT_DESC"):
		f.calls["updated"]++
		var items []map[string]any
		for id := f.total; id >= 1; id-- {
			items = append(items, map[string]any{"id": id, "updatedAt": f.updated[id]})
		}
		// newest first, like AniList
		for i := 0; i < len(items); i++ {
			for j := i + 1; j < len(items); j++ {
				if items[j]["updatedAt"].(int64) > items[i]["updatedAt"].(int64) {
					items[i], items[j] = items[j], items[i]
				}
			}
		}
		page = map[string]any{"pageInfo": map[string]any{"hasNextPage": false}, "media": items}
	case strings.Contains(in.Query, "id_in"):
		f.calls["detail"]++
		var items []map[string]any
		for _, v := range in.Variables["ids"].([]any) {
			items = append(items, media(int(v.(float64))))
		}
		page = map[string]any{"pageInfo": map[string]any{"hasNextPage": false}, "media": items}
	default:
		return &http.Response{StatusCode: 400, Body: io.NopCloser(strings.NewReader("unexpected query")), Request: r}, nil
	}
	body, _ := json.Marshal(map[string]any{"data": map[string]any{"Page": page}})
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(bytes.NewReader(body)), Request: r}, nil
}

func newMirrorService(t *testing.T, f *fakeAniList) (*AnimeCatalogService, *kv.Durable) {
	t.Helper()
	mr := miniredis.RunT(t)
	c := kv.New(redis.NewClient(&redis.Options{Addr: mr.Addr()}), "test")
	d, err := kv.OpenDurable(filepath.Join(t.TempDir(), "catalog.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	c.SetDurable(d)
	s := NewAnimeCatalogService(&http.Client{Transport: f})
	s.SetRedis(c)
	return s, d
}

func TestMirrorImportsEveryEntryThenStops(t *testing.T) {
	f := &fakeAniList{total: 60, updated: map[int]int64{}, calls: map[string]int{}, title: func(id int) string { return fmt.Sprintf("Anime %d", id) }}
	s, d := newMirrorService(t, f)
	ctx := context.Background()
	steps := 0
	for {
		p, err := s.MirrorStep(ctx)
		if err != nil {
			t.Fatal(err)
		}
		steps++
		if p.Complete || steps > 20 {
			break
		}
	}
	if f.calls["import"] != 3 { // 25 + 25 + 10 (a short page ends the import)
		t.Fatalf("import calls=%d steps=%d", f.calls["import"], steps)
	}
	if n := d.Count("detail:v2"); n != 60 {
		t.Fatalf("stored %d of 60", n)
	}
	// Every entry now answers from the local copy: no further AniList call.
	before := f.calls
	for _, id := range []int{1, 31, 60} {
		item, err := s.GetAnimeDetailsWithEpisodes(ctx, id)
		if err != nil || item.ID != id {
			t.Fatalf("id %d: %+v %v", id, item, err)
		}
	}
	if f.calls["detail"] != before["detail"] || f.calls["import"] != before["import"] {
		t.Fatalf("a mirrored entry must not call AniList: %v", f.calls)
	}
	if item, _ := s.GetAnimeDetailsWithEpisodes(ctx, 2); !item.IsAdult {
		t.Fatal("adult entries are stored with their flag")
	}
	// Complete and fresh: a step is idle.
	calls := f.calls["import"] + f.calls["updated"] + f.calls["detail"]
	if _, err := s.MirrorStep(ctx); err != nil || f.calls["import"]+f.calls["updated"]+f.calls["detail"] != calls {
		t.Fatalf("a finished mirror must idle until the refresh is due: err=%v calls=%v", err, f.calls)
	}
}

func TestMirrorResumesFromItsCursorAfterRestart(t *testing.T) {
	f := &fakeAniList{total: 60, updated: map[int]int64{}, calls: map[string]int{}, title: func(id int) string { return "A" }}
	s, d := newMirrorService(t, f)
	ctx := context.Background()
	if _, err := s.MirrorStep(ctx); err != nil { // ids 1..25
		t.Fatal(err)
	}
	// A new process: fresh service, same disk and Redis state.
	mr := miniredis.RunT(t)
	c := kv.New(redis.NewClient(&redis.Options{Addr: mr.Addr()}), "test")
	c.SetDurable(d)
	again := NewAnimeCatalogService(&http.Client{Transport: f})
	again.SetRedis(c)
	f.calls["import"] = 0
	for i := 0; i < 5; i++ {
		if p, err := again.MirrorStep(ctx); err != nil || p.Complete {
			if err != nil {
				t.Fatal(err)
			}
			break
		}
	}
	if f.calls["import"] != 2 { // 26..50 and 51..60 only
		t.Fatalf("resumed import made %d calls, want 2", f.calls["import"])
	}
	if n := d.Count("detail:v2"); n != 60 {
		t.Fatalf("stored %d", n)
	}
}

func TestMirrorDailyRefreshReloadsOnlyUpdatedEntries(t *testing.T) {
	f := &fakeAniList{total: 30, updated: map[int]int64{}, calls: map[string]int{}}
	f.title = func(id int) string { return "A" }
	s, _ := newMirrorService(t, f)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		if p, _ := s.MirrorStep(ctx); p.Complete {
			break
		}
	}
	// Make the refresh due, and let AniList report two entries changed since.
	s.mirrorClock = func() time.Time { return time.Now().Add(25 * time.Hour) }
	since := time.Now().Add(-time.Hour).Unix()
	for id := 1; id <= 30; id++ {
		f.updated[id] = since - 1000
	}
	f.updated[7] = time.Now().Unix()
	f.updated[19] = time.Now().Unix()
	f.title = func(id int) string {
		if id == 7 {
			return "Seven renamed"
		}
		return "A"
	}
	f.calls["detail"] = 0
	if _, err := s.MirrorStep(ctx); err != nil {
		t.Fatal(err)
	}
	if f.calls["updated"] != 1 || f.calls["detail"] != 1 {
		t.Fatalf("refresh calls: %v", f.calls)
	}
	item, err := s.GetAnimeDetailsWithEpisodes(ctx, 7)
	if err != nil || !strings.Contains(item.DisplayTitle, "Seven renamed") {
		t.Fatalf("refreshed entry not stored: %+v %v", item, err)
	}
	// Not due again for a day.
	f.calls["updated"] = 0
	if _, err := s.MirrorStep(ctx); err != nil || f.calls["updated"] != 0 {
		t.Fatalf("refresh must run once a day: %v %v", err, f.calls)
	}
}

func TestMirrorBacksOffWhenThrottled(t *testing.T) {
	mr := miniredis.RunT(t)
	c := kv.New(redis.NewClient(&redis.Options{Addr: mr.Addr()}), "test")
	d, err := kv.OpenDurable(filepath.Join(t.TempDir(), "c.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	c.SetDurable(d)
	s := NewAnimeCatalogService(&http.Client{Transport: throttledTransport{}})
	s.SetRedis(c)
	_, err = s.MirrorStep(context.Background())
	var limited *RateLimitError
	if err == nil || !asRateLimitErr(err, &limited) {
		t.Fatalf("a throttle must surface as RateLimitError so the loop can wait: %v", err)
	}
}

type throttledTransport struct{}

func (throttledTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: 429, Header: http.Header{"Retry-After": {"30"}}, Body: io.NopCloser(bytes.NewReader(nil)), Request: r}, nil
}

func asRateLimitErr(err error, target **RateLimitError) bool {
	for err != nil {
		if e, ok := err.(*RateLimitError); ok {
			*target = e
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}
