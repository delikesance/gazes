package kv

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

type doc struct {
	N    int
	Text string
}

func newClient(t *testing.T) (*Client, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	return New(redis.NewClient(&redis.Options{Addr: mr.Addr()}), "test"), mr
}

func TestCacheFetchesOnceAcrossInstances(t *testing.T) {
	c, _ := newClient(t)
	// Two "instances" share Redis but not memory.
	a := NewCache[doc](c, "d", CacheOptions{})
	b := NewCache[doc](c, "d", CacheOptions{})
	var calls atomic.Int32
	fetch := func(context.Context) (*doc, error) {
		calls.Add(1)
		time.Sleep(150 * time.Millisecond)
		return &doc{N: 7, Text: "x"}, nil
	}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			cache := a
			if i%2 == 1 {
				cache = b
			}
			got, err := cache.Get(context.Background(), "k", Policy[doc]{TTL: time.Minute}, fetch)
			if err != nil || got.N != 7 {
				t.Errorf("got %v %v", got, err)
			}
		}(i)
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("20 concurrent callers on 2 instances must cause 1 fetch, got %d", calls.Load())
	}
	// A third instance with cold memory reads Redis, no fetch.
	cold := NewCache[doc](c, "d", CacheOptions{})
	if _, err := cold.Get(context.Background(), "k", Policy[doc]{TTL: time.Minute}, fetch); err != nil || calls.Load() != 1 {
		t.Fatalf("cold instance must hit Redis: err=%v calls=%d", err, calls.Load())
	}
}

func TestCacheServesStaleAndRefreshesInBackground(t *testing.T) {
	c, mr := newClient(t)
	ca := NewCache[doc](c, "d", CacheOptions{L1TTL: time.Millisecond})
	var version atomic.Int32
	fetch := func(context.Context) (*doc, error) { return &doc{N: int(version.Add(1))}, nil }
	pol := Policy[doc]{TTL: time.Minute}
	if got, _ := ca.Get(context.Background(), "k", pol, fetch); got.N != 1 {
		t.Fatalf("first fetch: %v", got)
	}
	mr.SetTime(time.Now().Add(2 * time.Minute)) // not enough to expire Redis (stale window), enough to be soft-stale
	time.Sleep(5 * time.Millisecond)
	// Rewrite savedAt in the past by forcing soft expiry: store with 1 ms soft TTL.
	ca.Invalidate(context.Background(), "k")
	ca.write(context.Background(), ca.c.Up("d", "k"), &doc{N: 1}, time.Millisecond)
	time.Sleep(20 * time.Millisecond)
	got, err := ca.Get(context.Background(), "k", pol, fetch)
	if err != nil || got.N != 1 {
		t.Fatalf("stale value must be returned immediately, got %v %v", got, err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if got, _ := ca.Get(context.Background(), "k", pol, fetch); got.N == 2 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("background refresh never replaced the stale value")
}

func TestCacheFetchErrorFallsBackToStale(t *testing.T) {
	c, _ := newClient(t)
	ca := NewCache[doc](c, "d", CacheOptions{L1TTL: time.Millisecond})
	ca.write(context.Background(), c.Up("d", "k"), &doc{N: 5}, time.Millisecond)
	time.Sleep(20 * time.Millisecond)
	boom := errors.New("upstream down")
	got, err := ca.Get(context.Background(), "k", Policy[doc]{TTL: time.Minute}, func(context.Context) (*doc, error) { return nil, boom })
	if err != nil || got.N != 5 {
		t.Fatalf("stale must stand in for a failing upstream: %v %v", got, err)
	}
	// With nothing stored the error surfaces.
	if _, err := ca.Get(context.Background(), "other", Policy[doc]{TTL: time.Minute}, func(context.Context) (*doc, error) { return nil, boom }); !errors.Is(err, boom) {
		t.Fatalf("want upstream error, got %v", err)
	}
}

func TestCacheTTLForZeroIsNotStored(t *testing.T) {
	c, _ := newClient(t)
	ca := NewCache[doc](c, "d", CacheOptions{})
	var calls atomic.Int32
	fetch := func(context.Context) (*doc, error) { calls.Add(1); return &doc{N: 1}, nil }
	pol := Policy[doc]{TTLFor: func(*doc) time.Duration { return 0 }}
	for i := 0; i < 2; i++ {
		if _, err := ca.Get(context.Background(), "k", pol, fetch); err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != 2 {
		t.Fatalf("a value with TTL 0 must not be cached, calls=%d", calls.Load())
	}
}

func TestCacheCompressesLargeValues(t *testing.T) {
	big := &doc{Text: string(make([]byte, 50_000))}
	raw, err := encode(big, time.Now(), time.Minute)
	if err != nil || len(raw) > 2_000 || raw[1]&flagZstd == 0 {
		t.Fatalf("large payload must be zstd-compressed: len=%d err=%v", len(raw), err)
	}
	e, ok := decode[doc](raw)
	if !ok || len(e.val.Text) != 50_000 {
		t.Fatal("round trip failed")
	}
}

func TestGovernorSharedCooldown(t *testing.T) {
	c, _ := newClient(t)
	a := c.NewGovernor("anilist", 600, 100, time.Second)
	b := c.NewGovernor("anilist", 600, 100, time.Second) // another instance
	ctx := context.Background()
	if err := a.Acquire(ctx); err != nil {
		t.Fatal(err)
	}
	a.Penalize(ctx, 30*time.Second)
	var limited *LimitedError
	if err := b.Acquire(ctx); !errors.As(err, &limited) || limited.RetryAfter < 25*time.Second {
		t.Fatalf("a 429 seen by one instance must stop the others: %v", err)
	}
}

func TestGovernorTokenBucket(t *testing.T) {
	c, _ := newClient(t)
	g := c.NewGovernor("nyaa", 60, 3, 10*time.Millisecond)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if err := g.Acquire(ctx); err != nil {
			t.Fatalf("burst token %d: %v", i, err)
		}
	}
	var limited *LimitedError
	if err := g.Acquire(ctx); !errors.As(err, &limited) {
		t.Fatalf("burst exhausted and maxWait too short: want LimitedError, got %v", err)
	}
}
