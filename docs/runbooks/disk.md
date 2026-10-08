# disk_pct: disk usage

**Signal.** Used share of the volume of `GAZES_WATCH_DISK_PATH`, above the threshold (default 85 %). Without that variable the rule is `not_measured`.

**First checks.** What lives on that volume: the torrent download cache and the AV1 library (`internal/library`), the SQLite databases, logs. `get_costs` shows hours watched, a rough driver of cache churn.

**Probable causes.** The sliding-window cache is not evicting (invariant 3 in AGENTS.md), an AV1 library that grew, logs or diagnostic files.

**What Claude may do.** Annotate the issue with the breakdown a human provides, propose a retention or cache-size change as text. The torrent download cache and the AV1 library are not purge scopes: a human frees them. `purge_cache` only empties Redis caches and does not free this volume.

**Verify.** The rule is back under the threshold for three consecutive evaluations; the 24 h effect shows the value after the fix.
