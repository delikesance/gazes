package metadata

import (
	"context"
	"errors"
	"fmt"
	"github.com/gazes/gazes/internal/kv"
	"sort"
	"sync"
	"time"
)

// ScheduleEntry is one episode airing at a given time.
type ScheduleEntry struct {
	AiringAt    int64    `json:"airing_at"`
	Episode     int      `json:"episode"`
	MediaID     int      `json:"media_id"`
	Title       string   `json:"title"`
	PosterImage string   `json:"poster_image,omitempty"`
	Genres      []string `json:"genres,omitempty"`
	Studio      string   `json:"studio,omitempty"`
	Format      string   `json:"format,omitempty"`
	Episodes    int      `json:"episodes,omitempty"`
}

// ScheduleResponse lists the airings between From and To (unix seconds).
type ScheduleResponse struct {
	From    int64           `json:"from"`
	To      int64           `json:"to"`
	Partial bool            `json:"partial,omitempty"`
	Entries []ScheduleEntry `json:"entries"`
}

const (
	scheduleMaxSpan  = 42 * 24 * time.Hour
	schedulePageSize = 50
	scheduleMaxPages = 20
	scheduleWorkers  = 4
	scheduleCacheTTL = 365 * 24 * time.Hour // aired weeks never change, the light payload is not worth refetching
	// schedulePartialTTL keeps an incomplete answer a short while so missing pages are retried.
	schedulePartialTTL = 10 * time.Minute
	// scheduleMaxWait is the longest 429 pause worth waiting out inside one request.
	scheduleMaxWait = 6 * time.Second
)

const scheduleQuery = `
query ($page: Int, $perPage: Int, $from: Int, $to: Int) {
  Page(page: $page, perPage: $perPage) {
    pageInfo { hasNextPage }
    airingSchedules(airingAt_greater: $from, airingAt_lesser: $to, sort: TIME) {
      airingAt
      episode
      media {
        id
        title { english romaji native }
        coverImage { extraLarge large }
        genres
        format
        episodes
        isAdult
        countryOfOrigin
        studios(isMain: true) { nodes { name } }
      }
    }
  }
}`

