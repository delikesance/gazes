package admin

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gazes/gazes/internal/auth"
)

func ts(s string) int64 {
	t, err := time.Parse("2006-01-02 15:04:05", s)
	if err != nil {
		panic(err)
	}
	return t.Unix()
}

type sess struct {
	user, anime int64
	title       string
	start       string
	secs        float64
	dur         float64
	completed   int
}

func setup(t *testing.T) (*Rollup, string) {
	t.Helper()
	dir := t.TempDir()
	as, err := auth.OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { as.Close() })
	accPath := filepath.Join(dir, "accounts.sqlite")
	rw, err := sql.Open("sqlite3", accPath+"?_busy_timeout=5000")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { rw.Close() })

	users := map[int64]string{
		1: "2026-01-01 08:00:00", // before range
		2: "2026-03-10 23:59:59", // day 1, last second
		3: "2026-03-11 00:00:00", // day 2, midnight
		4: "2026-03-12 12:00:00", // day 3
	}
	for id, c := range users {
		if _, err := rw.Exec(`INSERT INTO users(id,email_idx,email_enc,pseudo,pass_hash,created_at) VALUES(?,?,?,?,?,?)`,
			id, fmt.Sprint("e", id), "x", fmt.Sprint("u", id), "h", ts(c)); err != nil {
			t.Fatal(err)
		}
	}
	data := []sess{
		{1, 100, "Alpha", "2026-03-10 09:00:00", 600, 1400, 1},
		{2, 100, "Alpha", "2026-03-10 09:30:00", 100, 1400, 0},
		{1, 200, "Beta", "2026-03-10 23:59:59", 50.4, 0, 0},    // no duration
		{1, 100, "Alpha", "2026-03-11 00:00:00", 300, 1400, 1}, // midnight UTC, 2nd Alpha for user1
		{3, 200, "Beta v2", "2026-03-11 00:30:00", 0, 0, 0},    // no duration, no seconds
		{3, 100, "Alpha", "2026-03-11 10:00:00", 200, 1400, 0},
		{2, 100, "Alpha", "2026-03-12 10:10:00", 400.6, 1400, 1},
		{4, 300, "Gamma", "2026-03-12 10:20:00", 10, 1400, 0},
		{4, 0, "", "2026-03-12 22:00:00", 5, 0, 0},             // unknown anime
		{1, 100, "Alpha", "2026-03-13 00:00:00", 999, 1400, 1}, // outside range
	}
	for i, s := range data {
		if _, err := rw.Exec(`INSERT INTO watch_sessions(user_id,session_id,season_id,anime_id,episode,title,started_at,updated_at,watched_seconds,duration,completed)
			VALUES(?,?,?,?,?,?,?,?,?,?,?)`, s.user, fmt.Sprint("s", i), 1, s.anime, 1, s.title, ts(s.start), ts(s.start), s.secs, s.dur, s.completed); err != nil {
			t.Fatal(err)
		}
	}
	ad, err := Open(filepath.Join(t.TempDir(), "admin.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ad.Close() })
	r, err := NewRollup(ad, accPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Close() })
	r.now = func() time.Time { return time.Unix(1_800_000_000, 0) }
	return r, accPath
}

func dump(t *testing.T, r *Rollup, q string) [][]any {
	t.Helper()
	rows, err := r.admin.Query(q)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	cols, _ := rows.Columns()
	var out [][]any
	for rows.Next() {
		v := make([]any, len(cols))
		p := make([]any, len(cols))
		for i := range v {
			p[i] = &v[i]
		}
		if err := rows.Scan(p...); err != nil {
			t.Fatal(err)
		}
		out = append(out, v)
	}
	return out
}

func i64(v ...int64) []any {
	o := make([]any, len(v))
	for i, x := range v {
		o[i] = x
	}
	return o
}

