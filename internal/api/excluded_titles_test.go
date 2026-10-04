package api

import (
	"encoding/json"
	"os"
	"slices"
	"testing"

	"github.com/gazes/gazes/internal/metadata"
)

func rezeroSeasons() ([]metadata.AnimeSeason, *metadata.AnimeCatalogItem) {
	base := "Re:Zero kara Hajimeru Isekai Seikatsu"
	seasons := []metadata.AnimeSeason{
		{ID: 21355 - 1, Title: base, Aliases: []string{base, "ReZero", "Re:Zero", "Re:ZERO -Starting Life in Another World-"}},
		{ID: 21355 + 1, Title: base + " 2nd Season", Aliases: []string{base + " 2nd Season", "Re:ZERO -Starting Life in Another World- Season 2"}},
		{ID: 189045, Title: base + " 3rd Season", Aliases: []string{base + " 3rd Season", "Re:ZERO -Starting Life in Another World- Season 3"}},
		{ID: 189046, Title: base + " 4th Season", Aliases: []string{base + " 4th Season", "Re:ZERO -Starting Life in Another World- Season 4"}},
		{ID: 1, Title: "Re:Zero kara Hajimeru Break Time", Aliases: []string{"Re:Zero kara Hajimeru Break Time"}},
		{ID: 2, Title: base + ": Memory Snow", Aliases: []string{base + ": Memory Snow"}},
		{ID: 3, Title: base + ": Hyouketsu no Kizuna", Aliases: []string{base + ": Hyouketsu no Kizuna"}},
	}
	item := &metadata.AnimeCatalogItem{
		ID:           189046,
		DisplayTitle: "Re:ZERO -Starting Life in Another World- Season 4",
		TitleRomaji:  base + " 4th Season",
		Aliases:      []string{base + " 4th Season", "Re:ZERO -Starting Life in Another World- Season 4"},
	}
	return seasons, item
}

func TestBuildExcludedTitlesNeverExcludesItemFranchiseBase(t *testing.T) {
	seasons, item := rezeroSeasons()
	excluded := buildExcludedTitles(item, seasons)
	for _, bad := range []string{
		"ReZero", "Re:Zero", "Zero kara Hajimeru Isekai Seikatsu 2nd Season", "Zero kara Hajimeru Isekai Seikatsu",
		"Re:Zero kara Hajimeru Isekai Seikatsu", "Re:Zero kara Hajimeru Isekai Seikatsu 2nd Season",
		"Starting Life in Another World", "Re:ZERO -Starting Life in Another World-",
	} {
		if slices.Contains(excluded, bad) {
			t.Errorf("%q must not be excluded; got %v", bad, excluded)
		}
	}
	for _, want := range []string{"Re:Zero kara Hajimeru Break Time", "Re:Zero kara Hajimeru Isekai Seikatsu: Memory Snow", "Memory Snow", "Hyouketsu no Kizuna"} {
		if !slices.Contains(excluded, want) {
			t.Errorf("%q should be excluded; got %v", want, excluded)
		}
	}
}

func TestExcludedTitleCompactKeyStripsSeasonMarkers(t *testing.T) {
	for _, in := range []string{"Foo 2nd Season", "Foo Season 2", "Foo S2", "Foo Saison 2", "Foo Part 2", "Foo Cour 2", "Foo 2nd Cour", "Foo Season 2nd"} {
		if got := compactTitleKey(in); got != "foo" {
			t.Errorf("compactTitleKey(%q) = %q, want foo", in, got)
		}
	}
}

