// Package donations records donations (BTCPay Server crypto invoices and Ko-fi webhooks).
//
// Privacy model: the admin always sees exactly who gave what (linked account, provider reference,
// amount), while the public side only ever sees a donor-chosen display name, and nothing at all by
// default ("anonymous"). Emails from providers are never stored.
package donations

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/gazes/gazes/internal/dbmigrate"
	_ "github.com/mattn/go-sqlite3"
)

const (
	ProviderBTCPay = "btcpay"
	ProviderKofi   = "kofi"
	ProviderManual = "manual"

	StatusPending = "pending"
	StatusSettled = "settled"
	StatusExpired = "expired"

	VisAnonymous = "anonymous" // never shown publicly (default)
	VisNamed     = "named"     // shown under DisplayName

	CurrencyEUR = "EUR"
)

var migrations = []dbmigrate.Migration{
	{Version: 1, Name: "baseline", Up: dbmigrate.SQL(
		`CREATE TABLE IF NOT EXISTS donations (
			id TEXT PRIMARY KEY,
			provider TEXT NOT NULL,
			provider_ref TEXT NOT NULL,
			user_id INTEGER,
			donor_label TEXT NOT NULL DEFAULT '',
			visibility TEXT NOT NULL DEFAULT 'anonymous',
			display_name TEXT NOT NULL DEFAULT '',
			amount_cents INTEGER NOT NULL,
			currency TEXT NOT NULL,
			status TEXT NOT NULL,
			message TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL,
			settled_at INTEGER,
			UNIQUE (provider, provider_ref)
		)`,
		`CREATE INDEX IF NOT EXISTS donations_status_settled ON donations(status, settled_at)`,
		`CREATE INDEX IF NOT EXISTS donations_user ON donations(user_id)`,
	)},
}

// Donation is one row. DonorLabel, UserID and ProviderRef are admin-only.
type Donation struct {
	ID          string
	Provider    string
	ProviderRef string
	UserID      *int64
	DonorLabel  string
	Visibility  string
	DisplayName string
	AmountCents int64
	Currency    string
	Status      string
	Message     string
	CreatedAt   int64
	SettledAt   *int64
}

type Store struct{ db *sql.DB }

// Open opens (creating if needed) dir/donations.sqlite.
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite3", filepath.Join(dir, "donations.sqlite")+"?_busy_timeout=5000&_foreign_keys=on&_journal_mode=WAL")
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

// NewID returns a fresh donation id.
func NewID() string { return newID() }

func newID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Insert adds a donation; a (provider, provider_ref) already stored is left untouched and reported
// as created=false (webhooks are delivered at least once).
func (s *Store) Insert(ctx context.Context, d Donation) (Donation, bool, error) {
	if d.ID == "" {
		d.ID = newID()
	}
	res, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO donations
		(id, provider, provider_ref, user_id, donor_label, visibility, display_name, amount_cents, currency, status, message, created_at, settled_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		d.ID, d.Provider, d.ProviderRef, d.UserID, d.DonorLabel, d.Visibility, d.DisplayName, d.AmountCents, d.Currency, d.Status, d.Message, d.CreatedAt, d.SettledAt)
	if err != nil {
		return d, false, err
	}
	n, _ := res.RowsAffected()
	return d, n == 1, nil
}

// SetProviderRef attaches the provider's invoice id to a pending donation.
func (s *Store) SetProviderRef(ctx context.Context, id, ref string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE donations SET provider_ref = ? WHERE id = ?`, ref, id)
	return err
}

// Settle marks a pending donation settled with the amount the provider confirmed. It reports
// whether the row changed (false: unknown or already settled, so a replayed webhook is a no-op).
func (s *Store) Settle(ctx context.Context, provider, ref string, amountCents int64, currency string, at time.Time) (bool, error) {
	res, err := s.db.ExecContext(ctx, `UPDATE donations SET status = ?, amount_cents = ?, currency = ?, settled_at = ?
		WHERE provider = ? AND provider_ref = ? AND status = ?`,
		StatusSettled, amountCents, currency, at.Unix(), provider, ref, StatusPending)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// Expire marks a pending donation expired.
func (s *Store) Expire(ctx context.Context, provider, ref string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE donations SET status = ? WHERE provider = ? AND provider_ref = ? AND status = ?`,
		StatusExpired, provider, ref, StatusPending)
	return err
}

