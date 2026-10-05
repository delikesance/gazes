package admin

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

const (
	opsMaxText      = 1000 // justification and expected_effect
	opsMaxReason    = 500  // kill switch reason
	idemRetention   = 7 * 24 * time.Hour
	approvalPending = "pending"
)

var (
	idemKeyPattern   = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,64}$`)
	approvalStatuses = map[string]bool{"pending": true, "approved": true, "executed": true, "rejected": true, "failed": true, "undone": true}
)

// mountOps registers the actions, approvals and kill switch routes ("Claude et MCP" page).
//
//   - Token routes: GET /approvals, GET /approvals/{id}, GET /ops/kill-switch, GET /ops/actions
//     (diagnostics:read) and POST /ops/actions/{name} (ops:write for reversible actions,
//     config:write for sensitive ones). All writes by token are refused with 423 while the kill
//     switch is on.
//   - Session-only routes (a bearer token is refused with 403 even with every scope, a session
//     must carry X-Gazes-Admin): approve, reject, undo and PUT /ops/kill-switch.
func (s *Service) mountOps(r chi.Router) {
	diag := s.Auth(ScopeDiagnosticsRead)
	r.With(diag).Get("/approvals", s.handleApprovalsList)
	r.With(diag).Get("/approvals/{id}", s.handleApprovalGet)
	r.With(diag).Get("/ops/kill-switch", s.handleKillSwitchGet)
	r.With(diag).Get("/ops/actions", s.handleActionsCatalogue)

	r.With(s.sessionOnly).Post("/approvals/{id}/approve", s.handleApprovalApprove)
	r.With(s.sessionOnly).Post("/approvals/{id}/reject", s.handleApprovalReject)
	r.With(s.sessionOnly).Post("/approvals/{id}/undo", s.handleApprovalUndo)
	r.With(s.sessionOnly).Put("/ops/kill-switch", s.handleKillSwitchPut)

	for _, a := range s.actionList() {
		r.With(s.Auth(a.Scope)).Post("/ops/actions/"+a.Name, s.handleAction(a.Name))
	}
}

// sessionOnly lets through only a logged-in admin session carrying the CSRF header. Any
// Authorization header is refused: a token, whatever its scopes, can never decide.
func (s *Service) sessionOnly(next http.Handler) http.Handler {
	guarded := s.Auth(ScopeConfigWrite)(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			writeAPIError(w, http.StatusForbidden, "session_required", "this operation needs an admin session, tokens cannot perform it")
			return
		}
		guarded.ServeHTTP(w, r)
	})
}

// actorOf identifies the caller: ("token:<id>", id) for a token, ("user:<id>", 0) for a session.
func (s *Service) actorOf(r *http.Request) (string, int64) {
	if t, ok := TokenFromContext(r.Context()); ok {
		return "token:" + strconv.FormatInt(t.ID, 10), t.ID
	}
	id, _ := s.sessionIdentify(r)
	return "user:" + strconv.FormatInt(id, 10), 0
}

func writeActionErr(w http.ResponseWriter, err error) {
	var ae *ActionError
	if errors.As(err, &ae) {
		writeAPIError(w, ae.Status, ae.Code, ae.Msg)
		return
	}
	slog.Warn("admin action failed", "err", err)
	writeAPIError(w, http.StatusInternalServerError, "internal", "internal error")
}

func writeOpsData(w http.ResponseWriter, status int, data any) {
	writeJSON(w, status, envelope{GeneratedAt: time.Now().UTC().Format(time.RFC3339), Data: data})
}

// argsSummary is the audited form of action args: sensitive keys redacted, sorted, bounded.
func opsArgsSummary(args map[string]any) string {
	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		if opsSensitiveKey.MatchString(k) {
			parts = append(parts, k+"=[redacted]")
			continue
		}
		v, _ := json.Marshal(args[k])
		parts = append(parts, k+"="+string(v))
	}
	r := []rune(strings.Join(parts, " "))
	if len(r) > 200 {
		r = r[:200]
	}
	return string(r)
}

func argsHash(name string, args map[string]any) string {
	b, _ := json.Marshal(args) // map keys are sorted: canonical
	h := sha256.Sum256(append([]byte(name+"\x00"), b...))
	return hex.EncodeToString(h[:])
}

// ---- actions --------------------------------------------------------------------------------

type actionBody struct {
	Args           map[string]any `json:"args"`
	DryRun         *bool          `json:"dry_run"`
	IdempotencyKey string         `json:"idempotency_key"`
	Justification  string         `json:"justification"`
	ExpectedEffect string         `json:"expected_effect"`
}

// handleActionsCatalogue: GET /ops/actions (diagnostics:read) — the registry, including declared
// but not implemented actions.
func (s *Service) handleActionsCatalogue(w http.ResponseWriter, r *http.Request) {
	type item struct {
		Name        string `json:"name"`
		Level       string `json:"level"`
		Scope       string `json:"scope"`
		Summary     string `json:"summary"`
		Implemented bool   `json:"implemented"`
	}
	items := []item{}
	for _, a := range s.actionList() {
		items = append(items, item{a.Name, a.Level, a.Scope, a.Summary, a.Implemented})
	}
	writeDataNoPeriod(w, map[string]any{"items": items, "reversible_budget_per_hour": s.reversibleBudget(r.Context())})
}

// handleAction: POST /ops/actions/{name} (ops:write or config:write by level). dry_run defaults
// to true. A reversible action with dry_run=false runs (budget, kill switch, audit) and returns an
// undo token; a sensitive one never runs here: it creates a pending approval.
func (s *Service) handleAction(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a, ok := s.lookupAction(name)
		if !ok {
			writeAPIError(w, http.StatusNotFound, "not_found", "unknown action")
			return
		}
		tool := "action:" + name
		actor, tokenID := s.actorOf(r)
		info, _ := r.Context().Value(authKey{}).(authInfo)
		viaToken := info.via == "token"
		if viaToken && s.WriteSuspended(r.Context()) {
			s.opsAudit(r.Context(), tokenID, tool, "", "suspended", 0)
			writeAPIError(w, http.StatusLocked, "write_suspended", "writes are suspended by the kill switch")
			return
		}
		var in actionBody
		if !issueDecode(w, r, &in) {
			return
		}
		if in.Args == nil {
			in.Args = map[string]any{}
		}
		summary := opsArgsSummary(in.Args)
		fail := func(err error) {
			s.opsAudit(r.Context(), tokenID, tool, summary, "error", 0)
			writeActionErr(w, err)
		}
		if in.IdempotencyKey != "" && !idemKeyPattern.MatchString(in.IdempotencyKey) {
			fail(badArgs("idempotency_key must match [A-Za-z0-9._:-]{1,64}"))
			return
		}
		if !issueFieldOK(in.Justification, opsMaxText) || !issueFieldOK(in.ExpectedEffect, opsMaxText) {
			fail(badArgs("justification and expected_effect are limited to %d characters", opsMaxText))
			return
		}
		if err := a.Validate(in.Args); err != nil {
			fail(err)
			return
		}
		ctx := withActor(r.Context(), actor)
		plan, err := a.Plan(ctx, in.Args)
		if err != nil {
			fail(err)
			return
		}
		dryRun := in.DryRun == nil || *in.DryRun
		if dryRun {
			s.opsAudit(r.Context(), tokenID, tool, summary, "dry_run", 0)
			writeOpsData(w, http.StatusOK, map[string]any{"status": "dry_run", "action": name, "level": a.Level, "plan": plan})
			return
		}

		s.ops.mu.Lock()
		defer s.ops.mu.Unlock()
		hash := argsHash(name, in.Args)
		if in.IdempotencyKey != "" {
			code, body, replay, err := s.idemLookup(ctx, tokenID, in.IdempotencyKey, tool, hash)
			if err != nil {
				fail(err)
				return
			}
			if replay {
				body["idempotent_replay"] = true
				writeOpsData(w, code, body)
				return
			}
		}
		argsJSON, _ := json.Marshal(in.Args)
		planJSON, _ := json.Marshal(plan)
		var (
			status int
			resp   map[string]any
		)
		switch a.Level {
		case ActionSensitive:
			if strings.TrimSpace(in.Justification) == "" || strings.TrimSpace(in.ExpectedEffect) == "" {
				fail(badArgs("justification and expected_effect are required for a sensitive action"))
				return
			}
			id, err := s.insertApproval(ctx, approvalRow{Status: approvalPending, Tool: name, Args: string(argsJSON), Justification: strings.TrimSpace(in.Justification),
				ExpectedEffect: strings.TrimSpace(in.ExpectedEffect), Plan: string(planJSON), RequestedBy: tokenID})
			if err != nil {
				fail(err)
				return
			}
			s.opsAudit(r.Context(), tokenID, tool, summary, "pending", id)
			status, resp = http.StatusAccepted, map[string]any{"status": "pending_approval", "approval_id": id, "action": name, "level": a.Level, "plan": plan}
		default:
			if viaToken {
				budget := s.reversibleBudget(ctx)
				used, err := s.reversibleUsed(ctx, tokenID)
				if err != nil {
					fail(err)
					return
				}
				if used >= budget {
					s.opsAudit(r.Context(), tokenID, tool, summary, "budget_exceeded", 0)
					w.Header().Set("Retry-After", "3600")
					writeAPIError(w, http.StatusTooManyRequests, "budget_exceeded", "hourly budget of reversible actions exhausted ("+strconv.Itoa(budget)+" per token)")
					return
				}
			}
			res, undo, err := a.Do(ctx, in.Args)
			if err != nil {
				fail(err)
				return
			}
			resJSON, _ := json.Marshal(res)
			id, err := s.insertApproval(ctx, approvalRow{Status: "executed", Tool: name, Args: string(argsJSON), Justification: strings.TrimSpace(in.Justification),
				ExpectedEffect: strings.TrimSpace(in.ExpectedEffect), Plan: string(planJSON), RequestedBy: tokenID, DecidedBy: actor, Result: string(resJSON), UndoToken: undo})
			if err != nil {
				if undo != "" && a.Undo != nil {
					_ = a.Undo(ctx, undo) // no record of the effect: take it back
				}
				fail(err)
				return
			}
			s.opsAudit(r.Context(), tokenID, tool, summary, "executed", id)
			status, resp = http.StatusOK, map[string]any{"status": "executed", "action": name, "level": a.Level, "action_id": id, "result": res}
			if undo != "" {
				resp["undo_token"] = undo
			}
		}
		if in.IdempotencyKey != "" {
			s.idemStore(ctx, tokenID, in.IdempotencyKey, tool, hash, status, resp)
		}
		writeOpsData(w, status, resp)
	}
}

// ---- idempotency ----------------------------------------------------------------------------
//
// A ledger table keyed (token, key) stores the full response of the first execution; the same key
// with the same arguments returns it without running again, the same key with other arguments is
// refused (409). A ledger is used rather than a column on approvals because reversible actions
// must be covered too, and replaying needs the original HTTP status and body (pending approval,
// undo token...). The execution path is serialised by opsState.mu, so a key cannot run twice.

func (s *Service) idemLookup(ctx context.Context, tokenID int64, key, tool, hash string) (int, map[string]any, bool, error) {
	var gotTool, gotHash, resp string
	var code int
	err := s.adminDB().QueryRowContext(ctx,
		`SELECT tool, args_hash, http_status, response FROM action_idempotency WHERE token_id = ? AND key = ?`, tokenID, key).Scan(&gotTool, &gotHash, &code, &resp)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil, false, nil
	}
	if err != nil {
		return 0, nil, false, err
	}
	if gotTool != tool || gotHash != hash {
		return 0, nil, false, &ActionError{Status: http.StatusConflict, Code: "idempotency_conflict", Msg: "this idempotency_key was already used with a different action or arguments"}
	}
	var body map[string]any
	if json.Unmarshal([]byte(resp), &body) != nil {
		return 0, nil, false, errors.New("corrupt idempotency record")
	}
	return code, body, true, nil
}

func (s *Service) idemStore(ctx context.Context, tokenID int64, key, tool, hash string, code int, resp map[string]any) {
	b, _ := json.Marshal(resp)
	now := s.now().Unix()
	if _, err := s.adminDB().ExecContext(ctx,
		`INSERT OR IGNORE INTO action_idempotency (token_id, key, tool, args_hash, http_status, response, created_at) VALUES (?,?,?,?,?,?,?)`,
		tokenID, key, tool, hash, code, string(b), now); err != nil {
		slog.Warn("admin idempotency write failed", "err", err)
	}
	_, _ = s.adminDB().ExecContext(ctx, `DELETE FROM action_idempotency WHERE created_at < ?`, now-int64(idemRetention.Seconds()))
}

// ---- approvals ------------------------------------------------------------------------------

type approvalRow struct {
	Status, Tool, Args, Justification, ExpectedEffect, Plan, DecidedBy, Result, UndoToken string
	RequestedBy                                                                           int64
}

func (s *Service) insertApproval(ctx context.Context, a approvalRow) (int64, error) {
	now := s.now().Unix()
	var decidedAt, executedAt any
	if a.DecidedBy != "" {
		decidedAt, executedAt = now, now
	}
	var requestedBy any
	if a.RequestedBy != 0 {
		requestedBy = a.RequestedBy
	}
	res, err := s.adminDB().ExecContext(ctx,
		`INSERT INTO approvals (created_at, tool, args, justification, expected_effect, status, decided_by, decided_at, undo_token, requested_by, plan, result, executed_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		now, a.Tool, a.Args, nullStr(a.Justification), nullStr(a.ExpectedEffect), a.Status, nullStr(a.DecidedBy), decidedAt, nullStr(a.UndoToken),
		requestedBy, nullStr(a.Plan), nullStr(a.Result), executedAt)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

