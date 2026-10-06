Gazes: torrent→browser streaming (Go `cmd/ internal/`, Next.js `web/`, FFmpeg remux→HLS). Go: torrent stream playback library indexer(resolver, VF/FR ranking) metadata admin mcp api auth cache kv.

# Gotchas (verified)
- Go needs build tag: `go test|vet|build -tags=nosqlite` (plain go cmd fails to build). Prefer `make test-backend|lint-backend|typecheck|test-web`.
- "permission denied" on go mod cache → `export GOPATH=$HOME/go`. pnpm: `pnpm --dir web install --frozen-lockfile` in fresh worktree.
- Baseline is red: lint-web has pre-existing errors, internal/indexer TestRanking/TestTargetedFrenchSources fail. Judge work by touched files/pkgs, not by whole `make check`; never mask errors w/ eslint-disable/test edits.
- PROD = this host. Push to `main` auto-deploys (.forgejo). Never push/merge to main unless asked (dev auto-deploys to pre-prod :8082). dev stack shares :8080 w/ prod → `GAZES_PORT=8081 make dev`. Never `docker compose down -v`.
- `dev` = integration branch; keep it fast-forwardable to main.
- Dev docker runs as root → `web/.next*` and `web/public/subtitles` are root-owned: `next build`/`pnpm build` fail EACCES (no sudo, don't fight it). Verify web = `pnpm exec tsc --noEmit` + `pnpm exec eslint <touched files>` + `curl localhost:8081/<route>` (dev stack hot-reloads the worktree: SSR HTML, meta tags).
- `rtk` condenses output (build logs can vanish) → rerun `rtk proxy <cmd>` when a result is empty/unusable.
- AniList rate-limits (backend 502/503 "anilist rate limited, retry in Ns") = transient: never loop catalog calls; wait, retry once.

# Autonomy
Autopilot engineer: decide+execute alone, end to end. Ask only for irreversible/external actions (data deletion, secrets, spend) or product tradeoff w/ 2 valid options (1 question + recommendation).
Flow: branch off dev → minimal change (bugs: failing test first) → test touched pkg only → self-review final diff (`reviewer` agent only if asked) → commit → PR→dev (skill forgejo-pr) → merge into dev yourself once self-review+targeted tests are green → check the pre-prod deploy (.forgejo deploy-preprod, :8082) → delete branch (local+remote)+worktree (check clean first; never main/dev). Report ≤3 lines: what, verified how, risk.
Merging PR→dev is yours (user-authorized). Promoting dev→main (= prod deploy) only when the user asks. If the permission classifier denies an API merge: give the PR link, don't work around.

# Release announcement (dev→main merged, user-requested)
Each release = numbered update (V1.0 = 2026-10-06; next V1.1, V1.2…; ask only if it looks major). After the promotion to main is done and prod deploy is green:
1. Diff `origin/main@{before}..main` (commits no-merges + `--shortstat`) → pick the biggest user-facing features. No lines/files counts on visuals.
2. Update the Design artifact (https://claude.ai/artifact/6koXpoKrGdFAVfdyX8WPTS, Gazes design system, French, no gradients): 2 slides 1600×900 `Discord.dc.html` (announce: title + 4 feature cards) and `Discord2.dc.html` (details, 4 cards of 2 sentences), both with the `Vx.y` badge. Read the artifact files before editing (user may have edited).
3. Render both to PNG (Playwright screenshot of each artboard at 1600×900; see artifact files, don't verify via the editor).
4. Send via webhook: URLs in `~/.config/gazes/discord-webhook`, one per line (chmod 600, never in repo/logs/output); send the same message to EACH webhook (loop `while read -r W`), report per-URL HTTP codes only. ONE Components V2 message (`flags: 32768`, no `content`/`embeds`): NO Container (it shrinks the images): Text Display (type 10: `# Gazes Vx.y` + one short line), Separator (type 14, divider false), then ONE Media Gallery (type 12) PER image (`attachment://Discord.png`, `attachment://Discord2.png`) so they stack full width (2 items in one gallery = side-by-side grid). Build payload with python json, post `-F 'payload_json=<payload.json' -F 'files[0]=@Discord.png' -F 'files[1]=@Discord2.png'` to `"$W?wait=true&with_components=true"`, keep returned message id (a V2 flag can't be added by editing an old message). Plain `content` + 2 attachments gets cropped into a mosaic: don't.
Message in French, no emoji, short, no PII. Webhook missing → say so, hand over PNGs, don't ask for the URL in chat. Sending is external/irreversible: only on a release the user asked for.

# UI/UX
Any UX/UI work (view, component, redesign, share image, copy layout) → invoke skill `ui-design` FIRST (principles + Gazes tokens + verification). Never gradients. Web specifics: `web/CLAUDE.md`.

# Prod MCP (mcp__gazes__*)
Read-only except create_issue/add_note. No PII out. "What to improve" → agent `prod-watch`. Priority = users hit×severity/effort; fix high-impact bugs unasked; after merge re-check metric, add_note.

# Tokens
Sonnet default; Opus only after 2 failed hypotheses. Exact symbol → `rg`; relationship/architecture → `graphify query|path|explain` (fresh worktree: `graphify update .`). Never read whole: `web/src/app/globals.css` (76K), `web/src/lib/i18n.ts` (21K), lockfiles, `go.sum`, README (27K) → `rg -n` + line ranges (generated/lock files are read-denied in settings). Never re-read edited files. Sub-agents cold-start (costly): don't spawn unless user asks; `prod-watch` only for "what to improve". One worktree per session, never write to other worktrees (hook enforces). Verify own work (build/lint/tests) before saying done. Targeted tests only; `make check` once, at end. Terse output, parallel independent tool calls, no recaps.
