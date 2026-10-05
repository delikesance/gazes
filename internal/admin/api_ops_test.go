package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func opsEnv(t *testing.T) *pbEnv {
	t.Helper()
	e := newPBEnv(t)
	for name, scopes := range map[string][]string{
		"cfg": {ScopeConfigWrite},
		"all": {ScopeMetricsRead, ScopeDiagnosticsRead, ScopeOpsWrite, ScopeConfigWrite},
	} {
		tok, _, err := e.svc.store.CreateToken(context.Background(), name, scopes, 0)
		if err != nil {
			t.Fatal(err)
		}
		e.tokens[name] = tok
	}
	return e
}

func (e *pbEnv) seedIssue(t *testing.T, id, status string) {
	t.Helper()
	e.exec(t, `INSERT INTO issues(id, created_at, updated_at, severity, status, title) VALUES (?,1,1,'low',?,'t')`, id, status)
}

func (e *pbEnv) issueRow(t *testing.T, id string) (status, note string) {
	t.Helper()
	var n *string
	if err := e.svc.adminDB().QueryRow(`SELECT status, note FROM issues WHERE id=?`, id).Scan(&status, &n); err != nil {
		t.Fatal(err)
	}
	if n != nil {
		note = *n
	}
	return
}

func (e *pbEnv) count(t *testing.T, q string, args ...any) int {
	t.Helper()
	var n int
	if err := e.svc.adminDB().QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (e *pbEnv) action(t *testing.T, name, body, tok string) pbResp {
	return e.do(t, "POST", "/ops/actions/"+name, body, e.as(tok))
}

func TestActionDryRunDefaultHasNoEffect(t *testing.T) {
	e := opsEnv(t)
	e.seedIssue(t, "i1", "new")
	for _, body := range []string{
		`{"args":{"id":"i1","status":"resolved"}}`,
		`{"args":{"id":"i1","status":"resolved"},"dry_run":true}`,
	} {
		r := e.action(t, "set_issue_status", body, "ops")
		if r.code != 200 || r.data["status"] != "dry_run" {
			t.Fatalf("%s: %d %s", body, r.code, r.body)
		}
		if st, _ := e.issueRow(t, "i1"); st != "new" {
			t.Fatalf("dry run changed the issue: %s", st)
		}
	}
	// a sensitive dry run neither writes the setting nor files an approval
	r := e.action(t, "set_alert_threshold", `{"args":{"rule":"disk_pct","value":90}}`, "cfg")
	if r.code != 200 || r.data["status"] != "dry_run" {
		t.Fatalf("%d %s", r.code, r.body)
	}
	if n := e.count(t, `SELECT COUNT(*) FROM approvals`); n != 0 {
		t.Fatalf("approvals = %d", n)
	}
	if n := e.count(t, `SELECT COUNT(*) FROM settings WHERE key LIKE 'threshold.%'`); n != 0 {
		t.Fatal("threshold written by a dry run")
	}
}

func TestReversibleExecuteAndUndo(t *testing.T) {
	e := opsEnv(t)
	e.seedIssue(t, "i1", "new")
	r := e.action(t, "set_issue_status", `{"args":{"id":"i1","status":"resolved","note":"fixed"},"dry_run":false}`, "ops")
	if r.code != 200 || r.data["status"] != "executed" || r.data["undo_token"] == "" || r.data["undo_token"] == nil {
		t.Fatalf("%d %s", r.code, r.body)
	}
	if st, n := e.issueRow(t, "i1"); st != "resolved" || n != "fixed" {
		t.Fatalf("issue = %s %q", st, n)
	}
	id := fmt.Sprint(int64(pbNum(t, r.data["action_id"])))
	// a token cannot undo; a session without CSRF header neither
	if u := e.do(t, "POST", "/approvals/"+id+"/undo", "", e.as("all")); u.code != 403 {
		t.Fatalf("token undo = %d", u.code)
	}
	if u := e.do(t, "POST", "/approvals/"+id+"/undo", "", pbAdminSession); u.code != 403 {
		t.Fatalf("no csrf = %d", u.code)
	}
	if u := e.do(t, "POST", "/approvals/"+id+"/undo", "", pbAdminCSRF); u.code != 200 || u.data["status"] != "undone" {
		t.Fatalf("undo = %d %s", u.code, u.body)
	}
	if st, n := e.issueRow(t, "i1"); st != "new" || n != "" {
		t.Fatalf("not restored: %s %q", st, n)
	}
	if u := e.do(t, "POST", "/approvals/"+id+"/undo", "", pbAdminCSRF); u.code != 409 {
		t.Fatalf("second undo = %d", u.code)
	}
	// create_issue then undo deletes it; add_note then undo restores the note
	c := e.action(t, "create_issue", `{"args":{"title":"boom","severity":"high"},"dry_run":false}`, "ops")
	if c.code != 200 {
		t.Fatalf("%d %s", c.code, c.body)
	}
	iss := c.data["result"].(map[string]any)["issue"].(map[string]any)["id"].(string)
	if n := e.count(t, `SELECT COUNT(*) FROM issues WHERE id=?`, iss); n != 1 {
		t.Fatal("issue not created")
	}
	a := e.action(t, "add_note", fmt.Sprintf(`{"args":{"issue_id":%q,"text":"first"},"dry_run":false}`, iss), "ops")
	a2 := e.action(t, "add_note", fmt.Sprintf(`{"args":{"issue_id":%q,"text":"second"},"dry_run":false}`, iss), "ops")
	if a.code != 200 || a2.code != 200 {
		t.Fatalf("%s %s", a.body, a2.body)
	}
	if _, n := e.issueRow(t, iss); n != "first\nsecond" {
		t.Fatalf("note = %q", n)
	}
	e.do(t, "POST", fmt.Sprintf("/approvals/%d/undo", int64(pbNum(t, a2.data["action_id"]))), "", pbAdminCSRF)
	if _, n := e.issueRow(t, iss); n != "first" {
		t.Fatalf("note after undo = %q", n)
	}
	e.do(t, "POST", fmt.Sprintf("/approvals/%d/undo", int64(pbNum(t, c.data["action_id"]))), "", pbAdminCSRF)
	if n := e.count(t, `SELECT COUNT(*) FROM issues WHERE id=?`, iss); n != 0 {
		t.Fatal("created issue not deleted by undo")
	}
	if n := e.count(t, `SELECT COUNT(*) FROM mcp_audit WHERE outcome='executed' AND approval_id IS NOT NULL`); n != 4 {
		t.Fatalf("executed audit rows = %d", n)
	}
	if n := e.count(t, `SELECT COUNT(*) FROM mcp_audit WHERE outcome='undone' AND token_id IS NULL`); n != 3 {
		t.Fatalf("undone audit rows = %d", n)
	}
}

func TestReversibleBudget(t *testing.T) {
	e := opsEnv(t)
	e.seedIssue(t, "i1", "new")
	if err := e.svc.setSetting(context.Background(), settingBudget, "2", "test"); err != nil {
		t.Fatal(err)
	}
	for i, want := range []int{200, 200, 429, 429} {
		r := e.action(t, "add_note", fmt.Sprintf(`{"args":{"issue_id":"i1","text":"n%d"},"dry_run":false}`, i), "ops")
		if r.code != want {
			t.Fatalf("call %d = %d %s", i, r.code, r.body)
		}
	}
	if !strings.Contains(string(e.action(t, "add_note", `{"args":{"issue_id":"i1","text":"x"},"dry_run":false}`, "ops").body), "budget_exceeded") {
		t.Fatal("no budget_exceeded code")
	}
	// dry runs and other tokens are not affected
	if r := e.action(t, "add_note", `{"args":{"issue_id":"i1","text":"x"}}`, "ops"); r.code != 200 {
		t.Fatalf("dry run = %d", r.code)
	}
	if r := e.action(t, "add_note", `{"args":{"issue_id":"i1","text":"y"},"dry_run":false}`, "all"); r.code != 200 {
		t.Fatalf("other token = %d %s", r.code, r.body)
	}
	if n := e.count(t, `SELECT COUNT(*) FROM mcp_audit WHERE outcome='budget_exceeded'`); n != 3 {
		t.Fatalf("budget audit rows = %d", n)
	}
	if _, n := e.issueRow(t, "i1"); n != "n0\nn1\ny" {
		t.Fatalf("note = %q", n)
	}
}

func TestIdempotency(t *testing.T) {
	e := opsEnv(t)
	body := `{"args":{"title":"once","severity":"low"},"dry_run":false,"idempotency_key":"k-1"}`
	r1 := e.action(t, "create_issue", body, "ops")
	r2 := e.action(t, "create_issue", body, "ops")
	if r1.code != 200 || r2.code != 200 {
		t.Fatalf("%d %d", r1.code, r2.code)
	}
	if n := e.count(t, `SELECT COUNT(*) FROM issues WHERE title='once'`); n != 1 {
		t.Fatalf("issues = %d", n)
	}
	if r2.data["idempotent_replay"] != true || r2.data["action_id"] != r1.data["action_id"] {
		t.Fatalf("replay = %s", r2.body)
	}
	if n := e.count(t, `SELECT COUNT(*) FROM approvals`); n != 1 {
		t.Fatalf("approvals = %d", n)
	}
	// same key, other arguments: refused; same key, other token: independent
	if r := e.action(t, "create_issue", `{"args":{"title":"other","severity":"low"},"dry_run":false,"idempotency_key":"k-1"}`, "ops"); r.code != 409 {
		t.Fatalf("conflict = %d", r.code)
	}
	if r := e.action(t, "create_issue", body, "all"); r.code != 200 || r.data["idempotent_replay"] == true {
		t.Fatalf("other token = %d %s", r.code, r.body)
	}
	if r := e.action(t, "create_issue", `{"args":{"title":"z","severity":"low"},"dry_run":false,"idempotency_key":"bad key!"}`, "ops"); r.code != 400 {
		t.Fatalf("bad key = %d", r.code)
	}
	// a sensitive request replays its pending approval, it does not file a second one
	s := `{"args":{"rule":"disk_pct","value":90},"dry_run":false,"idempotency_key":"s-1","justification":"j","expected_effect":"e"}`
	p1, p2 := e.action(t, "set_alert_threshold", s, "cfg"), e.action(t, "set_alert_threshold", s, "cfg")
	if p1.code != 202 || p2.code != 202 || p1.data["approval_id"] != p2.data["approval_id"] {
		t.Fatalf("%s %s", p1.body, p2.body)
	}
	if n := e.count(t, `SELECT COUNT(*) FROM approvals WHERE status='pending'`); n != 1 {
		t.Fatalf("pending = %d", n)
	}
}

func TestSensitiveNeverRunsWithoutApproval(t *testing.T) {
	e := opsEnv(t)
	sens := `{"args":{"rule":"disk_pct","value":90},"dry_run":false,"justification":"disk fills up","expected_effect":"alert earlier"}`
	// scope: ops:write alone cannot request a sensitive action
	if r := e.action(t, "set_alert_threshold", sens, "ops"); r.code != 403 {
		t.Fatalf("ops scope = %d", r.code)
	}
	for _, body := range []string{
		`{"args":{"rule":"disk_pct","value":90},"dry_run":false}`,
		`{"args":{"rule":"disk_pct","value":90},"dry_run":false,"justification":"j"}`,
	} {
		if r := e.action(t, "set_alert_threshold", body, "cfg"); r.code != 400 {
			t.Fatalf("missing justification/effect = %d %s", r.code, r.body)
		}
	}
	r := e.action(t, "set_alert_threshold", sens, "cfg")
	if r.code != 202 || r.data["status"] != "pending_approval" {
		t.Fatalf("%d %s", r.code, r.body)
	}
	if v, _ := e.svc.AlertThreshold(context.Background(), "disk_pct"); v != 85 {
		t.Fatalf("threshold changed without approval: %v", v)
	}
	id := fmt.Sprint(int64(pbNum(t, r.data["approval_id"])))
	g := e.do(t, "GET", "/approvals/"+id, "", e.as("diag"))
	if g.code != 200 || g.data["status"] != "pending" || g.data["justification"] != "disk fills up" {
		t.Fatalf("%d %s", g.code, g.body)
	}
	if strings.Contains(string(g.body), "undo_token") {
		t.Fatal("undo token leaked in the approval view")
	}
	list := e.do(t, "GET", "/approvals?status=pending", "", e.as("diag"))
	if items := pbList(t, list.data, "items"); len(items) != 1 {
		t.Fatalf("pending list = %s", list.body)
	}
	if r := e.do(t, "GET", "/approvals?status=nope", "", e.as("diag")); r.code != 400 {
		t.Fatalf("bad status = %d", r.code)
	}

	// a token, even with every scope, cannot decide
	for _, p := range []string{"/approve", "/reject"} {
		if r := e.do(t, "POST", "/approvals/"+id+p, "", e.as("all")); r.code != 403 {
			t.Fatalf("token %s = %d", p, r.code)
		}
	}
	// ... nor a token together with an admin session
	if r := e.do(t, "POST", "/approvals/"+id+"/approve", "", func(r *http.Request) { pbAdminCSRF(r); e.as("all")(r) }); r.code != 403 {
		t.Fatalf("token+session = %d", r.code)
	}
	// session without CSRF header, non admin session, anonymous
	if r := e.do(t, "POST", "/approvals/"+id+"/approve", "", pbAdminSession); r.code != 403 {
		t.Fatalf("no csrf = %d", r.code)
	}
	if r := e.do(t, "POST", "/approvals/"+id+"/approve", "", func(r *http.Request) {
		r.AddCookie(&http.Cookie{Name: "sid", Value: "2"})
		r.Header.Set("X-Gazes-Admin", "1")
	}); r.code != 403 {
		t.Fatalf("non admin = %d", r.code)
	}
	if r := e.do(t, "POST", "/approvals/"+id+"/approve", "", nil); r.code != 401 {
		t.Fatalf("anonymous = %d", r.code)
	}
	if v, _ := e.svc.AlertThreshold(context.Background(), "disk_pct"); v != 85 {
		t.Fatal("threshold changed by a refused approval")
	}

	// the admin session approves: executed once
	a := e.do(t, "POST", "/approvals/"+id+"/approve", "", pbAdminCSRF)
	if a.code != 200 || a.data["status"] != "approved" || a.data["decided_by"] != "user:1" || a.data["undoable"] != true {
		t.Fatalf("approve = %d %s", a.code, a.body)
	}
	if v, _ := e.svc.AlertThreshold(context.Background(), "disk_pct"); v != 90 {
		t.Fatalf("threshold = %v", v)
	}
	if r := e.do(t, "POST", "/approvals/"+id+"/approve", "", pbAdminCSRF); r.code != 409 {
		t.Fatalf("double approve = %d", r.code)
	}
	if r := e.do(t, "POST", "/approvals/"+id+"/reject", "", pbAdminCSRF); r.code != 409 {
		t.Fatalf("reject after approve = %d", r.code)
	}
	if r := e.do(t, "POST", "/approvals/999/approve", "", pbAdminCSRF); r.code != 404 {
		t.Fatalf("unknown = %d", r.code)
	}
	// undo restores the default
	if r := e.do(t, "POST", "/approvals/"+id+"/undo", "", pbAdminCSRF); r.code != 200 {
		t.Fatalf("undo = %d %s", r.code, r.body)
	}
	if v, _ := e.svc.AlertThreshold(context.Background(), "disk_pct"); v != 85 {
		t.Fatalf("threshold after undo = %v", v)
	}
	if n := e.count(t, `SELECT COUNT(*) FROM mcp_audit WHERE approval_id = ? AND outcome IN ('pending','approved','undone')`, id); n != 3 {
		t.Fatalf("audit rows = %d", n)
	}
}

func TestRejectNeverExecutes(t *testing.T) {
	e := opsEnv(t)
	r := e.action(t, "set_alert_threshold", `{"args":{"rule":"error_rate_pct","value":10},"dry_run":false,"justification":"j","expected_effect":"e"}`, "cfg")
	id := fmt.Sprint(int64(pbNum(t, r.data["approval_id"])))
	rj := e.do(t, "POST", "/approvals/"+id+"/reject", `{"reason":"no"}`, pbAdminCSRF)
	if rj.code != 200 || rj.data["status"] != "rejected" {
		t.Fatalf("%d %s", rj.code, rj.body)
	}
	if r := e.do(t, "POST", "/approvals/"+id+"/approve", "", pbAdminCSRF); r.code != 409 {
		t.Fatalf("approve after reject = %d", r.code)
	}
	if v, _ := e.svc.AlertThreshold(context.Background(), "error_rate_pct"); v != 5 {
		t.Fatalf("threshold = %v", v)
	}
	if n := e.count(t, `SELECT COUNT(*) FROM mcp_audit WHERE outcome='rejected'`); n != 1 {
		t.Fatal("reject not audited")
	}
}

func TestKillSwitch(t *testing.T) {
	e := opsEnv(t)
	e.seedIssue(t, "i1", "new")
	pend := e.action(t, "set_alert_threshold", `{"args":{"rule":"disk_pct","value":90},"dry_run":false,"justification":"j","expected_effect":"e"}`, "cfg")
	pid := fmt.Sprint(int64(pbNum(t, pend.data["approval_id"])))

	if r := e.do(t, "GET", "/ops/kill-switch", "", e.as("diag")); r.code != 200 || r.data["suspended"] != false {
		t.Fatalf("%d %s", r.code, r.body)
	}
	put := func(mut func(*http.Request), body string) pbResp {
		return e.do(t, "PUT", "/ops/kill-switch", body, mut)
	}
	on := `{"suspended":true,"reason":"incident"}`
	for name, mut := range map[string]func(*http.Request){"token": e.as("all"), "no csrf": pbAdminSession, "anonymous": nil} {
		if r := put(mut, on); r.code != 403 && r.code != 401 {
			t.Fatalf("%s put = %d", name, r.code)
		}
	}
	if e.svc.WriteSuspended(context.Background()) {
		t.Fatal("suspended by a refused call")
	}
	if r := put(pbAdminCSRF, `{"suspended":true,"extra":1}`); r.code != 400 {
		t.Fatalf("unknown field = %d", r.code)
	}
	if r := put(pbAdminCSRF, `{"reason":"x"}`); r.code != 400 {
		t.Fatalf("missing suspended = %d", r.code)
	}
	if r := put(pbAdminCSRF, on); r.code != 200 || r.data["suspended"] != true || r.data["reason"] != "incident" {
		t.Fatalf("on = %d %s", r.code, r.body)
	}

	// every token write is refused 423; reads are unchanged
	for _, c := range []struct{ method, path, body, tok string }{
		{"POST", "/ops/actions/set_issue_status", `{"args":{"id":"i1","status":"resolved"},"dry_run":false}`, "ops"},
		{"POST", "/ops/actions/set_issue_status", `{"args":{"id":"i1","status":"resolved"}}`, "ops"},
		{"POST", "/ops/actions/set_alert_threshold", `{"args":{"rule":"disk_pct","value":90},"dry_run":false,"justification":"j","expected_effect":"e"}`, "cfg"},
		{"POST", "/issues", `{"title":"t","severity":"low"}`, "ops"},
		{"PATCH", "/issues/i1", `{"status":"resolved"}`, "ops"},
	} {
		r := e.do(t, c.method, c.path, c.body, e.as(c.tok))
		if r.code != 423 || !strings.Contains(string(r.body), "write_suspended") {
			t.Fatalf("%s %s = %d %s", c.method, c.path, r.code, r.body)
		}
	}
	if st, _ := e.issueRow(t, "i1"); st != "new" {
		t.Fatal("issue changed while suspended")
	}
	if n := e.count(t, `SELECT COUNT(*) FROM mcp_audit WHERE outcome='suspended'`); n != 5 {
		t.Fatalf("suspended audit rows = %d", n)
	}
	for _, p := range []string{"/issues", "/approvals", "/ops/kill-switch", "/ops/actions"} {
		if r := e.do(t, "GET", p, "", e.as("diag")); r.code != 200 {
			t.Fatalf("read %s = %d", p, r.code)
		}
	}
	// approving is refused too, even for a human, until the switch is lifted
	if r := e.do(t, "POST", "/approvals/"+pid+"/approve", "", pbAdminCSRF); r.code != 423 {
		t.Fatalf("approve while suspended = %d", r.code)
	}
	// a human in session can still write
	if r := e.do(t, "PATCH", "/issues/i1", `{"status":"in_progress"}`, pbAdminCSRF); r.code != 200 {
		t.Fatalf("session write = %d %s", r.code, r.body)
	}
	if r := e.do(t, "POST", "/ops/actions/set_issue_status", `{"args":{"id":"i1","status":"resolved"},"dry_run":false}`, pbAdminCSRF); r.code != 200 {
		t.Fatalf("session action = %d %s", r.code, r.body)
	}
	// lift it
	if r := put(pbAdminCSRF, `{"suspended":false}`); r.code != 200 || r.data["suspended"] != false {
		t.Fatalf("off = %d %s", r.code, r.body)
	}
	if r := e.do(t, "PATCH", "/issues/i1", `{"status":"new"}`, e.as("ops")); r.code != 200 {
		t.Fatalf("token write after lift = %d", r.code)
	}
	if r := e.do(t, "POST", "/approvals/"+pid+"/approve", "", pbAdminCSRF); r.code != 200 {
		t.Fatalf("approve after lift = %d %s", r.code, r.body)
	}
	if n := e.count(t, `SELECT COUNT(*) FROM mcp_audit WHERE tool='kill_switch'`); n != 2 {
		t.Fatalf("kill switch audit rows = %d", n)
	}
}

func TestKillSwitchFailsClosed(t *testing.T) {
	e := opsEnv(t)
	e.exec(t, `DROP TABLE settings`)
	if !e.svc.WriteSuspended(context.Background()) {
		t.Fatal("unreadable kill switch must suspend writes")
	}
}

func TestActionInvalidArgs(t *testing.T) {
	e := opsEnv(t)
	e.seedIssue(t, "i1", "new")
	tests := []struct {
		name, action, body, tok string
		code                    int
	}{
		{"unknown action", "nope", `{}`, "ops", 404},
		{"bad status", "set_issue_status", `{"args":{"id":"i1","status":"done"}}`, "ops", 400},
		{"missing id", "set_issue_status", `{"args":{"status":"new"}}`, "ops", 400},
		{"bad id", "set_issue_status", `{"args":{"id":"../x","status":"new"}}`, "ops", 400},
		{"unknown arg", "set_issue_status", `{"args":{"id":"i1","status":"new","x":1}}`, "ops", 400},
		{"unknown issue", "set_issue_status", `{"args":{"id":"zz","status":"new"}}`, "ops", 404},
		{"unknown body field", "set_issue_status", `{"args":{"id":"i1","status":"new"},"oops":1}`, "ops", 400},
		{"not json", "set_issue_status", `nope`, "ops", 400},
		{"bad title", "create_issue", `{"args":{"title":"  ","severity":"low"}}`, "ops", 400},
		{"bad severity", "create_issue", `{"args":{"title":"t","severity":"urgent"}}`, "ops", 400},
		{"note too long", "add_note", `{"args":{"issue_id":"i1","text":"` + strings.Repeat("a", 2001) + `"}}`, "ops", 400},
		{"rule not allowed", "set_alert_threshold", `{"args":{"rule":"cpu_pct","value":5}}`, "cfg", 400},
		{"below min", "set_alert_threshold", `{"args":{"rule":"disk_pct","value":10}}`, "cfg", 400},
		{"above max", "set_alert_threshold", `{"args":{"rule":"error_rate_pct","value":51}}`, "cfg", 400},
		{"value not a number", "set_alert_threshold", `{"args":{"rule":"disk_pct","value":"90"}}`, "cfg", 400},
		{"metrics scope", "set_issue_status", `{"args":{"id":"i1","status":"new"}}`, "metrics", 403},
		{"diag scope", "create_issue", `{"args":{"title":"t","severity":"low"}}`, "diag", 403},
		{"body too large", "add_note", `{"args":{"issue_id":"i1","text":"` + strings.Repeat("a", 17<<10) + `"}}`, "ops", 413},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := e.action(t, tc.action, tc.body, tc.tok)
			if r.code != tc.code {
				t.Fatalf("%d %s", r.code, r.body)
			}
		})
	}
	if r := e.do(t, "POST", "/ops/actions/add_note", `{"args":{"issue_id":"i1","text":"a"}}`, func(r *http.Request) { e.as("ops")(r); r.Header.Set("Content-Type", "text/plain") }); r.code != 415 {
		t.Fatalf("content type = %d", r.code)
	}
	if st, n := e.issueRow(t, "i1"); st != "new" || n != "" {
		t.Fatal("invalid call changed the issue")
	}
}