type approvalView struct {
	ID             int64           `json:"id"`
	CreatedAt      string          `json:"created_at"`
	Tool           string          `json:"tool"`
	Args           json.RawMessage `json:"args"`
	Justification  *string         `json:"justification"`
	ExpectedEffect *string         `json:"expected_effect"`
	Plan           json.RawMessage `json:"plan"`
	Status         string          `json:"status"`
	RequestedBy    *int64          `json:"requested_by_token"`
	DecidedBy      *string         `json:"decided_by"`
	DecidedAt      *string         `json:"decided_at"`
	ExecutedAt     *string         `json:"executed_at"`
	Result         json.RawMessage `json:"result"`
	Undoable       bool            `json:"undoable"`
}

const approvalColumns = `id, created_at, tool, args, justification, expected_effect, plan, status, requested_by, decided_by, decided_at, executed_at, result, COALESCE(undo_token,'')`

func scanApproval(r issueRowScanner) (approvalView, error) {
	var v approvalView
	var created int64
	var just, exp, plan, decBy, result sql.NullString
	var reqBy, decAt, exAt sql.NullInt64
	var args, undo string
	if err := r.Scan(&v.ID, &created, &v.Tool, &args, &just, &exp, &plan, &v.Status, &reqBy, &decBy, &decAt, &exAt, &result, &undo); err != nil {
		return v, err
	}
	ts := func(n int64) *string { t := time.Unix(n, 0).UTC().Format(time.RFC3339); return &t }
	v.CreatedAt = *ts(created)
	v.Args = json.RawMessage(args)
	if !json.Valid(v.Args) {
		v.Args = json.RawMessage(`null`)
	}
	str := func(n sql.NullString) *string {
		if !n.Valid {
			return nil
		}
		return &n.String
	}
	v.Justification, v.ExpectedEffect, v.DecidedBy = str(just), str(exp), str(decBy)
	if plan.Valid && json.Valid([]byte(plan.String)) {
		v.Plan = json.RawMessage(plan.String)
	}
	if result.Valid && json.Valid([]byte(result.String)) {
		v.Result = json.RawMessage(result.String)
	}
	if reqBy.Valid {
		v.RequestedBy = &reqBy.Int64
	}
	if decAt.Valid {
		v.DecidedAt = ts(decAt.Int64)
	}
	if exAt.Valid {
		v.ExecutedAt = ts(exAt.Int64)
	}
	v.Undoable = undo != "" && (v.Status == "approved" || v.Status == "executed")
	return v, nil
}

