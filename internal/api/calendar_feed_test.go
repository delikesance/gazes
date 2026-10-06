package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"

	"github.com/gazes/gazes/internal/auth"
	"github.com/gazes/gazes/internal/metadata"
)

func TestICSTextEscapesBackslashBeforeTheOtherSpecials(t *testing.T) {
	if got, want := icsText("Fate\\Zero; Re:Zero, vol. 2\nfin"), `Fate\\Zero\; Re:Zero\, vol. 2\nfin`; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestICSFoldLineStaysUnder75OctetsAndKeepsCharactersWhole(t *testing.T) {
	line := "SUMMARY:" + strings.Repeat("épisode ", 20)
	folded := foldLine(line)
	parts := strings.Split(folded, "\r\n")
	if len(parts) < 2 {
		t.Fatal("a long line must fold")
	}
	var unfolded strings.Builder
	for i, p := range parts {
		if len(p) > 75 || !utf8.ValidString(p) {
			t.Fatalf("part %d is %d octets or splits a character: %q", i, len(p), p)
		}
		if i > 0 {
			if p[0] != ' ' {
				t.Fatalf("continuation must start with a space: %q", p)
			}
			p = p[1:]
		}
		unfolded.WriteString(p)
	}
	if unfolded.String() != line {
		t.Fatal("unfolding must give the original line")
	}
}

func TestBuildCalendarListsOnlyTheSavedAnimeAirings(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	owners := map[int]feedAnime{10: {ID: 1, Title: "Show; One"}}
	entries := []metadata.ScheduleEntry{
		{MediaID: 10, Episode: 3, AiringAt: 1_700_100_000},
		{MediaID: 99, Episode: 1, AiringAt: 1_700_200_000},
	}
	ics := buildCalendar("https://gazes.example", owners, entries, now)
	for _, want := range []string{"BEGIN:VCALENDAR\r\n", "UID:10-3@gazes\r\n", "SUMMARY:Show\\; One · épisode 3\r\n", "URL:https://gazes.example/anime/1\r\n", "DTSTART:20231116T", "REFRESH-INTERVAL;VALUE=DURATION:PT6H\r\n", "END:VCALENDAR"} {
		if !strings.Contains(ics, want) {
			t.Fatalf("missing %q in:\n%s", want, ics)
		}
	}
	if strings.Contains(ics, "UID:99-") {
		t.Fatal("an anime outside the list must not appear")
	}
	if strings.Count(ics, "BEGIN:VEVENT") != 1 {
		t.Fatalf("one event expected:\n%s", ics)
	}
	if empty := buildCalendar("https://gazes.example", owners, nil, now); !strings.Contains(empty, "END:VCALENDAR") || strings.Contains(empty, "VEVENT") {
		t.Fatal("an empty feed is still a valid calendar")
	}
}

func TestCalendarForFailsOnlyWhenEverythingWasThrottled(t *testing.T) {
	s := &Server{catalogService: metadata.NewAnimeCatalogService(&http.Client{Transport: throttledTransport{}})}
	_, err := s.calendarFor(context.Background(), []int64{1, 2}, "https://gazes.example", time.Now())
	var limited *metadata.RateLimitError
	if !errors.As(err, &limited) {
		t.Fatalf("a fully throttled build must report the throttle so the app retries, got %v", err)
	}
}

func TestCalendarFeedCacheExpiresAndIsBounded(t *testing.T) {
	var c calendarFeeds
	now := time.Now()
	c.put("a", "ICS", now)
	if got, ok := c.get("a", now.Add(calendarFeedTTL-time.Second)); !ok || got != "ICS" {
		t.Fatal("a fresh feed must be served from the cache")
	}
	if _, ok := c.get("a", now.Add(calendarFeedTTL)); ok {
		t.Fatal("an expired feed must be rebuilt")
	}
	for i := 0; i < calendarFeedMaxCache+50; i++ {
		c.put(fmt.Sprint(i), "x", now)
	}
	if len(c.items) > calendarFeedMaxCache {
		t.Fatalf("cache grew to %d", len(c.items))
	}
}

func TestCalendarFeedUnknownTokenIs404(t *testing.T) {
	svc, err := auth.New(auth.Options{Dir: t.TempDir(), Getenv: func(string) string { return "" }})
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	s := &Server{auth: svc}
	router := chi.NewRouter()
	router.Get("/calendar/{token}", s.HandleCalendarFeed)
	for _, tok := range []string{"nope.ics", "x"} {
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, httptest.NewRequest("GET", "/calendar/"+tok, nil))
		if rr.Code != http.StatusNotFound {
			t.Fatalf("%s: %d", tok, rr.Code)
		}
	}
}

func TestBuildCalendarOmitsTheURLWithoutASiteOrigin(t *testing.T) {
	owners := map[int]feedAnime{10: {ID: 1, Title: "Show"}}
	ics := buildCalendar("", owners, []metadata.ScheduleEntry{{MediaID: 10, Episode: 1, AiringAt: 1_700_100_000}}, time.Unix(1_700_000_000, 0))
	if strings.Contains(ics, "URL:") || !strings.Contains(ics, "BEGIN:VEVENT") {
		t.Fatalf("event without a URL expected:\n%s", ics)
	}
}
