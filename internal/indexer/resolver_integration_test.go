package indexer

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
)

type fakeProvider struct{ fail bool }

func (f fakeProvider) Name() string                                                  { return "fake" }
func (f fakeProvider) GetLatest(context.Context, string, int) ([]TorrentItem, error) { return nil, nil }
func (f fakeProvider) Search(_ context.Context, o SearchOptions) ([]TorrentItem, error) {
	if f.fail {
		return nil, errors.New("offline")
	}
	return []TorrentItem{{InfoHash: "ABC", Title: "Example S02E03 VF", Seeders: 0}, {InfoHash: "DEF", Title: "Example S02E03 VOSTFR", Seeders: 2}, {InfoHash: "ENG", Title: "Example S02E03 480p", Seeders: 1}, {InfoHash: "BAD", Title: "Example S01E03 VF", Seeders: 100}}, nil
}
func TestResolverCoverage(t *testing.T) {
	identity := EpisodeIdentity{Titles: []string{"Example Season 2", "Example Season 2"}, SeasonNumber: 2, EpisodeNumber: 3}
	r, err := NewEpisodeResolver(fakeProvider{}).ResolveSeasonSources(context.Background(), identity)
	if err != nil || len(r.Sources) != 3 {
		t.Fatalf("%+v %v", r, err)
	}
	if r.Sources[0].InfoHash != "def" || r.Sources[1].InfoHash != "eng" || r.Sources[2].InfoHash != "abc" {
		t.Fatal("seeded sources must precede unseeded")
	}
	if _, err := NewEpisodeResolver(fakeProvider{true}).ResolveSeasonSources(context.Background(), identity); err == nil {
		t.Fatal("provider failure reported as empty success")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := NewEpisodeResolver(fakeProvider{}).ResolveSeasonSources(ctx, identity); err == nil {
		t.Fatal("cancellation ignored")
	}
}

type renamedProvider struct {
	queries []string
	mu      sync.Mutex
}

func (p *renamedProvider) Name() string { return "renamed" }
func (p *renamedProvider) GetLatest(context.Context, string, int) ([]TorrentItem, error) {
	return nil, nil
}
func (p *renamedProvider) Search(_ context.Context, o SearchOptions) ([]TorrentItem, error) {
	p.mu.Lock()
	p.queries = append(p.queries, o.Query)
	p.mu.Unlock()
	return []TorrentItem{
		{InfoHash: "correct", Title: "Naruto Shippuden S01E01 VOSTFR", Seeders: 5},
		{InfoHash: "wrong-franchise-number", Title: "Naruto Shippuden S02E01 VF", Seeders: 100},
		{InfoHash: "original", Title: "Naruto S01E01 VF", Seeders: 100},
		{InfoHash: "wrong-title", Title: "Naruto S02E01 VF", Seeders: 100},
		{InfoHash: "wrong-episode", Title: "Naruto Shippuden S01E02 VF", Seeders: 100},
	}, nil
}
func TestNarutoShippudenExactSearch(t *testing.T) {
	provider := &renamedProvider{}
	identity := EpisodeIdentity{Titles: []string{"Naruto Shippuden", "Naruto: Shippuden"}, SeasonNumber: 1, EpisodeNumber: 1, AllowUnqualified: true}
	queries := EpisodeSearchQueries(identity)
	if len(queries) == 0 || queries[0] != "Naruto Shippuden S01E01" {
		t.Fatalf("wrong exact query: %v", queries)
	}
	result, err := NewEpisodeResolver(provider).ResolveSeasonSources(context.Background(), identity)
	if err != nil || len(result.Sources) != 1 || result.Sources[0].InfoHash != "correct" {
		t.Fatalf("wrong Shippuden source: %+v %v", result, err)
	}
	for _, query := range provider.queries {
		if strings.Contains(query, "S02") {
			t.Errorf("franchise season leaked into query: %s", query)
		}
	}
	original := EpisodeIdentity{Titles: []string{"Naruto"}, ExcludedTitles: []string{"Naruto: Shippuden"}, SeasonNumber: 1, EpisodeNumber: 1, AllowUnqualified: true}
	result, err = NewEpisodeResolver(provider).ResolveSeasonSources(context.Background(), original)
	if err != nil || len(result.Sources) != 1 || result.Sources[0].InfoHash != "original" {
		t.Fatalf("Shippuden leaked into original Naruto: %+v %v", result, err)
	}
}

type frenchProvider struct {
	mu      sync.Mutex
	queries []SearchOptions
}

func (p *frenchProvider) Name() string { return "french" }
func (p *frenchProvider) GetLatest(context.Context, string, int) ([]TorrentItem, error) {
	return nil, nil
}
func (p *frenchProvider) Search(_ context.Context, o SearchOptions) ([]TorrentItem, error) {
	p.mu.Lock()
	p.queries = append(p.queries, o)
	p.mu.Unlock()
	if strings.Contains(o.Query, "VOSTFR") || strings.Contains(o.Query, " VF") {
		return []TorrentItem{
			{InfoHash: "fr", Title: "Naruto.Shippuden.Ep.001-079.MULTi.[VF+VOSTFR].1080p.x265-Raton", Seeders: 12, Leechers: 1},
			{InfoHash: "mangacity", Title: "Naruto.Shippuden.iNTEGRALE.Multi.FR.VOSTFR.Dvdrip.X264-MANGACiTY", Seeders: 10, Leechers: 2},
		}, nil
	}
	return []TorrentItem{{InfoHash: "english", Title: "[Anime Time] Naruto Shippuden Complete (001-500 + Movies) [Dual Audio][1080p][HEVC 10bit x265][AAC][Eng Sub] [Batch]", Seeders: 256, Leechers: 47}, {InfoHash: "wrong-range", Title: "Naruto Shippuden (80-426) [Batch]", Seeders: 100}}, nil
}
func TestTargetedFrenchSources(t *testing.T) {
	provider := &frenchProvider{}
	result, err := NewEpisodeResolver(provider).ResolveSeasonSources(context.Background(), EpisodeIdentity{Titles: []string{"Naruto Shippuden", "Naruto Shippuuden"}, SeasonNumber: 1, EpisodeNumber: 14, AllowUnqualified: true})
	if err != nil || result.FrenchSources != 2 || result.TotalSources != 3 {
		t.Fatalf("%+v %v", result, err)
	}
	for i, expected := range []struct {
		hash  string
		score int
	}{{"fr", 235}, {"mangacity", 130}, {"english", 65}} {
		source := result.Sources[i]
		if source.InfoHash != expected.hash || source.ScoreRank != expected.score {
			t.Errorf("source %d: got %s (score %d), want %s (score %d)", i, source.InfoHash, source.ScoreRank, expected.hash, expected.score)
		}
	}
	sawFrenchCategory := false
	for _, query := range provider.queries {
		if query.Category == "1_3" {
			sawFrenchCategory = true
		}
	}
	if !sawFrenchCategory {
		t.Fatal("non-English discovery query missing")
	}
}

func TestSteelBallRunContinuationIdentity(t *testing.T) {
	override := NumberingOverrides[210482]
	identity := EpisodeIdentity{Titles: override.Titles, SeasonNumber: override.Season, EpisodeNumber: 1, AbsoluteEpisode: 2, TaggedEpisode: 2, AllowUnqualified: true}
	for _, title := range []string{"[ToonsHub] JoJos Bizarre Adventure S06E02 1080p (JoJo no Kimyou na Bouken: Steel Ball Run - 2nd - 3rd STAGE)", "[Erai-raws] JoJo no Kimyou na Bouken: Steel Ball Run - 02 [1080p]"} {
		if matched, _ := MatchEpisode(title, identity); !matched {
			t.Errorf("missed continuation %s", title)
		}
	}
	for _, title := range []string{"[ToonsHub] JoJos Bizarre Adventure S06E01 (Steel Ball Run)", "[SBR] Highschool DxD BorN (Batch)"} {
		if matched, _ := MatchEpisode(title, identity); matched {
			t.Errorf("accepted wrong episode %s", title)
		}
	}
	aliasIdentity := EpisodeIdentity{Titles: []string{"SBR"}, SeasonNumber: 1, EpisodeNumber: 1, AllowUnqualified: true}
	if matched, _ := MatchEpisode("[SBR] Highschool DxD BorN (Batch)", aliasIdentity); matched {
		t.Fatal("release group matched an anime alias")
	}
	if matched, _ := MatchEpisode("[Group] SBR - 01", aliasIdentity); !matched {
		t.Fatal("real acronym title rejected")
	}
}
