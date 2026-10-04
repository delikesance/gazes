package indexer_test

import (
	"context"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/gazes/gazes/internal/indexer"
)

func resolveHashes(t *testing.T, items []indexer.TorrentItem) []string {
	t.Helper()
	provider := sourceProvider{search: func(context.Context, indexer.SearchOptions) ([]indexer.TorrentItem, error) {
		return items, nil
	}}
	result, err := indexer.NewEpisodeResolver(provider).ResolveSeasonSources(context.Background(), tensuraIdentity())
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, source := range result.Sources {
		got = append(got, source.InfoHash)
		if marked := strings.Contains(source.MagnetURI, "xs=gazes%3Ac411"); marked != source.IndexerOnly {
			t.Fatalf("only private sources ask the backend for their torrent: %s", source.MagnetURI)
		}
	}
	sort.Strings(got)
	return got
}

func TestPrivateTrackerIsOnlyASeededVFFallback(t *testing.T) {
	title := tensuraIdentity().Titles[0]
	private := []indexer.TorrentItem{
		{Provider: "c411", InfoHash: "private-vf", Title: title + " S01 MULTI VFF 1080p", Seeders: 66, IndexerOnly: true, MagnetURI: "magnet:?xt=urn:btih:private-vf"},
		{InfoHash: "private-dead-vf", Title: title + " S01 VFF 720p", Seeders: 0, IndexerOnly: true},
		{InfoHash: "private-vostfr", Title: title + " S01 VOSTFR 1080p", Seeders: 70, IndexerOnly: true},
	}
	public := indexer.TorrentItem{InfoHash: "public-vostfr", Title: title + " S01 VOSTFR 720p", Seeders: 40}
	if got := resolveHashes(t, append([]indexer.TorrentItem{public}, private...)); !reflect.DeepEqual(got, []string{"private-vf", "public-vostfr"}) {
		t.Fatalf("without a public VF only the seeded private VF is kept: %v", got)
	}
	publicVF := indexer.TorrentItem{InfoHash: "public-vf", Title: title + " S01 VF 1080p", Seeders: 3}
	if got := resolveHashes(t, append([]indexer.TorrentItem{public, publicVF}, private...)); !reflect.DeepEqual(got, []string{"public-vf", "public-vostfr"}) {
		t.Fatalf("a seeded public VF makes the private tracker unnecessary: %v", got)
	}
}

func TestPrivateVFDoesNotEndPlaybackDiscovery(t *testing.T) {
	var mu sync.Mutex
	queries := 0
	provider := sourceProvider{search: func(_ context.Context, opts indexer.SearchOptions) ([]indexer.TorrentItem, error) {
		mu.Lock()
		queries++
		mu.Unlock()
		return []indexer.TorrentItem{{InfoHash: "private", Title: "Naruto Complete VF", Seeders: 80, IndexerOnly: true}}, nil
	}}
	result, err := indexer.NewEpisodeResolver(provider).ResolvePlaybackSources(context.Background(), indexer.EpisodeIdentity{Titles: []string{"Naruto"}, SeasonNumber: 1, EpisodeNumber: 1, AllowUnqualified: true})
	if err != nil || len(result.Sources) != 1 || result.Sources[0].InfoHash != "private" {
		t.Fatalf("%+v %v", result, err)
	}
	if queries <= 6 {
		t.Fatalf("a private VF must not stop the search for public ones after %d queries", queries)
	}
}
