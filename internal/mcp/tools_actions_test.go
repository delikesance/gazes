package mcp

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// adminDo plays a human: an admin session with the CSRF header, straight on the admin router.
func (e *env) adminDo(t *testing.T, method, path, body string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, "/api/v1/admin"+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Gazes-Admin", "1")
	req.AddCookie(&http.Cookie{Name: "sid", Value: "1"})
	rec := httptest.NewRecorder()
	e.router.ServeHTTP(rec, req)
	var env struct {
		Data map[string]any `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	return rec.Code, env.Data
}

func (e *env) exec(t *testing.T, q string, args ...any) {
	t.Helper()
	if _, err := e.db.Exec(q, args...); err != nil {
		t.Fatal(err)
	}
}

func (e *env) one(t *testing.T, q string, args ...any) string {
	t.Helper()
	var s sql.NullString
	if err := e.db.QueryRow(q, args...).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s.String
}

func jsonOf(t *testing.T, text string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(text), &m); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, text)
	}
	return m
}

func callOK(t *testing.T, cs *sdk.ClientSession, name string, args map[string]any) map[string]any {
	t.Helper()
	text, isErr := call(t, cs, name, args)
	if isErr {
		t.Fatalf("%s: error result %s", name, text)
	}
	return jsonOf(t, text)
}

func callErr(t *testing.T, cs *sdk.ClientSession, name string, args map[string]any) string {
	t.Helper()
	text, isErr := call(t, cs, name, args)
	if !isErr {
		t.Fatalf("%s: expected an error, got %s", name, text)
	}
	return text
}

func TestActionToolsListedByScope(t *testing.T) {
	e := newEnv(t, nil)
	actionNames := []string{"set_issue_status", "create_issue", "add_note", "set_alert_threshold", "get_approval"}
	tests := []struct {
		name   string
		scopes []string
		want   []string
		absent []string
	}{
		{"metrics only", []string{"metrics:read"}, nil, actionNames},
		{"ops:write", []string{"ops:write"}, []string{"set_issue_status", "create_issue", "add_note", "retry_source", "warm_cache", "requeue_av1"}, []string{"set_alert_threshold", "get_approval", "pause_source"}},
		{"config:write", []string{"config:write"}, []string{"set_alert_threshold", "pause_source", "purge_cache", "limit_concurrent_streams", "schedule_maintenance"}, []string{"set_issue_status", "create_issue", "add_note", "get_approval", "retry_source"}},
		{"diagnostics:read", []string{"diagnostics:read"}, []string{"get_approval"}, []string{"set_issue_status", "create_issue", "add_note", "set_alert_threshold"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tok, _ := e.token(t, tc.scopes...)
			names := toolNames(t, e.mustConnect(t, tok))
			for _, n := range tc.want {
				if !has(names, n) {
					t.Errorf("%s missing from %v", n, names)
				}
			}
			for _, n := range tc.absent {
				if has(names, n) {
					t.Errorf("%s must be hidden, got %v", n, names)
				}
			}
		})
	}
	// no tool can decide, undo, flip the kill switch or manage tokens
	all, _ := e.token(t, "metrics:read", "diagnostics:read", "ops:write", "config:write")
	for _, n := range toolNames(t, e.mustConnect(t, all)) {
		l := strings.ToLower(n)
		for _, bad := range []string{"approve", "reject", "undo", "kill", "switch", "token", "suspend"} {
			if strings.Contains(l, bad) {
				t.Errorf("forbidden tool exposed: %s", n)
			}
		}
	}
	// a token without the scope cannot call a hidden tool either
	m, _ := e.token(t, "metrics:read")
	if got := callErr(t, e.mustConnect(t, m), "set_issue_status", map[string]any{"id": "i1", "status": "new"}); !strings.Contains(got, "denied") {
		t.Fatalf("got %s", got)
	}
}

func TestActionToolsDryRunReversibleAndUndo(t *testing.T) {
	e := newEnv(t, nil)
	e.exec(t, `INSERT INTO issues(id, created_at, updated_at, severity, status, title) VALUES ('i1',1,1,'low','new','t')`)
	tok, tid := e.token(t, "ops:write")
	cs := e.mustConnect(t, tok)

	// default: dry run, nothing changes
	r := callOK(t, cs, "set_issue_status", map[string]any{"id": "i1", "status": "resolved"})
	if r["status"] != "dry_run" || r["plan"] == nil {
		t.Fatalf("%v", r)
	}
	if st := e.one(t, `SELECT status FROM issues WHERE id='i1'`); st != "new" {
		t.Fatalf("status = %s", st)
	}
	if r := callOK(t, cs, "set_issue_status", map[string]any{"id": "i1", "status": "resolved", "dry_run": true}); r["status"] != "dry_run" {
		t.Fatalf("%v", r)
	}

	// dry_run:false executes and returns an undo token
	r = callOK(t, cs, "set_issue_status", map[string]any{"id": "i1", "status": "resolved", "note": "done", "dry_run": false})
	if r["status"] != "executed" || r["undo_token"] == nil || r["action_id"] == nil {
		t.Fatalf("%v", r)
	}
	if st := e.one(t, `SELECT status FROM issues WHERE id='i1'`); st != "resolved" {
		t.Fatalf("status = %s", st)
	}
	// the human undoes it
	id := int64(r["action_id"].(float64))
	code, _ := e.adminDo(t, "POST", "/approvals/"+itoa(id)+"/undo", "")
	if code != 200 || e.one(t, `SELECT status FROM issues WHERE id='i1'`) != "new" {
		t.Fatalf("undo = %d", code)
	}

	// create_issue / add_note
	c := callOK(t, cs, "create_issue", map[string]any{"title": "Source down", "severity": "high", "evidence": "e", "dry_run": false})
	iss := c["result"].(map[string]any)["issue"].(map[string]any)["id"].(string)
	callOK(t, cs, "add_note", map[string]any{"issue_id": iss, "text": "looking", "dry_run": false})
	if n := e.one(t, `SELECT note FROM issues WHERE id=?`, iss); n != "looking" {
		t.Fatalf("note = %q", n)
	}

	// invalid arguments are refused before and by the API
	for name, args := range map[string]map[string]any{
		"bad enum":      {"id": "i1", "status": "done"},
		"missing":       {"status": "new"},
		"unknown":       {"id": "i1", "status": "new", "zzz": 1},
		"wrong type":    {"id": "i1", "status": "new", "dry_run": "no"},
		"bad id":        {"id": "A B", "status": "new"},
		"unknown issue": {"id": "nope", "status": "new"},
	} {
		callErr(t, cs, "set_issue_status", args)
		_ = name
	}

	// audit: every call above is in mcp_audit, for this token
	if n := e.one(t, `SELECT COUNT(*) FROM mcp_audit WHERE token_id=? AND tool='set_issue_status'`, tid); n != "9" {
		t.Fatalf("mcp audit rows for set_issue_status = %s", n)
	}
	if n := e.one(t, `SELECT COUNT(*) FROM mcp_audit WHERE tool LIKE 'action:%' AND outcome='executed' AND token_id=?`, tid); n != "3" {
		t.Fatalf("executed rows = %s", n)
	}
	if n := e.one(t, `SELECT COUNT(*) FROM mcp_audit WHERE tool='set_issue_status' AND outcome='error'`); n != "6" {
		t.Fatalf("error rows = %s", n)
	}
}

func itoa(n int64) string { b, _ := json.Marshal(n); return string(b) }

func TestActionToolBudgetAndIdempotency(t *testing.T) {
	e := newEnv(t, nil)
	e.exec(t, `INSERT INTO issues(id, created_at, updated_at, severity, status, title) VALUES ('i1',1,1,'low','new','t')`)
	tok, _ := e.token(t, "ops:write")
	cs := e.mustConnect(t, tok)

	a := callOK(t, cs, "create_issue", map[string]any{"title": "once", "severity": "low", "dry_run": false, "idempotency_key": "abc"})
	b := callOK(t, cs, "create_issue", map[string]any{"title": "once", "severity": "low", "dry_run": false, "idempotency_key": "abc"})
	if b["idempotent_replay"] != true || a["action_id"] != b["action_id"] {
		t.Fatalf("%v / %v", a, b)
	}
	if n := e.one(t, `SELECT COUNT(*) FROM issues WHERE title='once'`); n != "1" {
		t.Fatalf("issues = %s", n)
	}
	if got := callErr(t, cs, "create_issue", map[string]any{"title": "twice", "severity": "low", "dry_run": false, "idempotency_key": "abc"}); !strings.Contains(got, "409") {
		t.Fatalf("got %s", got)
	}

	// budget: 1 already used, allow 2 in total
	e.exec(t, `INSERT INTO settings(key, value) VALUES ('ops.reversible_budget_per_hour','2')`)
	callOK(t, cs, "add_note", map[string]any{"issue_id": "i1", "text": "1", "dry_run": false})
	got := callErr(t, cs, "add_note", map[string]any{"issue_id": "i1", "text": "2", "dry_run": false})
	if !strings.Contains(got, "429") || !strings.Contains(got, "budget_exceeded") {
		t.Fatalf("got %s", got)
	}
	if r := callOK(t, cs, "add_note", map[string]any{"issue_id": "i1", "text": "3"}); r["status"] != "dry_run" {
		t.Fatalf("dry run must stay available: %v", r)
	}
	if n := e.one(t, `SELECT COUNT(*) FROM mcp_audit WHERE outcome='budget_exceeded'`); n != "1" {
		t.Fatalf("budget audit = %s", n)
	}
}

func TestSensitiveActionNeedsApproval(t *testing.T) {
	e := newEnv(t, nil)
	tok, tid := e.token(t, "config:write", "diagnostics:read")
	cs := e.mustConnect(t, tok)
	thr := func() string {
		return e.one(t, `SELECT COALESCE((SELECT value FROM settings WHERE key='threshold.disk_pct'),'default')`)
	}

	if r := callOK(t, cs, "set_alert_threshold", map[string]any{"rule": "disk_pct", "value": 90}); r["status"] != "dry_run" {
		t.Fatalf("%v", r)
	}
	// justification and expected effect are mandatory
	callErr(t, cs, "set_alert_threshold", map[string]any{"rule": "disk_pct", "value": 90, "dry_run": false})
	callErr(t, cs, "set_alert_threshold", map[string]any{"rule": "disk_pct", "value": 99.5, "dry_run": false, "justification": "j", "expected_effect": "e"})
	callErr(t, cs, "set_alert_threshold", map[string]any{"rule": "cpu_pct", "value": 90, "dry_run": false, "justification": "j", "expected_effect": "e"})

	r := callOK(t, cs, "set_alert_threshold", map[string]any{"rule": "disk_pct", "value": 90, "dry_run": false,
		"justification": "disk is filling", "expected_effect": "alert earlier"})
	if r["status"] != "pending_approval" || r["approval_id"] == nil {
		t.Fatalf("%v", r)
	}
	if thr() != "default" {
		t.Fatal("sensitive action executed without approval")
	}
	id := itoa(int64(r["approval_id"].(float64)))
	if g := callOK(t, cs, "get_approval", map[string]any{"id": int(r["approval_id"].(float64))}); g["status"] != "pending" || g["tool"] != "set_alert_threshold" {
		t.Fatalf("%v", g)
	}
	// Claude has no way to approve: the API refuses a token
	req := httptest.NewRequest("POST", "/api/v1/admin/approvals/"+id+"/approve", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	e.router.ServeHTTP(rec, req)
	if rec.Code != 403 || thr() != "default" {
		t.Fatalf("token approve = %d", rec.Code)
	}

	// the human approves: executed once
	if code, d := e.adminDo(t, "POST", "/approvals/"+id+"/approve", ""); code != 200 || d["status"] != "approved" {
		t.Fatalf("approve = %d %v", code, d)
	}
	if thr() != "90" {
		t.Fatalf("threshold = %s", thr())
	}
	if code, _ := e.adminDo(t, "POST", "/approvals/"+id+"/approve", ""); code != 409 {
		t.Fatalf("double approve = %d", code)
	}
	if g := callOK(t, cs, "get_approval", map[string]any{"id": int(r["approval_id"].(float64))}); g["status"] != "approved" || g["decided_by"] != "user:1" {
		t.Fatalf("%v", g)
	}
	callErr(t, cs, "get_approval", map[string]any{"id": 9999})

	if n := e.one(t, `SELECT COUNT(*) FROM mcp_audit WHERE token_id=? AND tool='set_alert_threshold'`, tid); n != "5" {
		t.Fatalf("mcp audit rows = %s", n)
	}
	if n := e.one(t, `SELECT COUNT(*) FROM mcp_audit WHERE tool='action:set_alert_threshold' AND outcome='pending' AND approval_id IS NOT NULL`); n != "1" {
		t.Fatalf("pending audit = %s", n)
	}
	if n := e.one(t, `SELECT COUNT(*) FROM mcp_audit WHERE outcome='approved' AND approval_id=?`, id); n != "1" {
		t.Fatalf("approved audit = %s", n)
	}
}

func TestKillSwitchStopsMCPWrites(t *testing.T) {
	e := newEnv(t, nil)
	e.exec(t, `INSERT INTO issues(id, created_at, updated_at, severity, status, title) VALUES ('i1',1,1,'low','new','t')`)
	tok, _ := e.token(t, "metrics:read", "diagnostics:read", "ops:write", "config:write")
	cs := e.mustConnect(t, tok)
	if code, _ := e.adminDo(t, "PUT", "/ops/kill-switch", `{"suspended":true,"reason":"incident"}`); code != 200 {
		t.Fatalf("kill switch = %d", code)
	}
	for _, c := range []struct {
		tool string
		args map[string]any
	}{
		{"set_issue_status", map[string]any{"id": "i1", "status": "resolved", "dry_run": false}},
		{"set_issue_status", map[string]any{"id": "i1", "status": "resolved"}},
		{"create_issue", map[string]any{"title": "t", "severity": "low", "dry_run": false}},
		{"add_note", map[string]any{"issue_id": "i1", "text": "x", "dry_run": false}},
		{"set_alert_threshold", map[string]any{"rule": "disk_pct", "value": 90, "dry_run": false, "justification": "j", "expected_effect": "e"}},
	} {
		if got := callErr(t, cs, c.tool, c.args); !strings.Contains(got, "423") || !strings.Contains(got, "write_suspended") {
			t.Fatalf("%s: %s", c.tool, got)
		}
	}
	if st := e.one(t, `SELECT status FROM issues WHERE id='i1'`); st != "new" {
		t.Fatal("write went through")
	}
	if n := e.one(t, `SELECT COUNT(*) FROM approvals`); n != "0" {
		t.Fatalf("approvals = %s", n)
	}
	// reads are unchanged
	callOK(t, cs, "get_me", nil)
	callOK(t, cs, "get_overview", map[string]any{"period": 7})
	if n := e.one(t, `SELECT COUNT(*) FROM mcp_audit WHERE outcome='suspended'`); n != "5" {
		t.Fatalf("suspended audit = %s", n)
	}
	// the human lifts it
	if code, _ := e.adminDo(t, "PUT", "/ops/kill-switch", `{"suspended":false}`); code != 200 {
		t.Fatalf("lift = %d", code)
	}
	if r := callOK(t, cs, "set_issue_status", map[string]any{"id": "i1", "status": "resolved", "dry_run": false}); r["status"] != "executed" {
		t.Fatalf("%v", r)
	}
}