var ErrNotFound = errors.New("donation not found")

// Link attaches a donation to an account (admin action, for Ko-fi gifts that carry no account).
func (s *Store) Link(ctx context.Context, id string, userID *int64) error {
	res, err := s.db.ExecContext(ctx, `UPDATE donations SET user_id = ? WHERE id = ?`, userID, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

const cols = `id, provider, provider_ref, user_id, donor_label, visibility, display_name, amount_cents, currency, status, message, created_at, settled_at`

func scan(rows interface{ Scan(...any) error }) (Donation, error) {
	var d Donation
	var uid, settled sql.NullInt64
	if err := rows.Scan(&d.ID, &d.Provider, &d.ProviderRef, &uid, &d.DonorLabel, &d.Visibility, &d.DisplayName,
		&d.AmountCents, &d.Currency, &d.Status, &d.Message, &d.CreatedAt, &settled); err != nil {
		return d, err
	}
	if uid.Valid {
		d.UserID = &uid.Int64
	}
	if settled.Valid {
		d.SettledAt = &settled.Int64
	}
	return d, nil
}

// List returns donations newest first (admin).
func (s *Store) List(ctx context.Context, status string, limit, offset int) ([]Donation, error) {
	q := `SELECT ` + cols + ` FROM donations`
	args := []any{}
	if status != "" {
		q += ` WHERE status = ?`
		args = append(args, status)
	}
	q += ` ORDER BY created_at DESC, id LIMIT ? OFFSET ?`
	args = append(args, limit, offset)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Donation{}
	for rows.Next() {
		d, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// Totals returns the settled EUR total since `since` (unix) and the all-time settled EUR total.
// Other currencies are not converted and stay out of the totals.
func (s *Store) Totals(ctx context.Context, since int64) (month, all int64, err error) {
	err = s.db.QueryRowContext(ctx, `SELECT
		COALESCE(SUM(CASE WHEN settled_at >= ? THEN amount_cents END), 0),
		COALESCE(SUM(amount_cents), 0)
		FROM donations WHERE status = ? AND currency = ?`, since, StatusSettled, CurrencyEUR).Scan(&month, &all)
	return
}

// PublicNames returns the display names of settled, non-anonymous donors, newest first, deduplicated.
func (s *Store) PublicNames(ctx context.Context, limit int) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT display_name FROM donations
		WHERE status = ? AND visibility = ? AND display_name <> ''
		GROUP BY display_name ORDER BY MAX(settled_at) DESC LIMIT ?`, StatusSettled, VisNamed, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// SetVisibility changes what is shown publicly (admin moderation: anonymous clears the name).
func (s *Store) SetVisibility(ctx context.Context, id, visibility, displayName string) error {
	if visibility != VisNamed {
		visibility, displayName = VisAnonymous, ""
	}
	res, err := s.db.ExecContext(ctx, `UPDATE donations SET visibility = ?, display_name = ? WHERE id = ?`, visibility, displayName, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// Summary is the admin headline: settled EUR donations only.
type Summary struct {
	MonthCents int64 `json:"month_cents"`
	AllCents   int64 `json:"all_cents"`
	Count      int64 `json:"count"`
	Donors     int64 `json:"donors"` // distinct accounts or labels
}

func (s *Store) Summary(ctx context.Context, since int64) (Summary, error) {
	var out Summary
	err := s.db.QueryRowContext(ctx, `SELECT
		COALESCE(SUM(CASE WHEN settled_at >= ? THEN amount_cents END), 0),
		COALESCE(SUM(amount_cents), 0),
		COUNT(*),
		COUNT(DISTINCT CASE WHEN user_id IS NOT NULL THEN 'u' || user_id WHEN donor_label <> '' THEN 'l' || donor_label ELSE id END)
		FROM donations WHERE status = ? AND currency = ?`, since, StatusSettled, CurrencyEUR).Scan(&out.MonthCents, &out.AllCents, &out.Count, &out.Donors)
	return out, err
}
