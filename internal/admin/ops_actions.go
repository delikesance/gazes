package admin

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"strings"
)

// alertRule bounds one alert threshold (allow-list of set_alert_threshold).
type alertRule struct {
	Min, Max, Default float64
}

var alertRules = map[string]alertRule{
	"error_rate_pct":        {Min: 0.1, Max: 50, Default: 5},
	"startup_p95_s":         {Min: 0.5, Max: 60, Default: 5},
	"stream_saturation_pct": {Min: 10, Max: 100, Default: 85},
	"disk_pct":              {Min: 50, Max: 99, Default: 85},
}

// AlertThreshold returns the configured threshold of rule, or its default.
func (s *Service) AlertThreshold(ctx context.Context, rule string) (float64, error) {
	r, ok := alertRules[rule]
	if !ok {
		return 0, errors.New("unknown alert rule")
	}
	v, found, err := s.getSetting(ctx, settingThresholdPrefix+rule)
	if err != nil {
		return 0, err
	}
	if found {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f, nil
		}
	}
	return r.Default, nil
}

// ---- argument helpers -----------------------------------------------------------------------

func onlyKeys(args map[string]any, keys ...string) error {
	for k := range args {
		ok := false
		for _, a := range keys {
			if a == k {
				ok = true
			}
		}
		if !ok {
			return badArgs("unknown argument %q", k)
		}
	}
	return nil
}

func strArg(args map[string]any, name string, required bool, max int) (string, error) {
	v, present := args[name]
	if !present || v == nil {
		if required {
			return "", badArgs("missing required argument %q", name)
		}
		return "", nil
	}
	s, ok := v.(string)
	if !ok {
		return "", badArgs("argument %q must be a string", name)
	}
	s = strings.TrimSpace(s)
	if required && s == "" {
		return "", badArgs("argument %q must not be empty", name)
	}
	if !issueFieldOK(s, max) {
		return "", badArgs("argument %q is too long or not valid UTF-8 (max %d characters)", name, max)
	}
	return s, nil
}

func issueIDArg(args map[string]any, name string) (string, error) {
	id, err := strArg(args, name, true, 40)
	if err != nil {
		return "", err
	}
	if !issueIDPattern.MatchString(id) {
		return "", badArgs("argument %q must match [a-z0-9-]{1,40}", name)
	}
	return id, nil
}

// ---- registry -------------------------------------------------------------------------------

// actionList is the action registry.
func (s *Service) actionList() []Action {
	return []Action{
		s.actSetIssueStatus(),
		s.actCreateIssue(),
		s.actAddNote(),
		s.actSetAlertThreshold(),
		s.actRetrySource(),
		s.actWarmCache(),
		s.actRequeueAV1(),
		s.actPauseSource(),
		s.actPurgeCache(),
		s.actLimitStreams(),
		s.actScheduleMaintenance(),
	}
}

// issueUndo is the undo token of the issue actions.
type issueUndo struct {
	Op          string  `json:"op"` // "restore" | "delete"
	ID          string  `json:"id"`
	Status      string  `json:"status,omitempty"`
	RestoreNote bool    `json:"restore_note,omitempty"`
	Note        *string `json:"note"`
}

