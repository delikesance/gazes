package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gazes/gazes/internal/auth"
	"github.com/go-chi/chi/v5"
)

func at(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestParsePeriod(t *testing.T) {
	tests := []struct {
		name, query, now string
		wantErr          bool
		want             Period
	}{
		{"default 30", "", "2026-03-31T15:00:00Z", false, Period{30, "2026-03-02", "2026-03-31", "2026-01-31", "2026-03-01"}},
		{"7", "?period=7", "2026-03-31T00:00:00Z", false, Period{7, "2026-03-25", "2026-03-31", "2026-03-18", "2026-03-24"}},
		{"30", "?period=30", "2026-03-31T23:59:59Z", false, Period{30, "2026-03-02", "2026-03-31", "2026-01-31", "2026-03-01"}},
		{"90", "?period=90", "2026-03-31T10:00:00Z", false, Period{90, "2026-01-01", "2026-03-31", "2025-10-03", "2025-12-31"}},
		{"year boundary", "?period=7", "2026-01-03T10:00:00Z", false, Period{7, "2025-12-28", "2026-01-03", "2025-12-21", "2025-12-27"}},
		{"leap day", "?period=7", "2024-03-02T10:00:00Z", false, Period{7, "2024-02-25", "2024-03-02", "2024-02-18", "2024-02-24"}},
		{"non-UTC instant", "?period=7", "2026-03-31T23:30:00-05:00", false, Period{7, "2026-03-26", "2026-04-01", "2026-03-19", "2026-03-25"}},
		{"invalid number", "?period=14", "2026-03-31T10:00:00Z", true, Period{}},
		{"invalid text", "?period=abc", "2026-03-31T10:00:00Z", true, Period{}},
		{"zero", "?period=0", "2026-03-31T10:00:00Z", true, Period{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parsePeriod(httptest.NewRequest("GET", "/x"+tc.query, nil), at(tc.now))
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestParsePagination(t *testing.T) {
	tests := []struct {
		query   string
		want    Page
		wantErr bool
	}{
		{"", Page{50, 0}, false},
		{"?limit=10&offset=20", Page{10, 20}, false},
		{"?limit=200", Page{200, 0}, false},
		{"?limit=201", Page{200, 0}, false},
		{"?limit=100000", Page{200, 0}, false},
		{"?limit=0", Page{}, true},
		{"?limit=-1", Page{}, true},
		{"?limit=x", Page{}, true},
		{"?offset=-5", Page{}, true},
		{"?offset=x", Page{}, true},
	}
	for _, tc := range tests {
		got, err := parsePagination(httptest.NewRequest("GET", "/x"+tc.query, nil))
		if (err != nil) != tc.wantErr || got != tc.want {
			t.Errorf("%q: got %+v, %v; want %+v, err %v", tc.query, got, err, tc.want, tc.wantErr)
		}
	}
}

func TestDelta(t *testing.T) {
	tests := []struct {
		cur, prev float64
		want      *float64
	}{
		{110, 100, ptr(10)},
		{50, 100, ptr(-50)},
		{0, 100, ptr(-100)},
		{5, 0, nil},
		{0, 0, nil},
	}
	for _, tc := range tests {
		got := delta(tc.cur, tc.prev)
		if (got == nil) != (tc.want == nil) || (got != nil && *got != *tc.want) {
			t.Errorf("delta(%v,%v) = %v, want %v", tc.cur, tc.prev, got, tc.want)
		}
	}
}

func ptr(f float64) *float64 { return &f }

func TestWriteHelpers(t *testing.T) {
	rec := httptest.NewRecorder()
	writeData(rec, Period{Days: 7, From: "a", To: "b"}, map[string]int{"n": 1})
	if rec.Code != 200 || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("code %d, cache %q", rec.Code, rec.Header().Get("Cache-Control"))
	}
	var env struct {
		GeneratedAt string         `json:"generated_at"`
		Period      Period         `json:"period"`
		Data        map[string]int `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if _, err := time.Parse(time.RFC3339, env.GeneratedAt); err != nil || env.Period.Days != 7 || env.Data["n"] != 1 {
		t.Fatalf("bad envelope %+v (%v)", env, err)
	}

	rec = httptest.NewRecorder()
	writeAPIError(rec, 400, "bad_period", "nope")
	var e struct {
		Error struct{ Code, Message string }
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil {
		t.Fatal(err)
	}
	if rec.Code != 400 || e.Error.Code != "bad_period" || e.Error.Message != "nope" || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("bad error %d %+v", rec.Code, e)
	}
}

func newTestService(t *testing.T) *Service {
	t.Helper()
	dir := t.TempDir()
	as, err := auth.OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { as.Close() })
	st, err := Open(filepath.Join(dir, "admin.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	svc, err := NewService(st, filepath.Join(dir, "accounts.sqlite"),
		WithClock(func() time.Time { return at("2026-03-31T10:00:00Z") }),
		WithSessionAuth(cookieIdentify, fakeUsers{1: "admin", 2: "user"}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { svc.Close() })
	return svc
}

func TestMe(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	metrics, _, _ := svc.store.CreateToken(ctx, "m", []string{ScopeMetricsRead}, 0)
	diag, _, _ := svc.store.CreateToken(ctx, "d", []string{ScopeDiagnosticsRead}, 0)
	r := chi.NewRouter()
	svc.Mount(r)

	cookie := func(v string) func(*http.Request) {
		return func(r *http.Request) { r.AddCookie(&http.Cookie{Name: "sid", Value: v}) }
	}
	bearerWith := func(tok string, cookieVal string) func(*http.Request) {
		return func(r *http.Request) {
			r.Header.Set("Authorization", "Bearer "+tok)
			if cookieVal != "" {
				r.AddCookie(&http.Cookie{Name: "sid", Value: cookieVal})
			}
		}
	}
	tests := []struct {
		name       string
		mut        func(*http.Request)
		status     int
		via        string
		wantScopes []string
	}{
		{"no credentials", nil, 401, "", nil},
		{"invalid bearer with valid admin cookie: no fallback", bearerWith("gzs_bad", "1"), 401, "", nil},
		{"non-bearer Authorization with admin cookie: no fallback", func(r *http.Request) {
			r.Header.Set("Authorization", "Basic abc")
			r.AddCookie(&http.Cookie{Name: "sid", Value: "1"})
		}, 401, "", nil},
		{"non admin session", cookie("2"), 403, "", nil},
		{"admin session", cookie("1"), 200, "session", allScopes},
		{"token with scope", bearerWith(metrics, ""), 200, "token", []string{ScopeMetricsRead}},
		{"token without scope", bearerWith(diag, ""), 403, "", nil},
		{"token without scope, admin cookie: no fallback", bearerWith(diag, "1"), 403, "", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/v1/admin/me", nil)
			if tc.mut != nil {
				tc.mut(req)
			}
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			if rec.Code != tc.status {
				t.Fatalf("status %d, want %d (%s)", rec.Code, tc.status, rec.Body)
			}
			if tc.status != 200 {
				return
			}
			var env struct {
				Data struct {
					Via    string   `json:"via"`
					Role   string   `json:"role"`
					Scopes []string `json:"scopes"`
				} `json:"data"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
				t.Fatal(err)
			}
			if env.Data.Via != tc.via || len(env.Data.Scopes) != len(tc.wantScopes) {
				t.Fatalf("got %+v", env.Data)
			}
			for _, k := range []string{"email", "hash", "pseudo"} {
				if bytes.Contains(rec.Body.Bytes(), []byte(k)) {
					t.Fatalf("response leaks %q: %s", k, rec.Body)
				}
			}
		})
	}
}

func TestServiceDatabases(t *testing.T) {
	svc := newTestService(t)
	if _, err := svc.accountsDB().Exec(`DELETE FROM users`); err == nil {
		t.Fatal("accounts database must be read-only")
	}
	if _, err := svc.adminDB().Exec(`DELETE FROM metrics_daily`); err != nil {
		t.Fatal(err)
	}
	if err := svc.BackfillIfEmpty(context.Background(), 3); err != nil {
		t.Fatal(err)
	}
	var n int
	svc.adminDB().QueryRow(`SELECT COUNT(*) FROM metrics_daily`).Scan(&n)
	if n != 3 {
		t.Fatalf("backfill wrote %d days, want 3", n)
	}
}

func TestSessionWritesNeedCSRFHeader(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	ops, _, _ := svc.store.CreateToken(ctx, "ops", []string{ScopeOpsWrite}, 0)
	r := chi.NewRouter()
	r.With(svc.Auth(ScopeOpsWrite)).Post("/w", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) })
	r.With(svc.Auth(ScopeMetricsRead)).Get("/r", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) })

	do := func(method, path string, mut func(*http.Request)) int {
		req := httptest.NewRequest(method, path, nil)
		mut(req)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec.Code
	}
	admin := func(r *http.Request) { r.AddCookie(&http.Cookie{Name: "sid", Value: "1"}) }
	adminCSRF := func(r *http.Request) { admin(r); r.Header.Set("X-Gazes-Admin", "1") }
	bearer := func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+ops) }

	for _, tc := range []struct {
		name   string
		method string
		path   string
		mut    func(*http.Request)
		want   int
	}{
		{"session POST without header is refused", "POST", "/w", admin, 403},
		{"session POST with header", "POST", "/w", adminCSRF, 204},
		{"session GET needs no header", "GET", "/r", admin, 204},
		{"token POST needs no header", "POST", "/w", bearer, 204},
	} {
		if got := do(tc.method, tc.path, tc.mut); got != tc.want {
			t.Errorf("%s: status %d, want %d", tc.name, got, tc.want)
		}
	}
}
