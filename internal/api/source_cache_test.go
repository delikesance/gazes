package api

import (
	"context"
	"errors"
	"github.com/alicebob/miniredis/v2"
	"github.com/gazes/gazes/internal/kv"
	"github.com/redis/go-redis/v9"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gazes/gazes/internal/indexer"
)

func sources(n int, partial bool) *indexer.EpisodeSourcesResponse {
	return &indexer.EpisodeSourcesResponse{Partial: partial, Sources: make([]indexer.EpisodeSource, n)}
}

func TestSourceCacheServesRepeatLookups(t *testing.T) {
	c := newSourceCache(nil)
	var calls atomic.Int32
	fn := func(context.Context) (*indexer.EpisodeSourcesResponse, error) {
		calls.Add(1)
		return sources(3, false), nil
	}
	if _, hit, err := c.resolve(context.Background(), "k", fn); err != nil || hit {
		t.Fatalf("first lookup: hit=%v err=%v", hit, err)
	}
	if _, hit, err := c.resolve(context.Background(), "k", fn); err != nil || !hit {
		t.Fatalf("second lookup must be a cache hit: hit=%v err=%v", hit, err)
	}
	if calls.Load() != 1 {
		t.Fatalf("resolver ran %d times", calls.Load())
	}
}

func redisSources(t *testing.T, mr *miniredis.Miniredis) *sourceCache {
	t.Helper()
	return newSourceCache(kv.New(redis.NewClient(&redis.Options{Addr: mr.Addr()}), "test"))
}

func TestSourceCacheTTLDependsOnQuality(t *testing.T) {
	mr := miniredis.RunT(t)
	c := redisSources(t, mr)
	stale := 7 * 24 * time.Hour // Redis keeps entries past their soft TTL as a fallback
	for key, tc := range map[string]struct {
		res   *indexer.EpisodeSourcesResponse
		alive time.Duration
	}{
		"complete": {sources(2, false), sourcesTTLComplete},
		"partial":  {sources(2, true), sourcesTTLPartial},
		"empty":    {sources(0, true), sourcesTTLEmpty},
	} {
		res := tc.res
		if _, _, err := c.resolve(context.Background(), key, func(context.Context) (*indexer.EpisodeSourcesResponse, error) { return res, nil }); err != nil {
			t.Fatal(err)
		}
		ttl := mr.TTL("gz:v1:up:sources:" + key)
		low, high := time.Duration(float64(tc.alive)*0.9)+stale-time.Second, time.Duration(float64(tc.alive)*1.1)+stale+time.Second
		if ttl < low || ttl > high {
			t.Errorf("%s: redis ttl %v, want about %v (+jitter, +stale window)", key, ttl, tc.alive+stale)
		}
	}
}

func TestSourceCacheIsSharedAcrossInstances(t *testing.T) {
	mr := miniredis.RunT(t)
	a, b := redisSources(t, mr), redisSources(t, mr)
	var calls atomic.Int32
	fn := func(context.Context) (*indexer.EpisodeSourcesResponse, error) {
		calls.Add(1)
		time.Sleep(100 * time.Millisecond)
		return sources(3, false), nil
	}
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c := a
			if i%2 == 1 {
				c = b
			}
			if _, _, err := c.resolve(context.Background(), "k", fn); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("10 requests across 2 instances must resolve once, got %d", calls.Load())
	}
	if _, hit, _ := b.resolve(context.Background(), "k", fn); !hit {
		t.Fatal("a later request on the other instance must be a cache hit")
	}
}

func TestSourceCacheNeverServesEmptyAnswersStale(t *testing.T) {
	mr := miniredis.RunT(t)
	c := redisSources(t, mr)
	c.cache.Put(context.Background(), "k", sources(0, false), time.Millisecond)
	time.Sleep(20 * time.Millisecond)
	var calls atomic.Int32
	res, hit, err := c.resolve(context.Background(), "k", func(context.Context) (*indexer.EpisodeSourcesResponse, error) {
		calls.Add(1)
		return sources(2, false), nil
	})
	if err != nil || hit || calls.Load() != 1 || len(res.Sources) != 2 {
		t.Fatalf("an expired empty answer must be re-resolved, not served: hit=%v calls=%d res=%+v err=%v", hit, calls.Load(), res, err)
	}
}

func TestSourceCacheDoesNotCacheErrors(t *testing.T) {
	c := newSourceCache(nil)
	var calls atomic.Int32
	fn := func(context.Context) (*indexer.EpisodeSourcesResponse, error) {
		if calls.Add(1) == 1 {
			return nil, errors.New("boom")
		}
		return sources(1, false), nil
	}
	if _, _, err := c.resolve(context.Background(), "k", fn); err == nil {
		t.Fatal("first call must fail")
	}
	if _, _, err := c.resolve(context.Background(), "k", fn); err != nil || calls.Load() != 2 {
		t.Fatalf("an error must not be remembered: err=%v calls=%d", err, calls.Load())
	}
}

func TestSourceCacheSharesConcurrentResolutions(t *testing.T) {
	c := newSourceCache(nil)
	var calls atomic.Int32
	release := make(chan struct{})
	fn := func(context.Context) (*indexer.EpisodeSourcesResponse, error) {
		calls.Add(1)
		<-release
		return sources(2, false), nil
	}
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, _, err := c.resolve(context.Background(), "k", fn); err != nil {
				t.Error(err)
			}
		}()
	}
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("5 simultaneous requests ran the resolver %d times", calls.Load())
	}
}

func TestSourceCacheCallerCancellationDoesNotFailOthers(t *testing.T) {
	c := newSourceCache(nil)
	release := make(chan struct{})
	fn := func(ctx context.Context) (*indexer.EpisodeSourcesResponse, error) {
		<-release
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return sources(1, false), nil
	}
	first, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, _, err := c.resolve(first, "k", fn); done <- err }()
	time.Sleep(20 * time.Millisecond)
	second := make(chan error, 1)
	go func() { _, _, err := c.resolve(context.Background(), "k", fn); second <- err }()
	time.Sleep(20 * time.Millisecond)
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("the cancelled caller must return its own cancellation, got %v", err)
	}
	close(release)
	if err := <-second; err != nil {
		t.Fatalf("the other caller must still get the result: %v", err)
	}
}
