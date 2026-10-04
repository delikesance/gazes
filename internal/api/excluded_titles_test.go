package api

import (
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
