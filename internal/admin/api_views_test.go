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
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gazes/gazes/internal/auth"
	"github.com/go-chi/chi/v5"
)

type viewsFixture struct {
	svc    *Service
	router chi.Router
	rw     *sql.DB
	metric string
	diag   string
}

func newViewsFixture(t *testing.T, seed bool) *viewsFixture {
	t.Helper()
	dir := t.TempDir()
	as, err := auth.OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { as.Close() })
	rw, err := sql.Open("sqlite3", filepath.Join(dir, "accounts.sqlite")+"?_busy_timeout=5000")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { rw.Close() })
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
	f := &viewsFixture{svc: svc, rw: rw, router: chi.NewRouter()}
	svc.Mount(f.router)
	ctx := context.Background()
	f.metric, _, _ = st.CreateToken(ctx, "m", []string{ScopeMetricsRead}, 0)
	f.diag, _, _ = st.CreateToken(ctx, "d", []string{ScopeDiagnosticsRead}, 0)
	if seed {
		f.seed(t)
		if err := svc.Rollup().RollupRange(ctx, "2026-03-18", "2026-03-31"); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

func (f *viewsFixture) seed(t *testing.T) {
	t.Helper()
	for id, c := range map[int64]string{1: "2026-01-01 08:00:00", 3: "2026-03-20 08:00:00", 2: "2026-03-26 08:00:00", 4: "2026-03-28 08:00:00"} {
		if _, err := f.rw.Exec(`INSERT INTO users(id,email_idx,email_enc,pseudo,pass_hash,created_at) VALUES(?,?,?,?,?,?)`,
			id, fmt.Sprint("e", id), "x", fmt.Sprint("pseud", id), "h", ts(c)); err != nil {
			t.Fatal(err)
		}
	}
	type ws struct {
		user, anime         int64
		ep                  int
		title, genres, form string
		started             string
		start, end, watched float64
		dur                 float64
		comp                int
		audio, sub          string
		tz                  int
	}
	act := `["Action","Drama"]`
	data := []ws{
		{1, 100, 1, "Alpha", act, "TV", "2026-03-25 09:00:00", 0, 1400, 1400, 1400, 1, "jp", "fr", 60},
		{2, 100, 1, "Alpha", act, "TV", "2026-03-26 09:30:00", 0, 100, 100, 1400, 0, "jp", "fr", 60},
		{3, 100, 1, "Alpha", act, "TV", "2026-03-26 09:45:00", 0, 200, 200, 1400, 0, "jp", "fr", 60},
		{4, 100, 1, "Alpha", act, "TV", "2026-03-28 10:30:00", 0, 300, 300, 1400, 0, "jp", "fr", -300},
		{1, 100, 1, "Alpha", act, "TV", "2026-03-29 11:00:00", 700, 1400, 700, 1400, 1, "jp", "fr", 60},
		{1, 200, 1, "Beta", `["Action"]`, "MOVIE", "2026-03-27 23:00:00", 0, 700, 700, 1400, 0, "fr", "", 60},
		{2, 300, 2, "Gamma", `[]`, "OVA", "2026-03-30 00:00:00", 0, 0, 0, 0, 0, "", "", 60},
		// previous period
		{3, 100, 1, "Alpha", act, "TV", "2026-03-20 10:00:00", 0, 600, 600, 1400, 0, "jp", "fr", 60},
		{3, 100, 1, "Alpha", act, "TV", "2026-03-21 10:00:00", 0, 600, 600, 1400, 0, "jp", "fr", 60},
		{1, 400, 1, "Delta", `["Drama"]`, "TV", "2026-03-22 12:00:00", 0, 60, 60, 1400, 0, "jp", "fr", 60},
	}
	for i, s := range data {
		if _, err := f.rw.Exec(`INSERT INTO watch_sessions(user_id,session_id,season_id,anime_id,episode,title,genres,format,started_at,updated_at,start_position,end_position,watched_seconds,duration,completed,audio_lang,sub_lang,tz_offset)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			s.user, fmt.Sprint("s", i), s.anime, s.anime, s.ep, s.title, s.genres, s.form, ts(s.started), ts(s.started),
			s.start, s.end, s.watched, s.dur, s.comp, s.audio, s.sub, s.tz); err != nil {
			t.Fatal(err)
		}
	}
}

func (f *viewsFixture) get(t *testing.T, path string, mut func(*http.Request)) (int, []byte) {
	t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	if mut != nil {
		mut(req)
	}
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	return rec.Code, rec.Body.Bytes()
}

func (f *viewsFixture) data(t *testing.T, path string) map[string]any {
	t.Helper()
	code, body := f.get(t, path, func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+f.metric) })
	if code != 200 {
		t.Fatalf("%s: status %d: %s", path, code, body)
	}
	var env struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatal(err)
	}
	return env.Data
}

// at navigates a decoded JSON value with a dotted path (array indexes are numbers).
func jget(v any, path string) any {
	for _, k := range strings.Split(path, ".") {
		switch x := v.(type) {
		case map[string]any:
			v = x[k]
		case []any:
			i, err := strconv.Atoi(k)
			if err != nil || i >= len(x) {
				return "<missing>"
			}
			v = x[i]
		default:
			return "<missing>"
		}
	}
	return v
}

type vcheck struct {
	path string
	want any // float64, string, nil (JSON null) or "len:N"
}

func checkAll(t *testing.T, data map[string]any, checks []vcheck) {
	t.Helper()
	for _, c := range checks {
		got := jget(data, strings.TrimPrefix(strings.SplitN(c.path, "|", 2)[0], "len:"))
		if s, ok := c.want.(string); ok && strings.HasPrefix(s, "len:") {
			n, _ := strconv.Atoi(strings.TrimPrefix(s, "len:"))
			arr, isArr := got.([]any)
			if !isArr || len(arr) != n {
				t.Errorf("%s: got %v, want array of length %d", c.path, got, n)
			}
			continue
		}
		switch w := c.want.(type) {
		case float64:
			g, ok := got.(float64)
			if !ok || math.Abs(g-w) > 0.011 {
				t.Errorf("%s: got %v, want %v", c.path, got, w)
			}
		default:
			if got != c.want {
				t.Errorf("%s: got %v (%T), want %v", c.path, got, got, c.want)
			}
		}
	}
}

func TestOverview(t *testing.T) {
	f := newViewsFixture(t, true)
	checkAll(t, f.data(t, "/api/v1/admin/overview?period=7"), []vcheck{
		{"kpis.total_users.value", 4.0}, {"kpis.total_users.previous", 2.0}, {"kpis.total_users.delta_pct", 100.0},
		{"kpis.new_users.value", 2.0}, {"kpis.new_users.previous", 1.0},
		{"kpis.sessions.value", 7.0}, {"kpis.sessions.previous", 3.0}, {"kpis.sessions.delta_pct", 133.33},
		{"kpis.watch_hours.value", 0.94}, {"kpis.watch_hours.previous", 0.35},
		{"kpis.active_users_avg.value", 1.0}, {"kpis.active_users_avg.previous", 0.43}, {"kpis.active_users_avg.delta_pct", 133.33},
		{"kpis.completion_pct.value", 28.57}, {"kpis.completion_pct.previous", 0.0}, {"kpis.completion_pct.delta_pct", nil},
		{"series", "len:7"},
		{"series.0.day", "2026-03-25"}, {"series.0.sessions", 1.0},
		{"series.1.sessions", 2.0}, {"series.1.active_users", 2.0}, {"series.1.new_users", 1.0},
		{"series.6.day", "2026-03-31"}, {"series.6.sessions", 0.0},
		{"top_anime", "len:3"},
		{"top_anime.0.anime_id", 100.0}, {"top_anime.0.title", "Alpha"}, {"top_anime.0.sessions", 5.0},
		{"top_anime.0.share_pct", 71.43}, {"top_anime.0.delta_pct", 150.0},
		{"top_anime.1.anime_id", 200.0}, {"top_anime.1.delta_pct", nil},
		{"top_anime.2.anime_id", 300.0},
		{"heatmap.raw", "len:7"}, {"heatmap.raw.3.9", 2.0}, {"heatmap.normalized.3.9", 1.0}, {"heatmap.normalized.2.9", 0.5},
		{"heatmap.raw.0.0", 1.0}, {"heatmap.raw.1.0", 0.0},
		{"peak.weekday", 3.0}, {"peak.hour", 9.0}, {"peak.sessions", 2.0},
	})
}

func TestOverviewPeriods(t *testing.T) {
	f := newViewsFixture(t, true)
	for _, tc := range []struct {
		query string
		days  int
	}{{"", 30}, {"?period=7", 7}, {"?period=90", 90}} {
		d := f.data(t, "/api/v1/admin/overview"+tc.query)
		if got := len(jget(d, "series").([]any)); got != tc.days {
			t.Errorf("%q: series has %d days, want %d", tc.query, got, tc.days)
		}
	}
	// 30 days: both periods are folded into the totals (3 previous + 7 current in range).
	checkAll(t, f.data(t, "/api/v1/admin/overview?period=30"), []vcheck{{"kpis.sessions.value", 10.0}, {"kpis.sessions.previous", 0.0}})
}

func TestEmptyDatabase(t *testing.T) {
	f := newViewsFixture(t, false)
	checkAll(t, f.data(t, "/api/v1/admin/overview"), []vcheck{
		{"series", "len:30"}, {"kpis.sessions.value", 0.0}, {"kpis.sessions.delta_pct", nil},
		{"kpis.total_users.value", 0.0}, {"top_anime", "len:0"}, {"peak", nil}, {"heatmap.normalized.0.0", 0.0},
	})
	checkAll(t, f.data(t, "/api/v1/admin/views"), []vcheck{
		{"sessions_series", "len:30"}, {"avg_session_minutes", 0.0}, {"sessions_by_hour", "len:24"},
		{"completion_by_weekday", "len:7"}, {"by_timezone", "len:0"}, {"drop_episodes", "len:0"}, {"drop_points", "len:0"},
		{"retention_curve", "len:10"}, {"retention_curve.0.retained_pct", 0.0}, {"resume_vs_first.resume", 0.0},
		{"episodes_per_active_user.value", 0.0},
	})
	checkAll(t, f.data(t, "/api/v1/admin/catalog"), []vcheck{
		{"anime", "len:0"}, {"anime_total", 0.0}, {"quadrant.points", "len:0"}, {"genres", "len:0"}, {"formats", "len:0"},
		{"new_vs_catalog.new_series.share_pct", 0.0},
	})
}

func TestViews(t *testing.T) {
	f := newViewsFixture(t, true)
	checkAll(t, f.data(t, "/api/v1/admin/views?period=7"), []vcheck{
		{"sessions_series", "len:7"}, {"sessions_series.0.day", "2026-03-25"}, {"sessions_series.0.sessions", 1.0},
		{"sessions_series.0.prev_day", "2026-03-18"}, {"sessions_series.2.prev_day", "2026-03-20"}, {"sessions_series.2.prev_sessions", 1.0},
		{"sessions_series.3.prev_sessions", 1.0},
		{"avg_session_minutes", 8.1},
		{"session_length_buckets.0.range_minutes", "<5"}, {"session_length_buckets.0.sessions", 3.0},
		{"session_length_buckets.1.sessions", 3.0}, {"session_length_buckets.2.sessions", 1.0},
		{"session_length_buckets.3.sessions", 0.0}, {"session_length_buckets.4.range_minutes", ">60"},
		{"completion_by_weekday.2.name", "mer"}, {"completion_by_weekday.2.completion_pct", 100.0},
		{"completion_by_weekday.3.sessions", 2.0}, {"completion_by_weekday.3.completion_pct", 0.0},
		{"completion_by_weekday.6.completion_pct", 100.0}, {"completion_by_weekday.1.sessions", 0.0},
		{"sessions_by_hour", "len:24"}, {"sessions_by_hour.9.sessions", 3.0}, {"sessions_by_hour.0.sessions", 1.0},
		{"by_timezone", "len:2"}, {"by_timezone.0.tz_offset", 60.0}, {"by_timezone.0.sessions", 6.0}, {"by_timezone.0.share_pct", 85.71},
		{"by_timezone.1.tz_offset", -300.0},
		{"episodes_per_active_user.value", 1.5}, {"episodes_per_active_user.previous", 1.0}, {"episodes_per_active_user.delta_pct", 50.0},
		{"resume_vs_first.resume", 2.0}, {"resume_vs_first.first", 5.0}, {"resume_vs_first.resume_pct", 28.57},
		{"retention_curve", "len:10"},
		{"retention_curve.0.retained_pct", 100.0}, {"retention_curve.1.retained_pct", 80.0}, {"retention_curve.2.retained_pct", 60.0},
		{"retention_curve.3.retained_pct", 40.0}, {"retention_curve.5.eligible", 6.0}, {"retention_curve.5.present", 2.0},
		{"retention_curve.5.retained_pct", 33.33}, {"retention_curve.9.retained_pct", 33.33},
		{"drop_points", "len:3"}, {"drop_points.0.from_decile", 0.0}, {"drop_points.0.drop_pts", 20.0},
		{"drop_points.2.from_decile", 2.0}, {"drop_points.2.to_decile", 3.0},
		{"drop_episodes", "len:1"}, {"drop_episodes.0.anime_id", 100.0}, {"drop_episodes.0.title", "Alpha"},
		{"drop_episodes.0.episode", 1.0}, {"drop_episodes.0.sessions", 4.0}, {"drop_episodes.0.abandon_pct", 75.0},
		{"drop_episodes.0.median_drop_minute", 3.33},
	})
}

func TestCatalog(t *testing.T) {
	f := newViewsFixture(t, true)
	tests := []struct {
		name  string
		query string
		check []vcheck
	}{
		{"all", "?period=7", []vcheck{
			{"format", "all"}, {"anime_total", 3.0}, {"anime", "len:3"},
			{"anime.0.anime_id", 100.0}, {"anime.0.title", "Alpha"}, {"anime.0.format", "tv"}, {"anime.0.sessions", 5.0},
			{"anime.0.watch_hours", 0.75}, {"anime.0.completion_pct", 40.0}, {"anime.0.new_viewers", 3.0}, {"anime.0.delta_pct", 150.0},
			{"anime.1.anime_id", 200.0}, {"anime.1.format", "movie"}, {"anime.1.watch_hours", 0.19}, {"anime.1.delta_pct", nil},
			{"anime.2.anime_id", 300.0}, {"anime.2.format", "ova"},
			{"quadrant.median_sessions", 1.0}, {"quadrant.median_completion_pct", 0.0}, {"quadrant.points", "len:3"},
			{"quadrant.points.0.quadrant", "valeurs sûres"}, {"quadrant.points.0.quadrant_key", "safe"},
			{"quadrant.points.1.quadrant", "à retirer"},
			{"genres.0.genre", "Action"}, {"genres.0.sessions", 6.0}, {"genres.0.share_pct", 85.71}, {"genres.0.completion_pct", 33.33},
			{"genres.1.genre", "Drama"}, {"genres.1.sessions", 5.0}, {"genres.1.completion_pct", 40.0},
			{"formats.0.key", "tv"}, {"formats.0.share_pct", 71.43}, {"formats", "len:3"},
			{"audio_langs.0.key", "jp"}, {"audio_langs.0.sessions", 5.0}, {"audio_langs.2.key", "unknown"},
			{"sub_langs.0.key", "fr"}, {"sub_langs.1.key", "unknown"}, {"sub_langs.1.sessions", 2.0},
			{"new_vs_catalog.new_series.series", 2.0}, {"new_vs_catalog.new_series.sessions", 2.0}, {"new_vs_catalog.new_series.share_pct", 28.57},
			{"new_vs_catalog.catalog.series", 1.0}, {"new_vs_catalog.catalog.sessions", 5.0}, {"new_vs_catalog.catalog.share_pct", 71.43},
			{"new_vs_catalog.history_from", "2026-03-20"},
		}},
		{"default format is all", "?period=7", []vcheck{{"format", "all"}}},
		{"tv", "?period=7&format=tv", []vcheck{
			{"anime_total", 1.0}, {"anime.0.anime_id", 100.0}, {"genres.1.genre", "Drama"},
			{"formats", "len:3"}, {"formats.0.sessions", 5.0}, {"audio_langs.0.sessions", 5.0},
		}},
		{"movie", "?period=7&format=movie", []vcheck{{"anime_total", 1.0}, {"anime.0.anime_id", 200.0}, {"quadrant.points.0.quadrant_key", "retire"}}},
		{"ova uppercase param", "?period=7&format=OVA", []vcheck{{"anime_total", 1.0}, {"anime.0.anime_id", 300.0}}},
		{"pagination middle", "?period=7&limit=1&offset=1", []vcheck{{"anime", "len:1"}, {"anime.0.anime_id", 200.0}, {"anime_total", 3.0}, {"quadrant.points", "len:3"}}},
		{"pagination last", "?period=7&limit=2&offset=2", []vcheck{{"anime", "len:1"}, {"anime.0.anime_id", 300.0}}},
		{"pagination past the end", "?period=7&offset=3", []vcheck{{"anime", "len:0"}, {"anime_total", 3.0}}},
		{"limit capped", "?period=7&limit=100000", []vcheck{{"limit", 200.0}}},
		{"90 days takes in the previous period", "?period=90", []vcheck{{"anime_total", 4.0}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			checkAll(t, f.data(t, "/api/v1/admin/catalog"+tc.query), tc.check)
		})
	}
}

func TestViewsBadParams(t *testing.T) {
	f := newViewsFixture(t, true)
	for _, path := range []string{
		"/overview?period=14", "/views?period=x", "/catalog?period=0",
		"/catalog?format=bogus", "/catalog?limit=0", "/catalog?limit=-1", "/catalog?offset=-1", "/catalog?offset=x",
	} {
		code, body := f.get(t, "/api/v1/admin"+path, func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+f.metric) })
		if code != 400 || !strings.Contains(string(body), `"error"`) {
			t.Errorf("%s: status %d body %s", path, code, body)
		}
	}
}

func TestViewsAuthAndPrivacy(t *testing.T) {
	f := newViewsFixture(t, true)
	for _, path := range []string{"/overview", "/views", "/catalog"} {
		url := "/api/v1/admin" + path
		for _, tc := range []struct {
			name string
			mut  func(*http.Request)
			want int
		}{
			{"no credentials", nil, 401},
			{"wrong scope", func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+f.diag) }, 403},
			{"non admin session", func(r *http.Request) { r.AddCookie(&http.Cookie{Name: "sid", Value: "2"}) }, 403},
			{"admin session", func(r *http.Request) { r.AddCookie(&http.Cookie{Name: "sid", Value: "1"}) }, 200},
			{"metrics token", func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+f.metric) }, 200},
		} {
			code, body := f.get(t, url, tc.mut)
			if code != tc.want {
				t.Errorf("%s %s: status %d, want %d", path, tc.name, code, tc.want)
			}
			if code == 200 {
				for _, k := range []string{"email", "pseud", "pass_hash", "hash", "gzs_"} {
					if strings.Contains(string(body), k) {
						t.Errorf("%s %s: response contains %q", path, tc.name, k)
					}
				}
			}
		}
	}
}
