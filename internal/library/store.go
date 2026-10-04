package library

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	sqlite3 "github.com/mattn/go-sqlite3"
)

// Store is the SQLite index of cached episode copies.
type Store struct{ db *sql.DB }

const schema = `
CREATE TABLE IF NOT EXISTS episodes (
	season_id INTEGER NOT NULL,
	episode INTEGER NOT NULL,
	lang TEXT NOT NULL,
	stream_id TEXT NOT NULL UNIQUE,
	anime_id INTEGER NOT NULL DEFAULT 0,
	title TEXT NOT NULL DEFAULT '',
	state TEXT NOT NULL,
	prev_state TEXT NOT NULL DEFAULT '',
	info_hash TEXT NOT NULL DEFAULT '',
	file_index INTEGER NOT NULL DEFAULT 0,
	release_name TEXT NOT NULL DEFAULT '',
	disk_id TEXT NOT NULL DEFAULT '',
	rel_path TEXT NOT NULL DEFAULT '',
	sha256 TEXT NOT NULL DEFAULT '',
	video_codec TEXT NOT NULL DEFAULT '',
	last_error TEXT NOT NULL DEFAULT '',
	encode_skipped TEXT NOT NULL DEFAULT '',
	size_bytes INTEGER NOT NULL DEFAULT 0,
	original_size_bytes INTEGER NOT NULL DEFAULT 0,
	reserved_bytes INTEGER NOT NULL DEFAULT 0,
	duration_ms INTEGER NOT NULL DEFAULT 0,
	audio_tracks INTEGER NOT NULL DEFAULT 0,
	subtitle_tracks INTEGER NOT NULL DEFAULT 0,
	attempts INTEGER NOT NULL DEFAULT 0,
	created_at INTEGER NOT NULL DEFAULT 0,
	updated_at INTEGER NOT NULL DEFAULT 0,
	last_access_at INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (season_id, episode, lang)
);
CREATE INDEX IF NOT EXISTS episodes_state ON episodes(state);
CREATE INDEX IF NOT EXISTS episodes_disk ON episodes(disk_id);
CREATE TABLE IF NOT EXISTS meta (
	key TEXT PRIMARY KEY,
	value BLOB NOT NULL
);
CREATE TABLE IF NOT EXISTS encoder_status (
	id INTEGER PRIMARY KEY CHECK(id = 1),
	has_key INTEGER NOT NULL DEFAULT 0,
	season_id INTEGER NOT NULL DEFAULT 0,
	episode INTEGER NOT NULL DEFAULT 0,
	lang TEXT NOT NULL DEFAULT '',
	progress REAL NOT NULL DEFAULT 0,
	paused INTEGER NOT NULL DEFAULT 0,
	updated_at INTEGER NOT NULL DEFAULT 0
);`

const columns = `season_id, episode, lang, anime_id, title, state, prev_state, info_hash, file_index, release_name, disk_id, rel_path, sha256, video_codec, last_error, encode_skipped, size_bytes, original_size_bytes, reserved_bytes, duration_ms, audio_tracks, subtitle_tracks, attempts, created_at, updated_at, last_access_at`

// OpenStore opens (creating if needed) dir/library.sqlite.
func OpenStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	// _txlock=immediate: Update's read-then-write transaction takes the write lock up front
	// (waiting on busy_timeout) instead of failing with SQLITE_BUSY on lock upgrade.
	db, err := sql.Open("sqlite3", filepath.Join(dir, "library.sqlite")+"?_busy_timeout=5000&_journal_mode=WAL&_txlock=immediate")
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

func toMS(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixMilli()
}

func fromMS(ms int64) time.Time {
	if ms == 0 {
		return time.Time{}
	}
	return time.UnixMilli(ms)
}

type scanner interface{ Scan(dest ...any) error }

func scanEntry(r scanner) (Entry, error) {
	var e Entry
	var state, prev string
	var created, updated, access int64
	err := r.Scan(&e.SeasonID, &e.Episode, &e.Lang, &e.AnimeID, &e.Title, &state, &prev, &e.InfoHash, &e.FileIndex, &e.ReleaseName, &e.DiskID, &e.RelPath, &e.SHA256, &e.VideoCodec, &e.LastError, &e.EncodeSkipped,
		&e.SizeBytes, &e.OriginalSizeBytes, &e.ReservedBytes, &e.DurationMS, &e.AudioTracks, &e.SubtitleTracks, &e.Attempts, &created, &updated, &access)
	if errors.Is(err, sql.ErrNoRows) {
		return Entry{}, ErrNotFound
	}
	if err != nil {
		return Entry{}, err
	}
	e.State, e.PrevState = State(state), State(prev)
	e.CreatedAt, e.UpdatedAt, e.LastAccessAt = fromMS(created), fromMS(updated), fromMS(access)
	return e, nil
}

