package admin

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gazes/gazes/internal/auth"
	"github.com/go-chi/chi/v5"
)

// usersEnv is a service on a fresh accounts database plus a writable handle to it.
type usersEnv struct {
	svc     *Service
	rw      *sql.DB
	h       http.Handler
	token   string // metrics:read
	badTok  string // diagnostics:read only
	now     time.Time
	nextSID int
}

func newUsersEnv(t *testing.T, now string) *usersEnv {
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
	e := &usersEnv{now: at(now)}
	svc, err := NewService(st, filepath.Join(dir, "accounts.sqlite"),
		WithClock(func() time.Time { return e.now }),
		WithSessionAuth(cookieIdentify, fakeUsers{1: "admin", 2: "user"}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { svc.Close() })
	rw, err := sql.Open("sqlite3", filepath.Join(dir, "accounts.sqlite")+"?_busy_timeout=5000")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { rw.Close() })
	e.svc, e.rw = svc, rw
	e.token, _, _ = st.CreateToken(context.Background(), "m", []string{ScopeMetricsRead}, 0)
	e.badTok, _, _ = st.CreateToken(context.Background(), "d", []string{ScopeDiagnosticsRead}, 0)
	r := chi.NewRouter()
	svc.Mount(r)
	e.h = r
	return e
}

func (e *usersEnv) user(t *testing.T, id int64, pseudo string, created int64) {
	t.Helper()
	if _, err := e.rw.Exec(`INSERT INTO users(id,email_idx,email_enc,pseudo,pass_hash,created_at) VALUES(?,?,?,?,?,?)`,
		id, fmt.Sprint("secret-idx-", id), "secret-enc", pseudo, "secret-hash", created); err != nil {
		t.Fatal(err)
	}
}

func (e *usersEnv) watch(t *testing.T, uid, anime int64, title string, ep int, started int64, secs float64) {
	t.Helper()
	e.nextSID++
	if _, err := e.rw.Exec(`INSERT INTO watch_sessions(user_id,session_id,season_id,anime_id,episode,title,started_at,updated_at,watched_seconds,duration,completed)
		VALUES(?,?,?,?,?,?,?,?,?,1400,0)`, uid, fmt.Sprint("s", e.nextSID), anime*10, anime, ep, title, started, started, secs); err != nil {
		t.Fatal(err)
	}
}

func (e *usersEnv) get(path string, mut func(*http.Request)) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", path, nil)
	if mut != nil {
		mut(req)
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return rec
}

func asAdmin(r *http.Request) { r.AddCookie(&http.Cookie{Name: "sid", Value: "1"}) }
func (e *usersEnv) asToken(r *http.Request) {
	r.Header.Set("Authorization", "Bearer "+e.token)
}

func (e *usersEnv) data(t *testing.T, path string, mut func(*http.Request)) map[string]any {
	t.Helper()
	rec := e.get(path, mut)
	if rec.Code != 200 {
		t.Fatalf("GET %s: %d %s", path, rec.Code, rec.Body)
	}
	var env struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	return env.Data
}

