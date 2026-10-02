package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	argonTime    = 3
	argonMemory  = 64 * 1024 // KiB
	argonThreads = 2
	argonKeyLen  = 32
)

// normalizeEmail lower-cases and trims so one address maps to one account.
func normalizeEmail(email string) string { return strings.ToLower(strings.TrimSpace(email)) }

// blindIndex lets the database enforce uniqueness and look accounts up without
// storing a readable address.
func (k *Keys) blindIndex(email string) string {
	mac := hmac.New(sha256.New, k.Index)
	mac.Write([]byte(normalizeEmail(email)))
	return hex.EncodeToString(mac.Sum(nil))
}

func (k *Keys) encryptEmail(email string) (string, error) {
	block, err := aes.NewCipher(k.Enc)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := random(gcm.NonceSize())
	sealed := gcm.Seal(nonce, nonce, []byte(normalizeEmail(email)), []byte("gazes-email-v1"))
	return base64.StdEncoding.EncodeToString(sealed), nil
}

func (k *Keys) decryptEmail(enc string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(k.Enc)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("short ciphertext")
	}
	plain, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], []byte("gazes-email-v1"))
	return string(plain), err
}

// peppered returns HMAC(pepper, password): argon2id never sees the raw password
// and a stolen database alone cannot be attacked offline without the pepper.
func (k *Keys) peppered(password string) []byte {
	mac := hmac.New(sha256.New, k.Pepper)
	mac.Write([]byte(password))
	return mac.Sum(nil)
}

func (k *Keys) hashPassword(password string) (string, error) {
	salt := random(16)
	sum := argon2.IDKey(k.peppered(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(sum)), nil
}

func (k *Keys) verifyPassword(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 5 || parts[0] != "argon2id" {
		return false
	}
	var m, t uint32
	var p uint8
	if _, err := fmt.Sscanf(parts[2], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil {
		return false
	}
	salt, err1 := base64.RawStdEncoding.DecodeString(parts[3])
	want, err2 := base64.RawStdEncoding.DecodeString(parts[4])
	if err1 != nil || err2 != nil || len(want) == 0 {
		return false
	}
	got := argon2.IDKey(k.peppered(password), salt, t, m, p, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

// randomToken returns an unguessable URL-safe token.
func randomToken() string { return base64.RawURLEncoding.EncodeToString(random(32)) }

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
