package admin

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type hook struct {
	mu     sync.Mutex
	events []watchEvent
	sigs   []string
	bodies [][]byte
}

func (h *hook) handler(status int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var ev watchEvent
		_ = json.Unmarshal(b, &ev)
		h.mu.Lock()
		h.events = append(h.events, ev)
		h.sigs = append(h.sigs, r.Header.Get("X-Gazes-Signature"))
		h.bodies = append(h.bodies, b)
		h.mu.Unlock()
		w.WriteHeader(status)
	})
}

func (h *hook) names() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := []string{}
	for _, e := range h.events {
		out = append(out, e.Event+":"+e.Rule)
	}
	return out
}

func (e *pbEnv) seedSessions(t *testing.T, n int, ago time.Duration) {
	t.Helper()
	start := e.svc.now().Add(-ago).Unix()
	for i := 0; i < n; i++ {
		if _, err := e.accounts.Exec(`INSERT INTO watch_sessions(user_id,session_id,season_id,anime_id,episode,started_at,updated_at) VALUES(?,?,1,1,1,?,?)`,
			i+1, fmt.Sprint("w", start, "-", i), start, start); err != nil {
			t.Fatal(err)
		}
	}
}

func (e *pbEnv) seedErrorsAgo(t *testing.T, n int, ago time.Duration, code, source string) {
	t.Helper()
	for i := 0; i < n; i++ {
		e.exec(t, `INSERT INTO playback_errors(ts, code, source) VALUES (?,?,?)`, e.svc.now().Add(-ago).Unix(), code, nullStr(source))
	}
}

func (e *pbEnv) ruleState(t *testing.T, states []RuleState, rule string) RuleState {
	t.Helper()
	for _, s := range states {
		if s.Rule == rule {
			return s
		}
	}
	t.Fatalf("no state for %s", rule)
	return RuleState{}
}

func TestWatchBreachOpensOneIssueNotifiesOnceAndRecovers(t *testing.T) {
	e := opsEnv(t)
	h := &hook{}
	srv := httptest.NewServer(h.handler(200))
	defer srv.Close()
	e.svc.SetWatchConfig(WatchConfig{WebhookURL: srv.URL, WebhookSecret: "s3cret"})
	ctx := context.Background()

	e.seedSessions(t, 100, 10*time.Minute)
	e.seedErrorsAgo(t, 10, 5*time.Minute, "STREAM_TIMEOUT", "nyaa.si") // 10 % > 5 % default threshold

	states, err := e.svc.EvaluateWatch(ctx)
	if err != nil {
		t.Fatal(err)
	}
	st := e.ruleState(t, states, RuleErrorRate)
	if st.State != WatchBreached || st.Value == nil || *st.Value != 10 || st.IssueID == nil {
		t.Fatalf("error rate state = %+v", st)
	}
	if n := e.count(t, `SELECT COUNT(*) FROM issues WHERE source = 'watch:error_rate_pct' AND severity = 'high'`); n != 1 {
		t.Fatalf("issues = %d, want 1", n)
	}
	if got := h.names(); len(got) != 1 || got[0] != "watch.breached:error_rate_pct" {
		t.Fatalf("webhook events = %v", got)
	}
	mac := hmac.New(sha256.New, []byte("s3cret"))
	mac.Write(h.bodies[0])
	if h.sigs[0] != "sha256="+hex.EncodeToString(mac.Sum(nil)) {
		t.Fatalf("bad signature %q", h.sigs[0])
	}
	if h.events[0].IssueID != *st.IssueID || h.events[0].Runbook == "" {
		t.Fatalf("event = %+v", h.events[0])
	}

	// still breached: no second issue, no second webhook
	if _, err := e.svc.EvaluateWatch(ctx); err != nil {
		t.Fatal(err)
	}
	if n := e.count(t, `SELECT COUNT(*) FROM issues`); n != 1 {
		t.Fatalf("issues after second pass = %d", n)
	}
	if len(h.names()) != 1 {
		t.Fatalf("webhook repeated: %v", h.names())
	}

	// the cause disappears: stays breached for two passes, recovers on the third; the issue stays open
	e.exec(t, `DELETE FROM playback_errors`)
	for i := 1; i <= 3; i++ {
		states, _ = e.svc.EvaluateWatch(ctx)
		got := e.ruleState(t, states, RuleErrorRate).State
		want := WatchBreached
		if i == 3 {
			want = WatchOK
		}
		if got != want {
			t.Fatalf("pass %d: state %s, want %s", i, got, want)
		}
	}
	if got := h.names(); len(got) != 2 || got[1] != "watch.recovered:error_rate_pct" {
		t.Fatalf("webhook events = %v", got)
	}
	var status, note string
	if err := e.svc.adminDB().QueryRow(`SELECT status, COALESCE(note,'') FROM issues`).Scan(&status, &note); err != nil {
		t.Fatal(err)
	}
	if status != "new" || note == "" {
		t.Fatalf("recovery must note the issue and leave it open: status=%q note=%q", status, note)
	}
}

