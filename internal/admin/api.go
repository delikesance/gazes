package admin

import (
	"context"
	"database/sql"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
)

// Service is the admin read API: the admin store, a read-only view of the accounts
// database and the rollup job. Handlers live in api_*.go files and reach the databases
// through adminDB and accountsDB only.
type Service struct {
	store    *Store
	rollup   *Rollup
	now      func() time.Time
	identify func(*http.Request) (int64, bool)
	users    UserLookup
	ops      opsState
	watchCfg WatchConfig
	watchMu  sync.Mutex        // one evaluation of the watch rules at a time
	toolsFn  func() []ToolInfo // MCP tool catalogue, set by the API layer (nil = none)

	mu       sync.Mutex
	tokenMWs map[string]func(http.Handler) http.Handler
}

// ServiceOption customises a Service.
type ServiceOption func(*Service)

// WithClock injects the clock (tests).
func WithClock(now func() time.Time) ServiceOption {
	return func(s *Service) {
		if now != nil {
			s.now = now
		}
	}
}

// WithSessionAuth sets how admin sessions are identified and how roles are looked up.
func WithSessionAuth(identify func(*http.Request) (int64, bool), users UserLookup) ServiceOption {
	return func(s *Service) { s.identify, s.users = identify, users }
}

// NewService builds a Service over store and the accounts database file accountsPath
// (accounts.sqlite, which must exist; it is opened read-only).
func NewService(store *Store, accountsPath string, opts ...ServiceOption) (*Service, error) {
	ro, err := NewRollup(store, accountsPath)
	if err != nil {
		return nil, err
	}
	s := &Service{store: store, rollup: ro, now: time.Now, tokenMWs: map[string]func(http.Handler) http.Handler{}}
	for _, o := range opts {
		o(s)
	}
	ro.now = s.now
	return s, nil
}

// SetSessionAuth is WithSessionAuth for a Service that already exists (called by the server
// right before Mount). Not safe once requests are being served.
func (s *Service) SetSessionAuth(identify func(*http.Request) (int64, bool), users UserLookup) {
	s.identify, s.users = identify, users
}

// Rollup returns the metrics rollup job.
func (s *Service) Rollup() *Rollup { return s.rollup }

// Store returns the admin store (token management, used by the CLI and later milestones).
func (s *Service) Store() *Store { return s.store }

// adminDB is the admin database (metrics_*, playback_errors, issues...), read/write.
func (s *Service) adminDB() *sql.DB { return s.store.db }

// accountsDB is the accounts database (users, watch_sessions), opened read-only
// (mode=ro): any write fails. Handlers must never return emails, hashes or pseudos in
// aggregates.
func (s *Service) accountsDB() *sql.DB { return s.rollup.accounts }

// BackfillIfEmpty recomputes the last days days when metrics_daily has no row yet.
func (s *Service) BackfillIfEmpty(ctx context.Context, days int) error {
	var n int
	if err := s.adminDB().QueryRowContext(ctx, `SELECT COUNT(*) FROM metrics_daily`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	return s.rollup.Backfill(ctx, s.now(), days)
}

// Close closes the read-only accounts connection. The admin Store is closed by its opener.
func (s *Service) Close() error { return s.rollup.Close() }

// Mount registers the admin API under /api/v1/admin on r (a root router).
func (s *Service) Mount(r chi.Router) {
	r.Route("/api/v1/admin", func(r chi.Router) {
		r.With(s.Auth(ScopeMetricsRead)).Get("/me", s.handleMe)
		s.mountViews(r)
		s.mountUsers(r)
		s.mountPlayback(r)
		s.mountOps(r)
		s.mountSettings(r)
		s.mountWatch(r)
	})
}

type authInfo struct {
	via    string
	scopes []string
}

type authKey struct{}

var allScopes = []string{ScopeConfigWrite, ScopeDiagnosticsRead, ScopeMetricsRead, ScopeOpsWrite}

// Auth guards a route that needs scope (always give one, even for session-only data).
//
//   - Authorization header present: Bearer-token path only (RequireTokenWith, with the
//     scope, a per-token rate limit shared across routes). It NEVER falls back to the
//     session cookie: an invalid or missing-scope token is 401/403 even when the request
//     also carries a valid admin cookie.
//   - No Authorization header: admin session (RequireAdminSession: 401 not logged in,
//     403 not admin). A logged-in admin implicitly holds every scope, including ops:write
//     and config:write, because that is a human.
func (s *Service) Auth(scope string) func(http.Handler) http.Handler {
	s.mu.Lock()
	tokenMW, ok := s.tokenMWs[scope]
	if !ok {
		tokenMW = RequireTokenWith(s.store, TokenOptions{Now: s.now}, scope)
		s.tokenMWs[scope] = tokenMW
	}
	s.mu.Unlock()
	return func(next http.Handler) http.Handler {
		viaToken := tokenMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			info := authInfo{via: "token"}
			if t, ok := TokenFromContext(r.Context()); ok {
				info.scopes = t.Scopes
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), authKey{}, info)))
		}))
		viaSession := RequireAdminSession(s.sessionIdentify, sessionUsers{s})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// A cookie is ambient authority: state-changing requests must carry a custom header,
			// which a cross-site form or simple fetch cannot add (CSRF defence).
			if !isSafeMethod(r.Method) && r.Header.Get("X-Gazes-Admin") != "1" {
				writeAPIError(w, http.StatusForbidden, "csrf", "missing X-Gazes-Admin header")
				return
			}
			info := authInfo{via: "session", scopes: allScopes}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), authKey{}, info)))
		}))
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "" {
				viaToken.ServeHTTP(w, r)
				return
			}
			viaSession.ServeHTTP(w, r)
		})
	}
}

func (s *Service) sessionIdentify(r *http.Request) (int64, bool) {
	if s.identify == nil {
		return 0, false
	}
	return s.identify(r)
}

// sessionUsers reads the role lookup at request time (it may be set after Auth is called).
type sessionUsers struct{ s *Service }

func (u sessionUsers) UserRole(ctx context.Context, id int64) (string, error) {
	if u.s.users == nil {
		return "", sql.ErrNoRows
	}
	return u.s.users.UserRole(ctx, id)
}

func (s *Service) handleMe(w http.ResponseWriter, r *http.Request) {
	info, _ := r.Context().Value(authKey{}).(authInfo)
	out := map[string]any{"via": info.via, "scopes": append([]string{}, info.scopes...)}
	if info.via == "session" {
		out["role"] = roleAdmin
	}
	writeDataNoPeriod(w, out)
}

func isSafeMethod(m string) bool {
	return m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions
}
