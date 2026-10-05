# KV_UNAVAILABLE: Redis unavailable

**Signal.** Defined but not emitted yet: the Redis check lives in the cache diagnostics probe, not in a playback error path. `get_player_health` carries the cache diagnostics (status, latency, keys) when available.

**First checks.** `get_player_health` cache block; Redis container status; the leader election of the background jobs (rollup, watch) falls back to running everywhere when Redis is down.

**What Claude may do.** Annotate and escalate to a human: Redis recovery is infrastructure work.
