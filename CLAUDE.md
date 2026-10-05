Gazes: torrent→browser streaming (Go `cmd/ internal/`, Next.js `web/`, FFmpeg remux→HLS). Go: torrent stream playback library indexer(resolver, VF/FR ranking) metadata admin mcp api auth cache kv.

# Gotchas (verified)
- Go needs build tag: `go test|vet|build -tags=nosqlite` (plain go cmd fails to build). Prefer `make test-backend|lint-backend|typecheck|test-web`.
- "permission denied" on go mod cache → `export GOPATH=$HOME/go`. pnpm: `pnpm --dir web install --frozen-lockfile` in fresh worktree.
- Baseline is red: lint-web has pre-existing errors, internal/indexer TestRanking/TestTargetedFrenchSources fail. Judge work by touched files/pkgs, not by whole `make check`; never mask errors w/ eslint-disable/test edits.
- PROD = this host. Push to `main` auto-deploys (.forgejo). Never push/merge to main unless asked. dev stack shares :8080 w/ prod → `GAZES_PORT=8081 make dev`. Never `docker compose down -v`.
- `dev` = integration branch; keep it fast-forwardable to main.

# Autonomy
Decide+execute alone; ask only for irreversible/external actions (prod deploy, data deletion, secrets, spend) or product tradeoff w/ 2 valid options (1 question + recommendation).
Flow: branch off dev → minimal change (bugs: failing test first) → test touched pkg only → self-review final diff (`reviewer` agent only if asked) → commit → PR→dev (skill forgejo-pr) → merge → delete branch (local+remote)+worktree (check clean first; never main/dev). Report ≤3 lines: what, verified how, risk.

# Prod MCP (mcp__gazes__*)
Read-only except create_issue/add_note. No PII out. "What to improve" → agent `prod-watch`. Priority = users hit×severity/effort; fix high-impact bugs unasked; after merge re-check metric, add_note.

# Tokens
Sonnet default; Opus only after 2 failed hypotheses. Exact symbol → `rg`; relationship/architecture → `graphify query|path|explain` (fresh worktree: `graphify update .`). Read by range, never re-read edited files. Sub-agents cold-start (costly): don't spawn unless user asks; `prod-watch` only for "what to improve". One worktree per session, never write to other worktrees (hook enforces). Verify own work (build/lint/tests) before saying done. Targeted tests only; `make check` once, at end. Terse output, parallel independent tool calls, no recaps.
