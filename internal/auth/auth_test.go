package auth

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/mlkem"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	svc, err := New(Options{Dir: t.TempDir(), Getenv: func(string) string { return "" }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { svc.Close() })
	return svc
}

// seal plays the browser: encapsulate, derive, encrypt.
func seal(t *testing.T, info KEMInfo, route string, payload any) Envelope {
	t.Helper()
	ekBytes, _ := base64.StdEncoding.DecodeString(info.EK)
	ek, err := mlkem.NewEncapsulationKey768(ekBytes)
	if err != nil {
		t.Fatal(err)
	}
	secret, ct := ek.Encapsulate()
	nonce, _ := base64.StdEncoding.DecodeString(info.Nonce)
	key, _ := hkdf.Key(sha256.New, secret, nonce, envelopeInfo+route, 32)
	block, _ := aes.NewCipher(key)
	gcm, _ := cipher.NewGCM(block)
	iv := random(12)
	plain, _ := json.Marshal(payload)
	data := gcm.Seal(nil, iv, plain, envelopeAAD(info.KID, route, nonce))
	b64 := base64.StdEncoding.EncodeToString
	return Envelope{KID: info.KID, CT: b64(ct), Nonce: info.Nonce, IV: b64(iv), Data: b64(data)}
}

// solve plays the browser's proof-of-work.
func solve(t *testing.T, c Challenge) string {
	t.Helper()
	for n := 0; n <= c.MaxNumber; n++ {
		if proof(c.Salt, n) == c.Challenge {
			raw, _ := json.Marshal(solution{Algorithm: c.Algorithm, Challenge: c.Challenge, Number: n, Salt: c.Salt, Signature: c.Signature})
			return base64.StdEncoding.EncodeToString(raw)
		}
	}
	t.Fatal("challenge unsolvable")
	return ""
}

func (s *Service) kemInfo(t *testing.T) KEMInfo {
	rr := httptest.NewRecorder()
	s.Kem(rr, httptest.NewRequest("GET", "/auth/kem", nil))
	var info KEMInfo
	if err := json.Unmarshal(rr.Body.Bytes(), &info); err != nil {
		t.Fatal(err)
	}
	return info
}

func (s *Service) newCaptcha(t *testing.T) string {
	rr := httptest.NewRecorder()
	s.CaptchaChallenge(rr, httptest.NewRequest("GET", "/auth/captcha", nil))
	var c Challenge
	if err := json.Unmarshal(rr.Body.Bytes(), &c); err != nil {
		t.Fatal(err)
	}
	return solve(t, c)
}

