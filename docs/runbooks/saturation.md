# stream_saturation_pct: stream capacity

**Signal.** Open HLS playback sessions over the cap set by `limit_concurrent_streams` (default threshold 85 %). Without a cap, or with the legacy playback engine, the rule shows `not_measured`.

**First checks.** `get_player_health` (active sessions, error rate), the evening peak (20 h to 23 h, Visionnages page), host CPU and network.

**Probable causes.** A real peak, sessions not released by clients, a cap set below what the host handles.

**What Claude may do.** Annotate the issue; propose a new cap with `limit_concurrent_streams` (sensitive, a human approves). At the cap new sessions get 503 `stream_limit_reached`, open ones keep playing.

**Verify.** The rule is back under the threshold; `stream_limit_reached` refusals stop in the logs.
