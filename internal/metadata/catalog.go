package metadata

import (
	"context"
	"fmt"
	"github.com/gazes/gazes/internal/kv"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// EpisodeInfo represents single episode metadata.
type EpisodeInfo struct {
	AiringAt      int64  `json:"airing_at,omitempty"`
	Upcoming      bool   `json:"upcoming,omitempty"`
	EpisodeNumber int    `json:"episode_number"`
	Title         string `json:"title"`
	Thumbnail     string `json:"thumbnail,omitempty"`
	URL           string `json:"url,omitempty"`
	Site          string `json:"site,omitempty"`
	Summary       string `json:"summary,omitempty"`
}

// AnimeSeason represents a classified season, prequel, sequel, or movie within an anime franchise.
type AnimeSeason struct {
	ReleaseSeasonNumber int      `json:"release_season_number"`
	ID                  int      `json:"id"`
	Title               string   `json:"title"`
	SeasonName          string   `json:"season_name"`
	Format              string   `json:"format,omitempty"`
	Episodes            int      `json:"episodes,omitempty"`
	PosterImage         string   `json:"poster_image,omitempty"`
	SeasonYear          int      `json:"season_year,omitempty"`
	Status              string   `json:"status,omitempty"`
	Group               string   `json:"group"`
	SeasonNumber        int      `json:"season_number,omitempty"`
	StartDate           string   `json:"start_date,omitempty"`
	IsCurrent           bool     `json:"is_current"`
	Aliases             []string `json:"aliases,omitempty"`
	EpisodeOffset       int      `json:"episode_offset,omitempty"` // main-continuity episodes before this season
}

// AnimeRelation represents a connected anime season, prequel, sequel, movie, or spin-off.
type AnimeRelation struct {
	ID           int    `json:"id"`
	TitleEnglish string `json:"title_english,omitempty"`
	TitleRomaji  string `json:"title_romaji,omitempty"`
	DisplayTitle string `json:"display_title"`
	Format       string `json:"format,omitempty"`
	RelationType string `json:"relation_type"` // PREQUEL, SEQUEL, SIDE_STORY, ALTERNATIVE, PARENT, SPIN_OFF
	SeasonYear   int    `json:"season_year,omitempty"`
	Season       string `json:"season,omitempty"`
	Episodes     int    `json:"episodes,omitempty"`
	PosterImage  string `json:"poster_image,omitempty"`
	Status       string `json:"status,omitempty"`
}

// AnimeCatalogItem represents full anime metadata with episode listings and relations.
type AnimeCatalogItem struct {
	Format            string          `json:"format,omitempty"`
	StartDate         string          `json:"start_date,omitempty"`
	Aliases           []string        `json:"aliases,omitempty"`
	ID                int             `json:"id"`
	MediaTitle        string          `json:"media_title,omitempty"`
	MediaID           int             `json:"media_id,omitempty"`
	MediaPosterImage  string          `json:"media_poster_image,omitempty"`
	TitleEnglish      string          `json:"title_english,omitempty"`
	TitleRomaji       string          `json:"title_romaji,omitempty"`
	TitleNative       string          `json:"title_native,omitempty"`
	DisplayTitle      string          `json:"display_title"`
	FranchiseTitle    string          `json:"franchise_title,omitempty"`
	PosterImage       string          `json:"poster_image,omitempty"`
	BannerImage       string          `json:"banner_image,omitempty"`
	Description       string          `json:"description,omitempty"`
	Genres            []string        `json:"genres,omitempty"`
	Episodes          int             `json:"episodes,omitempty"`
	AvailableEpisodes *int            `json:"available_episodes,omitempty"`
	AverageScore      float64         `json:"average_score,omitempty"`
	SeasonYear        int             `json:"season_year,omitempty"`
	Status            string          `json:"status,omitempty"`
	EpisodeList       []EpisodeInfo   `json:"episode_list,omitempty"`
	AbsoluteEpisodes  []EpisodeInfo   `json:"absolute_episodes,omitempty"`
	Relations         []AnimeRelation `json:"relations,omitempty"`
	Seasons           []AnimeSeason   `json:"seasons,omitempty"`
}

type CatalogResponse struct {
	Season      string             `json:"season,omitempty"`
	SeasonYear  int                `json:"season_year,omitempty"`
	Page        int                `json:"page"`
	PerPage     int                `json:"per_page"`
	Partial     bool               `json:"partial,omitempty"`
	Warning     string             `json:"warning,omitempty"`
	HasNextPage bool               `json:"has_next_page"`
	Total       int                `json:"total,omitempty"`
	Items       []AnimeCatalogItem `json:"items"`
}

// CatalogProvider manages discovery and catalog queries.
type CatalogProvider interface {
	GetTrending(ctx context.Context, page, perPage int) (*CatalogResponse, error)
	GetPopular(ctx context.Context, page, perPage int) (*CatalogResponse, error)
	SearchCatalog(ctx context.Context, query string, genre string, page, perPage int) (*CatalogResponse, error)
	GetAnimeDetailsWithEpisodes(ctx context.Context, id int) (*AnimeCatalogItem, error)
}

// AnimeCatalogService implements CatalogProvider using AniList GraphQL API with local caching.
type AnimeCatalogService struct {
	anilist    *anilistClient
	catalogC   *kv.Cache[CatalogResponse]
	detailC    *kv.Cache[AnimeCatalogItem]
	franchiseC *kv.Cache[Franchise]
	scheduleC  *kv.Cache[ScheduleResponse]
}

// NewAnimeCatalogService creates a catalog service with process-local caches; call SetRedis to
// share caches, rate limiting and single-flight across instances.
func NewAnimeCatalogService(client *http.Client) *AnimeCatalogService {
	if client == nil {
		client = &http.Client{Timeout: 8 * time.Second}
	}
	s := &AnimeCatalogService{anilist: newAnilistClient(client, nil)}
	s.initCaches(nil)
	return s
}

// SetRedis moves the caches and the AniList governor to Redis. Call it before serving traffic.
func (s *AnimeCatalogService) SetRedis(c *kv.Client) {
	s.anilist = newAnilistClient(s.anilist.http, c)
	s.initCaches(c)
}

func (s *AnimeCatalogService) initCaches(c *kv.Client) {
	s.catalogC = kv.NewCache[CatalogResponse](c, "catalog", kv.CacheOptions{L1Max: 256})
	s.detailC = kv.NewCache[AnimeCatalogItem](c, "detail:v2", kv.CacheOptions{L1Max: 512})
	s.franchiseC = kv.NewCache[Franchise](c, "franchise:v2", kv.CacheOptions{L1Max: 512, FetchTimeout: 45 * time.Second})
	s.scheduleC = kv.NewCache[ScheduleResponse](c, "schedule", kv.CacheOptions{L1Max: 64})
}

const trendingQuery = `
query ($page: Int, $perPage: Int) {
  Page(page: $page, perPage: $perPage) {
    pageInfo {
      total
      hasNextPage
    }
    media(sort: TRENDING_DESC, type: ANIME) {
      id
      title {
        english
        romaji
        native
      }
      coverImage {
        extraLarge
        large
      }
      bannerImage
      startDate { year month day }
      nextAiringEpisode { episode }
      description
      genres
      episodes
      averageScore
      seasonYear
      status
    }
  }
}
`

const popularQuery = `
query ($page: Int, $perPage: Int) {
  Page(page: $page, perPage: $perPage) {
    pageInfo {
      total
      hasNextPage
    }
    media(sort: POPULARITY_DESC, type: ANIME) {
      id
      title {
        english
        romaji
        native
      }
      coverImage {
        extraLarge
        large
      }
      bannerImage
      description
      genres
      episodes
      averageScore
      seasonYear
      status
    }
  }
}
`

const searchQuery = `
query ($search: String, $genreIn: [String], $genreNotIn: [String], $tagIn: [String], $tagNotIn: [String], $page: Int, $perPage: Int) {
  Page(page: $page, perPage: $perPage) {
    pageInfo {
      total
      hasNextPage
    }
    media(search: $search, genre_in: $genreIn, genre_not_in: $genreNotIn, tag_in: $tagIn, tag_not_in: $tagNotIn, minimumTagRank: 60, sort: POPULARITY_DESC, type: ANIME) {
      id
      title {
        english
        romaji
        native
      }
      format
      startDate { year month day }
      coverImage {
        extraLarge
        large
      }
      bannerImage
      description
      genres
      episodes
      averageScore
      seasonYear
      status
      relations {
        edges {
          relationType
          node { id format }
        }
      }
    }
  }
}
`

// detailFields is everything a franchise entry needs; one lookup or a whole id_in batch selects it.
const detailFields = `
    id
    title {
      english
      romaji
      native
    }
    format
    synonyms
    startDate { year month day }
    nextAiringEpisode { episode }
    airingSchedule(perPage: 25, notYetAired: true) { nodes { episode airingAt } }
    season
    seasonYear
    coverImage {
      extraLarge
      large
    }
    bannerImage
    description
    genres
    episodes
    averageScore
    status
    relations {
      edges {
        relationType
        node {
          id
          title {
            english
            romaji
            native
          }
          format
          status
          season
          seasonYear
          episodes
          coverImage {
            extraLarge
            large
          }
          relations {
            edges {
              relationType
              node {
                id
                title {
                  english
                  romaji
                  native
                }
                format
                status
                season
                seasonYear
                episodes
                coverImage {
                  extraLarge
                  large
                }
              }
            }
          }
        }
      }
    }
    streamingEpisodes {
      title
      thumbnail
      url
      site
    }
`

const animeDetailWithEpisodesQuery = `
query ($id: Int) {
  Media(id: $id, type: ANIME) {` + detailFields + `  }
}
`

// animeDetailBatchQuery loads several entries in one call, so a franchise walk spends one AniList
// request per wave of relations instead of one per entry.
const animeDetailBatchQuery = `
query ($ids: [Int], $perPage: Int) {
  Page(perPage: $perPage) {
    media(id_in: $ids, type: ANIME) {` + detailFields + `    }
  }
}
`

type aniListRelationEdge struct {
	RelationType string `json:"relationType"`
	Node         struct {
		ID    int `json:"id"`
		Title struct {
			English string `json:"english"`
			Romaji  string `json:"romaji"`
			Native  string `json:"native"`
		} `json:"title"`
		Format     string `json:"format"`
		Status     string `json:"status"`
		Season     string `json:"season"`
		SeasonYear int    `json:"seasonYear"`
		Episodes   int    `json:"episodes"`
		CoverImage struct {
			ExtraLarge string `json:"extraLarge"`
			Large      string `json:"large"`
		} `json:"coverImage"`
		Relations struct {
			Edges []struct {
				RelationType string `json:"relationType"`
				Node         struct {
					ID    int `json:"id"`
					Title struct {
						English string `json:"english"`
						Romaji  string `json:"romaji"`
						Native  string `json:"native"`
					} `json:"title"`
					Format     string `json:"format"`
					Status     string `json:"status"`
					Season     string `json:"season"`
					SeasonYear int    `json:"seasonYear"`
					Episodes   int    `json:"episodes"`
					CoverImage struct {
						ExtraLarge string `json:"extraLarge"`
						Large      string `json:"large"`
					} `json:"coverImage"`
				} `json:"node"`
			} `json:"edges"`
		} `json:"relations"`
	} `json:"node"`
}

var seasonSuffixRegexes = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\s*[-:–—]?\s*(?:the\s+)?(?:final\s+season|final\s+act|the\s+final\s+chapters?)(?:\s*[-:–—]?\s*(?:part|cour)\s*\d+)?.*$`),
	regexp.MustCompile(`(?i)\s*[-:–—]?\s*season\s*\d+(?:\s*[-:–—]?\s*(?:part|cour)\s*\d+)?.*$`),
	regexp.MustCompile(`(?i)\s*[-:–—]?\s*\d+(?:st|nd|rd|th)\s+season(?:\s*[-:–—]?\s*(?:part|cour)\s*\d+)?.*$`),
	regexp.MustCompile(`(?i)\s*[-:–—]?\s*s\d{1,2}(?:\s*[-:–—]?\s*(?:part|cour)\s*\d+)?.*$`),
	regexp.MustCompile(`(?i)\s*[-:–—]?\s*(?:part|cour)\s*\d+.*$`),
	regexp.MustCompile(`(?i)\s*[-:–—]?\s*(?:2nd|3rd|4th|5th|6th|7th|8th)\s+season.*$`),
	regexp.MustCompile(`(?i)\s*[-:–—]?\s*(?:Hashira Training Arc|Entertainment District Arc|Mugen Train Arc|Swordsmith Village Arc|The Culling Game).*$`),
}

// CleanFranchiseTitle strips season numbers and sub-arcs to give the canonical franchise name.
func CleanFranchiseTitle(title string) string {
	cleaned := strings.TrimSpace(title)
	for _, re := range seasonSuffixRegexes {
		cleaned = re.ReplaceAllString(cleaned, "")
	}
	cleaned = strings.TrimSpace(cleaned)
	cleaned = strings.TrimRight(cleaned, "-:–— ")
	if cleaned == "" {
		return strings.TrimSpace(title)
	}
	return cleaned
}

func isFranchiseMatch(baseEnglish, baseRomaji, candEnglish, candRomaji string) bool {
	cleanBaseEng := strings.ToLower(CleanFranchiseTitle(baseEnglish))
	cleanBaseRom := strings.ToLower(CleanFranchiseTitle(baseRomaji))
	cleanCandEng := strings.ToLower(CleanFranchiseTitle(candEnglish))
	cleanCandRom := strings.ToLower(CleanFranchiseTitle(candRomaji))

	// Direct match of cleaned franchise title
	if cleanBaseEng != "" && cleanCandEng != "" && cleanBaseEng == cleanCandEng {
		return true
	}
	if cleanBaseRom != "" && cleanCandRom != "" && cleanBaseRom == cleanCandRom {
		return true
	}
	if cleanBaseEng != "" && cleanCandRom != "" && cleanBaseEng == cleanCandRom {
		return true
	}
	if cleanBaseRom != "" && cleanCandEng != "" && cleanBaseRom == cleanCandEng {
		return true
	}

	extractKey := func(s string) string {
		s = strings.TrimSpace(s)
		words := strings.Fields(s)
		if len(words) == 0 {
			return ""
		}
		if len(words) >= 2 {
			return words[0] + " " + words[1]
		}
		return words[0]
	}

	baseKeyEng := extractKey(cleanBaseEng)
	baseKeyRom := extractKey(cleanBaseRom)

	if len(baseKeyEng) >= 4 {
		if strings.Contains(cleanCandEng, baseKeyEng) || strings.Contains(cleanCandRom, baseKeyEng) {
			return true
		}
	}
	if len(baseKeyRom) >= 4 {
		if strings.Contains(cleanCandEng, baseKeyRom) || strings.Contains(cleanCandRom, baseKeyRom) {
			return true
		}
	}

	return false
}

func isMainTVSeason(format, title string) bool {
	f := strings.ToUpper(strings.TrimSpace(format))
	t := strings.ToLower(title)
	if f != "TV" {
		return false
	}
	if strings.Contains(t, "break time") || strings.Contains(t, "petit") || strings.Contains(t, "chibi") || strings.Contains(t, "mini") {
		return false
	}
	return true
}

func deriveSeasonLabel(title string, format string, seasonIndex int) string {
	titleLower := strings.ToLower(title)

	// Explicit season number mentions
	if strings.Contains(titleLower, "season 4") || strings.Contains(titleLower, "4th season") || strings.Contains(titleLower, "s4") {
		return "Saison 4"
	}
	if strings.Contains(titleLower, "season 3") || strings.Contains(titleLower, "3rd season") || strings.Contains(titleLower, "s3") {
		return "Saison 3"
	}
	if strings.Contains(titleLower, "season 2 part 2") || strings.Contains(titleLower, "2nd season part 2") || strings.Contains(titleLower, "season 2 - part 2") {
		return "Saison 2 (Partie 2)"
	}
	if strings.Contains(titleLower, "season 2") || strings.Contains(titleLower, "2nd season") || strings.Contains(titleLower, "s2") {
		return "Saison 2"
	}
	if strings.Contains(titleLower, "final season part 3") || strings.Contains(titleLower, "the final season part 3") {
		return "Saison Finale (Partie 3)"
	}
	if strings.Contains(titleLower, "final season part 2") || strings.Contains(titleLower, "the final season part 2") {
		return "Saison Finale (Partie 2)"
	}
	if strings.Contains(titleLower, "final season") || strings.Contains(titleLower, "the final season") {
		return "Saison Finale"
	}
	if strings.Contains(titleLower, "season 1") || strings.Contains(titleLower, "1st season") || strings.Contains(titleLower, "s1") {
		return "Saison 1"
	}

	// Arcs
	if strings.Contains(titleLower, "hashira training") {
		return "Saison 4 (Entraînement des Piliers)"
	}
	if strings.Contains(titleLower, "swordsmith village") {
		return "Saison 3 (Village des Forgerons)"
	}
	if strings.Contains(titleLower, "entertainment district") {
		return "Saison 2 (Quartier des Plaisirs)"
	}
	if strings.Contains(titleLower, "mugen train") {
		return "Saison 2 (Train de l'Infini)"
	}

	// Formats
	if format == "MOVIE" || strings.Contains(titleLower, "movie") || strings.Contains(titleLower, "film") {
		return "Film"
	}
	if format == "OVA" || format == "ONA" || format == "SPECIAL" || strings.Contains(titleLower, "ova") {
		if strings.Contains(titleLower, "break time") {
			return "Spécial (Break Time)"
		}
		if strings.Contains(titleLower, "petit") {
			return "Spécial (Petit)"
		}
		return "OVA / Spécial"
	}
	if strings.Contains(titleLower, "break time") || strings.Contains(titleLower, "petit") {
		return "Spécial (Break Time)"
	}

	// Sequential fallback
	if seasonIndex == 0 {
		return "Saison 1"
	}
	return fmt.Sprintf("Saison %d", seasonIndex+1)
}

type aniListMediaItem struct {
	Synonyms  []string `json:"synonyms"`
	StartDate struct {
		Year  int `json:"year"`
		Month int `json:"month"`
		Day   int `json:"day"`
	} `json:"startDate"`
	AiringSchedule struct {
		Nodes []struct {
			Episode  int   `json:"episode"`
			AiringAt int64 `json:"airingAt"`
		} `json:"nodes"`
	} `json:"airingSchedule"`
	NextAiringEpisode *struct {
		Episode int `json:"episode"`
	} `json:"nextAiringEpisode"`
	ID    int `json:"id"`
	Title struct {
		English string `json:"english"`
		Romaji  string `json:"romaji"`
		Native  string `json:"native"`
	} `json:"title"`
	Format     string `json:"format"`
	Season     string `json:"season"`
	SeasonYear int    `json:"seasonYear"`
	CoverImage struct {
		ExtraLarge string `json:"extraLarge"`
		Large      string `json:"large"`
	} `json:"coverImage"`
	BannerImage  string   `json:"bannerImage"`
	Description  string   `json:"description"`
	Genres       []string `json:"genres"`
	Episodes     int      `json:"episodes"`
	AverageScore int      `json:"averageScore"`
	Status       string   `json:"status"`
	Relations    struct {
		Edges []aniListRelationEdge `json:"edges"`
	} `json:"relations"`
	StreamingEpisodes []struct {
		Title     string `json:"title"`
		Thumbnail string `json:"thumbnail"`
		URL       string `json:"url"`
		Site      string `json:"site"`
	} `json:"streamingEpisodes"`
}

type aniListPageResponse struct {
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
	Data struct {
		Page struct {
			PageInfo struct {
				HasNextPage bool `json:"hasNextPage"`
				Total       int  `json:"total"`
			} `json:"pageInfo"`
			Media []aniListMediaItem `json:"media"`
		} `json:"Page"`
	} `json:"data"`
}

type aniListDetailResponse struct {
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
	Data struct {
		Media aniListMediaItem `json:"Media"`
	} `json:"data"`
}

func isAnimeFormat(format string) bool {
	f := strings.ToUpper(strings.TrimSpace(format))
	return f == "TV" || f == "TV_SHORT" || f == "MOVIE" || f == "OVA" || f == "ONA" || f == "SPECIAL"
}

func isValidRelationType(rt string) bool {
	switch rt {
	case "PREQUEL", "SEQUEL", "PARENT", "SIDE_STORY", "ALTERNATIVE", "SPIN_OFF", "SUMMARY":
		return true
	default:
		return false
	}
}

func formatCatalogItem(m *aniListMediaItem, includeEpisodes bool) AnimeCatalogItem {
	poster := m.CoverImage.ExtraLarge
	if poster == "" {
		poster = m.CoverImage.Large
	}

	displayTitle := m.Title.English
	if displayTitle == "" {
		displayTitle = m.Title.Romaji
	}
	if displayTitle == "" {
		displayTitle = m.Title.Native
	}

	var score float64
	if m.AverageScore > 0 {
		score = float64(m.AverageScore) / 10.0
	}

	franchiseTitle := CleanFranchiseTitle(displayTitle)

	item := AnimeCatalogItem{
		Format:            m.Format,
		StartDate:         fmt.Sprintf("%04d-%02d-%02d", m.StartDate.Year, m.StartDate.Month, m.StartDate.Day),
		Aliases:           append([]string{m.Title.English, m.Title.Romaji, m.Title.Native}, m.Synonyms...),
		ID:                m.ID,
		TitleEnglish:      m.Title.English,
		TitleRomaji:       m.Title.Romaji,
		TitleNative:       m.Title.Native,
		DisplayTitle:      displayTitle,
		FranchiseTitle:    franchiseTitle,
		PosterImage:       poster,
		BannerImage:       m.BannerImage,
		Description:       cleanHTML(m.Description),
		Genres:            m.Genres,
		Episodes:          m.Episodes,
		AvailableEpisodes: availableEpisodeCount(m),
		AverageScore:      score,
		SeasonYear:        m.SeasonYear,
		Status:            m.Status,
	}

	// Build classified seasons and relations map
	seasonMap := make(map[int]AnimeSeason)

	// Add current media
	seasonMap[m.ID] = AnimeSeason{
		ID:          m.ID,
		Title:       displayTitle,
		SeasonName:  "",
		Format:      m.Format,
		Episodes:    m.Episodes,
		PosterImage: poster,
		SeasonYear:  m.SeasonYear,
		Status:      m.Status,
		IsCurrent:   true,
	}

	if len(m.Relations.Edges) > 0 {
		relations := make([]AnimeRelation, 0, len(m.Relations.Edges))
		for _, edge := range m.Relations.Edges {
			if !isValidRelationType(edge.RelationType) {
				continue // Skip CHARACTER, OTHER, etc.
			}

			node := edge.Node
			relTitle := node.Title.English
			if relTitle == "" {
				relTitle = node.Title.Romaji
			}
			if relTitle == "" {
				relTitle = node.Title.Native
			}
			relPoster := node.CoverImage.ExtraLarge
			if relPoster == "" {
				relPoster = node.CoverImage.Large
			}

			// Only add relation if it belongs to the same franchise
			if isAnimeFormat(node.Format) {
				relations = append(relations, AnimeRelation{
					ID:           node.ID,
					TitleEnglish: node.Title.English,
					TitleRomaji:  node.Title.Romaji,
					DisplayTitle: relTitle,
					Format:       node.Format,
					RelationType: edge.RelationType,
					SeasonYear:   node.SeasonYear,
					Season:       node.Season,
					Episodes:     node.Episodes,
					PosterImage:  relPoster,
					Status:       node.Status,
				})

				if isAnimeFormat(node.Format) {
					if _, exists := seasonMap[node.ID]; !exists {
						seasonMap[node.ID] = AnimeSeason{
							ID:          node.ID,
							Title:       relTitle,
							Format:      node.Format,
							Episodes:    node.Episodes,
							PosterImage: relPoster,
							SeasonYear:  node.SeasonYear,
							Status:      node.Status,
							IsCurrent:   node.ID == m.ID,
						}
					}
				}
			}

			// Add nested anime relations (prequels of sequels or adaptations) ONLY if same franchise
			for _, nestedEdge := range node.Relations.Edges {
				if !isValidRelationType(nestedEdge.RelationType) {
					continue
				}
				nNode := nestedEdge.Node
				if isAnimeFormat(nNode.Format) {
					if isFranchiseMatch(m.Title.English, m.Title.Romaji, nNode.Title.English, nNode.Title.Romaji) {
						if _, exists := seasonMap[nNode.ID]; !exists {
							nTitle := nNode.Title.English
							if nTitle == "" {
								nTitle = nNode.Title.Romaji
							}
							if nTitle == "" {
								nTitle = nNode.Title.Native
							}
							nPoster := nNode.CoverImage.ExtraLarge
							if nPoster == "" {
								nPoster = nNode.CoverImage.Large
							}
							seasonMap[nNode.ID] = AnimeSeason{
								ID:          nNode.ID,
								Title:       nTitle,
								Format:      nNode.Format,
								Episodes:    nNode.Episodes,
								PosterImage: nPoster,
								SeasonYear:  nNode.SeasonYear,
								Status:      nNode.Status,
								IsCurrent:   nNode.ID == m.ID,
							}
						}
					}
				}
			}
		}
		item.Relations = relations
	}

	// Build clean seasons list
	if len(seasonMap) > 0 {
		var tvSeasons []AnimeSeason
		var otherSeasons []AnimeSeason

		for _, s := range seasonMap {
			if isMainTVSeason(s.Format, s.Title) {
				tvSeasons = append(tvSeasons, s)
			} else {
				otherSeasons = append(otherSeasons, s)
			}
		}

		// Sort TV seasons chronologically by year and ID
		sort.Slice(tvSeasons, func(i, j int) bool {
			if tvSeasons[i].SeasonYear != tvSeasons[j].SeasonYear {
				if tvSeasons[i].SeasonYear == 0 {
					return false
				}
				if tvSeasons[j].SeasonYear == 0 {
					return true
				}
				return tvSeasons[i].SeasonYear < tvSeasons[j].SeasonYear
			}
			return tvSeasons[i].ID < tvSeasons[j].ID
		})

		// Assign labels to TV seasons
		for i := range tvSeasons {
			tvSeasons[i].SeasonName = deriveSeasonLabel(tvSeasons[i].Title, tvSeasons[i].Format, i)
		}

		// Sort other seasons (Movies, OVAs) chronologically
		sort.Slice(otherSeasons, func(i, j int) bool {
			if otherSeasons[i].SeasonYear != otherSeasons[j].SeasonYear {
				if otherSeasons[i].SeasonYear == 0 {
					return false
				}
				if otherSeasons[j].SeasonYear == 0 {
					return true
				}
				return otherSeasons[i].SeasonYear < otherSeasons[j].SeasonYear
			}
			return otherSeasons[i].ID < otherSeasons[j].ID
		})

		for i := range otherSeasons {
			label := deriveSeasonLabel(otherSeasons[i].Title, otherSeasons[i].Format, i)
			otherSeasons[i].SeasonName = label
		}

		item.Seasons = append(tvSeasons, otherSeasons...)
	}

	if !includeEpisodes {
		return item
	}

	// Parse the provider listing. Fractional numbers (".5" recaps) are not episodes of the season.
	episodes := make([]EpisodeInfo, 0, len(m.StreamingEpisodes))
	highest := 0
	for i, sep := range m.StreamingEpisodes {
		epNum := i + 1
		if match := regexpProviderEpisode.FindStringSubmatch(sep.Title); match != nil {
			if match[2] != "" {
				continue
			}
			epNum, _ = strconv.Atoi(match[1])
		}
		if epNum <= 0 {
			continue
		}
		highest = max(highest, epNum)
		episodes = append(episodes, EpisodeInfo{
			EpisodeNumber: epNum,
			Title:         sep.Title,
			Thumbnail:     sep.Thumbnail,
			URL:           sep.URL,
			Site:          sep.Site,
		})
	}

	count := m.Episodes
	if count == 0 && m.NextAiringEpisode != nil {
		count = m.NextAiringEpisode.Episode - 1
	}
	if count > 2500 {
		count = 2500
	}
	// A listing reaching beyond the season is numbered across the whole series (AniList even
	// attaches one season's listing to every season of a franchise): set it aside for InSeason.
	if count > 0 && highest > count {
		item.AbsoluteEpisodes = episodes
		episodes = nil
	}

	// Merge duplicate provider listings and fill gaps, without inventing unknown totals.
	byNumber := make(map[int]EpisodeInfo)
	for _, ep := range episodes {
		if _, ok := byNumber[ep.EpisodeNumber]; !ok {
			byNumber[ep.EpisodeNumber] = ep
		}
	}
	for n := 1; n <= count; n++ {
		if _, ok := byNumber[n]; !ok {
			byNumber[n] = EpisodeInfo{EpisodeNumber: n, Title: fmt.Sprintf("Épisode %d", n)}
		}
	}
	for _, scheduled := range m.AiringSchedule.Nodes {
		if scheduled.Episode > 0 && scheduled.Episode <= 2500 {
			ep := byNumber[scheduled.Episode]
			if ep.EpisodeNumber == 0 {
				ep = EpisodeInfo{EpisodeNumber: scheduled.Episode, Title: fmt.Sprintf("Épisode %d", scheduled.Episode)}
			}
			ep.AiringAt = scheduled.AiringAt
			byNumber[scheduled.Episode] = ep
		}
	}
	episodes = make([]EpisodeInfo, 0, len(byNumber))
	for _, ep := range byNumber {
		ep.Upcoming = ep.AiringAt > time.Now().Unix() || m.Status == "NOT_YET_RELEASED" || (m.NextAiringEpisode != nil && ep.EpisodeNumber >= m.NextAiringEpisode.Episode)
		episodes = append(episodes, ep)
	}

	// Sort episodes by EpisodeNumber
	sort.Slice(episodes, func(i, j int) bool {
		return episodes[i].EpisodeNumber < episodes[j].EpisodeNumber
	})

	item.EpisodeList = episodes
	return item
}

var regexpProviderEpisode = regexp.MustCompile(`^Episode (\d+)(\.\d+)?\b`)

// InSeason returns the entry as the season placed after offset main-continuity episodes:
// the AbsoluteEpisodes (provider numbering across the whole series) replace the generic titles,
// but only when every one of them falls inside the season; otherwise the offset is doubtful and
// none apply. Only the franchise knows that offset.
func (item AnimeCatalogItem) InSeason(offset int) AnimeCatalogItem {
	episodes := append([]EpisodeInfo(nil), item.EpisodeList...)
	if offset > 0 {
		index := make(map[int]int, len(episodes))
		for i, ep := range episodes {
			index[ep.EpisodeNumber] = i
		}
		fits := true
		for _, provider := range item.AbsoluteEpisodes {
			if _, ok := index[provider.EpisodeNumber-offset]; !ok {
				fits = false
				break
			}
		}
		for _, provider := range item.AbsoluteEpisodes {
			if !fits {
				break
			}
			i, ok := index[provider.EpisodeNumber-offset]
			if !ok {
				continue
			}
			ep := &episodes[i]
			ep.Title, ep.Thumbnail, ep.URL, ep.Site = provider.Title, provider.Thumbnail, provider.URL, provider.Site
			delete(index, ep.EpisodeNumber)
		}
	}
	item.EpisodeList = episodes
	item.AbsoluteEpisodes = nil
	return item
}

// GetTrending returns current trending anime.
func (s *AnimeCatalogService) GetTrending(ctx context.Context, page, perPage int) (*CatalogResponse, error) {
	if page <= 0 {
		page = 1
	}
	if perPage <= 0 || perPage > 50 {
		perPage = 20
	}

	cacheKey := fmt.Sprintf("trending-%d-%d", page, perPage)
	variables := map[string]interface{}{
		"page":    page,
		"perPage": perPage,
	}

	return s.catalogC.Get(ctx, cacheKey, catalogPolicy(30*time.Minute), func(ctx context.Context) (*CatalogResponse, error) {
		return s.doGraphQLPageQuery(ctx, trendingQuery, variables)
	})
}

// GetPopular returns all-time popular anime.
func (s *AnimeCatalogService) GetPopular(ctx context.Context, page, perPage int) (*CatalogResponse, error) {
	if page <= 0 {
		page = 1
	}
	if perPage <= 0 || perPage > 50 {
		perPage = 20
	}

	cacheKey := fmt.Sprintf("popular-%d-%d", page, perPage)
	variables := map[string]interface{}{
		"page":    page,
		"perPage": perPage,
	}

	return s.catalogC.Get(ctx, cacheKey, catalogPolicy(time.Hour), func(ctx context.Context) (*CatalogResponse, error) {
		return s.doGraphQLPageQuery(ctx, popularQuery, variables)
	})
}

// partialCatalogTTL is how long a page whose franchise enrichment fell short is kept: long enough
// to absorb a burst of visitors, short enough that the next renewal completes it.
const partialCatalogTTL = 2 * time.Minute

// catalogPolicy keeps complete pages for ttl and partial ones only briefly.
func catalogPolicy(ttl time.Duration) kv.Policy[CatalogResponse] {
	return kv.Policy[CatalogResponse]{TTLFor: func(r *CatalogResponse) time.Duration {
		if r.Partial {
			return partialCatalogTTL
		}
		return ttl
	}}
}

// SearchCatalog searches anime by title and an optional single genre.
func (s *AnimeCatalogService) SearchCatalog(ctx context.Context, query string, genre string, page, perPage int) (*CatalogResponse, error) {
	var include []string
	if genre = strings.TrimSpace(genre); genre != "" {
		include = []string{genre}
	}
	return s.SearchCatalogFiltered(ctx, query, include, nil, page, perPage)
}

// searchGenres and searchTags are the names the catalogue filter accepts; AniList keeps them in
// two namespaces (genres are broad, tags such as Isekai are specific), so each name is routed to the right one.
var searchGenres = map[string]bool{
	"action": true, "adventure": true, "comedy": true, "drama": true, "ecchi": true, "fantasy": true, "horror": true,
	"mahou shoujo": true, "mecha": true, "music": true, "mystery": true, "psychological": true, "romance": true,
	"sci-fi": true, "slice of life": true, "sports": true, "supernatural": true, "thriller": true,
}
var searchTags = map[string]string{
	"isekai": "Isekai", "reincarnation": "Reincarnation", "magic": "Magic", "school": "School", "harem": "Harem",
	"gore": "Gore", "shounen": "Shounen", "seinen": "Seinen",
}

const maxFilterTerms = 8

// splitFilterTerms routes each requested name to AniList genres or tags, dropping unknown names and duplicates.
func splitFilterTerms(names []string) (genres, tags []string) {
	seen := map[string]bool{}
	for _, name := range names {
		key := strings.ToLower(strings.TrimSpace(name))
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		if searchGenres[key] {
			genres = append(genres, canonicalGenre(key))
		} else if tag, ok := searchTags[key]; ok {
			tags = append(tags, tag)
		}
		if len(genres)+len(tags) == maxFilterTerms {
			break
		}
	}
	sort.Strings(genres)
	sort.Strings(tags)
	return genres, tags
}

func canonicalGenre(key string) string {
	parts := strings.Split(key, " ")
	for i, part := range parts {
		if part == "sci-fi" {
			parts[i] = "Sci-Fi"
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, " ")
}

// SearchCatalogFiltered searches by title with several wanted (include) and unwanted (exclude)
// genres or tags. A name in both lists is treated as excluded.
func (s *AnimeCatalogService) SearchCatalogFiltered(ctx context.Context, query string, include, exclude []string, page, perPage int) (*CatalogResponse, error) {
	if page <= 0 {
		page = 1
	}
	if perPage <= 0 || perPage > 50 {
		perPage = 24
	}
	query = strings.TrimSpace(query)
	excludeSet := map[string]bool{}
	for _, name := range exclude {
		excludeSet[strings.ToLower(strings.TrimSpace(name))] = true
	}
	wanted := make([]string, 0, len(include))
	for _, name := range include {
		if !excludeSet[strings.ToLower(strings.TrimSpace(name))] {
			wanted = append(wanted, name)
		}
	}
	genreIn, tagIn := splitFilterTerms(wanted)
	genreNotIn, tagNotIn := splitFilterTerms(exclude)

	cacheKey := fmt.Sprintf("search-%s-%s-%s-%s-%s-%d-%d", strings.ToLower(query), strings.Join(genreIn, ","), strings.Join(tagIn, ","), strings.Join(genreNotIn, ","), strings.Join(tagNotIn, ","), page, perPage)
	variables := map[string]interface{}{"page": page, "perPage": perPage}
	if query != "" {
		variables["search"] = query
	}
	for name, values := range map[string][]string{"genreIn": genreIn, "genreNotIn": genreNotIn, "tagIn": tagIn, "tagNotIn": tagNotIn} {
		if len(values) > 0 {
			variables[name] = values
		}
	}

	return s.catalogC.Get(ctx, cacheKey, kv.Policy[CatalogResponse]{TTL: 15 * time.Minute}, func(ctx context.Context) (*CatalogResponse, error) {
		var parsed aniListPageResponse
		if err := s.anilist.post(ctx, searchQuery, variables, &parsed); err != nil {
			return nil, err
		}
		if len(parsed.Errors) > 0 {
			return nil, fmt.Errorf("anilist: %s", parsed.Errors[0].Message)
		}
		return s.formatSearchResponse(ctx, &parsed, page, perPage), nil
	})
}

// formatSearchResponse groups a search page with one AniList call. Walking each result's franchise
// costs one call per related entry and used to exhaust the shared AniList budget, leaving searches
// stuck behind rate limits. Instead, entries of one continuity inside the page share one card
// leading to their first main season, and franchises already in cache are used as they are.
func (s *AnimeCatalogService) formatSearchResponse(ctx context.Context, parsed *aniListPageResponse, page, perPage int) *CatalogResponse {
	media := parsed.Data.Page.Media
	inPage := make(map[int]bool, len(media))
	for _, m := range media {
		inPage[m.ID] = true
	}
	// up links an entry to one it belongs with, following the edges buildFranchise follows:
	// prequels and parents, then the reverse of a sequel, then a film retelling a series. A single
	// link per entry keeps a crossover from merging two series. Attached entries (side stories,
	// retellings) join a group without ever representing it.
	up := map[int]int{}
	attached := map[int]bool{}
	link := func(from, to int, attach bool) {
		if _, linked := up[from]; !linked && from != to && inPage[from] && inPage[to] {
			up[from] = to
			attached[from] = attach
		}
	}
	for _, m := range media {
		for _, rel := range m.Relations.Edges {
			if rel.RelationType == "PREQUEL" || rel.RelationType == "PARENT" {
				link(m.ID, rel.Node.ID, rel.RelationType == "PARENT")
			}
		}
	}
	for _, m := range media {
		for _, rel := range m.Relations.Edges {
			if rel.RelationType == "SEQUEL" {
				link(rel.Node.ID, m.ID, false)
			}
		}
	}
	for _, m := range media {
		for _, rel := range m.Relations.Edges {
			switch rel.RelationType {
			case "ALTERNATIVE", "SIDE_STORY", "SPIN_OFF", "SUMMARY":
				if m.Format == "MOVIE" && (rel.Node.Format == "TV" || rel.Node.Format == "TV_SHORT") {
					link(m.ID, rel.Node.ID, true)
				}
			}
		}
	}
	root := func(id int) (int, int) {
		depth := 0
		for seen := map[int]bool{id: true}; ; depth++ {
			next, ok := up[id]
			if !ok || seen[next] {
				return id, depth
			}
			seen[next] = true
			id = next
		}
	}

	formatted := make([]AnimeCatalogItem, len(media))
	roots := make([]int, len(media))
	depths := make([]int, len(media))
	for i := range media {
		formatted[i] = formatCatalogItem(&media[i], false)
		roots[i], depths[i] = root(media[i].ID)
	}
	// The card of a group is its first main season, as a franchise's canonical entry is.
	better := func(a, b int) bool {
		fa, fb := formatted[a], formatted[b]
		if xa, xb := attached[fa.ID], attached[fb.ID]; xa != xb {
			return xb
		}
		if ma, mb := isMainFormat(fa.Format), isMainFormat(fb.Format); ma != mb {
			return ma
		}
		if ua, ub := fa.StartDate == "0000-00-00", fb.StartDate == "0000-00-00"; ua != ub {
			return ub
		}
		if fa.StartDate != fb.StartDate {
			return fa.StartDate < fb.StartDate
		}
		if depths[a] != depths[b] {
			return depths[a] < depths[b]
		}
		return fa.ID < fb.ID
	}
	head := map[int]int{}
	for i, r := range roots {
		if h, ok := head[r]; !ok || better(i, h) {
			head[r] = i
		}
	}
	known := func(id int) *Franchise {
		if f, ok := s.franchiseC.Peek(ctx, strconv.Itoa(id)); ok && f.Complete {
			return f
		}
		return nil
	}

	seen := map[int]bool{}
	items := make([]AnimeCatalogItem, 0, len(media))
	for i := range media {
		item := formatted[i]
		item.MediaID = item.ID
		item.MediaTitle = item.DisplayTitle
		item.MediaPosterImage = item.PosterImage
		first := formatted[head[roots[i]]]
		franchise := known(first.ID)
		if franchise == nil && first.ID != item.ID {
			franchise = known(item.ID)
		}
		switch {
		case franchise != nil:
			item.ID = franchise.ID
			item.DisplayTitle = franchise.Title
			item.FranchiseTitle = franchise.Title
			item.PosterImage = franchise.PosterImage
		case first.ID != item.ID:
			item.ID = first.ID
			item.DisplayTitle = first.DisplayTitle
			item.FranchiseTitle = first.FranchiseTitle
			item.PosterImage = first.PosterImage
		}
		if seen[item.ID] {
			continue
		}
		seen[item.ID] = true
		items = append(items, item)
	}
	return &CatalogResponse{
		HasNextPage: parsed.Data.Page.PageInfo.HasNextPage,
		Page:        page,
		PerPage:     perPage,
		Total:       parsed.Data.Page.PageInfo.Total,
		Items:       items,
	}
}

func isMainFormat(format string) bool {
	return format == "TV" || format == "TV_SHORT" || format == "ONA"
}

// GetAnimeDetailsWithEpisodes gets full anime metadata with episode details.
func (s *AnimeCatalogService) GetAnimeDetailsWithEpisodes(ctx context.Context, id int) (*AnimeCatalogItem, error) {
	if id <= 0 {
		return nil, fmt.Errorf("invalid anime id: %d", id)
	}
	return s.detailC.Get(ctx, strconv.Itoa(id), kv.Policy[AnimeCatalogItem]{TTL: 2 * time.Hour}, func(ctx context.Context) (*AnimeCatalogItem, error) {
		var parsed aniListDetailResponse
		if err := s.anilist.post(ctx, animeDetailWithEpisodesQuery, map[string]interface{}{"id": id}, &parsed); err != nil {
			return nil, err
		}
		if len(parsed.Errors) > 0 {
			return nil, fmt.Errorf("anilist: %s", parsed.Errors[0].Message)
		}
		if parsed.Data.Media.ID == 0 {
			return nil, fmt.Errorf("anime not found for id %d", id)
		}
		item := formatCatalogItem(&parsed.Data.Media, true)
		return &item, nil
	})
}

// maxDetailBatch bounds one id_in lookup; AniList's own page size cap is 50.
const maxDetailBatch = 25

// prefetchDetails loads the entries of ids missing from the detail cache with batched AniList
// calls and stores them, so the lookups that follow are cache hits. A failed batch is ignored: the
// caller's own lookup then retries each entry and reports the error it meets.
func (s *AnimeCatalogService) prefetchDetails(ctx context.Context, ids []int) {
	missing := make([]int, 0, len(ids))
	for _, id := range ids {
		if _, ok := s.detailC.Peek(ctx, strconv.Itoa(id)); !ok {
			missing = append(missing, id)
		}
	}
	// A lone entry is the same single lookup, and its cache path also shares the work across callers.
	if len(missing) < 2 {
		return
	}
	for start := 0; start < len(missing); start += maxDetailBatch {
		batch := missing[start:min(start+maxDetailBatch, len(missing))]
		var parsed aniListPageResponse
		err := s.anilist.post(ctx, animeDetailBatchQuery, map[string]interface{}{"ids": batch, "perPage": len(batch)}, &parsed)
		if err != nil || len(parsed.Errors) > 0 {
			return
		}
		for i := range parsed.Data.Page.Media {
			m := &parsed.Data.Page.Media[i]
			if m.ID == 0 {
				continue
			}
			item := formatCatalogItem(m, true)
			s.detailC.Put(ctx, strconv.Itoa(m.ID), &item, 2*time.Hour)
		}
	}
}

func (s *AnimeCatalogService) doGraphQLPageQuery(ctx context.Context, query string, variables map[string]interface{}) (*CatalogResponse, error) {
	var parsed aniListPageResponse
	if err := s.anilist.post(ctx, query, variables, &parsed); err != nil {
		return nil, err
	}
	if len(parsed.Errors) > 0 {
		return nil, fmt.Errorf("anilist: %s", parsed.Errors[0].Message)
	}
	return s.formatPageResponse(ctx, &parsed, variables)
}

// formatPageResponse turns raw AniList media into catalog items, resolving franchises.
func (s *AnimeCatalogService) formatPageResponse(ctx context.Context, parsed *aniListPageResponse, variables map[string]interface{}) (*CatalogResponse, error) {
	// Resolve canonical continuity IDs in parallel with a bounded deadline.
	// Never merge unrelated works merely because their titles share a prefix.
	formattedItems := make([]AnimeCatalogItem, len(parsed.Data.Page.Media))
	partial := false
	var workers sync.WaitGroup
	var resultMu sync.Mutex
	sem := make(chan struct{}, 3)
	// Enrichment only: a card without its franchise still works, so it must not take the
	// AniList tokens a visitor's own request is waiting for.
	resolveCtx, cancel := context.WithTimeout(kv.Background(ctx), 20*time.Second)
	defer cancel()
	for i := range parsed.Data.Page.Media {
		workers.Add(1)
		go func(i int) {
			defer workers.Done()
			formatted := formatCatalogItem(&parsed.Data.Page.Media[i], false)
			// Discovery cards navigate to the franchise, while the featured
			// playback action must retain the season represented by its artwork.
			formatted.MediaID = formatted.ID
			formatted.MediaTitle = formatted.DisplayTitle
			formatted.MediaPosterImage = formatted.PosterImage
			select {
			case sem <- struct{}{}:
			case <-resolveCtx.Done():
				resultMu.Lock()
				partial = true
				formattedItems[i] = formatted
				resultMu.Unlock()
				return
			}
			defer func() { <-sem }()
			franchise, err := s.GetFranchise(resolveCtx, formatted.ID)
			if err == nil && franchise.Complete {
				formatted.ID = franchise.ID
				formatted.DisplayTitle = franchise.Title
				formatted.FranchiseTitle = franchise.Title
				formatted.PosterImage = franchise.PosterImage
			}
			resultMu.Lock()
			if err != nil || !franchise.Complete {
				partial = true
			}
			formattedItems[i] = formatted
			resultMu.Unlock()
		}(i)
	}
	workers.Wait()
	seenFranchise := make(map[int]bool)
	items := make([]AnimeCatalogItem, 0, len(formattedItems))
	for _, formatted := range formattedItems {
		identity := formatted.ID
		if _, seasonal := variables["season"]; seasonal {
			identity = formatted.MediaID
		}
		if seenFranchise[identity] {
			continue
		}
		seenFranchise[identity] = true
		items = append(items, formatted)
	}
	warning := ""
	if partial {
		warning = "Certaines informations de saisons sont temporairement indisponibles."
	}

	page := 1
	perPage := len(items)
	if p, ok := variables["page"].(int); ok {
		page = p
	}
	if pp, ok := variables["perPage"].(int); ok {
		perPage = pp
	}

	return &CatalogResponse{
		Partial: partial, Warning: warning,
		HasNextPage: parsed.Data.Page.PageInfo.HasNextPage,
		Page:        page,
		PerPage:     perPage,
		Total:       parsed.Data.Page.PageInfo.Total,
		Items:       items,
	}, nil
}

// Total planned episodes are not a release count for an ongoing series.
func availableEpisodeCount(m *aniListMediaItem) *int {
	count := 0
	switch {
	case m.Status == "NOT_YET_RELEASED":
	case m.Status == "FINISHED" && m.Episodes > 0:
		count = m.Episodes
	case m.NextAiringEpisode != nil && m.NextAiringEpisode.Episode > 0:
		count = m.NextAiringEpisode.Episode - 1
		if m.Episodes > 0 {
			count = min(count, m.Episodes)
		}
	default:
		return nil
	}
	return &count
}
