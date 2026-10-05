package mcp

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gazes/gazes/internal/admin"
	"github.com/gazes/gazes/internal/auth"
	"github.com/go-chi/chi/v5"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type fakeUsers struct{}

func (fakeUsers) UserRole(context.Context, int64) (string, error) { return "admin", nil }

type env struct {
	store  *admin.Store
	router http.Handler
	db     *sql.DB // second handle on admin.sqlite, to inspect and age rows
	server *Server
	ts     *httptest.Server
}

func newEnv(t *testing.T, tools []ToolDef, origins ...string) *env {
	t.Helper()
	dir := t.TempDir()
	as, err := auth.OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { as.Close() })
	adminPath := filepath.Join(dir, "admin.sqlite")
	st, err := admin.Open(adminPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	svc, err := admin.NewService(st, filepath.Join(dir, "accounts.sqlite"),
		admin.WithSessionAuth(func(r *http.Request) (int64, bool) { _, err := r.Cookie("sid"); return 1, err == nil }, fakeUsers{}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { svc.Close() })
	inner := chi.NewRouter()
	svc.Mount(inner)
	db, err := sql.Open("sqlite3", adminPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	srv := New(Config{Admin: inner, Tokens: st, Audit: st, Tools: tools, AllowedOrigins: origins}, nil)
	mux := http.NewServeMux()
	mux.Handle("/mcp", srv.HTTPHandler())
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return &env{store: st, router: inner, db: db, server: srv, ts: ts}
}

func (e *env) token(t *testing.T, scopes ...string) (string, int64) {
	t.Helper()
	plain, id, err := e.store.CreateToken(context.Background(), "t", scopes, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return plain, id
}

type bearerRT struct{ token string }

func (b bearerRT) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	if b.token != "" {
		r.Header.Set("Authorization", "Bearer "+b.token)
	}
	return http.DefaultTransport.RoundTrip(r)
}

func (e *env) connect(t *testing.T, token string) (*sdk.ClientSession, error) {
	t.Helper()
	c := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "1"}, nil)
	return c.Connect(context.Background(), &sdk.StreamableClientTransport{
		Endpoint: e.ts.URL + "/mcp", HTTPClient: &http.Client{Transport: bearerRT{token}}, DisableStandaloneSSE: true, MaxRetries: -1,
	}, nil)
}

func (e *env) mustConnect(t *testing.T, token string) *sdk.ClientSession {
	t.Helper()
	cs, err := e.connect(t, token)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs
}

func toolNames(t *testing.T, cs *sdk.ClientSession) []string {
	t.Helper()
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var n []string
	for _, tl := range res.Tools {
		n = append(n, tl.Name)
	}
	return n
}

func call(t *testing.T, cs *sdk.ClientSession, name string, args map[string]any) (string, bool) {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &sdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("call %s: protocol error %v", name, err)
	}
	var sb strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*sdk.TextContent); ok {
			sb.WriteString(tc.Text)
		}
	}
	return sb.String(), res.IsError
}

type auditRow struct {
	tool, outcome, args string
	token               sql.NullInt64
}

