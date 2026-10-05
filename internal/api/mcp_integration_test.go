package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gazes/gazes/internal/admin"
	"github.com/gazes/gazes/internal/auth"
	"github.com/gazes/gazes/internal/config"
)

// The MCP unit tests wire the admin router themselves. This one goes through the REAL server wiring
// (NewServer, WithAdmin, WithMCP, the main chi router in front of /mcp): that is where a tool call used
// to answer 405 for every read tool, because the inner router inherited the outer request's route context.
func TestMCPToolCallsThroughTheRealServer(t *testing.T) {
	dir := t.TempDir()
	as, err := auth.OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer as.Close()
	st, err := admin.Open(filepath.Join(dir, "admin.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	svc, err := admin.NewService(st, filepath.Join(dir, "accounts.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	tok, _, err := st.CreateToken(context.Background(), "t", []string{"metrics:read", "diagnostics:read", "ops:write"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	h := NewServer(&config.Config{}, nil, nil, nil, nil, WithAdmin(svc), WithMCP()).Router()

	rpc := func(sid, body string) (*httptest.ResponseRecorder, map[string]any) {
		req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+tok)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		if sid != "" {
			req.Header.Set("Mcp-Session-Id", sid)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		var out map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec, out
	}
	rec, _ := rpc("", `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`)
	sid := rec.Header().Get("Mcp-Session-Id")

	call := func(name, args string) (map[string]any, bool) {
		_, out := rpc(sid, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"`+name+`","arguments":`+args+`}}`)
		res, _ := out["result"].(map[string]any)
		isErr, _ := res["isError"].(bool)
		content, _ := res["content"].([]any)
		var text string
		if len(content) > 0 {
			text, _ = content[0].(map[string]any)["text"].(string)
		}
		var data map[string]any
		if err := json.Unmarshal([]byte(text), &data); err != nil && !isErr {
			t.Fatalf("%s: result is not JSON: %q", name, text)
		}
		if isErr {
			t.Errorf("%s answered an error: %s", name, text)
		}
		return data, isErr
	}

	if d, _ := call("get_me", `{}`); d["via"] != "token" {
		t.Errorf("get_me via = %v", d["via"])
	}
	if d, _ := call("get_overview", `{"period":7}`); d["kpis"] == nil {
		t.Errorf("get_overview has no kpis: %v", d)
	}
	if d, _ := call("get_watch_status", `{}`); len(d["rules"].([]any)) != 5 {
		t.Errorf("get_watch_status rules: %v", d["rules"])
	}
	call("list_issues", `{"limit":5}`)
	call("get_player_health", `{"period":7}`)
	call("get_users_summary", `{}`)
	// a write tool is a simulation by default and goes through the same path (POST)
	if d, _ := call("create_issue", `{"title":"integration","severity":"low"}`); d["status"] != "dry_run" {
		t.Errorf("create_issue must be a dry run by default: %v", d)
	}
	if d, _ := call("list_issues", `{"limit":5}`); len(d["items"].([]any)) != 0 {
		t.Errorf("a dry run must not create an issue: %v", d["items"])
	}
}
