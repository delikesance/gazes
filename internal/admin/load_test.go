package admin

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"
)

// TestLoadTimings seeds a large synthetic history and times the admin endpoints. It is a tool, not a
// regression test: it only runs with GAZES_LOAD_TEST=1 (GAZES_LOAD_SESSIONS, default 300000).
//
//	GAZES_LOAD_TEST=1 go test -tags=nosqlite -run TestLoadTimings -v ./internal/admin/
func TestLoadTimings(t *testing.T) {
	if os.Getenv("GAZES_LOAD_TEST") != "1" {
		t.Skip("set GAZES_LOAD_TEST=1 to run the load timings")
	}
	sessions := 300000
	if v, err := strconv.Atoi(os.Getenv("GAZES_LOAD_SESSIONS")); err == nil && v > 0 {
		sessions = v
	}
	users := sessions / 15
	e := opsEnv(t)
	now := e.svc.now()
	tx, err := e.accounts.Begin()
	if err != nil {
		t.Fatal(err)
	}
	uStmt, _ := tx.Prepare(`INSERT INTO users(email_idx,email_enc,pseudo,pass_hash,created_at) VALUES(?,?,?,?,?)`)
	for i := 0; i < users; i++ {
		created := now.Add(-time.Duration(i%120) * 24 * time.Hour).Unix()
		if _, err := uStmt.Exec(fmt.Sprint("e", i), "x", fmt.Sprint("user", i), "x", created); err != nil {
			t.Fatal(err)
		}
	}
	sStmt, _ := tx.Prepare(`INSERT INTO watch_sessions(user_id,session_id,season_id,anime_id,episode,title,genres,format,started_at,updated_at,start_position,end_position,watched_seconds,duration,completed,audio_lang,sub_lang,tz_offset)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`)
	for i := 0; i < sessions; i++ {
		started := now.Add(-time.Duration((i*7919)%(90*86400)) * time.Second).Unix()
		anime := i%400 + 1
		if _, err := sStmt.Exec(i%users+1, fmt.Sprint("s", i), 1, anime, i%12+1, fmt.Sprint("Anime ", anime), `["Action","Fantasy"]`, "TV",
			started, started+1300, 0, 1300, 1300, 1440, i%3 == 0, "ja", "fr", 60); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5000; i++ {
		e.exec(t, `INSERT INTO playback_errors(ts, code, source) VALUES (?,?,?)`, now.Add(-time.Duration(i)*time.Minute).Unix(), "STREAM_TIMEOUT", "nyaa.si")
	}

	start := time.Now()
	if err := e.svc.Rollup().RollupRange(context.Background(), now.Add(-89*24*time.Hour).Format("2006-01-02"), now.Format("2006-01-02")); err != nil {
		t.Fatal(err)
	}
	t.Logf("rollup 90 days over %d sessions: %v", sessions, time.Since(start).Round(time.Millisecond))
	start = time.Now()
	if _, err := e.svc.EvaluateWatch(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Logf("watch evaluation: %v", time.Since(start).Round(time.Millisecond))

	for _, path := range []string{"/overview?period=90", "/views?period=90", "/catalog?period=90&limit=200", "/users/summary?period=90",
		"/users?limit=50", "/users?limit=50&sort=sessions&dir=desc", "/users?q=user12", "/growth?period=90", "/playback/health?period=90", "/costs?period=90"} {
		start = time.Now()
		r := e.do(t, "GET", path, "", pbAdminSession)
		t.Logf("GET %-42s %4d  %v  (%d bytes)", path, r.code, time.Since(start).Round(time.Millisecond), len(r.body))
		if r.code != 200 {
			t.Errorf("%s: %d %s", path, r.code, r.body)
		}
	}
}
