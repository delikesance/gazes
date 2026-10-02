package diagnostics

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
)

type Options struct {
	Dir                       string
	Level                     slog.Level
	Retention                 time.Duration
	FileSize, FilesMax, DBMax int64
	QueueSize                 int
	Output, Errors            io.Writer
}

func Defaults() Options {
	return Options{Dir: "./diagnostics", Level: slog.LevelDebug, Retention: 7 * 24 * time.Hour, FileSize: 50 << 20, FilesMax: 1 << 30, DBMax: 1 << 30, QueueSize: 4096, Output: os.Stdout, Errors: os.Stderr}
}
func FromEnv(level string) Options {
	o := Defaults()
	_ = o.Level.UnmarshalText([]byte(level))
	if s := os.Getenv("LOG_DIR"); s != "" {
		o.Dir = s
	}
	if d, e := time.ParseDuration(os.Getenv("LOG_RETENTION")); e == nil && d > 0 {
		o.Retention = d
	}
	for k, p := range map[string]*int64{"LOG_FILE_BYTES": &o.FileSize, "LOG_FILES_MAX_BYTES": &o.FilesMax, "LOG_DB_MAX_BYTES": &o.DBMax} {
		if n, e := strconv.ParseInt(os.Getenv(k), 10, 64); e == nil && n >= 65536 {
			*p = n
		}
	}
	return o
}

type Event struct {
	ID        string `json:"id"`
	Timestamp string `json:"timestamp"`
	Level     string `json:"level"`
	Service   string `json:"service"`
	Event     string `json:"event"`
	Correlation
	Provider   string         `json:"provider,omitempty"`
	InfoHash   string         `json:"infohash,omitempty"`
	Attributes map[string]any `json:"attributes,omitempty"`
}
type Store struct {
	o               Options
	queue           chan Event
	stop, done      chan struct{}
	closing         atomic.Bool
	enqueueMu       sync.RWMutex
	dropped         atomic.Uint64
	db              *sql.DB
	file            *os.File
	fileName        string
	size            int64
	lastMaintenance time.Time
	lastWarning     time.Time
}
type Handler struct {
	store  *Store
	attrs  []slog.Attr
	groups []string
}

