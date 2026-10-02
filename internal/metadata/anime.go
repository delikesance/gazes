package metadata

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

// AnimeMetadata represents rich anime catalog details.
type AnimeMetadata struct {
	ID           int      `json:"id"`
	TitleEnglish string   `json:"title_english,omitempty"`
	TitleRomaji  string   `json:"title_romaji,omitempty"`
	TitleNative  string   `json:"title_native,omitempty"`
	DisplayTitle string   `json:"display_title"`
	PosterImage  string   `json:"poster_image,omitempty"`
	BannerImage  string   `json:"banner_image,omitempty"`
	Description  string   `json:"description,omitempty"`
	Genres       []string `json:"genres,omitempty"`
	Episodes     int      `json:"episodes,omitempty"`
	AverageScore float64  `json:"average_score,omitempty"`
	SeasonYear   int      `json:"season_year,omitempty"`
}

type cachedAnime struct {
	data      *AnimeMetadata
	expiresAt time.Time
}

// AnimeProvider resolves anime metadata from open-source APIs.
type AnimeProvider interface {
	GetAnimeByTitle(ctx context.Context, rawTitle string) (*AnimeMetadata, error)
}

// AnimeService queries AniList GraphQL and Kitsu REST API with in-memory caching.
type AnimeService struct {
	httpClient *http.Client
	cache      map[string]cachedAnime
	mu         sync.RWMutex
}

// NewAnimeService creates a new AnimeService instance.
func NewAnimeService(client *http.Client) *AnimeService {
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	return &AnimeService{
		httpClient: client,
		cache:      make(map[string]cachedAnime),
	}
}

// GetAnimeByTitle resolves anime metadata using AniList with Kitsu fallback.
func (s *AnimeService) GetAnimeByTitle(ctx context.Context, rawTitle string) (*AnimeMetadata, error) {
	cleanTitle := CleanAnimeTitle(rawTitle)
	if cleanTitle == "" {
		cleanTitle = rawTitle
	}

	cacheKey := strings.ToLower(strings.TrimSpace(cleanTitle))

	// 1. Check in-memory cache
	s.mu.RLock()
	cached, found := s.cache[cacheKey]
	s.mu.RUnlock()

	if found && time.Now().Before(cached.expiresAt) {
		return cached.data, nil
	}

	// 2. Query AniList GraphQL API (Primary)
	meta, err := s.fetchAniList(ctx, cleanTitle)
	if err == nil && meta != nil {
		s.saveCache(cacheKey, meta)
		return meta, nil
	}

	// 3. Fallback to Kitsu REST API
	meta, err = s.fetchKitsu(ctx, cleanTitle)
	if err == nil && meta != nil {
		s.saveCache(cacheKey, meta)
		return meta, nil
	}

	// Cache negative lookup for 10 minutes to avoid hammering APIs on non-anime
	s.saveCacheWithTTL(cacheKey, nil, 10*time.Minute)
	return nil, fmt.Errorf("anime metadata not found for title: %s", cleanTitle)
}

func (s *AnimeService) saveCache(key string, data *AnimeMetadata) {
	s.saveCacheWithTTL(key, data, 24*time.Hour)
}

func (s *AnimeService) saveCacheWithTTL(key string, data *AnimeMetadata, ttl time.Duration) {
	s.mu.Lock()
	s.cache[key] = cachedAnime{
		data:      data,
		expiresAt: time.Now().Add(ttl),
	}
	s.mu.Unlock()
}

// AniList GraphQL Query
const aniListQuery = `
query ($search: String) {
  Media(search: $search, type: ANIME) {
    id
    title {
      romaji
      english
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
  }
}
`

type aniListResponse struct {
	Data struct {
		Media struct {
			ID    int `json:"id"`
			Title struct {
				Romaji  string `json:"romaji"`
				English string `json:"english"`
				Native  string `json:"native"`
			} `json:"title"`
			CoverImage struct {
				ExtraLarge string `json:"extraLarge"`
				Large      string `json:"large"`
			} `json:"coverImage"`
			BannerImage  string   `json:"bannerImage"`
			Description  string   `json:"description"`
			Genres       []string `json:"genres"`
			Episodes     int      `json:"episodes"`
			AverageScore int      `json:"averageScore"`
			SeasonYear   int      `json:"seasonYear"`
		} `json:"Media"`
	} `json:"data"`
}

func (s *AnimeService) fetchAniList(ctx context.Context, title string) (*AnimeMetadata, error) {
	reqBody := map[string]interface{}{
		"query": aniListQuery,
		"variables": map[string]string{
			"search": title,
		},
	}

	jsonBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://graphql.anilist.co", bytes.NewBuffer(jsonBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Gazes/1.0 (AnimeStreamingEngine)")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("anilist returned status %d", resp.StatusCode)
	}

	var parsed aniListResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, err
	}

	media := parsed.Data.Media
	if media.ID == 0 {
		return nil, fmt.Errorf("no media found")
	}

	poster := media.CoverImage.ExtraLarge
	if poster == "" {
		poster = media.CoverImage.Large
	}

	displayTitle := media.Title.English
	if displayTitle == "" {
		displayTitle = media.Title.Romaji
	}
	if displayTitle == "" {
		displayTitle = media.Title.Native
	}

	// Clean HTML tags from description
	cleanDesc := cleanHTML(media.Description)

	var score float64
	if media.AverageScore > 0 {
		score = float64(media.AverageScore) / 10.0
	}

	return &AnimeMetadata{
		ID:           media.ID,
		TitleEnglish: media.Title.English,
		TitleRomaji:  media.Title.Romaji,
		TitleNative:  media.Title.Native,
		DisplayTitle: displayTitle,
		PosterImage:  poster,
		BannerImage:  media.BannerImage,
		Description:  cleanDesc,
		Genres:       media.Genres,
		Episodes:     media.Episodes,
		AverageScore: score,
		SeasonYear:   media.SeasonYear,
	}, nil
}

