package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gazes/gazes/internal/indexer"
	"github.com/gazes/gazes/internal/library"
)

type fakeSources struct {
	mu      sync.Mutex
	names   []string
	paused  map[string]time.Duration
	breaker map[string]time.Duration
	retried []string
}

func newFakeSources(names ...string) *fakeSources {
	return &fakeSources{names: names, paused: map[string]time.Duration{}, breaker: map[string]time.Duration{}}
}

func (f *fakeSources) ProviderNames() []string { return f.names }
func (f *fakeSources) Health(_ context.Context, n string) (indexer.ProviderHealth, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return indexer.ProviderHealth{Paused: f.paused[n], Cooldown: f.breaker[n]}, true
}
func (f *fakeSources) Pause(_ context.Context, n string, d time.Duration) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.paused[n] = d
	return true
}
func (f *fakeSources) Resume(_ context.Context, n string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.paused, n)
	return true
}
func (f *fakeSources) Retry(_ context.Context, n string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.retried = append(f.retried, n)
	delete(f.breaker, n)
	return true
}

type fakeQueue struct {
	entries map[library.Key]library.EncodeQueueState
}

func (q *fakeQueue) EncodeQueue(k library.Key) (library.EncodeQueueState, bool, error) {
	st, ok := q.entries[k]
	if !ok {
		return st, false, library.ErrNotFound
	}
	return st, st.EncodeSkipped == "encode_failed", nil
}
func (q *fakeQueue) RequeueEncode(k library.Key) (library.EncodeQueueState, error) {
	st, ok, err := q.EncodeQueue(k)
	if err != nil {
		return st, err
	}
	if !ok {
		return st, library.ErrNotRequeueable
	}
	q.entries[k] = library.EncodeQueueState{}
	return st, nil
}
func (q *fakeQueue) RestoreEncodeQueue(k library.Key, prev library.EncodeQueueState) error {
	if q.entries[k].EncodeSkipped != "" {
		return library.ErrNotRequeueable
	}
	q.entries[k] = prev
	return nil
}

type fixedStats int

func (n fixedStats) ActiveSessions() int { return int(n) }

// approveAction files a sensitive action and approves it as the admin session; it returns the
// approval id.
func (e *pbEnv) approveAction(t *testing.T, name, args string) string {
	t.Helper()
	r := e.action(t, name, `{"args":`+args+`,"dry_run":false,"justification":"j","expected_effect":"e"}`, "cfg")
	if r.code != 202 {
		t.Fatalf("%s request = %d %s", name, r.code, r.body)
	}
	id := fmt.Sprint(int64(pbNum(t, r.data["approval_id"])))
	if a := e.do(t, "POST", "/approvals/"+id+"/approve", "", pbAdminCSRF); a.code != 200 {
		t.Fatalf("%s approve = %d %s", name, a.code, a.body)
	}
	return id
}

func (e *pbEnv) undoApproval(t *testing.T, id string) pbResp {
	t.Helper()
	return e.do(t, "POST", "/approvals/"+id+"/undo", "", pbAdminCSRF)
}

func TestRetrySource(t *testing.T) {
	e := opsEnv(t)
	src := newFakeSources("nyaa", "c411")
	src.breaker["nyaa"] = 90 * time.Second
	e.svc.SetOpsHooks(OpsHooks{Sources: src})

	r := e.action(t, "retry_source", `{"args":{"source":"nyaa"}}`, "ops")
	if r.code != 200 || r.data["status"] != "dry_run" || len(src.retried) != 0 {
		t.Fatalf("dry run = %d %s (retried %v)", r.code, r.body, src.retried)
	}
	r = e.action(t, "retry_source", `{"args":{"source":"nyaa"},"dry_run":false}`, "ops")
	if r.code != 200 || r.data["status"] != "executed" || fmt.Sprint(src.retried) != "[nyaa]" {
		t.Fatalf("execute = %d %s (retried %v)", r.code, r.body, src.retried)
	}
	if _, ok := r.data["undo_token"]; ok {
		t.Fatal("retry_source has nothing to undo")
	}
	for _, c := range []struct {
		args string
		code int
	}{
		{`{"source":"unknown"}`, 400},
		{`{}`, 400},
		{`{"source":"nyaa","extra":1}`, 400},
	} {
		if r := e.action(t, "retry_source", `{"args":`+c.args+`}`, "ops"); r.code != c.code {
			t.Errorf("%s = %d %s", c.args, r.code, r.body)
		}
	}
	src.paused["c411"] = time.Hour
	if r := e.action(t, "retry_source", `{"args":{"source":"c411"},"dry_run":false}`, "ops"); r.code != 409 || !strings.Contains(string(r.body), "source_paused") {
		t.Fatalf("paused source = %d %s", r.code, r.body)
	}
}

