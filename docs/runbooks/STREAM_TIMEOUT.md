# STREAM_TIMEOUT: no peers or slow start

**Signal.** A playback session or segment hit its deadline: no peer for the torrent, or the start took longer than the manager allows (`internal/playback/manager.go`, `internal/torrent`). The rule `startup_p95_s` is `not_measured` until start time is instrumented.

**First checks.** `list_playback_errors(code=STREAM_TIMEOUT)`: same episode again and again (dead swarm) or all episodes (host or network)? `get_player_health`: active sessions and error rate.

**Probable causes.** Few or no seeders, a firewall or port issue on the BitTorrent port, disk too slow, the host CPU saturated by remuxing.

**What Claude may do.** Annotate with the affected episodes, suggest a better source for them as text, propose a deadline change as text.

**Verify.** The error count for the code drops for those episodes; `error_rate_pct` stays under its threshold.
