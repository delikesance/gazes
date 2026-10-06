package metadata

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/klauspost/compress/zstd"
	_ "github.com/mattn/go-sqlite3"
)

// anilistStore keeps every successful AniList answer on disk, with no expiry, so the catalog
// grows into our own database: a stored answer is served without calling AniList (or, past the
// max age a caller asks for, only when AniList cannot answer).
type anilistStore struct{ db *sql.DB }

var (
	sharedAnilistStore atomic.Pointer[anilistStore]
	storeEnc, _        = zstd.NewWriter(nil)
	storeDec, _        = zstd.NewReader(nil)
)

// OpenAnilistStore opens (creating it if needed) the durable AniList answer store and makes every
// AniList client use it. Without a call, answers are only cached by the in-memory and Redis layers.
func OpenAnilistStore(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	db, err := sql.Open("sqlite3", path+"?_busy_timeout=5000&_journal_mode=WAL&_synchronous=NORMAL")
	if err != nil {
		return err
	}
	db.SetMaxOpenConns(4)
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS anilist_responses (
		key TEXT PRIMARY KEY, body BLOB NOT NULL, fetched_at INTEGER NOT NULL)`); err != nil {
		db.Close()
		return err
	}
	sharedAnilistStore.Store(&anilistStore{db: db})
	return nil
}

// CloseAnilistStore detaches and closes the store.
func CloseAnilistStore() {
	if s := sharedAnilistStore.Swap(nil); s != nil {
		s.db.Close()
	}
}

// anilistKey identifies one request: the query text and its variables (map keys marshal sorted).
func anilistKey(query string, variables any) (string, error) {
	vars, err := json.Marshal(variables)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(query + "\x00" + string(vars)))
	return hex.EncodeToString(sum[:]), nil
}

func (s *anilistStore) get(ctx context.Context, key string) ([]byte, time.Time, bool) {
	var blob []byte
	var at int64
	if err := s.db.QueryRowContext(ctx, `SELECT body, fetched_at FROM anilist_responses WHERE key = ?`, key).Scan(&blob, &at); err != nil {
		return nil, time.Time{}, false
	}
	body, err := storeDec.DecodeAll(blob, nil)
	if err != nil {
		return nil, time.Time{}, false
	}
	return body, time.Unix(at, 0), true
}

func (s *anilistStore) put(ctx context.Context, key string, body []byte) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO anilist_responses (key, body, fetched_at) VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET body = excluded.body, fetched_at = excluded.fetched_at`,
		key, storeEnc.EncodeAll(body, nil), time.Now().Unix())
	return err
}

// storable reports whether an answer is a clean success: GraphQL reports failures with a 200.
func storable(body []byte) bool {
	var probe struct {
		Errors []json.RawMessage `json:"errors"`
		Data   json.RawMessage   `json:"data"`
	}
	return json.Unmarshal(body, &probe) == nil && len(probe.Errors) == 0 && len(probe.Data) > 0 && string(probe.Data) != "null"
}
