package kv

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// Durable is the last level of a Cache: a SQLite file that outlives Redis. A deploy that recreates
// Redis, an eviction or an upstream outage then costs a read from disk instead of a call to a
// rate-limited upstream. It stores the same envelope bytes Redis does, so freshness, staleness and
// the stale-while-revalidate logic stay in one place.
type Durable struct {
	db     *sql.DB
	errors atomic.Int64
}

// OpenDurable opens (creating it if needed) the store at path.
func OpenDurable(path string) (*Durable, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite3", path+"?_busy_timeout=5000&_journal_mode=WAL&_synchronous=NORMAL")
	if err != nil {
		return nil, err
	}
	// One writer at a time keeps SQLite from returning SQLITE_BUSY under the importer's batches.
	db.SetMaxOpenConns(4)
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS entries (
		domain TEXT NOT NULL,
		key TEXT NOT NULL,
		value BLOB NOT NULL,
		stored_at INTEGER NOT NULL,
		PRIMARY KEY (domain, key)
	) WITHOUT ROWID`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("durable store: %w", err)
	}
	return &Durable{db: db}, nil
}

// Close releases the file.
func (d *Durable) Close() error { return d.db.Close() }

// Errors counts failed reads and writes; the store degrades to a miss, never to an outage.
func (d *Durable) Errors() int64 { return d.errors.Load() }

// Get returns the stored bytes of domain/key.
func (d *Durable) Get(domain, key string) ([]byte, bool) {
	var v []byte
	err := d.db.QueryRow(`SELECT value FROM entries WHERE domain = ? AND key = ?`, domain, key).Scan(&v)
	if err != nil {
		if err != sql.ErrNoRows {
			d.errors.Add(1)
		}
		return nil, false
	}
	return v, true
}

// Set stores (or replaces) domain/key.
func (d *Durable) Set(domain, key string, value []byte) {
	if _, err := d.db.Exec(`INSERT INTO entries (domain, key, value, stored_at) VALUES (?, ?, ?, ?)
		ON CONFLICT(domain, key) DO UPDATE SET value = excluded.value, stored_at = excluded.stored_at`,
		domain, key, value, time.Now().UnixMilli()); err != nil {
		d.errors.Add(1)
	}
}

// Delete removes domain/key.
func (d *Durable) Delete(domain, key string) {
	if _, err := d.db.Exec(`DELETE FROM entries WHERE domain = ? AND key = ?`, domain, key); err != nil {
		d.errors.Add(1)
	}
}

// Count returns how many entries a domain holds.
func (d *Durable) Count(domain string) int {
	var n int
	if err := d.db.QueryRow(`SELECT COUNT(*) FROM entries WHERE domain = ?`, domain).Scan(&n); err != nil {
		d.errors.Add(1)
	}
	return n
}

// PruneOlderThan drops the entries of a domain written longer than age ago and returns how many.
// It bounds domains keyed by free-form input (search queries) that would otherwise grow forever.
func (d *Durable) PruneOlderThan(domain string, age time.Duration) int {
	res, err := d.db.Exec(`DELETE FROM entries WHERE domain = ? AND stored_at < ?`, domain, time.Now().Add(-age).UnixMilli())
	if err != nil {
		d.errors.Add(1)
		return 0
	}
	n, _ := res.RowsAffected()
	return int(n)
}
