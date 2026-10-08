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
CREATE TABLE IF NOT EXISTS playback_startups (
	ts INTEGER NOT NULL,
	ms INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS playback_startups_ts ON playback_startups(ts);
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
	// M5: watch rules (state per rule, run log, before/after of the fixes they triggered).
	{Version: 4, Name: "watch", Up: dbmigrate.SQL(
		`CREATE TABLE IF NOT EXISTS watch_state (
			rule TEXT PRIMARY KEY,
			state TEXT NOT NULL,
			value REAL,
			since INTEGER NOT NULL,
			issue_id TEXT,
			last_eval INTEGER NOT NULL,
			ok_streak INTEGER NOT NULL DEFAULT 0,
			detail TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS watch_runs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			ts INTEGER NOT NULL,
			breached INTEGER NOT NULL,
			duration_ms INTEGER NOT NULL,
			webhook_sent INTEGER NOT NULL DEFAULT 0,
			webhook_failed INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS watch_effects (
			issue_id TEXT PRIMARY KEY,
			rule TEXT NOT NULL,
			value_open REAL,
			opened_at INTEGER NOT NULL,
			resolved_at INTEGER,
			value_resolved REAL,
			value_after REAL,
			after_at INTEGER
		)`,
		`CREATE INDEX IF NOT EXISTS watch_runs_ts ON watch_runs(ts)`,
		`CREATE INDEX IF NOT EXISTS mcp_audit_ts ON mcp_audit(ts)`,
	)},
	// Per-day aggregates behind /views (session length, retention curve, drop episodes): the page
	// reads them when every day of its period is present and scans watch_sessions otherwise.
	{Version: 5, Name: "views_rollup", Up: dbmigrate.SQL(
		`CREATE TABLE IF NOT EXISTS metrics_views_daily (
			day TEXT PRIMARY KEY,
			sessions INTEGER NOT NULL DEFAULT 0,
			minutes REAL NOT NULL DEFAULT 0,
			buckets TEXT NOT NULL DEFAULT '[]',
			present TEXT NOT NULL DEFAULT '[]',
			eligible TEXT NOT NULL DEFAULT '[]'
		)`,
		`CREATE TABLE IF NOT EXISTS metrics_drop_daily (
			day TEXT NOT NULL,
			anime_id INTEGER NOT NULL,
			season_id INTEGER NOT NULL,
			episode INTEGER NOT NULL,
			title TEXT NOT NULL DEFAULT '',
			title_at INTEGER NOT NULL DEFAULT 0,
			sessions INTEGER NOT NULL DEFAULT 0,
			abandoned INTEGER NOT NULL DEFAULT 0,
			minutes TEXT NOT NULL DEFAULT '[]',
			PRIMARY KEY (day, anime_id, season_id, episode)
		)`,
	)},
	{Version: 6, Name: "catalog_rollup", Up: dbmigrate.SQL(
		`CREATE TABLE IF NOT EXISTS metrics_catalog_daily (
			day TEXT NOT NULL,
			kind TEXT NOT NULL,
			a TEXT NOT NULL,
			b TEXT NOT NULL DEFAULT '',
			n INTEGER NOT NULL DEFAULT 0,
			completed INTEGER NOT NULL DEFAULT 0,
			secs REAL NOT NULL DEFAULT 0,
			title TEXT NOT NULL DEFAULT '',
			title_at INTEGER NOT NULL DEFAULT 0,
			fmt_at INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (day, kind, a, b)
		)`,
	)},
	// A database created before playback_startups joined the baseline never got the table (the
	// baseline only runs once): the startup-time metric and the nightly prune failed on it.
	{Version: 7, Name: "playback_startups_table", Up: dbmigrate.SQL(
		`CREATE TABLE IF NOT EXISTS playback_startups (
			ts INTEGER NOT NULL,
			ms INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS playback_startups_ts ON playback_startups(ts)`,
	)},
	// Per-user-day activity behind the users and growth pages (segments, directory, funnel,
	// DAU/WAU/MAU, churn) and the per-day concurrency peaks behind /costs. A day's
	// metrics_activity_daily row is its coverage marker: it is written in the same transaction as
	// the day's metrics_user_daily rows, including for a day without any session.
	{Version: 8, Name: "user_activity_rollup", Up: dbmigrate.SQL(
		`CREATE TABLE IF NOT EXISTS metrics_user_daily (
			day TEXT NOT NULL,
			user_id INTEGER NOT NULL,
			sessions INTEGER NOT NULL DEFAULT 0,
			watch_seconds REAL NOT NULL DEFAULT 0,
			first_at INTEGER NOT NULL,
			last_at INTEGER NOT NULL,
			episodes TEXT NOT NULL DEFAULT '[]',
			PRIMARY KEY (day, user_id)
		)`,
		`CREATE INDEX IF NOT EXISTS metrics_user_daily_user ON metrics_user_daily(user_id, day)`,
		`CREATE TABLE IF NOT EXISTS metrics_activity_daily (
			day TEXT PRIMARY KEY,
			computed_at INTEGER NOT NULL,
			peak_own INTEGER NOT NULL DEFAULT 0,
			peak_full INTEGER NOT NULL DEFAULT 0,
			max_end INTEGER NOT NULL DEFAULT 0
		)`,
	)},
	// Per-day server load counters (bytes served, ffmpeg CPU, client mix, remux peak): see loadstats.
	{Version: 9, Name: "metrics_load_daily", Up: dbmigrate.SQL(
		`CREATE TABLE IF NOT EXISTS metrics_load_daily (
			day TEXT PRIMARY KEY,
			bytes_out INTEGER NOT NULL DEFAULT 0,
			cpu_copy_ms INTEGER NOT NULL DEFAULT 0,
			cpu_transcode_ms INTEGER NOT NULL DEFAULT 0,
			sessions INTEGER NOT NULL DEFAULT 0,
			sessions_apple INTEGER NOT NULL DEFAULT 0,
			sessions_noav1 INTEGER NOT NULL DEFAULT 0,
			sessions_transcode INTEGER NOT NULL DEFAULT 0,
			remux_rejected INTEGER NOT NULL DEFAULT 0,
			peak_remuxes INTEGER NOT NULL DEFAULT 0,
			peak_ffmpeg INTEGER NOT NULL DEFAULT 0
		)`,
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
