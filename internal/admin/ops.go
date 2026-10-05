package admin

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"sync"
	"time"
)

// Autonomy levels of an action (MCP design: "garde-fous"). Reversible actions run on their own,
// logged and budgeted; sensitive ones only run after a human approves them in the admin panel.
const (
	ActionReversible = "reversible"
	ActionSensitive  = "sensitive"
)

// Settings keys.
const (
	settingKillSwitch       = "kill_switch" // "on" | "off"
	settingKillSwitchReason = "kill_switch_reason"
	settingBudget           = "ops.reversible_budget_per_hour"
	settingThresholdPrefix  = "threshold."

	// DefaultReversibleBudget is the number of reversible actions one token may execute per hour.
	DefaultReversibleBudget = 30
)

// ActionError is an error an action reports to its caller with an HTTP status and a stable code.
type ActionError struct {
	Status int
	Code   string
	Msg    string
}

func (e *ActionError) Error() string { return e.Msg }

// ErrNotImplemented is returned by actions declared in the catalogue whose effect has no safe
// implementation yet. Such an action never has any effect.
var ErrNotImplemented = &ActionError{Status: http.StatusNotImplemented, Code: "not_implemented", Msg: "this action is declared but not implemented yet; it has no effect"}

func badArgs(format string, a ...any) error {
	return &ActionError{Status: http.StatusBadRequest, Code: "invalid_args", Msg: fmt.Sprintf(format, a...)}
}

func notFound(what string) error {
	return &ActionError{Status: http.StatusNotFound, Code: "not_found", Msg: what + " not found"}
}

// Plan describes what an action would do, computed without any effect (dry run).
type Plan struct {
	Summary string         `json:"summary"`
	Details map[string]any `json:"details,omitempty"`
}

// Result is what an executed action reports.
type Result map[string]any

// Action is one entry of the action registry.
type Action struct {
	Name        string
	Level       string // ActionReversible | ActionSensitive
	Scope       string // token scope required to call it: ops:write (reversible) or config:write (sensitive)
	Summary     string
	Implemented bool
	// Validate checks the arguments (types, bounds, allow-lists). It must not touch any state.
	Validate func(args map[string]any) error
	// Plan is the dry run: what would be done, with no effect.
	Plan func(ctx context.Context, args map[string]any) (Plan, error)
	// Do performs the action once and returns its result and an undo token ("" when not undoable).
	Do func(ctx context.Context, args map[string]any) (Result, string, error)
	// Undo reverts a previous Do from its undo token.
	Undo func(ctx context.Context, undoToken string) error
}

// opsState serialises the execution path of actions (idempotency check, budget check, effect,
// bookkeeping) so that two concurrent calls can neither exceed the budget nor run twice.
type opsState struct{ mu sync.Mutex }

type actorKey struct{}

// actorFrom returns who runs the current action: "token:<id>" or "user:<id>".
func actorFrom(ctx context.Context) string {
	a, _ := ctx.Value(actorKey{}).(string)
	if a == "" {
		return "system"
	}
	return a
}

func withActor(ctx context.Context, a string) context.Context {
	return context.WithValue(ctx, actorKey{}, a)
}

// lookupAction finds an action by name.
func (s *Service) lookupAction(name string) (Action, bool) {
	for _, a := range s.actionList() {
		if a.Name == name {
			return a, true
		}
	}
	return Action{}, false
}

// ---- settings -------------------------------------------------------------------------------

func (s *Service) getSetting(ctx context.Context, key string) (string, bool, error) {
	var v sql.NullString
	err := s.adminDB().QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return v.String, true, nil
}

func (s *Service) setSetting(ctx context.Context, key, value, by string) error {
	_, err := s.adminDB().ExecContext(ctx,
		`INSERT INTO settings (key, value, updated_at, updated_by) VALUES (?,?,?,?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at, updated_by = excluded.updated_by`,
		key, value, s.now().Unix(), by)
	return err
}

func (s *Service) deleteSetting(ctx context.Context, key string) error {
	_, err := s.adminDB().ExecContext(ctx, `DELETE FROM settings WHERE key = ?`, key)
	return err
}

