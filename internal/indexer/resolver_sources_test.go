package indexer_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/gazes/gazes/internal/indexer"
)

func TestPlaybackDiscoveryStopsAtSeededVF(t *testing.T) {
	var mu sync.Mutex
	var queries []string
	provider := sourceProvider{search: func(_ context.Context, opts indexer.SearchOptions) ([]indexer.TorrentItem, error) {
		mu.Lock()
		queries = append(queries, opts.Query)
		mu.Unlock()
		return []indexer.TorrentItem{{InfoHash: "vf", Title: "Naruto DVDRIP VostFr/Vf", Seeders: 2}, {InfoHash: "wrong", Title: "Naruto Yabai Complete VF", Seeders: 100}}, nil
	}}
	result, err := indexer.NewEpisodeResolver(provider).ResolvePlaybackSources(context.Background(), indexer.EpisodeIdentity{Titles: []string{"Naruto"}, SeasonNumber: 1, EpisodeNumber: 1, AllowUnqualified: true})
	if err != nil || len(result.Sources) != 1 || result.Sources[0].InfoHash != "vf" || !result.Partial {
		t.Fatalf("%+v %v", result, err)
	}
	if len(queries) != 6 {
		t.Fatalf("expected 6 initial searches, got %d: %v", len(queries), queries)
	}
	for _, query := range queries {
		if strings.Contains(query, "E01") {
			t.Fatalf("episode query ran before a usable season pack: %s", query)
		}
	}
}

func TestSeasonDiscoveryReusesQueriesAcrossEpisodesAndFallsBackToSingles(t *testing.T) {
	identity := indexer.EpisodeIdentity{Titles: []string{"Naruto"}, SeasonNumber: 1, EpisodeNumber: 1, AllowUnqualified: true}
	first := indexer.SeasonPlaybackOptions(identity)
	identity.EpisodeNumber = 20
	if !reflect.DeepEqual(first, indexer.SeasonPlaybackOptions(identity)) {
		t.Fatal("season discovery changed with the episode")
	}
	provider := sourceProvider{search: func(_ context.Context, opts indexer.SearchOptions) ([]indexer.TorrentItem, error) {
		if opts.Query == "Naruto S01E20" {
			return []indexer.TorrentItem{{InfoHash: "single", Title: "Naruto S01E20 VF 720p", Seeders: 2}}, nil
		}
		return nil, nil
	}}
	result, err := indexer.NewEpisodeResolver(provider).ResolvePlaybackSources(context.Background(), identity)
	if err != nil || len(result.Sources) != 1 || result.Sources[0].InfoHash != "single" || result.Sources[0].IsBatch {
		t.Fatalf("single episode fallback failed: %+v %v", result, err)
	}
}

func TestPlaybackDiscoveryContinuesUntilVFSearchesAreCovered(t *testing.T) {
	for _, seeders := range []int{0, 2} {
		provider := sourceProvider{search: func(_ context.Context, opts indexer.SearchOptions) ([]indexer.TorrentItem, error) {
			items := []indexer.TorrentItem{{InfoHash: "sub", Title: "Naruto Complete VOSTFR", Seeders: 10}}
			if opts.Query == "Naruto VF" {
				items = append(items, indexer.TorrentItem{InfoHash: "dead-vf", Title: "Naruto Complete VF", Seeders: 0})
			}
			if opts.Query == "Naruto MULTI" {
				items = append(items, indexer.TorrentItem{InfoHash: "vf", Title: "Naruto Complete VF", Seeders: seeders})
			}
			return items, nil
		}}
		result, err := indexer.NewEpisodeResolver(provider).ResolvePlaybackSources(context.Background(), indexer.EpisodeIdentity{Titles: []string{"Naruto"}, SeasonNumber: 1, EpisodeNumber: 1, AllowUnqualified: true})
		if err != nil || len(result.Sources) != 3 {
			t.Fatalf("fallback discovery incomplete: %+v %v", result, err)
		}
		want := "sub"
		if seeders > 0 {
			want = "vf"
		}
		if result.Sources[0].InfoHash != want {
			t.Fatalf("language/swarm ranking changed: %+v", result.Sources)
		}
	}
}