func (s *Service) post(t *testing.T, h http.HandlerFunc, route string, payload map[string]string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	if _, ok := payload["captcha"]; !ok {
		payload["captcha"] = s.newCaptcha(t)
	}
	body, _ := json.Marshal(seal(t, s.kemInfo(t), route, payload))
	req := httptest.NewRequest("POST", "/auth/"+route, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rr := httptest.NewRecorder()
	h(rr, req)
	return rr
}

func sessionCookie(rr *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range rr.Result().Cookies() {
		if c.Name == cookieName && c.Value != "" {
			return c
		}
	}
	return nil
}

func errCode(rr *httptest.ResponseRecorder) string {
	var e apiError
	_ = json.Unmarshal(rr.Body.Bytes(), &e)
	return e.Error
}

func TestRegisterLoginMeLogout(t *testing.T) {
	s := newTestService(t)
	reg := s.post(t, s.Register, "register", map[string]string{"email": "Ada@Example.com", "password": "correct horse", "pseudo": "ada"})
	if reg.Code != http.StatusCreated {
		t.Fatalf("register: %d %s", reg.Code, reg.Body)
	}
	c := sessionCookie(reg)
	if c == nil || !c.HttpOnly || c.SameSite != http.SameSiteLaxMode {
		t.Fatalf("session cookie flags: %#v", c)
	}
	me := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/auth/me", nil)
	req.AddCookie(c)
	s.Me(me, req)
	if !strings.Contains(me.Body.String(), `"email":"ada@example.com"`) {
		t.Fatalf("me: %s", me.Body)
	}
	login := s.post(t, s.Login, "login", map[string]string{"email": "ada@example.com", "password": "correct horse"})
	if login.Code != http.StatusOK || sessionCookie(login) == nil {
		t.Fatalf("login: %d %s", login.Code, login.Body)
	}
	out := httptest.NewRecorder()
	lreq := httptest.NewRequest("POST", "/auth/logout", nil)
	lreq.Header.Set("Content-Type", "application/json")
	lreq.AddCookie(c)
	s.Logout(out, lreq)
	if out.Code != http.StatusNoContent {
		t.Fatalf("logout: %d", out.Code)
	}
	again := httptest.NewRecorder()
	areq := httptest.NewRequest("GET", "/auth/me", nil)
	areq.AddCookie(c)
	s.Me(again, areq)
	if !strings.Contains(again.Body.String(), `"user":null`) {
		t.Fatalf("session must be revoked: %s", again.Body)
	}
}

func TestRegisterRejectsBadInputAndDuplicates(t *testing.T) {
	s := newTestService(t)
	ok := map[string]string{"email": "bob@example.com", "password": "longenough", "pseudo": "bob"}
	if rr := s.post(t, s.Register, "register", copyMap(ok)); rr.Code != http.StatusCreated {
		t.Fatalf("first register: %d %s", rr.Code, rr.Body)
	}
	dup := copyMap(ok)
	dup["email"] = " BOB@example.com "
	if rr := s.post(t, s.Register, "register", dup); rr.Code != http.StatusConflict || errCode(rr) != "email_taken" {
		t.Fatalf("duplicate: %d %s", rr.Code, rr.Body)
	}
	for field, bad := range map[string]string{"email": "not-an-email", "pseudo": "a", "password": "short"} {
		p := copyMap(ok)
		p["email"] = "new@example.com"
		p[field] = bad
		if rr := s.post(t, s.Register, "register", p); rr.Code != http.StatusUnprocessableEntity {
			t.Fatalf("%s=%q: %d %s", field, bad, rr.Code, rr.Body)
		}
	}
}

func copyMap(m map[string]string) map[string]string {
	c := map[string]string{}
	for k, v := range m {
		c[k] = v
	}
	return c
}

func TestLoginWrongPasswordIsGeneric(t *testing.T) {
	s := newTestService(t)
	s.post(t, s.Register, "register", map[string]string{"email": "eve@example.com", "password": "rightpassword", "pseudo": "eve"})
	wrong := s.post(t, s.Login, "login", map[string]string{"email": "eve@example.com", "password": "wrongpassword"})
	missing := s.post(t, s.Login, "login", map[string]string{"email": "nobody@example.com", "password": "wrongpassword"})
	if wrong.Code != http.StatusUnauthorized || missing.Code != http.StatusUnauthorized || errCode(wrong) != errCode(missing) {
		t.Fatalf("wrong=%d/%s missing=%d/%s", wrong.Code, errCode(wrong), missing.Code, errCode(missing))
	}
}

func TestEnvelopeRejectsReplayTamperingAndWrongRoute(t *testing.T) {
	s := newTestService(t)
	info := s.kemInfo(t)
	env := seal(t, info, "login", map[string]string{"email": "a@b.co"})
	if _, err := s.kem.Open(env, "login"); err != nil {
		t.Fatalf("first open: %v", err)
	}
	if _, err := s.kem.Open(env, "login"); err == nil {
		t.Fatal("replayed nonce must be rejected")
	}
	tampered := seal(t, s.kemInfo(t), "login", map[string]string{"email": "a@b.co"})
	raw, _ := base64.StdEncoding.DecodeString(tampered.Data)
	raw[0] ^= 1
	tampered.Data = base64.StdEncoding.EncodeToString(raw)
	if _, err := s.kem.Open(tampered, "login"); err == nil {
		t.Fatal("tampered ciphertext must be rejected")
	}
	wrongRoute := seal(t, s.kemInfo(t), "login", map[string]string{"email": "a@b.co"})
	if _, err := s.kem.Open(wrongRoute, "register"); err == nil {
		t.Fatal("an envelope is bound to its route")
	}
	badKID := seal(t, s.kemInfo(t), "login", map[string]string{})
	badKID.KID = "deadbeefdeadbeef"
	if _, err := s.kem.Open(badKID, "login"); err == nil {
		t.Fatal("unknown kid must be rejected")
	}
}

func TestEnvelopeNonceExpires(t *testing.T) {
	s := newTestService(t)
	env := seal(t, s.kemInfo(t), "login", map[string]string{})
	s.kem.now = func() time.Time { return time.Now().Add(nonceLifetime + time.Second) }
	if _, err := s.kem.Open(env, "login"); err == nil {
		t.Fatal("expired nonce must be rejected")
	}
}

func TestCaptchaValidExpiredReplayedForged(t *testing.T) {
	s := newTestService(t)
	rr := httptest.NewRecorder()
	s.CaptchaChallenge(rr, httptest.NewRequest("GET", "/auth/captcha", nil))
	var c Challenge
	_ = json.Unmarshal(rr.Body.Bytes(), &c)
	payload := solve(t, c)
	if !s.captcha.Verify(payload) {
		t.Fatal("valid solution rejected")
	}
	if s.captcha.Verify(payload) {
		t.Fatal("solution must be single-use")
	}
	forged := c
	forged.Signature = strings.Repeat("0", 64)
	if s.captcha.Verify(solve(t, forged)) {
		t.Fatal("wrong signature must be rejected")
	}
	c2 := s.captcha.New()
	pl := solve(t, c2)
	s.captcha.now = func() time.Time { return time.Now().Add(captchaLifetime + time.Minute) }
	if s.captcha.Verify(pl) {
		t.Fatal("expired challenge must be rejected")
	}
	if s.captcha.Verify("not base64 json") {
		t.Fatal("garbage must be rejected")
	}
}

func TestRegisterRequiresCaptcha(t *testing.T) {
	s := newTestService(t)
	rr := s.post(t, s.Register, "register", map[string]string{"email": "c@example.com", "password": "longenough", "pseudo": "carol", "captcha": ""})
	if rr.Code != http.StatusBadRequest || errCode(rr) != "captcha_failed" {
		t.Fatalf("%d %s", rr.Code, rr.Body)
	}
}

func TestLoginRateLimit(t *testing.T) {
	s := newTestService(t)
	var last *httptest.ResponseRecorder
	for i := 0; i < 10; i++ {
		last = s.post(t, s.Login, "login", map[string]string{"email": "rl@example.com", "password": "wrongpassword" + strconv.Itoa(i)})
	}
	if last.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 after repeated failures, got %d", last.Code)
	}
}