func approvalID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id < 1 {
		writeAPIError(w, http.StatusBadRequest, "bad_id", "id must be a positive integer")
		return 0, false
	}
	return id, true
}

func (s *Service) loadApproval(ctx context.Context, id int64) (approvalView, string, error) {
	row := s.adminDB().QueryRowContext(ctx, `SELECT `+approvalColumns+` FROM approvals WHERE id = ?`, id)
	// the undo token is read separately: it is never part of the view
	v, err := scanApproval(row)
	if errors.Is(err, sql.ErrNoRows) {
		return v, "", notFound("approval")
	}
	if err != nil {
		return v, "", err
	}
	var undo string
	_ = s.adminDB().QueryRowContext(ctx, `SELECT COALESCE(undo_token,'') FROM approvals WHERE id = ?`, id).Scan(&undo)
	return v, undo, nil
}

// handleApprovalsList: GET /approvals?status=&limit=&offset= (diagnostics:read).
func (s *Service) handleApprovalsList(w http.ResponseWriter, r *http.Request) {
	pg, err := parsePagination(r)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "bad_pagination", err.Error())
		return
	}
	status := r.URL.Query().Get("status")
	if status != "" && !approvalStatuses[status] {
		writeAPIError(w, http.StatusBadRequest, "bad_status", "status must be pending, approved, executed, rejected, failed or undone")
		return
	}
	var total int64
	if err := s.adminDB().QueryRowContext(r.Context(), `SELECT COUNT(*) FROM approvals WHERE (? = '' OR status = ?)`, status, status).Scan(&total); err != nil {
		pbServerError(w, err)
		return
	}
	rows, err := s.adminDB().QueryContext(r.Context(),
		`SELECT `+approvalColumns+` FROM approvals WHERE (? = '' OR status = ?) ORDER BY id DESC LIMIT ? OFFSET ?`, status, status, pg.Limit, pg.Offset)
	if err != nil {
		pbServerError(w, err)
		return
	}
	defer rows.Close()
	items := []approvalView{}
	for rows.Next() {
		v, err := scanApproval(rows)
		if err != nil {
			pbServerError(w, err)
			return
		}
		items = append(items, v)
	}
	if err := rows.Err(); err != nil {
		pbServerError(w, err)
		return
	}
	writeDataNoPeriod(w, map[string]any{"items": items, "page": map[string]any{"limit": pg.Limit, "offset": pg.Offset, "total": total}})
}

