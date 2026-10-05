package admin

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gazes/gazes/internal/auth"
	"github.com/gazes/gazes/internal/diagnostics"
	"github.com/go-chi/chi/v5"
)

// pbEnv is a Service with a writable handle on the accounts database (seeding only) and one
// token per scope.
type pbEnv struct {
	svc      *Service
	router   chi.Router
	accounts *sql.DB
	tokens   map[string]string
}

func newPBEnv(t *testing.T) *pbEnv {
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
	rw, err := sql.Open("sqlite3", filepath.Join(dir, "accounts.sqlite")+"?_busy_timeout=5000")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { rw.Close() })
	e := &pbEnv{svc: svc, accounts: rw, tokens: map[string]string{}}
	for name, scope := range map[string]string{"metrics": ScopeMetricsRead, "diag": ScopeDiagnosticsRead, "ops": ScopeOpsWrite} {
		tok, _, err := st.CreateToken(context.Background(), name, []string{scope}, 0)
		if err != nil {
			t.Fatal(err)
		}
		e.tokens[name] = tok
	}
	e.router = chi.NewRouter()
	svc.Mount(e.router)
	return e
}

func (e *pbEnv) exec(t *testing.T, q string, args ...any) {
	t.Helper()
	if _, err := e.svc.adminDB().Exec(q, args...); err != nil {
		t.Fatal(err)
	}
}

func (e *pbEnv) seedError(t *testing.T, when, code string, anime, ep any, source string) {
	t.Helper()
	e.exec(t, `INSERT INTO playback_errors(ts, code, anime_id, episode, source) VALUES (?,?,?,?,?)`, at(when).Unix(), code, anime, ep, nullStr(source))
}

func (e *pbEnv) seedDay(t *testing.T, day string, sessions, secs int) {
	t.Helper()
	e.exec(t, `INSERT INTO metrics_daily(day, sessions, watch_seconds, computed_at) VALUES (?,?,?,0)`, day, sessions, secs)
}

type pbResp struct {
	code int
	body []byte
	data map[string]any
}

func (e *pbEnv) do(t *testing.T, method, path, body string, mut func(*http.Request)) pbResp {
	t.Helper()
	var rd *strings.Reader
	if body != "" {
		rd = strings.NewReader(body)
	} else {
		rd = strings.NewReader("")
	}
	e.svc.respCache.clear() // tests change data between calls and want the handler, not the cache
	req := httptest.NewRequest(method, "/api/v1/admin"+path, rd)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if mut != nil {
		mut(req)
	}
	rec := httptest.NewRecorder()
	e.router.ServeHTTP(rec, req)
	out := pbResp{code: rec.Code, body: rec.Body.Bytes()}
	var env struct {
		Data map[string]any `json:"data"`
	}
	_ = json.Unmarshal(out.body, &env)
	out.data = env.Data
	return out
}

func (e *pbEnv) as(name string) func(*http.Request) {
	return func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+e.tokens[name]) }
}

func pbAdminSession(r *http.Request) { r.AddCookie(&http.Cookie{Name: "sid", Value: "1"}) }
func pbAdminCSRF(r *http.Request) {
	pbAdminSession(r)
	r.Header.Set("X-Gazes-Admin", "1")
}

func pbNum(t *testing.T, v any) float64 {
	t.Helper()
	f, ok := v.(float64)
	if !ok {
		t.Fatalf("not a number: %#v", v)
	}
	return f
}

func pbSub(t *testing.T, m map[string]any, k string) map[string]any {
	t.Helper()
	v, ok := m[k].(map[string]any)
	if !ok {
		t.Fatalf("%q is not an object in %v", k, m)
	}
	return v
}

func pbList(t *testing.T, m map[string]any, k string) []any {
	t.Helper()
	v, ok := m[k].([]any)
	if !ok {
		t.Fatalf("%q is not a list in %v", k, m)
	}
	return v
}

func (e *pbEnv) seedErrorSet(t *testing.T) {
	e.seedError(t, "2026-03-30T09:00:00Z", "STREAM_TIMEOUT", 100, 1, "nyaa.si")
	e.seedError(t, "2026-03-30T11:00:00Z", "STREAM_TIMEOUT", 100, 1, "nyaa.si")
	e.seedError(t, "2026-03-31T08:00:00Z", "REMUX_FAILED", 100, 1, "nyaa.si")
	e.seedError(t, "2026-03-10T12:00:00Z", "SRC_DEAD", nil, nil, "")
	e.seedError(t, "2026-03-05T12:00:00Z", "STREAM_TIMEOUT", 200, 3, "c411")
	e.seedError(t, "2026-02-10T12:00:00Z", "STREAM_TIMEOUT", 300, 2, "nyaa.si") // previous 30-day window
}