type schedulePage struct {
	Data struct {
		Page struct {
			PageInfo struct {
				HasNextPage bool `json:"hasNextPage"`
			} `json:"pageInfo"`
			AiringSchedules []struct {
				AiringAt int64 `json:"airingAt"`
				Episode  int   `json:"episode"`
				Media    *struct {
					ID    int `json:"id"`
					Title struct {
						English string `json:"english"`
						Romaji  string `json:"romaji"`
						Native  string `json:"native"`
					} `json:"title"`
					CoverImage struct {
						ExtraLarge string `json:"extraLarge"`
						Large      string `json:"large"`
					} `json:"coverImage"`
					Genres          []string `json:"genres"`
					Format          string   `json:"format"`
					Episodes        int      `json:"episodes"`
					IsAdult         bool     `json:"isAdult"`
					CountryOfOrigin string   `json:"countryOfOrigin"`
					Studios         struct {
						Nodes []struct {
							Name string `json:"name"`
						} `json:"nodes"`
					} `json:"studios"`
				} `json:"media"`
			} `json:"airingSchedules"`
		} `json:"Page"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// scheduleFormats are the formats a weekly release calendar is about.
var scheduleFormats = map[string]bool{"TV": true, "TV_SHORT": true, "ONA": true}

// GetSchedule returns the episodes airing in (from, to). Results are cached
// for a few minutes; pages beyond the first are fetched concurrently.
func (s *AnimeCatalogService) GetSchedule(ctx context.Context, from, to int64) (*ScheduleResponse, error) {
	if to <= from || time.Duration(to-from)*time.Second > scheduleMaxSpan {
		return nil, fmt.Errorf("invalid schedule range")
	}
	key := fmt.Sprintf("%d-%d", from, to)
	policy := kv.Policy[ScheduleResponse]{TTLFor: func(r *ScheduleResponse) time.Duration {
		if r.Partial {
			return schedulePartialTTL
		}
		return scheduleCacheTTL
	}}
	return s.scheduleC.Get(ctx, key, policy, func(ctx context.Context) (*ScheduleResponse, error) {
		return s.loadSchedule(ctx, from, to)
	})
}

func (s *AnimeCatalogService) loadSchedule(ctx context.Context, from, to int64) (*ScheduleResponse, error) {
	// AniList caps lastPage/total, so page until hasNextPage is false: page 1
	// alone (most ranges end there), then batches of concurrent pages.
	first, err := s.fetchSchedulePage(ctx, from, to, 1)
	if err != nil {
		return nil, err
	}
	pages := []*schedulePage{first}
	partial := false
	more := first.Data.Page.PageInfo.HasNextPage
	for start := 2; more && start <= scheduleMaxPages; start += scheduleWorkers {
		batch := make([]*schedulePage, scheduleWorkers)
		var wg sync.WaitGroup
		for i := 0; i < scheduleWorkers && start+i <= scheduleMaxPages; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				if page, err := s.fetchSchedulePage(ctx, from, to, start+i); err == nil {
					batch[i] = page
				}
			}(i)
		}
		wg.Wait()
		more = false
		for i, page := range batch {
			if start+i > scheduleMaxPages {
				break
			}
			if page == nil {
				// A rate-limited burst drops the latest pages (sorted by time), which
				// empties the end of the range; retry them one by one before giving up.
				if retried, err := s.fetchSchedulePage(ctx, from, to, start+i); err == nil {
					page = retried
				} else {
					partial = true
					continue
				}
			}
			pages = append(pages, page)
			if page.Data.Page.PageInfo.HasNextPage {
				more = true
			}
		}
		if more && start+scheduleWorkers > scheduleMaxPages {
			partial = true // more pages exist than we are willing to read
		}
	}

	resp := &ScheduleResponse{From: from, To: to, Partial: partial, Entries: []ScheduleEntry{}}
	seen := map[[2]int64]bool{}
	for _, page := range pages {
		for _, node := range page.Data.Page.AiringSchedules {
			m := node.Media
			if m == nil || m.IsAdult || m.CountryOfOrigin != "JP" || !scheduleFormats[m.Format] {
				continue
			}
			if id := [2]int64{int64(m.ID), int64(node.Episode)}; seen[id] {
				continue
			} else {
				seen[id] = true
			}
			title := m.Title.English
			if title == "" {
				title = m.Title.Romaji
			}
			if title == "" {
				title = m.Title.Native
			}
			poster := m.CoverImage.ExtraLarge
			if poster == "" {
				poster = m.CoverImage.Large
			}
			entry := ScheduleEntry{AiringAt: node.AiringAt, Episode: node.Episode, MediaID: m.ID, Title: title, PosterImage: poster, Genres: m.Genres, Format: m.Format, Episodes: m.Episodes}
			if len(m.Studios.Nodes) > 0 {
				entry.Studio = m.Studios.Nodes[0].Name
			}
			resp.Entries = append(resp.Entries, entry)
		}
	}
	sort.SliceStable(resp.Entries, func(i, j int) bool {
		if resp.Entries[i].AiringAt != resp.Entries[j].AiringAt {
			return resp.Entries[i].AiringAt < resp.Entries[j].AiringAt
		}
		return resp.Entries[i].MediaID < resp.Entries[j].MediaID
	})

	return resp, nil
}

// fetchSchedulePage retries once after a short 429 pause; longer pauses are returned to the caller.
func (s *AnimeCatalogService) fetchSchedulePage(ctx context.Context, from, to int64, page int) (*schedulePage, error) {
	for attempt := 0; ; attempt++ {
		result, err := s.fetchSchedulePageOnce(ctx, from, to, page)
		var limited *RateLimitError
		if !errors.As(err, &limited) || attempt >= 1 || limited.RetryAfter > scheduleMaxWait {
			return result, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(limited.RetryAfter):
		}
	}
}

func (s *AnimeCatalogService) fetchSchedulePageOnce(ctx context.Context, from, to int64, page int) (*schedulePage, error) {
	var parsed schedulePage
	if err := s.anilist.post(ctx, scheduleQuery, map[string]any{"page": page, "perPage": schedulePageSize, "from": from, "to": to}, &parsed); err != nil {
		return nil, err
	}
	if len(parsed.Errors) > 0 {
		return nil, fmt.Errorf("anilist: %s", parsed.Errors[0].Message)
	}
	return &parsed, nil
}