// handleApprovalGet: GET /approvals/{id} (diagnostics:read) — lets Claude follow a decision.
func (s *Service) handleApprovalGet(w http.ResponseWriter, r *http.Request) {
	id, ok := approvalID(w, r)
	if !ok {
		return
	}
	v, _, err := s.loadApproval(r.Context(), id)
	if err != nil {
		writeActionErr(w, err)
		return
	}
	writeDataNoPeriod(w, v)
}

// decide runs the common prologue of approve/reject: id, then the atomic pending -> status move.
// It answers the error itself and returns false when the approval was not claimed.
func (s *Service) claimPending(w http.ResponseWriter, r *http.Request, id int64, to string) bool {
	by, _ := s.actorOf(r)
	res, err := s.adminDB().ExecContext(r.Context(),
		`UPDATE approvals SET status = ?, decided_by = ?, decided_at = ? WHERE id = ? AND status = 'pending'`, to, by, s.now().Unix(), id)
	if err != nil {
		pbServerError(w, err)
		return false
	}
	if n, _ := res.RowsAffected(); n == 1 {
		return true
	}
	if _, _, err := s.loadApproval(r.Context(), id); err != nil {
		writeActionErr(w, err)
		return false
	}
	writeAPIError(w, http.StatusConflict, "already_decided", "this approval was already decided")
	return false
}

