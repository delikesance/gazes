# internal/mcp

MCP server for the admin panel. Tools hold no data logic: each `ToolDef` (tools.go) names an admin API
route, and a call runs that route in process (`serveAdmin`) with the caller's own `Authorization: Bearer`
token. Authentication, scopes, bounds (90 days, 200 rows), rate limit and privacy rules are therefore
exactly those of `/api/v1/admin`. Results are the envelope's `data`, bounded to 64 KiB (`truncated:true`).

Add a tool = add one line to `DefaultTools()` in tools.go:

    {Name: "get_issue", Level: LevelRead, Scope: "metrics:read", Method: "GET", Path: "/issues/{id}",
     Description: "...", Summary: "...",
     Params: []Param{{Name: "id", Type: "string", Required: true, In: "path"}}}

- `Scope` must match the scope the admin route requires; a tool whose scope the token lacks is hidden
  from `tools/list` and refused on call.
- `Params` become the JSON input schema; `In` is "query" (default) or "path". Use `Enum` for closed
  sets and `Sensitive: true` for values that must stay out of the audit log.
- Non-GET tools send a JSON body: `Param.In` "body" (top-level field) or "arg" (field of `args`). Action tools live in tools_actions.go (`actionTools()`): `dry_run` defaults to true, reversible actions run with `dry_run:false` and return an `undo_token`, sensitive ones only file a pending approval (`get_approval` follows it). Guard-rails (kill switch, budget, idempotency, audit) are enforced by the admin API (`internal/admin/api_ops.go`); approve/reject/undo/kill switch are session-only and have no tool.
- Every call is audited in `mcp_audit` (ok | error | denied); audit failures never fail the call.
- Test it by listing it in the scope tests of mcp_test.go (in-memory + httptest clients).
