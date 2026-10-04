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

func withStreamingEpisodes(m aniListMediaItem, titles ...string) aniListMediaItem {
	for _, title := range titles {
		m.StreamingEpisodes = append(m.StreamingEpisodes, struct {
			Title     string `json:"title"`
			Thumbnail string `json:"thumbnail"`
			URL       string `json:"url"`
			Site      string `json:"site"`
		}{Title: title})
	}
	return m
}

func TestEpisodeListSkipsFractionalProviderEpisodes(t *testing.T) {
	m := withStreamingEpisodes(aniListMediaItem{ID: 1, Episodes: 3, Status: "FINISHED"}, "Episode 2 - Real", "Episode 2.5 - Recap")
	item := formatCatalogItem(&m, true)
	if len(item.EpisodeList) != 3 || item.EpisodeList[1].Title != "Episode 2 - Real" {
		t.Fatalf("a .5 special must not replace episode 2: %+v", item.EpisodeList)
	}
}

// AniList attaches Slime's season 3 Crunchyroll listing (absolute numbers 48.5-72) to every
// season of the franchise: only season 3, which follows 24+12+12 episodes, owns it.
func TestAbsoluteProviderEpisodesBelongToTheirFranchiseSeason(t *testing.T) {
	titles := []string{"Episode 48.5 - Digression"}
	for n := 49; n <= 72; n++ {
		titles = append(titles, "Episode "+strconv.Itoa(n)+" - Title "+strconv.Itoa(n))
	}
	s := NewAnimeCatalogService(nil)
	entries := []struct {
		id, episodes int
		title, date  string
	}{
		{1, 24, "Slime", "2018-10-02"},
		{2, 12, "Slime Season 2", "2021-01-12"},
		{3, 12, "Slime Season 2 Part 2", "2021-07-06"},
		{4, 24, "Slime Season 3", "2024-04-05"},
	}
	for i, e := range entries {
		m := withStreamingEpisodes(aniListMediaItem{ID: e.id, Episodes: e.episodes, Status: "FINISHED"}, titles...)
		item := formatCatalogItem(&m, true)
		for _, ep := range item.EpisodeList {
			if ep.Title != "Épisode "+strconv.Itoa(ep.EpisodeNumber) {
				t.Fatalf("season %d: absolute provider episode leaked before franchise placement: %+v", e.id, ep)
			}
		}
		item.DisplayTitle, item.Format, item.StartDate = e.title, "TV", e.date
		if i > 0 {
			item.Relations = append(item.Relations, AnimeRelation{ID: entries[i-1].id, RelationType: "PREQUEL"})
		}
		if i < len(entries)-1 {
			item.Relations = append(item.Relations, AnimeRelation{ID: entries[i+1].id, RelationType: "SEQUEL"})
		}
		s.detailC.Put(context.Background(), strconv.Itoa(e.id), &item, time.Hour)
	}
	f, err := s.GetFranchise(context.Background(), 1)
	if err != nil || len(f.Seasons) != 4 {
		t.Fatalf("%+v %v", f, err)
	}
	for _, season := range f.Seasons {
		cached, err := s.GetAnimeDetailsWithEpisodes(context.Background(), season.ID)
		if err != nil {
			t.Fatal(err)
		}
		item := cached.InSeason(season.EpisodeOffset)
		if len(item.EpisodeList) != season.Episodes {
			t.Fatalf("season %d: got %d episodes, want %d", season.ID, len(item.EpisodeList), season.Episodes)
		}
		for i, ep := range item.EpisodeList {
			want := "Épisode " + strconv.Itoa(i+1)
			if season.ID == 4 {
				want = "Episode " + strconv.Itoa(i+49) + " - Title " + strconv.Itoa(i+49)
			}
			if ep.EpisodeNumber != i+1 || ep.Title != want {
				t.Fatalf("season %d episode %d: got %q, want %q", season.ID, ep.EpisodeNumber, ep.Title, want)
			}
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
	if f.Seasons[0].SeasonName != "Saison 1" || f.Seasons[1].SeasonName != "Saison 2 — Naruto Shippuden" {
		t.Fatalf("labels: %q, %q", f.Seasons[0].SeasonName, f.Seasons[1].SeasonName)
	}
}

func TestMainSeasonLabelsAreDistinctAndShort(t *testing.T) {
	s := NewAnimeCatalogService(nil)
	type row struct {
		id           int
		title, alias string
		start, label string
		season       int
	}
	rows := []row{
		{101280, "That Time I Got Reincarnated as a Slime", "Tensei Shitara Slime Datta Ken", "2018-10-02", "Saison 1", 1},
		{108511, "That Time I Got Reincarnated as a Slime Season 2", "Tensei Shitara Slime Datta Ken 2nd Season", "2021-01-12", "Saison 2 · Partie 1", 2},
		{116742, "That Time I Got Reincarnated as a Slime Season 2 Part 2", "Tensei Shitara Slime Datta Ken 2nd Season Part 2", "2021-07-06", "Saison 2 · Partie 2", 2},
		{156822, "That Time I Got Reincarnated as a Slime Season 3", "Tensei Shitara Slime Datta Ken 3rd Season", "2024-04-05", "Saison 3", 3},
		{182205, "That Time I Got Reincarnated as a Slime Season 4", "Tensei Shitara Slime Datta Ken 4th Season Part 1 & 2", "2026-04-01", "Saison 4 · Parties 1 et 2", 4},
		{217331, "Tensei Shitara Slime Datta Ken 4th Season Part 3", "Tensura 4 Part 3", "2026-10-01", "Saison 4 · Partie 3", 4},
	}
	for i, r := range rows {
		item := &AnimeCatalogItem{ID: r.id, DisplayTitle: r.title, Aliases: []string{"", r.alias}, Format: "TV", StartDate: r.start}
		if i > 0 {
			item.Relations = append(item.Relations, AnimeRelation{ID: rows[i-1].id, RelationType: "PREQUEL"})
		}
		if i < len(rows)-1 {
			item.Relations = append(item.Relations, AnimeRelation{ID: rows[i+1].id, RelationType: "SEQUEL"})
		}
		s.detailC.Put(context.Background(), strconv.Itoa(r.id), item, time.Hour)
	}
	f, err := s.GetFranchise(context.Background(), 101280)
	if err != nil || len(f.Seasons) != len(rows) {
		t.Fatalf("%+v %v", f, err)
	}
	for i, r := range rows {
		got := f.Seasons[i]
		if got.ID != r.id || got.SeasonName != r.label || got.SeasonNumber != r.season {
			t.Errorf("%d: got %q (season %d), want %q (season %d)", r.id, got.SeasonName, got.SeasonNumber, r.label, r.season)
		}
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