// handleApprovalApprove: POST /approvals/{id}/approve — SESSION ONLY. Executes the stored action
// exactly once; refused when already decided or while the kill switch is on.
func (s *Service) handleApprovalApprove(w http.ResponseWriter, r *http.Request) {
	id, ok := approvalID(w, r)
	if !ok {
		return
	}
	if s.WriteSuspended(r.Context()) {
		s.opsAudit(r.Context(), 0, "approval:approve", "id="+strconv.FormatInt(id, 10), "suspended", id)
		writeAPIError(w, http.StatusLocked, "write_suspended", "writes are suspended by the kill switch")
		return
	}
	s.ops.mu.Lock()
	defer s.ops.mu.Unlock()
	if !s.claimPending(w, r, id, "approved") {
		return
	}
	actor, _ := s.actorOf(r)
	ctx := withActor(r.Context(), actor)
	v, _, err := s.loadApproval(ctx, id)
	if err != nil {
		writeActionErr(w, err)
		return
	}
	failApproval := func(err error) {
		msg := err.Error()
		var ae *ActionError
		if !errors.As(err, &ae) {
			msg = "internal error"
		}
		b, _ := json.Marshal(map[string]string{"error": msg})
		_, _ = s.adminDB().ExecContext(ctx, `UPDATE approvals SET status = 'failed', result = ? WHERE id = ?`, string(b), id)
		s.opsAudit(ctx, 0, "approval:approve", "id="+strconv.FormatInt(id, 10), "error", id)
		writeActionErr(w, err)
	}
	a, found := s.lookupAction(v.Tool)
	var args map[string]any
	if !found || json.Unmarshal(v.Args, &args) != nil {
		failApproval(badArgs("the stored action is no longer valid"))
		return
	}
	if err := a.Validate(args); err != nil {
		failApproval(err)
		return
	}
	res, undo, err := a.Do(ctx, args)
	if err != nil {
		failApproval(err)
		return
	}
	resJSON, _ := json.Marshal(res)
	if _, err := s.adminDB().ExecContext(ctx, `UPDATE approvals SET result = ?, undo_token = ?, executed_at = ? WHERE id = ?`, string(resJSON), nullStr(undo), s.now().Unix(), id); err != nil {
		slog.Warn("admin approval result not recorded", "id", id, "err", err)
	}
	s.opsAudit(ctx, 0, "approval:approve", "tool="+v.Tool, "approved", id)
	out, _, err := s.loadApproval(ctx, id)
	if err != nil {
		writeActionErr(w, err)
		return
	}
	writeDataNoPeriod(w, out)
}