func TestNotImplementedActionsNeverHaveEffect(t *testing.T) {
	e := opsEnv(t)
	cat := e.do(t, "GET", "/ops/actions", "", e.as("diag"))
	items := pbList(t, cat.data, "items")
	impl := map[string]bool{}
	for _, it := range items {
		m := it.(map[string]any)
		impl[m["name"].(string)] = m["implemented"].(bool)
	}
	for _, n := range []string{"set_issue_status", "create_issue", "add_note", "set_alert_threshold"} {
		if !impl[n] {
			t.Errorf("%s should be implemented", n)
		}
	}
	stubs := []struct{ name, tok string }{
		{"retry_source", "ops"}, {"warm_cache", "ops"}, {"requeue_av1", "ops"},
		{"pause_source", "cfg"}, {"purge_cache", "cfg"}, {"limit_concurrent_streams", "cfg"}, {"schedule_maintenance", "cfg"},
	}
	for _, s := range stubs {
		if impl[s.name] {
			t.Errorf("%s should be declared not implemented", s.name)
		}
		for _, body := range []string{`{}`, `{"dry_run":false,"justification":"j","expected_effect":"e"}`} {
			r := e.action(t, s.name, body, s.tok)
			if r.code != 501 || !strings.Contains(string(r.body), "not_implemented") {
				t.Fatalf("%s %s = %d %s", s.name, body, r.code, r.body)
			}
		}
	}
	if n := e.count(t, `SELECT COUNT(*) FROM approvals`); n != 0 {
		t.Fatal("a not implemented action filed an approval")
	}
}