func TestRollupRange(t *testing.T) {
	r, _ := setup(t)
	if err := r.RollupRange(context.Background(), "2026-03-10", "2026-03-12"); err != nil {
		t.Fatal(err)
	}
	// Hand-computed. Columns: sessions, watch_seconds, active, new_users, completed, total_users.
	// 03-10: 3 sessions; secs 600+100+50.4=750.4 -> 750; users {1,2}; new_users 1 (user2); completed 1; total 2 (u1,u2)
	// 03-11: 3 sessions; 300+0+200=500; users {1,3}; new 1 (user3, midnight); completed 1; total 3
	// 03-12: 3 sessions; 400.6+10+5=415.6 -> 416; users {2,4}; new 1; completed 1; total 4
	daily := dump(t, r, `SELECT sessions,watch_seconds,active_users,new_users,completed_sessions,total_users FROM metrics_daily ORDER BY day`)
	wantDaily := [][]any{i64(3, 750, 2, 1, 1, 2), i64(3, 500, 2, 1, 1, 3), i64(3, 416, 2, 1, 1, 4)}
	if !reflect.DeepEqual(daily, wantDaily) {
		t.Fatalf("daily\n got %v\nwant %v", daily, wantDaily)
	}
	// Hours: 03-10 -> 9:2, 23:1 ; 03-11 -> 0:2, 10:1 ; 03-12 -> 10:2, 22:1
	hourly := dump(t, r, `SELECT day,hour,sessions FROM metrics_hourly ORDER BY day,hour`)
	var hs []string
	for _, h := range hourly {
		hs = append(hs, fmt.Sprintf("%v/%v/%v", h[0], h[1], h[2]))
	}
	wantH := "2026-03-10/9/2 2026-03-10/23/1 2026-03-11/0/2 2026-03-11/10/1 2026-03-12/10/2 2026-03-12/22/1"
	if strings.Join(hs, " ") != wantH {
		t.Fatalf("hourly\n got %s\nwant %s", strings.Join(hs, " "), wantH)
	}
	// Anime: day, anime, sessions, secs, completed, new_viewers
	// 03-10: 100 -> 2 sess, 700s, 1 comp, new viewers u1,u2 = 2 ; 200 -> 1, 50, 0, new u1 = 1
	// 03-11: 100 -> u1 (not new), u3 (new) : 2 sess, 500s, 1 comp, 1 new ; 200 -> u3 new: 1, 0, 0, 1
	// 03-12: 100 -> u2 (not new): 1, 401, 1, 0 ; 300 -> u4 new: 1, 10, 0, 1 ; anime 0 excluded
	anime := dump(t, r, `SELECT day,anime_id,sessions,watch_seconds,completed_sessions,new_viewers FROM metrics_anime_daily ORDER BY day,anime_id`)
	var as []string
	for _, a := range anime {
		as = append(as, fmt.Sprintf("%v %v %v %v %v %v", a...))
	}
	wantA := []string{
		"2026-03-10 100 2 700 1 2", "2026-03-10 200 1 50 0 1",
		"2026-03-11 100 2 500 1 1", "2026-03-11 200 1 0 0 1",
		"2026-03-12 100 1 401 1 0", "2026-03-12 300 1 10 0 1",
	}
	if !reflect.DeepEqual(as, wantA) {
		t.Fatalf("anime\n got %q\nwant %q", as, wantA)
	}
	if got := dump(t, r, `SELECT title FROM metrics_anime_daily WHERE day='2026-03-11' AND anime_id=200`); got[0][0] != "Beta v2" {
		t.Fatalf("title %v", got)
	}
}