func TestPlaybackAuth(t *testing.T) {
	e := newPBEnv(t)
	routes := []struct{ method, path, body, scope string }{
		{"GET", "/playback/health", "", "diag"},
		{"GET", "/playback/errors", "", "diag"},
		{"GET", "/playback/errors/summary", "", "diag"},
		{"GET", "/playback/sources", "", "diag"},
		{"GET", "/costs", "", "metrics"},
		{"GET", "/issues", "", "diag"},
		{"GET", "/issues/abc", "", "diag"},
		{"POST", "/issues", `{"title":"t","severity":"low"}`, "ops"},
		{"PATCH", "/issues/abc", `{"note":"n"}`, "ops"},
	}
	for _, rt := range routes {
		t.Run(rt.method+" "+rt.path, func(t *testing.T) {
			if got := e.do(t, rt.method, rt.path, rt.body, nil).code; got != 401 {
				t.Errorf("no credentials: %d, want 401", got)
			}
			if got := e.do(t, rt.method, rt.path, rt.body, func(r *http.Request) { r.AddCookie(&http.Cookie{Name: "sid", Value: "2"}) }).code; got != 403 {
				t.Errorf("non-admin session: %d, want 403", got)
			}
			for _, other := range []string{"metrics", "diag", "ops"} {
				if other == rt.scope {
					continue
				}
				if got := e.do(t, rt.method, rt.path, rt.body, e.as(other)).code; got != 403 {
					t.Errorf("token %s: %d, want 403", other, got)
				}
			}
			if got := e.do(t, rt.method, rt.path, rt.body, e.as(rt.scope)).code; got == 401 || got == 403 {
				t.Errorf("right scope refused: %d", got)
			}
		})
	}
}

func TestPlaybackErrorsList(t *testing.T) {
	e := newPBEnv(t)
	empty := e.do(t, "GET", "/playback/errors", "", e.as("diag"))
	if empty.code != 200 || len(pbList(t, empty.data, "items")) != 0 || pbNum(t, pbSub(t, empty.data, "page")["total"]) != 0 {
		t.Fatalf("empty db: %d %s", empty.code, empty.body)
	}
	e.seedErrorSet(t)

	tests := []struct {
		name, query string
		code        int
		wantTotal   float64
		wantCodes   []string // item codes in order
	}{
		{"all, newest group first", "", 200, 5, []string{"REMUX_FAILED", "STREAM_TIMEOUT", "SRC_DEAD", "STREAM_TIMEOUT", "STREAM_TIMEOUT"}},
		{"code filter", "?code=STREAM_TIMEOUT", 200, 3, []string{"STREAM_TIMEOUT", "STREAM_TIMEOUT", "STREAM_TIMEOUT"}},
		{"since day", "?since=2026-03-01", 200, 4, []string{"REMUX_FAILED", "STREAM_TIMEOUT", "SRC_DEAD", "STREAM_TIMEOUT"}},
		{"since RFC3339", "?since=2026-03-30T10:00:00Z", 200, 2, []string{"REMUX_FAILED", "STREAM_TIMEOUT"}},
		{"since far in the past is clamped to 90 days", "?since=2000-01-01", 200, 5, nil},
		{"page 2", "?limit=2&offset=2", 200, 5, []string{"SRC_DEAD", "STREAM_TIMEOUT"}},
		{"last partial page", "?limit=2&offset=4", 200, 5, []string{"STREAM_TIMEOUT"}},
		{"offset past the end", "?offset=100", 200, 5, []string{}},
		{"unknown code", "?code=NOPE", 400, 0, nil},
		{"bad since", "?since=yesterday", 400, 0, nil},
		{"bad limit", "?limit=0", 400, 0, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := e.do(t, "GET", "/playback/errors"+tc.query, "", e.as("diag"))
			if got.code != tc.code {
				t.Fatalf("status %d, want %d (%s)", got.code, tc.code, got.body)
			}
			if tc.code != 200 {
				return
			}
			if total := pbNum(t, pbSub(t, got.data, "page")["total"]); total != tc.wantTotal {
				t.Errorf("total %v, want %v", total, tc.wantTotal)
			}
			if tc.wantCodes == nil {
				return
			}
			items := pbList(t, got.data, "items")
			if len(items) != len(tc.wantCodes) {
				t.Fatalf("%d items, want %d: %s", len(items), len(tc.wantCodes), got.body)
			}
			for i, it := range items {
				if c := it.(map[string]any)["code"]; c != tc.wantCodes[i] {
					t.Errorf("item %d code %v, want %v", i, c, tc.wantCodes[i])
				}
			}
		})
	}

	top := pbList(t, e.do(t, "GET", "/playback/errors?limit=2", "", e.as("diag")).data, "items")[1].(map[string]any)
	if top["code"] != "STREAM_TIMEOUT" || pbNum(t, top["occurrences"]) != 2 || pbNum(t, top["anime_id"]) != 100 || pbNum(t, top["episode"]) != 1 || top["source"] != "nyaa.si" ||
		top["first_seen"] != "2026-03-30T09:00:00Z" || top["last_seen"] != "2026-03-30T11:00:00Z" {
		t.Errorf("grouped item wrong: %v", top)
	}
	sd := pbList(t, e.do(t, "GET", "/playback/errors?code=SRC_DEAD", "", e.as("diag")).data, "items")[0].(map[string]any)
	if sd["anime_id"] != nil || sd["episode"] != nil || sd["source"] != nil {
		t.Errorf("unknown anime/episode/source must be null: %v", sd)
	}
}