func TestPauseSourceNeedsApprovalAndUndoResumes(t *testing.T) {
	e := opsEnv(t)
	src := newFakeSources("nyaa", "c411")
	e.svc.SetOpsHooks(OpsHooks{Sources: src})

	for _, args := range []string{`{"source":"nyaa","minutes":1}`, `{"source":"nyaa","minutes":2000}`, `{"source":"nyaa","minutes":1.5}`, `{"source":"x","minutes":30}`} {
		if r := e.action(t, "pause_source", `{"args":`+args+`}`, "cfg"); r.code != 400 {
			t.Errorf("%s = %d %s", args, r.code, r.body)
		}
	}
	r := e.action(t, "pause_source", `{"args":{"source":"nyaa","minutes":30},"dry_run":false,"justification":"j","expected_effect":"e"}`, "cfg")
	if r.code != 202 || len(src.paused) != 0 {
		t.Fatalf("request = %d %s (paused %v)", r.code, r.body, src.paused)
	}
	id := fmt.Sprint(int64(pbNum(t, r.data["approval_id"])))
	if a := e.do(t, "POST", "/approvals/"+id+"/approve", "", pbAdminCSRF); a.code != 200 || src.paused["nyaa"] != 30*time.Minute {
		t.Fatalf("approve = %d %s (paused %v)", a.code, a.body, src.paused)
	}
	// the other source is the last one standing: it cannot be paused too
	if r := e.action(t, "pause_source", `{"args":{"source":"c411","minutes":30}}`, "cfg"); r.code != 409 || !strings.Contains(string(r.body), "last_source") {
		t.Fatalf("last source = %d %s", r.code, r.body)
	}
	if u := e.undoApproval(t, id); u.code != 200 || len(src.paused) != 0 {
		t.Fatalf("undo = %d %s (paused %v)", u.code, u.body, src.paused)
	}
}

func TestWarmCache(t *testing.T) {
	e := opsEnv(t)
	cached := map[[2]int]int{{10, 2}: 4}
	var warmed []string
	busy := false
	e.svc.SetOpsHooks(OpsHooks{
		EpisodeSources: func(_ context.Context, s, ep int) (int, bool) { n, ok := cached[[2]int{s, ep}]; return n, ok },
		WarmEpisode: func(s, ep int, refresh bool) bool {
			if busy {
				return false
			}
			warmed = append(warmed, fmt.Sprint(s, ":", ep, ":", refresh))
			return true
		},
	})
	run := func(args string) pbResp {
		return e.action(t, "warm_cache", `{"args":`+args+`,"dry_run":false}`, "ops")
	}
	if r := run(`{"season_id":10,"episode":2}`); r.code != 200 || pbSub(t, r.data, "result")["started"] != false || len(warmed) != 0 {
		t.Fatalf("cached episode = %d %s %v", r.code, r.body, warmed)
	}
	if r := run(`{"season_id":10,"episode":2,"refresh":true}`); r.code != 200 || pbSub(t, r.data, "result")["started"] != true {
		t.Fatalf("refresh = %d %s", r.code, r.body)
	}
	if r := run(`{"season_id":10,"episode":3}`); r.code != 200 || fmt.Sprint(warmed) != "[10:2:true 10:3:false]" {
		t.Fatalf("not cached = %d %s %v", r.code, r.body, warmed)
	}
	busy = true
	if r := run(`{"season_id":10,"episode":4}`); r.code != 429 {
		t.Fatalf("busy = %d %s", r.code, r.body)
	}
	for _, args := range []string{`{"season_id":0,"episode":1}`, `{"season_id":"10","episode":1}`, `{"season_id":10}`, `{"season_id":10,"episode":1,"refresh":"yes"}`} {
		if r := run(args); r.code != 400 {
			t.Errorf("%s = %d %s", args, r.code, r.body)
		}
	}
}

