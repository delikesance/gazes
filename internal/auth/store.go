package auth

import (
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	sqlite3 "github.com/mattn/go-sqlite3"
)

var ErrEmailTaken = errors.New("email already registered")

// Store persists users, sessions and watch progress in SQLite.
type Store struct{ db *sql.DB }

type User struct {
	ID       int64
	EmailIdx string
	EmailEnc string
	Pseudo   string
	PassHash string
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

const schema = `
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
CREATE INDEX IF NOT EXISTS watch_sessions_user_time ON watch_sessions(user_id, started_at);`

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
	if _, err := db.Exec(schema); err != nil {
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
	return s.scanUser(s.db.QueryRow(`SELECT id, email_idx, email_enc, pseudo, pass_hash FROM users WHERE email_idx = ?`, idx))
}

func (s *Store) UserByID(id int64) (*User, error) {
	return s.scanUser(s.db.QueryRow(`SELECT id, email_idx, email_enc, pseudo, pass_hash FROM users WHERE id = ?`, id))
}

func (s *Store) scanUser(row *sql.Row) (*User, error) {
	var u User
	if err := row.Scan(&u.ID, &u.EmailIdx, &u.EmailEnc, &u.Pseudo, &u.PassHash); err != nil {
		return nil, err
	}
	return &u, nil
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
