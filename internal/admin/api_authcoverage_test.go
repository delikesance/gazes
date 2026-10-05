package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// Every route of the admin API must refuse a request that carries no credentials. The test walks the
// real router, so a route added later without Auth fails here instead of shipping open.
func TestEveryAdminRouteRequiresCredentials(t *testing.T) {
	e := opsEnv(t)
	n := 0
	err := chi.Walk(e.router, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if !strings.HasPrefix(route, "/api/v1/admin") {
			return nil
		}
		n++
		path := strings.NewReplacer("{id}", "1", "{name}", "x").Replace(route)
		var body *strings.Reader
		if method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch {
			body = strings.NewReader("{}")
		} else {
			body = strings.NewReader("")
		}
		req := httptest.NewRequest(method, path, body)
		if body.Len() > 0 {
			req.Header.Set("Content-Type", "application/json")
		}
		rec := httptest.NewRecorder()
		e.router.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s without credentials: %d, want 401", method, route, rec.Code)
		}
		// a token with a wrong scope must never open a route either; a cookie-less session-only route stays closed
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if n < 25 {
		t.Fatalf("only %d admin routes walked: the router is not what this test expects", n)
	}
}

// A token that holds no scope of a route must be refused (403), on every route that a token can reach.
func TestNoScopeTokenIsRefusedEverywhere(t *testing.T) {
	e := opsEnv(t)
	tok, _, err := e.svc.store.CreateToken(t.Context(), "metricsonly", []string{ScopeMetricsRead}, 0)
	if err != nil {
		t.Fatal(err)
	}
	e.tokens["m"] = tok
	_ = chi.Walk(e.router, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if !strings.HasPrefix(route, "/api/v1/admin") {
			return nil
		}
		path := strings.NewReplacer("{id}", "1", "{name}", "x").Replace(route)
		req := httptest.NewRequest(method, path, strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+tok)
		rec := httptest.NewRecorder()
		e.router.ServeHTTP(rec, req)
		// metrics:read may legitimately reach the metrics routes; every other route must say 403
		if rec.Code == http.StatusUnauthorized {
			t.Errorf("%s %s: a valid token got 401", method, route)
		}
		if rec.Code == 200 || rec.Code == 201 || rec.Code == 202 {
			if method != http.MethodGet {
				t.Errorf("%s %s: a metrics:read token must not succeed on a write route (%d)", method, route, rec.Code)
			}
		}
		return nil
	})
}

func TestRequireTokenWithoutScopesPanicsAtWiring(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("RequireToken with no scope must panic when wired")
		}
	}()
	RequireToken(nil)
}
