package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/gazes/gazes/internal/dbmigrate"
	sqlite3 "github.com/mattn/go-sqlite3"
)

var ErrEmailTaken = errors.New("email already registered")

// Account roles stored in users.role.
const (
	RoleUser  = "user"
	RoleAdmin = "admin"
)

// ErrInvalidRole is returned by SetUserRole for a role other than user or admin.
var ErrInvalidRole = errors.New("invalid role")

// ErrAmbiguousPseudo is returned by SetUserRole when several users share the pseudo.
var ErrAmbiguousPseudo = errors.New("pseudo matches several users")

// Store persists users, sessions and watch progress in SQLite.
type Store struct{ db *sql.DB }

type User struct {
	ID       int64
	EmailIdx string
	EmailEnc string
	Pseudo   string
	PassHash string
	Role     string
}

// Progress is one season's resume point; Completed acts as a tombstone so a
// finished season disappears on every device.
type Progress struct {
	SeasonID  int64   `json:"season_id"`
	AnimeID   int64   `json:"anime_id"`
	Title     string  `json:"title"`
	Episode   int     `json:"episode"`
	Position  float64 `json:"position"`
	Completed bool    `json:"completed"`
	UpdatedAt int64   `json:"updated_at"`
}

// WatchSession is one sitting on one episode, kept for good (unlike Progress, completion never
// removes it) as raw material for recommendations.
type WatchSession struct {
	ID             string   `json:"id"`
	SeasonID       int64    `json:"season_id"`
	AnimeID        int64    `json:"anime_id"`
	Episode        int      `json:"episode"`
	Title          string   `json:"title"`
	Genres         []string `json:"genres"`
	Format         string   `json:"format"`
	StartedAt      int64    `json:"started_at"`
	UpdatedAt      int64    `json:"updated_at"`
	StartPosition  float64  `json:"start_position"`
	EndPosition    float64  `json:"end_position"`
	WatchedSeconds float64  `json:"watched_seconds"`
	Duration       float64  `json:"duration"`
	Completed      bool     `json:"completed"`
	AudioLang      string   `json:"audio_lang"`
	SubLang        string   `json:"sub_lang"`
	TZOffset       int      `json:"tz_offset"`
}

const baselineSchema = `
CREATE TABLE IF NOT EXISTS users (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	email_idx TEXT NOT NULL UNIQUE,
	email_enc TEXT NOT NULL,
	pseudo TEXT NOT NULL,
	pass_hash TEXT NOT NULL,
	created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS sessions (
	token_hash TEXT PRIMARY KEY,
	user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	expires_at INTEGER NOT NULL,
	last_seen INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS sessions_user ON sessions(user_id);
CREATE TABLE IF NOT EXISTS progress (
	user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	season_id INTEGER NOT NULL,
	anime_id INTEGER NOT NULL DEFAULT 0,
	title TEXT NOT NULL DEFAULT '',
	episode INTEGER NOT NULL,
	position REAL NOT NULL,
	completed INTEGER NOT NULL,
	updated_at INTEGER NOT NULL,
	PRIMARY KEY (user_id, season_id)
);
CREATE TABLE IF NOT EXISTS watch_sessions (
	user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	session_id TEXT NOT NULL,
	season_id INTEGER NOT NULL,
	anime_id INTEGER NOT NULL DEFAULT 0,
	episode INTEGER NOT NULL,
	title TEXT NOT NULL DEFAULT '',
	genres TEXT NOT NULL DEFAULT '[]',
	format TEXT NOT NULL DEFAULT '',
	started_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL,
	start_position REAL NOT NULL DEFAULT 0,
	end_position REAL NOT NULL DEFAULT 0,
	watched_seconds REAL NOT NULL DEFAULT 0,
	duration REAL NOT NULL DEFAULT 0,
	completed INTEGER NOT NULL DEFAULT 0,
	audio_lang TEXT NOT NULL DEFAULT '',
	sub_lang TEXT NOT NULL DEFAULT '',
	tz_offset INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (user_id, session_id)
);
CREATE TABLE IF NOT EXISTS hidden_anime (
	user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	anime_id INTEGER NOT NULL,
	created_at INTEGER NOT NULL,
	PRIMARY KEY (user_id, anime_id)
);
CREATE INDEX IF NOT EXISTS watch_sessions_user_time ON watch_sessions(user_id, started_at);`

