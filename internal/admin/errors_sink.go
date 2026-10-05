package admin

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gazes/gazes/internal/diagnostics"
)

// PlaybackError is one playback failure to record. Zero AnimeID/SeasonID/Episode mean
// "unknown" and are stored as NULL; they are filled from the request correlation
// (diagnostics.Get) when the context carries them.
type PlaybackError struct {
	Code     diagnostics.ErrorCode
	AnimeID  int64
	SeasonID int64
	Episode  int64
	Source   string // tracker / provider name, when known
	InfoHash string
	Message  string // redacted and truncated before storage
}

// ErrorSink receives playback errors. It is injected as an option into the code that
// detects them; a nil ErrorSink means "no recording" and must never change behaviour.
type ErrorSink interface {
	RecordPlaybackError(ctx context.Context, e PlaybackError)
}

// RecordPlaybackError records e on sink and is a no-op when sink is nil. It never blocks,
// never returns an error and never panics.
func RecordPlaybackError(ctx context.Context, sink ErrorSink, e PlaybackError) {
	if sink == nil {
		return
	}
	defer func() { _ = recover() }()
	sink.RecordPlaybackError(ctx, e)
}

const (
	errorQueueSize  = 256
	errorMaxMessage = 300
	errorMaxField   = 120
	errorWriteLimit = 3 * time.Second
)

// ErrorRecorder writes playback errors to the playback_errors table from one background
// goroutine fed by a bounded queue. When the queue is full the error is dropped silently:
// recording must never slow a playback down.
type ErrorRecorder struct {
	store   *Store
	now     func() time.Time
	queue   chan PlaybackError
	done    chan struct{}
	wg      sync.WaitGroup
	closed  atomic.Bool
	dropped atomic.Uint64
}

// NewErrorRecorder starts a recorder over store. Call Close to flush and stop it.
func NewErrorRecorder(store *Store) *ErrorRecorder {
	r := &ErrorRecorder{store: store, now: time.Now, queue: make(chan PlaybackError, errorQueueSize), done: make(chan struct{})}
	r.wg.Add(1)
	go r.run()
	return r
}

// Dropped is the number of errors discarded because the queue was full.
func (r *ErrorRecorder) Dropped() uint64 { return r.dropped.Load() }

// RecordPlaybackError queues e (see ErrorSink). Nil-receiver safe.
func (r *ErrorRecorder) RecordPlaybackError(ctx context.Context, e PlaybackError) {
	if r == nil || r.closed.Load() {
		return
	}
	defer func() { _ = recover() }() // send on a queue closed concurrently by Close
	if !e.Code.Valid() {
		e.Code = diagnostics.Unknown
	}
	if c := diagnostics.Get(ctx); c != (diagnostics.Correlation{}) {
		e.AnimeID = firstPositive(e.AnimeID, atoi64(c.AnimeID))
		e.SeasonID = firstPositive(e.SeasonID, atoi64(c.SeasonID))
		e.Episode = firstPositive(e.Episode, atoi64(c.Episode))
	}
	select {
	case r.queue <- e:
	default:
		r.dropped.Add(1)
	}
}

// Close stops accepting errors, writes what is queued and returns.
func (r *ErrorRecorder) Close() {
	if r == nil || !r.closed.CompareAndSwap(false, true) {
		return
	}
	close(r.done)
	r.wg.Wait()
}

func (r *ErrorRecorder) run() {
	defer r.wg.Done()
	for {
		select {
		case e := <-r.queue:
			r.write(e)
		case <-r.done:
			for {
				select {
				case e := <-r.queue:
					r.write(e)
				default:
					return
				}
			}
		}
	}
}

func (r *ErrorRecorder) write(e PlaybackError) {
	defer func() { _ = recover() }()
	ctx, cancel := context.WithTimeout(context.Background(), errorWriteLimit)
	defer cancel()
	_, _ = r.store.db.ExecContext(ctx,
		`INSERT INTO playback_errors (ts, code, anime_id, season_id, episode, source, info_hash, message) VALUES (?,?,?,?,?,?,?,?)`,
		r.now().Unix(), string(e.Code), nullInt(e.AnimeID), nullInt(e.SeasonID), nullInt(e.Episode),
		nullStr(truncate(e.Source, errorMaxField)), nullStr(truncate(strings.ToLower(e.InfoHash), 64)),
		nullStr(truncate(diagnostics.Redact(e.Message), errorMaxMessage)))
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	// Cut on a rune boundary.
	for n > 0 && s[n]&0xC0 == 0x80 {
		n--
	}
	return s[:n]
}

func nullInt(v int64) any {
	if v <= 0 {
		return nil
	}
	return v
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func atoi64(s string) int64 {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0
	}
	return n
}

func firstPositive(a, b int64) int64 {
	if a > 0 {
		return a
	}
	return b
}
