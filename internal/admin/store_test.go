package admin

import (
	"path/filepath"
	"testing"
)

func TestOpenTwiceKeepsDataAndSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "admin.sqlite")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO issues(id, created_at, updated_at, severity, title) VALUES('i1', 1, 1, 'low', 't')`); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	var status string
	if err := s.db.QueryRow(`SELECT status FROM issues WHERE id='i1'`).Scan(&status); err != nil || status != "new" {
		t.Fatalf("issue lost or default wrong: %q %v", status, err)
	}
	var v int
	if err := s.db.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil || v != len(migrations) {
		t.Fatalf("user_version = %d, %v", v, err)
	}
	tables := []string{"admin_tokens", "metrics_daily", "metrics_hourly", "metrics_anime_daily", "playback_errors", "issues", "mcp_audit", "approvals", "settings", "action_idempotency"}
	for _, tb := range tables {
		var n int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, tb).Scan(&n); err != nil || n != 1 {
			t.Errorf("table %s missing (%v)", tb, err)
		}
	}
	for _, ix := range []string{"playback_errors_ts", "playback_errors_code_ts"} {
		var n int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name=?`, ix).Scan(&n); err != nil || n != 1 {
			t.Errorf("index %s missing (%v)", ix, err)
		}
	}
}

func TestAdminTokenHashUnique(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "admin.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ins := `INSERT INTO admin_tokens(name, token_hash, scopes, created_at, expires_at) VALUES('n','h','metrics:read',1,2)`
	if _, err := s.db.Exec(ins); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(ins); err == nil {
		t.Fatal("duplicate token_hash accepted")
	}
}