func num(t *testing.T, v any) float64 {
	t.Helper()
	f, ok := v.(float64)
	if !ok {
		t.Fatalf("not a number: %#v", v)
	}
	return f
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func list(t *testing.T, v any) []any {
	t.Helper()
	l, ok := v.([]any)
	if !ok {
		t.Fatalf("not a list: %#v", v)
	}
	return l
}

const usersNow = "2026-03-31T10:00:00Z"

// seedUsers: 9 accounts, one per segment case, "ago(d)" = d days before usersNow.
func seedUsers(t *testing.T, e *usersEnv) {
	ago := func(d float64) int64 { return e.now.Unix() - int64(d*86400) }
	e.user(t, 1, "alice", ago(100))
	e.user(t, 2, "bob_x", ago(100))
	e.user(t, 3, "carol%", ago(100))
	e.user(t, 4, "dave", ago(100))
	e.user(t, 5, "erin", ago(3))
	e.user(t, 6, "frank", ago(50))
	e.user(t, 7, "gina", ago(2))
	e.user(t, 8, "hal", ago(100))
	e.user(t, 9, "ivan", ago(200))
	// alice: power user, 4 x 20 h in the last 30 d + 1 h 35 d ago; Alpha is her top anime.
	for i, d := range []float64{1, 3, 10, 12} {
		e.watch(t, 1, 100, "Alpha", i+1, ago(d), 72000)
	}
	e.watch(t, 1, 200, "Beta", 1, ago(35), 3600)
	// bob: regular.
	e.watch(t, 2, 200, "Beta", 1, ago(2), 3600)
	e.watch(t, 2, 200, "Beta", 2, ago(10), 1800)
	// carol: dormant (45 d). ivan: dormant (70 d).
	e.watch(t, 3, 100, "Alpha", 1, ago(45), 600)
	e.watch(t, 9, 100, "Alpha", 1, ago(70), 600)
	// dave: at risk, 4 sessions 20-25 d ago.
	for i, d := range []float64{20, 21, 22, 25} {
		e.watch(t, 4, 300, "Gamma", i+1, ago(d), 600)
	}
	// erin: new, 12 sessions yesterday (detail is capped at 10).
	for i := 0; i < 12; i++ {
		e.watch(t, 5, 400, "Delta", i+1, ago(1)-int64(i*60), 60)
	}
	// sessions table: 5 valid (4 expiring within 7 d, 1 within 24 h, 1 in 24-48 h), 1 expired.
	for i, exp := range []float64{-3, 0.5, 1.5, 3, 5, 20} {
		if _, err := e.rw.Exec(`INSERT INTO sessions(token_hash,user_id,expires_at,last_seen) VALUES(?,?,?,?)`,
			fmt.Sprint("hash-", i), 1, e.now.Unix()+int64(exp*86400), e.now.Unix()); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := e.rw.Exec(`INSERT INTO progress(user_id,season_id,anime_id,title,episode,position,completed,updated_at) VALUES
		(1,1000,100,'Alpha',4,123.5,0,?),(1,2000,200,'Beta',1,10,1,?)`, ago(1), ago(5)); err != nil {
		t.Fatal(err)
	}
}

func TestUsersAuth(t *testing.T) {
	e := newUsersEnv(t, usersNow)
	for _, path := range []string{"/api/v1/admin/users/summary", "/api/v1/admin/users", "/api/v1/admin/users/1", "/api/v1/admin/growth"} {
		for _, tc := range []struct {
			name   string
			mut    func(*http.Request)
			status int
		}{
			{"no credentials", nil, 401},
			{"non admin session", func(r *http.Request) { r.AddCookie(&http.Cookie{Name: "sid", Value: "2"}) }, 403},
			{"wrong scope", func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+e.badTok) }, 403},
		} {
			if rec := e.get(path, tc.mut); rec.Code != tc.status {
				t.Errorf("%s %s: %d, want %d", path, tc.name, rec.Code, tc.status)
			}
		}
	}
}

