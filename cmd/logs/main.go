// gazes-logs queries local persistent diagnostics; it never opens a network port.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/gazes/gazes/internal/diagnostics"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type output struct{ json bool }

func (o output) Write(p []byte) (int, error) {
	if o.json {
		return os.Stdout.Write(p)
	}
	var e diagnostics.Event
	if json.Unmarshal(p, &e) != nil {
		return len(p), nil
	}
	_, err := fmt.Fprintf(os.Stdout, "%s %-5s %-9s %-38s session=%s attempt=%s %s\n", e.Timestamp, e.Level, e.Service, e.Event, e.SessionID, e.AttemptID, strings.TrimSpace(string(mustJSON(e.Attributes))))
	return len(p), err
}
func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }
func main() {
	var f diagnostics.Filter
	dir := os.Getenv("LOG_DIR")
	if dir == "" {
		dir = "./diagnostics"
	}
	db := flag.String("db", filepath.Join(dir, "events.sqlite"), "SQLite database path")
	flag.StringVar(&f.Since, "since", "", "UTC RFC3339 lower bound")
	flag.StringVar(&f.Until, "until", "", "UTC RFC3339 upper bound")
	flag.StringVar(&f.Level, "level", "", "level: DEBUG INFO WARN ERROR")
	flag.StringVar(&f.Service, "service", "", "backend or browser")
	flag.StringVar(&f.Event, "event", "", "exact event name")
	flag.StringVar(&f.Episode, "episode", "", "episode number")
	flag.StringVar(&f.Anime, "anime", "", "anime ID")
	flag.StringVar(&f.Season, "season", "", "season ID")
	flag.StringVar(&f.Provider, "provider", "", "provider name")
	flag.StringVar(&f.Session, "session", "", "playback session ID")
	flag.StringVar(&f.Request, "request", "", "request ID")
	flag.IntVar(&f.Limit, "limit", 1000, "maximum events per query; use a larger value for exports")
	follow := flag.Bool("follow", false, "follow new events")
	jsonl := flag.Bool("json", false, "export JSONL")
	flag.Parse()
	f.Follow = *follow
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	for {
		last, err := diagnostics.Read(ctx, *db, f, output{*jsonl})
		if err != nil && err != io.EOF {
			fmt.Fprintln(os.Stderr, "cannot read diagnostics:", diagnostics.Redact(err.Error()))
			os.Exit(1)
		}
		f.After = last
		if !*follow {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second):
		}
	}
}
