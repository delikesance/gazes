package indexer_test

import (
	"testing"

	"github.com/gazes/gazes/internal/indexer"
)

func TestClassifyLanguage(t *testing.T) {
	tests := []struct {
		title        string
		expectedTag  indexer.LanguageTag
		expectedIsFr bool
	}{
		{
			title:        "[Tsundere-Raws] Sousou no Frieren - 28 [VOSTFR 1080p].mkv",
			expectedTag:  indexer.LangVOSTFR,
			expectedIsFr: true,
		},
		{
			title:        "[T3KASHi] Frieren Beyond Journeys End S02E10 VF 1080p CR WEBRiP AAC 2.0.mkv",
			expectedTag:  indexer.LangVF,
			expectedIsFr: true,
		},
		{
			title:        "[Erai-raws] Jujutsu Kaisen - 45 [1080p][Multiple Subtitle].mkv",
			expectedTag:  indexer.LangMULTI,
			expectedIsFr: false,
		},
		{
			title:        "Sousou no Frieren S02 MULTi AD 1080p WEB-DL",
			expectedTag:  indexer.LangMULTI,
			expectedIsFr: false,
		},
		{
			title:        "[SubsPlease] Solo Leveling - 12 (1080p) [9876ABCD].mkv",
			expectedTag:  indexer.LangOther,
			expectedIsFr: false,
		},
	}

	for _, tt := range tests {
		tag, _, isFr := indexer.ClassifyLanguage(tt.title)
		if tag != tt.expectedTag {
			t.Errorf("ClassifyLanguage(%q) tag = %v, want %v", tt.title, tag, tt.expectedTag)
		}
		if isFr != tt.expectedIsFr {
			t.Errorf("ClassifyLanguage(%q) isFr = %v, want %v", tt.title, isFr, tt.expectedIsFr)
		}
	}
}

func TestEpisodeIdentity(t *testing.T) {
	identity := indexer.EpisodeIdentity{Titles: []string{"Example Season 2"}, SeasonNumber: 2, EpisodeNumber: 3}
	for _, tt := range []struct {
		title        string
		match, batch bool
	}{
		{"[Group] Example S02E03 VOSTFR 1080p", true, false},
		{"Example S01E03 VF", false, false},
		{"Example S02E04 VF", false, false},
		{"Other S02E03", false, false},
		{"Example - 03 VOSTFR", false, false},
		{"Example Season 2 - 03 VOSTFR", true, false},
		{"Example S02 Complete 1080p", true, true},
		{"Example S02 01-12 MULTI", true, true},
		{"Example S02 04-12 MULTI", false, true},
		{"Example S02 1080p 2024", false, true},
	} {
		matched, batch := indexer.MatchEpisode(tt.title, identity)
		if matched != tt.match || batch != tt.batch {
			t.Errorf("%s: got %v,%v want %v,%v", tt.title, matched, batch, tt.match, tt.batch)
		}
	}
	identity.AbsoluteEpisode = 27
	if match, _ := indexer.MatchEpisode("Example - 27 VOSTFR", identity); !match {
		t.Fatal("absolute numbering not matched")
	}
}

func TestRanking(t *testing.T) {
	identity := indexer.EpisodeIdentity{EpisodeNumber: 1, SeasonNumber: 1}
	vf := indexer.RankSource(indexer.TorrentItem{Title: "Example VF 1080p", Seeders: 10}, identity, false)
	sub := indexer.RankSource(indexer.TorrentItem{Title: "Example VOSTFR 1080p", Seeders: 10}, identity, false)
	both := indexer.RankSource(indexer.TorrentItem{Title: "Example VF VOSTFR 1080p", Seeders: 10}, identity, false)
	multi := indexer.RankSource(indexer.TorrentItem{Title: "Example MULTI 1080p", Seeders: 10}, identity, false)
	healthy := indexer.RankSource(indexer.TorrentItem{Title: "Example MULTI 2160p", Seeders: 1000, Leechers: 1000}, identity, false)
	if vf.ScoreRank <= sub.ScoreRank || both.ScoreRank != vf.ScoreRank {
		t.Fatal("VF must outrank VOSTFR without stacking language bonuses")
	}
	if multi.IsFrench || multi.ScoreBreakdown.French != 0 {
		t.Fatal("generic multi must not claim French")
	}
	for _, title := range []string{"Example VF", "Example VOSTFR"} {
		weakFrench := indexer.RankSource(indexer.TorrentItem{Title: title, Seeders: 1}, identity, false)
		if weakFrench.ScoreRank <= healthy.ScoreRank {
			t.Fatal("seeded French source must beat even the highest-scoring non-French source")
		}
	}
	dual := indexer.RankSource(indexer.TorrentItem{Title: "Example [Dual Audio]", Seeders: 10}, identity, false)
	if dual.IsFrench || dual.ScoreBreakdown.French != 0 {
		t.Fatal("dual audio must not claim French")
	}
	for _, leechers := range []int{-2, 0, 1, 1000} {
		withLeechers := indexer.RankSource(indexer.TorrentItem{Title: "Example VF 1080p", Seeders: 10, Leechers: leechers}, identity, false)
		if withLeechers.ScoreRank != vf.ScoreRank || withLeechers.ScoreBreakdown.Leechers != 0 {
			t.Fatal("leechers must not contribute to the score")
		}
	}
	unknown := indexer.RankSource(indexer.TorrentItem{Title: "Example", Seeders: -1, Leechers: -2}, identity, false)
	if unknown.ScoreRank != 0 {
		t.Fatalf("unknown/negative data scored %d", unknown.ScoreRank)
	}
}

