package indexer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	ErrAuthorityMappingMissing = errors.New("arr authority mapping missing")
	ErrNoApprovedRelease       = errors.New("arr authority found no approved single-episode release")
)

// AuthorityMap binds immutable catalog ids to ids selected explicitly in Sonarr
// or Radarr. The binding is configuration, never a title-matching heuristic.
type AuthorityMap map[int]int

func ParseAuthorityMap(raw string) (AuthorityMap, error) {
	result := AuthorityMap{}
	if strings.TrimSpace(raw) == "" {
		return result, nil
	}
	var encoded map[string]int
	if err := json.Unmarshal([]byte(raw), &encoded); err != nil {
		return nil, fmt.Errorf("invalid arr authority map")
	}
	for catalog, authority := range encoded {
		mediaID, err := strconv.Atoi(catalog)
		if err != nil || mediaID <= 0 || authority <= 0 {
			return nil, fmt.Errorf("invalid arr authority map")
		}
		result[mediaID] = authority
	}
	return result, nil
}

type arrEndpoint struct {
	name, baseURL, apiKey string
	bindings              AuthorityMap
}

// ArrEpisodeResolver treats Sonarr/Radarr as the sole release authority. It
// does not search Prowlarr, parse titles, classify languages, or fall back to
// the legacy resolver. Prowlarr is expected to sync indexers into the *Arr apps.
type ArrEpisodeResolver struct {
	sonarr, radarr arrEndpoint
	client         *http.Client
}

func NewArrEpisodeResolver(sonarrURL, sonarrKey, sonarrMap, radarrURL, radarrKey, radarrMap string, client *http.Client) (*ArrEpisodeResolver, error) {
	series, err := ParseAuthorityMap(sonarrMap)
	if err != nil {
		return nil, err
	}
	movies, err := ParseAuthorityMap(radarrMap)
	if err != nil {
		return nil, err
	}
	if client == nil {
		client = &http.Client{Timeout: 12 * time.Second}
	}
	return &ArrEpisodeResolver{
		sonarr: arrEndpoint{name: "Sonarr", baseURL: strings.TrimRight(sonarrURL, "/"), apiKey: sonarrKey, bindings: series},
		radarr: arrEndpoint{name: "Radarr", baseURL: strings.TrimRight(radarrURL, "/"), apiKey: radarrKey, bindings: movies},
		client: client,
	}, nil
}

type arrEpisode struct {
	ID            int `json:"id"`
	SeasonNumber  int `json:"seasonNumber"`
	EpisodeNumber int `json:"episodeNumber"`
}

type arrLanguage struct {
	Name string `json:"name"`
}

type arrRelease struct {
	Title               string        `json:"title"`
	DownloadURL         string        `json:"downloadUrl"`
	Guid                string        `json:"guid"`
	Size                int64         `json:"size"`
	Seeders             int           `json:"seeders"`
	Leechers            int           `json:"leechers"`
	PublishDate         time.Time     `json:"publishDate"`
	Indexer             string        `json:"indexer"`
	Approved            bool          `json:"approved"`
	Rejected            bool          `json:"rejected"`
	TemporarilyRejected bool          `json:"temporarilyRejected"`
	Rejections          []string      `json:"rejections"`
	FullSeason          bool          `json:"fullSeason"`
	CustomFormatScore   int           `json:"customFormatScore"`
	Languages           []arrLanguage `json:"languages"`
}

