package metadata

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gazes/gazes/internal/kv"
)

var regexpPartNumbers = regexp.MustCompile(`(?i)\b(?:part|partie|cour)\s*(\d+)(?:\s*(?:&|and|et)\s*(\d+))?\b`)

var regexpSeasonNumber = regexp.MustCompile(`(?i)\b(?:season|saison|s)\s*(\d+)\b|\b(\d+)(?:st|nd|rd|th)\s+season`)

// ReleaseSeasonNumber keeps renamed continuations independent of their franchise position.
// Naruto Shippuden is franchise season 2, but its releases start at S01.
func ReleaseSeasonNumber(title, canonicalTitle string, displayedSeason int) int {
	if match := regexpSeasonNumber.FindStringSubmatch(title); len(match) > 1 {
		for _, capture := range match[1:] {
			if capture != "" {
				var n int
				fmt.Sscanf(capture, "%d", &n)
				if n > 0 {
					return n
				}
			}
		}
	}
	if CleanFranchiseTitle(title) != CleanFranchiseTitle(canonicalTitle) {
		return 1
	}
	return max(1, displayedSeason)
}

type Franchise struct {
	ID          int           `json:"id"`
	Title       string        `json:"title"`
	PosterImage string        `json:"poster_image,omitempty"`
	Description string        `json:"description,omitempty"`
	Seasons     []AnimeSeason `json:"seasons"`
	Complete    bool          `json:"complete"`
	Warning     string        `json:"warning,omitempty"`
}

// GetFranchise walks continuity relations instead of guessing membership from titles.
// Side stories are included but do not bridge otherwise unrelated continuities.
func (s *AnimeCatalogService) GetFranchise(ctx context.Context, id int) (*Franchise, error) {
	// Only complete franchises are stored; a partial one must be retried, not frozen.
	policy := kv.Policy[Franchise]{TTLFor: func(f *Franchise) time.Duration {
		if f.Complete {
			return 2 * time.Hour
		}
		return 0
	}}
	return s.franchiseC.Get(ctx, strconv.Itoa(id), policy, func(ctx context.Context) (*Franchise, error) {
		return s.buildFranchise(ctx, id)
	})
}

func (s *AnimeCatalogService) buildFranchise(ctx context.Context, id int) (*Franchise, error) {
	root, err := s.GetAnimeDetailsWithEpisodes(ctx, id)
	if err != nil {
		return nil, err
	}
	type pending struct {
		id   int
		main bool
	}
	queue := []pending{{id, true}}
	seen := map[int]bool{}
	entries := map[int]*AnimeCatalogItem{}
	main := map[int]bool{}
	complete := true
	walkCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	stopped := false
	for len(queue) > 0 && !stopped {
		// Entries come in waves (everything related to the previous wave), so one batched lookup
		// per wave replaces one AniList call per entry.
		wave := queue
		queue = nil
		wanted := make([]int, 0, len(wave))
		for _, next := range wave {
			if !seen[next.id] && next.id != id {
				wanted = append(wanted, next.id)
			}
		}
		s.prefetchDetails(walkCtx, wanted)
		for _, next := range wave {
			if seen[next.id] && !(next.main && !main[next.id]) {
				if next.main {
					main[next.id] = true
				}
				continue
			}
			seen[next.id] = true
			if len(entries) >= 80 || walkCtx.Err() != nil {
				complete, stopped = false, true
				break
			}
			item := root
			if next.id != id {
				item, err = s.GetAnimeDetailsWithEpisodes(walkCtx, next.id)
				if err != nil {
					complete = false
					continue
				}
			}
			entries[item.ID] = item
			main[item.ID] = next.main
			if !next.main {
				continue
			}
			for _, rel := range item.Relations {
				switch rel.RelationType {
				case "PREQUEL", "SEQUEL", "PARENT":
					queue = append(queue, pending{rel.ID, true})
				case "SIDE_STORY", "ALTERNATIVE", "SPIN_OFF", "SUMMARY":
					queue = append(queue, pending{rel.ID, next.id == id && item.Format == "MOVIE" && (rel.Format == "TV" || rel.Format == "TV_SHORT")})
				}
			}
		}
	}
	seasons := make([]AnimeSeason, 0, len(entries))
	for _, item := range entries {
		group := "extras"
		if main[item.ID] && (item.Format == "TV" || item.Format == "TV_SHORT" || item.Format == "ONA") {
			group = "main"
		}
		if item.Format == "MOVIE" {
			group = "movies"
		}
		seasons = append(seasons, AnimeSeason{ID: item.ID, Title: item.DisplayTitle, Format: item.Format, Episodes: item.Episodes, PosterImage: item.PosterImage, SeasonYear: item.SeasonYear, StartDate: item.StartDate, Status: item.Status, Group: group, Aliases: item.Aliases})
	}
	sort.Slice(seasons, func(i, j int) bool {
		a, b := seasons[i], seasons[j]
		if (a.StartDate == "0000-00-00") != (b.StartDate == "0000-00-00") {
			return b.StartDate == "0000-00-00"
		}
		if a.StartDate != b.StartDate {
			return a.StartDate < b.StartDate
		}
		return a.ID < b.ID
	})
	// Respect prequel edges even when release dates are missing or misleading.
	ordered := make([]AnimeSeason, 0, len(seasons))
	remaining := append([]AnimeSeason(nil), seasons...)
	placed := map[int]bool{}
	for len(remaining) > 0 {
		progress := false
		for i, entry := range remaining {
			blocked := false
			if entry.Group == "main" {
				for _, rel := range entries[entry.ID].Relations {
					if rel.RelationType == "PREQUEL" && main[rel.ID] && entries[rel.ID] != nil && !placed[rel.ID] {
						blocked = true
					}
				}
			}
			if blocked {
				continue
			}
			ordered = append(ordered, entry)
			placed[entry.ID] = true
			remaining = append(remaining[:i], remaining[i+1:]...)
			progress = true
			break
		}
		if !progress {
			ordered = append(ordered, remaining...)
			break
		}
	}
	seasons = ordered
	canonical := root
	n := 0
	previousStem := ""
	canonicalSelected := false
	explicitSeason := regexpSeasonNumber
	for i := range seasons {
		entry := &seasons[i]
		if entry.Group == "main" {
			stem := regexp.MustCompile(`(?i)\s*(?:part|cour|partie)\s*\d+.*$`).ReplaceAllString(entry.Title, "")
			if stem != previousStem {
				n++
			}
			previousStem = stem
			if !canonicalSelected {
				canonical = entries[entry.ID]
				canonicalSelected = true
			}
			entry.SeasonNumber = n
			if match := explicitSeason.FindStringSubmatch(entry.Title); len(match) > 1 {
				var number int
				for _, capture := range match[1:] {
					if capture != "" {
						fmt.Sscanf(capture, "%d", &number)
						break
					}
				}
				if number > 0 {
					entry.SeasonNumber = number
					n = number
				}
			}
		} else {
			entry.SeasonName = entry.Title
		}
	}
	labelMainSeasons(seasons, canonical)
	for i := range seasons {
		seasons[i].ReleaseSeasonNumber = ReleaseSeasonNumber(seasons[i].Title, canonical.DisplayTitle, seasons[i].SeasonNumber)
	}
	// Absolute provider numbering continues across main seasons; it is unknown past a season
	// whose episode count is unknown.
	offset := 0
	for i := range seasons {
		if seasons[i].Group != "main" || offset < 0 {
			continue
		}
		seasons[i].EpisodeOffset = offset
		if seasons[i].Episodes > 0 {
			offset += seasons[i].Episodes
		} else {
			offset = -1
		}
	}
	f := &Franchise{ID: canonical.ID, Title: canonical.DisplayTitle, PosterImage: canonical.PosterImage, Description: canonical.Description, Seasons: seasons, Complete: complete}
	if !complete {
		f.Warning = "Certaines saisons n’ont pas pu être chargées. Réessayez."
	}
	if complete {
		// Every main entry resolves to the same franchise: store it under each of their ids.
		for entryID := range entries {
			if main[entryID] && entryID != id {
				s.franchiseC.Put(ctx, strconv.Itoa(entryID), f, 2*time.Hour)
			}
		}
	}
	return f, nil
}

// labelMainSeasons gives each main entry a short label that stays distinct when split parts
// share a SeasonNumber, and keeps the title only for renamed continuations.
func labelMainSeasons(seasons []AnimeSeason, canonical *AnimeCatalogItem) {
	titles := func(title string, aliases []string) []string {
		all := []string{title}
		for _, alias := range aliases {
			if alias != "" {
				all = append(all, alias)
			}
		}
		return all
	}
	canonicalTitles := map[string]bool{}
	for _, title := range titles(canonical.DisplayTitle, canonical.Aliases) {
		canonicalTitles[strings.ToLower(CleanFranchiseTitle(title))] = true
	}
	perSeason := map[int]int{}
	for _, entry := range seasons {
		if entry.Group == "main" {
			perSeason[entry.SeasonNumber]++
		}
	}
	lastPart := map[int]int{}
	for i := range seasons {
		entry := &seasons[i]
		if entry.Group != "main" {
			continue
		}
		label := fmt.Sprintf("Saison %d", entry.SeasonNumber)
		entryTitles := titles(entry.Title, entry.Aliases)
		if perSeason[entry.SeasonNumber] > 1 {
			// Without a part in its titles, an entry continues after the earlier parts of its season.
			highest := lastPart[entry.SeasonNumber] + 1
			part := fmt.Sprintf(" · Partie %d", highest)
			for _, title := range entryTitles {
				if match := regexpPartNumbers.FindStringSubmatch(title); match != nil {
					part = " · Partie " + match[1]
					highest, _ = strconv.Atoi(match[1])
					if match[2] != "" {
						part = fmt.Sprintf(" · Parties %s et %s", match[1], match[2])
						highest, _ = strconv.Atoi(match[2])
					}
					break
				}
			}
			lastPart[entry.SeasonNumber] = max(lastPart[entry.SeasonNumber], highest)
			label += part
		}
		known := false
		for _, title := range entryTitles {
			known = known || canonicalTitles[strings.ToLower(CleanFranchiseTitle(title))]
		}
		if !known {
			label += " — " + entry.Title
		}
		entry.SeasonName = label
	}
}
