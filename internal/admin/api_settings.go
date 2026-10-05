package admin

import (
	"net/http"
	"sort"
	"time"

	"github.com/go-chi/chi/v5"
)

// ToolInfo describes one MCP tool for the panel's catalogue. It carries no argument values.
type ToolInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Level       string `json:"level"`
	Scope       string `json:"scope"`
	Method      string `json:"method"`
	Path        string `json:"path"`
	Summary     string `json:"summary"`
}

// SetToolCatalogue gives the service the MCP tool table (the mcp package imports admin, so the
// API layer injects it). It is read at request time.
func (s *Service) SetToolCatalogue(fn func() []ToolInfo) {
	s.mu.Lock()
	s.toolsFn = fn
	s.mu.Unlock()
}

func (s *Service) mountSettings(r chi.Router) {
	diag := s.Auth(ScopeDiagnosticsRead)
	r.With(diag).Get("/mcp/tools", s.handleMCPTools)
	r.With(diag).Get("/mcp/audit", s.handleMCPAudit)
	r.With(diag).Get("/settings", s.handleSettings)
	// Token metadata, creation and revocation are for humans (admin session); the local gazes-admin CLI does the same.
	r.With(s.sessionOnlyRead).Get("/tokens", s.handleTokensList)
	s.mountTokens(r)
}

// sessionOnlyRead is sessionOnly for a GET: a Bearer token is refused whatever its scopes.
func (s *Service) sessionOnlyRead(next http.Handler) http.Handler {
	guarded := s.Auth(ScopeConfigWrite)(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			writeAPIError(w, http.StatusForbidden, "session_required", "this view needs an admin session, tokens cannot read it")
			return
		}
		guarded.ServeHTTP(w, r)
	})
}

// handleMCPTools: GET /mcp/tools. Feeds the "Claude et MCP" page (tool catalogue).
func (s *Service) handleMCPTools(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	fn := s.toolsFn
	s.mu.Unlock()
	items := []ToolInfo{}
	if fn != nil {
		items = append(items, fn()...)
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	writeDataNoPeriod(w, map[string]any{"enabled": fn != nil, "items": items})
}

var auditOutcomes = map[string]bool{"ok": true, "error": true, "denied": true, "executed": true, "pending": true,
	"approved": true, "rejected": true, "undone": true, "suspended": true, "budget_exceeded": true, "dry_run": true}

// handleMCPAudit: GET /mcp/audit?outcome=&limit=&offset=. Feeds the call log of "Claude et MCP".
// Arguments were redacted and truncated when written.
func (s *Service) handleMCPAudit(w http.ResponseWriter, r *http.Request) {
	pg, err := parsePagination(r)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "bad_pagination", err.Error())
		return
	}
	outcome := r.URL.Query().Get("outcome")
	if outcome != "" && !auditOutcomes[outcome] {
		writeAPIError(w, http.StatusBadRequest, "bad_outcome", "unknown outcome")
		return
	}
	var total int64
	if err := s.adminDB().QueryRowContext(r.Context(), `SELECT COUNT(*) FROM mcp_audit WHERE (? = '' OR outcome = ?)`, outcome, outcome).Scan(&total); err != nil {
		pbServerError(w, err)
		return
	}
	rows, err := s.adminDB().QueryContext(r.Context(),
		`SELECT id, ts, token_id, tool, COALESCE(args_summary,''), outcome, COALESCE(duration_ms,0), approval_id
		 FROM mcp_audit WHERE (? = '' OR outcome = ?) ORDER BY id DESC LIMIT ? OFFSET ?`, outcome, outcome, pg.Limit, pg.Offset)
	if err != nil {
		pbServerError(w, err)
		return
	}
	defer rows.Close()
	type row struct {
		ID          int64  `json:"id"`
		TS          string `json:"ts"`
		TokenID     *int64 `json:"token_id"`
		Tool        string `json:"tool"`
		ArgsSummary string `json:"args_summary"`
		Outcome     string `json:"outcome"`
		DurationMS  int64  `json:"duration_ms"`
		ApprovalID  *int64 `json:"approval_id"`
	}
	items := []row{}
	for rows.Next() {
		var x row
		var ts int64
		if err := rows.Scan(&x.ID, &ts, &x.TokenID, &x.Tool, &x.ArgsSummary, &x.Outcome, &x.DurationMS, &x.ApprovalID); err != nil {
			pbServerError(w, err)
			return
		}
		x.TS = time.Unix(ts, 0).UTC().Format(time.RFC3339)
		items = append(items, x)
	}
	if err := rows.Err(); err != nil {
		pbServerError(w, err)
		return
	}
	writeDataNoPeriod(w, map[string]any{"total": total, "limit": pg.Limit, "offset": pg.Offset, "items": items})
}

// handleSettings: GET /settings. Alert thresholds with their bounds, the reversible-action budget
// and the kill switch. Feeds "Paramètres". Thresholds change only through the approval flow.
func (s *Service) handleSettings(w http.ResponseWriter, r *http.Request) {
	type threshold struct {
		Rule    string  `json:"rule"`
		Value   float64 `json:"value"`
		Default float64 `json:"default"`
		Min     float64 `json:"min"`
		Max     float64 `json:"max"`
	}
	rules := make([]string, 0, len(alertRules))
	for k := range alertRules {
		rules = append(rules, k)
	}
	sort.Strings(rules)
	th := []threshold{}
	for _, k := range rules {
		v, err := s.AlertThreshold(r.Context(), k)
		if err != nil {
			pbServerError(w, err)
			return
		}
		a := alertRules[k]
		th = append(th, threshold{k, v, a.Default, a.Min, a.Max})
	}
	ks, err := s.killSwitchState(r.Context())
	if err != nil {
		pbServerError(w, err)
		return
	}
	writeDataNoPeriod(w, map[string]any{
		"thresholds":                 th,
		"reversible_budget_per_hour": s.reversibleBudget(r.Context()),
		"kill_switch":                ks,
	})
}

// handleTokensList: GET /tokens (session only). Metadata only: never a hash or a secret.
func (s *Service) handleTokensList(w http.ResponseWriter, r *http.Request) {
	toks, err := s.store.ListTokens(r.Context())
	if err != nil {
		pbServerError(w, err)
		return
	}
	type item struct {
		ID         int64    `json:"id"`
		Name       string   `json:"name"`
		Scopes     []string `json:"scopes"`
		CreatedAt  string   `json:"created_at"`
		ExpiresAt  string   `json:"expires_at"`
		LastUsedAt *string  `json:"last_used_at"`
		Status     string   `json:"status"`
	}
	now := s.now()
	items := []item{}
	for _, t := range toks {
		it := item{ID: t.ID, Name: t.Name, Scopes: t.Scopes,
			CreatedAt: t.CreatedAt.UTC().Format(time.RFC3339), ExpiresAt: t.ExpiresAt.UTC().Format(time.RFC3339), Status: t.Status(now)}
		if t.LastUsedAt != nil {
			v := t.LastUsedAt.UTC().Format(time.RFC3339)
			it.LastUsedAt = &v
		}
		items = append(items, it)
	}
	writeDataNoPeriod(w, map[string]any{"items": items})
}