func (e *env) audit(t *testing.T) []auditRow {
	t.Helper()
	rows, err := e.db.Query(`SELECT tool, outcome, COALESCE(args_summary,''), token_id FROM mcp_audit ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []auditRow
	for rows.Next() {
		var r auditRow
		if err := rows.Scan(&r.tool, &r.outcome, &r.args, &r.token); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	return out
}

func has(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

func TestListToolsFollowsScopes(t *testing.T) {
	e := newEnv(t, nil)
	m, _ := e.token(t, "metrics:read")
	d, _ := e.token(t, "diagnostics:read")
	if n := toolNames(t, e.mustConnect(t, m)); !has(n, "get_overview") || !has(n, "get_me") {
		t.Fatalf("metrics token tools = %v", n)
	}
	n := toolNames(t, e.mustConnect(t, d))
	for _, metricsTool := range []string{"get_overview", "get_me", "get_views", "get_growth", "get_costs", "list_users"} {
		if has(n, metricsTool) {
			t.Fatalf("diagnostics-only token must not see %s, got %v", metricsTool, n)
		}
	}
	for _, diagTool := range []string{"get_player_health", "list_playback_errors", "list_issues"} {
		if !has(n, diagTool) {
			t.Fatalf("diagnostics token should see %s, got %v", diagTool, n)
		}
	}
}

func TestCallOKAndPrivacy(t *testing.T) {
	e := newEnv(t, nil)
	m, id := e.token(t, "metrics:read")
	cs := e.mustConnect(t, m)
	text, isErr := call(t, cs, "get_overview", map[string]any{"period": 7})
	if isErr {
		t.Fatalf("error result: %s", text)
	}
	var v map[string]any
	if err := json.Unmarshal([]byte(text), &v); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, text)
	}
	low := strings.ToLower(text)
	if strings.Contains(low, `"pseudo"`) || strings.Contains(low, `"email"`) {
		t.Fatalf("PII key in result: %s", text)
	}
	me, _ := call(t, cs, "get_me", nil)
	if !strings.Contains(me, `"via":"token"`) {
		t.Fatalf("get_me = %s", me)
	}
	rows := e.audit(t)
	if len(rows) != 2 || rows[0].tool != "get_overview" || rows[0].outcome != "ok" || rows[0].token.Int64 != id || rows[0].args != "period=7" {
		t.Fatalf("audit = %+v", rows)
	}
}

func TestDeniedAndErrorAudited(t *testing.T) {
	raw := ToolDef{Name: "overview_raw", Level: LevelRead, Scope: "metrics:read", Method: "GET", Path: "/overview",
		Description: "no enum", Params: []Param{{Name: "period", Type: "string"}, {Name: "api_key", Type: "string", Sensitive: true}}}
	diag := ToolDef{Name: "needs_diag", Level: LevelDiagnostic, Scope: "diagnostics:read", Method: "GET", Path: "/overview", Description: "x"}
	e := newEnv(t, []ToolDef{raw, diag})
	m, _ := e.token(t, "metrics:read")
	cs := e.mustConnect(t, m)

	text, isErr := call(t, cs, "overview_raw", map[string]any{"period": "14", "api_key": "s3cret"})
	if !isErr || !strings.Contains(text, "400") || !strings.Contains(text, "bad_period") || strings.Contains(text, "goroutine") {
		t.Fatalf("want readable 400 error, got %v %q", isErr, text)
	}
	// Absent from tools/list, refused when called anyway.
	if has(toolNames(t, cs), "needs_diag") {
		t.Fatal("needs_diag listed without scope")
	}
	text, isErr = call(t, cs, "needs_diag", nil)
	if !isErr || !strings.Contains(text, "denied") {
		t.Fatalf("want denied, got %v %q", isErr, text)
	}
	rows := e.audit(t)
	if len(rows) < 2 || rows[0].outcome != "error" || rows[1].outcome != "denied" {
		t.Fatalf("audit = %+v", rows)
	}
	if strings.Contains(rows[0].args, "s3cret") {
		t.Fatalf("sensitive value audited: %q", rows[0].args)
	}
}

func TestBadTokensRefused(t *testing.T) {
	e := newEnv(t, nil)
	if _, err := e.connect(t, ""); err == nil {
		t.Fatal("missing token accepted")
	}
	if _, err := e.connect(t, "gzs_nope"); err == nil {
		t.Fatal("invalid token accepted")
	}
	exp, id := e.token(t, "metrics:read")
	if _, err := e.db.Exec(`UPDATE admin_tokens SET expires_at = ? WHERE id = ?`, time.Now().Add(-time.Minute).Unix(), id); err != nil {
		t.Fatal(err)
	}
	if _, err := e.connect(t, exp); err == nil {
		t.Fatal("expired token accepted")
	}
	rev, rid := e.token(t, "metrics:read")
	if err := e.store.RevokeToken(context.Background(), rid); err != nil {
		t.Fatal(err)
	}
	if _, err := e.connect(t, rev); err == nil {
		t.Fatal("revoked token accepted")
	}
}

func rawInit(t *testing.T, url string, h map[string]string) *http.Response {
	t.Helper()
	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"x","version":"1"}}}`
	req, _ := http.NewRequest("POST", url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	for k, v := range h {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func TestSessionCookieNeverAccepted(t *testing.T) {
	e := newEnv(t, nil)
	resp := rawInit(t, e.ts.URL+"/mcp", map[string]string{"Cookie": "sid=1"})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("cookie-only status = %d, want 401", resp.StatusCode)
	}
}

func TestOrigin(t *testing.T) {
	e := newEnv(t, nil, "https://claude.ai")
	m, _ := e.token(t, "metrics:read")
	auth := "Bearer " + m
	if r := rawInit(t, e.ts.URL+"/mcp", map[string]string{"Authorization": auth, "Origin": "https://evil.example"}); r.StatusCode != http.StatusForbidden {
		t.Fatalf("bad origin status = %d", r.StatusCode)
	}
	if r := rawInit(t, e.ts.URL+"/mcp", map[string]string{"Authorization": auth, "Origin": "https://claude.ai"}); r.StatusCode != http.StatusOK {
		t.Fatalf("allowed origin status = %d", r.StatusCode)
	}
	if r := rawInit(t, e.ts.URL+"/mcp", map[string]string{"Authorization": auth}); r.StatusCode != http.StatusOK {
		t.Fatalf("no origin status = %d", r.StatusCode)
	}
	// Default: empty allow-list refuses every Origin.
	e2 := newEnv(t, nil)
	m2, _ := e2.token(t, "metrics:read")
	if r := rawInit(t, e2.ts.URL+"/mcp", map[string]string{"Authorization": "Bearer " + m2, "Origin": "https://claude.ai"}); r.StatusCode != http.StatusForbidden {
		t.Fatalf("default origin status = %d", r.StatusCode)
	}
}

// bigAdmin answers every admin request with a huge data payload.
type bigAdmin struct{ n int }

func (b bigAdmin) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"blob": strings.Repeat("é", b.n)}})
}

