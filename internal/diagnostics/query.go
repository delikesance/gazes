package diagnostics

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"strings"
	"time"
)

type Filter struct {
	Since, Until, Level, Service, Event, Episode, Anime, Season, Provider, Session, Request string
	Limit                                                                                   int
	After                                                                                   int64
	Follow                                                                                  bool
}

func Read(ctx context.Context, path string, f Filter, w io.Writer) (int64, error) {
	for _, p := range []*string{&f.Since, &f.Until} {
		if *p != "" {
			t, e := time.Parse(time.RFC3339Nano, *p)
			if e != nil {
				return f.After, fmt.Errorf("time filters require RFC3339 UTC timestamps")
			}
			*p = timestamp(t)
		}
	}
	db, err := sql.Open("sqlite3", "file:"+path+"?mode=ro&_busy_timeout=1000")
	if err != nil {
		return f.After, err
	}
	defer db.Close()
	where := []string{"rowid > ?"}
	args := []any{f.After}
	for _, p := range []struct{ column, value string }{{"level", strings.ToUpper(f.Level)}, {"service", f.Service}, {"event", f.Event}, {"episode", f.Episode}, {"anime_id", f.Anime}, {"season_id", f.Season}, {"provider", f.Provider}, {"playback_session_id", f.Session}, {"request_id", f.Request}} {
		if p.value != "" {
			where = append(where, p.column+" = ?")
			args = append(args, p.value)
		}
	}
	if f.Since != "" {
		where = append(where, "timestamp >= ?")
		args = append(args, f.Since)
	}
	if f.Until != "" {
		where = append(where, "timestamp <= ?")
		args = append(args, f.Until)
	}
	if f.Limit <= 0 {
		f.Limit = 1000
	}
	args = append(args, f.Limit)
	order := "timestamp,rowid"
	if f.Follow || f.After > 0 {
		order = "rowid"
	}
	rows, err := db.QueryContext(ctx, "SELECT rowid,payload FROM events WHERE "+strings.Join(where, " AND ")+" ORDER BY "+order+" LIMIT ?", args...)
	if err != nil {
		return f.After, err
	}
	defer rows.Close()
	last := f.After
	for rows.Next() {
		var row int64
		var payload string
		if err = rows.Scan(&row, &payload); err != nil {
			return last, err
		}
		if _, err = fmt.Fprintln(w, strings.TrimSpace(payload)); err != nil {
			return last, err
		}
		last = max(last, row)
	}
	return last, rows.Err()
}