func Open(o Options) (*Store, error) {
	if o.FilesMax > 0 && o.FileSize > o.FilesMax {
		o.FileSize = o.FilesMax
	}
	if o.QueueSize < 1 {
		o.QueueSize = 4096
	}
	if o.Output == nil {
		o.Output = io.Discard
	}
	if o.Errors == nil {
		o.Errors = io.Discard
	}
	dirErr := os.MkdirAll(o.Dir, 0700)
	_ = os.Chmod(o.Dir, 0700)
	s := &Store{o: o, queue: make(chan Event, o.QueueSize), stop: make(chan struct{}), done: make(chan struct{})}
	if dirErr != nil {
		s.warn(dirErr)
	}
	go s.run()
	return s, nil
}
func (s *Store) Handler() slog.Handler                          { return &Handler{store: s} }
func (h *Handler) Enabled(_ context.Context, l slog.Level) bool { return l >= h.store.o.Level }
func (h *Handler) WithAttrs(a []slog.Attr) slog.Handler {
	n := *h
	n.attrs = append(append([]slog.Attr{}, h.attrs...), a...)
	return &n
}
func (h *Handler) WithGroup(g string) slog.Handler {
	n := *h
	n.groups = append(append([]string{}, h.groups...), g)
	return &n
}
func (h *Handler) Handle(ctx context.Context, r slog.Record) error {
	attrs := map[string]any{}
	add := func(a slog.Attr) {
		if a.Equal(slog.Attr{}) {
			return
		}
		a.Value = a.Value.Resolve()
		key := strings.Join(append(append([]string{}, h.groups...), a.Key), ".")
		if a.Value.Kind() == slog.KindGroup {
			m := map[string]any{}
			for _, v := range a.Value.Group() {
				m[v.Key] = clean(v.Key, v.Value.Resolve().Any())
			}
			attrs[key] = clean(key, m)
		} else {
			attrs[key] = clean(key, a.Value.Any())
		}
	}
	for _, a := range h.attrs {
		add(a)
	}
	r.Attrs(func(a slog.Attr) bool { add(a); return true })
	e := Event{ID: uuid.NewString(), Timestamp: timestamp(r.Time), Level: r.Level.String(), Service: "backend", Event: Redact(r.Message), Correlation: Get(ctx), Attributes: attrs}
	for k, p := range map[string]*string{"request_id": &e.RequestID, "playback_session_id": &e.SessionID, "attempt_id": &e.AttemptID, "anime_id": &e.AnimeID, "season_id": &e.SeasonID, "episode": &e.Episode} {
		if v, ok := attrs[k].(string); ok && v != "" {
			*p = v
		}
	}
	if s, ok := attrs["service"].(string); ok {
		e.Service = s
	}
	for _, p := range []*string{&e.RequestID, &e.SessionID, &e.AttemptID, &e.AnimeID, &e.SeasonID, &e.Episode} {
		*p = Redact(*p)
	}
	e.Provider, _ = attrs["provider"].(string)
	e.InfoHash, _ = attrs["infohash"].(string)
	if len(e.Event) > 1024 {
		e.Event = e.Event[:1024] + " [TRUNCATED]"
	}
	if b, _ := json.Marshal(e); len(b) > 16384 {
		e.Attributes = map[string]any{"truncated": true, "original_bytes": len(b)}
	}
	h.store.enqueueMu.RLock()
	defer h.store.enqueueMu.RUnlock()
	if h.store.closing.Load() {
		h.store.dropped.Add(1)
		return nil
	}
	select {
	case h.store.queue <- e:
	default:
		h.store.dropped.Add(1)
	}
	return nil
}
func (s *Store) Dropped() uint64 { return s.dropped.Load() }
func (s *Store) Close(ctx context.Context) error {
	s.enqueueMu.Lock()
	if s.closing.CompareAndSwap(false, true) {
		close(s.stop)
	}
	s.enqueueMu.Unlock()
	select {
	case <-s.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (s *Store) warn(err error) {
	if time.Since(s.lastWarning) > time.Second {
		fmt.Fprintln(s.o.Errors, "diagnostics degraded:", Redact(err.Error()))
		s.lastWarning = time.Now()
	}
}
func (s *Store) connect() error {
	db, err := sql.Open("sqlite3", filepath.Join(s.o.Dir, "events.sqlite")+"?_journal_mode=WAL&_busy_timeout=100&_synchronous=NORMAL&_auto_vacuum=incremental")
	if err != nil {
		return err
	}
	db.SetMaxOpenConns(1)
	var version int
	if err = db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		db.Close()
		return err
	}
	if version > 1 {
		db.Close()
		return fmt.Errorf("unsupported diagnostics schema version %d", version)
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS events (
 seq INTEGER PRIMARY KEY AUTOINCREMENT, id TEXT UNIQUE, timestamp TEXT NOT NULL, level TEXT, service TEXT, event TEXT,
 request_id TEXT, playback_session_id TEXT, attempt_id TEXT, anime_id TEXT, season_id TEXT, episode TEXT,
 provider TEXT, infohash TEXT, payload TEXT NOT NULL);
 CREATE INDEX IF NOT EXISTS events_time ON events(timestamp);
 CREATE INDEX IF NOT EXISTS events_session ON events(playback_session_id,timestamp);
 CREATE INDEX IF NOT EXISTS events_episode ON events(anime_id,season_id,episode,timestamp);
 CREATE INDEX IF NOT EXISTS events_provider ON events(provider,timestamp);
 CREATE TABLE IF NOT EXISTS checkpoints(path TEXT PRIMARY KEY, offset INTEGER NOT NULL);
 PRAGMA user_version=1; PRAGMA journal_size_limit=16777216; PRAGMA wal_autocheckpoint=256;`)
	if err != nil {
		db.Close()
		return err
	}
	var pageSize int64
	_ = db.QueryRow("PRAGMA page_size").Scan(&pageSize)
	if pageSize > 0 {
		_, err = db.Exec(fmt.Sprintf("PRAGMA max_page_count=%d", max(int64(32), (oDBLimit(s.o.DBMax))/pageSize)))
		if err != nil {
			db.Close()
			return err
		}
	}
	s.db = db
	_ = os.Chmod(filepath.Join(s.o.Dir, "events.sqlite"), 0600)
	return nil
}
func (s *Store) rotate() error {
	if s.file != nil {
		_ = s.file.Close()
		s.file = nil
	}
	s.fileName = "gazes-" + time.Now().UTC().Format("2006-01-02") + "-" + fmt.Sprintf("%019d", time.Now().UnixNano()) + ".jsonl"
	f, err := os.OpenFile(filepath.Join(s.o.Dir, s.fileName), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	s.file = f
	s.size = 0
	return nil
}
func (s *Store) append(e Event) {
	b, err := json.Marshal(e)
	if err != nil {
		s.warn(err)
		return
	}
	b = append(b, '\n')
	_, _ = s.o.Output.Write(b)
	if s.file == nil || s.size+int64(len(b)) > s.o.FileSize {
		if err := s.rotate(); err != nil {
			s.warn(err)
			return
		}
	}
	n, err := s.file.Write(b)
	s.size += int64(n)
	if err != nil {
		s.warn(err)
		_ = s.file.Close()
		s.file = nil
	}
}

// Replay checkpointed JSONL in bounded transactions. The files remain the source
// of truth if SQLite is locked; IDs and offsets make recovery idempotent.
func (s *Store) replay() error {
	if s.db == nil {
		if err := s.connect(); err != nil {
			return err
		}
	}
	files, _ := filepath.Glob(filepath.Join(s.o.Dir, "gazes-*.jsonl"))
	sort.Strings(files)
	for _, path := range files {
		name := filepath.Base(path)
		var offset int64
		err := s.db.QueryRow("SELECT offset FROM checkpoints WHERE path=?", name).Scan(&offset)
		if err != nil && err != sql.ErrNoRows {
			return err
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		if _, err = f.Seek(offset, io.SeekStart); err != nil {
			f.Close()
			return err
		}
		reader := bufio.NewReader(f)
		for {
			lines := make([][]byte, 0, 200)
			end := offset
			for len(lines) < 200 {
				line, err := reader.ReadBytes('\n')
				if err != nil {
					break
				}
				end += int64(len(line))
				lines = append(lines, line)
			}
			if len(lines) == 0 {
				break
			}
			tx, err := s.db.Begin()
			if err != nil {
				f.Close()
				return err
			}
			for _, line := range lines {
				var e Event
				if json.Unmarshal(line, &e) != nil || e.ID == "" {
					continue
				}
				if parsed, e2 := time.Parse(time.RFC3339Nano, e.Timestamp); e2 == nil {
					e.Timestamp = timestamp(parsed)
				}
				_, err = tx.Exec(`INSERT OR IGNORE INTO events(id,timestamp,level,service,event,request_id,playback_session_id,attempt_id,anime_id,season_id,episode,provider,infohash,payload) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, e.ID, e.Timestamp, e.Level, e.Service, e.Event, e.RequestID, e.SessionID, e.AttemptID, e.AnimeID, e.SeasonID, e.Episode, e.Provider, e.InfoHash, string(line))
				if err != nil {
					break
				}
			}
			if err == nil {
				_, err = tx.Exec("INSERT OR REPLACE INTO checkpoints VALUES(?,?)", name, end)
			}
			if err != nil {
				_ = tx.Rollback()
				f.Close()
				return err
			}
			if err = tx.Commit(); err != nil {
				f.Close()
				return err
			}
			offset = end
		}
		f.Close()
	}
	return nil
}
func (s *Store) maintain() {
	files, _ := filepath.Glob(filepath.Join(s.o.Dir, "gazes-*.jsonl"))
	sort.Strings(files)
	var total int64
	for _, p := range files {
		if st, e := os.Stat(p); e == nil {
			total += st.Size()
		}
	}
	cutoff := time.Now().Add(-s.o.Retention)
	for _, p := range files {
		st, e := os.Stat(p)
		if e != nil || filepath.Base(p) == s.fileName {
			continue
		}
		if st.ModTime().Before(cutoff) || total+max(int64(0), s.o.FileSize-s.size) > s.o.FilesMax {
			if os.Remove(p) == nil {
				total -= st.Size()
				if s.db != nil {
					_, _ = s.db.Exec("DELETE FROM checkpoints WHERE path=?", filepath.Base(p))
				}
			}
		}
	}
	if s.db == nil {
		return
	}
	_, err := s.db.Exec("DELETE FROM events WHERE timestamp < ?", timestamp(cutoff))
	if err != nil {
		s.warn(err)
		return
	}
	// Bound logical content as well as allocated pages, leaving headroom for WAL.
	limit := max(int64(65536), s.o.DBMax-(16<<20))
	var bytes int64
	_ = s.db.QueryRow("SELECT COALESCE(SUM(length(payload)+256),0) FROM events").Scan(&bytes)
	for bytes > limit {
		res, err := s.db.Exec("DELETE FROM events WHERE id IN (SELECT id FROM events ORDER BY timestamp LIMIT 1000)")
		if err != nil {
			s.warn(err)
			break
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			break
		}
		_ = s.db.QueryRow("SELECT COALESCE(SUM(length(payload)+256),0) FROM events").Scan(&bytes)
	}
	_, _ = s.db.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
	_, _ = s.db.Exec("PRAGMA incremental_vacuum(4096)")
	if st, e := os.Stat(filepath.Join(s.o.Dir, "events.sqlite")); e == nil && st.Size() > limit {
		_, _ = s.db.Exec("VACUUM")
	}
	s.lastMaintenance = time.Now()
}
func (s *Store) run() {
	defer close(s.done)
	defer func() {
		if s.file != nil {
			_ = s.file.Sync()
			_ = s.file.Close()
		}
		if s.db != nil {
			s.db.Close()
		}
	}()
	if err := s.replay(); err != nil {
		s.warn(err)
	}
	s.maintain()
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	pending := 0
	var reported uint64
	flush := func() {
		if s.file != nil {
			_ = s.file.Sync()
		}
		if err := s.replay(); err != nil {
			s.warn(err)
		}
		pending = 0
		if time.Since(s.lastMaintenance) > time.Minute {
			s.maintain()
		}
	}
	for {
		select {
		case e := <-s.queue:
			s.append(e)
			pending++
			if pending >= 200 {
				flush()
			}
		case <-tick.C:
			if d := s.dropped.Load(); d != reported {
				s.append(Event{ID: uuid.NewString(), Timestamp: timestamp(time.Now()), Level: "WARN", Service: "backend", Event: "diagnostics.events_dropped", Attributes: map[string]any{"total": d, "since_last": d - reported}})
				reported = d
			}
			flush()
		case <-s.stop:
			for {
				select {
				case e := <-s.queue:
					s.append(e)
				default:
					if d := s.dropped.Load(); d != reported {
						s.append(Event{ID: uuid.NewString(), Timestamp: timestamp(time.Now()), Level: "WARN", Service: "backend", Event: "diagnostics.events_dropped", Attributes: map[string]any{"total": d, "since_last": d - reported}})
					}
					flush()
					return
				}
			}
		}
	}
}

// LimitedBuffer captures subprocess diagnostics without retaining unbounded stderr.
type LimitedBuffer struct {
	mu        sync.Mutex
	Data      []byte
	Limit     int
	Truncated bool
}

func (b *LimitedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	limit := b.Limit
	if limit == 0 {
		limit = 16384
	}
	n := min(len(p), max(0, limit-len(b.Data)))
	b.Data = append(b.Data, p[:n]...)
	if n < len(p) {
		b.Truncated = true
	}
	return len(p), nil
}
func (b *LimitedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	s := string(b.Data)
	if b.Truncated {
		s += " [TRUNCATED]"
	}
	return Redact(s)
}

func oDBLimit(maxBytes int64) int64 { return max(int64(65536), maxBytes-(16<<20)) }

func timestamp(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05.000000000Z") }
