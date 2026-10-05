package mcp

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestReadToolsMetricsScope(t *testing.T) {
	tools := readTools()
	metricsOnly := []string{"get_views", "list_top_anime", "get_users_summary", "list_users", "get_user", "get_growth", "get_retention", "get_funnel", "get_costs"}
	for _, name := range metricsOnly {
		found := false
		for _, d := range tools {
			if d.Name == name {
				found = true
				if d.Scope != "metrics:read" {
					t.Fatalf("%s scope = %s, want metrics:read", d.Name, d.Scope)
				}
				break
			}
		}
		if !found {
			t.Fatalf("missing tool %s", name)
		}
	}
}

func TestReadToolsDiagnosticsScope(t *testing.T) {
	tools := readTools()
	diagOnly := []string{"get_player_health", "list_playback_errors", "get_errors_summary", "get_sources_health", "list_issues", "get_issue"}
	for _, name := range diagOnly {
		found := false
		for _, d := range tools {
			if d.Name == name {
				found = true
				if d.Scope != "diagnostics:read" {
					t.Fatalf("%s scope = %s, want diagnostics:read", d.Name, d.Scope)
				}
				break
			}
		}
		if !found {
			t.Fatalf("missing tool %s", name)
		}
	}
}

func TestListToolsScopesFiltering(t *testing.T) {
	e := newEnv(t, nil)
	m, _ := e.token(t, "metrics:read")
	d, _ := e.token(t, "diagnostics:read")

	metricsTools := toolNames(t, e.mustConnect(t, m))
	diagTools := toolNames(t, e.mustConnect(t, d))

	// Metrics token should see get_overview, get_me, and metrics tools, not diagnostics tools
	metricsSet := map[string]bool{}
	for _, n := range metricsTools {
		metricsSet[n] = true
	}
	if !metricsSet["get_overview"] || !metricsSet["get_views"] || !metricsSet["get_growth"] || metricsSet["get_player_health"] {
		t.Fatalf("metrics token tools = %v", metricsTools)
	}

	// Diagnostics token should only see diagnostics tools, not metrics tools
	diagSet := map[string]bool{}
	for _, n := range diagTools {
		diagSet[n] = true
	}
	if !diagSet["get_player_health"] || !diagSet["list_issues"] || diagSet["get_overview"] {
		t.Fatalf("diagnostics token tools = %v, should not see metrics tools", diagTools)
	}
}

func TestReadToolCall_GetOverview(t *testing.T) {
	e := newEnv(t, nil)
	m, _ := e.token(t, "metrics:read")
	cs := e.mustConnect(t, m)
	text, isErr := call(t, cs, "get_overview", map[string]any{"period": 7})
	if isErr {
		t.Fatalf("error: %s", text)
	}
	var v map[string]any
	if err := json.Unmarshal([]byte(text), &v); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, text)
	}
}

func TestReadToolCall_GetViews(t *testing.T) {
	e := newEnv(t, nil)
	m, _ := e.token(t, "metrics:read")
	cs := e.mustConnect(t, m)
	text, isErr := call(t, cs, "get_views", map[string]any{"period": 7})
	if isErr {
		t.Fatalf("error: %s", text)
	}
	var v map[string]any
	if err := json.Unmarshal([]byte(text), &v); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, text)
	}
}

func TestReadToolCall_ListTopAnime(t *testing.T) {
	e := newEnv(t, nil)
	m, _ := e.token(t, "metrics:read")
	cs := e.mustConnect(t, m)
	text, isErr := call(t, cs, "list_top_anime", map[string]any{"period": 7, "format": "all", "limit": 10, "offset": 0})
	if isErr {
		t.Fatalf("error: %s", text)
	}
	var v map[string]any
	if err := json.Unmarshal([]byte(text), &v); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, text)
	}
}

func TestReadToolCall_GetUsersSummary(t *testing.T) {
	e := newEnv(t, nil)
	m, _ := e.token(t, "metrics:read")
	cs := e.mustConnect(t, m)
	text, isErr := call(t, cs, "get_users_summary", nil)
	if isErr {
		t.Fatalf("error: %s", text)
	}
	var v map[string]any
	if err := json.Unmarshal([]byte(text), &v); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, text)
	}
}

func TestReadToolCall_ListUsers(t *testing.T) {
	e := newEnv(t, nil)
	m, _ := e.token(t, "metrics:read")
	cs := e.mustConnect(t, m)
	// Token should not be able to search (q parameter)
	text, isErr := call(t, cs, "list_users", map[string]any{"limit": 10, "offset": 0})
	if isErr {
		t.Fatalf("error: %s", text)
	}
	var v map[string]any
	if err := json.Unmarshal([]byte(text), &v); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, text)
	}
}

