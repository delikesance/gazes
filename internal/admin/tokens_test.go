package admin

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newTokenStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "admin.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestCreateTokenValidation(t *testing.T) {
	s := newTokenStore(t)
	ctx := context.Background()
	tests := []struct {
		name   string
		tname  string
		scopes []string
		ttl    time.Duration
		ok     bool
	}{
		{"default ttl", "n", []string{ScopeMetricsRead}, 0, true},
		{"max ttl", "n", []string{ScopeMetricsRead}, MaxTokenTTL, true},
		{"above max ttl", "n", []string{ScopeMetricsRead}, MaxTokenTTL + time.Second, false},
		{"negative ttl", "n", []string{ScopeMetricsRead}, -time.Hour, false},
		{"unknown scope", "n", []string{"root"}, 0, false},
		{"no scope", "n", nil, 0, false},
		{"empty name", " ", []string{ScopeMetricsRead}, 0, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			plain, id, err := s.CreateToken(ctx, tc.tname, tc.scopes, tc.ttl)
			if (err == nil) != tc.ok {
				t.Fatalf("err = %v, want ok=%v", err, tc.ok)
			}
			if tc.ok && (!strings.HasPrefix(plain, "gzs_") || id == 0 || len(plain) != 4+43) {
				t.Fatalf("bad token %q id %d", plain, id)
			}
		})
	}
}

func TestVerifyToken(t *testing.T) {
	s := newTokenStore(t)
	ctx := context.Background()
	plain, id, err := s.CreateToken(ctx, "ci", []string{ScopeOpsWrite, ScopeMetricsRead, ScopeMetricsRead}, 0)
	if err != nil {
		t.Fatal(err)
	}
	expired, eid, _ := s.CreateToken(ctx, "old", []string{ScopeMetricsRead}, time.Hour)
	if _, err := s.db.Exec(`UPDATE admin_tokens SET expires_at = ? WHERE id = ?`, time.Now().Add(-time.Second).Unix(), eid); err != nil {
		t.Fatal(err)
	}
	revoked, rid, _ := s.CreateToken(ctx, "rev", []string{ScopeMetricsRead}, 0)
	if err := s.RevokeToken(ctx, rid); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name  string
		plain string
		ok    bool
	}{
		{"valid", plain, true},
		{"tampered last char", plain[:len(plain)-1] + flip(plain[len(plain)-1]), false},
		{"no prefix", strings.TrimPrefix(plain, "gzs_"), false},
		{"empty", "", false},
		{"unknown", "gzs_" + strings.Repeat("A", 43), false},
		{"expired", expired, false},
		{"revoked", revoked, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tok, err := s.VerifyToken(ctx, tc.plain)
			if tc.ok {
				if err != nil || tok.ID != id || tok.Name != "ci" {
					t.Fatalf("got %+v, %v", tok, err)
				}
				if got := strings.Join(tok.Scopes, ","); got != "metrics:read,ops:write" {
					t.Fatalf("scopes = %s", got)
				}
				if !tok.HasScope(ScopeOpsWrite) || tok.HasScope(ScopeConfigWrite) {
					t.Fatal("HasScope wrong")
				}
				return
			}
			if !errors.Is(err, ErrInvalidToken) {
				t.Fatalf("err = %v, want ErrInvalidToken", err)
			}
		})
	}
}

func flip(c byte) string {
	if c == 'A' {
		return "B"
	}
	return "A"
}

func TestTokenNeverStoredInClearAndListHasNoSecret(t *testing.T) {
	s := newTokenStore(t)
	ctx := context.Background()
	plain, id, err := s.CreateToken(ctx, "n", []string{ScopeMetricsRead}, 0)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := s.db.Query(`SELECT name, token_hash, scopes FROM admin_tokens`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var a, b, c string
		rows.Scan(&a, &b, &c)
		for _, v := range []string{a, b, c} {
			if strings.Contains(v, plain) || strings.Contains(v, strings.TrimPrefix(plain, "gzs_")) {
				t.Fatalf("clear token found in database: %q", v)
			}
		}
		if b != hashToken(plain) || len(b) != 64 {
			t.Fatalf("token_hash = %q", b)
		}
	}
	list, err := s.ListTokens(ctx)
	if err != nil || len(list) != 1 || list[0].ID != id || list[0].Status(time.Now()) != "active" {
		t.Fatalf("list = %+v, %v", list, err)
	}
	if err := s.RevokeToken(ctx, id); err != nil {
		t.Fatal(err)
	}
	list, _ = s.ListTokens(ctx)
	if list[0].Status(time.Now()) != "revoked" {
		t.Fatal("not revoked")
	}
	if err := s.RevokeToken(ctx, 999); err == nil {
		t.Fatal("unknown id should fail")
	}
}

func TestVerifyTokenUpdatesLastUsed(t *testing.T) {
	s := newTokenStore(t)
	ctx := context.Background()
	plain, id, _ := s.CreateToken(ctx, "n", []string{ScopeMetricsRead}, 0)
	if _, err := s.VerifyToken(ctx, plain); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var v *int64
		if s.db.QueryRow(`SELECT last_used_at FROM admin_tokens WHERE id=?`, id).Scan(&v); v != nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("last_used_at never updated")
}
