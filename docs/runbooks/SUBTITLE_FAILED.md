# SUBTITLE_FAILED: subtitle extraction failed

**Signal.** Subtitle extraction for a stream failed (`internal/api/subtitle_handlers.go`).

**First checks.** Which episodes; whether the file really has subtitle tracks; ffmpeg stderr in the diagnostics store.

**Probable causes.** An unsupported subtitle codec (bitmap subtitles), a damaged track, a timeout on a large file.

**What Claude may do.** Annotate the issue and group by release; propose a handling for the codec as text.

**Verify.** The count drops for new playbacks.