func TestPlaybackErrorsSummary(t *testing.T) {
	e := newPBEnv(t)
	empty := e.do(t, "GET", "/playback/errors/summary", "", e.as("diag"))
	if empty.code != 200 || len(pbList(t, empty.data, "daily")) != 30 || len(pbList(t, empty.data, "probable_causes")) != 0 || pbSub(t, empty.data, "total")["value"] != 0.0 {
		t.Fatalf("empty db: %d %s", empty.code, empty.body)
	}
	e.seedErrorSet(t)

	got := e.do(t, "GET", "/playback/errors/summary", "", e.as("diag"))
	if got.code != 200 {
		t.Fatalf("%d %s", got.code, got.body)
	}
	total := pbSub(t, got.data, "total")
	if total["value"] != 5.0 || total["previous"] != 1.0 || total["delta_pct"] != 400.0 {
		t.Errorf("total = %v", total)
	}
	byCode := map[string]map[string]any{}
	for _, c := range pbList(t, got.data, "by_code") {
		m := c.(map[string]any)
		byCode[m["code"].(string)] = pbSub(t, m, "count")
	}
	if byCode["STREAM_TIMEOUT"]["value"] != 3.0 || byCode["STREAM_TIMEOUT"]["previous"] != 1.0 || byCode["STREAM_TIMEOUT"]["delta_pct"] != 200.0 {
		t.Errorf("STREAM_TIMEOUT = %v", byCode["STREAM_TIMEOUT"])
	}
	if byCode["REMUX_FAILED"]["value"] != 1.0 || byCode["REMUX_FAILED"]["delta_pct"] != nil || byCode["KV_UNAVAILABLE"]["value"] != 0.0 {
		t.Errorf("by_code = %v", byCode)
	}
	if len(byCode) != len(diagnostics.ErrorCodes()) {
		t.Errorf("by_code must list every known code, got %d", len(byCode))
	}

	daily := pbList(t, got.data, "daily")
	if len(daily) != 30 {
		t.Fatalf("daily has %d days, want 30 (no gaps)", len(daily))
	}
	if daily[0].(map[string]any)["day"] != "2026-03-02" || daily[29].(map[string]any)["day"] != "2026-03-31" {
		t.Errorf("daily bounds: %v .. %v", daily[0], daily[29])
	}
	var sum float64
	for _, d := range daily {
		m := d.(map[string]any)
		sum += pbNum(t, m["total"])
		if m["day"] == "2026-03-30" && (m["total"] != 2.0 || pbSub(t, m, "by_code")["STREAM_TIMEOUT"] != 2.0) {
			t.Errorf("2026-03-30 = %v", m)
		}
	}
	if sum != 5 {
		t.Errorf("daily sums to %v, want 5", sum)
	}

	causes := map[string]map[string]any{}
	for _, c := range pbList(t, got.data, "probable_causes") {
		m := c.(map[string]any)
		causes[m["code"].(string)] = m
	}
	if len(causes) != 3 {
		t.Fatalf("probable_causes only for codes seen: %v", causes)
	}
	if files := fmt.Sprint(causes["STREAM_TIMEOUT"]["files"]); !strings.Contains(files, "internal/torrent") || !strings.Contains(files, "internal/playback/manager.go") {
		t.Errorf("STREAM_TIMEOUT files = %s", files)
	}
	if !strings.Contains(fmt.Sprint(causes["REMUX_FAILED"]["files"]), "internal/stream") || !strings.Contains(fmt.Sprint(causes["SRC_DEAD"]["files"]), "internal/indexer") {
		t.Errorf("causes = %v", causes)
	}

	// 7 days: only the two errors of the last two days.
	week := e.do(t, "GET", "/playback/errors/summary?period=7", "", e.as("diag"))
	if len(pbList(t, week.data, "daily")) != 7 || pbSub(t, week.data, "total")["value"] != 3.0 {
		t.Errorf("period=7: %s", week.body)
	}
	if e.do(t, "GET", "/playback/errors/summary?period=14", "", e.as("diag")).code != 400 {
		t.Error("period=14 must be 400")
	}
}