func TestEmailAndPasswordAreNotStoredInClear(t *testing.T) {
	dir := t.TempDir()
	s, err := New(Options{Dir: dir, Getenv: func(string) string { return "" }})
	if err != nil {
		t.Fatal(err)
	}
	s.post(t, s.Register, "register", map[string]string{"email": "secret.person@example.com", "password": "hunter2hunter2", "pseudo": "sp"})
	s.Close()
	for _, name := range []string{"accounts.sqlite", "accounts.sqlite-wal"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		if bytes.Contains(data, []byte("secret.person")) || bytes.Contains(data, []byte("hunter2")) {
			t.Fatalf("%s leaks credentials", name)
		}
	}
}

func TestSameOriginGuard(t *testing.T) {
	s := newTestService(t)
	for _, tc := range []struct {
		ct, origin string
		ok         bool
	}{
		{"application/json", "", true},
		{"application/json", "http://example.com", true},
		{"application/json", "http://evil.test", false},
		{"text/plain", "", false},
		{"application/x-www-form-urlencoded", "", false},
	} {
		req := httptest.NewRequest("POST", "http://example.com/auth/login", nil)
		req.Header.Set("Content-Type", tc.ct)
		if tc.origin != "" {
			req.Header.Set("Origin", tc.origin)
		}
		if got := s.sameOrigin(req); got != tc.ok {
			t.Errorf("%+v: got %v", tc, got)
		}
	}
}