func TestUsersSummary(t *testing.T) {
	e := newUsersEnv(t, usersNow)
	seedUsers(t, e)
	for _, via := range []struct {
		name string
		mut  func(*http.Request)
	}{{"admin", asAdmin}, {"token", e.asToken}} {
		t.Run(via.name, func(t *testing.T) {
			d := e.data(t, "/api/v1/admin/users/summary", via.mut)
			kpis := d["kpis"].(map[string]any)
			type k struct{ value, prev float64 }
			want := map[string]k{
				"total": {9, 7}, "active_7d": {3, 2}, "active_30d": {4, 2}, "dormant": {2, 1}, "no_session": {3, 4},
			}
			for name, w := range want {
				got := kpis[name].(map[string]any)
				if num(t, got["value"]) != w.value || num(t, got["previous"]) != w.prev {
					t.Errorf("kpi %s = %v, want %+v", name, got, w)
				}
			}
			if got := kpis["total"].(map[string]any)["delta_pct"]; !near(num(t, got), (9.0-7)/7*100) {
				t.Errorf("total delta = %v", got)
			}
			segs := map[string]map[string]any{}
			for _, s := range list(t, d["segments"]) {
				m := s.(map[string]any)
				segs[m["key"].(string)] = m
			}
			wantSeg := map[string]float64{"new": 2, "regular": 1, "power": 1, "dormant": 2, "at_risk": 1, "never_watched": 2}
			if len(segs) != len(wantSeg) {
				t.Fatalf("segments %v", segs)
			}
			for key, n := range wantSeg {
				if num(t, segs[key]["users"]) != n {
					t.Errorf("segment %s users = %v, want %v", key, segs[key]["users"], n)
				}
			}
			if !near(num(t, segs["power"]["share_pct"]), 100.0/9) || !near(num(t, segs["power"]["avg_watch_hours"]), 291600.0/3600) {
				t.Errorf("power = %v", segs["power"])
			}
			wantSen := map[string]float64{"lt_7d": 2, "7_30d": 0, "30_90d": 1, "90_180d": 5, "180_365d": 1, "gt_365d": 0}
			for _, s := range list(t, d["seniority"]) {
				m := s.(map[string]any)
				if num(t, m["users"]) != wantSen[m["key"].(string)] {
					t.Errorf("seniority %v", m)
				}
			}
			as := d["active_sessions"].(map[string]any)
			if num(t, as["valid"]) != 5 || num(t, as["expiring_7d"]) != 4 || num(t, as["expiring_24h"]) != 1 || num(t, as["expiring_24_48h"]) != 1 {
				t.Errorf("active_sessions = %v", as)
			}
		})
	}
	t.Run("empty database", func(t *testing.T) {
		e := newUsersEnv(t, usersNow)
		d := e.data(t, "/api/v1/admin/users/summary", asAdmin)
		if num(t, d["kpis"].(map[string]any)["total"].(map[string]any)["value"]) != 0 || len(list(t, d["segments"])) != 6 {
			t.Errorf("%v", d)
		}
	})
}

