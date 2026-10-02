package indexer_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/gazes/gazes/internal/indexer"
)

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
		if opts.Query == identity.Titles[0]+" VOSTFR" && opts.Category == "1_2" {
			return []indexer.TorrentItem{{InfoHash: "FR", Title: pack, Seeders: 189}}, nil
		}
		if opts.Query == identity.Titles[0]+" VF" && opts.Category == "1_3" {
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
