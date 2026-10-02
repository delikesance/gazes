package metadata

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"
	"time"
)

func TestFranchiseContinuityAndExtras(t *testing.T) {
	s := NewAnimeCatalogService(nil)
	items := []AnimeCatalogItem{
		{ID: 1, DisplayTitle: "Original", Format: "TV", StartDate: "2020-01-01", Relations: []AnimeRelation{{ID: 2, RelationType: "SEQUEL"}, {ID: 4, RelationType: "SIDE_STORY"}}},
		{ID: 2, DisplayTitle: "Entirely Different Title", Format: "TV", StartDate: "2021-01-01", Relations: []AnimeRelation{{ID: 1, RelationType: "PREQUEL"}, {ID: 3, RelationType: "SEQUEL"}}},
		{ID: 3, DisplayTitle: "Final Entry", Format: "TV", StartDate: "2022-01-01", Relations: []AnimeRelation{{ID: 2, RelationType: "PREQUEL"}}},
		{ID: 4, DisplayTitle: "Movie", Format: "MOVIE", StartDate: "2021-05-01", Relations: []AnimeRelation{{ID: 99, RelationType: "SEQUEL"}}},
	}
	for i := range items {
		s.detailC.Put(context.Background(), strconv.Itoa(items[i].ID), &items[i], time.Hour)
	}
	f, err := s.GetFranchise(context.Background(), 2)
	if err != nil || !f.Complete || f.ID != 1 || len(f.Seasons) != 4 {
		t.Fatalf("%+v %v", f, err)
	}
	if f.Seasons[2].Group != "movies" {
		t.Fatal("movie must stay separate")
	}
	again, err := s.GetFranchise(context.Background(), 3)
	if err != nil || again.ID != 1 {
		t.Fatal("later season must canonicalize")
	}
}
func TestEpisodeGapsAndDuplicateListings(t *testing.T) {
	m := aniListMediaItem{ID: 1, Episodes: 4, Status: "FINISHED"}
	m.StreamingEpisodes = append(m.StreamingEpisodes, struct {
		Title     string `json:"title"`
		Thumbnail string `json:"thumbnail"`
		URL       string `json:"url"`
		Site      string `json:"site"`
	}{Title: "Episode 2"})
	m.StreamingEpisodes = append(m.StreamingEpisodes, m.StreamingEpisodes[0])
	item := formatCatalogItem(&m, true)
	if len(item.EpisodeList) != 4 {
		t.Fatalf("got %d episodes", len(item.EpisodeList))
	}
	for i, ep := range item.EpisodeList {
		if ep.EpisodeNumber != i+1 {
			t.Fatal("episodes not complete and sorted")
		}
	}
}

func TestSplitPartsKeepCanonicalSeason(t *testing.T) {
	s := NewAnimeCatalogService(nil)
	a := &AnimeCatalogItem{ID: 1, DisplayTitle: "Example Season 1", Format: "TV", StartDate: "2020-01-01", Relations: []AnimeRelation{{ID: 2, RelationType: "SEQUEL"}}}
	b := &AnimeCatalogItem{ID: 2, DisplayTitle: "Example Season 1 Part 2", Format: "TV", StartDate: "2020-06-01", Relations: []AnimeRelation{{ID: 1, RelationType: "PREQUEL"}}}
	s.detailC.Put(context.Background(), "1", a, time.Hour)
	s.detailC.Put(context.Background(), "2", b, time.Hour)
	f, err := s.GetFranchise(context.Background(), 2)
	if err != nil || f.ID != 1 || len(f.Seasons) != 2 || f.Seasons[1].SeasonNumber != 1 {
		t.Fatalf("split season: %+v %v", f, err)
	}
}

func TestRenamedSequelsUseTheirOwnReleaseSeason(t *testing.T) {
	for _, tt := range []struct {
		title, root     string
		displayed, want int
	}{
		{"Naruto Shippuden", "Naruto", 2, 1},
		{"Naruto: Shippuden", "Naruto", 2, 1},
		{"Dragon Ball Z", "Dragon Ball", 2, 1},
		{"Dragon Ball Super", "Dragon Ball", 3, 1},
		{"Example Season 2", "Example", 2, 2},
		{"Example 3rd Season", "Example", 3, 3},
		{"Example Season 2 Part 2", "Example", 2, 2},
	} {
		if got := ReleaseSeasonNumber(tt.title, tt.root, tt.displayed); got != tt.want {
			t.Errorf("%s: got release season %d, want %d", tt.title, got, tt.want)
		}
	}
	s := NewAnimeCatalogService(nil)
	a := &AnimeCatalogItem{ID: 20, DisplayTitle: "Naruto", Format: "TV", StartDate: "2002-10-03", Relations: []AnimeRelation{{ID: 1735, RelationType: "SEQUEL"}}}
	b := &AnimeCatalogItem{ID: 1735, DisplayTitle: "Naruto Shippuden", Format: "TV", StartDate: "2007-02-15", Relations: []AnimeRelation{{ID: 20, RelationType: "PREQUEL"}}}
	s.detailC.Put(context.Background(), "20", a, time.Hour)
	s.detailC.Put(context.Background(), "1735", b, time.Hour)
	f, err := s.GetFranchise(context.Background(), 1735)
	if err != nil || f.ID != 20 || len(f.Seasons) != 2 {
		t.Fatalf("%+v %v", f, err)
	}
	if f.Seasons[1].SeasonNumber != 2 || f.Seasons[1].ReleaseSeasonNumber != 1 {
		t.Fatalf("display/release numbering: %+v", f.Seasons[1])
	}
}

func TestEpisodeSchedulePreservesConfirmedDates(t *testing.T) {
	var m aniListMediaItem
	if err := json.Unmarshal([]byte(`{"id":1,"episodes":3,"status":"RELEASING","nextAiringEpisode":{"episode":2},"airingSchedule":{"nodes":[{"episode":2,"airingAt":2000000000}]},"streamingEpisodes":[{"title":"Episode 1","thumbnail":"https://example.com/episode.jpg"}]}`), &m); err != nil {
		t.Fatal(err)
	}
	item := formatCatalogItem(&m, true)
	if len(item.EpisodeList) != 3 {
		t.Fatalf("unexpected episodes: %+v", item.EpisodeList)
	}
	if item.EpisodeList[0].Thumbnail == "" || item.EpisodeList[0].Upcoming {
		t.Fatal("released preview lost")
	}
	if item.EpisodeList[1].AiringAt != 2000000000 || !item.EpisodeList[1].Upcoming {
		t.Fatal("confirmed schedule lost")
	}
	if item.EpisodeList[2].AiringAt != 0 {
		t.Fatal("invented an unconfirmed date")
	}
}
