package auth

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func registerWithCodes(t *testing.T, s *Service, email, pseudo string) (*http.Cookie, []string) {
	t.Helper()
	rr := s.post(t, s.Register, "register", map[string]string{"email": email, "password": "longenough", "pseudo": pseudo})
	var out struct {
		Codes []string `json:"recovery_codes"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil || sessionCookie(rr) == nil {
		t.Fatalf("register: %d %s", rr.Code, rr.Body)
	}
	return sessionCookie(rr), out.Codes
}

func TestRegisterReturnsEightDistinctRecoveryCodesOnce(t *testing.T) {
	s := newTestService(t)
	_, codes := registerWithCodes(t, s, "rc@example.com", "recov")
	if len(codes) != recoveryCodeCount {
		t.Fatalf("want %d codes, got %d", recoveryCodeCount, len(codes))
	}
	seen := map[string]bool{}
	for _, c := range codes {
		if len(c) != 14 || seen[c] {
			t.Fatalf("bad or duplicate code %q", c)
		}
		seen[c] = true
	}
	var raw int
	if err := s.store.db.QueryRow(`SELECT COUNT(*) FROM recovery_codes WHERE code_hash = ?`, codes[0]).Scan(&raw); err != nil || raw != 0 {
		t.Fatal("codes must be stored hashed, never in clear")
	}
}

func TestRecoverResetsThePasswordWithAOneShotCode(t *testing.T) {
	s := newTestService(t)
	old, codes := registerWithCodes(t, s, "lost@example.com", "lostone")

	bad := s.post(t, s.Recover, "recover", map[string]string{"email": "lost@example.com", "code": "AAAA-BBBB-CCCC", "password": "brand new pass"})
	if bad.Code != http.StatusUnauthorized || errCode(bad) != "invalid_code" {
		t.Fatalf("wrong code: %d %s", bad.Code, bad.Body)
	}
	short := s.post(t, s.Recover, "recover", map[string]string{"email": "lost@example.com", "code": codes[0], "password": "short"})
	if short.Code != http.StatusUnprocessableEntity || errCode(short) != "invalid_password" {
		t.Fatalf("weak password: %d %s", short.Code, short.Body)
	}
	// lowercase and without dashes must still match
	typed := strings.ToLower(strings.ReplaceAll(codes[0], "-", ""))
	ok := s.post(t, s.Recover, "recover", map[string]string{"email": "lost@example.com", "code": typed, "password": "brand new pass"})
	if ok.Code != http.StatusOK || sessionCookie(ok) == nil {
		t.Fatalf("recover: %d %s", ok.Code, ok.Body)
	}
	if s.CurrentUser(reqWith(old)) != nil {
		t.Fatal("every older session must be revoked")
	}
	if rr := s.post(t, s.Login, "login", map[string]string{"email": "lost@example.com", "password": "brand new pass"}); rr.Code != http.StatusOK {
		t.Fatalf("new password must log in, got %d", rr.Code)
	}
	if rr := s.post(t, s.Login, "login", map[string]string{"email": "lost@example.com", "password": "longenough"}); rr.Code != http.StatusUnauthorized {
		t.Fatalf("old password must stop working, got %d", rr.Code)
	}
	again := s.post(t, s.Recover, "recover", map[string]string{"email": "lost@example.com", "code": codes[0], "password": "another pass 1"})
	if again.Code != http.StatusUnauthorized {
		t.Fatalf("a code works once, got %d", again.Code)
	}
	unknown := s.post(t, s.Recover, "recover", map[string]string{"email": "nobody@example.com", "code": codes[1], "password": "another pass 1"})
	if unknown.Code != http.StatusUnauthorized || errCode(unknown) != "invalid_code" {
		t.Fatalf("unknown email must look like a wrong code, got %d %s", unknown.Code, unknown.Body)
	}
}

func TestRegenerateRecoveryCodesReplacesTheOldOnes(t *testing.T) {
	s := newTestService(t)
	c, old := registerWithCodes(t, s, "regen@example.com", "regen1")
	if rr := s.post(t, s.NewRecoveryCodes, "recovery-codes", map[string]string{"password": "nope-nope"}, c); rr.Code != http.StatusForbidden {
		t.Fatalf("password required, got %d", rr.Code)
	}
	if rr := s.post(t, s.NewRecoveryCodes, "recovery-codes", map[string]string{"password": "longenough"}); rr.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous refused, got %d", rr.Code)
	}
	rr := s.post(t, s.NewRecoveryCodes, "recovery-codes", map[string]string{"password": "longenough"}, c)
	var out struct {
		Codes []string `json:"recovery_codes"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil || rr.Code != http.StatusOK || len(out.Codes) != recoveryCodeCount {
		t.Fatalf("regenerate: %d %s", rr.Code, rr.Body)
	}
	if rr := s.post(t, s.Recover, "recover", map[string]string{"email": "regen@example.com", "code": old[0], "password": "brand new pass"}); rr.Code != http.StatusUnauthorized {
		t.Fatalf("old codes must be dead, got %d", rr.Code)
	}
	if rr := s.post(t, s.Recover, "recover", map[string]string{"email": "regen@example.com", "code": out.Codes[0], "password": "brand new pass"}); rr.Code != http.StatusOK {
		t.Fatalf("new code must work, got %d", rr.Code)
	}
}