func TestUsersList(t *testing.T) {
	e := newUsersEnv(t, usersNow)
	seedUsers(t, e)
	ids := func(d map[string]any) []int {
		var out []int
		for _, u := range list(t, d["users"]) {
			out = append(out, int(num(t, u.(map[string]any)["user_id"])))
		}
		return out
	}
	eq := func(a, b []int) bool { return fmt.Sprint(a) == fmt.Sprint(b) }

	tests := []struct {
		name, query string
		mut         func(*http.Request)
		total       float64
		wantIDs     []int
	}{
		{"default newest first, id tie-break", "", asAdmin, 9, []int{7, 5, 6, 1, 2, 3, 4, 8, 9}},
		{"sort sessions desc", "?sort=sessions&dir=desc&limit=3", asAdmin, 9, []int{5, 1, 4}},
		{"sort user_id asc paged", "?sort=user_id&dir=asc&limit=2&offset=8", asAdmin, 9, []int{9}},
		{"offset past the end", "?offset=100", asAdmin, 9, []int{}},
		{"segment power", "?segment=power", asAdmin, 1, []int{1}},
		{"segment never_watched", "?segment=never_watched&sort=user_id&dir=asc", asAdmin, 2, []int{6, 8}},
		{"q underscore is literal", "?q=_", asAdmin, 1, []int{2}},
		{"q percent is literal", "?q=%25", asAdmin, 1, []int{3}},
		{"q case-insensitive substring + segment", "?q=ALI&segment=power", asAdmin, 1, []int{1}},
		{"q injection stays inert", "?q=%27%20OR%201%3D1%20--", asAdmin, 0, []int{}},
		{"token sort by sessions", "?sort=last_activity&dir=asc&segment=dormant", e.asToken, 2, []int{9, 3}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := e.data(t, "/api/v1/admin/users"+tc.query, tc.mut)
			if num(t, d["total"]) != tc.total {
				t.Errorf("total = %v, want %v", d["total"], tc.total)
			}
			if !eq(ids(d), tc.wantIDs) {
				t.Errorf("ids = %v, want %v", ids(d), tc.wantIDs)
			}
		})
	}

	t.Run("row content", func(t *testing.T) {
		d := e.data(t, "/api/v1/admin/users?segment=power", asAdmin)
		u := list(t, d["users"])[0].(map[string]any)
		if u["pseudo"] != "alice" || u["segment"] != "power" || num(t, u["sessions"]) != 5 || !near(num(t, u["watch_hours"]), 81) {
			t.Errorf("row = %v", u)
		}
		top := u["top_anime"].(map[string]any)
		if num(t, top["anime_id"]) != 100 || top["title"] != "Alpha" || !near(num(t, top["watch_hours"]), 80) {
			t.Errorf("top_anime = %v", top)
		}
		if u["last_activity"] != "2026-03-30T10:00:00Z" || u["created_at"] != "2025-12-21T10:00:00Z" {
			t.Errorf("times = %v / %v", u["last_activity"], u["created_at"])
		}
		d = e.data(t, "/api/v1/admin/users?segment=never_watched", asAdmin)
		u = list(t, d["users"])[0].(map[string]any)
		if u["last_activity"] != nil || u["top_anime"] != nil {
			t.Errorf("never watched row = %v", u)
		}
	})

	t.Run("token never sees pseudos or secrets", func(t *testing.T) {
		rec := e.get("/api/v1/admin/users?limit=200", e.asToken)
		if rec.Code != 200 {
			t.Fatal(rec.Code)
		}
		body := rec.Body.String()
		for _, bad := range []string{"pseudo", "alice", "email", "secret", "hash", "pass"} {
			if strings.Contains(strings.ToLower(body), bad) {
				t.Errorf("token response leaks %q: %s", bad, body)
			}
		}
		rec = e.get("/api/v1/admin/users?limit=200", asAdmin)
		for _, bad := range []string{"email", "secret", "hash", "pass"} {
			if strings.Contains(strings.ToLower(rec.Body.String()), bad) {
				t.Errorf("admin response leaks %q", bad)
			}
		}
	})

	for _, tc := range []struct {
		name, query string
		mut         func(*http.Request)
		status      int
	}{
		{"token cannot search", "?q=a", e.asToken, 403},
		{"token cannot sort by pseudo", "?sort=pseudo", e.asToken, 403},
		{"admin can sort by pseudo", "?sort=pseudo&dir=asc", asAdmin, 200},
		{"unknown sort", "?sort=email", asAdmin, 400},
		{"sql in sort", "?sort=id%3B%20DROP%20TABLE%20users", asAdmin, 400},
		{"bad dir", "?dir=sideways", asAdmin, 400},
		{"bad segment", "?segment=vip", asAdmin, 400},
		{"bad limit", "?limit=0", asAdmin, 400},
		{"bad offset", "?offset=-1", asAdmin, 400},
		{"q too long", "?q=" + strings.Repeat("a", 65), asAdmin, 400},
		{"limit is capped", "?limit=100000", asAdmin, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if rec := e.get("/api/v1/admin/users"+tc.query, tc.mut); rec.Code != tc.status {
				t.Errorf("status %d, want %d (%s)", rec.Code, tc.status, rec.Body)
			}
		})
	}
	var n int
	if err := e.rw.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n); err != nil || n != 9 {
		t.Fatalf("users table damaged: %d %v", n, err)
	}

	t.Run("empty database", func(t *testing.T) {
		e := newUsersEnv(t, usersNow)
		d := e.data(t, "/api/v1/admin/users", asAdmin)
		if num(t, d["total"]) != 0 || len(list(t, d["users"])) != 0 {
			t.Errorf("%v", d)
		}
	})
}

