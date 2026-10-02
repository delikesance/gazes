package diagnostics

import (
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"
	"regexp"
	"strings"
	"sync"
)

var credentials = regexp.MustCompile(`(?i)(apikey|api_key|token|password|secret|authorization|cookie|signature|credential|access_key)([\s"':=]+)([^\s&"'<>;,]+)`)
var bearer = regexp.MustCompile(`(?i)(bearer|basic)[ \t]+[a-zA-Z0-9+/=_~.-]+`)
var urls = regexp.MustCompile(`https?://[^\s"'<>]+`)
var magnets = regexp.MustCompile(`magnet:\?[^\s"'<>]+`)
var userinfo = regexp.MustCompile(`(https?://)[^\s/@]+@`)
var secretsMu sync.RWMutex
var secretValues []string

func RegisterSecret(value string) {
	if len(value) < 4 {
		return
	}
	secretsMu.Lock()
	defer secretsMu.Unlock()
	secretValues = append(secretValues, value)
}
func Redact(s string) string {
	secretsMu.RLock()
	for _, value := range secretValues {
		s = strings.ReplaceAll(s, value, "[REDACTED]")
	}
	secretsMu.RUnlock()
	s = magnets.ReplaceAllString(s, "[MAGNET REDACTED]")
	s = bearer.ReplaceAllString(s, "[AUTH REDACTED]")
	s = urls.ReplaceAllStringFunc(s, func(raw string) string {
		u, e := url.Parse(raw)
		if e != nil {
			return "[URL REDACTED]"
		}
		u.User = nil
		q := u.Query()
		for k := range q {
			if sensitive(k) {
				q.Set(k, "[REDACTED]")
			}
		}
		u.RawQuery = q.Encode()
		return u.String()
	})
	s = userinfo.ReplaceAllString(s, "${1}[REDACTED]@")
	return credentials.ReplaceAllString(s, "${1}${2}[REDACTED]")
}
func sensitive(key string) bool {
	key = strings.ToLower(key)
	for _, part := range []string{"apikey", "api_key", "authorization", "cookie", "password", "secret", "token", "magnet", "credential", "signature"} {
		if strings.Contains(key, part) {
			return true
		}
	}
	return false
}
func clean(key string, value any) any {
	if sensitive(key) {
		return "[REDACTED]"
	}
	switch v := value.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, x := range v {
			out[k] = clean(k, x)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, x := range v {
			out[i] = clean("", x)
		}
		return out
	case []string:
		out := make([]string, len(v))
		for i, x := range v {
			out[i] = Redact(x)
		}
		return out
	case string:
		s := Redact(v)
		if len(s) > 4096 {
			s = s[:4096] + " [TRUNCATED]"
		}
		return s
	case error:
		return clean(key, v.Error())
	case fmt.Stringer:
		return clean(key, v.String())
	default:
		if value == nil {
			return nil
		}
		kind := reflect.TypeOf(value).Kind()
		if kind >= reflect.Bool && kind <= reflect.Float64 {
			return value
		}
		data, err := json.Marshal(value)
		if err != nil {
			return clean(key, fmt.Sprint(value))
		}
		var converted any
		if json.Unmarshal(data, &converted) != nil {
			return "[UNSERIALIZABLE]"
		}
		return clean(key, converted)
	}
}
