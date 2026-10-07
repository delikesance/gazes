package admin

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"
)

// Watch rules (M5). Every minute the server measures a few health figures, compares them with the
// alert thresholds (settings), and on a breach opens ONE issue (constat) and POSTs a webhook so
// Claude (or a human) is woken up. This is server-side and independent of the kill switch: it only
// writes the admin database and sends the webhook, never touches the streaming path.
const (
	RuleErrorRate    = "error_rate_pct"
	RuleStartup      = "startup_p95_s"
	RuleSaturation   = "stream_saturation_pct"
	RuleDisk         = "disk_pct"
	RuleSourceFailed = "source_failures"

	DefaultWatchInterval = time.Minute

	watchErrorWindow     = time.Hour
	watchMinSessions     = 20 // below this many sessions in the window the error rate is not meaningful
	watchStartupWindow   = time.Hour
	watchMinStartups     = 20 // below this many startups in the window the p95 is not meaningful
	watchSourceWindow    = 30 * time.Minute
	watchSourceThreshold = 5.0 // SRC_DEAD failures of one source in the window
	watchNearRatio       = 0.8 // "near" starts at 80 % of the threshold
	watchRecoverStreak   = 3   // consecutive healthy evaluations before a breach is closed
	watchEffectDelay     = 24 * time.Hour
	watchKeepRuns        = 200
)

// Watch states of a rule.
const (
	WatchOK          = "ok"
	WatchNear        = "near"
	WatchBreached    = "breached"
	WatchNotMeasured = "not_measured"
)

// WatchConfig is the optional environment of the watch loop.
type WatchConfig struct {
	WebhookURL    string // http(s) endpoint receiving the events; empty disables notifications
	WebhookSecret string // HMAC-SHA256 key of the X-Gazes-Signature header; empty = unsigned
	DiskPath      string // directory whose filesystem the disk rule measures; empty = not measured
}

// SetWatchConfig sets the watch environment (call before RunWatch).
func (s *Service) SetWatchConfig(c WatchConfig) {
	if c.WebhookURL != "" {
		if u, err := url.Parse(c.WebhookURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			log.Printf("admin watch: ignoring GAZES_WATCH_WEBHOOK_URL (not an http(s) URL)")
			c.WebhookURL = ""
		}
	}
	s.mu.Lock()
	s.watchCfg = c
	s.mu.Unlock()
}

func (s *Service) watchConfig() WatchConfig {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.watchCfg
}

// RuleState is one rule's current state as shown on the panel and read by Claude.
type RuleState struct {
	Rule      string   `json:"rule"`
	Label     string   `json:"label"`
	State     string   `json:"state"`
	Value     *float64 `json:"value"`
	Threshold float64  `json:"threshold"`
	Unit      string   `json:"unit"`
	Detail    string   `json:"detail"`
	Since     *string  `json:"since"`
	IssueID   *string  `json:"issue_id"`
}

type measure struct {
	value  *float64
	detail string
}

func fptr(v float64) *float64 { return &v }

func (s *Service) measureErrorRate(ctx context.Context, now time.Time) measure {
	since := now.Add(-watchErrorWindow).Unix()
	var sessions, errs int64
	if err := s.accountsDB().QueryRowContext(ctx, `SELECT COUNT(*) FROM watch_sessions WHERE started_at >= ?`, since).Scan(&sessions); err != nil {
		return measure{detail: "séances illisibles"}
	}
	if err := s.adminDB().QueryRowContext(ctx, `SELECT COUNT(*) FROM playback_errors WHERE ts >= ?`, since).Scan(&errs); err != nil {
		return measure{detail: "erreurs illisibles"}
	}
	if sessions < watchMinSessions {
		return measure{detail: fmt.Sprintf("échantillon trop faible (%d séances sur 60 min)", sessions)}
	}
	return measure{value: fptr(float64(errs) / float64(sessions) * 100), detail: fmt.Sprintf("%d erreurs pour %d séances sur 60 min", errs, sessions)}
}

