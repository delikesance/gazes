package main

import (
	"bytes"
	"context"
	"database/sql"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gazes/gazes/internal/admin"
	"github.com/gazes/gazes/internal/auth"
)

type fakeRoles struct{ roles map[string]string }

func (f *fakeRoles) SetUserRole(_ context.Context, p, r string) error {
	if _, ok := f.roles[p]; !ok {
		return sql.ErrNoRows
	}
	f.roles[p] = r
	return nil
}
func (f *fakeRoles) RoleByPseudo(_ context.Context, p string) (string, error) {
	r, ok := f.roles[p]
	if !ok {
		return "", sql.ErrNoRows
	}
	return r, nil
}
func (f *fakeRoles) SetUserRoleByID(context.Context, int64, string) error { return sql.ErrNoRows }
func (f *fakeRoles) UserRole(context.Context, int64) (string, error)      { return "", sql.ErrNoRows }
func (f *fakeRoles) CountAdmins(context.Context) (int, error) {
	n := 0
	for _, r := range f.roles {
		if r == auth.RoleAdmin {
			n++
		}
	}
	return n, nil
}

func TestGrantRevoke(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
		want    map[string]string
	}{
		{"grant", []string{"grant", "bob"}, "", map[string]string{"alice": "admin", "bob": "admin"}},
		{"grant unknown", []string{"grant", "zed"}, "no user", nil},
		{"revoke non-last", []string{"revoke", "alice"}, "", map[string]string{"alice": "user", "bob": "user"}},
		{"revoke flag first", []string{"revoke", "--force", "alice"}, "", map[string]string{"alice": "user", "bob": "user"}},
		{"revoke plain user", []string{"revoke", "bob"}, "", map[string]string{"alice": "admin", "bob": "user"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeRoles{roles: map[string]string{"alice": "admin", "bob": "user"}}
			if tc.name == "revoke non-last" {
				f.roles["bob"] = "admin"
				tc.want["bob"] = "admin"
			}
			var out, errb bytes.Buffer
			err := run(context.Background(), tc.args, &out, &errb, f, nil)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			for p, r := range tc.want {
				if f.roles[p] != r {
					t.Errorf("%s = %s, want %s", p, f.roles[p], r)
				}
			}
		})
	}
}

func TestRevokeLastAdminNeedsForce(t *testing.T) {
	f := &fakeRoles{roles: map[string]string{"alice": "admin"}}
	var out, errb bytes.Buffer
	if err := run(context.Background(), []string{"revoke", "alice"}, &out, &errb, f, nil); err == nil || f.roles["alice"] != "admin" {
		t.Fatalf("last admin removed: %v", err)
	}
	if err := run(context.Background(), []string{"revoke", "alice", "--force"}, &out, &errb, f, nil); err != nil || f.roles["alice"] != "user" {
		t.Fatalf("force failed: %v", err)
	}
}

func TestTokenCommands(t *testing.T) {
	st, err := admin.Open(filepath.Join(t.TempDir(), "admin.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	var out, errb bytes.Buffer
	if err := run(ctx, []string{"token", "create", "--name", "ci", "--scopes", "metrics:read,ops:write", "--ttl", "720h"}, &out, &errb, nil, st); err != nil {
		t.Fatal(err)
	}
	plain := strings.TrimSpace(out.String())
	if !strings.HasPrefix(plain, "gzs_") || strings.Contains(plain, "\n") {
		t.Fatalf("stdout should be only the token: %q", out.String())
	}
	if _, err := st.VerifyToken(ctx, plain); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := run(ctx, []string{"token", "list"}, &out, &errb, nil, st); err != nil || strings.Contains(out.String(), plain) || !strings.Contains(out.String(), "ci") {
		t.Fatalf("list: %v %q", err, out.String())
	}
	if err := run(ctx, []string{"token", "create", "--name", "x", "--scopes", "bogus"}, &out, &errb, nil, st); err == nil {
		t.Fatal("bad scope accepted")
	}
	if err := run(ctx, []string{"token", "create", "--name", "x", "--scopes", "metrics:read", "--ttl", "9000h"}, &out, &errb, nil, st); err == nil {
		t.Fatal("ttl above max accepted")
	}
	if err := run(ctx, []string{"token", "revoke", "1"}, &out, &errb, nil, st); err != nil {
		t.Fatal(err)
	}
	if _, err := st.VerifyToken(ctx, plain); err == nil {
		t.Fatal("revoked token still valid")
	}
}

func TestRoleByAccountIDWhenPseudosAreShared(t *testing.T) {
	st, err := auth.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	now := time.Now()
	var ids []int64
	for i, mail := range []string{"a", "b", "c"} {
		pseudo := "twin"
		if i == 2 {
			pseudo = "solo"
		}
		id, err := st.CreateUser(auth.User{EmailIdx: mail, EmailEnc: mail, Pseudo: pseudo, PassHash: "x"}, now)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	var out bytes.Buffer
	if err := grant(ctx, &out, st, "twin"); err == nil || !strings.Contains(err.Error(), "matches several users") {
		t.Fatalf("an ambiguous pseudo must be refused, got %v", err)
	}
	target := "#" + strconv.FormatInt(ids[1], 10)
	if err := grant(ctx, &out, st, target); err != nil {
		t.Fatal(err)
	}
	if r, _ := st.UserRole(ctx, ids[1]); r != auth.RoleAdmin {
		t.Fatalf("role of %s = %q", target, r)
	}
	if r, _ := st.UserRole(ctx, ids[0]); r != auth.RoleUser {
		t.Fatalf("the homonym must stay a user, got %q", r)
	}
	if err := revokeAdmin(ctx, &out, st, target, false); err == nil || !strings.Contains(err.Error(), "last admin") {
		t.Fatalf("the last admin must be protected, got %v", err)
	}
	if err := revokeAdmin(ctx, &out, st, target, true); err != nil {
		t.Fatal(err)
	}
	if err := grant(ctx, &out, st, "#999999"); err == nil || !strings.Contains(err.Error(), "no user") {
		t.Fatalf("unknown id: %v", err)
	}
	for _, bad := range []string{"#", "#0", "#-1", "#1x", "# 1", "#12345678901234567"} {
		if _, ok := parseID(bad); ok {
			t.Errorf("parseID(%q) must be rejected", bad)
		}
	}
	if id, ok := parseID("#42"); !ok || id != 42 {
		t.Fatalf("parseID(#42) = %d %v", id, ok)
	}
}
