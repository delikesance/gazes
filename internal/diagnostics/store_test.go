package diagnostics

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func options(t *testing.T) Options {
	o := Defaults()
	o.Dir = t.TempDir()
	o.Output = io.Discard
	o.Errors = io.Discard
	return o
}
func closeStore(t *testing.T, s *Store) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if e := s.Close(ctx); e != nil {
		t.Fatal(e)
	}
}
func rows(t *testing.T, dir string) int {
	t.Helper()
	db, e := sql.Open("sqlite3", filepath.Join(dir, "events.sqlite"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	var n int
	if e = db.QueryRow("SELECT count(*) FROM events").Scan(&n); e != nil {
		t.Fatal(e)
	}
	return n
}
func TestPersistentSanitizedCorrelationAndLevel(t *testing.T) {
	o := options(t)
	var stdout bytes.Buffer
	o.Output = &stdout
	o.Level = slog.LevelInfo
	s, e := Open(o)
	if e != nil {
		t.Fatal(e)
	}
	RegisterSecret("known-api-key")
	ctx := With(context.Background(), Correlation{RequestID: "request", SessionID: "session", AttemptID: "attempt", AnimeID: "101280", SeasonID: "101280", Episode: "1"})
	logger := slog.New(s.Handler())
	logger.DebugContext(ctx, "hidden")
	logger.InfoContext(ctx, "provider.completed", "provider", "anidex", "apikey", "secret-value", "reason", "url?apikey=known-api-key&token=raw-token", "nested", map[string]any{"authorization": "Bearer other-token"})
	closeStore(t, s)
	if rows(t, o.Dir) != 1 {
		t.Fatal("level ignored")
	}
	if strings.Contains(stdout.String(), "known-api-key") || strings.Contains(stdout.String(), "raw-token") || strings.Contains(stdout.String(), "other-token") {
		t.Fatalf("secret leak: %s", stdout.String())
	}
	var result bytes.Buffer
	_, e = Read(context.Background(), filepath.Join(o.Dir, "events.sqlite"), Filter{Session: "session", Anime: "101280", Episode: "1", Provider: "anidex"}, &result)
	if e != nil {
		t.Fatal(e)
	}
	var record Event
	if e = json.Unmarshal(bytes.TrimSpace(result.Bytes()), &record); e != nil || record.RequestID != "request" || record.AttemptID != "attempt" {
		t.Fatalf("missing correlation: %s %v", result.String(), e)
	}
	files, _ := filepath.Glob(filepath.Join(o.Dir, "gazes-*.jsonl"))
	data, _ := os.ReadFile(files[0])
	if !bytes.Equal(data, stdout.Bytes()) || !bytes.Equal(data, result.Bytes()) {
		t.Fatal("sinks differ")
	}
	if st, _ := os.Stat(o.Dir); st.Mode().Perm() != 0700 {
		t.Fatal("directory permissions")
	}
	if st, _ := os.Stat(files[0]); st.Mode().Perm() != 0600 {
		t.Fatal("file permissions")
	}
}
func TestReplayNoDuplicatesAndMigration(t *testing.T) {
	o := options(t)
	s, _ := Open(o)
	slog.New(s.Handler()).Info("initial")
	closeStore(t, s)
	s, _ = Open(o)
	slog.New(s.Handler()).Info("second")
	closeStore(t, s)
	if rows(t, o.Dir) != 2 {
		t.Fatal("replay duplicates")
	}
	db, _ := sql.Open("sqlite3", filepath.Join(o.Dir, "events.sqlite"))
	defer db.Close()
	_, _ = db.Exec("DELETE FROM checkpoints")
	s, _ = Open(o)
	closeStore(t, s)
	if rows(t, o.Dir) != 2 {
		t.Fatal("ID deduplication failed")
	}
	var version int
	db.QueryRow("PRAGMA user_version").Scan(&version)
	if version != 1 {
		t.Fatal("migration missing")
	}
}

type gateWriter struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (g *gateWriter) Write(p []byte) (int, error) {
	g.once.Do(func() { close(g.entered) })
	<-g.release
	return len(p), nil
}
func TestQueueSaturationNeverBlocksCaller(t *testing.T) {
	o := options(t)
	g := &gateWriter{entered: make(chan struct{}), release: make(chan struct{})}
	o.Output = g
	o.QueueSize = 2
	s, _ := Open(o)
	logger := slog.New(s.Handler())
	logger.Info("first")
	<-g.entered
	started := time.Now()
	for range 10000 {
		logger.Debug("noisy")
	}
	if time.Since(started) > time.Second {
		t.Fatal("caller blocked")
	}
	if s.Dropped() == 0 {
		t.Fatal("lost events not counted")
	}
	close(g.release)
	closeStore(t, s)
}
func TestUnavailableDirectoryKeepsStdout(t *testing.T) {
	o := options(t)
	path := filepath.Join(o.Dir, "not-a-directory")
	os.WriteFile(path, []byte("file"), 0600)
	o.Dir = path
	var out bytes.Buffer
	o.Output = &out
	s, e := Open(o)
	if e != nil {
		t.Fatal("logging prevents startup")
	}
	slog.New(s.Handler()).Error("playback.failed")
	closeStore(t, s)
	if !strings.Contains(out.String(), "playback.failed") {
		t.Fatal("lost fallback")
	}
}
func TestFullDiskReportsFailure(t *testing.T) {
	f, e := os.OpenFile("/dev/full", os.O_WRONLY, 0600)
	if e != nil {
		t.Skip("requires /dev/full")
	}
	defer f.Close()
	o := options(t)
	var out, errs bytes.Buffer
	o.Output = &out
	o.Errors = &errs
	s := &Store{o: o, file: f}
	s.append(Event{ID: "x", Event: "still-playing"})
	if !strings.Contains(out.String(), "still-playing") || !strings.Contains(errs.String(), "degraded") {
		t.Fatal("disk failure not visible")
	}
}
func TestRotationAndRetention(t *testing.T) {
	o := options(t)
	o.FileSize = 500
	o.FilesMax = 1000
	s, _ := Open(o)
	logger := slog.New(s.Handler())
	for range 15 {
		logger.Info("rotate", "detail", strings.Repeat("a", 100))
	}
	closeStore(t, s)
	files, _ := filepath.Glob(filepath.Join(o.Dir, "gazes-*.jsonl"))
	if len(files) < 2 {
		t.Fatal("rotation missing")
	}
	old := time.Now().Add(-10 * 24 * time.Hour)
	for _, f := range files {
		os.Chtimes(f, old, old)
	}
	s, _ = Open(o)
	closeStore(t, s)
	files, _ = filepath.Glob(filepath.Join(o.Dir, "gazes-*.jsonl"))
	if len(files) != 0 {
		t.Fatal("old files retained")
	}
}
func TestSQLiteLockedRecoveryAndShutdown(t *testing.T) {
	o := options(t)
	s, _ := Open(o)
	logger := slog.New(s.Handler())
	logger.Info("before")
	deadline := time.Now().Add(3 * time.Second)
	path := filepath.Join(o.Dir, "events.sqlite")
	for {
		if _, e := os.Stat(path); e == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("db not created")
		}
		time.Sleep(10 * time.Millisecond)
	}
	db, _ := sql.Open("sqlite3", path+"?_busy_timeout=100")
	defer db.Close()
	if _, e := db.Exec("BEGIN IMMEDIATE"); e != nil {
		t.Fatal(e)
	}
	logger.Error("during-lock")
	time.Sleep(1200 * time.Millisecond)
	files, _ := filepath.Glob(filepath.Join(o.Dir, "gazes-*.jsonl"))
	var found bool
	for _, f := range files {
		data, _ := os.ReadFile(f)
		found = found || bytes.Contains(data, []byte("during-lock"))
	}
	if !found {
		t.Fatal("SQLite lock prevented JSONL")
	}
	_, _ = db.Exec("ROLLBACK")
	closeStore(t, s)
	if rows(t, o.Dir) != 2 {
		t.Fatal("missed replay after recovery")
	}
}
func TestBoundedSubprocessOutput(t *testing.T) {
	b := &LimitedBuffer{Limit: 10}
	n, e := b.Write([]byte(strings.Repeat("x", 100)))
	if n != 100 || e != nil || len(b.Data) != 10 || !strings.Contains(b.String(), "TRUNCATED") {
		t.Fatal("unbounded stderr")
	}
}

func TestRedactsHeadersEncodedURLsAndStructuredSecrets(t *testing.T) {
	for _, input := range []string{"Authorization: Bearer opaque-token", "https://user:password@host/path?api%5fkey=opaque-token&keep=yes", "magnet:?xt=urn:btih:abcd&tr=http://private", "cookie=session-secret"} {
		text := Redact(input)
		for _, secret := range []string{"opaque-token", "password@", "session-secret", "urn:btih:abcd"} {
			if strings.Contains(text, secret) {
				t.Fatalf("leak in %s", text)
			}
		}
	}
	type nested struct {
		Password string `json:"password"`
		Value    string `json:"value"`
	}
	cleaned := clean("object", nested{Password: "private", Value: "normal"})
	data, _ := json.Marshal(cleaned)
	if bytes.Contains(data, []byte("private")) {
		t.Fatal("typed object secret leaked")
	}
}
func TestRetentionPurgesSQLiteAndCapsFiles(t *testing.T) {
	o := options(t)
	o.FileSize = 500
	o.FilesMax = 1200
	s, _ := Open(o)
	logger := slog.New(s.Handler())
	for range 30 {
		logger.Info("cap", "value", strings.Repeat("x", 100))
	}
	closeStore(t, s)
	s, _ = Open(o)
	closeStore(t, s)
	files, _ := filepath.Glob(filepath.Join(o.Dir, "gazes-*.jsonl"))
	var total int64
	for _, f := range files {
		st, _ := os.Stat(f)
		total += st.Size()
	}
	if total > o.FilesMax {
		t.Fatalf("file cap exceeded: %d", total)
	}
	db, _ := sql.Open("sqlite3", filepath.Join(o.Dir, "events.sqlite"))
	_, err := db.Exec("UPDATE events SET timestamp='2000-01-01T00:00:00Z'")
	db.Close()
	if err != nil {
		t.Fatal(err)
	}
	s, _ = Open(o)
	closeStore(t, s)
	if rows(t, o.Dir) != 0 {
		t.Fatal("old database events retained")
	}
}
func TestFollowCursorSurvivesPruning(t *testing.T) {
	o := options(t)
	s, _ := Open(o)
	slog.New(s.Handler()).Info("before-prune")
	closeStore(t, s)
	var out bytes.Buffer
	last, err := Read(context.Background(), filepath.Join(o.Dir, "events.sqlite"), Filter{}, &out)
	if err != nil {
		t.Fatal(err)
	}
	db, _ := sql.Open("sqlite3", filepath.Join(o.Dir, "events.sqlite"))
	db.Exec("DELETE FROM events")
	db.Close()
	s, _ = Open(o)
	slog.New(s.Handler()).Info("after-prune")
	closeStore(t, s)
	out.Reset()
	_, err = Read(context.Background(), filepath.Join(o.Dir, "events.sqlite"), Filter{After: last}, &out)
	if err != nil || !strings.Contains(out.String(), "after-prune") {
		t.Fatalf("follow skipped new events: %v %s", err, out.String())
	}
}

func TestTimeFilters(t *testing.T) {
	o := options(t)
	s, _ := Open(o)
	logger := slog.New(s.Handler())
	logger.Info("time-filtered")
	closeStore(t, s)
	var out bytes.Buffer
	_, err := Read(context.Background(), filepath.Join(o.Dir, "events.sqlite"), Filter{Since: time.Now().Add(-time.Minute).UTC().Format(time.RFC3339), Until: time.Now().Add(time.Minute).UTC().Format(time.RFC3339)}, &out)
	if err != nil || !strings.Contains(out.String(), "time-filtered") {
		t.Fatalf("time range failed: %v %s", err, out.String())
	}
	if _, err = Read(context.Background(), filepath.Join(o.Dir, "events.sqlite"), Filter{Since: "bad-date"}, io.Discard); err == nil {
		t.Fatal("bad date accepted")
	}
}

// Many small events: the four indexes and the AUTOINCREMENT table cost more than payload+256, so the
// logical size used to stay under the cap while the file hit max_page_count and every insert failed
// with "database or disk is full" (production, 2026-10-06).
func TestMaintenanceKeepsDatabaseUnderItsPageCap(t *testing.T) {
	o := options(t)
	var errs bytes.Buffer
	o.Errors = &errs
	o.DBMax = 16<<20 + 4<<20 // 4 MiB of database
	n := 0
	for round := 0; round < 40; round++ {
		s, err := Open(o)
		if err != nil {
			t.Fatal(err)
		}
		logger := slog.New(s.Handler())
		for range 1500 {
			n++
			logger.Info("e"+strings.Repeat("x", n%7), "n", n)
		}
		closeStore(t, s)
	}
	if strings.Contains(errs.String(), "full") {
		t.Fatalf("database filled up despite maintenance: %s", errs.String())
	}
	if rows(t, o.Dir) == 0 {
		t.Fatal("no events kept")
	}
	st, _ := os.Stat(filepath.Join(o.Dir, "events.sqlite"))
	if st.Size() > oDBLimit(o.DBMax) {
		t.Fatalf("file %d exceeds cap %d", st.Size(), oDBLimit(o.DBMax))
	}
}

func TestFutureSchemaIsNotOverwritten(t *testing.T) {
	o := options(t)
	db, _ := sql.Open("sqlite3", filepath.Join(o.Dir, "events.sqlite"))
	db.Exec("PRAGMA user_version=99")
	db.Close()
	var stdout bytes.Buffer
	o.Output = &stdout
	s, _ := Open(o)
	slog.New(s.Handler()).Warn("fallback-file")
	closeStore(t, s)
	db, _ = sql.Open("sqlite3", filepath.Join(o.Dir, "events.sqlite"))
	defer db.Close()
	var version int
	db.QueryRow("PRAGMA user_version").Scan(&version)
	if version != 99 || !strings.Contains(stdout.String(), "fallback-file") {
		t.Fatal("future schema mutated or fallback lost")
	}
}
