package auth

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
)

func TestOpenStoreMigratesLegacyDatabase(t *testing.T) {
	dir := t.TempDir()
	// A pre-versioning database: old schema, user_version 0, with data.
	db, err := sql.Open("sqlite3", filepath.Join(dir, "accounts.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(baselineSchema); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO users(email_idx, email_enc, pseudo, pass_hash, created_at) VALUES('i','e','alice','h',1)`); err != nil {
		t.Fatal(err)
	}
	db.Close()

	s, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	u, err := s.UserByEmailIdx("i")
	if err != nil {
		t.Fatal(err)
	}
	if u.Pseudo != "alice" || u.Role != RoleUser {
		t.Fatalf("user = %+v", u)
	}
	var v int
	if err := s.db.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil || v != 8 {
		t.Fatalf("user_version = %d, %v", v, err)
	}
	for _, idx := range []string{"watch_sessions_started", "users_created"} {
		var n int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name=?`, idx).Scan(&n); err != nil || n != 1 {
			t.Fatalf("index %s missing: %d %v", idx, n, err)
		}
	}
}

func TestOpenStoreFreshAndReopen(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 2; i++ {
		s, err := OpenStore(dir)
		if err != nil {
			t.Fatalf("open #%d: %v", i, err)
		}
		s.Close()
	}
}

func TestOpenStoreRefusesNewerDatabase(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite3", filepath.Join(dir, "accounts.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`PRAGMA user_version = 99`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if s, err := OpenStore(dir); err == nil {
		s.Close()
		t.Fatal("expected error for newer database")
	}
}

func TestSetUserRoleAndCountAdmins(t *testing.T) {
	s, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for i, p := range []string{"alice", "bob", "dup", "dup"} {
		if _, err := s.db.Exec(`INSERT INTO users(email_idx, email_enc, pseudo, pass_hash, created_at) VALUES(?,?,?,?,1)`, string(rune('a'+i)), "e", p, "h"); err != nil {
			t.Fatal(err)
		}
	}
	ctx := context.Background()
	tests := []struct {
		name, pseudo, role string
		wantErr            error
		wantAdmins         int
	}{
		{"grant", "alice", "admin", nil, 1},
		{"grant again is idempotent", "alice", "admin", nil, 1},
		{"invalid role", "bob", "root", ErrInvalidRole, 1},
		{"empty role", "bob", "", ErrInvalidRole, 1},
		{"unknown user", "nobody", "admin", sql.ErrNoRows, 1},
		{"ambiguous pseudo", "dup", "admin", ErrAmbiguousPseudo, 1},
		{"revoke", "alice", "user", nil, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := s.SetUserRole(ctx, tc.pseudo, tc.role)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			n, err := s.CountAdmins(ctx)
			if err != nil || n != tc.wantAdmins {
				t.Fatalf("admins = %d, %v; want %d", n, err, tc.wantAdmins)
			}
		})
	}
}
