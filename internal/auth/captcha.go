package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Self-hosted ALTCHA: a signed SHA-256 proof-of-work. The server signs a
// challenge; the browser finds the number that reproduces it; the solved
// payload is verified once, before its expiry.
const (
	captchaLifetime  = 5 * time.Minute
	captchaMaxNumber = 150000
)

// Challenge is the JSON the ALTCHA client library expects.
type Challenge struct {
	Algorithm string `json:"algorithm"`
	Challenge string `json:"challenge"`
	Salt      string `json:"salt"`
	Signature string `json:"signature"`
	MaxNumber int    `json:"maxnumber"`
}

type solution struct {
	Algorithm string `json:"algorithm"`
	Challenge string `json:"challenge"`
	Number    int    `json:"number"`
	Salt      string `json:"salt"`
	Signature string `json:"signature"`
}

// Captcha issues and verifies challenges.
type Captcha struct {
	key  []byte
	mu   sync.Mutex
	used map[string]time.Time
	// shared, when set, records solved challenges in Redis so a replay fails on every instance.
	shared State
	now    func() time.Time
}

func NewCaptcha(key []byte) *Captcha {
	return &Captcha{key: key, used: map[string]time.Time{}, now: time.Now}
}

func (c *Captcha) sign(challenge string) string {
	mac := hmac.New(sha256.New, c.key)
	mac.Write([]byte(challenge))
	return hex.EncodeToString(mac.Sum(nil))
}

func proof(salt string, number int) string {
	sum := sha256.Sum256([]byte(salt + strconv.Itoa(number)))
	return hex.EncodeToString(sum[:])
}

// New creates a fresh challenge.
func (c *Captcha) New() Challenge {
	n, err := rand.Int(rand.Reader, big.NewInt(captchaMaxNumber+1))
	if err != nil {
		panic(err)
	}
	salt := hex.EncodeToString(random(12)) + "?expires=" + strconv.FormatInt(c.now().Add(captchaLifetime).Unix(), 10)
	challenge := proof(salt, int(n.Int64()))
	return Challenge{Algorithm: "SHA-256", Challenge: challenge, Salt: salt, Signature: c.sign(challenge), MaxNumber: captchaMaxNumber}
}

// Verify checks a base64 JSON payload from the client and consumes it.
func (c *Captcha) Verify(payload string) bool {
	raw, err := base64.StdEncoding.DecodeString(payload)
	if err != nil || len(raw) > 2048 {
		return false
	}
	var s solution
	if json.Unmarshal(raw, &s) != nil || s.Algorithm != "SHA-256" || s.Number < 0 || s.Number > captchaMaxNumber {
		return false
	}
	if subtle.ConstantTimeCompare([]byte(c.sign(s.Challenge)), []byte(s.Signature)) != 1 {
		return false
	}
	if subtle.ConstantTimeCompare([]byte(proof(s.Salt, s.Number)), []byte(s.Challenge)) != 1 {
		return false
	}
	_, query, ok := strings.Cut(s.Salt, "?expires=")
	if !ok {
		return false
	}
	exp, err := strconv.ParseInt(query, 10, 64)
	if err != nil {
		return false
	}
	expires := time.Unix(exp, 0)
	now := c.now()
	if now.After(expires) {
		return false
	}
	if c.shared != nil {
		first, err := c.shared.Once("captcha:"+s.Signature, time.Until(expires)+time.Second)
		return err == nil && first
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for sig, until := range c.used {
		if now.After(until) {
			delete(c.used, sig)
		}
	}
	if _, replay := c.used[s.Signature]; replay {
		return false
	}
	c.used[s.Signature] = expires
	return true
}
