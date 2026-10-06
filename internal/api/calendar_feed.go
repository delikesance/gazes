package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"

	"github.com/gazes/gazes/internal/metadata"
)

const (
	// A feed is rebuilt at most this often per account: calendar apps poll on their own schedule,
	// and building one resolves franchises and reads the airing schedule.
	calendarFeedTTL = 6 * time.Hour
	// Only the most recently saved anime make it into a feed, to bound the franchise lookups.
	calendarFeedMaxAnime = 100
	calendarFeedSpan     = 42 * 24 * time.Hour
	calendarFeedMaxCache = 256
)

type feedAnime struct {
	ID    int
	Title string
}

type cachedCalendar struct {
	ics   string
	built time.Time
}

// calendarFeeds keeps built feeds in memory, bounded; the zero value is ready to use.
type calendarFeeds struct {
	mu    sync.Mutex
	items map[string]cachedCalendar
}

func (c *calendarFeeds) get(key string, now time.Time) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	it, ok := c.items[key]
	if !ok || now.Sub(it.built) >= calendarFeedTTL {
		return "", false
	}
	return it.ics, true
}

func (c *calendarFeeds) put(key, ics string, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.items == nil {
		c.items = map[string]cachedCalendar{}
	}
	if len(c.items) >= calendarFeedMaxCache {
		for k := range c.items { // arbitrary victim: the cache only exists to spare upstream calls
			delete(c.items, k)
			break
		}
	}
	c.items[key] = cachedCalendar{ics: ics, built: now}
}

// icsText escapes a TEXT value (RFC 5545 §3.3.11): backslash first, then ; , and newlines.
func icsText(text string) string {
	r := strings.NewReplacer("\\", "\\\\", ";", "\\;", ",", "\\,", "\r\n", "\\n", "\n", "\\n")
	return r.Replace(text)
}

// foldLine folds a content line at 75 octets (RFC 5545 §3.1), never inside a UTF-8 character.
func foldLine(line string) string {
	var out strings.Builder
	size := 0
	for _, r := range line {
		n := utf8.RuneLen(r)
		if size+n > 75 {
			out.WriteString("\r\n ")
			size = 1 // the continuation space counts toward the 75 octets
		}
		out.WriteRune(r)
		size += n
	}
	return out.String()
}

func icsStamp(unix int64) string { return time.Unix(unix, 0).UTC().Format("20060102T150405Z") }

// buildCalendar renders the airings of the saved anime as an iCalendar feed. owners maps a season's
// media id to its franchise.
func buildCalendar(origin string, owners map[int]feedAnime, entries []metadata.ScheduleEntry, now time.Time) string {
	lines := []string{"BEGIN:VCALENDAR", "VERSION:2.0", "PRODID:-//Gazes//Ma liste//FR", "CALSCALE:GREGORIAN",
		"X-WR-CALNAME:Gazes · Ma liste", "REFRESH-INTERVAL;VALUE=DURATION:PT6H", "X-PUBLISHED-TTL:PT6H"}
	for _, e := range entries {
		anime, ok := owners[e.MediaID]
		if !ok {
			continue
		}
		lines = append(lines, "BEGIN:VEVENT",
			fmt.Sprintf("UID:%d-%d@gazes", e.MediaID, e.Episode),
			"DTSTAMP:"+icsStamp(now.Unix()),
			"DTSTART:"+icsStamp(e.AiringAt),
			"DTEND:"+icsStamp(e.AiringAt+1440),
			"SUMMARY:"+icsText(fmt.Sprintf("%s · épisode %d", anime.Title, e.Episode)))
		// A relative URL is not valid in a calendar: without a configured site origin, leave it out.
		if origin != "" {
			lines = append(lines, fmt.Sprintf("URL:%s/anime/%d", origin, anime.ID))
		}
		lines = append(lines, "END:VEVENT")
	}
	lines = append(lines, "END:VCALENDAR")
	for i, l := range lines {
		lines[i] = foldLine(l)
	}
	return strings.Join(lines, "\r\n") + "\r\n"
}

// calendarFor resolves the saved anime to their franchises (cached upstream) and renders the feed.
// It fails only when nothing could be resolved because of a throttle, so the app retries later.
func (s *Server) calendarFor(ctx context.Context, ids []int64, origin string, now time.Time) (string, error) {
	if len(ids) > calendarFeedMaxAnime {
		ids = ids[:calendarFeedMaxAnime]
	}
	owners := map[int]feedAnime{}
	var firstErr error
	resolved := 0
	for _, id := range ids {
		f, err := s.catalogService.GetFranchise(ctx, int(id))
		if err != nil {
			var limited *metadata.RateLimitError
			if errors.As(err, &limited) && firstErr == nil {
				firstErr = err
			}
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			continue
		}
		resolved++
		for _, season := range f.Seasons {
			owners[season.ID] = feedAnime{ID: f.ID, Title: f.Title}
		}
	}
	if resolved == 0 && firstErr != nil {
		return "", firstErr
	}
	from := now.Unix()
	schedule, err := s.catalogService.GetSchedule(ctx, from, from+int64(calendarFeedSpan.Seconds())-60)
	if err != nil {
		return "", err
	}
	return buildCalendar(origin, owners, schedule.Entries, now), nil
}

// HandleCalendarFeed serves GET /api/v1/calendar/{token}.ics: the secret URL a calendar app
// subscribes to. The token is the only credential, so unknown tokens get a plain 404.
func (s *Server) HandleCalendarFeed(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSuffix(chi.URLParam(r, "token"), ".ics")
	if s.auth == nil {
		http.NotFound(w, r)
		return
	}
	userID, ok := s.auth.FeedUser(token)
	if !ok {
		http.NotFound(w, r)
		return
	}
	now := time.Now()
	key := fmt.Sprint(userID)
	ics, cached := s.calendarCache.get(key, now)
	if !cached {
		origin := strings.TrimRight(s.cfg.SiteURL, "/")
		var err error
		if ics, err = s.calendarFor(r.Context(), s.auth.WatchlistOf(token), origin, now); err != nil {
			s.catalogFailure(w, r, "failed to build a calendar feed", "calendar temporarily unavailable", err)
			return
		}
		s.calendarCache.put(key, ics, now)
	}
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	_, _ = w.Write([]byte(ics))
}
