package indexer_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/gazes/gazes/internal/indexer"
)

// recordingProvider logs every query it receives; paced ones declare a request delay like Prowlarr's C411.
type recordingProvider struct {
	name  string
	pace  indexer.Pacing
	items func(indexer.SearchOptions) []indexer.TorrentItem

	mu   sync.Mutex
	seen []indexer.SearchOptions
}

func (p *recordingProvider) Name() string { return p.name }
func (p *recordingProvider) GetLatest(context.Context, string, int) ([]indexer.TorrentItem, error) {
	return nil, nil
}
func (p *recordingProvider) Pacing() indexer.Pacing { return p.pace }
func (p *recordingProvider) Search(_ context.Context, o indexer.SearchOptions) ([]indexer.TorrentItem, error) {
	p.mu.Lock()
	p.seen = append(p.seen, o)
	p.mu.Unlock()
	if p.items == nil {
		return nil, nil
	}
	return p.items(o), nil
}
func (p *recordingProvider) queries() []indexer.SearchOptions {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]indexer.SearchOptions(nil), p.seen...)
}

func pacedIdentity() indexer.EpisodeIdentity {
	return indexer.EpisodeIdentity{Titles: []string{"Frieren"}, SeasonNumber: 4, EpisodeNumber: 1, AllowUnqualified: true}
}

func manyItems(o indexer.SearchOptions) []indexer.TorrentItem {
	out := make([]indexer.TorrentItem, 80) // a full page tempts the resolver to fetch page 2
	for i := range out {
		out[i] = indexer.TorrentItem{InfoHash: fmt.Sprintf("%02d%s%036d", i, "ab", len(o.Query)), Title: "Unrelated release", Seeders: 1}
	}
	return out
}

func queryKey(o indexer.SearchOptions) string {
	return fmt.Sprintf("%s|%s|p%d", o.Category, o.Query, o.Page)
}

func resolveBoth(t *testing.T, playback bool, withPaced bool) (paced, normal *recordingProvider, took time.Duration) {
	t.Helper()
	paced = &recordingProvider{name: "c411", pace: indexer.Pacing{MaxConcurrent: 1, MinInterval: 300 * time.Millisecond}, items: manyItems}
	normal = &recordingProvider{name: "nyaa.si"}
	var multi *indexer.MultiProvider
	if withPaced {
		multi = indexer.NewMultiProvider(paced, normal)
	} else {
		multi = indexer.NewMultiProvider(normal)
	}
	resolver := indexer.NewEpisodeResolver(multi)
	start := time.Now()
	var err error
	if playback {
		_, err = resolver.ResolvePlaybackSources(context.Background(), pacedIdentity())
	} else {
		_, err = resolver.ResolveSeasonSources(context.Background(), pacedIdentity())
	}
	if err != nil {
		t.Fatal(err)
	}
	return paced, normal, time.Since(start)
}

func TestPacedProviderGetsSmallPrioritizedQuerySubset(t *testing.T) {
	for _, playback := range []bool{true, false} {
		paced, normal, took := resolveBoth(t, playback, true)
		got := paced.queries()
		want := []string{"1_0|Frieren S04 VF|p1", "1_0|Frieren VF|p1", "1_0|Frieren MULTI|p1"}
		if len(got) != len(want) {
			t.Fatalf("playback=%v: paced provider must receive exactly %d queries, got %v", playback, len(want), got)
		}
		for i, o := range got {
			if queryKey(o) != want[i] {
				t.Fatalf("playback=%v: query %d = %q, want %q (priority order, no page 2)", playback, i, queryKey(o), want[i])
			}
		}
		if took > 5*time.Second {
			t.Fatalf("resolve must stay far inside its 20 s deadline: %v", took)
		}
		for _, o := range normal.queries() {
			if o.Scope == indexer.ScopePaced {
				t.Fatalf("the paced-only subset leaked to a normal provider: %v", o)
			}
		}
	}
}

func TestNonPacedProviderStillReceivesFullQuerySet(t *testing.T) {
	_, withPaced, _ := resolveBoth(t, true, true)
	_, alone, _ := resolveBoth(t, true, false)
	keys := func(p *recordingProvider) map[string]bool {
		m := map[string]bool{}
		for _, o := range p.queries() {
			m[queryKey(o)] = true
		}
		return m
	}
	a, b := keys(withPaced), keys(alone)
	if len(b) < 10 {
		t.Fatalf("reference run unexpectedly small: %d", len(b))
	}
	for k := range b {
		if !a[k] {
			t.Fatalf("normal provider lost query %s once a paced provider exists", k)
		}
	}
	if len(a) != len(b) {
		t.Fatalf("normal provider received %d distinct queries, want %d", len(a), len(b))
	}
}

func TestPacedProviderNeverCoolsDownDuringResolve(t *testing.T) {
	paced, _, _ := resolveBoth(t, true, true)
	// A cooldown would have rejected calls before they reached the provider, shrinking the count below the budget.
	if n := len(paced.queries()); n != 3 {
		t.Fatalf("all 3 budgeted queries must reach the paced provider, got %d", n)
	}
}

func TestPacedProviderSkippedWhenFrenchPackAlreadyFound(t *testing.T) {
	paced := &recordingProvider{name: "c411", pace: indexer.Pacing{MaxConcurrent: 1, MinInterval: 300 * time.Millisecond}}
	normal := &recordingProvider{name: "nyaa.si", items: func(indexer.SearchOptions) []indexer.TorrentItem {
		return []indexer.TorrentItem{{InfoHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Title: "Frieren S04 VF 1080p Complete", Seeders: 9}}
	}}
	if _, err := indexer.NewEpisodeResolver(indexer.NewMultiProvider(paced, normal)).ResolvePlaybackSources(context.Background(), pacedIdentity()); err != nil {
		t.Fatal(err)
	}
	if n := len(paced.queries()); n != 0 {
		t.Fatalf("a public VF pack makes the private tracker unnecessary, paced got %d queries", n)
	}
}

func TestFastPhaseTimeoutDefaultAndOverride(t *testing.T) {
	firstFast := func(configure func(*indexer.EpisodeResolver)) time.Duration {
		var mu sync.Mutex
		best := time.Hour
		p := sourceProvider{search: func(ctx context.Context, o indexer.SearchOptions) ([]indexer.TorrentItem, error) {
			if dl, ok := ctx.Deadline(); ok {
				mu.Lock()
				best = min(best, time.Until(dl))
				mu.Unlock()
			}
			return nil, nil
		}}
		r := indexer.NewEpisodeResolver(p)
		configure(r)
		if _, err := r.ResolvePlaybackSources(context.Background(), pacedIdentity()); err != nil {
			t.Fatal(err)
		}
		return best
	}
	if d := firstFast(func(*indexer.EpisodeResolver) {}); d > 3*time.Second || d < 2*time.Second {
		t.Fatalf("default fast phase must stay 3 s, deadline left %v", d)
	}
	if d := firstFast(func(r *indexer.EpisodeResolver) { r.SetFastPhaseTimeout(400 * time.Millisecond) }); d > 400*time.Millisecond {
		t.Fatalf("override ignored, deadline left %v", d)
	}
	if d := firstFast(func(r *indexer.EpisodeResolver) { r.SetFastPhaseTimeout(0) }); d > 3*time.Second || d < 2*time.Second {
		t.Fatalf("non-positive override must keep the default, deadline left %v", d)
	}
}