func TestUserDetail(t *testing.T) {
	e := newUsersEnv(t, usersNow)
	seedUsers(t, e)

	d := e.data(t, "/api/v1/admin/users/1", asAdmin)
	if d["pseudo"] != "alice" || d["segment"] != "power" || num(t, d["user_id"]) != 1 {
		t.Errorf("detail = %v", d)
	}
	rs := list(t, d["recent_sessions"])
	if len(rs) != 5 || rs[0].(map[string]any)["started_at"] != "2026-03-30T10:00:00Z" ||
		rs[0].(map[string]any)["title"] != "Alpha" || num(t, rs[0].(map[string]any)["watched_seconds"]) != 72000 {
		t.Errorf("recent = %v", rs)
	}
	pr := list(t, d["progress"]) // only the unfinished one
	if len(pr) != 1 || num(t, pr[0].(map[string]any)["season_id"]) != 1000 || num(t, pr[0].(map[string]any)["position"]) != 123.5 ||
		num(t, pr[0].(map[string]any)["episode"]) != 4 || num(t, pr[0].(map[string]any)["anime_id"]) != 100 {
		t.Errorf("progress = %v", pr)
	}
	if len(list(t, e.data(t, "/api/v1/admin/users/5", asAdmin)["recent_sessions"])) != 10 {
		t.Error("recent sessions must be capped at 10")
	}
	if got := e.data(t, "/api/v1/admin/users/6", asAdmin); got["segment"] != "never_watched" ||
		len(list(t, got["recent_sessions"])) != 0 || len(list(t, got["progress"])) != 0 {
		t.Errorf("empty user = %v", got)
	}

	rec := e.get("/api/v1/admin/users/1", e.asToken)
	if rec.Code != 200 {
		t.Fatal(rec.Code)
	}
	for _, bad := range []string{"pseudo", "alice", "email", "secret", "hash"} {
		if strings.Contains(strings.ToLower(rec.Body.String()), bad) {
			t.Errorf("token detail leaks %q", bad)
		}
	}
	if strings.Contains(strings.ToLower(e.get("/api/v1/admin/users/1", asAdmin).Body.String()), "secret") {
		t.Error("admin detail leaks secrets")
	}
	for _, id := range []string{"999", "0", "-1", "abc", "1.5", "1%27", "99999999999999999999"} {
		if rec := e.get("/api/v1/admin/users/"+id, asAdmin); rec.Code != 404 {
			t.Errorf("id %q: %d, want 404", id, rec.Code)
		}
	}
}