func (s *Service) measureStartup(ctx context.Context, now time.Time) measure {
	p50, p95, n, err := s.pbStartupPercentiles(ctx, now.Add(-watchStartupWindow).Unix(), now.Unix()+1)
	if err != nil {
		return measure{detail: "démarrages illisibles"}
	}
	if n < watchMinStartups {
		return measure{detail: fmt.Sprintf("échantillon trop faible (%d démarrages sur 60 min)", n)}
	}
	return measure{value: fptr(p95 / 1000), detail: fmt.Sprintf("p50 %.1f s, p95 %.1f s sur %d démarrages (60 min)", p50/1000, p95/1000, n)}
}

func (s *Service) measureDisk(path string) measure {
	if path == "" {
		return measure{detail: "chemin non configuré (GAZES_WATCH_DISK_PATH)"}
	}
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil || st.Blocks == 0 {
		return measure{detail: "volume illisible"}
	}
	used := float64(st.Blocks-st.Bavail) / float64(st.Blocks) * 100
	return measure{value: fptr(used), detail: fmt.Sprintf("%.1f %% du volume utilisé", used)}
}

func (s *Service) measureSources(ctx context.Context, now time.Time) measure {
	rows, err := s.adminDB().QueryContext(ctx,
		`SELECT COALESCE(NULLIF(source,''),'(inconnue)'), COUNT(*) FROM playback_errors WHERE code = 'SRC_DEAD' AND ts >= ? GROUP BY 1 ORDER BY 2 DESC LIMIT 3`,
		now.Add(-watchSourceWindow).Unix())
	if err != nil {
		return measure{detail: "erreurs illisibles"}
	}
	defer rows.Close()
	var top float64
	var parts []string
	for rows.Next() {
		var src string
		var n int64
		if rows.Scan(&src, &n) != nil {
			continue
		}
		if float64(n) > top {
			top = float64(n)
		}
		parts = append(parts, fmt.Sprintf("%s : %d", src, n))
	}
	if len(parts) == 0 {
		return measure{value: fptr(0), detail: "aucune source en échec sur 30 min"}
	}
	return measure{value: fptr(top), detail: "SRC_DEAD sur 30 min : " + strings.Join(parts, ", ")}
}

// measureSaturation is the share of the concurrent stream limit in use (limit_concurrent_streams).
func (s *Service) measureSaturation(ctx context.Context) measure {
	st := s.pbDeps().stats
	if st == nil {
		return measure{detail: "flux non comptés (moteur de lecture HLS désactivé)"}
	}
	limit := s.StreamLimit(ctx)
	if limit == 0 {
		return measure{detail: "aucune limite de flux définie"}
	}
	n := st.ActiveSessions()
	return measure{value: fptr(float64(n) / float64(limit) * 100), detail: fmt.Sprintf("%d flux ouverts pour une limite de %d", n, limit)}
}

type ruleDef struct {
	rule, label, unit string
	threshold         func(ctx context.Context, s *Service) float64
	measure           func(ctx context.Context, s *Service, now time.Time, cfg WatchConfig) measure
}

func thresholdOf(rule string) func(context.Context, *Service) float64 {
	return func(ctx context.Context, s *Service) float64 {
		v, err := s.AlertThreshold(ctx, rule)
		if err != nil {
			return alertRules[rule].Default
		}
		return v
	}
}

func watchRules() []ruleDef {
	return []ruleDef{
		{RuleErrorRate, "Taux d'erreur de lecture", "%", thresholdOf(RuleErrorRate),
			func(ctx context.Context, s *Service, now time.Time, _ WatchConfig) measure {
				return s.measureErrorRate(ctx, now)
			}},
		{RuleStartup, "Démarrage p95", "s", thresholdOf(RuleStartup),
			func(ctx context.Context, s *Service, now time.Time, _ WatchConfig) measure {
				return s.measureStartup(ctx, now)
			}},
		{RuleSaturation, "Saturation des flux", "%", thresholdOf(RuleSaturation),
			func(ctx context.Context, s *Service, _ time.Time, _ WatchConfig) measure {
				return s.measureSaturation(ctx)
			}},
		{RuleDisk, "Occupation du disque", "%", thresholdOf(RuleDisk),
			func(_ context.Context, s *Service, _ time.Time, cfg WatchConfig) measure {
				return s.measureDisk(cfg.DiskPath)
			}},
		{RuleSourceFailed, "Sources en échec (SRC_DEAD)", "échecs", func(context.Context, *Service) float64 { return watchSourceThreshold },
			func(ctx context.Context, s *Service, now time.Time, _ WatchConfig) measure {
				return s.measureSources(ctx, now)
			}},
	}
}