func TestOpsRoutesAreAuthenticated(t *testing.T) {
	e := opsEnv(t)
	for _, c := range []struct{ method, path string }{
		{"GET", "/approvals"}, {"GET", "/approvals/1"}, {"GET", "/ops/kill-switch"}, {"GET", "/ops/actions"},
		{"POST", "/ops/actions/add_note"}, {"PUT", "/ops/kill-switch"},
	} {
		if r := e.do(t, c.method, c.path, "", nil); r.code != 401 {
			t.Errorf("%s %s anonymous = %d", c.method, c.path, r.code)
		}
		if r := e.do(t, c.method, c.path, "", e.as("metrics")); r.code != 403 {
			t.Errorf("%s %s metrics token = %d", c.method, c.path, r.code)
		}
	}
}

func TestThresholdRoundTrip(t *testing.T) {
	e := opsEnv(t)
	if _, err := e.svc.AlertThreshold(context.Background(), "nope"); err == nil {
		t.Fatal("unknown rule accepted")
	}
	if err := e.svc.setSetting(context.Background(), settingThresholdPrefix+"disk_pct", "91", "t"); err != nil {
		t.Fatal(err)
	}
	r := e.action(t, "set_alert_threshold", `{"args":{"rule":"disk_pct","value":95}}`, "cfg")
	var plan struct{ Details map[string]float64 }
	b, _ := json.Marshal(r.data["plan"])
	_ = json.Unmarshal(b, &plan)
	if plan.Details["from"] != 91 || plan.Details["to"] != 95 {
		t.Fatalf("plan = %s", b)
	}
}