func TestPlaybackSources(t *testing.T) {
	e := newPBEnv(t)
	if got := e.do(t, "GET", "/playback/sources", "", e.as("diag")); got.code != 200 || len(pbList(t, got.data, "sources")) != 0 {
		t.Fatalf("empty: %s", got.body)
	}
	e.seedErrorSet(t)
	got := e.do(t, "GET", "/playback/sources", "", e.as("diag"))
	if got.code != 200 || got.data["total_failures"] != 5.0 {
		t.Fatalf("%d %s", got.code, got.body)
	}
	items := pbList(t, got.data, "sources")
	want := []struct {
		name string
		n, p float64
	}{{"nyaa.si", 3, 1}, {"c411", 1, 0}, {"unknown", 1, 0}}
	if len(items) != len(want) {
		t.Fatalf("%d sources: %s", len(items), got.body)
	}
	for i, w := range want {
		it := items[i].(map[string]any)
		f := pbSub(t, it, "failures")
		if it["source"] != w.name || f["value"] != w.n || f["previous"] != w.p {
			t.Errorf("source %d = %v, want %+v", i, it, w)
		}
		if it["failure_rate"] != nil || it["measured"] != false {
			t.Errorf("failure rate must be null/not measured: %v", it)
		}
	}
	if pbNum(t, items[0].(map[string]any)["share_pct"]) != 60 {
		t.Errorf("share = %v", items[0])
	}
}

func TestPlaybackHealth(t *testing.T) {
	e := newPBEnv(t)

	// Empty database: nothing invented.
	h := e.do(t, "GET", "/playback/health", "", e.as("diag"))
	if h.code != 200 {
		t.Fatalf("%d %s", h.code, h.body)
	}
	if as := pbSub(t, h.data, "active_sessions"); as["value"] != 0.0 || as["measured"] != true || as["source"] != "watch_sessions" {
		t.Errorf("active_sessions without engine stats = %v", as) // falls back to recent watch logs
	}
	if er := pbSub(t, h.data, "error_rate"); er["value"] != nil || er["previous"] != nil {
		t.Errorf("error_rate without sessions must be null: %v", er)
	}
	st := pbSub(t, h.data, "startup_ms")
	if st["p50"] != nil || st["p95"] != nil || st["measured"] != false {
		t.Errorf("startup = %v", st)
	}
	if c := pbSub(t, h.data, "cache"); c["data"] != nil || c["measured"] != false {
		t.Errorf("cache = %v", c)
	}

	for _, ms := range []float64{1000, 2000, 3000, 0, -5, 999999} { // the last three are implausible
		e.svc.RecordStartup(context.Background(), ms)
	}
	h = e.do(t, "GET", "/playback/health", "", e.as("diag"))
	if st := pbSub(t, h.data, "startup_ms"); st["p50"] != 2000.0 || st["samples"] != 3.0 || st["measured"] != true {
		t.Errorf("startup after samples = %v", st)
	}

	e.seedErrorSet(t)
	e.seedDay(t, "2026-03-30", 10, 0) // current window: 5 errors / 10 sessions
	e.seedDay(t, "2026-02-10", 4, 0)  // previous window: 1 error / 4 sessions
	e.svc.SetPlaybackStats(pbFakeStats(7))
	e.svc.SetCacheDiagnostics(func(context.Context) any { return map[string]any{"redis": "disabled"} })

	h = e.do(t, "GET", "/playback/health", "", e.as("diag"))
	if as := pbSub(t, h.data, "active_sessions"); as["value"] != 7.0 || as["measured"] != true {
		t.Errorf("active_sessions = %v", as)
	}
	er := pbSub(t, h.data, "error_rate")
	if er["value"] != 50.0 || er["previous"] != 25.0 || er["delta_pct"] != 100.0 || er["errors"] != 5.0 || er["sessions"] != 10.0 {
		t.Errorf("error_rate = %v", er)
	}
	if src := pbSub(t, h.data, "sources"); src["failing"] != 2.0 || src["active"] != nil || src["measured"] != false {
		t.Errorf("sources = %v", src) // nyaa.si and c411 in the window
	}
	if c := pbSub(t, h.data, "cache"); c["measured"] != true || pbSub(t, c, "data")["redis"] != "disabled" {
		t.Errorf("cache = %v", c)
	}
	pbAssertNoPII(t, h.body)
}