func TestProgressMergeNewestWinsAndRequiresLogin(t *testing.T) {
	s := newTestService(t)
	reg := s.post(t, s.Register, "register", map[string]string{"email": "p@example.com", "password": "longenough", "pseudo": "prog"})
	c := sessionCookie(reg)
	put := func(items []Progress) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]any{"progress": items})
		req := httptest.NewRequest("PUT", "/me/progress", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(c)
		rr := httptest.NewRecorder()
		s.PutProgress(rr, req)
		return rr
	}
	now := time.Now().Unix()
	put([]Progress{{SeasonID: 7, AnimeID: 70, Title: "Seven", Episode: 3, Position: 100, UpdatedAt: now}})
	rr := put([]Progress{{SeasonID: 7, Episode: 2, Position: 50, UpdatedAt: now - 100}, {SeasonID: 8, Episode: 1, Position: 20, UpdatedAt: now}})
	var got struct{ Progress []Progress }
	_ = json.Unmarshal(rr.Body.Bytes(), &got)
	by := map[int64]Progress{}
	for _, p := range got.Progress {
		by[p.SeasonID] = p
	}
	if by[7].Episode != 3 || by[7].Title != "Seven" || by[7].AnimeID != 70 || by[8].Episode != 1 {
		t.Fatalf("newest must win: %+v", got.Progress)
	}
	anon := httptest.NewRecorder()
	s.GetProgress(anon, httptest.NewRequest("GET", "/me/progress", nil))
	if anon.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous: %d", anon.Code)
	}
}

