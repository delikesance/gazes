# REMUX_FAILED: ffmpeg could not remux

**Signal.** ffmpeg exited with an error producing a segment or a stream (`internal/stream/remux.go`, `internal/playback/manager.go`).

**First checks.** One file or many? `list_playback_errors(code=REMUX_FAILED)` gives anime and episode; the diagnostics store (`gazes-logs`) has the ffmpeg stderr.

**Probable causes.** A corrupt or unusual container, a codec ffmpeg cannot copy, a truncated download, an out-of-date ffmpeg.

**What Claude may do.** Collect the affected files on the issue, propose the fix (another release of the episode, an ffmpeg option) as text. Never re-encode video as a workaround: remux-over-transcode is an architectural invariant (AGENTS.md).

**Verify.** The code's count drops; the affected episodes play.