// migrations is the ordered schema history of accounts.sqlite. A database created before
// versioning (user_version 0) is adopted by migration 1, which is idempotent.
var migrations = []dbmigrate.Migration{
	{Version: 1, Name: "baseline", Up: dbmigrate.SQL(baselineSchema)},
	{Version: 2, Name: "users_role_and_watch_started_index", Up: func(tx *sql.Tx) error {
		var n int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('users') WHERE name = 'role'`).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			if _, err := tx.Exec(`ALTER TABLE users ADD COLUMN role TEXT NOT NULL DEFAULT 'user'`); err != nil {
				return err
			}
		}
		_, err := tx.Exec(`CREATE INDEX IF NOT EXISTS watch_sessions_started ON watch_sessions(started_at)`)
		return err
	}},
}

// OpenStore opens (creating if needed) dir/accounts.sqlite.
func OpenStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite3", filepath.Join(dir, "accounts.sqlite")+"?_busy_timeout=5000&_foreign_keys=on&_journal_mode=WAL")
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

func (s *Store) CreateUser(u User, now time.Time) (int64, error) {
	res, err := s.db.Exec(`INSERT INTO users(email_idx, email_enc, pseudo, pass_hash, created_at) VALUES(?,?,?,?,?)`,
		u.EmailIdx, u.EmailEnc, u.Pseudo, u.PassHash, now.Unix())
	if err != nil {
		var se sqlite3.Error
		if errors.As(err, &se) && se.ExtendedCode == sqlite3.ErrConstraintUnique {
			return 0, ErrEmailTaken
		}
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) UserByEmailIdx(idx string) (*User, error) {
	return s.scanUser(s.db.QueryRow(`SELECT id, email_idx, email_enc, pseudo, pass_hash, role FROM users WHERE email_idx = ?`, idx))
}

func (s *Store) UserByID(id int64) (*User, error) {
	return s.scanUser(s.db.QueryRow(`SELECT id, email_idx, email_enc, pseudo, pass_hash, role FROM users WHERE id = ?`, id))
}

func (s *Store) scanUser(row *sql.Row) (*User, error) {
	var u User
	if err := row.Scan(&u.ID, &u.EmailIdx, &u.EmailEnc, &u.Pseudo, &u.PassHash, &u.Role); err != nil {
		return nil, err
	}
	return &u, nil
}

// SetUserRole sets a user's role (user or admin), looked up by pseudo. Pseudos are not unique,
// so an ambiguous pseudo is refused rather than promoting several accounts.
func (s *Store) SetUserRole(ctx context.Context, pseudo, role string) error {
	if role != RoleUser && role != RoleAdmin {
		return ErrInvalidRole
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE pseudo = ?`, pseudo).Scan(&n); err != nil {
		return err
	}
	switch {
	case n == 0:
		return sql.ErrNoRows
	case n > 1:
		return ErrAmbiguousPseudo
	}
	if _, err := tx.ExecContext(ctx, `UPDATE users SET role = ? WHERE pseudo = ?`, role, pseudo); err != nil {
		return err
	}
	return tx.Commit()
}

// CountAdmins returns how many users hold the admin role.
func (s *Store) CountAdmins(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE role = ?`, RoleAdmin).Scan(&n)
	return n, err
}

func (s *Store) CreateSession(tokenHash string, userID int64, expires, now time.Time) error {
	_, err := s.db.Exec(`INSERT INTO sessions(token_hash, user_id, expires_at, last_seen) VALUES(?,?,?,?)`, tokenHash, userID, expires.Unix(), now.Unix())
	return err
}

// SessionUser resolves a live session and slides its expiry at most once a day.
func (s *Store) SessionUser(tokenHash string, now time.Time, lifetime time.Duration) (*User, error) {
	var userID, expires, lastSeen int64
	err := s.db.QueryRow(`SELECT user_id, expires_at, last_seen FROM sessions WHERE token_hash = ?`, tokenHash).Scan(&userID, &expires, &lastSeen)
	if err != nil {
		return nil, err
	}
	if now.Unix() > expires {
		_, _ = s.db.Exec(`DELETE FROM sessions WHERE token_hash = ?`, tokenHash)
		return nil, sql.ErrNoRows
	}
	if now.Unix()-lastSeen > 24*3600 {
		_, _ = s.db.Exec(`UPDATE sessions SET last_seen = ?, expires_at = ? WHERE token_hash = ?`, now.Unix(), now.Add(lifetime).Unix(), tokenHash)
	}
	return s.UserByID(userID)
}

func (s *Store) DeleteSession(tokenHash string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE token_hash = ?`, tokenHash)
	return err
}

func (s *Store) PurgeExpired(now time.Time) {
	_, _ = s.db.Exec(`DELETE FROM sessions WHERE expires_at < ?`, now.Unix())
}

