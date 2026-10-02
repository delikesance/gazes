package auth

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/mail"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	cookieName      = "gazes_session"
	sessionLifetime = 30 * 24 * time.Hour
	maxBody         = 16 << 10
)

var pseudoPattern = regexp.MustCompile(`^[\p{L}\p{N}_.-]{3,24}$`)

// Options configure a Service.
type Options struct {
	Dir        string
	Production bool
	TrustProxy bool // honour X-Forwarded-For / X-Forwarded-Proto from the edge proxy
	Getenv     func(string) string
}

// Service wires the store, keys, envelope, captcha and rate limits to HTTP handlers.
type Service struct {
	store      *Store
	keys       *Keys
	kem        *KEM
	captcha    *Captcha
	limits     *limiter
	trustProxy bool
	now        func() time.Time
	dummyHash  string
}

// New opens the store and loads keys.
func New(o Options) (*Service, error) {
	keys, err := LoadKeys(o.Dir, o.Production, o.Getenv)
	if err != nil {
		return nil, err
	}
	store, err := OpenStore(o.Dir)
	if err != nil {
		return nil, err
	}
	kem, err := LoadKEM(o.Dir)
	if err != nil {
		store.Close()
		return nil, err
	}
	dummy, _ := keys.hashPassword("gazes-dummy-password")
	s := &Service{store: store, keys: keys, kem: kem, captcha: NewCaptcha(keys.Altcha), limits: newLimiter(), trustProxy: o.TrustProxy, now: time.Now, dummyHash: dummy}
	go s.janitor()
	return s, nil
}

func (s *Service) Close() error { return s.store.Close() }

func (s *Service) janitor() {
	for range time.Tick(time.Hour) {
		s.store.PurgeExpired(s.now())
	}
}

// ---- helpers ---------------------------------------------------------------

type apiError struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, apiError{Error: code})
}

func (s *Service) clientIP(r *http.Request) string {
	if s.trustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			first, _, _ := strings.Cut(xff, ",")
			if ip := strings.TrimSpace(first); net.ParseIP(ip) != nil {
				return ip
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (s *Service) secure(r *http.Request) bool {
	return r.TLS != nil || (s.trustProxy && r.Header.Get("X-Forwarded-Proto") == "https")
}

// sameOrigin blocks cross-site form posts: JSON content type plus a matching Origin.
func (s *Service) sameOrigin(r *http.Request) bool {
	if ct, _, _ := strings.Cut(r.Header.Get("Content-Type"), ";"); strings.TrimSpace(ct) != "application/json" {
		return false
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	host := r.Host
	if s.trustProxy {
		if fh := r.Header.Get("X-Forwarded-Host"); fh != "" {
			host = fh
		}
	}
	return strings.EqualFold(u.Host, host)
}

func (s *Service) setSession(w http.ResponseWriter, r *http.Request, userID int64) error {
	token := randomToken()
	now := s.now()
	if err := s.store.CreateSession(hashToken(token), userID, now.Add(sessionLifetime), now); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: token, Path: "/", HttpOnly: true, Secure: s.secure(r), SameSite: http.SameSiteLaxMode, MaxAge: int(sessionLifetime.Seconds())})
	return nil
}

func (s *Service) currentUser(r *http.Request) *User {
	c, err := r.Cookie(cookieName)
	if err != nil || c.Value == "" {
		return nil
	}
	u, err := s.store.SessionUser(hashToken(c.Value), s.now(), sessionLifetime)
	if err != nil {
		return nil
	}
	return u
}

// decode reads an encrypted envelope body and returns the plaintext payload.
func (s *Service) decode(w http.ResponseWriter, r *http.Request, route string) ([]byte, bool) {
	if !s.limits.hit("req|"+s.clientIP(r), 60, time.Minute) {
		fail(w, http.StatusTooManyRequests, "rate_limited")
		return nil, false
	}
	if !s.sameOrigin(r) {
		fail(w, http.StatusForbidden, "forbidden")
		return nil, false
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBody))
	var env Envelope
	if err != nil || json.Unmarshal(body, &env) != nil {
		fail(w, http.StatusBadRequest, "invalid_request")
		return nil, false
	}
	plain, err := s.kem.Open(env, route)
	if err != nil {
		fail(w, http.StatusBadRequest, "invalid_request")
		return nil, false
	}
	return plain, true
}

type userView struct {
	ID     int64  `json:"id"`
	Pseudo string `json:"pseudo"`
	Email  string `json:"email,omitempty"`
}

func (s *Service) view(u *User) userView {
	v := userView{ID: u.ID, Pseudo: u.Pseudo}
	if email, err := s.keys.decryptEmail(u.EmailEnc); err == nil {
		v.Email = email
	}
	return v
}

func validEmail(email string) bool {
	if len(email) > 254 || !utf8.ValidString(email) {
		return false
	}
	a, err := mail.ParseAddress(email)
	return err == nil && a.Address == email && strings.Contains(email[strings.LastIndex(email, "@"):], ".")
}

func validPassword(p string) bool { return len(p) >= 8 && len(p) <= 256 && utf8.ValidString(p) }

// ---- handlers --------------------------------------------------------------

// Kem returns the public key and a single-use nonce for the next request.
func (s *Service) Kem(w http.ResponseWriter, r *http.Request) {
	if !s.limits.hit("req|"+s.clientIP(r), 60, time.Minute) {
		fail(w, http.StatusTooManyRequests, "rate_limited")
		return
	}
	writeJSON(w, http.StatusOK, s.kem.Issue())
}

