# Admin panel: operating guide

The admin panel is the web UI under `/admin`, the REST API under `/api/v1/admin`, and the MCP server under `/mcp`. They share one service (`internal/admin`) and one database (`admin.sqlite`). Design and history: [admin-panel-plan.md](admin-panel-plan.md). Claude's routines and the webhook: [admin-claude-routines.md](admin-claude-routines.md). Incident procedures: [runbooks/](runbooks/README.md).

## Bootstrap

1. The server needs Redis (as for the rest of Gazes). The admin database opens next to the accounts database (`GAZES_ADMIN_DB`, default `<ACCOUNTS_DIR>/admin.sqlite`). If it cannot be opened the server logs an error and runs **without** admin: streaming never depends on it.
2. Make the first administrator (run on the server machine, it opens the local databases): `gazes-admin grant <pseudo>`. Pseudos are not unique: when several accounts share one, use the account id, `gazes-admin grant '#42'`. `gazes-admin revoke <pseudo | #id>` refuses to remove the last administrator unless `--force`.
3. Sign in on the site with that account, open `/admin`. A visitor who is not an administrator gets a plain 404.
4. Connect Claude: in the panel, "Claude et MCP" has a "Connecter Claude" section. Pick the permissions and a validity, press "Générer le lien": it creates a token and shows the ready-to-paste `claude mcp add --scope user --transport http gazes https://<your site>/mcp --header "Authorization: Bearer gzs_..."` (and the same as a `.mcp.json` block). The token is shown once and never stored in the browser. The site relays `/mcp` to the backend (`next.config.ts`), so the public address works. On the server machine the CLI does the same: `gazes-admin token create --name claude --scopes metrics:read,diagnostics:read --ttl 720h`, `token list`, `token revoke <id>`; in the panel, revoke from "Paramètres".

## Environment

| Variable | Default | Role |
|---|---|---|
| `GAZES_ADMIN_DB` | `<ACCOUNTS_DIR>/admin.sqlite` | admin database |
| `GAZES_WATCH_WEBHOOK_URL` / `GAZES_WATCH_WEBHOOK_SECRET` | empty | watch events and their HMAC key |
| `GAZES_WATCH_DISK_PATH` | empty | volume measured by the `disk_pct` rule (empty = not measured) |
| `GAZES_COST_SERVER_MONTH`, `GAZES_COST_BANDWIDTH_PER_GB`, `GAZES_COST_STORAGE_PER_GB_MONTH`, `GAZES_GB_PER_WATCH_HOUR` | empty | inputs of the Business page costs (empty = `[À RENSEIGNER]`, never a made-up figure) |
| `GAZES_SITE_URL`, `GAZES_BTCPAY_URL`, `GAZES_BTCPAY_STORE_ID`, `GAZES_BTCPAY_API_KEY`, `GAZES_BTCPAY_WEBHOOK_SECRET`, `GAZES_KOFI_URL`, `GAZES_KOFI_TOKEN`, `GAZES_DONATION_GOAL_EUR` | empty | donations, see `docs/donations.md` (empty = the `/soutenir` page says "bientôt") |

## What runs in the background

One replica per period (Redis election): the metrics rollup every 10 minutes (idempotent, replayable) and the watch evaluation every minute. The first start backfills 30 days. Rows older than the retention (setting `retention_days`, default 180, 30 to 730; no screen or action sets it yet, edit the `settings` table) are pruned hourly from `playback_errors`, `playback_startups` and `mcp_audit`.

## Security model in one page

- The panel guard only hides the area; the Go API decides every request. Every admin route is refused without credentials (a test walks the real router to enforce it).
- **Session** (an administrator signed in on the site): all scopes; state-changing requests also need the `X-Gazes-Admin: 1` header (CSRF). **Token**: `Authorization: Bearer gzs_...` only, never the cookie; four scopes (`metrics:read`, `diagnostics:read`, `ops:write`, `config:write`); stored as SHA-256, shown once, 30 days by default.
- A token never receives a pseudo or an e-mail; no e-mail is ever read by the panel. Approving, rejecting, undoing, the kill switch and creating or revoking a token are session-only (with the CSRF header): a token gets 403 whatever its scopes, so a token can never mint another. At most 20 tokens are active at once, and every creation and revocation is audited. Sensitive actions only create an approval a human decides, executed at most once.
- Runtime actions, by level. Reversible: `retry_source` (close a source's circuit breaker), `warm_cache` (resolve an episode's sources in the background, two at a time), `requeue_av1` (put an abandoned AV1 encode back in the queue; undo takes it out again unless the encoder already started it). Sensitive: `pause_source` (5 to 1440 min, never the last active source; undo resumes), `purge_cache` (not undoable), `limit_concurrent_streams` (new HLS sessions get 503 `stream_limit_reached` at the cap, open ones play on; it also feeds the `stream_saturation_pct` rule; undo restores the previous cap), `schedule_maintenance` (at most 24 h, within 30 days, shown on `/status`, watch notifications muted while it runs; undo restores the previous window).
- The kill switch makes every token write fail with 423. Reversible actions are dry runs unless `dry_run:false`, 30 per hour per token.
- Every MCP call and every decision is written to `mcp_audit` (arguments redacted and truncated).

## Capacity (measured)

`go test -tags=nosqlite -run TestLoadTimings -v ./internal/admin/` with `GAZES_LOAD_TEST=1` (300 000 sessions over 90 days, 20 000 accounts, synthetic):

| Step | Time |
|---|---|
| Rollup of 90 days | 0.8 s |
| Watch evaluation | 1 ms |
| `/overview` (rollups) | 8 ms |
| `/playback/health` | 1 ms |
| `/users?q=` | 21 ms |
| `/users/summary`, `/users`, `/costs` (cold) | 0.3 s |
| `/growth` (cold) | 0.6 s |
| `/catalog` (cold) | 0.95 s |
| `/views` (cold) | 2.3 s |

The heavy aggregates are cached for one minute per caller kind and URL (`X-Gazes-Cache: hit|miss`), and concurrent identical requests share one computation. `/views` and `/catalog` read per-day aggregates (`metrics_views_daily`, `metrics_drop_daily`, `metrics_catalog_daily`, written by the rollup with the other `metrics_*` tables) when every day of the period has been rolled up, and otherwise fall back to a period-bounded scan of `watch_sessions` that gives the same answer (tests compare both). Today and yesterday are recomputed every 10 minutes, so a period that includes today lags by up to that long. After an upgrade that created these tables the startup backfill rebuilds the last 90 days. The other per-user aggregates (segments, directory, funnel, timezones, resume) still scan `watch_sessions` (`TODO(rollup)`).

## Backups and upgrades

Back up `accounts.sqlite` (it is the source of truth) and `admin.sqlite` (settings, tokens, issues, approvals, audit; the metrics tables can be rebuilt by the rollup). Schemas are versioned (`PRAGMA user_version`) and migrated at start; a database newer than the running binary is refused rather than downgraded.

## Known limits

Not measured yet, shown as `[À MESURER]`: stream capacity, device, country, acquisition source, favorites. The runtime actions (`retry_source`, `warm_cache`, `requeue_av1`, `pause_source`, `purge_cache`, `limit_concurrent_streams`, `schedule_maintenance`) act on the server process; called through the standalone `gazes-mcp` binary, or when their part is off (library disabled, legacy playback engine for the stream limit), they answer 503 `unavailable` with no effect. `purge_cache` only covers `episode_sources` and `indexer_results`: the catalog caches are left out on purpose (refilling them hits the AniList limit), and the torrent download cache on disk is not a scope. Without Redis the leader election is skipped and two replicas would both run the background jobs.