// ---- kill switch ----------------------------------------------------------------------------

// WriteSuspended reports whether the kill switch is on: while it is, every write made with a
// token is refused. It fails closed: when the state cannot be read, writes are suspended.
func (s *Service) WriteSuspended(ctx context.Context) bool {
	v, _, err := s.getSetting(ctx, settingKillSwitch)
	if err != nil {
		slog.Warn("admin kill switch unreadable, failing closed", "err", err)
		return true
	}
	return v == "on"
}

// KillSwitch is the state returned by GET /ops/kill-switch.
type KillSwitch struct {
	Suspended bool    `json:"suspended"`
	Reason    string  `json:"reason"`
	UpdatedAt *string `json:"updated_at"`
	UpdatedBy *string `json:"updated_by"`
}

func (s *Service) killSwitchState(ctx context.Context) (KillSwitch, error) {
	ks := KillSwitch{}
	var v string
	var at sql.NullInt64
	var by sql.NullString
	err := s.adminDB().QueryRowContext(ctx, `SELECT COALESCE(value,''), updated_at, updated_by FROM settings WHERE key = ?`, settingKillSwitch).Scan(&v, &at, &by)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return ks, err
	}
	ks.Suspended = v == "on"
	if at.Valid {
		t := time.Unix(at.Int64, 0).UTC().Format(time.RFC3339)
		ks.UpdatedAt = &t
	}
	if by.Valid {
		ks.UpdatedBy = &by.String
	}
	if r, ok, err := s.getSetting(ctx, settingKillSwitchReason); err == nil && ok && ks.Suspended {
		ks.Reason = r
	}
	return ks, nil
}

func (s *Service) setKillSwitch(ctx context.Context, suspended bool, reason, by string) error {
	v := "off"
	if suspended {
		v = "on"
	}
	if err := s.setSetting(ctx, settingKillSwitch, v, by); err != nil {
		return err
	}
	return s.setSetting(ctx, settingKillSwitchReason, reason, by)
}

// refuseTokenWhenSuspended is a route middleware: while the kill switch is on, a request
// authenticated by token is answered 423 write_suspended. A human with a session is never blocked.
func (s *Service) refuseTokenWhenSuspended(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if info, _ := r.Context().Value(authKey{}).(authInfo); info.via == "token" && s.WriteSuspended(r.Context()) {
			var tid int64
			if t, ok := TokenFromContext(r.Context()); ok {
				tid = t.ID
			}
			s.opsAudit(r.Context(), tid, "http:"+r.Method+" "+r.URL.Path, "", "suspended", 0)
			writeAPIError(w, http.StatusLocked, "write_suspended", "writes are suspended by the kill switch")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ---- budget ---------------------------------------------------------------------------------

func (s *Service) reversibleBudget(ctx context.Context) int {
	if v, ok, err := s.getSetting(ctx, settingBudget); err == nil && ok {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			return n
		}
	}
	return DefaultReversibleBudget
}

// reversibleUsed counts the reversible actions a token executed in the last hour, from mcp_audit.
func (s *Service) reversibleUsed(ctx context.Context, tokenID int64) (int, error) {
	var n int
	err := s.adminDB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM mcp_audit WHERE token_id = ? AND outcome = 'executed' AND tool LIKE 'action:%' AND ts > ?`,
		tokenID, s.now().Add(-time.Hour).Unix()).Scan(&n)
	return n, err
}

// ---- audit ----------------------------------------------------------------------------------

var opsSensitiveKey = regexp.MustCompile(`(?i)token|secret|pass|auth|key|cookie|email|pseudo|credential`)

// opsAudit appends an admin-side audit row (state-significant events of the ops layer). Failures
// are logged, never returned.
func (s *Service) opsAudit(ctx context.Context, tokenID int64, tool, summary, outcome string, approvalID int64) {
	actx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	if err := s.store.RecordMCPAudit(actx, MCPAuditEntry{
		TS: s.now(), TokenID: tokenID, Tool: tool, ArgsSummary: summary, Outcome: outcome, ApprovalID: approvalID,
	}); err != nil {
		slog.Warn("admin ops audit write failed", "tool", tool, "err", err)
	}
}