func entryArgs(e Entry) []any {
	return []any{e.SeasonID, e.Episode, e.Lang, e.AnimeID, e.Title, string(e.State), string(e.PrevState), e.InfoHash, e.FileIndex, e.ReleaseName, e.DiskID, e.RelPath, e.SHA256, e.VideoCodec, e.LastError, e.EncodeSkipped,
		e.SizeBytes, e.OriginalSizeBytes, e.ReservedBytes, e.DurationMS, e.AudioTracks, e.SubtitleTracks, e.Attempts, toMS(e.CreatedAt), toMS(e.UpdatedAt), toMS(e.LastAccessAt)}
}

// Create inserts e, stamping CreatedAt/UpdatedAt when unset. It returns ErrExists if the key is taken.
func (s *Store) Create(e Entry) error {
	id, err := s.StreamID(e.Key)
	if err != nil {
		return err
	}
	now := time.Now()
	if e.CreatedAt.IsZero() {
		e.CreatedAt = now
	}
	if e.UpdatedAt.IsZero() {
		e.UpdatedAt = now
	}
	args := append([]any{id}, entryArgs(e)...)
	_, err = s.db.Exec(`INSERT INTO episodes(stream_id, `+columns+`) VALUES(`+strings.TrimSuffix(strings.Repeat("?,", 27), ",")+`)`, args...)
	var se sqlite3.Error
	if errors.As(err, &se) && se.Code == sqlite3.ErrConstraint {
		return ErrExists
	}
	return err
}

func (s *Store) Get(k Key) (Entry, error) {
	return scanEntry(s.db.QueryRow(`SELECT `+columns+` FROM episodes WHERE season_id = ? AND episode = ? AND lang = ?`, k.SeasonID, k.Episode, k.Lang))
}

func (s *Store) Delete(k Key) error {
	_, err := s.db.Exec(`DELETE FROM episodes WHERE season_id = ? AND episode = ? AND lang = ?`, k.SeasonID, k.Episode, k.Lang)
	return err
}

// Update applies fn to the entry in one transaction and stamps UpdatedAt. If fn fails nothing is
// written. The key itself cannot be changed by fn.
func (s *Store) Update(k Key, fn func(*Entry) error) (Entry, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return Entry{}, err
	}
	defer tx.Rollback()
	e, err := scanEntry(tx.QueryRow(`SELECT `+columns+` FROM episodes WHERE season_id = ? AND episode = ? AND lang = ?`, k.SeasonID, k.Episode, k.Lang))
	if err != nil {
		return Entry{}, err
	}
	if err := fn(&e); err != nil {
		return Entry{}, err
	}
	e.Key = k
	e.UpdatedAt = time.UnixMilli(time.Now().UnixMilli())
	if _, err := tx.Exec(`UPDATE episodes SET anime_id=?, title=?, state=?, prev_state=?, info_hash=?, file_index=?, release_name=?, disk_id=?, rel_path=?, sha256=?, video_codec=?, last_error=?, encode_skipped=?,
		size_bytes=?, original_size_bytes=?, reserved_bytes=?, duration_ms=?, audio_tracks=?, subtitle_tracks=?, attempts=?, created_at=?, updated_at=?, last_access_at=?
		WHERE season_id = ? AND episode = ? AND lang = ?`,
		append(entryArgs(e)[3:], k.SeasonID, k.Episode, k.Lang)...); err != nil {
		return Entry{}, err
	}
	if err := tx.Commit(); err != nil {
		return Entry{}, err
	}
	return e, nil
}

