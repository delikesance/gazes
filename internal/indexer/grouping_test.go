package indexer_test

import (
	"testing"

	"github.com/gazes/gazes/internal/indexer"
	"github.com/gazes/gazes/internal/metadata"
)

func TestExtractReleaseGroup(t *testing.T) {
	tests := []struct {
		title    string
		expected string
	}{
		{"[SubsPlease] Jujutsu Kaisen - 45 (1080p) [ABCD1234].mkv", "SubsPlease"},
		{"[Erai-raws] One Piece - 1090 [1080p][Multiple Subtitle].mkv", "Erai-raws"},
		{"[ToonsHub] Bleach Thousand-Year Blood War - 26 [1080p].mkv", "ToonsHub"},
		{"Solo Leveling - S01E08 (1080p).mkv", "Other"},
	}

	for _, tt := range tests {
		got := indexer.ExtractReleaseGroup(tt.title)
		if got != tt.expected {
			t.Errorf("ExtractReleaseGroup(%q) = %q, want %q", tt.title, got, tt.expected)
		}
	}
}

func TestExtractQuality(t *testing.T) {
	tests := []struct {
		title    string
		expected string
	}{
		{"[SubsPlease] Frieren - 28 (1080p).mkv", "1080p"},
		{"[Erai-raws] Frieren - 28 (720p).mkv", "720p"},
		{"[ToonsHub] Frieren - 28 (480p).mkv", "480p"},
		{"[SomeGroup] Frieren - 28 (4K UHD).mkv", "4K UHD"},
		{"[SomeGroup] Frieren - 28.mkv", "Unknown"},
	}

	for _, tt := range tests {
		got := indexer.ExtractQuality(tt.title)
		if got != tt.expected {
			t.Errorf("ExtractQuality(%q) = %q, want %q", tt.title, got, tt.expected)
		}
	}
}

func TestGroupTorrentsByAnime(t *testing.T) {
	items := []indexer.TorrentItem{
		{
			Title:   "[SubsPlease] Frieren: Beyond Journey's End - 28 (1080p) [12345678].mkv",
			Seeders: 150,
			AnimeDetails: &metadata.AnimeMetadata{
				ID:           154587,
				DisplayTitle: "Sousou no Frieren",
			},
		},
		{
			Title:   "[Erai-raws] Frieren: Beyond Journey's End - 28 [720p].mkv",
			Seeders: 50,
			AnimeDetails: &metadata.AnimeMetadata{
				ID:           154587,
				DisplayTitle: "Sousou no Frieren",
			},
		},
		{
			Title:   "[SubsPlease] Jujutsu Kaisen - 45 (1080p) [87654321].mkv",
			Seeders: 200,
			AnimeDetails: &metadata.AnimeMetadata{
				ID:           145064,
				DisplayTitle: "Jujutsu Kaisen 2nd Season",
			},
		},
	}

	groups := indexer.GroupTorrentsByAnime(items)

	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}

	// First group should be Jujutsu Kaisen because max seeders (200) > Frieren (150)
	if groups[0].ID != "anilist-145064" {
		t.Errorf("expected first group ID to be anilist-145064, got %s", groups[0].ID)
	}
	if groups[0].MaxSeeders != 200 {
		t.Errorf("expected Jujutsu Kaisen max seeders 200, got %d", groups[0].MaxSeeders)
	}
	if groups[0].ReleaseCount != 1 {
		t.Errorf("expected Jujutsu Kaisen release count 1, got %d", groups[0].ReleaseCount)
	}

	// Second group should be Frieren with 2 releases
	if groups[1].ID != "anilist-154587" {
		t.Errorf("expected second group ID to be anilist-154587, got %s", groups[1].ID)
	}
	if groups[1].ReleaseCount != 2 {
		t.Errorf("expected Frieren release count 2, got %d", groups[1].ReleaseCount)
	}
	if groups[1].MaxSeeders != 150 {
		t.Errorf("expected Frieren max seeders 150, got %d", groups[1].MaxSeeders)
	}
	if len(groups[1].Qualities) != 2 {
		t.Errorf("expected 2 qualities for Frieren, got %v", groups[1].Qualities)
	}
}
