package admin

import (
	"bytes"
	"context"
	"testing"
	"time"
)

func TestSettingsAndAuditRoutesAreAuthenticated(t *testing.T) {
	e := opsEnv(t)
	metricsOnly, _, err := e.svc.store.CreateToken(context.Background(), "m", []string{ScopeMetricsRead}, 0)
	if err != nil {
		t.Fatal(err)
	}
	e.tokens["metrics"] = metricsOnly
	for _, path := range []string{"/settings", "/mcp/tools", "/mcp/audit"} {
		if got := e.do(t, "GET", path, "", nil).code; got != 401 {
			t.Errorf("%s without credentials: %d, want 401", path, got)
		}
		if got := e.do(t, "GET", path, "", e.as("metrics")).code; got != 403 {
			t.Errorf("%s with metrics:read only: %d, want 403", path, got)
		}
		if got := e.do(t, "GET", path, "", e.as("all")).code; got != 200 {
			t.Errorf("%s with diagnostics:read: %d, want 200", path, got)
		}
		if got := e.do(t, "GET", path, "", pbAdminSession).code; got != 200 {
			t.Errorf("%s with an admin session: %d, want 200", path, got)
		}
	}
}

func TestSettingsShowsThresholdsBudgetAndKillSwitch(t *testing.T) {
	e := opsEnv(t)
	r := e.do(t, "GET", "/settings", "", e.as("all"))
	ths, _ := r.data["thresholds"].([]any)
	if len(ths) != len(alertRules) {
		t.Fatalf("thresholds = %d, want %d", len(ths), len(alertRules))
	}
	first := ths[0].(map[string]any)
	if first["rule"] != "disk_pct" || pbNum(t, first["value"]) != 85 || pbNum(t, first["min"]) != 50 || pbNum(t, first["max"]) != 99 {
		t.Fatalf("first threshold = %v", first)
	}
	if pbNum(t, r.data["reversible_budget_per_hour"]) != 30 {
		t.Fatalf("budget = %v", r.data["reversible_budget_per_hour"])
	}
	if ks, _ := r.data["kill_switch"].(map[string]any); ks["suspended"] != false {
		t.Fatalf("kill switch = %v", r.data["kill_switch"])
	}
	if err := e.svc.setSetting(context.Background(), settingThresholdPrefix+"disk_pct", "90", "test"); err != nil {
		t.Fatal(err)
	}
	r = e.do(t, "GET", "/settings", "", e.as("all"))
	if v := r.data["thresholds"].([]any)[0].(map[string]any)["value"]; pbNum(t, v) != 90 {
		t.Fatalf("threshold after change = %v", v)
	}
}

func TestMCPToolsCatalogue(t *testing.T) {
	e := opsEnv(t)
	r := e.do(t, "GET", "/mcp/tools", "", e.as("all"))
	if r.data["enabled"] != false || len(r.data["items"].([]any)) != 0 {
		t.Fatalf("no catalogue set: %v", r.data)
	}
	e.svc.SetToolCatalogue(func() []ToolInfo {
		return []ToolInfo{{Name: "b_tool", Level: "read", Scope: ScopeMetricsRead, Method: "GET", Path: "/b"}, {Name: "a_tool", Level: "reversible"}}
	})
	items := e.do(t, "GET", "/mcp/tools", "", e.as("all")).data["items"].([]any)
	if len(items) != 2 || items[0].(map[string]any)["name"] != "a_tool" {
		t.Fatalf("items must be sorted by name: %v", items)
	}
}

func TestMCPAuditListFiltersAndPaginates(t *testing.T) {
	e := opsEnv(t)
	ctx := context.Background()
	for i, o := range []string{"ok", "ok", "denied", "error"} {
		if err := e.svc.store.RecordMCPAudit(ctx, MCPAuditEntry{TS: time.Unix(1700000000+int64(i), 0), TokenID: 7, Tool: "get_overview",
			ArgsSummary: "period=30", Outcome: o, DurationMS: 5}); err != nil {
			t.Fatal(err)
		}
	}
	all := e.do(t, "GET", "/mcp/audit", "", e.as("all"))
	if pbNum(t, all.data["total"]) != 4 || len(all.data["items"].([]any)) != 4 {
		t.Fatalf("all rows: %v", all.data)
	}
	if first := all.data["items"].([]any)[0].(map[string]any); first["outcome"] != "error" {
		t.Fatalf("newest first expected, got %v", first)
	}
	if r := e.do(t, "GET", "/mcp/audit?outcome=ok", "", e.as("all")); pbNum(t, r.data["total"]) != 2 {
		t.Fatalf("outcome filter: %v", r.data)
	}
	if r := e.do(t, "GET", "/mcp/audit?limit=1&offset=3", "", e.as("all")); len(r.data["items"].([]any)) != 1 {
		t.Fatalf("pagination: %v", r.data)
	}
	if r := e.do(t, "GET", "/mcp/audit?limit=201", "", e.as("all")); r.code != 200 || pbNum(t, r.data["limit"]) != 200 {
		t.Fatalf("limit above the cap must be clamped to 200: %d %v", r.code, r.data["limit"])
	}
	for _, bad := range []string{"?outcome=x'%3B--", "?limit=0", "?offset=-1"} {
		if got := e.do(t, "GET", "/mcp/audit"+bad, "", e.as("all")).code; got != 400 {
			t.Errorf("%s: %d, want 400", bad, got)
		}
	}
}

func TestTokensListIsSessionOnlyAndLeaksNothing(t *testing.T) {
	e := opsEnv(t)
	if got := e.do(t, "GET", "/tokens", "", nil).code; got != 401 {
		t.Errorf("no credentials: %d", got)
	}
	if got := e.do(t, "GET", "/tokens", "", e.as("all")).code; got != 403 {
		t.Errorf("token with every scope must be refused: %d", got)
	}
	r := e.do(t, "GET", "/tokens", "", pbAdminSession)
	if r.code != 200 || len(r.data["items"].([]any)) != 5 {
		t.Fatalf("session list: %d %s", r.code, r.body)
	}
	for _, secret := range []string{e.tokens["cfg"], e.tokens["all"], "token_hash", "hash"} {
		if bytes.Contains(r.body, []byte(secret)) {
			t.Fatalf("response leaks %q", secret)
		}
	}
	it := r.data["items"].([]any)[0].(map[string]any)
	if it["status"] != "active" {
		t.Fatalf("status = %v", it["status"])
	}
}