type pbFakeStats int

func (f pbFakeStats) ActiveSessions() int { return int(f) }

func pbAssertNoPII(t *testing.T, body []byte) {
	t.Helper()
	for _, k := range []string{"email", "pseudo", "pass_hash", "token_hash", "gzs_"} {
		if bytes.Contains(bytes.ToLower(body), []byte(k)) {
			t.Fatalf("response leaks %q: %s", k, body)
		}
	}
}

func TestCosts(t *testing.T) {
	e := newPBEnv(t)

	// Empty database and no inputs: zeros for usage, null for money.
	got := e.do(t, "GET", "/costs", "", e.as("metrics"))
	if got.code != 200 {
		t.Fatalf("%d %s", got.code, got.body)
	}
	if v := pbSub(t, pbSub(t, got.data, "usage"), "watch_hours")["value"]; v != 0.0 {
		t.Errorf("watch_hours = %v", v)
	}
	for _, k := range []string{"server", "bandwidth", "storage", "total", "per_watch_hour", "per_active_user"} {
		c := pbSub(t, pbSub(t, got.data, "costs"), k)
		if c["value"] != nil || c["measured"] != false {
			t.Errorf("%s must be null/not measured without inputs: %v", k, c)
		}
	}

	// Usage: 3 hours watched, 3 simultaneous sessions at 10:15-10:20 on 2026-03-30.
	e.seedDay(t, "2026-03-30", 4, 7200)
	e.seedDay(t, "2026-03-31", 2, 3600)
	e.seedDay(t, "2026-02-10", 1, 1800)
	for i, s := range []struct{ start, end string }{
		{"2026-03-30T10:00:00Z", "2026-03-30T10:30:00Z"},
		{"2026-03-30T10:10:00Z", "2026-03-30T10:20:00Z"},
		{"2026-03-30T10:15:00Z", "2026-03-30T10:40:00Z"},
		{"2026-03-30T11:00:00Z", "2026-03-30T11:05:00Z"},
	} {
		uid := int64(i%3 + 1) // 3 distinct users over 4 sessions
		e.accounts.Exec(`INSERT OR IGNORE INTO users(id,email_idx,email_enc,pseudo,pass_hash,created_at) VALUES(?,?,?,?,?,0)`, uid, fmt.Sprint("e", uid), "x", fmt.Sprint("u", uid), "h")
		if _, err := e.accounts.Exec(`INSERT INTO watch_sessions(user_id,session_id,season_id,anime_id,episode,started_at,updated_at) VALUES(?,?,1,1,1,?,?)`,
			uid, fmt.Sprint("s", i), at(s.start).Unix(), at(s.end).Unix()); err != nil {
			t.Fatal(err)
		}
	}
	got = e.do(t, "GET", "/costs", "", e.as("metrics"))
	usage := pbSub(t, got.data, "usage")
	if h := pbSub(t, usage, "watch_hours"); h["value"] != 3.0 || h["previous"] != 0.5 || h["delta_pct"] != 500.0 {
		t.Errorf("watch_hours = %v", h)
	}
	if s := pbSub(t, usage, "sessions"); s["value"] != 6.0 || s["previous"] != 1.0 {
		t.Errorf("sessions = %v", s)
	}
	if u := pbSub(t, usage, "active_users"); u["value"] != 3.0 {
		t.Errorf("active_users = %v", u)
	}
	if p := pbSub(t, usage, "peak_concurrent_sessions"); p["value"] != 3.0 || p["estimated"] != true || p["truncated"] != false {
		t.Errorf("peak = %v", p)
	}

	// Server price only: prorated, partial total, per-hour and per-user derived.
	e.svc.SetCostInputs(CostInputs{ServerMonth: pbFP(300)})
	costs := pbSub(t, e.do(t, "GET", "/costs", "", e.as("metrics")).data, "costs")
	if v := pbSub(t, costs, "server")["value"]; v != 300.0 {
		t.Errorf("server = %v", v)
	}
	if b := pbSub(t, costs, "bandwidth"); b["value"] != nil || b["measured"] != false {
		t.Errorf("bandwidth needs both inputs: %v", b)
	}
	if tot := pbSub(t, costs, "total"); tot["value"] != 300.0 || tot["partial"] != true {
		t.Errorf("total = %v", tot)
	}
	if v := pbSub(t, costs, "per_watch_hour")["value"]; v != 100.0 {
		t.Errorf("per_watch_hour = %v", v)
	}
	if v := pbSub(t, costs, "per_active_user")["value"]; v != 100.0 {
		t.Errorf("per_active_user = %v", v)
	}

	// All inputs, 7 days: server = 300*7/30 = 70; bandwidth = 0.1*2*3h (all 3 hours are in the last 2 days) = 0.6.
	e.svc.SetCostInputs(CostInputs{ServerMonth: pbFP(300), BandwidthPerGB: pbFP(0.1), GBPerWatchHour: pbFP(2), StoragePerGBMonth: pbFP(0.02)})
	costs = pbSub(t, e.do(t, "GET", "/costs?period=7", "", e.as("metrics")).data, "costs")
	if v := pbSub(t, costs, "server")["value"]; v != 70.0 {
		t.Errorf("server(7d) = %v", v)
	}
	if v := pbSub(t, costs, "bandwidth")["value"]; v != 0.6000000000000001 && v != 0.6 {
		t.Errorf("bandwidth = %v", v)
	}
	if pbSub(t, costs, "storage")["value"] != nil {
		t.Error("storage volume is not measured: cost must stay null")
	}
	if tot := pbSub(t, costs, "total"); tot["partial"] != true || pbNum(t, tot["value"]) < 70.59 || pbNum(t, tot["value"]) > 70.61 {
		t.Errorf("total = %v", tot)
	}
	pbAssertNoPII(t, e.do(t, "GET", "/costs", "", e.as("metrics")).body)
	if e.do(t, "GET", "/costs?period=1", "", e.as("metrics")).code != 400 {
		t.Error("bad period must be 400")
	}
}