// handleApprovalReject: POST /approvals/{id}/reject — SESSION ONLY. Optional body {"reason":string}.
func (s *Service) handleApprovalReject(w http.ResponseWriter, r *http.Request) {
	id, ok := approvalID(w, r)
	if !ok {
		return
	}
	var in struct {
		Reason string `json:"reason"`
	}
	if r.ContentLength != 0 {
		if !issueDecode(w, r, &in) {
			return
		}
		if !issueFieldOK(in.Reason, opsMaxReason) {
			writeAPIError(w, http.StatusBadRequest, "bad_reason", "reason is too long (max 500 characters)")
			return
		}
	}
	s.ops.mu.Lock()
	defer s.ops.mu.Unlock()
	if !s.claimPending(w, r, id, "rejected") {
		return
	}
	if in.Reason != "" {
		b, _ := json.Marshal(map[string]string{"reason": in.Reason})
		_, _ = s.adminDB().ExecContext(r.Context(), `UPDATE approvals SET result = ? WHERE id = ?`, string(b), id)
	}
	s.opsAudit(r.Context(), 0, "approval:reject", "id="+strconv.FormatInt(id, 10), "rejected", id)
	v, _, err := s.loadApproval(r.Context(), id)
	if err != nil {
		writeActionErr(w, err)
		return
	}
	writeDataNoPeriod(w, v)
}

