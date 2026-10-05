package admin

import (
	"context"
	"database/sql"
	"time"
)

// MCPAuditEntry is one row of mcp_audit: a single MCP tool call.
type MCPAuditEntry struct {
	TS          time.Time
	TokenID     int64 // 0 stores NULL (no token, e.g. a refused call)
	Tool        string
	ArgsSummary string // already redacted and truncated by the caller
	Outcome     string // ok | error | denied (MCP calls); executed | pending | approved | rejected | undone | suspended | budget_exceeded | dry_run (admin ops)
	DurationMS  int64
	ApprovalID  int64 // 0 stores NULL
}

// RecordMCPAudit appends a row to mcp_audit.
func (s *Store) RecordMCPAudit(ctx context.Context, e MCPAuditEntry) error {
	var tid sql.NullInt64
	if e.TokenID != 0 {
		tid = sql.NullInt64{Int64: e.TokenID, Valid: true}
	}
	var aid sql.NullInt64
	if e.ApprovalID != 0 {
		aid = sql.NullInt64{Int64: e.ApprovalID, Valid: true}
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO mcp_audit (ts, token_id, tool, args_summary, outcome, duration_ms, approval_id) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		e.TS.Unix(), tid, e.Tool, e.ArgsSummary, e.Outcome, e.DurationMS, aid)
	return err
}
