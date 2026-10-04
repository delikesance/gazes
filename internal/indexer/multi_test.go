package indexer_test

import (
	"context"
	"errors"
	"github.com/gazes/gazes/internal/indexer"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestMultiDeadlinePreservesHealthySources(t *testing.T) {
	good := sourceProvider{search: func(context.Context, indexer.SearchOptions) ([]indexer.TorrentItem, error) {
		return []indexer.TorrentItem{{InfoHash: "vf", Title: "Naruto VF", Seeders: 2}}, nil
	}}
	slow := sourceProvider{search: func(ctx context.Context, _ indexer.SearchOptions) ([]indexer.TorrentItem, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	items, err := indexer.NewMultiProvider(good, slow).Search(ctx, indexer.SearchOptions{Query: "Naruto VF"})
	var partial *indexer.PartialError
	if len(items) != 1 || items[0].InfoHash != "vf" || !errors.As(err, &partial) || partial.AllFailed {
		t.Fatalf("slow indexer erased healthy VF results: %+v %v", items, err)
	}
}

func TestMultiDeduplicatesCachesAndKeepsPartialResults(t *testing.T) {
	var calls atomic.Int32
	healthy := sourceProvider{search: func(context.Context, indexer.SearchOptions) ([]indexer.TorrentItem, error) {
		calls.Add(1)
		return []indexer.TorrentItem{{InfoHash: "ABC", Title: "Example VF", Seeders: 5}}, nil
	}}
	failed := sourceProvider{search: func(context.Context, indexer.SearchOptions) ([]indexer.TorrentItem, error) {
		return nil, errors.New("offline")
	}}
	multi := indexer.NewMultiProvider(healthy, healthy, failed)
	for range 2 {
		items, err := multi.Search(context.Background(), indexer.SearchOptions{Query: "Example"})
		var partial *indexer.PartialError
		if !errors.As(err, &partial) || len(items) != 1 || items[0].InfoHash != "abc" {
			t.Fatalf("%+v %v", items, err)
		}
	}
	if calls.Load() != 2 {
		t.Fatalf("cache missed: %d", calls.Load())
	}
}
func TestMultiCoalescesConcurrentQueries(t *testing.T) {
	var calls atomic.Int32
	entered := make(chan struct{})
	release := make(chan struct{})
	p := sourceProvider{search: func(context.Context, indexer.SearchOptions) ([]indexer.TorrentItem, error) {
		if calls.Add(1) == 1 {
			close(entered)
		}
		<-release
		return []indexer.TorrentItem{{InfoHash: "one"}}, nil
	}}
	m := indexer.NewMultiProvider(p)
	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			items, err := m.Search(context.Background(), indexer.SearchOptions{Query: "Example"})
			if err != nil || len(items) != 1 {
				t.Errorf("%v %v", items, err)
			}
		}()
	}
	<-entered
	close(release)
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("duplicate requests: %d", calls.Load())
	}
}

func TestMultiPartialResultsReachEpisodeResolver(t *testing.T) {
	good := sourceProvider{search: func(context.Context, indexer.SearchOptions) ([]indexer.TorrentItem, error) {
		return []indexer.TorrentItem{{InfoHash: "fr", Title: "Example S01E01 VF", Seeders: 2}}, nil
	}}
	bad := sourceProvider{search: func(context.Context, indexer.SearchOptions) ([]indexer.TorrentItem, error) {
		return nil, errors.New("blocked")
	}}
	result, err := indexer.NewEpisodeResolver(indexer.NewMultiProvider(good, bad)).ResolveSeasonSources(context.Background(), indexer.EpisodeIdentity{Titles: []string{"Example"}, SeasonNumber: 1, EpisodeNumber: 1})
	if err != nil || result.TotalSources != 1 || !result.Partial || !result.Sources[0].IsFrench {
		t.Fatalf("available VF lost: %+v %v", result, err)
	}
}
func TestMultiCooldownAndCancellation(t *testing.T) {
	var calls atomic.Int32
	bad := sourceProvider{search: func(context.Context, indexer.SearchOptions) ([]indexer.TorrentItem, error) {
		calls.Add(1)
		return nil, &indexer.HTTPError{Provider: "bad", Status: 503}
	}}
	m := indexer.NewMultiProvider(bad)
	for _, query := range []string{"first", "second"} {
		_, err := m.Search(context.Background(), indexer.SearchOptions{Query: query})
		var partial *indexer.PartialError
		if !errors.As(err, &partial) || !partial.AllFailed {
			t.Fatalf("missing failure: %v", err)
		}
	}
	if calls.Load() != 1 {
		t.Fatal("failed provider was not cooled down")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := m.Search(ctx, indexer.SearchOptions{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("lost cancellation: %v", err)
	}
}