// MergeProgress upserts each entry only if it is newer than what is stored.
func (s *Store) MergeProgress(userID int64, items []Progress) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, p := range items {
		done := 0
		if p.Completed {
			done = 1
		}
		if _, err := tx.Exec(`INSERT INTO progress(user_id, season_id, anime_id, title, episode, position, completed, updated_at) VALUES(?,?,?,?,?,?,?,?)
			ON CONFLICT(user_id, season_id) DO UPDATE SET anime_id=excluded.anime_id, title=excluded.title, episode=excluded.episode, position=excluded.position, completed=excluded.completed, updated_at=excluded.updated_at
			WHERE excluded.updated_at > progress.updated_at`, userID, p.SeasonID, p.AnimeID, p.Title, p.Episode, p.Position, done, p.UpdatedAt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ListProgress(userID int64) ([]Progress, error) {
	rows, err := s.db.Query(`SELECT season_id, anime_id, title, episode, position, completed, updated_at FROM progress WHERE user_id = ? ORDER BY updated_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Progress{}
	for rows.Next() {
		var p Progress
		var done int
		if err := rows.Scan(&p.SeasonID, &p.AnimeID, &p.Title, &p.Episode, &p.Position, &done, &p.UpdatedAt); err != nil {
			return nil, err
		}
		p.Completed = done == 1
		out = append(out, p)
	}
	return out, rows.Err()
}

// MergeWatchSessions upserts each session only if it is newer than the stored one.
func (s *Store) MergeWatchSessions(userID int64, items []WatchSession) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, w := range items {
		genres, _ := json.Marshal(w.Genres)
		done := 0
		if w.Completed {
			done = 1
		}
		if _, err := tx.Exec(`INSERT INTO watch_sessions(user_id, session_id, season_id, anime_id, episode, title, genres, format, started_at, updated_at, start_position, end_position, watched_seconds, duration, completed, audio_lang, sub_lang, tz_offset)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
			ON CONFLICT(user_id, session_id) DO UPDATE SET season_id=excluded.season_id, anime_id=excluded.anime_id, episode=excluded.episode, title=excluded.title, genres=excluded.genres, format=excluded.format,
				updated_at=excluded.updated_at, end_position=excluded.end_position, watched_seconds=excluded.watched_seconds, duration=excluded.duration, completed=excluded.completed, audio_lang=excluded.audio_lang, sub_lang=excluded.sub_lang
			WHERE excluded.updated_at > watch_sessions.updated_at`,
			userID, w.ID, w.SeasonID, w.AnimeID, w.Episode, w.Title, string(genres), w.Format, w.StartedAt, w.UpdatedAt, w.StartPosition, w.EndPosition, w.WatchedSeconds, w.Duration, done, w.AudioLang, w.SubLang, w.TZOffset); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ListWatchSessions returns a user's sessions updated after since, newest first (at most limit).
func (s *Store) ListWatchSessions(userID, since int64, limit int) ([]WatchSession, error) {
	rows, err := s.db.Query(`SELECT session_id, season_id, anime_id, episode, title, genres, format, started_at, updated_at, start_position, end_position, watched_seconds, duration, completed, audio_lang, sub_lang, tz_offset
		FROM watch_sessions WHERE user_id = ? AND updated_at > ? ORDER BY updated_at DESC LIMIT ?`, userID, since, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WatchSession{}
	for rows.Next() {
		var w WatchSession
		var genres string
		var done int
		if err := rows.Scan(&w.ID, &w.SeasonID, &w.AnimeID, &w.Episode, &w.Title, &genres, &w.Format, &w.StartedAt, &w.UpdatedAt, &w.StartPosition, &w.EndPosition, &w.WatchedSeconds, &w.Duration, &done, &w.AudioLang, &w.SubLang, &w.TZOffset); err != nil {
			return nil, err
		}
		if json.Unmarshal([]byte(genres), &w.Genres) != nil || w.Genres == nil {
			w.Genres = []string{}
		}
		w.Completed = done == 1
		out = append(out, w)
	}
	return out, rows.Err()
}

// UpdateHidden adds and removes "not interested" anime in one transaction.
func (s *Store) UpdateHidden(userID int64, add, remove []int64, now time.Time) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, id := range add {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO hidden_anime(user_id, anime_id, created_at) VALUES(?,?,?)`, userID, id, now.Unix()); err != nil {
			return err
		}
	}
	for _, id := range remove {
		if _, err := tx.Exec(`DELETE FROM hidden_anime WHERE user_id = ? AND anime_id = ?`, userID, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ListHidden(userID int64) ([]int64, error) {
	rows, err := s.db.Query(`SELECT anime_id FROM hidden_anime WHERE user_id = ? ORDER BY created_at DESC LIMIT 2000`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// DeleteWatchData erases a user's watch sessions and hidden anime. The account and the resume
// points (progress) are left alone.
func (s *Store) DeleteWatchData(userID int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM watch_sessions WHERE user_id = ?`, userID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM hidden_anime WHERE user_id = ?`, userID); err != nil {
		return err
	}
	return tx.Commit()
}