func TestPartAndMovieIdentity(t *testing.T) {
	part := indexer.EpisodeIdentity{Titles: []string{"Example Season 2 Part 2"}, SeasonNumber: 2, PartNumber: 2, EpisodeNumber: 3}
	if match, _ := indexer.MatchEpisode("Example S02E03", part); match {
		t.Fatal("unqualified episode leaks into part 2")
	}
	if match, _ := indexer.MatchEpisode("Example Season 2 Part 2 - 03", part); !match {
		t.Fatal("explicit part not matched")
	}
	movie := indexer.EpisodeIdentity{Titles: []string{"Example Movie"}, SeasonNumber: 1, EpisodeNumber: 1, AllowUnqualified: true, Standalone: true}
	if match, _ := indexer.MatchEpisode("Example Movie VOSTFR 1080p", movie); !match {
		t.Fatal("standalone movie not matched")
	}
	single := indexer.EpisodeIdentity{Titles: []string{"Example"}, SeasonNumber: 1, EpisodeNumber: 3, AllowUnqualified: true}
	if match, _ := indexer.MatchEpisode("Example 03 1080p", single); !match {
		t.Fatal("bare episode after title not matched")
	}
	if match, _ := indexer.MatchEpisode("Example 1080p", single); match {
		t.Fatal("resolution mistaken for episode")
	}
	if match, batch := indexer.MatchEpisode("Example S01E01-E12 VOSTFR", single); !match || !batch {
		t.Fatal("tagged range not matched")
	}
}

func TestGenericAliasCannotLeakIntoLaterArc(t *testing.T) {
	titles := []string{"Example Swordsmith Arc", "Example"}
	identity := indexer.EpisodeIdentity{Titles: titles, UnqualifiedTitles: indexer.UniqueSeasonAliases(titles, []string{"Example"}), SeasonNumber: 3, EpisodeNumber: 1}
	if match, _ := indexer.MatchEpisode("Example - 01 VOSTFR", identity); match {
		t.Fatal("generic alias leaked season 1 into season 3")
	}
	if match, _ := indexer.MatchEpisode("Example Swordsmith Arc - 01 VOSTFR", identity); !match {
		t.Fatal("distinct arc title not matched")
	}
}

