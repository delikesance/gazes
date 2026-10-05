package admin

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

func TestTokenCreationIsSessionOnly(t *testing.T) {
	e := opsEnv(t)
	body := `{"name":"claude-code","scopes":["metrics:read"]}`
	if got := e.do(t, "POST", "/tokens", body, nil).code; got != 401 {
		t.Errorf("no credentials: %d, want 401", got)
	}
	if got := e.do(t, "POST", "/tokens", body, e.as("all")).code; got != 403 {
		t.Errorf("a token with every scope must not mint a token: %d, want 403", got)
	}
	if got := e.do(t, "POST", "/tokens", body, pbAdminSession).code; got != 403 {
		t.Errorf("a session without the CSRF header: %d, want 403", got)
	}
	if got := e.do(t, "DELETE", "/tokens/1", "", e.as("all")).code; got != 403 {
		t.Errorf("a token must not revoke a token: %d, want 403", got)
	}
	if got := e.do(t, "DELETE", "/tokens/1", "", pbAdminSession).code; got != 403 {
		t.Errorf("revoke without the CSRF header: %d, want 403", got)
	}
}

func TestTokenCreateReturnsTheTokenOnceAndItWorks(t *testing.T) {
	e := opsEnv(t)
	ctx := context.Background()
	r := e.do(t, "POST", "/tokens", `{"name":" claude-code ","scopes":["diagnostics:read","metrics:read"],"ttl_hours":48}`, pbAdminCSRF)
	if r.code != 201 {
		t.Fatalf("%d %s", r.code, r.body)
	}
	plain, _ := r.data["token"].(string)
	if !strings.HasPrefix(plain, "gzs_") || r.data["name"] != "claude-code" {
		t.Fatalf("token/name = %q / %v", plain, r.data["name"])
	}
	if sc := r.data["scopes"].([]any); len(sc) != 2 {
		t.Fatalf("scopes = %v", sc)
	}
	tok, err := e.svc.store.VerifyToken(ctx, plain)
	if err != nil || !tok.HasScope(ScopeMetricsRead) || tok.HasScope(ScopeOpsWrite) {
		t.Fatalf("created token must carry exactly the requested scopes: %v %+v", err, tok)
	}
	if n := e.count(t, `SELECT COUNT(*) FROM admin_tokens WHERE token_hash = ?`, plain); n != 0 {
		t.Fatal("the plain token must never be stored")
	}
	if strings.Contains(string(e.do(t, "GET", "/tokens", "", pbAdminSession).body), plain) {
		t.Fatal("the token list must never contain the plain token")
	}
	if n := e.count(t, `SELECT COUNT(*) FROM mcp_audit WHERE tool = 'token:create' AND outcome = 'ok'`); n != 1 {
		t.Fatalf("token creation must be audited: %d", n)
	}
	if strings.Contains(e.auditText(t), plain) {
		t.Fatal("the audit log must not contain the token")
	}
	// the new token works on a route of its scope and nowhere else
	if got := e.do(t, "GET", "/me", "", func(req *http.Request) { req.Header.Set("Authorization", "Bearer "+plain) }).code; got != 200 {
		t.Errorf("GET /me with the new token: %d", got)
	}
	if got := e.do(t, "POST", "/tokens", `{"name":"x","scopes":["metrics:read"]}`, func(req *http.Request) { req.Header.Set("Authorization", "Bearer "+plain) }).code; got != 403 {
		t.Errorf("the new token must not mint tokens: %d", got)
	}
}

func TestTokenCreateValidation(t *testing.T) {
	e := opsEnv(t)
	long := strings.Repeat("a", 61)
	for name, body := range map[string]string{
		"no name":        `{"scopes":["metrics:read"]}`,
		"blank name":     `{"name":"  ","scopes":["metrics:read"]}`,
		"long name":      `{"name":"` + long + `","scopes":["metrics:read"]}`,
		"control char":   `{"name":"a\u0007b","scopes":["metrics:read"]}`,
		"no scopes":      `{"name":"x","scopes":[]}`,
		"unknown scope":  `{"name":"x","scopes":["root"]}`,
		"ttl zero":       `{"name":"x","scopes":["metrics:read"],"ttl_hours":0}`,
		"ttl too long":   `{"name":"x","scopes":["metrics:read"],"ttl_hours":8761}`,
		"unknown field":  `{"name":"x","scopes":["metrics:read"],"admin":true}`,
		"not json":       `nope`,
		"scopes as text": `{"name":"x","scopes":"metrics:read"}`,
	} {
		if got := e.do(t, "POST", "/tokens", body, pbAdminCSRF).code; got != 400 {
			t.Errorf("%s: %d, want 400", name, got)
		}
	}
	if got := e.do(t, "POST", "/tokens", `{"name":"x","scopes":["metrics:read"],"ttl_hours":8760}`, pbAdminCSRF).code; got != 201 {
		t.Errorf("a one-year token is allowed: %d", got)
	}
}

func TestTokenCreateIsCapped(t *testing.T) {
	e := opsEnv(t) // the env already holds 5 tokens
	ok := 0
	for i := 0; i < 30; i++ {
		r := e.do(t, "POST", "/tokens", `{"name":"t","scopes":["metrics:read"]}`, pbAdminCSRF)
		if r.code == 201 {
			ok++
			continue
		}
		if r.code != 409 {
			t.Fatalf("expected 409 once the cap is reached, got %d %s", r.code, r.body)
		}
		break
	}
	if want := tokenMaxActive - 5; ok != want {
		t.Fatalf("created %d tokens before the cap, want %d", ok, want)
	}
}

func TestTokenRevoke(t *testing.T) {
	e := opsEnv(t)
	ctx := context.Background()
	r := e.do(t, "POST", "/tokens", `{"name":"temp","scopes":["metrics:read"]}`, pbAdminCSRF)
	plain, id := r.data["token"].(string), int64(pbNum(t, r.data["id"]))
	if _, err := e.svc.store.VerifyToken(ctx, plain); err != nil {
		t.Fatal(err)
	}
	if got := e.do(t, "DELETE", "/tokens/"+strconv.FormatInt(id, 10), "", pbAdminCSRF).code; got != 204 {
		t.Fatalf("revoke: %d", got)
	}
	if _, err := e.svc.store.VerifyToken(ctx, plain); err == nil {
		t.Fatal("a revoked token must stop working at once")
	}
	if got := e.do(t, "DELETE", "/tokens/"+strconv.FormatInt(id, 10), "", pbAdminCSRF).code; got != 204 {
		t.Errorf("revoking twice is a no-op: %d", got)
	}
	for _, bad := range []string{"/tokens/999999", "/tokens/abc", "/tokens/0", "/tokens/-1"} {
		if got := e.do(t, "DELETE", bad, "", pbAdminCSRF).code; got != 404 {
			t.Errorf("DELETE %s: %d, want 404", bad, got)
		}
	}
	if n := e.count(t, `SELECT COUNT(*) FROM mcp_audit WHERE tool = 'token:revoke'`); n != 2 {
		t.Fatalf("revocations must be audited: %d", n)
	}
}

func (e *pbEnv) auditText(t *testing.T) string {
	t.Helper()
	rows, err := e.svc.adminDB().Query(`SELECT tool || ' ' || COALESCE(args_summary,'') FROM mcp_audit`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var b strings.Builder
	for rows.Next() {
		var s string
		_ = rows.Scan(&s)
		b.WriteString(s + "\n")
	}
	return b.String()
}