func TestPurgeCache(t *testing.T) {
	e := opsEnv(t)
	var purged []string
	e.svc.SetOpsHooks(OpsHooks{PurgeCache: func(_ context.Context, scope string) (int, error) {
		purged = append(purged, scope)
		return 7, nil
	}})
	if r := e.action(t, "purge_cache", `{"args":{"scope":"catalog"}}`, "cfg"); r.code != 400 {
		t.Fatalf("catalog scope = %d %s", r.code, r.body)
	}
	id := e.approveAction(t, "purge_cache", `{"scope":"episode_sources"}`)
	if fmt.Sprint(purged) != "[episode_sources]" {
		t.Fatalf("purged = %v", purged)
	}
	g := e.do(t, "GET", "/approvals/"+id, "", e.as("diag"))
	if g.data["undoable"] != false || !strings.Contains(string(g.body), `"removed_keys":7`) {
		t.Fatalf("approval = %s", g.body)
	}
}

func TestRequeueAV1(t *testing.T) {
	e := opsEnv(t)
	failed := library.Key{SeasonID: 10, Episode: 2, Lang: "vf"}
	encoded := library.Key{SeasonID: 10, Episode: 3, Lang: "vf"}
	q := &fakeQueue{entries: map[library.Key]library.EncodeQueueState{
		failed:  {EncodeSkipped: "encode_failed", Attempts: 3, LastError: "ffmpeg exit 1"},
		encoded: {},
	}}
	e.svc.SetOpsHooks(OpsHooks{Library: q})

	r := e.action(t, "requeue_av1", `{"args":{"season_id":10,"episode":2,"lang":"vf"}}`, "ops")
	if r.code != 200 || !strings.Contains(string(r.body), `"attempts":3`) {
		t.Fatalf("plan = %d %s", r.code, r.body)
	}
	r = e.action(t, "requeue_av1", `{"args":{"season_id":10,"episode":2,"lang":"vf"},"dry_run":false}`, "ops")
	if r.code != 200 || q.entries[failed] != (library.EncodeQueueState{}) {
		t.Fatalf("execute = %d %s %+v", r.code, r.body, q.entries[failed])
	}
	if u := e.undoApproval(t, fmt.Sprint(int64(pbNum(t, r.data["action_id"])))); u.code != 200 || q.entries[failed].Attempts != 3 {
		t.Fatalf("undo = %d %s %+v", u.code, u.body, q.entries[failed])
	}
	for _, c := range []struct {
		args string
		code int
	}{
		{`{"season_id":10,"episode":3,"lang":"vf"}`, 409}, // not abandoned
		{`{"season_id":10,"episode":9,"lang":"vf"}`, 404}, // no such copy
		{`{"season_id":10,"episode":2,"lang":"fr"}`, 400}, // bad lang
		{`{"season_id":10,"episode":2}`, 400},             // missing lang
	} {
		if r := e.action(t, "requeue_av1", `{"args":`+c.args+`,"dry_run":false}`, "ops"); r.code != c.code {
			t.Errorf("%s = %d %s", c.args, r.code, r.body)
		}
	}
}

