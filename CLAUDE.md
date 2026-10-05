# CLAUDE.md

See [AGENTS.md](AGENTS.md) for project guidelines and architectural invariants.

## Agent Orchestration (mandatory)

- The main session acts as **orchestrator and reasoner**: it scopes the problem, plans, splits work into bounded tasks, reviews sub-agent results, and makes the final decisions.
- Hands-on work (codebase exploration, investigation, implementation, tests) is delegated to **sub-agents**.
- Sub-agents never inherit the orchestrator's model: always pass `model` explicitly on every Agent tool call.
  - Default is **Sonnet 5.5** (`model: "sonnet"`) for implementation and anything needing design or multi-file reasoning.
  - **Haiku 4.5** (`model: "haiku"`) is allowed for simple, bounded, mechanical tasks: codebase lookups, fact-finding, running tests/lint, generating types, replicating an existing pattern, docs. If Haiku fails a task once, escalate it to Sonnet.
- Prefer querying the graphify knowledge graph (once set up, see `docs/admin-panel-plan.md`) over reading files; keep task briefs and reports short.
- The orchestrator verifies sub-agent output (diffs, test results) before reporting anything as done.
