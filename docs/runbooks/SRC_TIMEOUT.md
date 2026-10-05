# SRC_TIMEOUT: source resolution timed out

**Signal.** Source resolution exceeded its deadline (`internal/api/catalog_handlers.go`, `internal/indexer`).

**First checks.** `get_errors_summary` trend; whether one source or all of them; load on the host at that time.

**Probable causes.** A slow provider, DNS or network trouble, too many parallel searches.

**What Claude may do.** Annotate, correlate with `SRC_DEAD` and `get_player_health`, propose a timeout or concurrency change as text.

**Verify.** The code's daily count drops back in `get_errors_summary`.