// List returns entries matching f, ordered by f.OrderBy ascending.
func (s *Store) List(f Filter) ([]Entry, error) {
	var where []string
	var args []any
	if len(f.States) > 0 {
		where = append(where, "state IN ("+strings.TrimSuffix(strings.Repeat("?,", len(f.States)), ",")+")")
		for _, st := range f.States {
			args = append(args, string(st))
		}
	}
	if f.DiskID != "" {
		where = append(where, "disk_id = ?")
		args = append(args, f.DiskID)
	}
	if f.SeasonID != 0 {
		where = append(where, "season_id = ?")
		args = append(args, f.SeasonID)
	}
	if f.Episode != 0 {
		where = append(where, "episode = ?")
		args = append(args, f.Episode)
	}
	q := `SELECT ` + columns + ` FROM episodes`
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	switch f.OrderBy {
	case "last_access_at":
		q += " ORDER BY last_access_at ASC, season_id, episode, lang"
	case "", "updated_at":
		q += " ORDER BY updated_at ASC, season_id, episode, lang"
	default:
		return nil, fmt.Errorf("library: unknown order %q", f.OrderBy)
	}
	if f.Limit > 0 {
		q += " LIMIT ?"
		args = append(args, f.Limit)
	}
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Entry{}
	for rows.Next() {
		e, err := scanEntry(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// Touch records the last time a copy was read.
func (s *Store) Touch(k Key, at time.Time) error {
	res, err := s.db.Exec(`UPDATE episodes SET last_access_at = ? WHERE season_id = ? AND episode = ? AND lang = ?`, toMS(at), k.SeasonID, k.Episode, k.Lang)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// Recover repairs the index after a crash: interrupted encodes go back to ORIGINAL, DOWNLOADING
// rows are only counted (the acquirer owns them) and every reservation is released.
func (s *Store) Recover() (RecoverReport, error) {
	var r RecoverReport
	tx, err := s.db.Begin()
	if err != nil {
		return r, err
	}
	defer tx.Rollback()
	if err := tx.QueryRow(`SELECT COUNT(*) FROM episodes WHERE state = ?`, string(StateDownloading)).Scan(&r.Downloading); err != nil {
		return r, err
	}
	res, err := tx.Exec(`UPDATE episodes SET state = ?, updated_at = ? WHERE state = ?`, string(StateOriginal), time.Now().UnixMilli(), string(StateEncoding))
	if err != nil {
		return r, err
	}
	n, _ := res.RowsAffected()
	r.Encoding = int(n)
	if _, err := tx.Exec(`UPDATE episodes SET reserved_bytes = 0 WHERE reserved_bytes != 0`); err != nil {
		return r, err
	}
	return r, tx.Commit()
}

// Secret returns the HMAC secret, generating and persisting 32 random bytes on first use.
func (s *Store) Secret() ([]byte, error) {
	fresh := make([]byte, 32)
	if _, err := rand.Read(fresh); err != nil {
		return nil, err
	}
	if _, err := s.db.Exec(`INSERT OR IGNORE INTO meta(key, value) VALUES('secret', ?)`, fresh); err != nil {
		return nil, err
	}
	var secret []byte
	if err := s.db.QueryRow(`SELECT value FROM meta WHERE key = 'secret'`).Scan(&secret); err != nil {
		return nil, err
	}
	return secret, nil
}

// StreamID is the opaque public id of a copy: 40 hex chars of HMAC-SHA256(secret, key).
func (s *Store) StreamID(k Key) (string, error) {
	secret, err := s.Secret()
	if err != nil {
		return "", err
	}
	m := hmac.New(sha256.New, secret)
	m.Write([]byte(k.String()))
	return hex.EncodeToString(m.Sum(nil))[:40], nil
}

func (s *Store) ByStreamID(id string) (Entry, error) {
	return scanEntry(s.db.QueryRow(`SELECT `+columns+` FROM episodes WHERE stream_id = ?`, id))
}

func (s *Store) SetEncoderStatus(st EncoderStatus) error {
	var hasKey int
	var k Key
	if st.Key != nil {
		hasKey, k = 1, *st.Key
	}
	paused := 0
	if st.Paused {
		paused = 1
	}
	_, err := s.db.Exec(`INSERT INTO encoder_status(id, has_key, season_id, episode, lang, progress, paused, updated_at) VALUES(1,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET has_key=excluded.has_key, season_id=excluded.season_id, episode=excluded.episode, lang=excluded.lang, progress=excluded.progress, paused=excluded.paused, updated_at=excluded.updated_at`,
		hasKey, k.SeasonID, k.Episode, k.Lang, st.Progress, paused, toMS(st.UpdatedAt))
	return err
}

// EncoderStatus returns the last status written, or the zero value if none.
func (s *Store) EncoderStatus() (EncoderStatus, error) {
	var st EncoderStatus
	var hasKey, paused int
	var k Key
	var updated int64
	err := s.db.QueryRow(`SELECT has_key, season_id, episode, lang, progress, paused, updated_at FROM encoder_status WHERE id = 1`).
		Scan(&hasKey, &k.SeasonID, &k.Episode, &k.Lang, &st.Progress, &paused, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return EncoderStatus{}, nil
	}
	if err != nil {
		return EncoderStatus{}, err
	}
	if hasKey == 1 {
		st.Key = &k
	}
	st.Paused = paused == 1
	st.UpdatedAt = fromMS(updated)
	return st, nil
}
