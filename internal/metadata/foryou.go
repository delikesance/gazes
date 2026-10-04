package metadata

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gazes/gazes/internal/kv"
)

const maxForYouSeeds = 5

const recommendationMedia = `
        id
        title { english romaji native }
        coverImage { extraLarge large }
        bannerImage
        startDate { year month day }
        nextAiringEpisode { episode }
        description
        genres
        episodes
        averageScore
        seasonYear
        status`

type aniListRecommendationsResponse struct {
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
	Data map[string]struct {
		Recommendations []struct {
			Rating              int               `json:"rating"`
			MediaRecommendation *aniListMediaItem `json:"mediaRecommendation"`
		} `json:"recommendations"`
	} `json:"data"`
}

// forYouQuery asks AniList, in one request, what viewers of each seed also recommend.
func forYouQuery(seeds []int) string {
	var b strings.Builder
	b.WriteString("query {")
	for i, id := range seeds {
		fmt.Fprintf(&b, "\n  r%d: Page(perPage: 12) { recommendations(mediaId: %d, sort: RATING_DESC) { rating mediaRecommendation {%s\n    } } }", i, id, recommendationMedia)
	}
	b.WriteString("\n}")
	return b.String()
}

// GetForYou blends recommendations derived from what the viewer watched (seeds, most recent
// first) with what is trending and all-time popular. Without seeds it is trending + popular.
func (s *AnimeCatalogService) GetForYou(ctx context.Context, seeds []int, taste Taste, page, perPage int) (*CatalogResponse, error) {
	if page <= 0 {
		page = 1
	}
	if perPage <= 0 || perPage > 50 {
		perPage = 24
	}
	if len(seeds) > maxForYouSeeds {
		seeds = seeds[:maxForYouSeeds]
	}
	key := make([]string, len(seeds))
	for i, id := range seeds {
		key[i] = fmt.Sprint(id)
	}
	cacheKey := fmt.Sprintf("foryou-%s-%s-%d-%d", strings.Join(key, ","), taste.Fingerprint(), page, perPage)
	return s.catalogC.Get(ctx, cacheKey, kv.Policy[CatalogResponse]{TTL: 30 * time.Minute}, func(ctx context.Context) (*CatalogResponse, error) {
		return s.buildForYou(ctx, seeds, taste, page, perPage)
	})
}

func (s *AnimeCatalogService) buildForYou(ctx context.Context, seeds []int, taste Taste, page, perPage int) (*CatalogResponse, error) {
	trending, trendErr := s.GetTrending(ctx, page, perPage)
	popular, popErr := s.GetPopular(ctx, page, perPage)
	if trendErr != nil && popErr != nil {
		return nil, trendErr
	}

	var personal []AnimeCatalogItem
	if len(seeds) > 0 && page == 1 {
		personal = s.recommendationsFor(ctx, seeds, taste, perPage)
	}

	// Never suggest what the viewer already watches: exclude every season of each seed's franchise.
	watched := map[int]bool{}
	for id := range taste.Dropped {
		watched[id] = true
	}
	for _, id := range seeds {
		watched[id] = true
		if franchise, err := s.GetFranchise(ctx, id); err == nil {
			watched[franchise.ID] = true
			for _, season := range franchise.Seasons {
				watched[season.ID] = true
			}
		}
	}

	var lists [3][]AnimeCatalogItem
	lists[0] = personal
	if trending != nil {
		lists[1] = trending.Items
	}
	if popular != nil {
		lists[2] = popular.Items
	}
	if !taste.Empty() {
		lists[1] = rankByTaste(lists[1], taste)
		lists[2] = rankByTaste(lists[2], taste)
	}
	// Two personal picks, then one trending and so on; popular fills the remaining slot of each round.
	pattern := []int{0, 0, 1, 0, 0, 2}
	if len(personal) == 0 {
		pattern = []int{1, 2}
	}
	seen := map[int]bool{}
	pos := [3]int{}
	items := make([]AnimeCatalogItem, 0, perPage)
	for len(items) < perPage {
		progressed := false
		for _, which := range pattern {
			for pos[which] < len(lists[which]) {
				item := lists[which][pos[which]]
				pos[which]++
				if watched[item.ID] || watched[item.MediaID] || seen[item.ID] {
					continue
				}
				seen[item.ID] = true
				items = append(items, item)
				progressed = true
				break
			}
			if len(items) == perPage {
				break
			}
		}
		if !progressed {
			break
		}
	}

	hasNext := (trending != nil && trending.HasNextPage) || (popular != nil && popular.HasNextPage)
	return &CatalogResponse{Page: page, PerPage: perPage, HasNextPage: hasNext, Items: items,
		Partial: (trending != nil && trending.Partial) || (popular != nil && popular.Partial)}, nil
}

