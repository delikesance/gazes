package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	// MaxResultBytes bounds a tool result (the JSON text sent to the model).
	MaxResultBytes = 64 << 10
	// MaxArgsSummary bounds the audited argument summary, in characters.
	MaxArgsSummary = 200

	adminPrefix = "/api/v1/admin"
)

// toolError is an error that is safe to show to the model.
type toolError struct {
	msg    string
	status int // HTTP status of the admin API, 0 when the call never reached it
	denied bool
}

func (e *toolError) Error() string { return e.msg }

// buildRequest turns tool arguments into the synthetic admin request.
func buildRequest(ctx context.Context, d ToolDef, args map[string]any, bearer string) (*http.Request, error) {
	known := map[string]Param{}
	for _, p := range d.Params {
		known[p.Name] = p
	}
	for k := range args {
		if _, ok := known[k]; !ok {
			return nil, &toolError{msg: fmt.Sprintf("unknown argument %q", k)}
		}
	}
	path := d.Path
	q := url.Values{}
	body := map[string]any{}
	bodyArgs := map[string]any{}
	for _, p := range d.Params {
		v, present := args[p.Name]
		if !present || v == nil {
			if p.Required {
				return nil, &toolError{msg: fmt.Sprintf("missing required argument %q", p.Name)}
			}
			continue
		}
		s, err := argString(p, v)
		if err == nil && (p.In == "body" || p.In == "arg") {
			if len(p.Enum) > 0 && !contains(p.Enum, s) {
				return nil, &toolError{msg: fmt.Sprintf("argument %q must be one of %s", p.Name, strings.Join(p.Enum, ", "))}
			}
			if p.In == "body" {
				body[p.Name] = v
			} else {
				bodyArgs[p.Name] = v
			}
			continue
		}
		if err != nil {
			return nil, &toolError{msg: fmt.Sprintf("argument %q: %v", p.Name, err)}
		}
		if len(p.Enum) > 0 && !contains(p.Enum, s) {
			return nil, &toolError{msg: fmt.Sprintf("argument %q must be one of %s", p.Name, strings.Join(p.Enum, ", "))}
		}
		if p.In == "path" {
			path = strings.ReplaceAll(path, "{"+p.Name+"}", url.PathEscape(s))
		} else {
			q.Set(p.Name, s)
		}
	}
	target := adminPrefix + path
	if len(q) > 0 {
		target += "?" + q.Encode()
	}
	var payload io.Reader
	hasBody := d.Method != http.MethodGet && d.Method != http.MethodHead
	if hasBody {
		if len(bodyArgs) > 0 {
			body["args"] = bodyArgs
		}
		b, err := json.Marshal(body)
		if err != nil {
			return nil, &toolError{msg: "invalid arguments"}
		}
		payload = bytes.NewReader(b)
	}
	req := httptest.NewRequestWithContext(ctx, d.Method, target, payload)
	if hasBody {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("Accept", "application/json")
	req.RemoteAddr = "mcp-in-process"
	return req, nil
}

func argString(p Param, v any) (string, error) {
	switch p.typ() {
	case "integer":
		f, ok := v.(float64)
		if !ok || f != float64(int64(f)) {
			return "", errors.New("must be an integer")
		}
		return strconv.FormatInt(int64(f), 10), nil
	case "number":
		f, ok := v.(float64)
		if !ok {
			return "", errors.New("must be a number")
		}
		return strconv.FormatFloat(f, 'g', -1, 64), nil
	case "boolean":
		b, ok := v.(bool)
		if !ok {
			return "", errors.New("must be a boolean")
		}
		return strconv.FormatBool(b), nil
	}
	s, ok := v.(string)
	if !ok {
		return "", errors.New("must be a string")
	}
	return s, nil
}

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

// serveAdmin runs req through the admin router and returns the status and body. A panic in the
// router is reported as an opaque error, never as a stack trace.
func serveAdmin(h http.Handler, req *http.Request) (status int, body []byte, err error) {
	defer func() {
		if recover() != nil {
			status, body, err = 0, nil, &toolError{msg: "internal error"}
		}
	}()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code, rec.Body.Bytes(), nil
}

// shapeResponse converts the admin response into the tool result text: the envelope's data (bounded
// to MaxResultBytes) on success, a readable *toolError otherwise.
func shapeResponse(status int, body []byte) (string, error) {
	if status >= 200 && status < 300 {
		var env struct {
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(body, &env); err != nil || len(env.Data) == 0 {
			return "", &toolError{msg: "internal error: unexpected response", status: status}
		}
		return boundJSON(env.Data), nil
	}
	code, msg := errorEnvelope(body)
	if code == "" {
		code = strings.ReplaceAll(strings.ToLower(http.StatusText(status)), " ", "_")
	}
	text := fmt.Sprintf("error %d %s", status, code)
	if msg != "" {
		text += ": " + msg
	}
	return "", &toolError{msg: text, status: status, denied: status == http.StatusUnauthorized || status == http.StatusForbidden}
}

// errorEnvelope reads both {"error":{"code","message"}} and the legacy {"error":"message"}.
func errorEnvelope(body []byte) (code, msg string) {
	var e struct {
		Error json.RawMessage `json:"error"`
	}
	if json.Unmarshal(body, &e) != nil || len(e.Error) == 0 {
		return "", ""
	}
	var s string
	if json.Unmarshal(e.Error, &s) == nil {
		return "", s
	}
	var o struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if json.Unmarshal(e.Error, &o) == nil {
		return o.Code, o.Message
	}
	return "", ""
}

// boundJSON compacts data and, when it exceeds MaxResultBytes, replaces it with a valid JSON
// object {"truncated":true,"total_bytes":N,"preview":"<start of the data>"}.
func boundJSON(data json.RawMessage) string {
	s := string(data)
	var buf bytes.Buffer
	if json.Compact(&buf, data) == nil {
		s = buf.String()
	}
	if len(s) <= MaxResultBytes {
		return s
	}
	cut := MaxResultBytes - 256
	for {
		for cut > 0 && !utf8.RuneStart(s[cut]) {
			cut--
		}
		out, _ := json.Marshal(map[string]any{"truncated": true, "total_bytes": len(s), "preview": s[:cut]})
		if len(out) <= MaxResultBytes || cut <= 0 {
			return string(out)
		}
		cut -= (len(out) - MaxResultBytes) + 16
		if cut < 0 {
			cut = 0
		}
	}
}

var sensitiveKey = regexp.MustCompile(`(?i)token|secret|pass|auth|key|cookie|email|pseudo|credential`)

// argsSummary is the audited form of args: sensitive values removed, keys sorted, at most
// MaxArgsSummary characters.
func argsSummary(d ToolDef, args map[string]any) string {
	if len(args) == 0 {
		return ""
	}
	sens := map[string]bool{}
	for _, p := range d.Params {
		if p.Sensitive {
			sens[p.Name] = true
		}
	}
	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	clean := make([]string, 0, len(keys))
	for _, k := range keys {
		if sens[k] || sensitiveKey.MatchString(k) {
			clean = append(clean, k+"=[redacted]")
			continue
		}
		v, _ := json.Marshal(args[k])
		clean = append(clean, k+"="+string(v))
	}
	s := strings.Join(clean, " ")
	if utf8.RuneCountInString(s) > MaxArgsSummary {
		r := []rune(s)
		s = string(r[:MaxArgsSummary])
	}
	return s
}
