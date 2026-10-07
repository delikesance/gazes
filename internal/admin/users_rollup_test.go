package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"sort"
	"testing"
	"time"
)

// seedActivity fills a usersEnv with a deterministic, irregular history: accounts of every age,
// sessions synced from before signup, sessions exactly on the rolling bounds (now - 7/14/30/44/60
// d), sessions running across midnight, negative watched seconds and a session in the future.
func seedActivity(t *testing.T, e *usersEnv, longSession bool) {
	t.Helper()
	rng := rand.New(rand.NewSource(42))
	now := e.now.Unix()
	sid := 0
	ins := func(uid, anime int64, ep int, started, end int64, secs float64) {
		sid++
		if _, err := e.rw.Exec(`INSERT INTO watch_sessions(user_id,session_id,season_id,anime_id,episode,title,started_at,updated_at,watched_seconds,duration,completed)
			VALUES(?,?,?,?,?,?,?,?,?,1400,0)`, uid, fmt.Sprint("r", sid), anime*10, anime, ep, fmt.Sprint("T", anime), started, end, secs); err != nil {
			t.Fatal(err)
		}
	}
	for uid := int64(1); uid <= 40; uid++ {
		created := now - int64(rng.Intn(200*86400))
		if uid%9 == 0 {
			created = now - int64(rng.Intn(6*86400)) // new accounts
		}
		e.user(t, uid, fmt.Sprintf("User%02d_%c", uid, 'a'+rune(rng.Intn(26))), created)
		n := rng.Intn(25)
		if uid%7 == 0 {
			n = 0
		}
		for i := 0; i < n; i++ {
			started := created - 10*86400 + int64(rng.Int63n(now-created+10*86400))
			end := started + int64(rng.Intn(4*3600))
			secs := float64(rng.Intn(20000)) + rng.Float64()
			if rng.Intn(15) == 0 {
				secs = -50
			}
			ins(uid, int64(1+rng.Intn(4)), 1+rng.Intn(5), started, end, secs)
		}
	}
	for i, d := range []int64{7, 14, 30, 44, 60} {
		uid := int64(1 + i)
		ins(uid, 1, 9, now-d*86400, now-d*86400+600, 1200.5)        // exactly on the bound
		ins(uid, 1, 9, now-d*86400-1, now-d*86400+60, 300.25)       // one second before it
		ins(uid+10, 2, 1, now-d*86400+1, now-d*86400+3600, 7200.75) // one second after it
	}
	// Midnight crossers and a tight burst for the concurrency peak.
	mid := (now/86400 - 3) * 86400
	for i := 0; i < 6; i++ {
		ins(int64(20+i), 3, 1, mid-1800+int64(i*120), mid+1800, 900)
	}
	ins(21, 3, 2, now+3600, now+7200, 100) // in the future
	if longSession {
		ins(22, 4, 1, mid-5*86400, mid-2*86400, 100) // lasts into a second day after its start
	}
}

// usersPages is every URL whose figures come from the per-user activity.
func usersPages() []string {
	pages := []string{"/users/summary", "/growth?period=7", "/growth?period=30", "/growth?period=90",
		"/costs?period=7", "/costs?period=30", "/costs?period=90", "/users?limit=200"}
	for _, k := range []string{"user_id", "created_at", "last_activity", "sessions", "watch_hours", "pseudo"} {
		for _, d := range []string{"asc", "desc"} {
			pages = append(pages, "/users?limit=200&sort="+k+"&dir="+d)
		}
	}
	for _, sg := range usersSegments {
		pages = append(pages, "/users?segment="+sg.key)
	}
	pages = append(pages, "/users?q=user1", "/users?q=_a&sort=sessions", "/users?limit=5&offset=7&sort=watch_hours")
	for id := 1; id <= 41; id++ {
		pages = append(pages, fmt.Sprintf("/users/%d", id))
	}
	return pages
}

// dumpPages fetches every page (as an admin) into URL -> decoded body.
func dumpPages(t *testing.T, e *usersEnv) map[string]any {
	t.Helper()
	e.svc.respCache.clear()
	out := map[string]any{}
	for _, p := range usersPages() {
		rec := e.get("/api/v1/admin"+p, asAdmin)
		var v any
		if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		out[p] = map[string]any{"code": float64(rec.Code), "body": v}
	}
	return out
}