func TestTruncationAndStaticCredential(t *testing.T) {
	srv := New(Config{Admin: bigAdmin{200_000}, Tokens: nil}, &Credential{TokenID: 3, Plain: "gzs_x", Scopes: []string{"metrics:read"}})
	ct, st := sdk.NewInMemoryTransports()
	ss, err := srv.SDK().Connect(context.Background(), st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	cs, err := sdk.NewClient(&sdk.Implementation{Name: "t", Version: "1"}, nil).Connect(context.Background(), ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	text, isErr := call(t, cs, "get_me", nil)
	if isErr {
		t.Fatalf("error: %.200s", text)
	}
	if len(text) > MaxResultBytes {
		t.Fatalf("result %d bytes > %d", len(text), MaxResultBytes)
	}
	var v struct {
		Truncated bool   `json:"truncated"`
		Total     int    `json:"total_bytes"`
		Preview   string `json:"preview"`
	}
	if err := json.Unmarshal([]byte(text), &v); err != nil || !v.Truncated || v.Total < 200_000 || v.Preview == "" {
		t.Fatalf("bad truncated payload: %v %+v", err, v)
	}
}

func TestSmallResultNotTruncated(t *testing.T) {
	if s := boundJSON(json.RawMessage(`{ "a": 1 }`)); s != `{"a":1}` {
		t.Fatalf("got %s", s)
	}
}

func TestArgsSummaryBounded(t *testing.T) {
	d := ToolDef{}
	s := argsSummary(d, map[string]any{"q": strings.Repeat("x", 500), "password": "p", "email": "a@b"})
	if len([]rune(s)) > MaxArgsSummary || strings.Contains(s, "a@b") || strings.Contains(s, "=p") {
		t.Fatalf("summary %q", s)
	}
}

type failingAudit struct{}

func (failingAudit) RecordMCPAudit(context.Context, admin.MCPAuditEntry) error {
	return context.DeadlineExceeded
}

func TestAuditFailureDoesNotFailCall(t *testing.T) {
	srv := New(Config{Admin: bigAdmin{10}, Audit: failingAudit{}}, &Credential{TokenID: 1, Plain: "gzs_x", Scopes: []string{"metrics:read"}})
	ct, st := sdk.NewInMemoryTransports()
	ss, _ := srv.SDK().Connect(context.Background(), st, nil)
	defer ss.Close()
	cs, err := sdk.NewClient(&sdk.Implementation{Name: "t", Version: "1"}, nil).Connect(context.Background(), ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	if text, isErr := call(t, cs, "get_me", nil); isErr {
		t.Fatalf("audit failure broke the call: %s", text)
	}
}