// Kitsu API Structures
type kitsuResponse struct {
	Data []struct {
		Attributes struct {
			CanonicalTitle string            `json:"canonicalTitle"`
			Titles         map[string]string `json:"titles"`
			Synopsis       string            `json:"synopsis"`
			PosterImage    struct {
				Large    string `json:"large"`
				Original string `json:"original"`
			} `json:"posterImage"`
			CoverImage struct {
				Large    string `json:"large"`
				Original string `json:"original"`
			} `json:"coverImage"`
			EpisodeCount  int    `json:"episodeCount"`
			AverageRating string `json:"averageRating"`
			StartDate     string `json:"startDate"`
		} `json:"attributes"`
	} `json:"data"`
}

func (s *AnimeService) fetchKitsu(ctx context.Context, title string) (*AnimeMetadata, error) {
	kitsuURL := fmt.Sprintf("https://kitsu.io/api/edge/anime?filter[text]=%s&page[limit]=1", url.QueryEscape(title))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, kitsuURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.api+json")
	req.Header.Set("User-Agent", "Gazes/1.0 (AnimeStreamingEngine)")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("kitsu returned status %d", resp.StatusCode)
	}

	var parsed kitsuResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil || len(parsed.Data) == 0 {
		return nil, fmt.Errorf("no kitsu media found")
	}

	item := parsed.Data[0].Attributes
	poster := item.PosterImage.Large
	if poster == "" {
		poster = item.PosterImage.Original
	}

	banner := item.CoverImage.Large
	if banner == "" {
		banner = item.CoverImage.Original
	}

	var score float64
	if item.AverageRating != "" {
		var r float64
		if _, err := fmt.Sscanf(item.AverageRating, "%f", &r); err == nil {
			score = r / 10.0
		}
	}

	var year int
	if len(item.StartDate) >= 4 {
		_, _ = fmt.Sscanf(item.StartDate[:4], "%d", &year)
	}

	return &AnimeMetadata{
		TitleEnglish: item.Titles["en"],
		TitleRomaji:  item.Titles["en_jp"],
		TitleNative:  item.Titles["ja_jp"],
		DisplayTitle: item.CanonicalTitle,
		PosterImage:  poster,
		BannerImage:  banner,
		Description:  cleanHTML(item.Synopsis),
		Episodes:     item.EpisodeCount,
		AverageScore: score,
		SeasonYear:   year,
	}, nil
}

// Regex patterns for torrent release tag removal
var (
	bracketPattern   = regexp.MustCompile(`\[[^\]]*\]|\([^\)]*\)`)
	fileExtPattern   = regexp.MustCompile(`\.(mkv|mp4|avi|webm|ts)$`)
	episodePattern   = regexp.MustCompile(`(?i)(S\d{1,2}E\d{1,2}|Season\s*\d+|\bS\d+\b|\bE\d+\b|Episode\s*\d+|-\s*\d{1,4}(\s*v\d+)?)\b.*$`)
	qualityPattern   = regexp.MustCompile(`(?i)(1080p|720p|480p|2160p|4k|hevc|avc|h\.?264|h\.?265|x264|x265|aac|flac|web-dl|webrip|bdrip|dvdrip|10bit|10bits|multisub|multi-sub|dual audio)`)
	crcPattern       = regexp.MustCompile(`(?i)[0-9A-F]{8}`)
	cleanSpacesRegex = regexp.MustCompile(`\s+`)
)

// CleanAnimeTitle cleans torrent titles like "[SubsPlease] Sousou no Frieren S2 - 10 (1080p) [E441F1A4].mkv"
// into canonical search queries like "Sousou no Frieren".
func CleanAnimeTitle(title string) string {
	cleaned := fileExtPattern.ReplaceAllString(title, "")

	// Strip release group brackets like [SubsPlease], [Erai-raws], [ToonsHub]
	cleaned = bracketPattern.ReplaceAllString(cleaned, " ")

	// Strip quality / codec keywords
	cleaned = qualityPattern.ReplaceAllString(cleaned, " ")

	// Strip CRC checksums
	cleaned = crcPattern.ReplaceAllString(cleaned, " ")

	// Strip episode suffixes like - 10 or S02E10
	cleaned = episodePattern.ReplaceAllString(cleaned, " ")

	// Normalize separators & whitespace
	cleaned = strings.ReplaceAll(cleaned, "_", " ")
	cleaned = strings.ReplaceAll(cleaned, ".", " ")
	cleaned = strings.ReplaceAll(cleaned, "-", " ")
	cleaned = cleanSpacesRegex.ReplaceAllString(cleaned, " ")
	cleaned = strings.TrimSpace(cleaned)

	return cleaned
}

func cleanHTML(s string) string {
	re := regexp.MustCompile(`<[^>]*>`)
	cleaned := re.ReplaceAllString(s, "")
	cleaned = strings.ReplaceAll(cleaned, "&quot;", `"`)
	cleaned = strings.ReplaceAll(cleaned, "&#039;", `'`)
	cleaned = strings.ReplaceAll(cleaned, "&amp;", `&`)
	return strings.TrimSpace(cleaned)
}
