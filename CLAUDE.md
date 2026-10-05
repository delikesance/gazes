# CLAUDE.md

See [AGENTS.md](AGENTS.md) for project guidelines and architectural invariants.

## Working mode: inline, no sub-agents (token budget)

- Do the work in the main session. **Do not spawn sub-agents** (Agent tool) unless the user explicitly asks for one in that request. Every sub-agent starts cold and re-reads context, which costs far more tokens than working inline with a warm prompt cache.
- Keep the context lean: query the graphify knowledge graph (`graphify query "..."`, outputs in `graphify-out/`, see `docs/admin-panel-plan.md`) before reading files; read only the files and line ranges you need; keep command output short (`| tail`, `| head`, `grep`); do not re-read files you just wrote.
- Verify your own work before reporting it done: run the relevant build, lint and tests and read the results.
- Prefer one worktree per session and do not write to other worktrees from this session (a hook enforces it).
