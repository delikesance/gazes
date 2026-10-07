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
// The runtime actions (sources, caches, AV1 queue, stream limit, maintenance) answer 503
// unavailable when their collaborator is not in this process (the stdio binary, for instance).

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
		actionTool("retry_source", LevelReversible, "ops:write",
			rev+"Retry a failing stream source now: close its circuit breaker so the next searches query it instead of waiting for the backoff. Refused while the source is paused. Nothing to undo.", "Retry a stream source",
			Param{Name: "source", Required: true, Description: "Source (provider) name, as listed by the sources health tool."}),
		actionTool("warm_cache", LevelReversible, "ops:write",
			rev+"Resolve the sources of one episode in the background so its next play starts from the cache. No effect when already cached unless refresh is true. Nothing to undo.", "Warm an episode's sources",
			Param{Name: "season_id", Type: "integer", Required: true, Description: "AniList id of the season (anime_id of the playback errors)."},
			Param{Name: "episode", Type: "integer", Required: true, Description: "Episode number."},
			Param{Name: "refresh", Type: "boolean", Description: "Drop the cached findings and resolve again."}),
		actionTool("requeue_av1", LevelReversible, "ops:write",
			rev+"Put a library copy whose AV1 encode was abandoned (3 failures) back in the encode queue with a fresh attempt count.", "Requeue an AV1 encode",
			Param{Name: "season_id", Type: "integer", Required: true, Description: "AniList id of the season."},
			Param{Name: "episode", Type: "integer", Required: true, Description: "Episode number."},
			Param{Name: "lang", Required: true, Enum: []string{"vostfr", "vf"}}),
		actionTool("pause_source", LevelSensitive, "config:write",
			sens+"Pause a stream source: its searches are skipped on every instance, the other sources answer. Refused when it would leave no active source. Undoing the approval resumes it.", "Pause a stream source",
			Param{Name: "source", Required: true, Description: "Source (provider) name."},
			Param{Name: "minutes", Type: "integer", Required: true, Description: "Pause length, 5 to 1440 minutes."}),
		actionTool("purge_cache", LevelSensitive, "config:write",
			sens+"Purge a cache scope. episode_sources: every episode's source findings (each is resolved again on its next play). indexer_results: the trackers' cached answers. Cannot be undone; it refills on demand.", "Purge a cache scope",
			Param{Name: "scope", Required: true, Enum: []string{"episode_sources", "indexer_results"}}),
		actionTool("limit_concurrent_streams", LevelSensitive, "config:write",
			sens+"Cap the concurrent playback sessions (HLS engine): new sessions get 503 stream_limit_reached while the cap is reached, open ones play on. 0 lifts the cap. Also feeds the stream_saturation_pct watch rule.", "Limit concurrent streams",
			Param{Name: "max_streams", Type: "integer", Required: true, Description: "Maximum concurrent sessions, 0 to 1000 (0 = no limit)."}),
		actionTool("schedule_maintenance", LevelSensitive, "config:write",
			sens+"Schedule a maintenance window (at most 24 h, starting within 30 days). It is announced on the public status page, and watch notifications are muted while it runs (issues are still opened). Replaces any window already scheduled.", "Schedule a maintenance",
			Param{Name: "starts_at", Description: "RFC 3339 start (default: now)."},
			Param{Name: "ends_at", Required: true, Description: "RFC 3339 end."},
			Param{Name: "message", Description: "One line shown to visitors (max 200 characters, plain text)."}),
		{
			Name: "get_approval", Level: LevelRead, Scope: "diagnostics:read", Method: "GET", Path: "/approvals/{id}",
			Description: "Follow a pending approval: its status (pending, approved, rejected, failed, undone), who decided, and the result once executed.",
			Summary:     "Read an approval's decision",
			Params:      []Param{{Name: "id", Type: "integer", Required: true, In: "path", Description: "approval_id returned by an action tool."}},
		},
	}
}
