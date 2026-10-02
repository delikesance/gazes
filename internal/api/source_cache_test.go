package api

import (
	"context"
	"errors"
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
	c := newSourceCache()
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

func TestSourceCacheTTLDependsOnQuality(t *testing.T) {
	now := time.Now()
	c := newSourceCache()
	c.now = func() time.Time { return now }
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
		c.mu.Lock()
		got := c.entries[key].expires.Sub(now)
		c.mu.Unlock()
		if got != tc.alive {
			t.Errorf("%s: ttl %v, want %v", key, got, tc.alive)
		}
	}
	// After the empty TTL a fresh resolution runs.
	now = now.Add(sourcesTTLEmpty + time.Second)
	var calls atomic.Int32
	_, hit, _ := c.resolve(context.Background(), "empty", func(context.Context) (*indexer.EpisodeSourcesResponse, error) {
		calls.Add(1)
		return sources(1, false), nil
	})
	if hit || calls.Load() != 1 {
		t.Fatalf("expired entry must be resolved again: hit=%v calls=%d", hit, calls.Load())
	}
}

func TestSourceCacheDoesNotCacheErrors(t *testing.T) {
	c := newSourceCache()
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
	c := newSourceCache()
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
	c := newSourceCache()
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
