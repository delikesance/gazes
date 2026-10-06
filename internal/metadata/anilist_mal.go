package metadata

import "context"

// malBatch is how many MyAnimeList ids one AniList request resolves (AniList pages hold 50).
const malBatch = 50

const anilistMalQuery = `query ($ids: [Int], $perPage: Int) {
  Page(perPage: $perPage) {
    media(idMal_in: $ids, type: ANIME) { id idMal isAdult title { romaji english } }
  }
}`

// AniListByMalIDs maps MyAnimeList ids to AniList titles, 50 ids per governed request, at most max
// ids in all (Truncated when more were given). Adult titles are dropped and each title kept once.
func (s *AnimeCatalogService) AniListByMalIDs(ctx context.Context, malIDs []int, max int) (*AniListList, error) {
	out := &AniListList{Entries: []AniListListEntry{}}
	valid := make([]int, 0, len(malIDs))
	seenMal := map[int]bool{}
	for _, id := range malIDs {
		if id > 0 && !seenMal[id] {
			seenMal[id] = true
			valid = append(valid, id)
		}
	}
	if len(valid) > max {
		valid, out.Truncated = valid[:max], true
	}
	seen := map[int]bool{}
	for start := 0; start < len(valid); start += malBatch {
		batch := valid[start:min(start+malBatch, len(valid))]
		var parsed struct {
			Data struct {
				Page struct {
					Media []struct {
						ID      int  `json:"id"`
						IsAdult bool `json:"isAdult"`
						Title   struct {
							Romaji  string `json:"romaji"`
							English string `json:"english"`
						} `json:"title"`
					} `json:"media"`
				} `json:"Page"`
			} `json:"data"`
		}
		if err := s.anilist.post(ctx, anilistMalQuery, map[string]any{"ids": batch, "perPage": malBatch}, &parsed); err != nil {
			return nil, err
		}
		for _, m := range parsed.Data.Page.Media {
			if m.IsAdult || seen[m.ID] {
				continue
			}
			seen[m.ID] = true
			title := m.Title.English
			if title == "" {
				title = m.Title.Romaji
			}
			out.Entries = append(out.Entries, AniListListEntry{ID: m.ID, Title: title})
		}
	}
	return out, nil
}
