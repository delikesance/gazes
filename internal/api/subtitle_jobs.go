package api

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"sync"
	"time"

	"github.com/gazes/gazes/internal/diagnostics"
)

const (
	// A subtitle window needs every video byte of its range from the swarm, so a slow
	// swarm can take far longer than one HTTP request may wait. The extraction keeps
	// running after the request gives up and the retry joins it.
	subtitleJobTimeout    = 3 * time.Minute
	subtitleJobLinger     = 90 * time.Second // keep an unclaimed running job this long after its last waiter timed out
	subtitleResultTTL     = 2 * time.Minute
	subtitleResultEntries = 32
	subtitleResultMaxSize = 64 << 20
	// Each new extraction is an ffmpeg reading the swarm; beyond this many running at once, new ones are refused.
	subtitleMaxConcurrent = 4
)

// subtitleWaitTimeout bounds one request; it stays below the frontend proxy's 30-second deadline.
var subtitleWaitTimeout = 25 * time.Second

var errSubtitleTooLarge = errors.New("subtitle output too large")

type subtitleJob struct {
	done     chan struct{}
	cancel   context.CancelFunc
	waiters  int
	linger   *time.Timer
	data     []byte
	err      error
	stderr   string
	finished time.Time
}

// subtitleJobs shares ffmpeg extractions between identical requests and keeps finished
// windows briefly, so a retry or a repeated seek never reads the swarm twice.
// The zero value is ready to use.
type subtitleJobs struct {
	mu      sync.Mutex
	jobs    map[string]*subtitleJob
	running int // unfinished jobs, bounded by subtitleMaxConcurrent
}

func (j *subtitleJobs) purgeLocked() {
	now := time.Now()
	oldest, oldestKey, finished := now, "", 0
	for key, job := range j.jobs {
		if job.finished.IsZero() {
			continue
		}
		finished++
		if now.Sub(job.finished) > subtitleResultTTL {
			delete(j.jobs, key)
			finished--
		} else if job.finished.Before(oldest) {
			oldest, oldestKey = job.finished, key
		}
	}
	if finished > subtitleResultEntries && oldestKey != "" {
		delete(j.jobs, oldestKey)
	}
}

// join attaches the caller to the job for key, starting it when none is running or cached.
// It returns nil when a new extraction is needed but subtitleMaxConcurrent are already running.
func (j *subtitleJobs) join(key string, logger *slog.Logger, args []string, ffmpeg string) *subtitleJob {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.jobs == nil {
		j.jobs = map[string]*subtitleJob{}
	}
	j.purgeLocked()
	job := j.jobs[key]
	if job == nil {
		if j.running >= subtitleMaxConcurrent {
			return nil
		}
		j.running++
		ctx, cancel := context.WithTimeout(context.Background(), subtitleJobTimeout)
		job = &subtitleJob{done: make(chan struct{}), cancel: cancel}
		j.jobs[key] = job
		go j.run(key, job, ctx, logger, ffmpeg, args)
	}
	job.waiters++
	if job.linger != nil {
		job.linger.Stop()
		job.linger = nil
	}
	return job
}

func (j *subtitleJobs) run(key string, job *subtitleJob, ctx context.Context, logger *slog.Logger, ffmpeg string, args []string) {
	defer job.cancel()
	var out cappedBuffer
	out.limit = subtitleResultMaxSize
	var stderr diagnostics.LimitedBuffer
	cmd := exec.CommandContext(ctx, ffmpeg, args...)
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil && ctx.Err() != nil {
		err = ctx.Err() // a killed process reports the signal, not why it was killed
	}
	if err == nil && out.exceeded {
		err = errSubtitleTooLarge
	}
	j.mu.Lock()
	j.running--
	job.err, job.stderr, job.finished = err, stderr.String(), time.Now()
	if err == nil {
		job.data = out.Bytes()
	} else {
		// Failures are never cached: the next request starts over.
		delete(j.jobs, key)
		if ctx.Err() == nil || job.waiters > 0 {
			logger.Error("failed to extract subtitles", "err", err, "stderr", job.stderr)
		}
	}
	if job.linger != nil {
		job.linger.Stop()
		job.linger = nil
	}
	j.mu.Unlock()
	close(job.done)
}

// leave detaches one waiter. A client that disconnected stops an unfinished extraction as soon
// as nobody else needs it; a request that merely timed out leaves it running for a retry.
func (j *subtitleJobs) leave(job *subtitleJob, disconnected bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	job.waiters--
	if job.waiters > 0 || !job.finished.IsZero() {
		return
	}
	if disconnected {
		job.cancel()
		return
	}
	job.linger = time.AfterFunc(subtitleJobLinger, func() {
		j.mu.Lock()
		defer j.mu.Unlock()
		if job.waiters == 0 && job.finished.IsZero() {
			job.cancel()
		}
	})
}

// cappedBuffer collects ffmpeg output up to a limit instead of growing without bound.
type cappedBuffer struct {
	bytes.Buffer
	limit    int
	exceeded bool
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > b.limit {
		b.exceeded = true
		return 0, fmt.Errorf("subtitle output exceeds %d bytes", b.limit)
	}
	return b.Buffer.Write(p)
}