func TestBuildExcludedTitlesSkipsMainSeasonForeignTitles(t *testing.T) {
	seasons, item := rezeroSeasons()
	for i := range seasons {
		seasons[i].Format, seasons[i].Group = "TV", "main"
		if i >= 4 {
			seasons[i].Format, seasons[i].Group = "TV_SHORT", "main"
		}
	}
	seasons[0].Aliases = append(seasons[0].Aliases, "Re：从零开始的异世界生活", "Re:Zero — жизнь с нуля в другом мире", "Re:Zero Empezar de cero en un mundo diferente")
	seasons[1].Aliases = append(seasons[1].Aliases, "Re:从零开始的异世界生活第二季", "Re:Zero — жизнь с нуля в другом мире. Второй сезон")
	seasons[2].Aliases = append(seasons[2].Aliases, "Re:从零开始的异世界生活 第三季")
	seasons = append(seasons, metadata.AnimeSeason{ID: 9, Format: "OVA", Group: "extras", Title: "Re:ZERO OVAs", Aliases: []string{"Re:ゼロから始める異世界生活 氷結の絆"}},
		metadata.AnimeSeason{ID: 10, Format: "MOVIE", Group: "movies", Title: "Re:Zero Movie", Aliases: []string{"Re:Zero Movie Special"}})
	excluded := buildExcludedTitles(item, seasons)
	for _, bad := range []string{"Re：从零开始的异世界生活", "Re:从零开始的异世界生活第二季", "Re:从零开始的异世界生活 第三季", "Re:Zero — жизнь с нуля в другом мире", "Re:Zero Empezar de cero en un mundo diferente"} {
		if slices.Contains(excluded, bad) {
			t.Errorf("main-season title %q must not be excluded", bad)
		}
	}
	for _, want := range []string{"Re:Zero kara Hajimeru Break Time", "Re:ゼロから始める異世界生活 氷結の絆", "Re:Zero Movie Special", "Re:ZERO OVAs"} {
		if !slices.Contains(excluded, want) {
			t.Errorf("%q should be excluded; got %v", want, excluded)
		}
	}
}

func TestBuildExcludedTitlesBreakTimeTVShortStaysExcluded(t *testing.T) {
	item := &metadata.AnimeCatalogItem{ID: 1, DisplayTitle: "Foo Season 4", Aliases: []string{"Foo Season 4"}}
	seasons := []metadata.AnimeSeason{
		{ID: 1, Format: "TV", Group: "main", Title: "Foo Season 4"},
		{ID: 2, Format: "TV_SHORT", Group: "main", Title: "Foo Break Time"},
		{ID: 3, Format: "ONA", Group: "main", Title: "Foo Kyuukei Jikan 2nd Season"},
		{ID: 4, Format: "TV", Group: "main", Title: "Foo Season 3", Aliases: []string{"Foo Zhongwen Third"}},
	}
	excluded := buildExcludedTitles(item, seasons)
	for _, want := range []string{"Foo Break Time", "Foo Kyuukei Jikan 2nd Season"} {
		if !slices.Contains(excluded, want) {
			t.Errorf("%q should be excluded; got %v", want, excluded)
		}
	}
	if slices.Contains(excluded, "Foo Zhongwen Third") {
		t.Errorf("TV main season alias must not be excluded; got %v", excluded)
	}
}

func loadRezeroFixture(t *testing.T) ([]metadata.AnimeSeason, *metadata.AnimeCatalogItem) {
	t.Helper()
	raw, err := os.ReadFile("testdata/rezero_franchise.json")
	if err != nil {
		t.Fatal(err)
	}
	var fx struct {
		Item    metadata.AnimeCatalogItem `json:"item"`
		Seasons []metadata.AnimeSeason    `json:"seasons"`
	}
	if err := json.Unmarshal(raw, &fx); err != nil {
		t.Fatal(err)
	}
	return fx.Seasons, &fx.Item
}

func TestBuildExcludedTitlesRealRezeroFranchise(t *testing.T) {
	seasons, item := loadRezeroFixture(t)
	excluded := buildExcludedTitles(item, seasons)
	for _, s := range seasons {
		if s.Group != "main" || s.Format != "TV" {
			continue
		}
		for _, alias := range append([]string{s.Title}, s.Aliases...) {
			if slices.Contains(excluded, alias) {
				t.Errorf("main-season title %q (id %d) must not be excluded", alias, s.ID)
			}
		}
	}
	for _, want := range []string{
		"Re:Zero kara Hajimeru Kyuukei Jikan (Break Time)", "Re:PETIT ~Re: Starting Life in Another World From PETIT~",
		"Re:ZERO -Starting Life in Another World- OVAs", "Re:Zero kara Hajimeru Isekai Seikatsu - Memory Snow", "Hyouketsu no Kizuna",
		"Re:Zero kara Hajimeru Kyuukei Jikan (Break Time) 3rd Season",
	} {
		if !slices.Contains(excluded, want) {
			t.Errorf("%q should be excluded; got %v", want, excluded)
		}
	}
}