func TestReadToolCall_GetGrowth(t *testing.T) {
	e := newEnv(t, nil)
	m, _ := e.token(t, "metrics:read")
	cs := e.mustConnect(t, m)
	text, isErr := call(t, cs, "get_growth", map[string]any{"period": 7})
	if isErr {
		t.Fatalf("error: %s", text)
	}
	var v map[string]any
	if err := json.Unmarshal([]byte(text), &v); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, text)
	}
}

func TestReadToolCall_GetRetention(t *testing.T) {
	e := newEnv(t, nil)
	m, _ := e.token(t, "metrics:read")
	cs := e.mustConnect(t, m)
	text, isErr := call(t, cs, "get_retention", map[string]any{"period": 7})
	if isErr {
		t.Fatalf("error: %s", text)
	}
	var v map[string]any
	if err := json.Unmarshal([]byte(text), &v); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, text)
	}
}

func TestReadToolCall_GetFunnel(t *testing.T) {
	e := newEnv(t, nil)
	m, _ := e.token(t, "metrics:read")
	cs := e.mustConnect(t, m)
	text, isErr := call(t, cs, "get_funnel", map[string]any{"period": 7})
	if isErr {
		t.Fatalf("error: %s", text)
	}
	var v map[string]any
	if err := json.Unmarshal([]byte(text), &v); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, text)
	}
}

func TestReadToolCall_GetCosts(t *testing.T) {
	e := newEnv(t, nil)
	m, _ := e.token(t, "metrics:read")
	cs := e.mustConnect(t, m)
	text, isErr := call(t, cs, "get_costs", map[string]any{"period": 7})
	if isErr {
		t.Fatalf("error: %s", text)
	}
	var v map[string]any
	if err := json.Unmarshal([]byte(text), &v); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, text)
	}
}

func TestReadToolCall_GetPlayerHealth(t *testing.T) {
	e := newEnv(t, nil)
	d, _ := e.token(t, "diagnostics:read")
	cs := e.mustConnect(t, d)
	text, isErr := call(t, cs, "get_player_health", map[string]any{"period": 7})
	if isErr {
		t.Fatalf("error: %s", text)
	}
	var v map[string]any
	if err := json.Unmarshal([]byte(text), &v); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, text)
	}
}

func TestReadToolCall_ListPlaybackErrors(t *testing.T) {
	e := newEnv(t, nil)
	d, _ := e.token(t, "diagnostics:read")
	cs := e.mustConnect(t, d)
	text, isErr := call(t, cs, "list_playback_errors", map[string]any{"limit": 10, "offset": 0})
	if isErr {
		t.Fatalf("error: %s", text)
	}
	var v map[string]any
	if err := json.Unmarshal([]byte(text), &v); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, text)
	}
}

func TestReadToolCall_GetErrorsSummary(t *testing.T) {
	e := newEnv(t, nil)
	d, _ := e.token(t, "diagnostics:read")
	cs := e.mustConnect(t, d)
	text, isErr := call(t, cs, "get_errors_summary", map[string]any{"period": 7})
	if isErr {
		t.Fatalf("error: %s", text)
	}
	var v map[string]any
	if err := json.Unmarshal([]byte(text), &v); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, text)
	}
}

func TestReadToolCall_GetSourcesHealth(t *testing.T) {
	e := newEnv(t, nil)
	d, _ := e.token(t, "diagnostics:read")
	cs := e.mustConnect(t, d)
	text, isErr := call(t, cs, "get_sources_health", map[string]any{"period": 7})
	if isErr {
		t.Fatalf("error: %s", text)
	}
	var v map[string]any
	if err := json.Unmarshal([]byte(text), &v); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, text)
	}
}

func TestReadToolCall_ListIssues(t *testing.T) {
	e := newEnv(t, nil)
	d, _ := e.token(t, "diagnostics:read")
	cs := e.mustConnect(t, d)
	text, isErr := call(t, cs, "list_issues", map[string]any{"limit": 10, "offset": 0})
	if isErr {
		t.Fatalf("error: %s", text)
	}
	var v map[string]any
	if err := json.Unmarshal([]byte(text), &v); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, text)
	}
}

func TestReadToolsNoPrivateKeys(t *testing.T) {
	e := newEnv(t, nil)
	m, _ := e.token(t, "metrics:read")
	cs := e.mustConnect(t, m)
	// Test several tools to ensure no pseudo/email leaks
	for _, toolCall := range []struct {
		name string
		args map[string]any
	}{
		{"get_overview", map[string]any{"period": 7}},
		{"get_users_summary", nil},
		{"list_users", map[string]any{"limit": 10, "offset": 0}},
	} {
		text, isErr := call(t, cs, toolCall.name, toolCall.args)
		if isErr {
			t.Fatalf("%s error: %s", toolCall.name, text)
		}
		low := strings.ToLower(text)
		if strings.Contains(low, `"pseudo"`) || strings.Contains(low, `"email"`) {
			t.Fatalf("%s leaked PII: %s", toolCall.name, text)
		}
	}
}
