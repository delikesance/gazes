package indexer_test

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/gazes/gazes/internal/indexer"
)

func tensuraCorpus(t testing.TB) indexer.SourceAudit {
	t.Helper()
	data, err := os.ReadFile("testdata/tensura-nyaa.json")
	if err != nil {
		t.Fatal(err)
	}
	var a indexer.SourceAudit
	if err = json.Unmarshal(data, &a); err != nil {
		t.Fatal(err)
	}
	return a
}

func TestTensuraCorpusKeepsTVPacksAndRejectsSeparateExtras(t *testing.T) {
	a := indexer.ReplaySourceAudit(tensuraCorpus(t))
	decisions := map[string]indexer.CandidateDecision{}
	for _, d := range a.Decisions {
		decisions[d.InfoHash] = d
	}
	// Independently identified TV season packs from the live listings and file lists.
	for _, hash := range []string{"21ff74256476e035de1de7e04fa368a0abbe9886", "40ab140c978a47c1de5869d23d458ac5b3e8e141", "1e49744792f7f6cf91f9501ec14edf11a52f5c36", "4e7924d184d8d9e94eadde4436a685a32926c552", "c23cc66c9b5bdcd10db45a12884d4283ca1dd317", "72117eae4e11bb12769ee6395ea4481ddebb929e"} {
		if !decisions[hash].Accepted {
			t.Errorf("TV pack lost: %+v", decisions[hash])
		}
	}
	if decisions["cd35afd1fe7524a7f7b7f4372c004084213131a7"].Accepted {
		t.Fatal("standalone OVAs became TV episodes")
	}
	for _, source := range a.Result.Sources {
		if source.LanguageTag == indexer.LangVF {
			t.Errorf("corpus contains no season-one VF claim: %s", source.Title)
		}
	}
}

func TestAuditPreservesRejectedCandidatesAndReplaysWithoutDiscovery(t *testing.T) {
	identity := tensuraIdentity()
	p := indexer.NewAuditProvider(sourceProvider{search: func(context.Context, indexer.SearchOptions) ([]indexer.TorrentItem, error) {
		return []indexer.TorrentItem{{InfoHash: "tv", Title: identity.Titles[0] + " S01E01 VF"}, {InfoHash: "wrong", Title: identity.Titles[0] + " S03E01 VF"}}, nil
	}})
	_, err := p.Search(context.Background(), indexer.SearchOptions{Query: "example"})
	if err != nil {
		t.Fatal(err)
	}
	a := p.Snapshot(identity)
	if len(a.Items) != 2 || len(a.Searches) != 1 || len(a.Decisions) != 2 {
		t.Fatalf("raw evidence lost: %+v", a)
	}
	a.Result = &indexer.EpisodeSourcesResponse{Partial: true, Warning: "partial capture"}
	r := indexer.ReplaySourceAudit(a)
	if r.Result.TotalSources != 1 || r.Result.Sources[0].InfoHash != "tv" || !r.Result.Partial || r.Result.Warning != "partial capture" {
		t.Fatalf("invalid replay: %+v", r.Result)
	}
	if !reflect.DeepEqual(a.Searches, r.Searches) {
		t.Fatal("replay changed discovery evidence")
	}
}

func TestFrenchPlanBoundsCostAndCoversTaglessAliases(t *testing.T) {
	a := tensuraCorpus(t)
	options := indexer.FrenchSearchOptions(a.Identity)
	if len(options) > 18 {
		t.Fatalf("French discovery budget grew: %d", len(options))
	}
	seen := map[string]bool{}
	frenchAlias := false
	for _, o := range options {
		key := o.Query + "|" + o.Category
		if seen[key] {
			t.Fatalf("duplicate: %s", key)
		}
		seen[key] = true
		if o.Query == "Moi, quand je me réincarne en Slime" && o.Category == "1_3" {
			frenchAlias = true
		}
	}
	if !frenchAlias {
		t.Fatal("tagless French alias absent")
	}
	if total := len(options) + len(indexer.EpisodeSearchQueries(a.Identity)); total > 34 {
		t.Fatalf("combined query budget grew: %d", total)
	}
}

func TestFrenchSubtitleWordsNeverClaimVF(t *testing.T) {
	for _, title := range []string{"Example S01E01 French Subs", "Example S01E01 English Audio French Subtitles", "Example Subtitles in French"} {
		tag, _, fr := indexer.ClassifyLanguage(title)
		if tag != indexer.LangVOSTFR || !fr {
			t.Errorf("subtitle-only claim misclassified: %s => %s", title, tag)
		}
	}
	for _, title := range []string{"Example VFF", "Example VFQ", "Example VF2", "Example VF French Subs"} {
		tag, _, _ := indexer.ClassifyLanguage(title)
		if tag != indexer.LangVF {
			t.Errorf("French dub claim lost: %s", title)
		}
	}
}

func TestTensuraPublishedFrenchPacksRemainCandidates(t *testing.T) {
	identity := tensuraCorpus(t).Identity
	for _, title := range []string{
		"THAT TIME I GOT REINCARNATED AS A SLIME S01 1080p BLURAY MULTi VFF PCM AC3 x264-FoX",
		"That.Time.I.Got.Reincarnated.as.a.Slime.S01 + 5 OAV.MULTi.1080p.BluRay.x264-SHiNiGAMi (Tensei Shitara Slime Datta Ken / Moi quand je me réincarne en Slime)",
	} {
		accepted, batch, reason := indexer.MatchEpisodeDebug(title, identity)
		if !accepted || !batch {
			t.Errorf("published TV pack lost: %s: %s", title, reason)
		}
	}
	for _, title := range []string{
		"That Time I Got Reincarnated as a Slime S01 OAV 5 MULTI",
		"That Time I Got Reincarnated as a Slime S01E02 + 5 OAV MULTI",
		"That Time I Got Reincarnated as a Slime S02 + 5 OAV MULTI",
	} {
		if accepted, _, reason := indexer.MatchEpisodeDebug(title, identity); accepted {
			t.Errorf("wrong episode or standalone extras accepted: %s: %s", title, reason)
		}
	}
}

func TestFastDiscoveryReturnsUnconfirmedMultiForTrackInspection(t *testing.T) {
	provider := sourceProvider{search: func(context.Context, indexer.SearchOptions) ([]indexer.TorrentItem, error) {
		return []indexer.TorrentItem{{InfoHash: "multi", Title: "Example S01 Complete MULTI", Seeders: 10}}, nil
	}}
	res, err := indexer.NewEpisodeResolver(provider).ResolvePlaybackSources(context.Background(), indexer.EpisodeIdentity{Titles: []string{"Example"}, SeasonNumber: 1, EpisodeNumber: 1, AllowUnqualified: true})
	if err != nil || res.TotalSources != 1 || !res.Partial || res.Sources[0].IsFrench || res.Sources[0].FrenchEvidence != "unconfirmed" {
		t.Fatalf("MULTI misrepresented: %+v %v", res, err)
	}
}

func BenchmarkTensuraMatcher(b *testing.B) {
	a := tensuraCorpus(b)
	b.Run("prepare_each_candidate", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			for _, item := range a.Items {
				indexer.MatchEpisodeDebug(item.Title, a.Identity)
			}
		}
	})
	b.Run("prepare_once", func(b *testing.B) {
		matcher := indexer.NewEpisodeMatcher(a.Identity)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			for _, item := range a.Items {
				matcher.Match(item.Title)
			}
		}
	})
}