func TestGrowth(t *testing.T) {
	e := newUsersEnv(t, usersNow)
	day := func(s string) int64 { return at(s + "Z").Unix() }
	e.user(t, 10, "u0", day("2025-12-01T00:00:00"))
	e.user(t, 1, "u1", day("2026-03-26T08:00:00"))
	e.user(t, 2, "u2", day("2026-03-25T12:00:00"))
	e.user(t, 3, "u3", day("2026-03-28T00:00:00"))
	e.user(t, 4, "u4", day("2026-03-31T01:00:00"))
	e.user(t, 5, "u5", day("2026-02-01T10:00:00"))
	e.user(t, 6, "u6", day("2026-02-01T11:00:00"))
	e.user(t, 7, "u7", day("2026-03-20T09:00:00"))
	w := func(uid int64, ep int, ts string) { e.watch(t, uid, 100, "A", ep, day(ts), 100) }
	w(10, 1, "2026-03-05T10:00:00")
	w(10, 1, "2026-03-24T10:00:00")
	w(10, 1, "2026-03-31T09:00:00")
	w(1, 1, "2026-03-26T09:00:00")
	w(1, 2, "2026-03-27T09:00:00")
	w(1, 3, "2026-03-28T09:00:00")
	w(2, 1, "2026-03-25T12:30:00")
	w(4, 1, "2026-03-31T02:00:00")
	w(5, 1, "2026-02-01T10:10:00")
	w(5, 2, "2026-02-02T10:00:00")
	w(5, 3, "2026-02-08T10:00:00")
	w(5, 3, "2026-03-03T11:00:00")
	w(6, 1, "2026-02-01T12:00:00")
	w(7, 1, "2026-03-23T10:00:00")
	w(7, 2, "2026-03-24T10:00:00")
	w(7, 3, "2026-03-25T10:00:00")
	w(7, 3, "2026-03-27T10:00:00")

	kpi := func(d map[string]any, name string) (v, p float64, dl any) {
		m := d["activity"].(map[string]any)[name].(map[string]any)
		return num(t, m["value"]), num(t, m["previous"]), m["delta_pct"]
	}

	t.Run("period 7: activity", func(t *testing.T) {
		d := e.data(t, "/api/v1/admin/growth?period=7", asAdmin)
		for name, w := range map[string][2]float64{"dau": {2, 2}, "wau": {5, 2}, "mau": {6, 3}} {
			v, p, _ := kpi(d, name)
			if v != w[0] || p != w[1] {
				t.Errorf("%s = %v/%v, want %v", name, v, p, w)
			}
		}
		if _, _, dl := kpi(d, "wau"); !near(num(t, dl), 150) {
			t.Errorf("wau delta = %v", dl)
		}
		if v, p, dl := kpi(d, "stickiness_pct"); !near(v, 100.0/3) || !near(p, 200.0/3) || !near(num(t, dl), -50) {
			t.Errorf("stickiness = %v %v %v", v, p, dl)
		}
		ser := list(t, d["activity_series"])
		if len(ser) != 7 {
			t.Fatalf("series has %d days", len(ser))
		}
		pt := ser[5].(map[string]any) // 2026-03-30
		if pt["day"] != "2026-03-30" || num(t, pt["dau"]) != 0 || num(t, pt["wau"]) != 4 || num(t, pt["mau"]) != 5 {
			t.Errorf("2026-03-30 = %v", pt)
		}
		if last := ser[6].(map[string]any); last["day"] != "2026-03-31" || num(t, last["wau"]) != 5 {
			t.Errorf("last = %v", last)
		}
		wk := list(t, d["signups_per_week"])
		if len(wk) != 2 || wk[0].(map[string]any)["week_start"] != "2026-03-23" || num(t, wk[0].(map[string]any)["signups"]) != 3 ||
			num(t, wk[1].(map[string]any)["signups"]) != 1 {
			t.Errorf("signups = %v", wk)
		}
		cu := list(t, d["cumulative_users"])
		if len(cu) != 7 || num(t, cu[0].(map[string]any)["users"]) != 5 || num(t, cu[6].(map[string]any)["users"]) != 8 {
			t.Errorf("cumulative = %v", cu)
		}
		// Funnel for the 4 accounts of the last 7 days: nothing is measurable at J7 yet.
		steps := list(t, d["funnel"].(map[string]any)["steps"])
		j7 := steps[3].(map[string]any)
		if num(t, d["funnel"].(map[string]any)["cohort_size"]) != 4 || num(t, j7["excluded_too_recent"]) != 4 ||
			num(t, j7["eligible"]) != 0 || j7["conversion_pct"] != nil {
			t.Errorf("funnel = %v", d["funnel"])
		}
	})

	t.Run("period 90", func(t *testing.T) {
		d := e.data(t, "/api/v1/admin/growth?period=90", asAdmin)
		if len(list(t, d["activity_series"])) != 90 || len(list(t, d["cumulative_users"])) != 90 {
			t.Error("series must have one point per day")
		}
		if _, p, dl := kpi(d, "dau"); p != 0 || dl != nil {
			t.Errorf("no previous activity: previous %v delta %v", p, dl)
		}
		f := d["funnel"].(map[string]any)
		steps := list(t, f["steps"])
		want := []struct {
			key         string
			users, elig float64
			excl        float64
			conv        any
		}{
			{"signup", 7, 7, 0, nil},
			{"first_session", 6, 7, 0, 600.0 / 7},
			{"three_episodes", 3, 6, 0, 50.0},
			{"active_d7", 2, 2, 4, 100.0},
			{"active_d30", 1, 1, 5, 100.0},
		}
		if num(t, f["cohort_size"]) != 7 || len(steps) != 5 {
			t.Fatalf("funnel = %v", f)
		}
		for i, w := range want {
			s := steps[i].(map[string]any)
			if s["key"] != w.key || num(t, s["users"]) != w.users || num(t, s["eligible"]) != w.elig || num(t, s["excluded_too_recent"]) != w.excl {
				t.Errorf("step %d = %v, want %+v", i, s, w)
			}
			if w.conv == nil {
				if s["conversion_pct"] != nil {
					t.Errorf("step %d conversion = %v", i, s["conversion_pct"])
				}
			} else if !near(num(t, s["conversion_pct"]), w.conv.(float64)) {
				t.Errorf("step %d conversion = %v, want %v", i, s["conversion_pct"], w.conv)
			}
		}

		coh := map[string]map[string]any{}
		for _, c := range list(t, d["cohorts"]) {
			m := c.(map[string]any)
			coh[m["week_start"].(string)] = m
		}
		if len(coh) != 14 {
			t.Errorf("%d weekly cohorts, want 14", len(coh))
		}
		check := func(week string, users float64, cells [4]any) {
			t.Helper()
			m := coh[week]
			if m == nil || num(t, m["users"]) != users {
				t.Fatalf("cohort %s = %v", week, m)
			}
			for i, k := range []string{"d1", "d7", "d14", "d30"} {
				switch c := cells[i].(type) {
				case nil:
					if m[k] != nil {
						t.Errorf("cohort %s %s = %v, want null", week, k, m[k])
					}
				case float64:
					if m[k] == nil || !near(num(t, m[k]), c) {
						t.Errorf("cohort %s %s = %v, want %v", week, k, m[k], c)
					}
				}
			}
		}
		check("2026-01-26", 2, [4]any{50.0, 50.0, 0.0, 50.0})
		check("2026-03-16", 1, [4]any{0.0, 100.0, nil, nil})
		check("2026-03-23", 3, [4]any{100.0 / 3, nil, nil, nil})
		check("2026-03-30", 1, [4]any{nil, nil, nil, nil})
		check("2026-03-09", 0, [4]any{nil, nil, nil, nil})

		wantTTF := map[string]float64{"lt_1h": 2, "1h_24h": 3, "1d_7d": 1, "gt_7d": 0, "never": 1}
		for _, b := range list(t, d["time_to_first_session"].(map[string]any)["buckets"]) {
			m := b.(map[string]any)
			if num(t, m["users"]) != wantTTF[m["key"].(string)] {
				t.Errorf("ttf %v", m)
			}
		}
		// delays of the 6 accounts with a session: 10 min, 30 min, 1 h ×3, 3 d 1 h
		if m := d["time_to_first_session"].(map[string]any)["median_seconds"]; m == nil || num(t, m) != 3600 {
			t.Errorf("ttf median = %v, want 3600", m)
		}
		ch := d["churn"].(map[string]any)
		if num(t, ch["previous_window_active"]) != 2 || num(t, ch["churned"]) != 1 || !near(num(t, ch["churn_pct"]), 50) {
			t.Errorf("churn = %v", ch)
		}
		nm := list(t, d["not_measured"])
		if len(nm) != 2 || nm[0] != "visitor_to_signup" || nm[1] != "acquisition_sources" ||
			d["visitor_to_signup"].(map[string]any)["measured"] != false || d["acquisition_sources"].(map[string]any)["measured"] != false {
			t.Errorf("not_measured = %v", nm)
		}
		wk := list(t, d["signups_per_week"])
		var sum float64
		for _, x := range wk {
			sum += num(t, x.(map[string]any)["signups"])
		}
		if sum != 7 {
			t.Errorf("signups sum = %v, want 7", sum)
		}
		if cu := list(t, d["cumulative_users"]); num(t, cu[89].(map[string]any)["users"]) != 8 || num(t, cu[0].(map[string]any)["users"]) != 1 {
			t.Errorf("cumulative = first %v last %v", cu[0], cu[89])
		}
	})

	t.Run("token response carries no pseudo or email", func(t *testing.T) {
		rec := e.get("/api/v1/admin/growth", e.asToken)
		body := strings.ToLower(rec.Body.String())
		if rec.Code != 200 || strings.Contains(body, "pseudo") || strings.Contains(body, "email") || strings.Contains(body, "secret") {
			t.Errorf("%d %s", rec.Code, body)
		}
	})
	for _, q := range []string{"?period=14", "?period=abc"} {
		if rec := e.get("/api/v1/admin/growth"+q, asAdmin); rec.Code != 400 {
			t.Errorf("%s: %d", q, rec.Code)
		}
	}

	t.Run("empty database", func(t *testing.T) {
		e := newUsersEnv(t, usersNow)
		d := e.data(t, "/api/v1/admin/growth?period=7", asAdmin)
		if v, _, dl := kpi(d, "mau"); v != 0 || dl != nil {
			t.Errorf("mau = %v %v", v, dl)
		}
		if len(list(t, d["activity_series"])) != 7 || len(list(t, d["cohorts"])) != 2 ||
			d["churn"].(map[string]any)["churn_pct"] != nil {
			t.Errorf("%v", d)
		}
		if num(t, d["funnel"].(map[string]any)["cohort_size"]) != 0 {
			t.Errorf("%v", d["funnel"])
		}
		if m := d["time_to_first_session"].(map[string]any)["median_seconds"]; m != nil {
			t.Errorf("ttf median without accounts = %v, want null", m)
		}
	})
}
