package indexer

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/gazes/gazes/internal/metadata"
)

// AnimeGroup consolidates multiple torrent releases of the same anime show.
type AnimeGroup struct {
	ID            string                  `json:"id"`
	Title         string                  `json:"title"`
	AnimeDetails  *metadata.AnimeMetadata `json:"anime_details,omitempty"`
	Releases      []TorrentItem           `json:"releases"`
	ReleaseCount  int                     `json:"release_count"`
	MaxSeeders    int                     `json:"max_seeders"`
	BestRelease   *TorrentItem            `json:"best_release,omitempty"`
	Qualities     []string                `json:"qualities"`
	ReleaseGroups []string                `json:"release_groups"`
}

var (
	groupTagRegex = regexp.MustCompile(`^\[([a-zA-Z0-9_\-\.\s]+)\]`)
	quality1080p  = regexp.MustCompile(`(?i)\b1080p\b`)
	quality720p   = regexp.MustCompile(`(?i)\b720p\b`)
	quality480p   = regexp.MustCompile(`(?i)\b480p\b`)
	quality4k     = regexp.MustCompile(`(?i)\b(2160p|4k|uhd)\b`)
)

// ExtractReleaseGroup extracts the release group name (e.g. "SubsPlease") from a torrent title.
func ExtractReleaseGroup(title string) string {
	matches := groupTagRegex.FindStringSubmatch(strings.TrimSpace(title))
	if len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}
	return "Other"
}

// ExtractQuality returns standard resolution labels found in the torrent title.
func ExtractQuality(title string) string {
	title = strings.NewReplacer("_", " ", ".", " ").Replace(title)
	if quality4k.MatchString(title) {
		return "4K UHD"
	}
	if quality1080p.MatchString(title) {
		return "1080p"
	}
	if quality720p.MatchString(title) {
		return "720p"
	}
	if quality480p.MatchString(title) {
		return "480p"
	}
	return "Unknown"
}

// GroupTorrentsByAnime groups a flat list of torrent items into distinct anime collections.
func GroupTorrentsByAnime(items []TorrentItem) []AnimeGroup {
	if len(items) == 0 {
		return nil
	}

	groupMap := make(map[string]*AnimeGroup)
	groupOrder := make([]string, 0)

	for _, item := range items {
		if item.Seeders <= 0 {
			continue
		}
		// Determine grouping key
		var key, displayTitle string
		if item.AnimeDetails != nil && item.AnimeDetails.ID > 0 {
			key = fmt.Sprintf("anilist-%d", item.AnimeDetails.ID)
			displayTitle = item.AnimeDetails.DisplayTitle
		} else if item.AnimeDetails != nil && item.AnimeDetails.DisplayTitle != "" {
			key = strings.ToLower(item.AnimeDetails.DisplayTitle)
			displayTitle = item.AnimeDetails.DisplayTitle
		} else {
			clean := metadata.CleanAnimeTitle(item.Title)
			if clean == "" {
				clean = item.Title
			}
			key = strings.ToLower(clean)
			displayTitle = clean
		}

		ag, exists := groupMap[key]
		if !exists {
			ag = &AnimeGroup{
				ID:            key,
				Title:         displayTitle,
				AnimeDetails:  item.AnimeDetails,
				Releases:      make([]TorrentItem, 0),
				Qualities:     make([]string, 0),
				ReleaseGroups: make([]string, 0),
			}
			groupMap[key] = ag
			groupOrder = append(groupOrder, key)
		} else if ag.AnimeDetails == nil && item.AnimeDetails != nil {
			ag.AnimeDetails = item.AnimeDetails
			if item.AnimeDetails.DisplayTitle != "" {
				ag.Title = item.AnimeDetails.DisplayTitle
			}
		}

		ag.Releases = append(ag.Releases, item)
	}

	result := make([]AnimeGroup, 0, len(groupOrder))

	for _, key := range groupOrder {
		ag := groupMap[key]
		ag.ReleaseCount = len(ag.Releases)

		// Sort releases in group by seeders descending
		sort.Slice(ag.Releases, func(i, j int) bool {
			return ag.Releases[i].Seeders > ag.Releases[j].Seeders
		})

		// Compute metrics, qualities, release groups, and pick best release
		qualSet := make(map[string]struct{})
		groupSet := make(map[string]struct{})
		maxSeeds := 0
		var best1080p *TorrentItem

		for i := range ag.Releases {
			rel := &ag.Releases[i]
			if rel.Seeders > maxSeeds {
				maxSeeds = rel.Seeders
			}

			q := ExtractQuality(rel.Title)
			qualSet[q] = struct{}{}

			rg := ExtractReleaseGroup(rel.Title)
			if rg != "Other" {
				groupSet[rg] = struct{}{}
			}

			if best1080p == nil && (strings.Contains(q, "1080p") || strings.Contains(q, "4K")) {
				best1080p = rel
			}
		}

		ag.MaxSeeders = maxSeeds

		if best1080p != nil {
			ag.BestRelease = best1080p
		} else if len(ag.Releases) > 0 {
			ag.BestRelease = &ag.Releases[0]
		}

		for q := range qualSet {
			ag.Qualities = append(ag.Qualities, q)
		}
		sort.Strings(ag.Qualities)

		for rg := range groupSet {
			ag.ReleaseGroups = append(ag.ReleaseGroups, rg)
		}
		sort.Strings(ag.ReleaseGroups)

		result = append(result, *ag)
	}

	// Sort anime groups by max seeders descending
	sort.Slice(result, func(i, j int) bool {
		return result[i].MaxSeeders > result[j].MaxSeeders
	})

	return result
}