func TestLimitConcurrentStreams(t *testing.T) {
	e := opsEnv(t)
	e.svc.SetPlaybackStats(fixedStats(3))
	ctx := context.Background()
	if r := e.action(t, "limit_concurrent_streams", `{"args":{"max_streams":-1}}`, "cfg"); r.code != 400 {
		t.Fatalf("negative = %d", r.code)
	}
	id := e.approveAction(t, "limit_concurrent_streams", `{"max_streams":4}`)
	if n := e.svc.StreamLimit(ctx); n != 4 {
		t.Fatalf("limit = %d", n)
	}
	states, err := e.svc.EvaluateWatch(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if st := e.ruleState(t, states, RuleSaturation); st.Value == nil || *st.Value != 75 {
		t.Fatalf("saturation = %+v", st)
	}
	if u := e.undoApproval(t, id); u.code != 200 || e.svc.StreamLimit(ctx) != 0 {
		t.Fatalf("undo = %d %s, limit %d", u.code, u.body, e.svc.StreamLimit(ctx))
	}
	states, _ = e.svc.EvaluateWatch(ctx)
	if st := e.ruleState(t, states, RuleSaturation); st.State != WatchNotMeasured {
		t.Fatalf("saturation without a limit = %+v", st)
	}
	var nilSvc *Service
	if nilSvc.StreamLimit(ctx) != 0 {
		t.Fatal("no admin service must mean no limit")
	}
}

func TestScheduleMaintenance(t *testing.T) {
	e := opsEnv(t) // clock: 2026-03-31T10:00:00Z
	for _, args := range []string{
		`{"ends_at":"2026-03-31T09:00:00Z"}`,                                    // past
		`{"starts_at":"2026-03-31T12:00:00Z","ends_at":"2026-03-31T11:00:00Z"}`, // inverted
		`{"starts_at":"2026-03-31T12:00:00Z","ends_at":"2026-04-01T13:00:00Z"}`, // > 24 h
		`{"starts_at":"2026-05-31T12:00:00Z","ends_at":"2026-05-31T13:00:00Z"}`, // > 30 days ahead
		`{"ends_at":"tomorrow"}`,                                                // not RFC 3339
		`{"ends_at":"2026-03-31T11:00:00Z","message":"<b>x</b>"}`,               // markup
	} {
		if r := e.action(t, "schedule_maintenance", `{"args":`+args+`}`, "cfg"); r.code != 400 {
			t.Errorf("%s = %d %s", args, r.code, r.body)
		}
	}
	status := func() map[string]any {
		rec := httptest.NewRecorder()
		e.svc.PublicStatus(rec, httptest.NewRequest("GET", "/api/v1/status", nil))
		var out map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return out
	}
	if _, ok := status()["maintenance"]; ok {
		t.Fatal("maintenance announced before it was scheduled")
	}
	id := e.approveAction(t, "schedule_maintenance", `{"ends_at":"2026-03-31T11:00:00Z","message":"Changement de disque"}`)
	m, ok := status()["maintenance"].(map[string]any)
	if !ok || m["starts_at"] != "2026-03-31T10:00:00Z" || m["ends_at"] != "2026-03-31T11:00:00Z" || m["message"] != "Changement de disque" {
		t.Fatalf("status = %v", status())
	}

	// a breach during the window opens its issue but wakes nobody
	h := &hook{}
	srv := httptest.NewServer(h.handler(200))
	defer srv.Close()
	e.svc.SetWatchConfig(WatchConfig{WebhookURL: srv.URL})
	e.seedSessions(t, 100, 10*time.Minute)
	e.seedErrorsAgo(t, 10, 5*time.Minute, "STREAM_TIMEOUT", "nyaa.si")
	if _, err := e.svc.EvaluateWatch(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(h.names()) != 0 {
		t.Fatalf("notified during maintenance: %v", h.names())
	}
	if n := e.count(t, `SELECT COUNT(*) FROM issues WHERE source = 'watch:error_rate_pct'`); n != 1 {
		t.Fatalf("issues = %d", n)
	}
	if n := e.count(t, `SELECT COUNT(*) FROM watch_runs WHERE webhook_failed > 0`); n != 0 {
		t.Fatal("a muted notification counted as a failure")
	}

	if u := e.undoApproval(t, id); u.code != 200 {
		t.Fatalf("undo = %d %s", u.code, u.body)
	}
	if _, ok := status()["maintenance"]; ok {
		t.Fatal("maintenance still announced after undo")
	}
}

// The registry must not be able to reach an effect through a stored approval once its collaborator
// is gone (approved after a restart without the library, for instance).
func TestApprovedRuntimeActionFailsCleanlyWithoutCollaborator(t *testing.T) {
	e := opsEnv(t)
	e.svc.SetOpsHooks(OpsHooks{PurgeCache: func(context.Context, string) (int, error) { return 0, nil }})
	r := e.action(t, "purge_cache", `{"args":{"scope":"indexer_results"},"dry_run":false,"justification":"j","expected_effect":"e"}`, "cfg")
	if r.code != 202 {
		t.Fatalf("%d %s", r.code, r.body)
	}
	e.svc.SetOpsHooks(OpsHooks{})
	id := fmt.Sprint(int64(pbNum(t, r.data["approval_id"])))
	if a := e.do(t, "POST", "/approvals/"+id+"/approve", "", pbAdminCSRF); a.code != http.StatusServiceUnavailable {
		t.Fatalf("approve = %d %s", a.code, a.body)
	}
	if g := e.do(t, "GET", "/approvals/"+id, "", e.as("diag")); g.data["status"] != "failed" {
		t.Fatalf("approval = %s", g.body)
	}
}
