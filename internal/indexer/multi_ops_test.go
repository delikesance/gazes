package indexer

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

func TestPauseSkipsTheProviderOnEveryInstance(t *testing.T) {
	mr := miniredis.RunT(t)
	p := &countingProvider{}
	a, b := sharedMulti(t, mr, p), sharedMulti(t, mr, p)
	ctx := context.Background()
	if names := a.ProviderNames(); len(names) != 1 || names[0] != "fake" {
		t.Fatalf("names = %v", names)
	}
	if a.Pause(ctx, "nope", time.Minute) || a.Resume(ctx, "nope") || a.Retry(ctx, "nope") {
		t.Fatal("unknown provider accepted")
	}
	if _, ok := a.Health(ctx, "nope"); ok {
		t.Fatal("unknown provider has a health")
	}
	if !a.Pause(ctx, "fake", 10*time.Minute) {
		t.Fatal("pause refused")
	}
	if h, _ := b.Health(ctx, "fake"); h.Paused <= 0 || h.Cooldown != 0 {
		t.Fatalf("health on the other instance = %+v", h)
	}
	if _, err := b.Search(ctx, SearchOptions{Query: "paused"}); err == nil || p.calls.Load() != 0 {
		t.Fatalf("a paused provider was called: err=%v calls=%d", err, p.calls.Load())
	}
	a.Resume(ctx, "fake")
	if items, err := b.Search(ctx, SearchOptions{Query: "resumed"}); err != nil || len(items) != 1 || p.calls.Load() != 1 {
		t.Fatalf("after resume: items=%v err=%v calls=%d", items, err, p.calls.Load())
	}
}

func TestRetryClosesTheBreakerAndPurgeForgetsResults(t *testing.T) {
	mr := miniredis.RunT(t)
	p := &countingProvider{}
	m := sharedMulti(t, mr, p)
	ctx := context.Background()

	p.fail.Store(true) // a 429 opens the circuit at once
	_, _ = m.Search(ctx, SearchOptions{Query: "q1"})
	if h, _ := m.Health(ctx, "fake"); h.Cooldown <= 0 || h.Paused != 0 {
		t.Fatalf("breaker not open: %+v", h)
	}
	p.fail.Store(false)
	m.Retry(ctx, "fake")
	if h, _ := m.Health(ctx, "fake"); h.Cooldown != 0 {
		t.Fatalf("breaker still open after retry: %+v", h)
	}
	if _, err := m.Search(ctx, SearchOptions{Query: "q2"}); err != nil {
		t.Fatal(err)
	}
	calls := p.calls.Load()
	if _, err := m.Search(ctx, SearchOptions{Query: "q2"}); err != nil || p.calls.Load() != calls {
		t.Fatalf("the repeated query was not served from the cache (calls %d -> %d)", calls, p.calls.Load())
	}
	if n, err := m.PurgeCache(ctx); err != nil || n == 0 {
		t.Fatalf("purge = %d, %v", n, err)
	}
	if _, err := m.Search(ctx, SearchOptions{Query: "q2"}); err != nil || p.calls.Load() != calls+1 {
		t.Fatalf("purged query not sent again (calls %d -> %d)", calls, p.calls.Load())
	}
}
