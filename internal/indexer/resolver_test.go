package indexer_test

import (
	"strings"
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

func TestNarutoNyaaReleasesAcrossTVEpisodes(t *testing.T) {
	for episode := 1; episode <= 220; episode++ {
		identity := indexer.EpisodeIdentity{Titles: []string{"Naruto"}, ExcludedTitles: []string{"Naruto Shippuden"}, SeasonNumber: 1, EpisodeNumber: episode, AllowUnqualified: true}
		for _, title := range []string{"Naruto Yabai Intégrale 1080p x264 [VO][VF] + Multi Sub [ITA][POR][ES][EN]", "Naruto Yabaï Complete VF", "Naruto Kai Complete VOSTFR", "Naruto FullEdit Episode 01-03", "Naruto SD Complete", "Naruto Spin-Off - Rock Lee Complete", "[Naruto-Kun.Hu] Black Torch 08 [1080p].mkv", "Naruto OVA 01 VF", "Naruto Shippuden Complete VF", "[Judas] Naruto - Movies 01-03 [BD 1080p][Dual Audio]"} {
			if matched, _ := indexer.MatchEpisode(title, identity); matched {
				t.Errorf("episode %d accepted unrelated release %q", episode, title)
			}
		}
		if matched, _ := indexer.MatchEpisode("Naruto DVDRIP VostFr/Vf", identity); !matched {
			t.Errorf("episode %d rejected the original TV pack", episode)
		}
	}
}

func TestNarutoLanguageRankingFromNyaaTitles(t *testing.T) {
	identity := indexer.EpisodeIdentity{Titles: []string{"Naruto"}, SeasonNumber: 1, EpisodeNumber: 1}
	vf := indexer.RankSource(indexer.TorrentItem{Title: "Naruto DVDRIP VostFr/Vf", Seeders: 2}, identity, true)
	vostfr := indexer.RankSource(indexer.TorrentItem{Title: "Naruto Complete VOSTFR 1080p", Seeders: 10000}, identity, true)
	dual := indexer.RankSource(indexer.TorrentItem{Title: "Naruto Complete Series + Movies (High Quality)(Dual Audio) MKV DVDRip", Seeders: 10000}, identity, true)
	if vf.ScoreRank != 213 || vf.LanguageTag != indexer.LangVF || !(vf.ScoreRank > vostfr.ScoreRank && vostfr.ScoreRank > dual.ScoreRank) || dual.IsFrench {
		t.Fatalf("VF > VOSTFR > unconfirmed dual audio violated: %+v / %+v / %+v", vf, vostfr, dual)
	}
}

func TestNarutoExclusionsNormalizeAccentsInLiveReleaseTitles(t *testing.T) {
	identity := indexer.EpisodeIdentity{Titles: []string{"Naruto"}, ExcludedTitles: []string{"Naruto Shippuden", "Boruto Naruto Next Generations"}, SeasonNumber: 1, EpisodeNumber: 1, AllowUnqualified: true}
	for _, title := range []string{
		"[MiracleSharingan] NARUTO SHIPPÛDEN Épisode 473 à 488[VOSTFR_Version_Pro][720p][AAC]",
		"Naruto_Shippûden_375_Miracle-Sharingan_Fansub_Version_Finale_1280x720 [H264-HD-VOSTFR].mp4",
		"Boruto - Naruto Next Générations vostfr 005 à 030 par Fansub-Miracle-Sharingan",
	} {
		if matched, _ := indexer.MatchEpisode(title, identity); matched {
			t.Errorf("accepted sibling title %q", title)
		}
	}
	if matched, _ := indexer.MatchEpisode("Naruto DVDRIP VostFr/Vf", identity); !matched {
		t.Fatal("rejected original Naruto")
	}
}

func TestLatinAccentMatchingDoesNotCollapseJapaneseTitles(t *testing.T) {
	identity := indexer.EpisodeIdentity{Titles: []string{"ガール"}, SeasonNumber: 1, EpisodeNumber: 1, AllowUnqualified: true}
	if matched, _ := indexer.MatchEpisode("カール Complete", identity); matched {
		t.Fatal("distinct Japanese titles collapsed")
	}
	if matched, _ := indexer.MatchEpisode("ガール Complete", identity); !matched {
		t.Fatal("Japanese alias no longer matched")
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
		{"Example S02 1080p 2024", true, true},
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
	if match, batch := indexer.MatchEpisode("Example 1080p", single); match && !batch {
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
	if len(options) != 5 || options[0].Query != "Tensei Shitara Slime Datta Ken VF" || options[0].Category != "1_0" || options[4].Category != "1_3" {
		t.Fatalf("VF must cover all anime categories, with a tagless non-English fallback: %+v", options)
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

func TestSeasonExtrasPackIdentity(t *testing.T) {
	identity := indexer.EpisodeIdentity{Titles: []string{"One Punch Man"}, SeasonNumber: 1, EpisodeNumber: 1, AllowUnqualified: true}
	for _, tt := range []struct {
		title        string
		match, batch bool
	}{
		{"One Punch Man S01 + OAV + OAD - MULTi VF/VOSTFR [BD 1080p Opus] v2 (Darki) | FRENCH", true, true},
		{"One Punch Man S01 + Extras MULTi", true, true},
		{"One Punch Man S02 + OAV VF", false, false},
		{"One Punch Man OAV S01E01 VF", false, false},
		{"One Punch Man S01 + OAV - 01 VF", false, false},
		{"One Punch Man OAD S01 VF", false, false},
		{"One Punch Man S01 1080p", true, true},
	} {
		matched, batch := indexer.MatchEpisode(tt.title, identity)
		if matched != tt.match || batch != tt.batch {
			t.Errorf("%s: got %v,%v want %v,%v", tt.title, matched, batch, tt.match, tt.batch)
		}
	}
}

// Stripping a season/part qualifier from "X Part 6 (Part 2)" used to leave "X ( )",
// which then never matched as the base of a sibling season's own title.
func TestCleanTitleForSearchDropsEmptyBrackets(t *testing.T) {
	cases := map[string]string{
		"JoJo's Bizarre Adventure Part 6 (Part 2)":    "JoJos Bizarre Adventure",
		"Tensei Shitara Slime Datta Ken (2nd Season)": "Tensei Shitara Slime Datta Ken",
		"Show [Part 3]":              "Show",
		"Keep (Real Subtitle) Title": "Keep (Real Subtitle) Title",
	}
	for in, want := range cases {
		if got := indexer.CleanTitleForSearch(in); got != want {
			t.Errorf("CleanTitleForSearch(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCompleteSeriesPack(t *testing.T) {
	first := indexer.EpisodeIdentity{Titles: []string{"Death Note"}, SeasonNumber: 1, EpisodeNumber: 6, AllowUnqualified: true}
	for _, tt := range []struct {
		title string
		match bool
	}{
		{"Death Note - BDRIP - VF VOSTFR - 1080p x265 AC3", true},
		{"[Sav1our] Death Note (EN|ES|FR|PT|RU) [BD][720p][AV1][OPUS][Multi Dual Audio]", true},
		{"Death Note [1080p]", true},
		{"Death Note 07 [BerSerk]", false},
		{"[Odji-san] Death Note Kaï Films 1 à 6 (intégrale) 1080p [DUB][VO|VF|EN]", false},
	} {
		if match, _ := indexer.MatchEpisode(tt.title, first); match != tt.match {
			t.Errorf("%s: match = %v, want %v", tt.title, match, tt.match)
		}
	}
	later := first
	later.SeasonNumber = 2
	if match, _ := indexer.MatchEpisode("Death Note - BDRIP - VF VOSTFR - 1080p x265 AC3", later); match {
		t.Error("a numberless series pack must not satisfy a season 2 target")
	}
}

func TestRezeroS04ExclusionsDoNotRejectOwnReleases(t *testing.T) {
	identity := indexer.EpisodeIdentity{
		Titles:         []string{"Re:Zero kara Hajimeru Isekai Seikatsu 4th Season", "Re:ZERO -Starting Life in Another World- Season 4"},
		ExcludedTitles: []string{"Re:Zero kara Hajimeru Break Time", "Re:Zero kara Hajimeru Isekai Seikatsu: Memory Snow", "Memory Snow", "Hyouketsu no Kizuna"},
		SeasonNumber:   4, EpisodeNumber: 19,
	}
	for _, title := range []string{
		"[ToonsHub] ReZero kara Hajimeru Isekai Seikatsu S04E19 1080p CR WEB-DL AAC2.0 H 264 (Re:ZERO -Starting Life in Another World- Season 4)",
		"[Erai-raws] Re:Zero kara Hajimeru Isekai Seikatsu 4th Season - 19 [1080p CR WEB-DL AVC AAC][MultiSub]",
		"[Judas] Re Zero kara Hajimeru Isekai Seikatsu (Season 4) - S04E19 [1080p][HEVC x265 10bit]",
	} {
		if ok, _, why := indexer.MatchEpisodeDebug(title, identity); !ok {
			t.Errorf("%q should match: %s", title, why)
		}
	}
	for _, title := range []string{
		"[Erai-raws] Re:Zero kara Hajimeru Isekai Seikatsu 2nd Season - 19 [1080p]",
		"[SubsPlease] Re Zero kara Hajimeru Isekai Seikatsu S3 - 19",
		"[Erai-raws] Re:Zero kara Hajimeru Isekai Seikatsu 3rd Season - 19 [1080p]",
		"[X] Re:Zero kara Hajimeru Break Time - 19 [1080p]",
		"[X] Re:Zero kara Hajimeru Isekai Seikatsu: Memory Snow - 19 [1080p]",
	} {
		if ok, _, why := indexer.MatchEpisodeDebug(title, identity); ok {
			t.Errorf("%q should be rejected (%s)", title, why)
		}
	}
}

func TestRezeroS04ChineseReleasesWithoutMainSeasonExclusions(t *testing.T) {
	// Exclusions now hold only non-main entries; foreign-language main-season titles are gone.
	identity := indexer.EpisodeIdentity{
		Titles: []string{"Re:Zero kara Hajimeru Isekai Seikatsu 4th Season", "Re:ZERO -Starting Life in Another World- Season 4", "Re：从零开始的异世界生活"},
		ExcludedTitles: []string{"Re:Zero kara Hajimeru Kyuukei Jikan (Break Time)", "Re:PETIT ~Re: Starting Life in Another World From PETIT~",
			"Re:ZERO -Starting Life in Another World- OVAs", "Memory Snow", "Hyouketsu no Kizuna"},
		SeasonNumber: 4, EpisodeNumber: 19,
	}
	for _, title := range []string{
		"Re：从零开始的异世界生活 第四季 - 19 [1080p]",
		"[晚街与灯][Re：从零开始的异世界生活 第四季 / Re:Zero kara Hajimeru Isekai Seikatsu 4th Season][19 - 总第85][WebRip][1080P_AVC_AAC][简日双语内嵌]",
		"Re：从零开始的异世界生活 第4季 - 19 [1080p]",
		"Re：从零开始的异世界生活 第四期 - 19 [1080p]",
	} {
		if ok, _, why := indexer.MatchEpisodeDebug(title, identity); !ok {
			t.Errorf("%q should match S04E19: %s", title, why)
		}
	}
	for _, title := range []string{
		"Re：从零开始的异世界生活 第二季 - 19 [1080p]",
		"Re：从零开始的异世界生活 第三季 - 19 [1080p]",
		"Re：从零开始的异世界生活 第2期 - 19 [1080p]",
		"Re：从零开始的异世界生活 第一季 - 19 [1080p]",
		"Re：从零开始的异世界生活 第十季 - 19 [1080p]",
		"Re：从零开始的异世界生活 - 19 [1080p]",
		"[X][Re：从零开始的异世界生活 第二季 / Re:Zero kara Hajimeru Isekai Seikatsu][19][1080P]",
		"Re:Zero kara Hajimeru Kyuukei Jikan (Break Time) - 19 [1080p]",
	} {
		if ok, _, why := indexer.MatchEpisodeDebug(title, identity); ok {
			t.Errorf("%q must not match S04E19 (%s)", title, why)
		}
	}
	// With the real S4 identity (AllowUnqualified false, absolute numbering) an unqualified
	// S1-style title must not leak through its absolute episode.
	real := identity
	real.Titles = identity.Titles[:2]
	real.AbsoluteEpisode = 151
	if ok, _, why := indexer.MatchEpisodeDebug("Re:Zero kara Hajimeru Isekai Seikatsu 第二季 - 19 [1080p]", real); ok {
		t.Errorf("S2 Chinese marker leaked: %s", why)
	}
}

func rezeroS04E19Identity() indexer.EpisodeIdentity {
	return indexer.EpisodeIdentity{
		Titles:       []string{"Re:Zero kara Hajimeru Isekai Seikatsu 4th Season", "Re:ZERO -Starting Life in Another World- Season 4", "Re：从零开始的异世界生活"},
		SeasonNumber: 4, EpisodeNumber: 19, AbsoluteEpisode: 85,
	}
}

func TestSingleEpisodeFormsRejectWrongEpisode(t *testing.T) {
	identity := rezeroS04E19Identity()
	for _, title := range []string{
		"[晚街与灯][Re：从零开始的异世界生活 第四季 / Re:Zero kara Hajimeru Isekai Seikatsu 4th Season][18 - 总第84][WEB-DL Remux][1080P_AVC_AAC][简繁日内封PGS]",
		"[晚街与灯][Re：从零开始的异世界生活 第四季 / Re:Zero kara Hajimeru Isekai Seikatsu 4th Season][13 - 总第79][WebRip][1080P_AVC_AAC][简日双语内嵌]",
		"[晚街与灯][Re：从零开始的异世界生活 第四季 / Re:Zero kara Hajimeru Isekai Seikatsu 4th Season][18 - 總第84][WebRip][1080P]",
		"[EA]Re_Zero_kara_Hajimeru_Isekai_Seikatsu_4th_15_[1920x1080][HEVC][C1549343].mkv",
		"[EA]Re_Zero_kara_Hajimeru_Isekai_Seikatsu_4th_15_[1920x1080][HEVC].mkv",
	} {
		ok, _, why := indexer.MatchEpisodeDebug(title, identity)
		if ok || !strings.Contains(why, "episode mismatch") {
			t.Errorf("%q should be an episode mismatch for E19, got ok=%v %q", title, ok, why)
		}
	}
}

func TestSingleEpisodeFormsMatchRightEpisode(t *testing.T) {
	identity := rezeroS04E19Identity()
	for _, title := range []string{
		"[晚街与灯][Re：从零开始的异世界生活 第四季 / Re:Zero kara Hajimeru Isekai Seikatsu 4th Season][19 - 总第85][WebRip][1080P_AVC_AAC][简日双语内嵌]",
		"[晚街与灯][Re：从零开始的异世界生活 第四季 / Re:Zero kara Hajimeru Isekai Seikatsu 4th Season][19 - 總第85][WebRip][1080P]",
		"[EA]Re_Zero_kara_Hajimeru_Isekai_Seikatsu_4th_19_[1920x1080][HEVC][C1549343].mkv",
		"[EA]Re_Zero_kara_Hajimeru_Isekai_Seikatsu_4th_19v2_[1920x1080][HEVC].mkv",
	} {
		ok, batch, why := indexer.MatchEpisodeDebug(title, identity)
		if !ok || batch || !strings.Contains(why, "explicit episode 19") {
			t.Errorf("%q should match as explicit episode 19, got ok=%v batch=%v %q", title, ok, batch, why)
		}
	}
	// Absolute-only marker (no leading episode) matches via the identity's absolute number.
	if ok, _, why := indexer.MatchEpisodeDebug("[X][Re:Zero kara Hajimeru Isekai Seikatsu 4th Season][总第85][1080P]", identity); !ok {
		t.Errorf("总第85 should match absolute 85: %s", why)
	}
	if ok, _, why := indexer.MatchEpisodeDebug("[X][Re:Zero kara Hajimeru Isekai Seikatsu 4th Season][总第84][1080P]", identity); ok {
		t.Errorf("总第84 must not match E19/abs 85: %s", why)
	}
}

func TestSingleEpisodeFormsDoNotMisreadOtherNumbers(t *testing.T) {
	identity := rezeroS04E19Identity()
	// Not episodes: batches, ranges and resolution/codec/CRC/year numbers must stay pack candidates or be rejected as ranges.
	for _, tc := range []struct{ title, want string }{
		{"[EA]Re_Zero_kara_Hajimeru_Isekai_Seikatsu_4th_Season_01-11_[1920x1080][HEVC]", "range 01-11 does not contain"},
		{"[X] Re Zero kara Hajimeru Isekai Seikatsu 4th Season [01-24] [1080p]", "contains ep 19"},
		{"[X] Re Zero kara Hajimeru Isekai Seikatsu 4th Season 01~24 [1080p]", "contains ep 19"},
	} {
		ok, batch, why := indexer.MatchEpisodeDebug(tc.title, identity)
		if !batch || !strings.Contains(why, tc.want) {
			t.Errorf("%q: want range verdict containing %q, got ok=%v batch=%v %q", tc.title, tc.want, ok, batch, why)
		}
	}
	for _, title := range []string{
		"[X] Re Zero kara Hajimeru Isekai Seikatsu 4th Season [1920x1080][HEVC]",
		"[X] Re Zero kara Hajimeru Isekai Seikatsu 4th Season 1080p x265 10bit",
		"[X] Re Zero kara Hajimeru Isekai Seikatsu 4th Season [ABCD1234]",
		"[X] Re Zero kara Hajimeru Isekai Seikatsu 4th Season (2024) [1080p]",
		"[X] Re_Zero_kara_Hajimeru_Isekai_Seikatsu_4th_1080_[1920x1080]",
		"[X] Re_Zero_kara_Hajimeru_Isekai_Seikatsu_4th_2024_[1080p]",
	} {
		ok, batch, why := indexer.MatchEpisodeDebug(title, identity)
		if !ok || !batch || !strings.Contains(why, "season 4 pack") {
			t.Errorf("%q should stay a season-4 pack candidate, got ok=%v batch=%v %q", title, ok, batch, why)
		}
	}
}

func mhaS1E1Identity() indexer.EpisodeIdentity {
	return indexer.EpisodeIdentity{
		Titles:       []string{"My Hero Academia", "Boku no Hero Academia"},
		SeasonNumber: 1, EpisodeNumber: 1, AbsoluteEpisode: 1,
	}
}

func TestFanEditsAreRejectedForEpisodeRequests(t *testing.T) {
	mha := mhaS1E1Identity()
	for _, tc := range []struct {
		title    string
		identity indexer.EpisodeIdentity
	}{
		{"[Johker] My Hero Academia Henshū (Fan-Kaï) - Saisons 1/2/3/4/5/6 - 1080p.MULTI.VF/VOSTFR.x264", mha},
		{"[XYZ] Naruto Kai (Fan Kai) - 01 [1080p]", indexer.EpisodeIdentity{Titles: []string{"Naruto"}, SeasonNumber: 1, EpisodeNumber: 1, AbsoluteEpisode: 1}},
		{"One Piece Fan-Kai - 12 VOSTFR", indexer.EpisodeIdentity{Titles: []string{"One Piece"}, SeasonNumber: 1, EpisodeNumber: 12, AbsoluteEpisode: 12}},
		{"Bleach Henshu 03 MULTI", indexer.EpisodeIdentity{Titles: []string{"Bleach"}, SeasonNumber: 1, EpisodeNumber: 3, AbsoluteEpisode: 3}},
		{"Attack on Titan Fan Edit Ep 5", indexer.EpisodeIdentity{Titles: []string{"Attack on Titan"}, SeasonNumber: 1, EpisodeNumber: 5, AbsoluteEpisode: 5}},
	} {
		ok, _, why := indexer.MatchEpisodeDebug(tc.title, tc.identity)
		if ok || !strings.Contains(why, "fan edit") {
			t.Errorf("%q should be rejected as a fan edit, got ok=%v %q", tc.title, ok, why)
		}
	}
}

func TestFanEditFilterKeepsLegitReleases(t *testing.T) {
	for _, tc := range []struct {
		title    string
		identity indexer.EpisodeIdentity
	}{
		{"[SubsPlease] Dragon Ball Kai - 01 (1080p)", indexer.EpisodeIdentity{Titles: []string{"Dragon Ball Kai"}, SeasonNumber: 1, EpisodeNumber: 1, AbsoluteEpisode: 1}},
		{"[Erai-raws] Boku no Hero Academia - 01 [1080p][Multiple Subtitle]", mhaS1E1Identity()},
		{"Kaiju No. 8 - 01", indexer.EpisodeIdentity{Titles: []string{"Kaiju No. 8"}, SeasonNumber: 1, EpisodeNumber: 1, AbsoluteEpisode: 1}},
	} {
		ok, _, why := indexer.MatchEpisodeDebug(tc.title, tc.identity)
		if !ok {
			t.Errorf("%q should still match, got %q", tc.title, why)
		}
	}
}

func TestMultiSortsBetweenVOSTFRAndEnglish(t *testing.T) {
	identity := indexer.EpisodeIdentity{EpisodeNumber: 1, SeasonNumber: 1}
	sources := []indexer.EpisodeSource{
		indexer.RankSource(indexer.TorrentItem{InfoHash: "en", Title: "Example 2160p", Seeders: 1000}, identity, false),
		indexer.RankSource(indexer.TorrentItem{InfoHash: "multi", Title: "Example MULTI 480p", Seeders: 1}, identity, false),
		indexer.RankSource(indexer.TorrentItem{InfoHash: "sub", Title: "Example VOSTFR 480p", Seeders: 1}, identity, false),
	}
	indexer.SortEpisodeSources(sources)
	if got := sources[0].InfoHash + "," + sources[1].InfoHash + "," + sources[2].InfoHash; got != "sub,multi,en" {
		t.Fatalf("language tiers lost: %s", got)
	}
}
