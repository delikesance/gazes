package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/mlkem"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// The envelope protects credentials with a post-quantum KEM on top of TLS:
//
//	client: (ct, secret) = ML-KEM-768.Encapsulate(ek)
//	key     = HKDF-SHA256(secret, salt = nonce, info = "gazes-auth-v1|" + route)
//	data    = AES-256-GCM(key, iv, plaintext JSON, aad = kid|route|nonce)
//
// The nonce is issued by the server, single-use and short-lived, so a captured
// envelope cannot be replayed.
const (
	envelopeInfo  = "gazes-auth-v1|"
	nonceLifetime = 2 * time.Minute
	maxNonces     = 20000
)

var errEnvelope = errors.New("invalid envelope")

// Envelope is the JSON body of an encrypted request. All fields are base64
// (standard encoding) except kid.
type Envelope struct {
	KID   string `json:"kid"`
	CT    string `json:"ct"`
	Nonce string `json:"nonce"`
	IV    string `json:"iv"`
	Data  string `json:"data"`
}

// KEMInfo is what GET /auth/kem returns.
type KEMInfo struct {
	KID   string `json:"kid"`
	EK    string `json:"ek"`
	Nonce string `json:"nonce"`
}

// KEM holds the server's ML-KEM-768 key and the pending nonces.
type KEM struct {
	dk  *mlkem.DecapsulationKey768
	kid string
	ek  []byte

	mu     sync.Mutex
	nonces map[string]time.Time
	now    func() time.Time
}

// LoadKEM loads the persisted key seed from dir/kem.key or creates it (0600).
func LoadKEM(dir string) (*KEM, error) {
	path := filepath.Join(dir, "kem.key")
	var dk *mlkem.DecapsulationKey768
	if seed, err := os.ReadFile(path); err == nil {
		if k, kerr := mlkem.NewDecapsulationKey768(seed); kerr == nil {
			dk = k
		}
	}
	if dk == nil {
		k, err := mlkem.GenerateKey768()
		if err != nil {
			return nil, err
		}
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, err
		}
		if err := os.WriteFile(path, k.Bytes(), 0o600); err != nil {
			return nil, err
		}
		dk = k
	}
	return newKEM(dk), nil
}

func newKEM(dk *mlkem.DecapsulationKey768) *KEM {
	ek := dk.EncapsulationKey().Bytes()
	sum := sha256.Sum256(ek)
	return &KEM{dk: dk, kid: hex.EncodeToString(sum[:8]), ek: ek, nonces: map[string]time.Time{}, now: time.Now}
}

// Issue returns the public key and a fresh single-use nonce.
func (k *KEM) Issue() KEMInfo {
	nonce := random(16)
	k.mu.Lock()
	defer k.mu.Unlock()
	now := k.now()
	if len(k.nonces) >= maxNonces {
		k.purgeLocked(now)
		if len(k.nonces) >= maxNonces {
			k.nonces = map[string]time.Time{} // under flood: drop old nonces rather than grow
		}
	}
	k.nonces[string(nonce)] = now.Add(nonceLifetime)
	return KEMInfo{KID: k.kid, EK: base64.StdEncoding.EncodeToString(k.ek), Nonce: base64.StdEncoding.EncodeToString(nonce)}
}

func (k *KEM) purgeLocked(now time.Time) {
	for n, exp := range k.nonces {
		if now.After(exp) {
			delete(k.nonces, n)
		}
	}
}

// consume marks a nonce as used; it reports false if it was unknown, expired or used.
func (k *KEM) consume(nonce []byte) bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	exp, ok := k.nonces[string(nonce)]
	if !ok {
		return false
	}
	delete(k.nonces, string(nonce))
	return !k.now().After(exp)
}

// Open authenticates and decrypts an envelope for the given route. Every
// failure returns the same error so callers cannot be used as an oracle.
func (k *KEM) Open(env Envelope, route string) ([]byte, error) {
	if env.KID != k.kid {
		return nil, errEnvelope
	}
	dec := base64.StdEncoding.DecodeString
	ct, e1 := dec(env.CT)
	nonce, e2 := dec(env.Nonce)
	iv, e3 := dec(env.IV)
	data, e4 := dec(env.Data)
	if e1 != nil || e2 != nil || e3 != nil || e4 != nil || len(ct) != mlkem.CiphertextSize768 || len(nonce) != 16 || len(iv) != 12 {
		return nil, errEnvelope
	}
	// Consume first: a bad tag still burns the nonce, so guessing is not free.
	if !k.consume(nonce) {
		return nil, errEnvelope
	}
	secret, err := k.dk.Decapsulate(ct)
	if err != nil {
		return nil, errEnvelope
	}
	key, err := hkdf.Key(sha256.New, secret, nonce, envelopeInfo+route, 32)
	if err != nil {
		return nil, errEnvelope
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, errEnvelope
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, errEnvelope
	}
	plain, err := gcm.Open(nil, iv, data, envelopeAAD(env.KID, route, nonce))
	if err != nil {
		return nil, errEnvelope
	}
	return plain, nil
}

func envelopeAAD(kid, route string, nonce []byte) []byte {
	aad := append([]byte(kid+"|"+route+"|"), nonce...)
	return aad
}
