package metadata

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

var seasonalQuery = strings.Replace(
	strings.Replace(trendingQuery, "query ($page: Int, $perPage: Int)", "query ($page: Int, $perPage: Int, $season: MediaSeason, $seasonYear: Int)", 1),
	"media(sort: TRENDING_DESC, type: ANIME)", "media(sort: TRENDING_DESC, type: ANIME, season: $season, seasonYear: $seasonYear)", 1,
)

func catalogSeason(now time.Time) (string, int) {
	seasons := [...]string{"WINTER", "SPRING", "SUMMER", "FALL"}
	return seasons[(int(now.Month())-1)/3], now.Year()
}

// GetCurrentSeason keeps individual releases instead of collapsing a franchise's seasons.
func (s *AnimeCatalogService) GetCurrentSeason(ctx context.Context, page, perPage int) (*CatalogResponse, error) {
	if page <= 0 {
		page = 1
	}
	if perPage <= 0 || perPage > 50 {
		perPage = 24
	}
	season, year := catalogSeason(time.Now().UTC())
	key := fmt.Sprintf("seasonal-%s-%d-%d-%d", season, year, page, perPage)
	return s.catalogC.Get(ctx, key, catalogPolicy(30*time.Minute), func(ctx context.Context) (*CatalogResponse, error) {
		result, err := s.doGraphQLPageQuery(ctx, seasonalQuery, map[string]interface{}{"page": page, "perPage": perPage, "season": season, "seasonYear": year})
		if err != nil {
			return nil, err
		}
		sortSeasonalReleases(result.Items)
		result.Season = season
		result.SeasonYear = year
		return result, nil
	})
}

// Complete premiere dates sort first; partial or missing dates stay at the end.
func sortSeasonalReleases(items []AnimeCatalogItem) {
	sort.SliceStable(items, func(i, j int) bool {
		a, errA := time.Parse("2006-01-02", items[i].StartDate)
		b, errB := time.Parse("2006-01-02", items[j].StartDate)
		knownA := errA == nil && a.Year() > 0
		knownB := errB == nil && b.Year() > 0
		if knownA != knownB {
			return knownA
		}
		return knownA && a.Before(b)
	})
}