func pbFP(v float64) *float64 { return &v }

func TestIssues(t *testing.T) {
	e := newPBEnv(t)
	post := func(body string, mut func(*http.Request)) pbResp { return e.do(t, "POST", "/issues", body, mut) }

	// Empty list.
	if l := e.do(t, "GET", "/issues", "", e.as("diag")); l.code != 200 || len(pbList(t, l.data, "items")) != 0 {
		t.Fatalf("empty list: %s", l.body)
	}

	created := post(`{"title":" Sources lentes ","severity":"high","evidence":"3 timeouts","suggested_fix":"augmenter le délai","source":"claude"}`, e.as("ops"))
	if created.code != 201 {
		t.Fatalf("create: %d %s", created.code, created.body)
	}
	id, _ := created.data["id"].(string)
	if !issueIDPattern.MatchString(id) || created.data["status"] != "new" || created.data["title"] != "Sources lentes" || created.data["note"] != nil ||
		created.data["created_at"] != "2026-03-31T10:00:00Z" || created.data["evidence"] != "3 timeouts" {
		t.Fatalf("created = %v", created.data)
	}

	// Session writes need the CSRF header; with it they work.
	if got := post(`{"title":"b","severity":"low"}`, pbAdminSession).code; got != 403 {
		t.Errorf("session without CSRF header: %d, want 403", got)
	}
	second := post(`{"title":"b","severity":"low"}`, pbAdminCSRF)
	if second.code != 201 {
		t.Fatalf("session create: %d %s", second.code, second.body)
	}

	// Validation.
	big := `{"title":"` + strings.Repeat("a", 17<<10) + `","severity":"low"}`
	for _, tc := range []struct {
		name, body, ctype string
		want              int
	}{
		{"unknown field", `{"title":"t","severity":"low","extra":1}`, "application/json", 400},
		{"trailing data", `{"title":"t","severity":"low"} {}`, "application/json", 400},
		{"not json", `nope`, "application/json", 400},
		{"empty title", `{"title":"  ","severity":"low"}`, "application/json", 400},
		{"long title", `{"title":"` + strings.Repeat("a", 201) + `","severity":"low"}`, "application/json", 400},
		{"bad severity", `{"title":"t","severity":"urgent"}`, "application/json", 400},
		{"missing severity", `{"title":"t"}`, "application/json", 400},
		{"long evidence", `{"title":"t","severity":"low","evidence":"` + strings.Repeat("a", 4001) + `"}`, "application/json", 400},
		{"too large", big, "application/json", 413},
		{"wrong content type", `{"title":"t","severity":"low"}`, "text/plain", 415},
		{"json with charset", `{"title":"t","severity":"low"}`, "application/json; charset=utf-8", 201},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := e.do(t, "POST", "/issues", tc.body, func(r *http.Request) { e.as("ops")(r); r.Header.Set("Content-Type", tc.ctype) })
			if got.code != tc.want {
				t.Errorf("%d, want %d (%s)", got.code, tc.want, got.body)
			}
		})
	}
	nobody := e.do(t, "POST", "/issues", "", e.as("ops"))
	if nobody.code != 415 {
		t.Errorf("no body/content type: %d", nobody.code)
	}

	// Get.
	if g := e.do(t, "GET", "/issues/"+id, "", e.as("diag")); g.code != 200 || g.data["id"] != id {
		t.Errorf("get: %d %s", g.code, g.body)
	}
	for _, bad := range []string{"NOPE", "a_b", strings.Repeat("a", 41), "abc.def"} {
		if got := e.do(t, "GET", "/issues/"+bad, "", e.as("diag")).code; got != 400 {
			t.Errorf("id %q: %d, want 400", bad, got)
		}
	}
	if got := e.do(t, "GET", "/issues/missing-1", "", e.as("diag")).code; got != 404 {
		t.Errorf("unknown id: %d, want 404", got)
	}

	// Patch.
	patch := func(id, body string) pbResp { return e.do(t, "PATCH", "/issues/"+id, body, e.as("ops")) }
	e.svc.now = func() time.Time { return at("2026-04-01T10:00:00Z") }
	p := patch(id, `{"status":"in_progress","note":"en cours"}`)
	if p.code != 200 || p.data["status"] != "in_progress" || p.data["note"] != "en cours" || p.data["updated_at"] != "2026-04-01T10:00:00Z" || p.data["created_at"] != "2026-03-31T10:00:00Z" {
		t.Errorf("patch = %d %s", p.code, p.body)
	}
	if p := patch(id, `{"status":"resolved"}`); p.data["note"] != "en cours" || p.data["status"] != "resolved" {
		t.Errorf("status-only patch must keep the note: %s", p.body)
	}
	if p := patch(id, `{"note":""}`); p.data["note"] != nil || p.data["status"] != "resolved" {
		t.Errorf("empty note clears it: %s", p.body)
	}
	for name, body := range map[string]string{
		"empty": `{}`, "bad status": `{"status":"done"}`, "unknown field": `{"title":"x"}`,
		"long note": `{"note":"` + strings.Repeat("a", 2001) + `"}`,
	} {
		if got := patch(id, body).code; got != 400 {
			t.Errorf("patch %s: %d, want 400", name, got)
		}
	}
	if got := patch("missing-1", `{"note":"x"}`).code; got != 404 {
		t.Errorf("patch unknown id: %d", got)
	}
	if got := e.do(t, "PATCH", "/issues/"+id, `{"note":"x"}`, pbAdminSession).code; got != 403 {
		t.Errorf("session PATCH without CSRF: %d", got)
	}

	// List: filters and pagination.
	e.exec(t, `UPDATE issues SET created_at = created_at + 5 WHERE id = ?`, second.data["id"])
	for _, tc := range []struct {
		query string
		code  int
		n     int
		total float64
	}{
		{"", 200, 3, 3}, // created, second, charset one
		{"?status=resolved", 200, 1, 1},
		{"?status=new", 200, 2, 2},
		{"?severity=high", 200, 1, 1},
		{"?severity=low&status=new", 200, 2, 2},
		{"?limit=1&offset=1", 200, 1, 3},
		{"?offset=10", 200, 0, 3},
		{"?status=bogus", 400, 0, 0},
		{"?severity=bogus", 400, 0, 0},
	} {
		got := e.do(t, "GET", "/issues"+tc.query, "", e.as("diag"))
		if got.code != tc.code {
			t.Errorf("%s: %d, want %d", tc.query, got.code, tc.code)
			continue
		}
		if tc.code == 200 && (len(pbList(t, got.data, "items")) != tc.n || pbSub(t, got.data, "page")["total"] != tc.total) {
			t.Errorf("%s: %s", tc.query, got.body)
		}
	}
	if first := pbList(t, e.do(t, "GET", "/issues", "", e.as("diag")).data, "items")[0].(map[string]any); first["id"] != second.data["id"] {
		t.Errorf("newest first: %v", first)
	}
}

