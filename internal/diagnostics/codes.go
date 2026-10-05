package diagnostics

// ErrorCode is a structured playback error code, stored by the admin error sink and
// grouped by the admin API. Add a code here only together with its probable cause in
// the admin API table.
type ErrorCode string

const (
	// StreamTimeout: no peer answered or playback took too long to start.
	StreamTimeout ErrorCode = "STREAM_TIMEOUT"
	// RemuxFailed: ffmpeg exited with an error while remuxing a stream or segment.
	RemuxFailed ErrorCode = "REMUX_FAILED"
	// SourceDead: every source/provider for an episode failed.
	SourceDead ErrorCode = "SRC_DEAD"
	// SourceTimeout: a source/provider did not answer in time.
	SourceTimeout ErrorCode = "SRC_TIMEOUT"
	// KVUnavailable: the shared Redis state could not be reached.
	KVUnavailable ErrorCode = "KV_UNAVAILABLE"
	// SubtitleFailed: subtitle extraction failed.
	SubtitleFailed ErrorCode = "SUBTITLE_FAILED"
	// Unknown: a playback error that fits no other code.
	Unknown ErrorCode = "UNKNOWN"
)

// ErrorCodes lists every code, in display order.
func ErrorCodes() []ErrorCode {
	return []ErrorCode{StreamTimeout, RemuxFailed, SourceDead, SourceTimeout, KVUnavailable, SubtitleFailed, Unknown}
}

// Valid reports whether c is a known code.
func (c ErrorCode) Valid() bool {
	for _, k := range ErrorCodes() {
		if c == k {
			return true
		}
	}
	return false
}
