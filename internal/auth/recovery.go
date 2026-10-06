package auth

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

const (
	recoveryCodeCount = 8
	// Crockford-style alphabet: no 0/O/1/I/L, so a code survives being read aloud or copied by hand.
	recoveryAlphabet = "23456789ABCDEFGHJKMNPQRSTVWXYZ"
)

// newRecoveryCodes returns n random codes shaped XXXX-XXXX-XXXX (60 bits each).
func newRecoveryCodes(n int) []string {
	codes := make([]string, n)
	for i := range codes {
		raw := random(12)
		var b strings.Builder
		for j, v := range raw {
			if j > 0 && j%4 == 0 {
				b.WriteByte('-')
			}
			b.WriteByte(recoveryAlphabet[int(v)%len(recoveryAlphabet)])
		}
		codes[i] = b.String()
	}
	return codes
}

// normalizeRecoveryCode accepts a code typed in lowercase or without dashes.
func normalizeRecoveryCode(code string) string {
	code = strings.ToUpper(strings.NewReplacer("-", "", " ", "").Replace(code))
	if len(code) != 12 {
		return code
	}
	return code[:4] + "-" + code[4:8] + "-" + code[8:]
}

// recoveryHash is keyed with the server pepper: the codes are random, so no slow hash is needed,
// but a stolen database alone must not reveal them.
func (s *Service) recoveryHash(code string) string {
	return s.keys.blindIndex("recovery|" + normalizeRecoveryCode(code))
}

// issueRecoveryCodes replaces a user's codes and returns the new ones in clear (shown once).
func (s *Service) issueRecoveryCodes(userID int64) ([]string, error) {
	codes := newRecoveryCodes(recoveryCodeCount)
	hashes := make([]string, len(codes))
	for i, c := range codes {
		hashes[i] = s.recoveryHash(c)
	}
	return codes, s.store.ReplaceRecoveryCodes(userID, hashes, s.now())
}

// Recover sets a new password for an account from its e-mail and one unused recovery code.
func (s *Service) Recover(w http.ResponseWriter, r *http.Request) {
	plain, ok := s.decode(w, r, "recover")
	if !ok {
		return
	}
	c, ok := s.parseCredentials(w, plain)
	if !ok {
		return
	}
	if !validPassword(c.Password) {
		fail(w, http.StatusUnprocessableEntity, "invalid_password")
		return
	}
	idx := s.keys.blindIndex(c.Email)
	if s.throttled(w, "recover-ip|"+s.clientIP(r), 20, 15*time.Minute) || s.throttled(w, "recover-email|"+idx, 5, 15*time.Minute) {
		return
	}
	// Hash before looking the account up, so an unknown e-mail costs the same time as a known one
	// (login does the same with its dummy hash) and the answer cannot be used to probe accounts.
	hash, err := s.keys.hashPassword(c.Password)
	if err != nil {
		fail(w, http.StatusInternalServerError, "server_error")
		return
	}
	user, err := s.store.UserByEmailIdx(idx)
	if err != nil {
		fail(w, http.StatusUnauthorized, "invalid_code")
		return
	}
	used, err := s.store.UseRecoveryCode(user.ID, s.recoveryHash(c.Code), hash, s.now())
	if err != nil {
		fail(w, http.StatusInternalServerError, "server_error")
		return
	}
	if !used {
		fail(w, http.StatusUnauthorized, "invalid_code")
		return
	}
	if s.setSession(w, r, user.ID) != nil {
		fail(w, http.StatusInternalServerError, "server_error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": s.view(user)})
}

// NewRecoveryCodes replaces the signed-in account's codes after a password check.
func (s *Service) NewRecoveryCodes(w http.ResponseWriter, r *http.Request) {
	u := s.currentUser(r)
	if u == nil {
		fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	plain, ok := s.decode(w, r, "recovery-codes")
	if !ok {
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	if json.Unmarshal(plain, &body) != nil {
		fail(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if s.throttled(w, "recovery-codes|"+s.clientIP(r), 5, 15*time.Minute) {
		return
	}
	if !s.keys.verifyPassword(body.Password, u.PassHash) {
		fail(w, http.StatusForbidden, "invalid_credentials")
		return
	}
	codes, err := s.issueRecoveryCodes(u.ID)
	if err != nil {
		fail(w, http.StatusInternalServerError, "server_error")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"recovery_codes": codes})
}