func TestResolveNarutoLiveNyaaSnapshots(t *testing.T) {
	data, err := os.ReadFile("testdata/naruto-nyaa.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Episodes []struct {
			Episode int                   `json:"episode"`
			Sources []indexer.TorrentItem `json:"sources"`
		} `json:"episodes"`
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	for _, snapshot := range fixture.Episodes {
		provider := sourceProvider{search: func(context.Context, indexer.SearchOptions) ([]indexer.TorrentItem, error) {
			return snapshot.Sources, nil
		}}
		identity := indexer.EpisodeIdentity{Titles: []string{"Naruto"}, ExcludedTitles: []string{"Naruto Shippuden", "Boruto"}, SeasonNumber: 1, EpisodeNumber: snapshot.Episode, AllowUnqualified: true}
		result, err := indexer.NewEpisodeResolver(provider).ResolveSeasonSources(context.Background(), identity)
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Sources) == 0 || result.Sources[0].InfoHash != "e9046a56e25fa17557bd2c585d4a8a9e6d2091fc" {
			t.Fatalf("episode %d: original VF TV pack must lead: %+v", snapshot.Episode, result.Sources)
		}
		for _, source := range result.Sources {
			if source.InfoHash == "3bb2afd33f6cf8ba94cfb2bb17e3960531756562" || source.Title == "[Judas] Naruto - Movies 01-03 [BD 1080p][HEVC x265 10bit][Dual-Audio][Eng-Subs]" {
				t.Errorf("episode %d retained non-TV source %s", snapshot.Episode, source.Title)
			}
		}
		t.Logf("episode %d: %d captured sources -> %d valid TV sources; VF first", snapshot.Episode, len(snapshot.Sources), len(result.Sources))
	}
}

type sourceProvider struct {
	search func(context.Context, indexer.SearchOptions) ([]indexer.TorrentItem, error)
}

func (sourceProvider) Name() string { return "unit fixture" }
func (sourceProvider) GetLatest(context.Context, string, int) ([]indexer.TorrentItem, error) {
	return nil, nil
}
func (p sourceProvider) Search(ctx context.Context, opts indexer.SearchOptions) ([]indexer.TorrentItem, error) {
	return p.search(ctx, opts)
}

func tensuraIdentity() indexer.EpisodeIdentity {
	return indexer.EpisodeIdentity{Titles: []string{"Tensei Shitara Slime Datta Ken"}, ExcludedTitles: []string{"Tensura Nikki", "Slime Diaries"}, SeasonNumber: 1, EpisodeNumber: 1, AllowUnqualified: true}
}

func TestResolveTensuraFrenchDiscoveryAndRanking(t *testing.T) {
	identity := tensuraIdentity()
	pack := "[Trix] Tensei Shitara Slime Datta Ken - S01+02+OADs+Tensura Nikki [Dual Audio] [Multi Subs] (BD 1080p AV1) VOSTFR"
	provider := sourceProvider{search: func(_ context.Context, opts indexer.SearchOptions) ([]indexer.TorrentItem, error) {
		if opts.Query == identity.Titles[0]+" VOSTFR" && opts.Category == "1_0" {
			return []indexer.TorrentItem{{InfoHash: "FR", Title: pack, Seeders: 189}}, nil
		}
		if opts.Query == identity.Titles[0]+" VF" && opts.Category == "1_0" {
			return []indexer.TorrentItem{{InfoHash: "vf", Title: identity.Titles[0] + " S01E01 VF 720p", Seeders: 2}}, nil
		}
		return []indexer.TorrentItem{
			{InfoHash: "english", Title: identity.Titles[0] + " S01E01 [Dual Audio] 2160p", Seeders: 10000},
			{InfoHash: "dead", Title: identity.Titles[0] + " S01E01 VF 1080p", Seeders: 0},
			{InfoHash: "nikki", Title: "Tensura Nikki " + identity.Titles[0] + " S01E01 VOSTFR", Seeders: 500},
			{InfoHash: "wrong-season", Title: identity.Titles[0] + " S03E01 VOSTFR", Seeders: 500},
		}, nil
	}}
	result, err := indexer.NewEpisodeResolver(provider).ResolveSeasonSources(context.Background(), identity)
	if err != nil {
		t.Fatal(err)
	}
	if result.Partial || result.TotalSources != 4 || result.FrenchSources != 3 {
		t.Fatalf("unexpected response: %+v", result)
	}
	got := []string{}
	for _, source := range result.Sources {
		got = append(got, source.InfoHash)
		if !reflect.DeepEqual(source.ExcludedTitles, identity.ExcludedTitles) {
			t.Fatal("file matcher lost spin-off exclusions")
		}
	}
	if !reflect.DeepEqual(got, []string{"vf", "fr", "english", "dead"}) {
		t.Fatalf("seeded French must beat English; dead sources last: %v", got)
	}
	if !result.Sources[1].IsBatch || result.Sources[2].IsFrench {
		t.Fatal("mixed pack or unconfirmed dual audio misclassified")
	}
}

