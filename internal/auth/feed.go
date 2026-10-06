package auth

import "net/http"

// NewCalendarFeed creates (or replaces) the secret token of the signed-in account's calendar
// feed. The token is shown once: only its hash is stored, and a new call invalidates the old URL.
func (s *Service) NewCalendarFeed(w http.ResponseWriter, r *http.Request) {
	u := s.currentUser(r)
	if u == nil {
		fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !s.sameOrigin(r) {
		fail(w, http.StatusForbidden, "forbidden")
		return
	}
	token := randomToken()
	if err := s.store.SetFeedToken(u.ID, hashToken(token), s.now()); err != nil {
		fail(w, http.StatusInternalServerError, "server_error")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"token": token})
}

// FeedUser resolves a calendar feed token to its account.
func (s *Service) FeedUser(token string) (int64, bool) {
	if token == "" {
		return 0, false
	}
	id, err := s.store.UserIDByFeedToken(hashToken(token))
	return id, err == nil
}

// WatchlistOf returns the saved anime of the account behind a feed token (most recent first).
func (s *Service) WatchlistOf(token string) []int64 {
	id, ok := s.FeedUser(token)
	if !ok {
		return nil
	}
	ids, _ := s.store.ListWatchlist(id)
	return ids
}
