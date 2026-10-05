package admin

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Token scopes (plan D4).
const (
	ScopeMetricsRead     = "metrics:read"
	ScopeDiagnosticsRead = "diagnostics:read"
	ScopeOpsWrite        = "ops:write"
	ScopeConfigWrite     = "config:write"
)

const (
	tokenPrefix     = "gzs_"
	tokenRandBytes  = 32
	DefaultTokenTTL = 30 * 24 * time.Hour
	MaxTokenTTL     = 365 * 24 * time.Hour
	lastUsedGrain   = time.Minute
)

var validScopes = map[string]bool{ScopeMetricsRead: true, ScopeDiagnosticsRead: true, ScopeOpsWrite: true, ScopeConfigWrite: true}

// ErrInvalidToken is returned for unknown, malformed, expired and revoked tokens alike.
var ErrInvalidToken = errors.New("invalid token")

// Token is the metadata of an admin token. It never carries the secret nor its hash.
type Token struct {
	ID         int64
	Name       string
	Scopes     []string
	CreatedAt  time.Time
	ExpiresAt  time.Time
	LastUsedAt *time.Time
	RevokedAt  *time.Time
}

// HasScope reports whether the token grants scope.
func (t *Token) HasScope(scope string) bool {
	if t == nil {
		return false
	}
	for _, s := range t.Scopes {
		if s == scope {
			return true
		}
	}
	return false
}

// Status is "revoked", "expired" or "active".
func (t *Token) Status(now time.Time) string {
	switch {
	case t.RevokedAt != nil:
		return "revoked"
	case !now.Before(t.ExpiresAt):
		return "expired"
	}
	return "active"
}

// ParseScopes validates, deduplicates and sorts a scope list.
func ParseScopes(scopes []string) ([]string, error) {
	seen := map[string]bool{}
	var out []string
	for _, s := range scopes {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if !validScopes[s] {
			return nil, fmt.Errorf("unknown scope %q", s)
		}
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("at least one scope is required")
	}
	sort.Strings(out)
	return out, nil
}

func hashToken(plain string) string {
	h := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(h[:])
}

// CreateToken mints a token. The plain value is returned once and only its SHA-256 is stored.
// A zero ttl means DefaultTokenTTL; ttl above MaxTokenTTL or negative is refused.
func (s *Store) CreateToken(ctx context.Context, name string, scopes []string, ttl time.Duration) (string, int64, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", 0, errors.New("token name is required")
	}
	sc, err := ParseScopes(scopes)
	if err != nil {
		return "", 0, err
	}
	if ttl == 0 {
		ttl = DefaultTokenTTL
	}
	if ttl < 0 || ttl > MaxTokenTTL {
		return "", 0, fmt.Errorf("ttl must be between 1s and %s", MaxTokenTTL)
	}
	buf := make([]byte, tokenRandBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", 0, err
	}
	plain := tokenPrefix + base64.RawURLEncoding.EncodeToString(buf)
	now := time.Now()
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO admin_tokens(name, token_hash, scopes, created_at, expires_at) VALUES(?,?,?,?,?)`,
		name, hashToken(plain), strings.Join(sc, ","), now.Unix(), now.Add(ttl).Unix())
	if err != nil {
		return "", 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return "", 0, err
	}
	return plain, id, nil
}

type rowScanner interface{ Scan(...any) error }

func scanToken(r rowScanner) (*Token, error) {
	var t Token
	var scopes string
	var created, expires int64
	var used, revoked *int64
	if err := r.Scan(&t.ID, &t.Name, &scopes, &created, &expires, &used, &revoked); err != nil {
		return nil, err
	}
	if scopes != "" {
		t.Scopes = strings.Split(scopes, ",")
	}
	t.CreatedAt, t.ExpiresAt = time.Unix(created, 0), time.Unix(expires, 0)
	if used != nil {
		v := time.Unix(*used, 0)
		t.LastUsedAt = &v
	}
	if revoked != nil {
		v := time.Unix(*revoked, 0)
		t.RevokedAt = &v
	}
	return &t, nil
}

const tokenCols = `id, name, scopes, created_at, expires_at, last_used_at, revoked_at`

// VerifyToken resolves a plain token. Unknown, malformed, expired and revoked tokens all yield
// ErrInvalidToken. last_used_at is refreshed in the background, at most once a minute.
func (s *Store) VerifyToken(ctx context.Context, plain string) (*Token, error) {
	if !strings.HasPrefix(plain, tokenPrefix) || len(plain) > 128 {
		return nil, ErrInvalidToken
	}
	h := hashToken(plain)
	t, err := scanToken(s.db.QueryRowContext(ctx, `SELECT `+tokenCols+` FROM admin_tokens WHERE token_hash = ?`, h))
	if err != nil {
		// Unknown tokens and real DB errors stay distinct, but unknown maps to ErrInvalidToken.
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrInvalidToken
		}
		return nil, err
	}
	now := time.Now()
	if t.Status(now) != "active" {
		return nil, ErrInvalidToken
	}
	if t.LastUsedAt == nil || now.Sub(*t.LastUsedAt) >= lastUsedGrain {
		id := t.ID
		go func() {
			c, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_, _ = s.db.ExecContext(c, `UPDATE admin_tokens SET last_used_at = ? WHERE id = ?`, now.Unix(), id)
		}()
		v := now
		t.LastUsedAt = &v
	}
	return t, nil
}

// RevokeToken marks a token revoked (idempotent). It returns an error for an unknown id.
func (s *Store) RevokeToken(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `UPDATE admin_tokens SET revoked_at = COALESCE(revoked_at, ?) WHERE id = ?`, time.Now().Unix(), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("token %d not found", id)
	}
	return nil
}

// ListTokens returns every token's metadata, newest first. No hash, no secret.
func (s *Store) ListTokens(ctx context.Context) ([]Token, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+tokenCols+` FROM admin_tokens ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Token
	for rows.Next() {
		t, err := scanToken(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}