func classify(m measure, threshold float64) string {
	switch {
	case m.value == nil:
		return WatchNotMeasured
	case *m.value >= threshold:
		return WatchBreached
	case *m.value >= threshold*watchNearRatio:
		return WatchNear
	}
	return WatchOK
}

type storedState struct {
	state   string
	since   int64
	issueID sql.NullString
	streak  int
}

func (s *Service) loadState(ctx context.Context, rule string) (storedState, bool) {
	var st storedState
	err := s.adminDB().QueryRowContext(ctx, `SELECT state, since, issue_id, ok_streak FROM watch_state WHERE rule = ?`, rule).Scan(&st.state, &st.since, &st.issueID, &st.streak)
	return st, err == nil
}

// EvaluateWatch runs one evaluation of every rule: updates the stored states, opens an issue and
// notifies on a new breach, closes a breach after watchRecoverStreak healthy passes, and settles the
// before/after of the fixes. It returns the states (also served by GET /watch).
func (s *Service) EvaluateWatch(ctx context.Context) ([]RuleState, error) {
	start := time.Now()
	now := s.now()
	cfg := s.watchConfig()
	if s.inMaintenance(ctx, now) {
		cfg.WebhookURL = "" // planned work: issues are still opened, nobody is woken up
	}
	var out []RuleState
	var breached, sent, failed int
	for _, d := range watchRules() {
		th := d.threshold(ctx, s)
		m := d.measure(ctx, s, now, cfg)
		raw := classify(m, th)
		prev, had := s.loadState(ctx, d.rule)
		state, since, issueID, streak := raw, now.Unix(), prev.issueID, 0
		if had && prev.state == raw {
			since = prev.since
		}
		switch {
		case raw == WatchBreached && (!had || prev.state != WatchBreached):
			id, err := s.openWatchIssue(ctx, d, m, th, prev.issueID)
			if err != nil {
				log.Printf("admin watch: opening issue for %s: %v", d.rule, err)
			} else {
				issueID = sql.NullString{String: id, Valid: true}
				if s.notify(ctx, cfg, "watch.breached", d, m, th, id, now) {
					sent++
				} else if cfg.WebhookURL != "" {
					failed++
				}
			}
		case had && prev.state == WatchBreached && raw != WatchBreached:
			streak = prev.streak + 1
			if streak < watchRecoverStreak {
				state, since = WatchBreached, prev.since // not yet recovered: stay breached
			} else {
				streak = 0
				if issueID.Valid {
					s.noteWatchIssue(ctx, issueID.String, fmt.Sprintf("Retour à la normale (%s).", m.detail))
					if s.notify(ctx, cfg, "watch.recovered", d, m, th, issueID.String, now) {
						sent++
					} else if cfg.WebhookURL != "" {
						failed++
					}
				}
			}
		}
		if state == WatchBreached {
			breached++
		}
		var val any
		if m.value != nil {
			val = *m.value
		}
		if _, err := s.adminDB().ExecContext(ctx,
			`INSERT INTO watch_state(rule, state, value, since, issue_id, last_eval, ok_streak, detail) VALUES (?,?,?,?,?,?,?,?)
			 ON CONFLICT(rule) DO UPDATE SET state=excluded.state, value=excluded.value, since=excluded.since, issue_id=excluded.issue_id, last_eval=excluded.last_eval, ok_streak=excluded.ok_streak, detail=excluded.detail`,
			d.rule, state, val, since, issueID, now.Unix(), streak, m.detail); err != nil {
			return nil, err
		}
		rs := RuleState{Rule: d.rule, Label: d.label, State: state, Value: m.value, Threshold: th, Unit: d.unit, Detail: m.detail}
		t := time.Unix(since, 0).UTC().Format(time.RFC3339)
		rs.Since = &t
		if issueID.Valid {
			id := issueID.String
			rs.IssueID = &id
		}
		out = append(out, rs)
	}
	s.settleEffects(ctx, now, out)
	if _, err := s.adminDB().ExecContext(ctx, `INSERT INTO watch_runs(ts, breached, duration_ms, webhook_sent, webhook_failed) VALUES (?,?,?,?,?)`,
		now.Unix(), breached, time.Since(start).Milliseconds(), sent, failed); err != nil {
		return out, err
	}
	_, _ = s.adminDB().ExecContext(ctx, `DELETE FROM watch_runs WHERE id <= (SELECT MAX(id) FROM watch_runs) - ?`, watchKeepRuns)
	return out, nil
}