func TestErrorRecorder(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "admin.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	// A nil sink (interface or *ErrorRecorder) is a no-op and never panics.
	RecordPlaybackError(context.Background(), nil, PlaybackError{Code: diagnostics.StreamTimeout})
	var nilRec *ErrorRecorder
	RecordPlaybackError(context.Background(), nilRec, PlaybackError{Code: diagnostics.StreamTimeout})
	nilRec.Close()

	rec := NewErrorRecorder(st)
	rec.now = func() time.Time { return at("2026-03-31T10:00:00Z") }
	ctx := diagnostics.With(context.Background(), diagnostics.Correlation{AnimeID: "42", SeasonID: "7", Episode: "3"})
	RecordPlaybackError(ctx, rec, PlaybackError{Code: diagnostics.RemuxFailed, Source: "nyaa.si", InfoHash: "ABCDEF", Message: strings.Repeat("é", 400)})
	RecordPlaybackError(context.Background(), rec, PlaybackError{Code: "BOGUS", AnimeID: 5, Episode: 9})
	rec.Close()
	rec.Close()                                                                   // idempotent
	RecordPlaybackError(ctx, rec, PlaybackError{Code: diagnostics.StreamTimeout}) // after Close: dropped silently

	rows, err := st.db.Query(`SELECT code, COALESCE(anime_id,0), COALESCE(season_id,0), COALESCE(episode,0), COALESCE(source,''), COALESCE(info_hash,''), COALESCE(message,''), ts FROM playback_errors ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	type row struct {
		code, src, hash, msg  string
		anime, season, ep, ts int64
	}
	var got []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.code, &r.anime, &r.season, &r.ep, &r.src, &r.hash, &r.msg, &r.ts); err != nil {
			t.Fatal(err)
		}
		got = append(got, r)
	}
	if len(got) != 2 {
		t.Fatalf("%d rows, want 2: %+v", len(got), got)
	}
	a, b := got[0], got[1]
	if a.code != "REMUX_FAILED" || a.anime != 42 || a.season != 7 || a.ep != 3 || a.src != "nyaa.si" || a.hash != "abcdef" || a.ts != at("2026-03-31T10:00:00Z").Unix() {
		t.Errorf("row 0 = %+v", a)
	}
	if len(a.msg) > errorMaxMessage || !strings.HasPrefix(a.msg, "é") || strings.ContainsRune(a.msg, '�') {
		t.Errorf("message not truncated on a rune boundary: %d bytes", len(a.msg))
	}
	if b.code != "UNKNOWN" || b.anime != 5 || b.ep != 9 {
		t.Errorf("row 1 = %+v (invalid code must become UNKNOWN)", b)
	}
}

// A full queue must drop errors without ever blocking the caller.
func TestErrorRecorderNeverBlocks(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "admin.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	rec := &ErrorRecorder{store: st, now: time.Now, queue: make(chan PlaybackError, 2), done: make(chan struct{})} // no consumer
	done := make(chan struct{})
	go func() {
		for i := 0; i < 1000; i++ {
			rec.RecordPlaybackError(context.Background(), PlaybackError{Code: diagnostics.StreamTimeout})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("RecordPlaybackError blocked on a full queue")
	}
	if rec.Dropped() != 998 {
		t.Errorf("dropped %d, want 998", rec.Dropped())
	}
}

func TestPlaybackErrorsViaRecorderRoundTrip(t *testing.T) {
	e := newPBEnv(t)
	rec := NewErrorRecorder(e.svc.store)
	rec.now = func() time.Time { return at("2026-03-31T09:00:00Z") }
	for i := 0; i < 3; i++ {
		rec.RecordPlaybackError(context.Background(), PlaybackError{Code: diagnostics.SourceTimeout, AnimeID: 1, Episode: 1, Source: "nyaa.si"})
	}
	rec.Close()
	got := e.do(t, "GET", "/playback/errors", "", e.as("diag"))
	items := pbList(t, got.data, "items")
	if len(items) != 1 || items[0].(map[string]any)["occurrences"] != 3.0 {
		t.Errorf("round trip: %s", got.body)
	}
}