func TestWatchNeedsEnoughSessionsAndSkipsWhatItCannotMeasure(t *testing.T) {
	e := opsEnv(t)
	e.seedSessions(t, 5, 10*time.Minute)
	e.seedErrorsAgo(t, 5, 5*time.Minute, "STREAM_TIMEOUT", "x") // 100 % but only 5 sessions
	states, err := e.svc.EvaluateWatch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, rule := range []string{RuleErrorRate, RuleStartup, RuleSaturation, RuleDisk} {
		if st := e.ruleState(t, states, rule); st.State != WatchNotMeasured || st.Value != nil {
			t.Errorf("%s = %+v, want not_measured without a value", rule, st)
		}
	}
	if n := e.count(t, `SELECT COUNT(*) FROM issues`); n != 0 {
		t.Fatalf("an unmeasured rule must not open an issue: %d", n)
	}
}

func TestWatchDeadSourceBreach(t *testing.T) {
	e := opsEnv(t)
	e.seedErrorsAgo(t, 4, 10*time.Minute, "SRC_DEAD", "c411")
	states, _ := e.svc.EvaluateWatch(context.Background())
	if st := e.ruleState(t, states, RuleSourceFailed); st.State != WatchNear {
		t.Fatalf("4 failures should be near: %+v", st)
	}
	e.seedErrorsAgo(t, 1, 10*time.Minute, "SRC_DEAD", "c411")
	e.seedErrorsAgo(t, 9, 40*time.Minute, "SRC_DEAD", "c411") // outside the 30 min window
	states, _ = e.svc.EvaluateWatch(context.Background())
	st := e.ruleState(t, states, RuleSourceFailed)
	if st.State != WatchBreached || *st.Value != 5 {
		t.Fatalf("5 failures in the window should breach: %+v", st)
	}
	if n := e.count(t, `SELECT COUNT(*) FROM issues WHERE severity = 'medium' AND source = 'watch:source_failures'`); n != 1 {
		t.Fatalf("source issue = %d", n)
	}
}

func TestWatchWebhookFailuresNeverBreakTheLoop(t *testing.T) {
	for name, status := range map[string]int{"server error": 500, "redirect not followed": 302} {
		t.Run(name, func(t *testing.T) {
			e := opsEnv(t)
			h := &hook{}
			srv := httptest.NewServer(h.handler(status))
			defer srv.Close()
			e.svc.SetWatchConfig(WatchConfig{WebhookURL: srv.URL})
			e.seedSessions(t, 100, 10*time.Minute)
			e.seedErrorsAgo(t, 10, 5*time.Minute, "STREAM_TIMEOUT", "x")
			if _, err := e.svc.EvaluateWatch(context.Background()); err != nil {
				t.Fatal(err)
			}
			if n := e.count(t, `SELECT COUNT(*) FROM issues`); n != 1 {
				t.Fatalf("the issue must exist even when the webhook fails: %d", n)
			}
			if n := e.count(t, `SELECT webhook_failed FROM watch_runs ORDER BY id DESC LIMIT 1`); n != 1 {
				t.Fatalf("webhook_failed = %d", n)
			}
		})
	}
	// an invalid URL is dropped, not used
	e := opsEnv(t)
	e.svc.SetWatchConfig(WatchConfig{WebhookURL: "file:///etc/passwd"})
	if e.svc.watchConfig().WebhookURL != "" {
		t.Fatal("non http(s) webhook URL must be ignored")
	}
}

func TestWatchEffectIsMeasuredAfterTheFix(t *testing.T) {
	e := opsEnv(t)
	ctx := context.Background()
	e.seedSessions(t, 100, 10*time.Minute)
	e.seedErrorsAgo(t, 10, 5*time.Minute, "STREAM_TIMEOUT", "x")
	states, _ := e.svc.EvaluateWatch(ctx)
	id := *e.ruleState(t, states, RuleErrorRate).IssueID

	// fixed: errors gone, a human resolves the issue
	e.exec(t, `DELETE FROM playback_errors`)
	e.exec(t, `UPDATE issues SET status = 'resolved' WHERE id = ?`, id)
	if _, err := e.svc.EvaluateWatch(ctx); err != nil {
		t.Fatal(err)
	}
	var resolvedAt *int64
	var vo, vr, va *float64
	read := func() {
		if err := e.svc.adminDB().QueryRow(`SELECT resolved_at, value_open, value_resolved, value_after FROM watch_effects WHERE issue_id = ?`, id).Scan(&resolvedAt, &vo, &vr, &va); err != nil {
			t.Fatal(err)
		}
	}
	read()
	if resolvedAt == nil || *vo != 10 || vr == nil || *vr != 0 || va != nil {
		t.Fatalf("after resolving: resolved_at=%v open=%v resolved=%v after=%v", resolvedAt, vo, vr, va)
	}
	// 24 h later (sessions fresh again so the rate is measurable)
	e.svc.now = func() time.Time { return at("2026-04-01T11:00:00Z") }
	e.seedSessions(t, 100, 10*time.Minute)
	if _, err := e.svc.EvaluateWatch(ctx); err != nil {
		t.Fatal(err)
	}
	read()
	if va == nil || *va != 0 {
		t.Fatalf("value_after = %v", va)
	}
	r := e.do(t, "GET", "/watch", "", e.as("all"))
	eff := r.data["effects"].([]any)[0].(map[string]any)
	if eff["verdict"] != "improved" || eff["issue_id"] != id {
		t.Fatalf("effect = %v", eff)
	}
}