func TestRollupIdempotent(t *testing.T) {
	r, _ := setup(t)
	ctx := context.Background()
	qs := []string{
		`SELECT * FROM metrics_daily ORDER BY day`,
		`SELECT * FROM metrics_hourly ORDER BY day,hour`,
		`SELECT * FROM metrics_anime_daily ORDER BY day,anime_id`,
	}
	snap := func() [][][]any {
		var o [][][]any
		for _, q := range qs {
			o = append(o, dump(t, r, q))
		}
		return o
	}
	for i := 0; i < 2; i++ {
		if err := r.RollupRange(ctx, "2026-03-10", "2026-03-12"); err != nil {
			t.Fatal(err)
		}
	}
	first := snap()
	if err := r.RollupRange(ctx, "2026-03-10", "2026-03-12"); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, snap()) {
		t.Fatal("replay changed rows")
	}
	if len(first[0]) != 3 || len(first[1]) != 6 || len(first[2]) != 6 {
		t.Fatalf("unexpected counts %d %d %d", len(first[0]), len(first[1]), len(first[2]))
	}
	// Overlapping sub-range replay must not change anything either.
	if err := r.RollupRange(ctx, "2026-03-11", "2026-03-11"); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, snap()) {
		t.Fatal("sub-range replay changed rows")
	}
}

func TestRollupRangeErrors(t *testing.T) {
	r, _ := setup(t)
	tests := []struct{ name, from, to string }{
		{"reversed", "2026-03-12", "2026-03-10"},
		{"too long", "2024-01-01", "2026-03-10"},
		{"bad from", "nope", "2026-03-10"},
		{"bad to", "2026-03-10", "10/03/2026"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := r.RollupRange(context.Background(), tc.from, tc.to); err == nil {
				t.Fatal("expected error")
			}
		})
	}
	// Exactly 400 days is accepted, 401 is not.
	if err := r.RollupRange(context.Background(), "2025-02-04", "2026-03-10"); err != nil {
		t.Fatalf("400 days: %v", err)
	}
	if err := r.RollupRange(context.Background(), "2025-02-03", "2026-03-10"); err == nil {
		t.Fatal("401 days should fail")
	}
}

func TestRollupRecentAndBackfill(t *testing.T) {
	r, _ := setup(t)
	ctx := context.Background()
	now := time.Date(2026, 3, 11, 15, 0, 0, 0, time.UTC)
	if err := r.RollupRecent(ctx, now); err != nil {
		t.Fatal(err)
	}
	if got := dump(t, r, `SELECT day FROM metrics_daily ORDER BY day`); len(got) != 2 || got[0][0] != "2026-03-10" || got[1][0] != "2026-03-11" {
		t.Fatalf("recent %v", got)
	}
	if err := r.Backfill(ctx, now, 5); err != nil {
		t.Fatal(err)
	}
	if got := dump(t, r, `SELECT COUNT(*) FROM metrics_daily`); got[0][0] != int64(5) {
		t.Fatalf("backfill %v", got)
	}
	if err := r.Backfill(ctx, now, 100000); err != nil {
		t.Fatalf("clamped backfill: %v", err)
	}
}

func TestAccountsReadOnly(t *testing.T) {
	r, accPath := setup(t)
	for _, q := range []string{
		`INSERT INTO users(email_idx,email_enc,pseudo,pass_hash,created_at) VALUES('z','z','z','z',1)`,
		`DELETE FROM watch_sessions`,
		`UPDATE users SET pseudo='x'`,
	} {
		if _, err := r.accounts.Exec(q); err == nil {
			t.Fatalf("write succeeded: %s", q)
		}
	}
	var n int
	rw, _ := sql.Open("sqlite3", accPath)
	defer rw.Close()
	if err := rw.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n); err != nil || n != 4 {
		t.Fatalf("users %d %v", n, err)
	}
}

func TestRunStopsOnCancel(t *testing.T) {
	r, _ := setup(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		r.Run(ctx, 10*time.Millisecond, func() time.Time { return time.Date(2026, 3, 11, 0, 0, 0, 0, time.UTC) })
		close(done)
	}()
	deadline := time.After(2 * time.Second)
	for {
		if n := dump(t, r, `SELECT COUNT(*) FROM metrics_daily`)[0][0]; n == int64(2) {
			break
		}
		select {
		case <-deadline:
			t.Fatal("Run did not roll up")
		case <-time.After(10 * time.Millisecond):
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop on cancel")
	}
}