var watchRunbooks = map[string]string{
	RuleErrorRate:    "docs/runbooks/error-rate.md",
	RuleDisk:         "docs/runbooks/disk.md",
	RuleSourceFailed: "docs/runbooks/SRC_DEAD.md",
	RuleStartup:      "docs/runbooks/STREAM_TIMEOUT.md",
	RuleSaturation:   "docs/runbooks/saturation.md",
}

// openWatchIssue opens the issue of a new breach, or reuses the previous one while it is not resolved.
func (s *Service) openWatchIssue(ctx context.Context, d ruleDef, m measure, th float64, prev sql.NullString) (string, error) {
	if prev.Valid {
		var status string
		if err := s.adminDB().QueryRowContext(ctx, `SELECT status FROM issues WHERE id = ?`, prev.String).Scan(&status); err == nil && status != "resolved" {
			s.noteWatchIssue(ctx, prev.String, fmt.Sprintf("Nouveau dépassement (%s).", m.detail))
			return prev.String, nil
		}
	}
	sev := "high"
	if d.rule == RuleSourceFailed {
		sev = "medium"
	}
	evidence := fmt.Sprintf("%s : %s (seuil %.1f %s).", d.label, m.detail, th, d.unit)
	fix := "Suivre le runbook " + watchRunbooks[d.rule] + " ; mesurer de nouveau après correction."
	in := issueInput{Title: d.label + " au-dessus du seuil", Severity: sev, Evidence: evidence, SuggestedFix: fix, Source: "watch:" + d.rule}
	iss, err := s.insertIssue(ctx, in)
	if err != nil {
		return "", err
	}
	var vo any
	if m.value != nil {
		vo = *m.value
	}
	_, _ = s.adminDB().ExecContext(ctx, `INSERT OR REPLACE INTO watch_effects(issue_id, rule, value_open, opened_at) VALUES (?,?,?,?)`, iss.ID, d.rule, vo, s.now().Unix())
	return iss.ID, nil
}

func (s *Service) noteWatchIssue(ctx context.Context, id, text string) {
	line := s.now().UTC().Format("2006-01-02 15:04") + " UTC · " + text
	_, _ = s.adminDB().ExecContext(ctx, `UPDATE issues SET note = CASE WHEN note IS NULL OR note = '' THEN ? ELSE note || char(10) || ? END, updated_at = ? WHERE id = ?`,
		line, line, s.now().Unix(), id)
}

