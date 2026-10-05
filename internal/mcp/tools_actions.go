package mcp

// Action tools: bounded writes. Each one calls POST /ops/actions/{name} of the admin API, which
// owns every guard-rail (kill switch, hourly budget, idempotency, audit, approvals):
//
//   - dry_run defaults to TRUE: the tool returns the plan and changes nothing.
//   - dry_run:false on a REVERSIBLE action executes it and returns an undo_token and an action_id.
//   - dry_run:false on a SENSITIVE action never executes it: it files a pending approval
//     ({"status":"pending_approval","approval_id":N}) that a human decides in the admin panel.
//     justification and expected_effect are then required. get_approval follows the decision.
//
// Approving, rejecting, undoing, the kill switch and token management are deliberately NOT tools.
// The actions declared in the admin registry without an implementation (retry_source,
// warm_cache, requeue_av1, pause_source, purge_cache, limit_concurrent_streams,
// schedule_maintenance) have no tool: see GET /ops/actions.

// actionCommon are the parameters every action tool takes.
func actionCommon(sensitive bool) []Param {
	just := "Why this action is needed."
	exp := "What should change once it is done."
	if sensitive {
		just += " Required when dry_run is false (the action then waits for human approval)."
		exp += " Required when dry_run is false."
	}
	return []Param{
		{Name: "dry_run", Type: "boolean", In: "body", Description: "Default true: only return the plan, change nothing. Pass false to execute (reversible actions) or to request approval (sensitive actions)."},
		{Name: "idempotency_key", In: "body", Description: "Optional caller-chosen key (1-64 of A-Z a-z 0-9 . _ : -). Repeating a call with the same key and arguments returns the first result without running again."},
		{Name: "justification", In: "body", Description: just},
		{Name: "expected_effect", In: "body", Description: exp},
	}
}

func actionTool(name string, level Level, scope, desc, summary string, params ...Param) ToolDef {
	for i := range params {
		params[i].In = "arg"
	}
	return ToolDef{
		Name: name, Level: level, Scope: scope, Method: "POST", Path: "/ops/actions/" + name,
		Description: desc, Summary: summary,
		Params: append(params, actionCommon(level == LevelSensitive)...),
	}
}

func actionTools() []ToolDef {
	const rev = "Reversible action: with dry_run:false it runs immediately, is budgeted per hour, and returns an undo_token. dry_run defaults to true. "
	const sens = "Sensitive action: it never runs by itself. dry_run:false files a pending approval for a human (returns approval_id); follow it with get_approval. dry_run defaults to true. "
	return []ToolDef{
		actionTool("set_issue_status", LevelReversible, "ops:write",
			rev+"Set the status of an issue and optionally replace its triage note.", "Change issue status",
			Param{Name: "id", Required: true, Description: "Issue id (iss-...)."},
			Param{Name: "status", Required: true, Enum: []string{"new", "in_progress", "resolved"}},
			Param{Name: "note", Description: "Optional triage note (max 2000 characters); replaces the current note."}),
		actionTool("create_issue", LevelReversible, "ops:write",
			rev+"Raise a new issue (finding) with status new.", "Create an issue",
			Param{Name: "title", Required: true, Description: "Short title (max 200 characters)."},
			Param{Name: "severity", Required: true, Enum: []string{"low", "medium", "high", "critical"}},
			Param{Name: "evidence", Description: "Supporting facts (max 4000 characters)."},
			Param{Name: "suggested_fix", Description: "Proposed fix (max 2000 characters)."},
			Param{Name: "source", Description: "Origin of the finding (max 100 characters)."}),
		actionTool("add_note", LevelReversible, "ops:write",
			rev+"Append a line to the triage note of an issue.", "Add an issue note",
			Param{Name: "issue_id", Required: true, Description: "Issue id (iss-...)."},
			Param{Name: "text", Required: true, Description: "Text to append (max 2000 characters in total)."}),
		actionTool("set_alert_threshold", LevelSensitive, "config:write",
			sens+"Change an alert threshold. Allowed rules and bounds: error_rate_pct 0.1-50, startup_p95_s 0.5-60, stream_saturation_pct 10-100, disk_pct 50-99.", "Change an alert threshold",
			Param{Name: "rule", Required: true, Enum: []string{"error_rate_pct", "startup_p95_s", "stream_saturation_pct", "disk_pct"}},
			Param{Name: "value", Type: "number", Required: true, Description: "New threshold, within the rule's bounds."}),
		{
			Name: "get_approval", Level: LevelRead, Scope: "diagnostics:read", Method: "GET", Path: "/approvals/{id}",
			Description: "Follow a pending approval: its status (pending, approved, rejected, failed, undone), who decided, and the result once executed.",
			Summary:     "Read an approval's decision",
			Params:      []Param{{Name: "id", Type: "integer", Required: true, In: "path", Description: "approval_id returned by an action tool."}},
		},
	}
}
