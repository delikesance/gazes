# Running Claude on Gazes: webhook and routines

How the monitoring reaches Claude, and what to schedule. Nothing here runs by itself until you configure it.

## 1. What the server does by itself

Every minute (one replica, elected through Redis) the server evaluates the watch rules (`error_rate_pct`, `disk_pct`, `source_failures`; `startup_p95_s` and `stream_saturation_pct` are not measured yet). On a new breach it opens one issue (source `watch:<rule>`, severity high or medium, a runbook in `suggested_fix`) and, when configured, POSTs a webhook. After three healthy evaluations it notes "back to normal" on the issue and sends `watch.recovered`; it never closes an issue itself. Playback errors and the MCP call log older than the retention (default 180 days, 30 to 730) are pruned hourly.

| Variable | Role |
|---|---|
| `GAZES_WATCH_WEBHOOK_URL` | http(s) URL receiving the events (optional, no redirects followed, 5 s timeout) |
| `GAZES_WATCH_WEBHOOK_SECRET` | HMAC-SHA256 key: header `X-Gazes-Signature: sha256=<hex of the body>` (optional) |
| `GAZES_WATCH_DISK_PATH` | directory whose volume `disk_pct` measures (optional) |

Event body: `{event, rule, label, value, threshold, unit, detail, issue_id, at, runbook}`. No user data.

## 2. Connecting Claude

1. On the server machine: `gazes-admin token create --name claude --scopes metrics:read,diagnostics:read,ops:write --ttl 720h` (add `config:write` only if Claude should be able to PROPOSE threshold changes; a human still approves). The token is shown once.
2. In Claude Code: `claude mcp add --transport http gazes https://<host>/mcp --header "Authorization: Bearer <token>"`.
3. The page "Claude et MCP" shows the tools, the approval queue, the kill switch and the call log.

## 3. What to schedule in Claude

Create these as scheduled tasks on your side (they are not created for you):

- **Every 15 minutes, a check.** Prompt: "Call get_watch_status. If a rule is breached or an issue opened by the watch is still new, follow the runbook named in its suggested_fix (docs/runbooks), read the evidence with get_errors_summary, list_playback_errors and get_sources_health, then add_note on the issue with the numbers and move it to in_progress. Do not invent values: a null or measured:false figure is unknown. Propose fixes as text; do not claim a fix without a later measure."
- **Daily digest.** Prompt: use the MCP prompt `weekly-review` with a 7-day window, summarise changes since yesterday, list open issues and any `effects` with verdict `not_improved`.
- **Weekly review and planning.** The prompts `weekly-review`, `cost-audit`, `activation-review` and `business-plan-12m` of the MCP server.
- **On a webhook.** If your automation can start a Claude run from an HTTP event, give it the `issue_id` and the runbook path from the body and use the first prompt above.

## 4. Guarantees to keep in mind

Claude reads aggregates only (never e-mails, never pseudos). Reversible writes are simulations unless `dry_run:false`, limited to 30 per hour per token. Sensitive actions only create an approval a human must decide. The kill switch (page "Claude et MCP") makes every token write fail with 423.
