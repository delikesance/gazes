# source_failures / SRC_DEAD: a source is dead

**Signal.** Five or more `SRC_DEAD` errors from one source in 30 minutes (rule `source_failures`, severity medium). The detail names the top sources.

**First checks.** `get_sources_health`: share of failures per source. `list_playback_errors(code=SRC_DEAD)`: which anime and episodes. Check whether the source answers (tracker status, DNS).

**Probable causes.** The tracker or provider is down or blocking, credentials or API changed (`internal/indexer`), no provider returned results for these titles (`providers_unavailable` also counts here).

**What Claude may do.** Note the affected sources and episodes on the issue, mark it `in_progress`, then `retry_source` once the tracker answers again (it closes the circuit breaker). A source that keeps failing: request `pause_source` (sensitive, a human approves; refused for the last active source). For one episode, `warm_cache` with `refresh` resolves it again.

**Verify.** `source_failures` back to 0 failures in the window; the 24 h effect confirms it did not come back.
