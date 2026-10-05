package admin

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
)

const (
	issueMaxBody     = 16 << 10
	issueMaxTitle    = 200
	issueMaxEvidence = 4000
	issueMaxFix      = 2000
	issueMaxSource   = 100
	issueMaxNote     = 2000
)

var (
	issueIDPattern = regexp.MustCompile(`^[a-z0-9-]{1,40}$`)
	issueStatuses  = map[string]bool{"new": true, "in_progress": true, "resolved": true}
	issueSeverity  = map[string]bool{"low": true, "medium": true, "high": true, "critical": true}
)

type issue struct {
	ID           string  `json:"id"`
	CreatedAt    string  `json:"created_at"`
	UpdatedAt    string  `json:"updated_at"`
	Severity     string  `json:"severity"`
	Status       string  `json:"status"`
	Title        string  `json:"title"`
	Evidence     *string `json:"evidence"`
	SuggestedFix *string `json:"suggested_fix"`
	Source       *string `json:"source"`
	Note         *string `json:"note"`
}

const issueColumns = `id, created_at, updated_at, severity, status, title, evidence, suggested_fix, source, note`

type issueRowScanner interface{ Scan(...any) error }

func scanIssue(r issueRowScanner) (issue, error) {
	var i issue
	var created, updated int64
	var ev, fix, src, note sql.NullString
	if err := r.Scan(&i.ID, &created, &updated, &i.Severity, &i.Status, &i.Title, &ev, &fix, &src, &note); err != nil {
		return issue{}, err
	}
	i.CreatedAt = time.Unix(created, 0).UTC().Format(time.RFC3339)
	i.UpdatedAt = time.Unix(updated, 0).UTC().Format(time.RFC3339)
	for _, p := range []struct {
		dst **string
		v   sql.NullString
	}{{&i.Evidence, ev}, {&i.SuggestedFix, fix}, {&i.Source, src}, {&i.Note, note}} {
		if p.v.Valid {
			v := p.v.String
			*p.dst = &v
		}
	}
	return i, nil
}

// issueDecode reads a strict JSON body (16 KiB max, application/json, no unknown field, no
// trailing data). It answers the error itself and returns false on failure.
func issueDecode(w http.ResponseWriter, r *http.Request, dst any) bool {
	mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mt != "application/json" {
		writeAPIError(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "Content-Type must be application/json")
		return false
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, issueMaxBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeAPIError(w, http.StatusRequestEntityTooLarge, "body_too_large", "body exceeds 16 KiB")
			return false
		}
		writeAPIError(w, http.StatusBadRequest, "bad_json", "invalid JSON body")
		return false
	}
	if _, err := dec.Token(); err != io.EOF {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeAPIError(w, http.StatusRequestEntityTooLarge, "body_too_large", "body exceeds 16 KiB")
			return false
		}
		writeAPIError(w, http.StatusBadRequest, "bad_json", "unexpected data after the JSON body")
		return false
	}
	return true
}

func issueFieldOK(v string, max int) bool {
	return utf8.ValidString(v) && utf8.RuneCountInString(v) <= max
}

func issueID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := chi.URLParam(r, "id")
	if !issueIDPattern.MatchString(id) {
		writeAPIError(w, http.StatusBadRequest, "bad_id", "id must match [a-z0-9-]{1,40}")
		return "", false
	}
	return id, true
}

// handleIssuesList: GET /issues?status=&severity=&limit=&offset= (diagnostics:read) — feeds the
// "Claude et MCP" page (findings list).
func (s *Service) handleIssuesList(w http.ResponseWriter, r *http.Request) {
	pg, err := parsePagination(r)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "bad_pagination", err.Error())
		return
	}
	q := r.URL.Query()
	status, severity := q.Get("status"), q.Get("severity")
	if status != "" && !issueStatuses[status] {
		writeAPIError(w, http.StatusBadRequest, "bad_status", "status must be new, in_progress or resolved")
		return
	}
	if severity != "" && !issueSeverity[severity] {
		writeAPIError(w, http.StatusBadRequest, "bad_severity", "severity must be low, medium, high or critical")
		return
	}
	where := `(? = '' OR status = ?) AND (? = '' OR severity = ?)`
	args := []any{status, status, severity, severity}
	var total int64
	if err := s.adminDB().QueryRowContext(r.Context(), `SELECT COUNT(*) FROM issues WHERE `+where, args...).Scan(&total); err != nil {
		pbServerError(w, err)
		return
	}
	rows, err := s.adminDB().QueryContext(r.Context(),
		`SELECT `+issueColumns+` FROM issues WHERE `+where+` ORDER BY created_at DESC, id LIMIT ? OFFSET ?`,
		append(args, pg.Limit, pg.Offset)...)
	if err != nil {
		pbServerError(w, err)
		return
	}
	defer rows.Close()
	items := []issue{}
	for rows.Next() {
		i, err := scanIssue(rows)
		if err != nil {
			pbServerError(w, err)
			return
		}
		items = append(items, i)
	}
	if err := rows.Err(); err != nil {
		pbServerError(w, err)
		return
	}
	writeDataNoPeriod(w, map[string]any{"items": items, "page": map[string]any{"limit": pg.Limit, "offset": pg.Offset, "total": total}})
}

