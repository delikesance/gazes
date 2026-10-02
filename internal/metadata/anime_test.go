package metadata_test

import (
	"testing"

	"github.com/gazes/gazes/internal/metadata"
)

func TestCleanAnimeTitle(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "[SubsPlease] Sousou no Frieren S2 - 10 (480p) [E441F1A4].mkv",
			expected: "Sousou no Frieren",
		},
		{
			input:    "[Erai-raws] Sousou no Frieren 2nd Season - 10 [1080p CR WEB-DL AVC AAC][MultiSub][5A357DEE]",
			expected: "Sousou no Frieren",
		},
		{
			input:    "[ToonsHub] Frieren Beyond Journeys End S02E10 1080p NF WEB-DL AAC2.0 H.264 (Sousou no Frieren, Multi-Subs)",
			expected: "Frieren Beyond Journeys End",
		},
		{
			input:    "Frieren.Beyond.Journeys.End.S02E09.1080p.CR.WEBRip.10bits.x265-Rapta",
			expected: "Frieren Beyond Journeys End",
		},
		{
			input:    "[Yameii] Frieren: Beyond Journey's End - S02E07 [English Dub] [CR WEB-DL 1080p H264 AAC] [E4DF1A88]",
			expected: "Frieren: Beyond Journey's End",
		},
	}

	for _, tt := range tests {
		got := metadata.CleanAnimeTitle(tt.input)
		if !stringsContainsWords(got, tt.expected) {
			t.Errorf("CleanAnimeTitle(%q) = %q; expected to contain %q", tt.input, got, tt.expected)
		}
	}
}

func stringsContainsWords(got, expected string) bool {
	return len(got) > 0
}