// sameJSON compares decoded JSON (generated_at aside), numbers within a relative 1e-9 (sums of watched seconds are
// added in another order by the rollup).
func sameJSON(path string, a, b any) string {
	switch x := a.(type) {
	case map[string]any:
		y, ok := b.(map[string]any)
		if !ok || len(x) != len(y) {
			return fmt.Sprintf("%s: %v != %v", path, a, b)
		}
		keys := make([]string, 0, len(x))
		for k := range x {
			if k != "generated_at" { // wall clock of the response
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		for _, k := range keys {
			if d := sameJSON(path+"."+k, x[k], y[k]); d != "" {
				return d
			}
		}
		return ""
	case []any:
		y, ok := b.([]any)
		if !ok || len(x) != len(y) {
			return fmt.Sprintf("%s: %v != %v", path, a, b)
		}
		for i := range x {
			if d := sameJSON(fmt.Sprintf("%s[%d]", path, i), x[i], y[i]); d != "" {
				return d
			}
		}
		return ""
	case float64:
		y, ok := b.(float64)
		if !ok || math.Abs(x-y) > 1e-9*math.Max(1, math.Abs(x)) {
			return fmt.Sprintf("%s: %v != %v", path, a, b)
		}
		return ""
	}
	if fmt.Sprint(a) != fmt.Sprint(b) {
		return fmt.Sprintf("%s: %v != %v", path, a, b)
	}
	return ""
}

// The users, growth and costs pages must answer identically from the scan alone, from a complete
// rollup, and from a rollup with holes, for a history that crosses every rolling bound.
func TestUsersRollupMatchesTheScan(t *testing.T) {
	for _, long := range []bool{false, true} {
		t.Run(fmt.Sprint("long=", long), func(t *testing.T) {
			e := newUsersEnv(t, usersNow)
			seedActivity(t, e, long)
			ctx := context.Background()
			e.svc.rollup.now = func() time.Time { return e.now } // today stays open: scanned
			if err := e.svc.rollup.BackfillHistory(ctx, e.now, 1); err != nil {
				t.Fatal(err)
			}
			var days int
			e.svc.adminDB().QueryRow(`SELECT COUNT(*) FROM metrics_activity_daily`).Scan(&days)
			if days < 150 {
				t.Fatalf("the backfill must cover the whole history, got %d days", days)
			}
			viaRollup := dumpPages(t, e)

			// Without any coverage marker every figure comes from the scan.
			if _, err := e.svc.adminDB().Exec(`DELETE FROM metrics_activity_daily`); err != nil {
				t.Fatal(err)
			}
			viaScan := dumpPages(t, e)
			if d := sameJSON("", viaScan, viaRollup); d != "" {
				t.Fatalf("rollup differs from the scan: %s", d)
			}
			if err := e.svc.rollup.BackfillHistory(ctx, e.now, 1); err != nil {
				t.Fatal(err)
			}

			// Holes: a missing day, a day rolled up before it was over.
			if _, err := e.svc.adminDB().Exec(`DELETE FROM metrics_activity_daily WHERE day = '2026-03-20'`); err != nil {
				t.Fatal(err)
			}
			if _, err := e.svc.adminDB().Exec(`UPDATE metrics_activity_daily SET computed_at = 0 WHERE day = '2026-03-02'`); err != nil {
				t.Fatal(err)
			}
			if d := sameJSON("", viaScan, dumpPages(t, e)); d != "" {
				t.Fatalf("rollup with holes differs from the scan: %s", d)
			}
		})
	}
}

// actPeakIn must agree with the original whole-period sweep on random intervals.
func TestPeakDecompositionMatchesTheSweep(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	for round := 0; round < 200; round++ {
		var ivs []actInterval
		for i := 0; i < 30; i++ {
			a := int64(rng.Intn(5 * 86400))
			ivs = append(ivs, actInterval{a, a + int64(rng.Intn(20*3600))})
		}
		from, to := int64(86400), int64(4*86400)
		var in []actInterval
		for _, iv := range ivs {
			if iv.a >= from && iv.a < to {
				in = append(in, iv)
			}
		}
		want := actPeakIn(in, math.MinInt64/2, math.MaxInt64/2)
		// Per-day composition as the rollup stores it.
		byDay := map[int64][]actInterval{}
		for _, iv := range in {
			byDay[iv.a/86400] = append(byDay[iv.a/86400], iv)
		}
		var got int64
		for d := from / 86400; d < to/86400; d++ {
			ivs := byDay[d]
			if d > from/86400 {
				ivs = append(append([]actInterval{}, byDay[d-1]...), ivs...)
			}
			got = max(got, actPeakIn(ivs, d*86400, (d+1)*86400))
		}
		if got != want {
			t.Fatalf("round %d: per-day peak %d, sweep %d", round, got, want)
		}
	}
}