// handleIssueGet: GET /issues/{id} (diagnostics:read) — feeds the "Claude et MCP" page (finding detail).
func (s *Service) handleIssueGet(w http.ResponseWriter, r *http.Request) {
	id, ok := issueID(w, r)
	if !ok {
		return
	}
	i, err := scanIssue(s.adminDB().QueryRowContext(r.Context(), `SELECT `+issueColumns+` FROM issues WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		writeAPIError(w, http.StatusNotFound, "not_found", "issue not found")
		return
	}
	if err != nil {
		pbServerError(w, err)
		return
	}
	writeDataNoPeriod(w, i)
}

// handleIssueCreate: POST /issues (ops:write) — feeds the "Claude et MCP" page (a finding raised
// by Claude or an admin). Answers 201 with the created issue.
func (s *Service) handleIssueCreate(w http.ResponseWriter, r *http.Request) {
	var in issueInput
	if !issueDecode(w, r, &in) {
		return
	}
	if code, msg := in.validate(); code != "" {
		writeAPIError(w, http.StatusBadRequest, code, msg)
		return
	}
	i, err := s.insertIssue(r.Context(), in)
	if err != nil {
		pbServerError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, envelope{GeneratedAt: time.Now().UTC().Format(time.RFC3339), Data: i})
}

// issueInput is the body of POST /issues and the arguments of the create_issue action.
type issueInput struct {
	Title        string `json:"title"`
	Severity     string `json:"severity"`
	Evidence     string `json:"evidence"`
	SuggestedFix string `json:"suggested_fix"`
	Source       string `json:"source"`
}

// validate trims the title and returns an API error code and message, or "" when valid.
func (in *issueInput) validate() (code, msg string) {
	in.Title = strings.TrimSpace(in.Title)
	switch {
	case in.Title == "" || !issueFieldOK(in.Title, issueMaxTitle):
		return "bad_title", "title is required (max 200 characters)"
	case !issueSeverity[in.Severity]:
		return "bad_severity", "severity must be low, medium, high or critical"
	case !issueFieldOK(in.Evidence, issueMaxEvidence):
		return "bad_evidence", "evidence is too long (max 4000 characters)"
	case !issueFieldOK(in.SuggestedFix, issueMaxFix):
		return "bad_suggested_fix", "suggested_fix is too long (max 2000 characters)"
	case !issueFieldOK(in.Source, issueMaxSource):
		return "bad_source", "source is too long (max 100 characters)"
	}
	return "", ""
}

// insertIssue stores a validated issue with status "new" and returns it.
func (s *Service) insertIssue(ctx context.Context, in issueInput) (issue, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return issue{}, err
	}
	id := "iss-" + hex.EncodeToString(b[:])
	now := s.now().Unix()
	if _, err := s.adminDB().ExecContext(ctx,
		`INSERT INTO issues (id, created_at, updated_at, severity, status, title, evidence, suggested_fix, source) VALUES (?,?,?,?,?,?,?,?,?)`,
		id, now, now, in.Severity, "new", in.Title, nullStr(in.Evidence), nullStr(in.SuggestedFix), nullStr(in.Source)); err != nil {
		return issue{}, err
	}
	return scanIssue(s.adminDB().QueryRowContext(ctx, `SELECT `+issueColumns+` FROM issues WHERE id = ?`, id))
}

// handleIssueUpdate: PATCH /issues/{id} (ops:write) — feeds the "Claude et MCP" page (triage:
// status new|in_progress|resolved and a note). Absent fields are left unchanged.
func (s *Service) handleIssueUpdate(w http.ResponseWriter, r *http.Request) {
	id, ok := issueID(w, r)
	if !ok {
		return
	}
	var in struct {
		Status *string `json:"status"`
		Note   *string `json:"note"`
	}
	if !issueDecode(w, r, &in) {
		return
	}
	if in.Status == nil && in.Note == nil {
		writeAPIError(w, http.StatusBadRequest, "empty_patch", "provide status and/or note")
		return
	}
	if in.Status != nil && !issueStatuses[*in.Status] {
		writeAPIError(w, http.StatusBadRequest, "bad_status", "status must be new, in_progress or resolved")
		return
	}
	if in.Note != nil && !issueFieldOK(*in.Note, issueMaxNote) {
		writeAPIError(w, http.StatusBadRequest, "bad_note", "note is too long (max 2000 characters)")
		return
	}
	var note any
	if in.Note != nil {
		note = nullStr(strings.TrimSpace(*in.Note))
	}
	res, err := s.adminDB().ExecContext(r.Context(),
		`UPDATE issues SET updated_at = ?, status = COALESCE(?, status), note = CASE WHEN ? THEN ? ELSE note END WHERE id = ?`,
		s.now().Unix(), in.Status, in.Note != nil, note, id)
	if err != nil {
		pbServerError(w, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeAPIError(w, http.StatusNotFound, "not_found", "issue not found")
		return
	}
	i, err := scanIssue(s.adminDB().QueryRowContext(r.Context(), `SELECT `+issueColumns+` FROM issues WHERE id = ?`, id))
	if err != nil {
		pbServerError(w, err)
		return
	}
	writeDataNoPeriod(w, i)
}
