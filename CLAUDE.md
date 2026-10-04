# CLAUDE.md

See [AGENTS.md](AGENTS.md) for project guidelines and architectural invariants.

## Agent Orchestration (mandatory)

- The main session acts as **orchestrator and reasoner**: it scopes the problem, plans, splits work into bounded tasks, reviews sub-agent results, and makes the final decisions.
- Hands-on work (codebase exploration, investigation, implementation, tests) is delegated to **sub-agents**.
- Sub-agents **always run on Sonnet 5.5**: pass `model: "sonnet"` on every Agent tool call. Never let a sub-agent inherit the orchestrator's model.
- The orchestrator verifies sub-agent output (diffs, test results) before reporting anything as done.
