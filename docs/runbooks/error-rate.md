# error_rate_pct: playback error rate

**Signal.** Errors recorded in the last 60 minutes divided by sessions started in the same window, above the threshold (default 5 %). Needs at least 20 sessions in the window, otherwise the rule is `not_measured`.

**First checks.** `get_watch_status` (value, since), then `get_errors_summary(7)` for the split by code and `list_playback_errors` for the groups (anime, episode, source). One code dominating means follow that code's runbook; a spread across codes with one source means [SRC_DEAD](SRC_DEAD.md).

**Probable causes.** A dead or slow source or tracker, a swarm without peers for a popular episode, an ffmpeg failure on one file, a deployment that broke the player.

**What Claude may do.** Annotate the issue with the dominant code and the numbers (`add_note`), move it to `in_progress`, and write the proposed code or infrastructure change as text for a human. If the threshold itself is wrong for the traffic, propose a new one with `set_alert_threshold` (a human approves).

**Verify.** `get_watch_status` shows the rule back under the threshold for three consecutive evaluations; 24 h after the issue is resolved the effect table shows the value measured then (`improved` when it is under the threshold).
