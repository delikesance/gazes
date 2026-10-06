package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newFeedToken(t *testing.T, s *Service, c *http.Cookie) string {
	t.Helper()
	req := httptest.NewRequest("POST", "/me/calendar-feed", nil)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(c)
	rr := httptest.NewRecorder()
	s.NewCalendarFeed(rr, req)
	var out struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil || rr.Code != http.StatusOK || len(out.Token) < 40 {
		t.Fatalf("feed token: %d %s", rr.Code, rr.Body)
	}
	return out.Token
}

func TestCalendarFeedTokenResolvesToItsOwnerAndRotates(t *testing.T) {
	s := newTestService(t)
	c := registerForTest(t, s, "feed@example.com", "feeder")
	u := s.CurrentUser(reqWith(c))
	first := newFeedToken(t, s, c)
	if id, ok := s.FeedUser(first); !ok || id != u.ID {
		t.Fatalf("token must resolve to its owner: %d %v", id, ok)
	}
	if _, ok := s.FeedUser("not-a-token"); ok {
		t.Fatal("unknown token accepted")
	}
	if _, ok := s.FeedUser(""); ok {
		t.Fatal("empty token accepted")
	}
	second := newFeedToken(t, s, c)
	if second == first {
		t.Fatal("a new token must differ")
	}
	if _, ok := s.FeedUser(first); ok {
		t.Fatal("the old token must stop working once rotated")
	}
	var stored string
	if err := s.store.db.QueryRow(`SELECT token_hash FROM calendar_feeds WHERE user_id = ?`, u.ID).Scan(&stored); err != nil || stored == second {
		t.Fatal("the token must be stored hashed, never in clear")
	}
}

func TestCalendarFeedNeedsASessionAndDiesWithTheAccount(t *testing.T) {
	s := newTestService(t)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/me/calendar-feed", nil)
	req.Header.Set("Content-Type", "application/json")
	s.NewCalendarFeed(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous must be refused, got %d", rr.Code)
	}
	c := registerForTest(t, s, "gone@example.com", "goner")
	tok := newFeedToken(t, s, c)
	u := s.CurrentUser(reqWith(c))
	_ = s.store.UpdateWatchlist(u.ID, []int64{4, 9}, nil, time.Now())
	if ids := s.WatchlistOf(tok); len(ids) != 2 {
		t.Fatalf("watchlist behind the token: %v", ids)
	}
	if err := s.store.DeleteUser(u.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.FeedUser(tok); ok {
		t.Fatal("the token must die with the account")
	}
}
