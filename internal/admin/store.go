// Package admin holds the admin panel's own storage: a SQLite database
// (admin.sqlite) kept separate from the accounts database. Times are unix
// seconds; days are YYYY-MM-DD in UTC.
package admin

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"

	"github.com/gazes/gazes/internal/dbmigrate"
	_ "github.com/mattn/go-sqlite3"
)

// Store wraps the admin database.
type Store struct{ db *sql.DB }

const baselineSchema = `
CREATE TABLE IF NOT EXISTS admin_tokens (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	token_hash TEXT NOT NULL UNIQUE,
	scopes TEXT NOT NULL,
	created_at INTEGER NOT NULL,
	expires_at INTEGER NOT NULL,
	last_used_at INTEGER,
	revoked_at INTEGER
);
CREATE TABLE IF NOT EXISTS metrics_daily (
	day TEXT PRIMARY KEY,
	sessions INTEGER NOT NULL DEFAULT 0,
	watch_seconds INTEGER NOT NULL DEFAULT 0,
	active_users INTEGER NOT NULL DEFAULT 0,
	new_users INTEGER NOT NULL DEFAULT 0,
	completed_sessions INTEGER NOT NULL DEFAULT 0,
	total_users INTEGER NOT NULL DEFAULT 0,
	computed_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS metrics_hourly (
	day TEXT NOT NULL,
	hour INTEGER NOT NULL,
	sessions INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (day, hour)
);
CREATE TABLE IF NOT EXISTS metrics_anime_daily (
	day TEXT NOT NULL,
	anime_id INTEGER NOT NULL,
	title TEXT NOT NULL DEFAULT '',
	sessions INTEGER NOT NULL DEFAULT 0,
	watch_seconds INTEGER NOT NULL DEFAULT 0,
	completed_sessions INTEGER NOT NULL DEFAULT 0,
	new_viewers INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (day, anime_id)
);
CREATE TABLE IF NOT EXISTS playback_errors (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	ts INTEGER NOT NULL,
	code TEXT NOT NULL,
	anime_id INTEGER,
	season_id INTEGER,
	episode INTEGER,
	source TEXT,
	info_hash TEXT,
	message TEXT
);
CREATE INDEX IF NOT EXISTS playback_errors_ts ON playback_errors(ts);
CREATE INDEX IF NOT EXISTS playback_errors_code_ts ON playback_errors(code, ts);
CREATE TABLE IF NOT EXISTS issues (
	id TEXT PRIMARY KEY,
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL,
	severity TEXT NOT NULL,
	status TEXT NOT NULL DEFAULT 'new',
	title TEXT NOT NULL,
	evidence TEXT,
	suggested_fix TEXT,
	source TEXT
);
CREATE TABLE IF NOT EXISTS mcp_audit (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	ts INTEGER NOT NULL,
	token_id INTEGER,
	tool TEXT NOT NULL,
	args_summary TEXT,
	outcome TEXT NOT NULL,
	duration_ms INTEGER,
	approval_id INTEGER
);
CREATE TABLE IF NOT EXISTS approvals (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	created_at INTEGER NOT NULL,
	tool TEXT NOT NULL,
	args TEXT NOT NULL,
	justification TEXT,
	expected_effect TEXT,
	status TEXT NOT NULL DEFAULT 'pending',
	decided_by TEXT,
	decided_at INTEGER,
	undo_token TEXT
);`

var migrations = []dbmigrate.Migration{
	{Version: 1, Name: "baseline", Up: dbmigrate.SQL(baselineSchema)},
	// The admin PATCH /issues/{id} stores a free-form triage note next to the status.
	{Version: 2, Name: "issues_note", Up: dbmigrate.SQL(`ALTER TABLE issues ADD COLUMN note TEXT`)},
	// M4 actions: settings (kill switch, alert thresholds, budget), the richer approvals queue and
	// the idempotency ledger of action calls.
	{Version: 3, Name: "ops_actions", Up: dbmigrate.SQL(
		`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT,
			updated_at INTEGER,
			updated_by TEXT
		)`,
		`ALTER TABLE approvals ADD COLUMN requested_by INTEGER`,
		`ALTER TABLE approvals ADD COLUMN plan TEXT`,
		`ALTER TABLE approvals ADD COLUMN result TEXT`,
		`ALTER TABLE approvals ADD COLUMN executed_at INTEGER`,
		`CREATE INDEX IF NOT EXISTS approvals_status ON approvals(status, id)`,
		`CREATE TABLE IF NOT EXISTS action_idempotency (
			token_id INTEGER NOT NULL,
			key TEXT NOT NULL,
			tool TEXT NOT NULL,
			args_hash TEXT NOT NULL,
			http_status INTEGER NOT NULL,
			response TEXT NOT NULL,
			created_at INTEGER NOT NULL,
			PRIMARY KEY (token_id, key)
		)`,
		`CREATE INDEX IF NOT EXISTS mcp_audit_token_ts ON mcp_audit(token_id, ts)`,
	)},
}

// Open opens (creating if needed) the admin database at path and migrates it.
// Driver and pragmas match the accounts store.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite3", path+"?_busy_timeout=5000&_foreign_keys=on&_journal_mode=WAL")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	if err := dbmigrate.Apply(context.Background(), db, migrations); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }
