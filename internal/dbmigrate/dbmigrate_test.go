package dbmigrate

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func openDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "t.sqlite")+"?_busy_timeout=5000")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func version(t *testing.T, db *sql.DB) int {
	t.Helper()
	var v int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestApply(t *testing.T) {
	good := []Migration{
		{1, "a", SQL(`CREATE TABLE a(x INTEGER)`)},
		{2, "b", SQL(`CREATE TABLE b(x INTEGER)`)},
	}
	tests := []struct {
		name    string
		ms      []Migration
		want    int
		wantErr string
	}{
		{"empty", nil, 0, ""},
		{"ok", good, 2, ""},
		{"duplicate version", []Migration{good[0], good[0]}, 0, "strictly greater"},
		{"decreasing", []Migration{good[1], good[0]}, 0, "strictly greater"},
		{"zero version", []Migration{{0, "z", SQL(`SELECT 1`)}}, 0, "strictly greater"},
		{"nil Up", []Migration{{1, "n", nil}}, 0, "no Up"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db := openDB(t)
			err := Apply(context.Background(), db, tc.ms)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := version(t, db); got != tc.want {
				t.Fatalf("version = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestApplyIdempotent(t *testing.T) {
	db := openDB(t)
	calls := 0
	ms := []Migration{{1, "a", func(tx *sql.Tx) error {
		calls++
		_, err := tx.Exec(`CREATE TABLE a(x INTEGER)`)
		return err
	}}}
	for i := 0; i < 3; i++ {
		if err := Apply(context.Background(), db, ms); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 1 {
		t.Fatalf("Up ran %d times, want 1", calls)
	}
}

func TestApplyRollsBackFailedMigration(t *testing.T) {
	db := openDB(t)
	boom := errors.New("boom")
	ms := []Migration{
		{1, "a", SQL(`CREATE TABLE a(x INTEGER)`)},
		{2, "bad", func(tx *sql.Tx) error {
			if _, err := tx.Exec(`CREATE TABLE b(x INTEGER)`); err != nil {
				return err
			}
			return boom
		}},
	}
	err := Apply(context.Background(), db, ms)
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
	if v := version(t, db); v != 1 {
		t.Fatalf("version = %d, want 1", v)
	}
	if _, err := db.Exec(`SELECT 1 FROM b`); err == nil {
		t.Fatal("table b should have been rolled back")
	}
}

func TestApplyNewerDatabase(t *testing.T) {
	db := openDB(t)
	if _, err := db.Exec(`PRAGMA user_version = 5`); err != nil {
		t.Fatal(err)
	}
	err := Apply(context.Background(), db, []Migration{{1, "a", SQL(`CREATE TABLE a(x INTEGER)`)}})
	if err == nil || !strings.Contains(err.Error(), "newer") {
		t.Fatalf("err = %v, want newer-database error", err)
	}
	if v := version(t, db); v != 5 {
		t.Fatalf("version changed to %d", v)
	}
}