func TestProgressRejectsOversizedTitle(t *testing.T) {
	s := newTestService(t)
	c := sessionCookie(s.post(t, s.Register, "register", map[string]string{"email": "t@example.com", "password": "longenough", "pseudo": "titler"}))
	body, _ := json.Marshal(map[string]any{"progress": []Progress{{SeasonID: 1, Episode: 1, Title: strings.Repeat("x", 201), UpdatedAt: time.Now().Unix()}}})
	req := httptest.NewRequest("PUT", "/me/progress", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(c)
	rr := httptest.NewRecorder()
	s.PutProgress(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("got %d", rr.Code)
	}
}

func TestProductionRequiresKeys(t *testing.T) {
	if _, err := LoadKeys(t.TempDir(), true, func(string) string { return "" }); err == nil {
		t.Fatal("production without keys must fail")
	}
	if _, err := LoadKeys(t.TempDir(), true, func(string) string { return "short" }); err == nil {
		t.Fatal("malformed key must fail")
	}
}

func TestDevKeysPersist(t *testing.T) {
	dir := t.TempDir()
	get := func(string) string { return "" }
	a, _ := LoadKeys(dir, false, get)
	b, _ := LoadKeys(dir, false, get)
	if !bytes.Equal(a.Enc, b.Enc) || !bytes.Equal(a.Pepper, b.Pepper) {
		t.Fatal("dev keys must persist across restarts")
	}
	k1, _ := LoadKEM(dir)
	k2, _ := LoadKEM(dir)
	if k1.kid != k2.kid {
		t.Fatal("kem key must persist")
	}
}

func TestWatchHistoryKeepsCompletedSessionsAndNewestWins(t *testing.T) {
	s := newTestService(t)
	c := sessionCookie(s.post(t, s.Register, "register", map[string]string{"email": "h@example.com", "password": "longenough", "pseudo": "hist"}))
	call := func(method string, handler http.HandlerFunc, payload any, query string) *httptest.ResponseRecorder {
		var body []byte
		if payload != nil {
			body, _ = json.Marshal(payload)
		}
		req := httptest.NewRequest(method, "/me/history"+query, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(c)
		rr := httptest.NewRecorder()
		handler(rr, req)
		return rr
	}
	now := time.Now().Unix()
	first := WatchSession{ID: "a", SeasonID: 7, AnimeID: 70, Episode: 1, Title: "Seven", Genres: []string{"Action", "Fantasy"}, Format: "TV", StartedAt: now - 600, UpdatedAt: now - 300, EndPosition: 100, WatchedSeconds: 90, Duration: 1400, AudioLang: "jpn", SubLang: "fre", TZOffset: 120}
	if rr := call("PUT", s.PutHistory, map[string]any{"sessions": []WatchSession{first}}, ""); rr.Code != http.StatusOK {
		t.Fatalf("put: %d %s", rr.Code, rr.Body)
	}
	done := first
	done.UpdatedAt, done.EndPosition, done.WatchedSeconds, done.Completed = now, 1400, 1380, true
	stale := first
	stale.UpdatedAt, stale.WatchedSeconds = now-400, 10
	call("PUT", s.PutHistory, map[string]any{"sessions": []WatchSession{done, stale}}, "")
	var got struct{ Sessions []WatchSession }
	_ = json.Unmarshal(call("GET", s.GetHistory, nil, "").Body.Bytes(), &got)
	if len(got.Sessions) != 1 || !got.Sessions[0].Completed || got.Sessions[0].WatchedSeconds != 1380 || got.Sessions[0].Genres[1] != "Fantasy" || got.Sessions[0].SubLang != "fre" {
		t.Fatalf("completed session must be kept and the newest write win: %+v", got.Sessions)
	}
	bad := first
	bad.ID, bad.WatchedSeconds = "b", -1
	if rr := call("PUT", s.PutHistory, map[string]any{"sessions": []WatchSession{bad}}, ""); rr.Code != http.StatusBadRequest {
		t.Fatalf("negative watch time must be rejected, got %d", rr.Code)
	}
	if rr := call("GET", s.GetHistory, nil, "?since="+strconv.FormatInt(now, 10)); !strings.Contains(rr.Body.String(), `"sessions":[]`) {
		t.Fatalf("since must filter: %s", rr.Body)
	}
}

func TestHiddenAnimeAddRemoveAndValidation(t *testing.T) {
	s := newTestService(t)
	c := sessionCookie(s.post(t, s.Register, "register", map[string]string{"email": "n@example.com", "password": "longenough", "pseudo": "nope"}))
	call := func(handler http.HandlerFunc, payload any) *httptest.ResponseRecorder {
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest("PUT", "/me/hidden", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(c)
		rr := httptest.NewRecorder()
		handler(rr, req)
		return rr
	}
	var got struct{ IDs []int64 }
	_ = json.Unmarshal(call(s.PutHidden, map[string]any{"add": []int64{10, 20, 10}}).Body.Bytes(), &got)
	if len(got.IDs) != 2 {
		t.Fatalf("adding twice must not duplicate: %v", got.IDs)
	}
	_ = json.Unmarshal(call(s.PutHidden, map[string]any{"remove": []int64{10}}).Body.Bytes(), &got)
	if len(got.IDs) != 1 || got.IDs[0] != 20 {
		t.Fatalf("remove: %v", got.IDs)
	}
	if rr := call(s.PutHidden, map[string]any{"add": []int64{0}}); rr.Code != http.StatusBadRequest {
		t.Fatalf("invalid id must be rejected, got %d", rr.Code)
	}
	req := httptest.NewRequest("GET", "/me/hidden", nil)
	rr := httptest.NewRecorder()
	s.GetHidden(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous must be refused, got %d", rr.Code)
	}
}

func TestDeleteHistoryErasesLogAndHiddenButKeepsProgress(t *testing.T) {
	s := newTestService(t)
	c := sessionCookie(s.post(t, s.Register, "register", map[string]string{"email": "d@example.com", "password": "longenough", "pseudo": "eraser"}))
	send := func(method string, handler http.HandlerFunc, payload any) *httptest.ResponseRecorder {
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(method, "/me/x", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(c)
		rr := httptest.NewRecorder()
		handler(rr, req)
		return rr
	}
	now := time.Now().Unix()
	send("PUT", s.PutHistory, map[string]any{"sessions": []WatchSession{{ID: "a", SeasonID: 1, Episode: 1, StartedAt: now - 10, UpdatedAt: now, WatchedSeconds: 60}}})
	send("PUT", s.PutHidden, map[string]any{"add": []int64{5}})
	send("PUT", s.PutProgress, map[string]any{"progress": []Progress{{SeasonID: 1, Episode: 1, Position: 60, UpdatedAt: now}}})
	if rr := send("DELETE", s.DeleteHistory, nil); rr.Code != http.StatusOK {
		t.Fatalf("delete: %d", rr.Code)
	}
	var sessions struct{ Sessions []WatchSession }
	var hidden struct{ IDs []int64 }
	var progress struct{ Progress []Progress }
	_ = json.Unmarshal(send("GET", s.GetHistory, nil).Body.Bytes(), &sessions)
	_ = json.Unmarshal(send("GET", s.GetHidden, nil).Body.Bytes(), &hidden)
	_ = json.Unmarshal(send("GET", s.GetProgress, nil).Body.Bytes(), &progress)
	if len(sessions.Sessions) != 0 || len(hidden.IDs) != 0 || len(progress.Progress) != 1 {
		t.Fatalf("log and hidden must go, resume points stay: %d %d %d", len(sessions.Sessions), len(hidden.IDs), len(progress.Progress))
	}
	anon := httptest.NewRecorder()
	s.DeleteHistory(anon, httptest.NewRequest("DELETE", "/me/history", nil))
	if anon.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous must be refused, got %d", anon.Code)
	}
}

func TestCurrentUserReadsSessionCookie(t *testing.T) {
	s := newTestService(t)
	reg := s.post(t, s.Register, "register", map[string]string{"email": "ada@example.com", "password": "correct horse", "pseudo": "ada"})
	c := sessionCookie(reg)
	if c == nil {
		t.Fatalf("register: %d %s", reg.Code, reg.Body)
	}
	anon := httptest.NewRequest("GET", "/", nil)
	if s.CurrentUser(anon) != nil {
		t.Fatal("anonymous request has a user")
	}
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(c)
	if u := s.CurrentUser(req); u == nil || u.Pseudo != "ada" || u.ID == 0 {
		t.Fatalf("user: %+v", u)
	}
	bad := httptest.NewRequest("GET", "/", nil)
	bad.AddCookie(&http.Cookie{Name: cookieName, Value: "forged"})
	if s.CurrentUser(bad) != nil {
		t.Fatal("forged cookie accepted")
	}
}

func TestMeShowsTheRoleOnlyForAnAdministrator(t *testing.T) {
	s := newTestService(t)
	reg := s.post(t, s.Register, "register", map[string]string{"email": "root@example.com", "password": "correct horse", "pseudo": "root"})
	if reg.Code != http.StatusCreated {
		t.Fatalf("register: %d %s", reg.Code, reg.Body)
	}
	cookie := sessionCookie(reg)
	me := func() map[string]any {
		req := httptest.NewRequest("GET", "/auth/me", nil)
		req.AddCookie(cookie)
		rr := httptest.NewRecorder()
		s.Me(rr, req)
		var body struct {
			User map[string]any `json:"user"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil || body.User == nil {
			t.Fatalf("me: %d %s", rr.Code, rr.Body)
		}
		return body.User
	}
	if _, has := me()["role"]; has {
		t.Fatalf("a regular user must not get a role field: %v", me())
	}
	if err := s.store.SetUserRole(t.Context(), "root", RoleAdmin); err != nil {
		t.Fatal(err)
	}
	if got := me()["role"]; got != RoleAdmin {
		t.Fatalf("an administrator must get role=admin, got %v", got)
	}
	if err := s.store.SetUserRole(t.Context(), "root", RoleUser); err != nil {
		t.Fatal(err)
	}
	if _, has := me()["role"]; has {
		t.Fatal("the role must disappear once revoked")
	}
}