// settleEffects records the value of a rule when its issue gets resolved, and 24 h later again,
// so that "fixed" can be judged on measures rather than on a status.
func (s *Service) settleEffects(ctx context.Context, now time.Time, states []RuleState) {
	cur := map[string]*float64{}
	for _, st := range states {
		cur[st.Rule] = st.Value
	}
	rows, err := s.adminDB().QueryContext(ctx,
		`SELECT e.issue_id, e.rule, e.resolved_at, i.status FROM watch_effects e JOIN issues i ON i.id = e.issue_id WHERE e.value_after IS NULL`)
	if err != nil {
		return
	}
	type pend struct {
		id, rule, status string
		resolved         sql.NullInt64
	}
	var ps []pend
	for rows.Next() {
		var p pend
		if rows.Scan(&p.id, &p.rule, &p.resolved, &p.status) == nil {
			ps = append(ps, p)
		}
	}
	rows.Close()
	for _, p := range ps {
		v := cur[p.rule]
		switch {
		case !p.resolved.Valid && p.status == "resolved":
			var val any
			if v != nil {
				val = *v
			}
			_, _ = s.adminDB().ExecContext(ctx, `UPDATE watch_effects SET resolved_at = ?, value_resolved = ? WHERE issue_id = ?`, now.Unix(), val, p.id)
		case p.resolved.Valid && v != nil && now.Unix() >= p.resolved.Int64+int64(watchEffectDelay/time.Second):
			_, _ = s.adminDB().ExecContext(ctx, `UPDATE watch_effects SET value_after = ?, after_at = ? WHERE issue_id = ?`, *v, now.Unix(), p.id)
		}
	}
}

type watchEvent struct {
	Event     string   `json:"event"`
	Rule      string   `json:"rule"`
	Label     string   `json:"label"`
	Value     *float64 `json:"value"`
	Threshold float64  `json:"threshold"`
	Unit      string   `json:"unit"`
	Detail    string   `json:"detail"`
	IssueID   string   `json:"issue_id"`
	At        string   `json:"at"`
	Runbook   string   `json:"runbook"`
}

var webhookClient = &http.Client{
	Timeout:       5 * time.Second,
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

// notify POSTs one event to the webhook (best effort, one attempt, no redirect). The body holds rule
// figures and an issue id only: no user data.
func (s *Service) notify(ctx context.Context, cfg WatchConfig, event string, d ruleDef, m measure, th float64, issueID string, now time.Time) bool {
	if cfg.WebhookURL == "" {
		return false
	}
	body, err := json.Marshal(watchEvent{Event: event, Rule: d.rule, Label: d.label, Value: m.value, Threshold: th, Unit: d.unit,
		Detail: m.detail, IssueID: issueID, At: now.UTC().Format(time.RFC3339), Runbook: watchRunbooks[d.rule]})
	if err != nil {
		return false
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.WebhookURL, bytes.NewReader(body))
	if err != nil {
		return false
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Gazes-Event", event)
	if cfg.WebhookSecret != "" {
		mac := hmac.New(sha256.New, []byte(cfg.WebhookSecret))
		mac.Write(body)
		req.Header.Set("X-Gazes-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}
	resp, err := webhookClient.Do(req)
	if err != nil {
		log.Printf("admin watch: webhook %s failed: %v", event, errors.Unwrap(err))
		return false
	}
	resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

// RunWatch evaluates the rules every interval until ctx ends, like Rollup.RunElected: elect (when
// non-nil) lets one replica per period do the work. It also prunes old rows once an hour.
func (s *Service) RunWatch(ctx context.Context, interval time.Duration, elect func(context.Context) bool) {
	if interval <= 0 {
		interval = DefaultWatchInterval
	}
	var lastPrune time.Time
	pass := func() {
		if elect != nil && !elect(ctx) {
			return
		}
		if !s.watchMu.TryLock() {
			return
		}
		defer s.watchMu.Unlock()
		defer func() {
			if p := recover(); p != nil {
				log.Printf("admin watch: panic: %v", p)
			}
		}()
		if _, err := s.EvaluateWatch(ctx); err != nil && ctx.Err() == nil {
			log.Printf("admin watch: %v", err)
		}
		if now := s.now(); now.Sub(lastPrune) >= time.Hour {
			lastPrune = now
			if err := s.Prune(ctx); err != nil && ctx.Err() == nil {
				log.Printf("admin prune: %v", err)
			}
		}
	}
	pass()
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			pass()
		}
	}
}