func (r *ArrEpisodeResolver) get(ctx context.Context, endpoint arrEndpoint, path string, dst any) error {
	if endpoint.baseURL == "" || endpoint.apiKey == "" {
		return fmt.Errorf("%s authority is not configured", endpoint.name)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.baseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Api-Key", endpoint.apiKey)
	req.Header.Set("Accept", "application/json")
	res, err := r.client.Do(req)
	if err != nil {
		return fmt.Errorf("%s authority unavailable: %w", endpoint.name, err)
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("%s authority returned HTTP %d", endpoint.name, res.StatusCode)
	}
	if err := json.NewDecoder(res.Body).Decode(dst); err != nil {
		return fmt.Errorf("%s authority returned invalid JSON", endpoint.name)
	}
	return nil
}

func (r *ArrEpisodeResolver) ResolveSeasonSources(ctx context.Context, identity EpisodeIdentity) (*EpisodeSourcesResponse, error) {
	return r.resolve(ctx, identity)
}

func (r *ArrEpisodeResolver) ResolvePlaybackSources(ctx context.Context, identity EpisodeIdentity) (*EpisodeSourcesResponse, error) {
	return r.resolve(ctx, identity)
}

func (r *ArrEpisodeResolver) resolve(ctx context.Context, identity EpisodeIdentity) (*EpisodeSourcesResponse, error) {
	if identity.MediaID <= 0 {
		return nil, fmt.Errorf("%w: catalog media id is absent", ErrAuthorityMappingMissing)
	}
	movie := strings.EqualFold(identity.Format, "MOVIE")
	endpoint := r.sonarr
	if movie {
		endpoint = r.radarr
	}
	authorityID := endpoint.bindings[identity.MediaID]
	if authorityID <= 0 {
		return nil, fmt.Errorf("%w: AniList %d has no %s binding", ErrAuthorityMappingMissing, identity.MediaID, endpoint.name)
	}

	var releases []arrRelease
	if movie {
		if err := r.get(ctx, endpoint, "/api/v3/release?movieId="+strconv.Itoa(authorityID), &releases); err != nil {
			return nil, err
		}
	} else {
		var episodes []arrEpisode
		path := "/api/v3/episode?seriesId=" + strconv.Itoa(authorityID) + "&seasonNumber=" + strconv.Itoa(identity.SeasonNumber)
		if err := r.get(ctx, endpoint, path, &episodes); err != nil {
			return nil, err
		}
		episodeID := 0
		for _, episode := range episodes {
			if episode.SeasonNumber == identity.SeasonNumber && episode.EpisodeNumber == identity.EpisodeNumber {
				episodeID = episode.ID
				break
			}
		}
		if episodeID == 0 {
			return nil, fmt.Errorf("%s has no mapped S%02dE%02d", endpoint.name, identity.SeasonNumber, identity.EpisodeNumber)
		}
		if err := r.get(ctx, endpoint, "/api/v3/release?episodeId="+strconv.Itoa(episodeID), &releases); err != nil {
			return nil, err
		}
	}

	sources := authoritySources(identity, endpoint.name, releases)
	if len(sources) == 0 {
		return nil, ErrNoApprovedRelease
	}
	title := ""
	if len(identity.Titles) > 0 {
		title = identity.Titles[0]
	}
	return &EpisodeSourcesResponse{
		AnimeTitle: title, SeasonNumber: identity.SeasonNumber, EpisodeNumber: identity.EpisodeNumber,
		TotalSources: len(sources), Sources: sources,
	}, nil
}

var arrInfoHash = regexp.MustCompile(`(?i)(?:^|[?&])xt=urn:btih:([a-f0-9]{40})(?:&|$)`)

func authoritySources(identity EpisodeIdentity, authority string, releases []arrRelease) []EpisodeSource {
	result := make([]EpisodeSource, 0, len(releases))
	seen := map[string]bool{}
	for _, release := range releases {
		// Sonarr/Radarr own every semantic decision. Gazes only enforces the
		// authority verdict and validates transport-safe single-episode magnets.
		if !release.Approved || release.Rejected || release.TemporarilyRejected || len(release.Rejections) != 0 || release.FullSeason {
			continue
		}
		magnet := strings.TrimSpace(release.DownloadURL)
		if !strings.HasPrefix(strings.ToLower(magnet), "magnet:?") {
			continue
		}
		match := arrInfoHash.FindStringSubmatch(magnet)
		if len(match) != 2 {
			continue
		}
		hash := strings.ToLower(match[1])
		if seen[hash] {
			continue
		}
		seen[hash] = true
		languages := make([]string, 0, len(release.Languages))
		for _, language := range release.Languages {
			if strings.TrimSpace(language.Name) != "" {
				languages = append(languages, language.Name)
			}
		}
		label := authority + " approved"
		if len(languages) > 0 {
			label += " · " + strings.Join(languages, ", ")
		}
		result = append(result, EpisodeSource{
			AnimeTitle: firstTitle(identity.Titles), TorrentItem: TorrentItem{
				Provider: release.Indexer, ID: release.Guid, Title: release.Title, InfoHash: hash,
				MagnetURI: magnet, SizeBytes: release.Size, Seeders: max(0, release.Seeders),
				Leechers: max(0, release.Leechers), PublishDate: release.PublishDate,
			},
			EpisodeNumber: identity.EpisodeNumber, SeasonNumber: identity.SeasonNumber,
			LanguageTag: LangOther, LanguageLabel: label, ScoreRank: release.CustomFormatScore,
		})
	}
	// Preserve *Arr's semantic order: Gazes must not introduce a second ranking policy.
	return result
}

func firstTitle(titles []string) string {
	if len(titles) == 0 {
		return ""
	}
	return titles[0]
}
