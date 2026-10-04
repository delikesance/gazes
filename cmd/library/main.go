// gazes-library inspects and administers the local AV1 episode library; it never opens a network port.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/gazes/gazes/internal/library"
)

const usage = `usage: gazes-library <command> [flags]

commands:
  status [--json]                           copies per state, disks, encoder, recent errors, AV1 savings
  list [--state S] [--season ID] [--json]   list copies
  delete <season> <episode> <lang>          remove a copy (file, then index entry)

environment: LIBRARY_INDEX_DIR, LIBRARY_POOL_DIR, LIBRARY_RESERVE_PERCENT, LIBRARY_RESERVE_BYTES`

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	done := make(chan int, 1)
	go func() { done <- run(os.Args[1:], os.Stdout, os.Stderr) }()
	select {
	case code := <-done:
		if code != 0 {
			os.Exit(code)
		}
	case <-ctx.Done():
		os.Exit(1)
	}
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

type opts struct {
	index, pool string
	percent     int
	bytes       int64
}

func (o *opts) bind(fs *flag.FlagSet) {
	pct, _ := strconv.Atoi(getenv("LIBRARY_RESERVE_PERCENT", "10"))
	b, err := strconv.ParseInt(getenv("LIBRARY_RESERVE_BYTES", "50000000000"), 10, 64)
	if err != nil {
		b = 50_000_000_000
	}
	fs.StringVar(&o.index, "index-dir", getenv("LIBRARY_INDEX_DIR", "./library-index"), "index directory")
	fs.StringVar(&o.pool, "pool-dir", getenv("LIBRARY_POOL_DIR", "./library-pool"), "pool root directory")
	fs.IntVar(&o.percent, "reserve-percent", pct, "disk reserve percentage")
	fs.Int64Var(&o.bytes, "reserve-bytes", b, "disk reserve in bytes")
}

func (o *opts) open() (*library.Store, *library.Pool, error) {
	store, err := library.OpenStore(o.index)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot open library index: %w", err)
	}
	return store, library.NewPool(o.pool, store, library.SysStatFS, o.percent, o.bytes), nil
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, usage)
		return 1
	}
	var err error
	switch args[0] {
	case "status":
		err = cmdStatus(args[1:], stdout, stderr)
	case "list":
		err = cmdList(args[1:], stdout)
	case "delete":
		err = cmdDelete(args[1:], stdout)
	case "help", "-h", "--help":
		fmt.Fprintln(stdout, usage)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command %q\n%s\n", args[0], usage)
		return 1
	}
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	if err != nil {
		fmt.Fprintln(stderr, "gazes-library:", err)
		return 1
	}
	return 0
}

func newFlags(name string, stderr io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	return fs
}

