package admin

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

const (
	settingRetentionDays       = "retention_days"
	defaultRetentionDays       = 180
	minRetentionDays           = 30
	maxRetentionDays           = 730
	watchRunsShown             = 10
	watchEffectsShown          = 20
	watchEffectVerdictImproved = "improved"
)

func (s *Service) mountWatch(r chi.Router) {
	r.With(s.Auth(ScopeDiagnosticsRead)).Get("/watch", s.handleWatch)
}

// RetentionDays is how long playback errors and the MCP call log are kept (settings, 30 to 730 days).
func (s *Service) RetentionDays(ctx context.Context) int {
	v, ok, err := s.getSetting(ctx, settingRetentionDays)
	if err != nil || !ok {
		return defaultRetentionDays
	}
	n := 0
	for _, c := range v {
		if c < '0' || c > '9' || n > 100000 {
			return defaultRetentionDays
		}
		n = n*10 + int(c-'0')
	}
	if n < minRetentionDays {
		return minRetentionDays
	}
	if n > maxRetentionDays {
		return maxRetentionDays
	}
	return n
}

// Prune deletes rows older than the retention from the tables that grow without bound, and the
// idempotency ledger after a week (a retry window far beyond any client).
func (s *Service) Prune(ctx context.Context) error {
	now := s.now()
	cutoff := now.Add(-time.Duration(s.RetentionDays(ctx)) * 24 * time.Hour).Unix()
	for _, q := range []struct {
		sql string
		at  int64
	}{
		{`DELETE FROM playback_errors WHERE ts < ?`, cutoff},
		{`DELETE FROM mcp_audit WHERE ts < ?`, cutoff},
		{`DELETE FROM action_idempotency WHERE created_at < ?`, now.Add(-7 * 24 * time.Hour).Unix()},
	} {
		if _, err := s.adminDB().ExecContext(ctx, q.sql, q.at); err != nil {
			return err
		}
	}
	return nil
}

// handleWatch: GET /watch (diagnostics:read). The stored state of every rule, the last runs and the
// before/after of the fixes triggered by a breach. Reading never evaluates anything. Feeds the
// "Surveillance automatique" section of "Claude et MCP" and the MCP tool get_watch_status.
func (s *Service) handleWatch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cfg := s.watchConfig()
	rules := []RuleState{}
	for _, d := range watchRules() {
		th := d.threshold(ctx, s)
		rs := RuleState{Rule: d.rule, Label: d.label, State: WatchNotMeasured, Threshold: th, Unit: d.unit, Detail: "pas encore évalué"}
		var val sql.NullFloat64
		var since int64
		var issue sql.NullString
		err := s.adminDB().QueryRowContext(ctx, `SELECT state, value, since, issue_id, detail FROM watch_state WHERE rule = ?`, d.rule).Scan(&rs.State, &val, &since, &issue, &rs.Detail)
		if err == nil {
			if val.Valid {
				v := val.Float64
				rs.Value = &v
			}
			t := time.Unix(since, 0).UTC().Format(time.RFC3339)
			rs.Since = &t
			if issue.Valid {
				id := issue.String
				rs.IssueID = &id
			}
		} else if err != sql.ErrNoRows {
			pbServerError(w, err)
			return
		}
		rules = append(rules, rs)
	}

	type run struct {
		TS            string `json:"ts"`
		Breached      int    `json:"breached"`
		DurationMS    int64  `json:"duration_ms"`
		WebhookSent   int    `json:"webhook_sent"`
		WebhookFailed int    `json:"webhook_failed"`
	}
	runs := []run{}
	rows, err := s.adminDB().QueryContext(ctx, `SELECT ts, breached, duration_ms, webhook_sent, webhook_failed FROM watch_runs ORDER BY id DESC LIMIT ?`, watchRunsShown)
	if err != nil {
		pbServerError(w, err)
		return
	}
	for rows.Next() {
		var x run
		var ts int64
		if rows.Scan(&ts, &x.Breached, &x.DurationMS, &x.WebhookSent, &x.WebhookFailed) == nil {
			x.TS = time.Unix(ts, 0).UTC().Format(time.RFC3339)
			runs = append(runs, x)
		}
	}
	rows.Close()

	type effect struct {
		IssueID       string   `json:"issue_id"`
		Rule          string   `json:"rule"`
		Title         string   `json:"title"`
		Status        string   `json:"status"`
		ValueOpen     *float64 `json:"value_open"`
		ValueResolved *float64 `json:"value_resolved"`
		ValueAfter    *float64 `json:"value_after"`
		OpenedAt      string   `json:"opened_at"`
		ResolvedAt    *string  `json:"resolved_at"`
		Verdict       string   `json:"verdict"` // open | pending | improved | not_improved
	}
	effects := []effect{}
	er, err := s.adminDB().QueryContext(ctx,
		`SELECT e.issue_id, e.rule, i.title, i.status, e.value_open, e.value_resolved, e.value_after, e.opened_at, e.resolved_at
		 FROM watch_effects e JOIN issues i ON i.id = e.issue_id ORDER BY e.opened_at DESC LIMIT ?`, watchEffectsShown)
	if err != nil {
		pbServerError(w, err)
		return
	}
	for er.Next() {
		var x effect
		var vo, vr, va sql.NullFloat64
		var opened int64
		var resolved sql.NullInt64
		if er.Scan(&x.IssueID, &x.Rule, &x.Title, &x.Status, &vo, &vr, &va, &opened, &resolved) != nil {
			continue
		}
		x.OpenedAt = time.Unix(opened, 0).UTC().Format(time.RFC3339)
		if vo.Valid {
			x.ValueOpen = &vo.Float64
		}
		if vr.Valid {
			x.ValueResolved = &vr.Float64
		}
		if va.Valid {
			x.ValueAfter = &va.Float64
		}
		if resolved.Valid {
			t := time.Unix(resolved.Int64, 0).UTC().Format(time.RFC3339)
			x.ResolvedAt = &t
		}
		switch {
		case !resolved.Valid:
			x.Verdict = "open"
		case !va.Valid:
			x.Verdict = "pending"
		default:
			x.Verdict = "not_improved"
			if th, err := s.AlertThreshold(ctx, x.Rule); err == nil || x.Rule == RuleSourceFailed {
				if x.Rule == RuleSourceFailed {
					th = watchSourceThreshold
				}
				if va.Float64 < th {
					x.Verdict = watchEffectVerdictImproved
				}
			}
		}
		effects = append(effects, x)
	}
	er.Close()

	writeDataNoPeriod(w, map[string]any{
		"interval_seconds":   int(DefaultWatchInterval / time.Second),
		"webhook_configured": cfg.WebhookURL != "",
		"disk_configured":    cfg.DiskPath != "",
		"retention_days":     s.RetentionDays(ctx),
		"rules":              rules,
		"runs":               runs,
		"effects":            effects,
	})
}
