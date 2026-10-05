package admin

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeRoles map[int64]string

func (f fakeRoles) SetUserRoleByID(_ context.Context, id int64, role string) error {
	if _, ok := f[id]; !ok {
		return sql.ErrNoRows
	}
	f[id] = role
	return nil
}

func TestGrantAdmin(t *testing.T) {
	e := newUsersEnv(t, "2026-03-10T12:00:00Z")
	roles := fakeRoles{2: "user"}
	e.svc.SetRoleSetter(roles)

	post := func(path, body string, mut func(*http.Request)) int {
		req := httptest.NewRequest("POST", path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if mut != nil {
			mut(req)
		}
		rec := httptest.NewRecorder()
		e.h.ServeHTTP(rec, req)
		return rec.Code
	}
	csrf := func(r *http.Request) { asAdmin(r); r.Header.Set("X-Gazes-Admin", "1") }

	cases := []struct {
		name, path, body string
		mut              func(*http.Request)
		want             int
		role             string
	}{
		{"anonymous", "/api/v1/admin/users/2/role", `{"role":"admin"}`, nil, 401, "user"},
		{"no csrf header", "/api/v1/admin/users/2/role", `{"role":"admin"}`, asAdmin, 403, "user"},
		{"token refused", "/api/v1/admin/users/2/role", `{"role":"admin"}`, e.asToken, 403, "user"},
		{"bad role", "/api/v1/admin/users/2/role", `{"role":"root"}`, csrf, 400, "user"},
		{"demotion not exposed", "/api/v1/admin/users/2/role", `{"role":"user"}`, csrf, 400, "user"},
		{"unknown user", "/api/v1/admin/users/9/role", `{"role":"admin"}`, csrf, 404, "user"},
		{"ok", "/api/v1/admin/users/2/role", `{"role":"admin"}`, csrf, 200, "admin"},
		{"idempotent", "/api/v1/admin/users/2/role", `{"role":"admin"}`, csrf, 200, "admin"},
	}
	for _, tc := range cases {
		if got := post(tc.path, tc.body, tc.mut); got != tc.want {
			t.Errorf("%s: status %d, want %d", tc.name, got, tc.want)
		}
		if roles[2] != tc.role {
			t.Errorf("%s: role %q, want %q", tc.name, roles[2], tc.role)
		}
	}
}
