package indexer

import (
	"context"
	"testing"
	"time"
)

type budgetProvider struct{ remaining chan time.Duration }

func (p budgetProvider) Name() string { return "budget" }
func (p budgetProvider) Search(ctx context.Context, _ SearchOptions) ([]TorrentItem, error) {
	deadline, _ := ctx.Deadline()
	p.remaining <- time.Until(deadline)
	return []TorrentItem{{InfoHash: "abc"}}, nil
}
func (p budgetProvider) GetLatest(ctx context.Context, _ string, _ int) ([]TorrentItem, error) {
	return p.Search(ctx, SearchOptions{})
}

func TestQueuedSearchKeepsNetworkBudget(t *testing.T) {
	p := budgetProvider{remaining: make(chan time.Duration, 1)}
	m := NewMultiProvider(p)
	s := m.states[0]
	s.slots <- struct{}{}
	s.slots <- struct{}{}
	released := make(chan struct{})
	go func() { time.Sleep(250 * time.Millisecond); <-s.slots; close(released) }()
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	if _, err := m.Search(ctx, SearchOptions{Query: "episode"}); err != nil {
		t.Fatal(err)
	}
	<-released
	<-s.slots
	if remaining := <-p.remaining; remaining < 3900*time.Millisecond {
		t.Fatalf("queue consumed network deadline: %s", remaining)
	}
}

func TestCancelledQueueDoesNotCoolDownProvider(t *testing.T) {
	p := budgetProvider{remaining: make(chan time.Duration, 1)}
	m := NewMultiProvider(p)
	s := m.states[0]
	s.slots <- struct{}{}
	s.slots <- struct{}{}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, _ = s.search(ctx, SearchOptions{Query: "cancelled"}, false)
	<-s.slots
	<-s.slots
	if s.gov.Cooldown(context.Background()) > 0 {
		t.Fatal("cancelled queue opened provider circuit")
	}
	if _, err := m.Search(context.Background(), SearchOptions{Query: "healthy"}); err != nil {
		t.Fatal(err)
	}
}