func humanSize(n int64) string {
	const unit = 1000
	if n < unit && n > -unit {
		return fmt.Sprintf("%d B", n)
	}
	f, exp := float64(n), 0
	for ; (f >= unit || f <= -unit) && exp < 5; exp++ {
		f /= unit
	}
	return fmt.Sprintf("%.1f %cB", f, "kMGTPE"[exp-1])
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

type diskView struct {
	Label    string `json:"label"`
	DiskID   string `json:"disk_id"`
	Capacity int64  `json:"capacity_bytes"`
	Free     int64  `json:"free_bytes"`
	Reserve  int64  `json:"reserve_bytes"`
	Used     int64  `json:"used_bytes"`
	Present  bool   `json:"present"`
}

type encodingView struct {
	Key      string  `json:"key"`
	Progress float64 `json:"progress"`
	Paused   bool    `json:"paused"`
}

type errorView struct {
	Key       string `json:"key"`
	Error     string `json:"error"`
	UpdatedAt string `json:"updated_at"`
}

type statusView struct {
	Counts         map[string]int `json:"counts"`
	Disks          []diskView     `json:"disks"`
	Encoding       *encodingView  `json:"encoding"`
	PendingEncodes int            `json:"pending_encodes"`
	RecentErrors   []errorView    `json:"recent_errors"`
	SavedBytes     int64          `json:"saved_bytes"`
}

func cmdStatus(args []string, stdout, stderr io.Writer) error {
	fs := newFlags("status", stderr)
	var o opts
	o.bind(fs)
	asJSON := fs.Bool("json", false, "JSON output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	store, pool, err := o.open()
	if err != nil {
		return err
	}
	defer store.Close()
	// Scan is idempotent; a pool that cannot be read only means no disk is reported as present.
	if _, _, serr := pool.Scan(); serr != nil {
		fmt.Fprintln(stderr, "warning: disk scan:", serr)
	}
	all, err := store.List(library.Filter{})
	if err != nil {
		return err
	}
	st := statusView{Counts: map[string]int{}, Disks: []diskView{}, RecentErrors: []errorView{}}
	var orig, av1 int64
	var errs []library.Entry
	for _, e := range all {
		st.Counts[string(e.State)]++
		if e.State == library.StateOriginal && e.EncodeSkipped == "" {
			st.PendingEncodes++
		}
		if e.State == library.StateAV1 && e.OriginalSizeBytes > 0 {
			orig += e.OriginalSizeBytes
			av1 += e.SizeBytes
		}
		if e.LastError != "" {
			errs = append(errs, e)
		}
	}
	st.SavedBytes = orig - av1
	for i := 0; i < len(errs); i++ { // most recent first
		for j := i + 1; j < len(errs); j++ {
			if errs[j].UpdatedAt.After(errs[i].UpdatedAt) {
				errs[i], errs[j] = errs[j], errs[i]
			}
		}
	}
	for i, e := range errs {
		if i == 5 {
			break
		}
		st.RecentErrors = append(st.RecentErrors, errorView{Key: e.Key.String(), Error: e.LastError, UpdatedAt: e.UpdatedAt.UTC().Format(time.RFC3339)})
	}
	for _, d := range pool.Disks() {
		st.Disks = append(st.Disks, diskView{Label: d.Label, DiskID: d.ID, Capacity: d.Capacity, Free: d.Free, Reserve: d.Reserve, Used: d.Capacity - d.Free, Present: d.Present})
	}
	es, err := store.EncoderStatus()
	if err != nil {
		return err
	}
	if es.Key != nil {
		st.Encoding = &encodingView{Key: es.Key.String(), Progress: es.Progress, Paused: es.Paused}
	}
	if *asJSON {
		return writeJSON(stdout, st)
	}
	printStatus(stdout, st)
	return nil
}

func printStatus(w io.Writer, st statusView) {
	fmt.Fprintln(w, "Copies:")
	for _, s := range []library.State{library.StateDownloading, library.StateOriginal, library.StateEncoding, library.StateAV1, library.StateUnavailable} {
		fmt.Fprintf(w, "  %-12s %d\n", s, st.Counts[string(s)])
	}
	fmt.Fprintln(w, "Disks:")
	if len(st.Disks) == 0 {
		fmt.Fprintln(w, "  none")
	}
	tw := tabwriter.NewWriter(w, 2, 0, 2, ' ', 0)
	for _, d := range st.Disks {
		fmt.Fprintf(tw, "  %s\t%s\tcapacity %s\tfree %s\treserve %s\tused %s\tpresent=%t\n", d.Label, d.DiskID, humanSize(d.Capacity), humanSize(d.Free), humanSize(d.Reserve), humanSize(d.Used), d.Present)
	}
	tw.Flush()
	if st.Encoding != nil {
		paused := ""
		if st.Encoding.Paused {
			paused = " (paused)"
		}
		fmt.Fprintf(w, "Encoding: %s %.0f%%%s, %d waiting\n", st.Encoding.Key, st.Encoding.Progress*100, paused, st.PendingEncodes)
	} else {
		fmt.Fprintf(w, "Encoding: idle, %d waiting\n", st.PendingEncodes)
	}
	fmt.Fprintln(w, "Recent errors:")
	if len(st.RecentErrors) == 0 {
		fmt.Fprintln(w, "  none")
	}
	for _, e := range st.RecentErrors {
		fmt.Fprintf(w, "  %s %s %s\n", e.UpdatedAt, e.Key, e.Error)
	}
	fmt.Fprintf(w, "Saved by AV1: %s\n", humanSize(st.SavedBytes))
}

type entryView struct {
	SeasonID    int    `json:"season_id"`
	Episode     int    `json:"episode"`
	Lang        string `json:"lang"`
	State       string `json:"state"`
	Title       string `json:"title"`
	DiskID      string `json:"disk_id"`
	RelPath     string `json:"rel_path"`
	SizeBytes   int64  `json:"size_bytes"`
	OriginalSz  int64  `json:"original_size_bytes"`
	VideoCodec  string `json:"video_codec"`
	Skipped     string `json:"encode_skipped"`
	LastError   string `json:"last_error"`
	UpdatedAt   string `json:"updated_at"`
	LastAccess  string `json:"last_access_at"`
	ReleaseName string `json:"release_name"`
}

func cmdList(args []string, stdout io.Writer) error {
	fs := newFlags("list", os.Stderr)
	var o opts
	o.bind(fs)
	state := fs.String("state", "", "only this state (DOWNLOADING ORIGINAL ENCODING AV1 UNAVAILABLE)")
	season := fs.Int("season", 0, "only this season ID")
	asJSON := fs.Bool("json", false, "JSON output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	f := library.Filter{SeasonID: *season}
	if *state != "" {
		s := library.State(strings.ToUpper(*state))
		switch s {
		case library.StateDownloading, library.StateOriginal, library.StateEncoding, library.StateAV1, library.StateUnavailable:
			f.States = []library.State{s}
		default:
			return fmt.Errorf("unknown state %q", *state)
		}
	}
	store, err := library.OpenStore(o.index)
	if err != nil {
		return fmt.Errorf("cannot open library index: %w", err)
	}
	defer store.Close()
	entries, err := store.List(f)
	if err != nil {
		return err
	}
	if *asJSON {
		rows := make([]entryView, 0, len(entries))
		for _, e := range entries {
			rows = append(rows, entryView{e.SeasonID, e.Episode, e.Lang, string(e.State), e.Title, e.DiskID, e.RelPath, e.SizeBytes, e.OriginalSizeBytes,
				e.VideoCodec, e.EncodeSkipped, e.LastError, e.UpdatedAt.UTC().Format(time.RFC3339), e.LastAccessAt.UTC().Format(time.RFC3339), e.ReleaseName})
		}
		return writeJSON(stdout, rows)
	}
	tw := tabwriter.NewWriter(stdout, 2, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "SEASON\tEP\tLANG\tSTATE\tSIZE\tORIGINAL\tTITLE")
	for _, e := range entries {
		fmt.Fprintf(tw, "%d\t%d\t%s\t%s\t%s\t%s\t%s\n", e.SeasonID, e.Episode, e.Lang, e.State, humanSize(e.SizeBytes), humanSize(e.OriginalSizeBytes), e.Title)
	}
	return tw.Flush()
}

func cmdDelete(args []string, stdout io.Writer) error {
	fs := newFlags("delete", os.Stderr)
	var o opts
	o.bind(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 3 {
		return errors.New("usage: delete <season> <episode> <lang>")
	}
	season, err1 := strconv.Atoi(fs.Arg(0))
	ep, err2 := strconv.Atoi(fs.Arg(1))
	lang := fs.Arg(2)
	if err1 != nil || err2 != nil || (lang != "vostfr" && lang != "vf") {
		return errors.New("usage: delete <season> <episode> <vostfr|vf>")
	}
	k := library.Key{SeasonID: season, Episode: ep, Lang: lang}
	store, pool, err := o.open()
	if err != nil {
		return err
	}
	defer store.Close()
	e, err := store.Get(k)
	if err != nil {
		return fmt.Errorf("copy %s: %w", k, err)
	}
	busy := func(s library.State) bool { return s == library.StateDownloading || s == library.StateEncoding }
	if busy(e.State) || (e.State == library.StateUnavailable && busy(e.PrevState)) {
		return fmt.Errorf("copy %s is %s; refusing to delete", k, e.State)
	}
	if e.DiskID != "" && e.RelPath != "" {
		if _, _, serr := pool.Scan(); serr != nil {
			fmt.Fprintln(os.Stderr, "warning: disk scan:", serr)
		}
		path, perr := pool.Path(e.DiskID, e.RelPath)
		if perr != nil {
			if errors.Is(perr, library.ErrDiskAbsent) {
				return fmt.Errorf("copy %s: disk %s is absent; entry kept", k, e.DiskID)
			}
			return perr
		}
		if rerr := os.Remove(path); rerr != nil && !errors.Is(rerr, os.ErrNotExist) {
			return fmt.Errorf("cannot remove %s: %w", path, rerr)
		}
	}
	if err := store.Delete(k); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "deleted %s\n", k)
	return nil
}
