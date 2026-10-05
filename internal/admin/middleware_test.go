package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type fakeUsers map[int64]string

func (f fakeUsers) UserRole(_ context.Context, id int64) (string, error) {
	r, ok := f[id]
	if !ok {
		return "", context.Canceled
	}
	return r, nil
}

var ok200 = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })

// cookieIdentify mimics a session cookie: "sid=<n>" identifies user n.
func cookieIdentify(r *http.Request) (int64, bool) {
	c, err := r.Cookie("sid")
	if err != nil {
		return 0, false
	}
	switch c.Value {
	case "1":
		return 1, true
	case "2":
		return 2, true
	}
	return 0, false
}

func do(h http.Handler, mut func(*http.Request)) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", "/x", nil)
	if mut != nil {
		mut(req)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestRequireToken(t *testing.T) {
	s := newTokenStore(t)
	ctx := context.Background()
	good, _, _ := s.CreateToken(ctx, "g", []string{ScopeMetricsRead}, 0)
	revoked, rid, _ := s.CreateToken(ctx, "r", []string{ScopeMetricsRead}, 0)
	s.RevokeToken(ctx, rid)
	expired, eid, _ := s.CreateToken(ctx, "e", []string{ScopeMetricsRead}, time.Hour)
	s.db.Exec(`UPDATE admin_tokens SET expires_at = 1 WHERE id = ?`, eid)
	h := RequireToken(s, ScopeMetricsRead)(ok200)
	bearer := func(v string) func(*http.Request) { return func(r *http.Request) { r.Header.Set("Authorization", v) } }
	tests := []struct {
		name string
		mut  func(*http.Request)
		h    http.Handler
		code int
	}{
		{"no credentials", nil, h, 401},
		{"valid session cookie without bearer", func(r *http.Request) { r.AddCookie(&http.Cookie{Name: "sid", Value: "1"}) }, h, 401},
		{"basic scheme", bearer("Basic " + good), h, 401},
		{"unknown", bearer("Bearer gzs_nope"), h, 401},
		{"revoked", bearer("Bearer " + revoked), h, 401},
		{"expired", bearer("Bearer " + expired), h, 401},
		{"missing scope", bearer("Bearer " + good), RequireToken(s, ScopeOpsWrite)(ok200), 403},
		{"ok", bearer("Bearer " + good), h, 200},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := do(tc.h, tc.mut)
			if rec.Code != tc.code {
				t.Fatalf("code = %d, want %d", rec.Code, tc.code)
			}
			if tc.code == 401 && rec.Header().Get("WWW-Authenticate") != "Bearer" {
				t.Fatal("missing WWW-Authenticate")
			}
		})
	}
	// Same body for unknown, revoked and expired.
	a, b, c := do(h, bearer("Bearer gzs_nope")).Body.String(), do(h, bearer("Bearer "+revoked)).Body.String(), do(h, bearer("Bearer "+expired)).Body.String()
	if a != b || b != c {
		t.Fatalf("bodies differ: %q %q %q", a, b, c)
	}
}

func TestRequireTokenContextAndRateLimit(t *testing.T) {
	s := newTokenStore(t)
	ctx := context.Background()
	tok, id, _ := s.CreateToken(ctx, "g", []string{ScopeMetricsRead}, 0)
	other, oid, _ := s.CreateToken(ctx, "o", []string{ScopeMetricsRead}, 0)
	now := time.Now()
	var seen *Token
	h := RequireTokenWith(s, TokenOptions{RatePerMinute: 3, Now: func() time.Time { return now }}, ScopeMetricsRead)(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { seen, _ = TokenFromContext(r.Context()) }))
	auth := func(v string) func(*http.Request) {
		return func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+v) }
	}
	for i := 0; i < 3; i++ {
		if rec := do(h, auth(tok)); rec.Code != 200 {
			t.Fatalf("request %d: %d", i, rec.Code)
		}
	}
	if seen == nil || seen.ID != id {
		t.Fatalf("token not in context: %+v", seen)
	}
	rec := do(h, auth(tok))
	if rec.Code != 429 || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("code = %d retry-after = %q", rec.Code, rec.Header().Get("Retry-After"))
	}
	if rec := do(h, auth(other)); rec.Code != 200 || seen.ID != oid {
		t.Fatalf("other token limited: %d", rec.Code)
	}
	now = now.Add(21 * time.Second) // 3/min refills one token every 20s
	if rec := do(h, auth(tok)); rec.Code != 200 {
		t.Fatalf("after refill: %d", rec.Code)
	}
	if _, ok := TokenFromContext(context.Background()); ok {
		t.Fatal("empty context has token")
	}
}

func TestRequireAdminSession(t *testing.T) {
	s := newTokenStore(t)
	users := fakeUsers{1: "admin", 2: "user"}
	h := RequireAdminSession(cookieIdentify, users)(ok200)
	cookie := func(v string) func(*http.Request) {
		return func(r *http.Request) { r.AddCookie(&http.Cookie{Name: "sid", Value: v}) }
	}
	plain, _, _ := s.CreateToken(context.Background(), "g", []string{ScopeMetricsRead}, 0)
	tests := []struct {
		name string
		mut  func(*http.Request)
		code int
	}{
		{"anonymous", nil, 401},
		{"non admin", cookie("2"), 403},
		{"admin", cookie("1"), 200},
		{"valid token is not a session", func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+plain) }, 401},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if rec := do(h, tc.mut); rec.Code != tc.code {
				t.Fatalf("code = %d, want %d", rec.Code, tc.code)
			}
		})
	}
	// Conversely an admin session does not pass RequireToken (covered by "valid session cookie").
	rec := do(RequireToken(s, ScopeMetricsRead)(ok200), cookie("1"))
	if rec.Code != 401 {
		t.Fatalf("admin session passed RequireToken: %d", rec.Code)
	}
}