func TestResolveSourcesKeepsBestDuplicate(t *testing.T) {
	provider := sourceProvider{search: func(_ context.Context, opts indexer.SearchOptions) ([]indexer.TorrentItem, error) {
		seeders := 1
		if opts.Category == "1_3" {
			seeders = 9
		}
		return []indexer.TorrentItem{{InfoHash: "ABC", Title: "Example S01E01 VOSTFR", Seeders: seeders}, {InfoHash: "", Title: "Example S01E01 VF", Seeders: 100}}, nil
	}}
	result, err := indexer.NewEpisodeResolver(provider).ResolveSeasonSources(context.Background(), indexer.EpisodeIdentity{Titles: []string{"Example"}, SeasonNumber: 1, EpisodeNumber: 1})
	if err != nil {
		t.Fatal(err)
	}
	if result.TotalSources != 1 || result.Sources[0].InfoHash != "abc" || result.Sources[0].Seeders != 9 {
		t.Fatalf("duplicates or empty hashes leaked: %+v", result)
	}
}

func TestResolveSourcesProviderFailures(t *testing.T) {
	for _, allFail := range []bool{false, true} {
		t.Run(map[bool]string{false: "partial results", true: "all searches fail"}[allFail], func(t *testing.T) {
			provider := sourceProvider{search: func(_ context.Context, opts indexer.SearchOptions) ([]indexer.TorrentItem, error) {
				if allFail || opts.Category == "1_3" {
					return nil, errors.New("provider unavailable")
				}
				return []indexer.TorrentItem{{InfoHash: "ok", Title: "Example S01E01 VOSTFR", Seeders: 2}}, nil
			}}
			result, err := indexer.NewEpisodeResolver(provider).ResolveSeasonSources(context.Background(), indexer.EpisodeIdentity{Titles: []string{"Example"}, SeasonNumber: 1, EpisodeNumber: 1})
			if allFail {
				if err == nil {
					t.Fatal("provider failure must return an error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !result.Partial || result.Warning == "" || result.TotalSources != 1 {
				t.Fatalf("partial results lost: %+v", result)
			}
		})
	}
}

func TestResolveSourcesCancellationAndInvalidIdentity(t *testing.T) {
	provider := sourceProvider{search: func(ctx context.Context, _ indexer.SearchOptions) ([]indexer.TorrentItem, error) {
		return nil, ctx.Err()
	}}
	resolver := indexer.NewEpisodeResolver(provider)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := resolver.ResolveSeasonSources(ctx, tensuraIdentity())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
	for _, identity := range []indexer.EpisodeIdentity{{EpisodeNumber: 1}, {Titles: []string{"Example"}, EpisodeNumber: 0}} {
		if _, err := resolver.ResolveSeasonSources(context.Background(), identity); err == nil {
			t.Fatal("invalid identity accepted")
		}
	}
}

func TestResolveSeasonAndExtrasPack(t *testing.T) {
	const title = "One Punch Man S01 + OAV + OAD - MULTi VF/VOSTFR [BD 1080p Opus] v2 (Darki) | FRENCH"
	const hash = "20bcf004aaffcff0b868edc0c8f45fa4dc316794"
	identity := indexer.EpisodeIdentity{Titles: []string{"One Punch Man"}, ExcludedTitles: []string{"One Punch Man OVA"}, SeasonNumber: 1, EpisodeNumber: 1, AllowUnqualified: true}
	provider := sourceProvider{search: func(_ context.Context, _ indexer.SearchOptions) ([]indexer.TorrentItem, error) {
		return []indexer.TorrentItem{{InfoHash: hash, Title: title, Seeders: 10}}, nil
	}}
	result, err := indexer.NewEpisodeResolver(provider).ResolveSeasonSources(context.Background(), identity)
	if err != nil {
		t.Fatal(err)
	}
	if result.TotalSources != 1 {
		t.Fatalf("season-plus-extras pack missing: %v", result.DebugLog)
	}
	source := result.Sources[0]
	if !source.IsBatch || source.LanguageTag != indexer.LangVF || source.InfoHash != hash {
		t.Fatalf("incorrect pack classification: %+v", source)
	}
}