func TestNarutoEpisode14ReportedReleases(t *testing.T) {
	identity := indexer.EpisodeIdentity{Titles: []string{"Naruto Shippuden", "Naruto Shippuuden"}, SeasonNumber: 1, EpisodeNumber: 14, AllowUnqualified: true}
	for _, tt := range []struct {
		title string
		want  bool
	}{
		{"[Anime Time] Naruto Shippuden Complete (001-500 + Movies) [Dual Audio][1080p][HEVC 10bit x265][AAC][Eng Sub] [Batch]", true},
		{"[HorribleSubs] Naruto Shippuuden (80-426) [1080p] (Batch)", false},
		{"[HorribleSubs] Naruto Shippuuden (427-500) [1080p] (Unofficial Batch)", false},
		{"[Taka]_Naruto_Shippuuden_311-352_[720p][BATCH]", false},
		{"[Taka]_Naruto_Shippuuden_116_-_283_[480p][BATCH]", false},
		{"[DB]_Naruto_Shippuuden_033-056_[Batch]", false},
		{"[DB]_Naruto_Shippuuden_001-032_[Batch]", true},
		{"[Rikudou] Naruto Shippuden Batch 02 (051-100) [MULTI-SUB] HEVC x265 10bit", false},
		{"[Rikudou] Naruto Shippuden Batch 01 + Bonus (001-050) [MULTI-SUB] HEVC x265 10bit", true},
		{"Naruto Shippuuden - Ed - 14 - [HD][by Angel7000].avi", false},
		{"[Anime Time] Naruto Movie Collection (Naruto + Naruto Shippuden) [BD] [Dual Audio] [1080p] [Batch]", false},
		{"[Anime Time] Naruto + Naruto Shippuden (Missing and Fixed) + Naruto All OP,ED,OST [Dual Audio][480p+1080p][Batch]", false},
		{"Naruto.Shippuden.Ep.001-079.MULTi.[VF+VOSTFR].1080p.x265-Raton", true},
		{"Naruto.Shippuden.Ep.080-289.MULTi.[VF+VOSTFR].1080p.x264-Raton", false},
		{"Naruto.Shippuden.Ep.296-500.MULTi.[VF+VOSTFR].1080p.x264-Raton", false},
		{"Naruto.Shippuden.iNTEGRALE.Multi.FR.VOSTFR.Dvdrip.X264-MANGACiTY", true},
		{"Naruto_Shippuuden_014_VOSTFR_1080p.mkv", true},
		{"Naruto Shippuden S01E14 VF 1080p", true},
		{"Naruto Shippuden S01E15 VF 1080p", false},
	} {
		match, _ := indexer.MatchEpisode(tt.title, identity)
		if match != tt.want {
			t.Errorf("%q: got %v want %v", tt.title, match, tt.want)
		}
	}
	for _, title := range []string{"Naruto_Shippuuden_014_VOSTFR_1080p.mkv", "Naruto.Shippuden.Ep.001-079.MULTi.[VF+VOSTFR].1080p.x265-Raton", "Naruto.Shippuden.S01E14.VFVOSTFR"} {
		source := indexer.RankSource(indexer.TorrentItem{Title: title}, identity, false)
		if !source.IsFrench {
			t.Errorf("missed explicit French: %s", title)
		}
	}
	_, _, isFrench := indexer.ClassifyLanguage("[Anime Time] Naruto Shippuden Complete [Dual Audio][Eng Sub]")
	if isFrench {
		t.Fatal("English dual audio incorrectly claimed French")
	}
}

func TestTensuraMultiSeasonFrenchPack(t *testing.T) {
	title := "[Trix] Tensei Shitara Slime Datta Ken - S01+02+OADs+Tensura Nikki [Dual Audio] [Multi Subs] (BD 1080p AV1) - Moi quand je me réincarne en Slime VOSTFR"
	identity := indexer.EpisodeIdentity{Titles: []string{"Tensei Shitara Slime Datta Ken"}, ExcludedTitles: []string{"Tensura Nikki", "Slime Diaries"}, SeasonNumber: 1, EpisodeNumber: 1, AllowUnqualified: true}
	for _, season := range []int{1, 2} {
		identity.SeasonNumber = season
		match, batch := indexer.MatchEpisode(title, identity)
		if !match || !batch {
			t.Fatalf("season %d missing", season)
		}
	}
	identity.SeasonNumber = 3
	if match, _ := indexer.MatchEpisode(title, identity); match {
		t.Fatal("pack contains no season three")
	}
	identity.SeasonNumber = 1
	if match, _ := indexer.MatchEpisode("Tensei Shitara Slime Datta Ken Tensura Nikki - 01 VOSTFR", identity); match {
		t.Fatal("standalone spin-off accepted")
	}
}

func TestFrenchSearchPrioritizesMultilingualPacks(t *testing.T) {
	identity := indexer.EpisodeIdentity{Titles: []string{"Tensei Shitara Slime Datta Ken"}, SeasonNumber: 1, EpisodeNumber: 1}
	options := indexer.FrenchSearchOptions(identity)
	if len(options) < 2 || options[0].Query != "Tensei Shitara Slime Datta Ken VF" || options[0].Category != "1_2" || options[1].Category != "1_3" {
		t.Fatalf("French packs must be searched first in both translated categories: %+v", options)
	}
}

func TestWeakVFBeatsHealthyVOSTFR(t *testing.T) {
	identity := indexer.EpisodeIdentity{EpisodeNumber: 1, SeasonNumber: 1}
	vf := indexer.RankSource(indexer.TorrentItem{Title: "Example VF 480p", Seeders: 1}, identity, false)
	vostfr := indexer.RankSource(indexer.TorrentItem{Title: "Example VOSTFR MULTI 2160p", Seeders: 10000}, identity, false)
	if vf.ScoreRank <= vostfr.ScoreRank || vf.ScoreBreakdown.French != 200 || vostfr.ScoreBreakdown.French != 100 {
		t.Fatalf("VF priority lost: VF=%+v VOSTFR=%+v", vf.ScoreBreakdown, vostfr.ScoreBreakdown)
	}
}