// CaptchaChallenge issues a proof-of-work challenge.
func (s *Service) CaptchaChallenge(w http.ResponseWriter, r *http.Request) {
	if !s.limits.hit("req|"+s.clientIP(r), 60, time.Minute) {
		fail(w, http.StatusTooManyRequests, "rate_limited")
		return
	}
	writeJSON(w, http.StatusOK, s.captcha.New())
}

type credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Pseudo   string `json:"pseudo"`
	Captcha  string `json:"captcha"`
}

func (s *Service) parseCredentials(w http.ResponseWriter, plain []byte) (credentials, bool) {
	var c credentials
	if json.Unmarshal(plain, &c) != nil {
		fail(w, http.StatusBadRequest, "invalid_request")
		return c, false
	}
	c.Email = normalizeEmail(c.Email)
	if !s.captcha.Verify(c.Captcha) {
		fail(w, http.StatusBadRequest, "captcha_failed")
		return c, false
	}
	return c, true
}

func (s *Service) Register(w http.ResponseWriter, r *http.Request) {
	plain, ok := s.decode(w, r, "register")
	if !ok {
		return
	}
	c, ok := s.parseCredentials(w, plain)
	if !ok {
		return
	}
	if !validEmail(c.Email) {
		fail(w, http.StatusUnprocessableEntity, "invalid_email")
		return
	}
	if !pseudoPattern.MatchString(c.Pseudo) {
		fail(w, http.StatusUnprocessableEntity, "invalid_pseudo")
		return
	}
	if !validPassword(c.Password) {
		fail(w, http.StatusUnprocessableEntity, "invalid_password")
		return
	}
	hash, err := s.keys.hashPassword(c.Password)
	enc, err2 := s.keys.encryptEmail(c.Email)
	if err != nil || err2 != nil {
		fail(w, http.StatusInternalServerError, "server_error")
		return
	}
	id, err := s.store.CreateUser(User{EmailIdx: s.keys.blindIndex(c.Email), EmailEnc: enc, Pseudo: c.Pseudo, PassHash: hash}, s.now())
	if errors.Is(err, ErrEmailTaken) {
		fail(w, http.StatusConflict, "email_taken")
		return
	}
	if err != nil || s.setSession(w, r, id) != nil {
		fail(w, http.StatusInternalServerError, "server_error")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"user": userView{ID: id, Pseudo: c.Pseudo, Email: c.Email}})
}

func (s *Service) Login(w http.ResponseWriter, r *http.Request) {
	plain, ok := s.decode(w, r, "login")
	if !ok {
		return
	}
	c, ok := s.parseCredentials(w, plain)
	if !ok {
		return
	}
	idx := s.keys.blindIndex(c.Email)
	ipKey, emailKey := "login-ip|"+s.clientIP(r), "login-email|"+idx
	if !s.limits.hit(ipKey, 30, 15*time.Minute) || !s.limits.hit(emailKey, 8, 15*time.Minute) {
		fail(w, http.StatusTooManyRequests, "rate_limited")
		return
	}
	user, err := s.store.UserByEmailIdx(idx)
	hash := s.dummyHash // run argon2 either way so timing does not reveal which emails exist
	if err == nil {
		hash = user.PassHash
	}
	valid := s.keys.verifyPassword(c.Password, hash)
	if err != nil || !valid {
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			fail(w, http.StatusInternalServerError, "server_error")
			return
		}
		fail(w, http.StatusUnauthorized, "invalid_credentials")
		return
	}
	s.limits.reset(emailKey)
	if s.setSession(w, r, user.ID) != nil {
		fail(w, http.StatusInternalServerError, "server_error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": s.view(user)})
}

func (s *Service) Logout(w http.ResponseWriter, r *http.Request) {
	if !s.sameOrigin(r) {
		fail(w, http.StatusForbidden, "forbidden")
		return
	}
	if c, err := r.Cookie(cookieName); err == nil {
		_ = s.store.DeleteSession(hashToken(c.Value))
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", HttpOnly: true, Secure: s.secure(r), SameSite: http.SameSiteLaxMode, MaxAge: -1})
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) Me(w http.ResponseWriter, r *http.Request) {
	u := s.currentUser(r)
	if u == nil {
		writeJSON(w, http.StatusOK, map[string]any{"user": nil})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": s.view(u)})
}

func (s *Service) GetProgress(w http.ResponseWriter, r *http.Request) {
	u := s.currentUser(r)
	if u == nil {
		fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	items, err := s.store.ListProgress(u.ID)
	if err != nil {
		fail(w, http.StatusInternalServerError, "server_error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"progress": items})
}

// PutProgress merges client entries (newest wins) and returns the merged state.
func (s *Service) PutProgress(w http.ResponseWriter, r *http.Request) {
	u := s.currentUser(r)
	if u == nil {
		fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !s.sameOrigin(r) {
		fail(w, http.StatusForbidden, "forbidden")
		return
	}
	var body struct {
		Progress []Progress `json:"progress"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&body) != nil || len(body.Progress) > 500 {
		fail(w, http.StatusBadRequest, "invalid_request")
		return
	}
	now := s.now().Unix()
	for i := range body.Progress {
		p := &body.Progress[i]
		if p.SeasonID <= 0 || p.AnimeID < 0 || p.Episode <= 0 || p.Position < 0 || p.UpdatedAt <= 0 || p.UpdatedAt > now+300 || utf8.RuneCountInString(p.Title) > 200 || !utf8.ValidString(p.Title) {
			fail(w, http.StatusBadRequest, "invalid_request")
			return
		}
	}
	if err := s.store.MergeProgress(u.ID, body.Progress); err != nil {
		fail(w, http.StatusInternalServerError, "server_error")
		return
	}
	s.GetProgress(w, r)
}
