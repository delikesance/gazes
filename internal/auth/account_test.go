package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

func registerForTest(t *testing.T, s *Service, email, pseudo string) *http.Cookie {
	t.Helper()
	c := sessionCookie(s.post(t, s.Register, "register", map[string]string{"email": email, "password": "longenough", "pseudo": pseudo}))
	if c == nil {
		t.Fatal("register failed")
	}
	return c
}

func authedGet(h http.HandlerFunc, c *http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", "/me/x", nil)
	req.AddCookie(c)
	rr := httptest.NewRecorder()
	h(rr, req)
	return rr
}

func TestDeleteAccountNeedsThePasswordAndErasesEverything(t *testing.T) {
	s := newTestService(t)
	c := registerForTest(t, s, "bye@example.com", "bye")
	other := registerForTest(t, s, "stay@example.com", "stay")
	u := s.CurrentUser(func() *http.Request { r := httptest.NewRequest("GET", "/", nil); r.AddCookie(c); return r }())
	now := time.Now().Unix()
	_ = s.store.UpdateWatchlist(u.ID, []int64{1}, nil, time.Now())
	_ = s.store.UpdateHidden(u.ID, []int64{2}, nil, time.Now())
	_ = s.store.MergeProgress(u.ID, []Progress{{SeasonID: 1, Episode: 1, Position: 5, UpdatedAt: now}})

	if rr := s.post(t, s.DeleteAccount, "delete-account", map[string]string{"password": "wrong-password"}, c); rr.Code != http.StatusForbidden || errCode(rr) != "invalid_credentials" {
		t.Fatalf("wrong password must be refused, got %d %s", rr.Code, rr.Body)
	}
	if s.CurrentUser(reqWith(c)) == nil {
		t.Fatal("a refused deletion must keep the account")
	}
	if rr := s.post(t, s.DeleteAccount, "delete-account", map[string]string{"password": "longenough"}); rr.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous must be refused, got %d", rr.Code)
	}

	var unlinked int64
	s.SetOnAccountDeleted(func(_ context.Context, id int64) { unlinked = id })
	rr := s.post(t, s.DeleteAccount, "delete-account", map[string]string{"password": "longenough"}, c)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", rr.Code, rr.Body)
	}
	if unlinked != u.ID {
		t.Fatalf("hook not called with the user id: %d", unlinked)
	}
	if s.CurrentUser(reqWith(c)) != nil {
		t.Fatal("the session must die with the account")
	}
	for _, table := range []string{"users", "sessions", "progress", "hidden_anime", "watchlist", "watch_sessions"} {
		var n int
		q := `SELECT COUNT(*) FROM ` + table + ` WHERE ` + map[bool]string{true: "id", false: "user_id"}[table == "users"] + ` = ?`
		if err := s.store.db.QueryRow(q, u.ID).Scan(&n); err != nil || n != 0 {
			t.Fatalf("%s still holds %d rows (%v)", table, n, err)
		}
	}
	if s.CurrentUser(reqWith(other)) == nil {
		t.Fatal("another account must be untouched")
	}
	if rr := s.post(t, s.Login, "login", map[string]string{"email": "bye@example.com", "password": "longenough"}); rr.Code != http.StatusUnauthorized {
		t.Fatalf("the deleted account must not log in, got %d", rr.Code)
	}
}

func reqWith(c *http.Cookie) *http.Request {
	r := httptest.NewRequest("GET", "/", nil)
	r.AddCookie(c)
	return r
}

func TestExportReturnsTheAccountData(t *testing.T) {
	s := newTestService(t)
	c := registerForTest(t, s, "me@example.com", "memo")
	u := s.CurrentUser(reqWith(c))
	_ = s.store.UpdateWatchlist(u.ID, []int64{7}, nil, time.Now())
	_ = s.store.UpdateHidden(u.ID, []int64{9}, nil, time.Now())
	_ = s.store.MergeProgress(u.ID, []Progress{{SeasonID: 3, Episode: 2, Position: 40, UpdatedAt: time.Now().Unix()}})

	rr := authedGet(s.Export, c)
	if rr.Code != http.StatusOK || rr.Header().Get("Cache-Control") != "no-store" || rr.Header().Get("Content-Disposition") == "" {
		t.Fatalf("export: %d %v", rr.Code, rr.Header())
	}
	var out struct {
		Pseudo    string     `json:"pseudo"`
		Email     string     `json:"email"`
		Watchlist []int64    `json:"watchlist"`
		Hidden    []int64    `json:"hidden"`
		Progress  []Progress `json:"progress"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Pseudo != "memo" || out.Email != "me@example.com" || len(out.Watchlist) != 1 || len(out.Hidden) != 1 || len(out.Progress) != 1 {
		t.Fatalf("incomplete export: %+v", out)
	}
	anon := httptest.NewRecorder()
	s.Export(anon, httptest.NewRequest("GET", "/me/export", nil))
	if anon.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous must be refused, got %d", anon.Code)
	}
}

func TestSessionsAreListedAndRevocableByTheirOwner(t *testing.T) {
	s := newTestService(t)
	c := registerForTest(t, s, "dev@example.com", "devone")
	other := registerForTest(t, s, "dev2@example.com", "devtwo")
	u := s.CurrentUser(reqWith(c))
	// a second device of the same user
	second := randomToken()
	if err := s.store.CreateSession(hashToken(second), u.ID, time.Now().Add(time.Hour), time.Now()); err != nil {
		t.Fatal(err)
	}

	var list struct {
		Sessions []SessionInfo `json:"sessions"`
	}
	_ = json.Unmarshal(authedGet(s.ListSessions, c).Body.Bytes(), &list)
	if len(list.Sessions) != 2 {
		t.Fatalf("want 2 sessions, got %+v", list.Sessions)
	}
	var secondID string
	currents := 0
	for _, x := range list.Sessions {
		if x.Current {
			currents++
		} else {
			secondID = x.ID
		}
	}
	if currents != 1 || secondID == "" {
		t.Fatalf("exactly one current session expected: %+v", list.Sessions)
	}

	revoke := func(id string, by *http.Cookie) int {
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", id)
		req := httptest.NewRequest("DELETE", "/me/sessions/"+id, nil)
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(by)
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
		rr := httptest.NewRecorder()
		s.RevokeSession(rr, req)
		return rr.Code
	}
	if code := revoke(secondID, other); code != http.StatusNotFound {
		t.Fatalf("another user must not revoke it, got %d", code)
	}
	if code := revoke(secondID, c); code != http.StatusNoContent {
		t.Fatalf("revoke: %d", code)
	}
	r := httptest.NewRequest("GET", "/", nil)
	r.AddCookie(&http.Cookie{Name: cookieName, Value: second})
	if s.CurrentUser(r) != nil {
		t.Fatal("revoked session still valid")
	}
	if s.CurrentUser(reqWith(c)) == nil {
		t.Fatal("the current session must survive")
	}
}
