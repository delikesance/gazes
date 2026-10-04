package indexer

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gazes/gazes/internal/kv"
	"github.com/redis/go-redis/v9"
)

type countingProvider struct {
	calls atomic.Int32
	fail  atomic.Bool
}

func (p *countingProvider) Name() string { return "fake" }
func (p *countingProvider) Search(ctx context.Context, o SearchOptions) ([]TorrentItem, error) {
	p.calls.Add(1)
	time.Sleep(80 * time.Millisecond)
	if p.fail.Load() {
		return nil, &HTTPError{Provider: "fake", Status: 429}
	}
	return []TorrentItem{{InfoHash: "aa", Title: "t", Seeders: 5}}, nil
}
func (p *countingProvider) GetLatest(ctx context.Context, category string, page int) ([]TorrentItem, error) {
	return p.Search(ctx, SearchOptions{})
}

func sharedMulti(t *testing.T, mr *miniredis.Miniredis, p Provider) *MultiProvider {
	t.Helper()
	m := NewMultiProvider(p)
	m.SetRedis(kv.New(redis.NewClient(&redis.Options{Addr: mr.Addr()}), "test"))
	return m
}

func TestIndexerQueriesAreSharedAcrossInstances(t *testing.T) {
	mr := miniredis.RunT(t)
	p := &countingProvider{}
	a, b := sharedMulti(t, mr, p), sharedMulti(t, mr, p)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			m := a
			if i%2 == 1 {
				m = b
			}
			items, err := m.Search(context.Background(), SearchOptions{Query: "Death Note 06"})
			if err != nil || len(items) != 1 {
				t.Errorf("items=%v err=%v", items, err)
			}
		}(i)
	}
	wg.Wait()
	if p.calls.Load() != 1 {
		t.Fatalf("8 searches across 2 instances must hit the tracker once, got %d", p.calls.Load())
	}
}

func TestIndexerCooldownIsSharedAndStaleStandsIn(t *testing.T) {
	mr := miniredis.RunT(t)
	p := &countingProvider{}
	a, b := sharedMulti(t, mr, p), sharedMulti(t, mr, p)
	// Warm the cache, let it go soft-stale, then make the tracker throttle us.
	if _, err := a.Search(context.Background(), SearchOptions{Query: "x"}); err != nil {
		t.Fatal(err)
	}
	a.states[0].rcache.Put(context.Background(), "false:x::0::", &idxResult{Items: []TorrentItem{{InfoHash: "bb", Title: "old", Seeders: 9}}}, time.Millisecond)
	time.Sleep(20 * time.Millisecond)
	p.fail.Store(true)
	items, err := b.Search(context.Background(), SearchOptions{Query: "x"})
	if err != nil || len(items) != 1 {
		t.Fatalf("a stale answer must stand in for a failing tracker: %v %v", items, err)
	}
	// A different query has no stale answer: it fails once, then every instance is in cooldown.
	if _, err := a.Search(context.Background(), SearchOptions{Query: "y"}); err == nil {
		t.Fatal("first failing query must report the failure")
	}
	before := p.calls.Load()
	if _, err := b.Search(context.Background(), SearchOptions{Query: "z"}); err == nil || p.calls.Load() != before {
		t.Fatalf("the other instance must not call the tracker during the shared cooldown (calls %d -> %d, err %v)", before, p.calls.Load(), err)
	}
}