// recommendationsFor ranks AniList recommendations for the seeds, favouring the most recently watched.
func (s *AnimeCatalogService) recommendationsFor(ctx context.Context, seeds []int, taste Taste, limit int) []AnimeCatalogItem {
	var parsed aniListRecommendationsResponse
	if err := s.anilist.post(ctx, forYouQuery(seeds), map[string]interface{}{}, &parsed); err != nil || len(parsed.Errors) > 0 {
		return nil
	}
	score := map[int]float64{}
	media := map[int]aniListMediaItem{}
	for i := range seeds {
		weight := 1.0 - 0.15*float64(i)
		for _, rec := range parsed.Data[fmt.Sprintf("r%d", i)].Recommendations {
			m := rec.MediaRecommendation
			if m == nil || m.ID == 0 || rec.Rating <= 0 {
				continue
			}
			score[m.ID] += float64(rec.Rating) * weight
			media[m.ID] = *m
		}
	}
	ids := make([]int, 0, len(score))
	for id := range score {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(a, b int) bool {
		if score[ids[a]] != score[ids[b]] {
			return score[ids[a]] > score[ids[b]]
		}
		return ids[a] < ids[b]
	})
	// Take a wider pool than we show, so the taste profile can promote what fits the viewer.
	if len(ids) > limit*2 {
		ids = ids[:limit*2]
	}
	if len(ids) == 0 {
		return nil
	}
	top := score[ids[0]]
	page := &aniListPageResponse{}
	for _, id := range ids {
		page.Data.Page.Media = append(page.Data.Page.Media, media[id])
	}
	res, err := s.formatPageResponse(ctx, page, map[string]interface{}{"page": 1, "perPage": limit * 2})
	if err != nil {
		return nil
	}
	items := res.Items
	if !taste.Empty() {
		// 60% community votes, 40% fit with the viewer's own taste.
		combined := func(item AnimeCatalogItem) float64 {
			return 0.6*score[item.MediaID]/top + 0.4*(taste.Score(item.Genres)+1)/2
		}
		sort.SliceStable(items, func(a, b int) bool { return combined(items[a]) > combined(items[b]) })
	}
	if len(items) > limit {
		items = items[:limit]
	}
	return items
}

// rankByTaste reorders a list so titles matching the viewer's taste rise, keeping the original
// order (popularity or trend) as the other half of the score.
func rankByTaste(items []AnimeCatalogItem, taste Taste) []AnimeCatalogItem {
	if len(items) < 2 {
		return items
	}
	ranked := append([]AnimeCatalogItem(nil), items...)
	original := map[int]int{}
	for i, item := range items {
		original[item.ID] = i
	}
	combined := func(item AnimeCatalogItem) float64 {
		return 0.5*(1-float64(original[item.ID])/float64(len(items))) + 0.5*(taste.Score(item.Genres)+1)/2
	}
	sort.SliceStable(ranked, func(a, b int) bool { return combined(ranked[a]) > combined(ranked[b]) })
	return ranked
}
