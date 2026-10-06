package kv

import (
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestDurableSurvivesReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "catalog.sqlite")
	d, err := OpenDurable(path)
	if err != nil {
		t.Fatal(err)
	}
	d.Set("detail", "1", []byte("one"))
	d.Set("detail", "1", []byte("uno")) // replaces
	d.Set("detail", "2", []byte("two"))
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	d, err = OpenDurable(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if got, ok := d.Get("detail", "1"); !ok || string(got) != "uno" {
		t.Fatalf("got %q %v", got, ok)
	}
	if _, ok := d.Get("detail", "3"); ok {
		t.Fatal("missing key must miss")
	}
	if n := d.Count("detail"); n != 2 {
		t.Fatalf("count=%d", n)
	}
	d.Delete("detail", "1")
	if _, ok := d.Get("detail", "1"); ok {
		t.Fatal("deleted key must miss")
	}
}

func TestDurablePruneOlderThan(t *testing.T) {
	d, err := OpenDurable(filepath.Join(t.TempDir(), "c.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	d.Set("catalog", "old", []byte("x"))
	d.Set("detail", "old", []byte("x"))
	time.Sleep(5 * time.Millisecond)
	if n := d.PruneOlderThan("catalog", time.Millisecond); n != 1 {
		t.Fatalf("pruned %d", n)
	}
	if _, ok := d.Get("detail", "old"); !ok {
		t.Fatal("other domains must be left alone")
	}
}

// A Redis wipe (a deploy recreating it) must not cost the catalog: the durable copy answers, and an
// upstream outage no longer reaches the visitor for anything seen before.
func TestCacheServesFromDurableAfterRedisWipe(t *testing.T) {
	c, mr := newClient(t)
	d, err := OpenDurable(filepath.Join(t.TempDir(), "c.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	c.SetDurable(d)
	cache := NewCache[doc](c, "detail", CacheOptions{Durable: true})
	var calls atomic.Int32
	fetch := func(context.Context) (*doc, error) { calls.Add(1); return &doc{N: 1, Text: "a"}, nil }
	p := Policy[doc]{TTL: time.Hour}
	if _, err := cache.Get(context.Background(), "k", p, fetch); err != nil {
		t.Fatal(err)
	}
	mr.FlushAll()
	cold := NewCache[doc](c, "detail", CacheOptions{Durable: true})
	got, err := cold.Get(context.Background(), "k", p, fetch)
	if err != nil || got.N != 1 || calls.Load() != 1 {
		t.Fatalf("fresh durable entry must answer without upstream: %v %v calls=%d", got, err, calls.Load())
	}
	// Redis is warm again after the read-through.
	if !mr.Exists(c.Up("detail", "k")) {
		t.Fatal("durable hit must repopulate Redis")
	}
}

func TestStaleDurableBeatsUpstreamError(t *testing.T) {
	c, mr := newClient(t)
	d, err := OpenDurable(filepath.Join(t.TempDir(), "c.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	c.SetDurable(d)
	cache := NewCache[doc](c, "detail", CacheOptions{Durable: true})
	ctx := context.Background()
	cache.Put(ctx, "k", &doc{N: 9}, time.Millisecond)
	time.Sleep(10 * time.Millisecond)
	mr.FlushAll()
	cold := NewCache[doc](c, "detail", CacheOptions{Durable: true})
	boom := func(context.Context) (*doc, error) { return nil, errors.New("anilist rate limited") }
	// StaleFor 0 would refuse a stale value; the durable copy still beats the error.
	got, err := cold.Get(ctx, "k", Policy[doc]{TTL: time.Minute, StaleFor: func(*doc) time.Duration { return 0 }}, boom)
	if err != nil || got.N != 9 {
		t.Fatalf("got %v %v", got, err)
	}
}

func TestCacheWithoutDurableOptionIgnoresStore(t *testing.T) {
	c, mr := newClient(t)
	d, err := OpenDurable(filepath.Join(t.TempDir(), "c.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	c.SetDurable(d)
	cache := NewCache[doc](c, "sources", CacheOptions{})
	cache.Put(context.Background(), "k", &doc{N: 1}, time.Hour)
	if d.Count("sources") != 0 {
		t.Fatal("only caches that opt in are stored durably")
	}
	_ = mr
}

func TestInvalidateDropsDurableCopy(t *testing.T) {
	c, _ := newClient(t)
	d, err := OpenDurable(filepath.Join(t.TempDir(), "c.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	c.SetDurable(d)
	cache := NewCache[doc](c, "detail", CacheOptions{Durable: true})
	cache.Put(context.Background(), "k", &doc{N: 1}, time.Hour)
	cache.Invalidate(context.Background(), "k")
	if _, ok := d.Get("detail", "k"); ok {
		t.Fatal("invalidate must drop the durable copy")
	}
}