func (s *Service) loadIssue(ctx context.Context, id string) (issue, error) {
	i, err := scanIssue(s.adminDB().QueryRowContext(ctx, `SELECT `+issueColumns+` FROM issues WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return issue{}, notFound("issue")
	}
	return i, err
}

func (s *Service) undoIssue(ctx context.Context, token string) error {
	var u issueUndo
	if json.Unmarshal([]byte(token), &u) != nil || !issueIDPattern.MatchString(u.ID) {
		return badArgs("invalid undo token")
	}
	switch u.Op {
	case "delete":
		_, err := s.adminDB().ExecContext(ctx, `DELETE FROM issues WHERE id = ?`, u.ID)
		return err
	case "restore":
		var note any
		if u.Note != nil {
			note = *u.Note
		}
		status := any(nil)
		if u.Status != "" {
			status = u.Status
		}
		res, err := s.adminDB().ExecContext(ctx,
			`UPDATE issues SET updated_at = ?, status = COALESCE(?, status), note = CASE WHEN ? THEN ? ELSE note END WHERE id = ?`,
			s.now().Unix(), status, u.RestoreNote, note, u.ID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return notFound("issue")
		}
		return nil
	}
	return badArgs("invalid undo token")
}

func tokenOf(u issueUndo) string {
	b, _ := json.Marshal(u)
	return string(b)
}

func (s *Service) actSetIssueStatus() Action {
	parse := func(args map[string]any) (id, status string, note *string, err error) {
		if err = onlyKeys(args, "id", "status", "note"); err != nil {
			return
		}
		if id, err = issueIDArg(args, "id"); err != nil {
			return
		}
		if status, err = strArg(args, "status", true, 20); err != nil {
			return
		}
		if !issueStatuses[status] {
			err = badArgs("status must be new, in_progress or resolved")
			return
		}
		if _, ok := args["note"]; ok && args["note"] != nil {
			var n string
			if n, err = strArg(args, "note", false, issueMaxNote); err != nil {
				return
			}
			note = &n
		}
		return
	}
	return Action{
		Name: "set_issue_status", Level: ActionReversible, Scope: ScopeOpsWrite, Implemented: true,
		Summary: "Set the status of an issue (and optionally its triage note)",
		Validate: func(args map[string]any) error {
			_, _, _, err := parse(args)
			return err
		},
		Plan: func(ctx context.Context, args map[string]any) (Plan, error) {
			id, status, note, err := parse(args)
			if err != nil {
				return Plan{}, err
			}
			cur, err := s.loadIssue(ctx, id)
			if err != nil {
				return Plan{}, err
			}
			d := map[string]any{"id": id, "title": cur.Title, "status_from": cur.Status, "status_to": status}
			if note != nil {
				d["note_replaced"] = true
			}
			return Plan{Summary: "Change issue " + id + " from " + cur.Status + " to " + status, Details: d}, nil
		},
		Do: func(ctx context.Context, args map[string]any) (Result, string, error) {
			id, status, note, err := parse(args)
			if err != nil {
				return nil, "", err
			}
			cur, err := s.loadIssue(ctx, id)
			if err != nil {
				return nil, "", err
			}
			var noteVal any
			if note != nil {
				noteVal = nullStr(*note)
			}
			if _, err := s.adminDB().ExecContext(ctx,
				`UPDATE issues SET updated_at = ?, status = ?, note = CASE WHEN ? THEN ? ELSE note END WHERE id = ?`,
				s.now().Unix(), status, note != nil, noteVal, id); err != nil {
				return nil, "", err
			}
			u := issueUndo{Op: "restore", ID: id, Status: cur.Status, RestoreNote: note != nil, Note: cur.Note}
			upd, err := s.loadIssue(ctx, id)
			if err != nil {
				return nil, "", err
			}
			return Result{"issue": upd}, tokenOf(u), nil
		},
		Undo: s.undoIssue,
	}
}

func (s *Service) actCreateIssue() Action {
	parse := func(args map[string]any) (issueInput, error) {
		var in issueInput
		if err := onlyKeys(args, "title", "severity", "evidence", "suggested_fix", "source"); err != nil {
			return in, err
		}
		for _, f := range []struct {
			dst  *string
			name string
			max  int
			req  bool
		}{
			{&in.Title, "title", issueMaxTitle, true}, {&in.Severity, "severity", 20, true},
			{&in.Evidence, "evidence", issueMaxEvidence, false}, {&in.SuggestedFix, "suggested_fix", issueMaxFix, false},
			{&in.Source, "source", issueMaxSource, false},
		} {
			v, err := strArg(args, f.name, f.req, f.max)
			if err != nil {
				return in, err
			}
			*f.dst = v
		}
		if code, msg := in.validate(); code != "" {
			return in, badArgs("%s: %s", code, msg)
		}
		return in, nil
	}
	return Action{
		Name: "create_issue", Level: ActionReversible, Scope: ScopeOpsWrite, Implemented: true,
		Summary: "Raise a new issue (finding)",
		Validate: func(args map[string]any) error {
			_, err := parse(args)
			return err
		},
		Plan: func(_ context.Context, args map[string]any) (Plan, error) {
			in, err := parse(args)
			if err != nil {
				return Plan{}, err
			}
			return Plan{Summary: "Create a " + in.Severity + " issue: " + in.Title, Details: map[string]any{"title": in.Title, "severity": in.Severity, "status": "new"}}, nil
		},
		Do: func(ctx context.Context, args map[string]any) (Result, string, error) {
			in, err := parse(args)
			if err != nil {
				return nil, "", err
			}
			i, err := s.insertIssue(ctx, in)
			if err != nil {
				return nil, "", err
			}
			return Result{"issue": i}, tokenOf(issueUndo{Op: "delete", ID: i.ID}), nil
		},
		Undo: s.undoIssue,
	}
}

func (s *Service) actAddNote() Action {
	parse := func(args map[string]any) (id, text string, err error) {
		if err = onlyKeys(args, "issue_id", "text"); err != nil {
			return
		}
		if id, err = issueIDArg(args, "issue_id"); err != nil {
			return
		}
		text, err = strArg(args, "text", true, issueMaxNote)
		return
	}
	// appended joins the existing note and the new text, bounded like any note.
	appended := func(cur *string, text string) (string, error) {
		out := text
		if cur != nil && *cur != "" {
			out = *cur + "\n" + text
		}
		if !issueFieldOK(out, issueMaxNote) {
			return "", badArgs("the note would exceed %d characters", issueMaxNote)
		}
		return out, nil
	}
	return Action{
		Name: "add_note", Level: ActionReversible, Scope: ScopeOpsWrite, Implemented: true,
		Summary: "Append a line to the triage note of an issue",
		Validate: func(args map[string]any) error {
			_, _, err := parse(args)
			return err
		},
		Plan: func(ctx context.Context, args map[string]any) (Plan, error) {
			id, text, err := parse(args)
			if err != nil {
				return Plan{}, err
			}
			cur, err := s.loadIssue(ctx, id)
			if err != nil {
				return Plan{}, err
			}
			if _, err := appended(cur.Note, text); err != nil {
				return Plan{}, err
			}
			return Plan{Summary: "Append a note to issue " + id, Details: map[string]any{"id": id, "title": cur.Title, "added_chars": len([]rune(text))}}, nil
		},
		Do: func(ctx context.Context, args map[string]any) (Result, string, error) {
			id, text, err := parse(args)
			if err != nil {
				return nil, "", err
			}
			cur, err := s.loadIssue(ctx, id)
			if err != nil {
				return nil, "", err
			}
			n, err := appended(cur.Note, text)
			if err != nil {
				return nil, "", err
			}
			if _, err := s.adminDB().ExecContext(ctx, `UPDATE issues SET updated_at = ?, note = ? WHERE id = ?`, s.now().Unix(), n, id); err != nil {
				return nil, "", err
			}
			upd, err := s.loadIssue(ctx, id)
			if err != nil {
				return nil, "", err
			}
			return Result{"issue": upd}, tokenOf(issueUndo{Op: "restore", ID: id, RestoreNote: true, Note: cur.Note}), nil
		},
		Undo: s.undoIssue,
	}
}

func (s *Service) actSetAlertThreshold() Action {
	parse := func(args map[string]any) (string, float64, error) {
		if err := onlyKeys(args, "rule", "value"); err != nil {
			return "", 0, err
		}
		rule, err := strArg(args, "rule", true, 40)
		if err != nil {
			return "", 0, err
		}
		r, ok := alertRules[rule]
		if !ok {
			return "", 0, badArgs("rule must be one of error_rate_pct, startup_p95_s, stream_saturation_pct, disk_pct")
		}
		v, ok := args["value"].(float64)
		if !ok || math.IsNaN(v) || math.IsInf(v, 0) {
			return "", 0, badArgs("argument \"value\" must be a number")
		}
		if v < r.Min || v > r.Max {
			return "", 0, badArgs("value for %s must be between %g and %g", rule, r.Min, r.Max)
		}
		return rule, v, nil
	}
	type thrUndo struct {
		Rule string  `json:"rule"`
		Prev *string `json:"prev"`
	}
	return Action{
		Name: "set_alert_threshold", Level: ActionSensitive, Scope: ScopeConfigWrite, Implemented: true,
		Summary: "Change an alert threshold (error_rate_pct, startup_p95_s, stream_saturation_pct, disk_pct)",
		Validate: func(args map[string]any) error {
			_, _, err := parse(args)
			return err
		},
		Plan: func(ctx context.Context, args map[string]any) (Plan, error) {
			rule, v, err := parse(args)
			if err != nil {
				return Plan{}, err
			}
			cur, err := s.AlertThreshold(ctx, rule)
			if err != nil {
				return Plan{}, err
			}
			r := alertRules[rule]
			return Plan{Summary: "Change threshold " + rule + " from " + strconv.FormatFloat(cur, 'g', -1, 64) + " to " + strconv.FormatFloat(v, 'g', -1, 64),
				Details: map[string]any{"rule": rule, "from": cur, "to": v, "min": r.Min, "max": r.Max}}, nil
		},
		Do: func(ctx context.Context, args map[string]any) (Result, string, error) {
			rule, v, err := parse(args)
			if err != nil {
				return nil, "", err
			}
			prev, found, err := s.getSetting(ctx, settingThresholdPrefix+rule)
			if err != nil {
				return nil, "", err
			}
			if err := s.setSetting(ctx, settingThresholdPrefix+rule, strconv.FormatFloat(v, 'g', -1, 64), actorFrom(ctx)); err != nil {
				return nil, "", err
			}
			u := thrUndo{Rule: rule}
			if found {
				u.Prev = &prev
			}
			b, _ := json.Marshal(u)
			return Result{"rule": rule, "value": v}, string(b), nil
		},
		Undo: func(ctx context.Context, token string) error {
			var u thrUndo
			if json.Unmarshal([]byte(token), &u) != nil {
				return badArgs("invalid undo token")
			}
			if _, ok := alertRules[u.Rule]; !ok {
				return badArgs("invalid undo token")
			}
			if u.Prev == nil {
				return s.deleteSetting(ctx, settingThresholdPrefix+u.Rule)
			}
			return s.setSetting(ctx, settingThresholdPrefix+u.Rule, *u.Prev, actorFrom(ctx))
		},
	}
}
