package metadata

import (
	"testing"
	"time"
)

func TestBuildTasteWeightsCompletionRecencyAndAbandonment(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	day := int64(86400)
	sessions := []TasteSession{
		{AnimeID: 1, Genres: []string{"Action", "Fantasy"}, WatchedSeconds: 1380, Duration: 1400, Completed: true, UpdatedAt: now.Unix() - day},
		{AnimeID: 2, Genres: []string{"Romance"}, WatchedSeconds: 200, Duration: 1400, UpdatedAt: now.Unix() - day},                      // tried, dropped
		{AnimeID: 3, Genres: []string{"Horror"}, WatchedSeconds: 30, Duration: 1400, UpdatedAt: now.Unix() - day},                        // accidental click
		{AnimeID: 4, Genres: []string{"Comedy"}, WatchedSeconds: 1400, Duration: 1400, Completed: true, UpdatedAt: now.Unix() - 400*day}, // long ago
	}
	taste := BuildTaste(sessions, now)
	if taste.Genres["action"] != 1 || taste.Genres["fantasy"] != 1 {
		t.Fatalf("recent completion must dominate: %v", taste.Genres)
	}
	if taste.Genres["romance"] >= 0 {
		t.Fatalf("an abandoned genre must weigh negatively: %v", taste.Genres)
	}
	if _, ok := taste.Genres["horror"]; ok {
		t.Fatalf("an accidental click must leave no trace: %v", taste.Genres)
	}
	if taste.Genres["comedy"] >= taste.Genres["action"]/4 {
		t.Fatalf("old watching must fade: %v", taste.Genres)
	}
	if !taste.Dropped[2] || taste.Dropped[1] {
		t.Fatalf("dropped: %v", taste.Dropped)
	}
	if len(taste.Seeds) != 2 || taste.Seeds[0] != 1 {
		t.Fatalf("seeds are the anime watched for real, newest first: %v", taste.Seeds)
	}
}

func TestTasteScoreAndFingerprint(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	taste := BuildTaste([]TasteSession{{AnimeID: 1, Genres: []string{"Action"}, WatchedSeconds: 1400, Duration: 1400, Completed: true, UpdatedAt: now.Unix()}}, now)
	if taste.Score([]string{"Action", "Drama"}) <= taste.Score([]string{"Drama"}) {
		t.Fatal("a liked genre must raise the score")
	}
	if taste.Score(nil) != 0 {
		t.Fatal("no genres, no opinion")
	}
	if (Taste{}).Fingerprint() != "none" || taste.Fingerprint() == "none" || taste.Fingerprint() != taste.Fingerprint() {
		t.Fatal("fingerprint must be stable and distinguish empty profiles")
	}
}