func TestWatchEndpoint(t *testing.T) {
	e := opsEnv(t)
	e.svc.SetWatchConfig(WatchConfig{WebhookURL: "https://hooks.example/secret-path"})
	if got := e.do(t, "GET", "/watch", "", nil).code; got != 401 {
		t.Errorf("no credentials: %d", got)
	}
	metricsOnly, _, _ := e.svc.store.CreateToken(context.Background(), "m", []string{ScopeMetricsRead}, 0)
	e.tokens["metrics"] = metricsOnly
	if got := e.do(t, "GET", "/watch", "", e.as("metrics")).code; got != 403 {
		t.Errorf("metrics:read only: %d", got)
	}
	r := e.do(t, "GET", "/watch", "", e.as("all")) // reading evaluates nothing
	if r.code != 200 || len(r.data["rules"].([]any)) != 5 || r.data["webhook_configured"] != true {
		t.Fatalf("%d %s", r.code, r.body)
	}
	if first := r.data["rules"].([]any)[0].(map[string]any); first["state"] != WatchNotMeasured || first["detail"] != "pas encore évalué" {
		t.Fatalf("unevaluated rule = %v", first)
	}
	if n := e.count(t, `SELECT COUNT(*) FROM watch_runs`); n != 0 {
		t.Fatalf("GET /watch must not evaluate: %d runs", n)
	}
	if len(r.body) == 0 || strings.Contains(string(r.body), "secret-path") {
		t.Fatal("the webhook URL must never be returned")
	}
}

func TestPruneKeepsRecentRowsAndClampsRetention(t *testing.T) {
	e := opsEnv(t)
	ctx := context.Background()
	old := e.svc.now().Add(-200 * 24 * time.Hour).Unix()
	recent := e.svc.now().Add(-10 * 24 * time.Hour).Unix()
	for _, ts := range []int64{old, recent} {
		e.exec(t, `INSERT INTO playback_errors(ts, code) VALUES (?, 'X')`, ts)
		e.exec(t, `INSERT INTO mcp_audit(ts, tool, outcome, duration_ms) VALUES (?, 'get_me', 'ok', 1)`, ts)
	}
	if err := e.svc.Prune(ctx); err != nil {
		t.Fatal(err)
	}
	for _, tbl := range []string{"playback_errors", "mcp_audit"} {
		if n := e.count(t, `SELECT COUNT(*) FROM `+tbl); n != 1 {
			t.Errorf("%s rows after prune = %d, want 1", tbl, n)
		}
	}
	for in, want := range map[string]int{"": 180, "5": 30, "400": 400, "99999": 730, "abc": 180} {
		if in != "" {
			if err := e.svc.setSetting(ctx, settingRetentionDays, in, "test"); err != nil {
				t.Fatal(err)
			}
		}
		if got := e.svc.RetentionDays(ctx); got != want {
			t.Errorf("retention %q = %d, want %d", in, got, want)
		}
	}
}

func TestRunWatchStopsWithItsContext(t *testing.T) {
	e := opsEnv(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { e.svc.RunWatch(ctx, 10*time.Millisecond, nil); close(done) }()
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("RunWatch did not stop")
	}
	if n := e.count(t, `SELECT COUNT(*) FROM watch_runs`); n == 0 {
		t.Fatal("RunWatch never evaluated")
	}
}

func TestWatchStartupP95(t *testing.T) {
	e := opsEnv(t)
	ctx := context.Background()
	for i := 0; i < watchMinStartups-1; i++ {
		e.svc.RecordStartup(ctx, 9000)
	}
	states, _ := e.svc.EvaluateWatch(ctx)
	if st := e.ruleState(t, states, RuleStartup); st.State != WatchNotMeasured || st.Value != nil {
		t.Fatalf("%d startups must stay unmeasured: %+v", watchMinStartups-1, st)
	}
	e.exec(t, `INSERT INTO playback_startups(ts, ms) VALUES (?, 60000)`, e.svc.now().Add(-2*time.Hour).Unix()) // outside the window
	e.svc.RecordStartup(ctx, 9000)
	states, _ = e.svc.EvaluateWatch(ctx)
	st := e.ruleState(t, states, RuleStartup)
	if st.State != WatchBreached || st.Value == nil || *st.Value != 9 {
		t.Fatalf("p95 of 9 s against the 5 s default should breach: %+v", st)
	}
	if n := e.count(t, `SELECT COUNT(*) FROM issues WHERE source = 'watch:startup_p95_s'`); n != 1 {
		t.Fatalf("breach should open one issue, got %d", n)
	}
}