// handleApprovalUndo: POST /approvals/{id}/undo — SESSION ONLY. Reverts an executed action (reversible
// ones that ran on their own and approved sensitive ones) with its stored undo token, once.
func (s *Service) handleApprovalUndo(w http.ResponseWriter, r *http.Request) {
	id, ok := approvalID(w, r)
	if !ok {
		return
	}
	s.ops.mu.Lock()
	defer s.ops.mu.Unlock()
	v, undo, err := s.loadApproval(r.Context(), id)
	if err != nil {
		writeActionErr(w, err)
		return
	}
	if !v.Undoable {
		writeAPIError(w, http.StatusConflict, "not_undoable", "this action cannot be undone (not executed, already undone or without undo token)")
		return
	}
	a, found := s.lookupAction(v.Tool)
	if !found || a.Undo == nil {
		writeAPIError(w, http.StatusConflict, "not_undoable", "this action cannot be undone")
		return
	}
	res, err := s.adminDB().ExecContext(r.Context(), `UPDATE approvals SET status = 'undone' WHERE id = ? AND status = ?`, id, v.Status)
	if err != nil {
		pbServerError(w, err)
		return
	}
	if n, _ := res.RowsAffected(); n != 1 {
		writeAPIError(w, http.StatusConflict, "not_undoable", "this action was already undone")
		return
	}
	actor, _ := s.actorOf(r)
	if err := a.Undo(withActor(r.Context(), actor), undo); err != nil {
		_, _ = s.adminDB().ExecContext(r.Context(), `UPDATE approvals SET status = ? WHERE id = ?`, v.Status, id)
		s.opsAudit(r.Context(), 0, "approval:undo", "tool="+v.Tool, "error", id)
		writeActionErr(w, err)
		return
	}
	s.opsAudit(r.Context(), 0, "approval:undo", "tool="+v.Tool, "undone", id)
	out, _, err := s.loadApproval(r.Context(), id)
	if err != nil {
		writeActionErr(w, err)
		return
	}
	writeDataNoPeriod(w, out)
}

// ---- kill switch ----------------------------------------------------------------------------

// handleKillSwitchGet: GET /ops/kill-switch (diagnostics:read).
func (s *Service) handleKillSwitchGet(w http.ResponseWriter, r *http.Request) {
	ks, err := s.killSwitchState(r.Context())
	if err != nil {
		pbServerError(w, err)
		return
	}
	writeDataNoPeriod(w, ks)
}

// handleKillSwitchPut: PUT /ops/kill-switch — SESSION ONLY, body {"suspended":bool,"reason":string}.
func (s *Service) handleKillSwitchPut(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Suspended *bool  `json:"suspended"`
		Reason    string `json:"reason"`
	}
	if !issueDecode(w, r, &in) {
		return
	}
	if in.Suspended == nil {
		writeAPIError(w, http.StatusBadRequest, "bad_suspended", "suspended (boolean) is required")
		return
	}
	if !issueFieldOK(in.Reason, opsMaxReason) {
		writeAPIError(w, http.StatusBadRequest, "bad_reason", "reason is too long (max 500 characters)")
		return
	}
	by, _ := s.actorOf(r)
	reason := strings.TrimSpace(in.Reason)
	if err := s.setKillSwitch(r.Context(), *in.Suspended, reason, by); err != nil {
		pbServerError(w, err)
		return
	}
	s.opsAudit(r.Context(), 0, "kill_switch", "suspended="+strconv.FormatBool(*in.Suspended), "executed", 0)
	ks, err := s.killSwitchState(r.Context())
	if err != nil {
		pbServerError(w, err)
		return
	}
	writeDataNoPeriod(w, ks)
}
