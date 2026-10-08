# Runbooks

One page per watch rule or playback error code. They are what the watch issues point to (`suggested_fix`) and what Claude reads when triaging. Each page has the same sections: **Signal**, **First checks**, **Probable causes**, **What Claude may do**, **Verify**.

What Claude may do is limited to what the admin API allows today: read everything (`diagnostics:read`), create/annotate/resolve issues (`ops:write`, reversible), retry a source, warm an episode or requeue an AV1 encode (`ops:write`, reversible), and propose a threshold change, a source pause, a cache purge, a stream limit or a maintenance window (`config:write`, needs a human approval). Fixes in code or infrastructure are proposed as text to a human.

| Page | Rule or code |
|---|---|
| [error-rate.md](error-rate.md) | `error_rate_pct` |
| [disk.md](disk.md) | `disk_pct` |
| [saturation.md](saturation.md) | `stream_saturation_pct` (measured once a stream limit is set) |
| [SRC_DEAD.md](SRC_DEAD.md) | `source_failures`, `SRC_DEAD` |
| [SRC_TIMEOUT.md](SRC_TIMEOUT.md) | `SRC_TIMEOUT` |
| [STREAM_TIMEOUT.md](STREAM_TIMEOUT.md) | `STREAM_TIMEOUT`, `startup_p95_s` (not measured yet) |
| [REMUX_FAILED.md](REMUX_FAILED.md) | `REMUX_FAILED` |
| [SUBTITLE_FAILED.md](SUBTITLE_FAILED.md) | `SUBTITLE_FAILED` |
| [KV_UNAVAILABLE.md](KV_UNAVAILABLE.md) | `KV_UNAVAILABLE` (defined, not emitted yet) |
