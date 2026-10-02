// Package auth implements accounts: argon2id passwords, email encrypted at rest,
// an ML-KEM-768 request envelope, an ALTCHA proof-of-work captcha and sessions.
package auth

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Keys are the symmetric secrets the service needs. Each is 32 bytes.
type Keys struct {
	Enc    []byte // AES-256-GCM key for emails at rest
	Index  []byte // HMAC key for the email blind index
	Pepper []byte // HMAC key mixed into passwords before argon2id
	Altcha []byte // HMAC key signing captcha challenges
}

type keyFile struct {
	Enc    string `json:"enc"`
	Index  string `json:"index"`
	Pepper string `json:"pepper"`
	Altcha string `json:"altcha"`
}

var keyEnv = [4]string{"ACCOUNTS_ENC_KEY", "ACCOUNTS_INDEX_KEY", "ACCOUNTS_PEPPER", "ALTCHA_HMAC_KEY"}

// LoadKeys reads the keys from the environment. A missing key is a startup
// error in production; in development the keys are generated once and kept in
// dir/keys.json (mode 0600) so restarts keep existing accounts readable.
func LoadKeys(dir string, production bool, getenv func(string) string) (*Keys, error) {
	vals := make([][]byte, len(keyEnv))
	missing := false
	for i, name := range keyEnv {
		raw := getenv(name)
		if raw == "" {
			missing = true
			continue
		}
		b, err := base64.StdEncoding.DecodeString(raw)
		if err != nil || len(b) != 32 {
			return nil, fmt.Errorf("%s must be 32 bytes, base64 encoded", name)
		}
		vals[i] = b
	}
	if !missing {
		return &Keys{Enc: vals[0], Index: vals[1], Pepper: vals[2], Altcha: vals[3]}, nil
	}
	if production {
		return nil, errors.New("accounts: set ACCOUNTS_ENC_KEY, ACCOUNTS_INDEX_KEY, ACCOUNTS_PEPPER and ALTCHA_HMAC_KEY (32 bytes, base64) in production")
	}
	path := filepath.Join(dir, "keys.json")
	if data, err := os.ReadFile(path); err == nil {
		var kf keyFile
		if json.Unmarshal(data, &kf) == nil {
			dec := func(s string) []byte { b, _ := base64.StdEncoding.DecodeString(s); return b }
			k := &Keys{Enc: dec(kf.Enc), Index: dec(kf.Index), Pepper: dec(kf.Pepper), Altcha: dec(kf.Altcha)}
			if len(k.Enc) == 32 && len(k.Index) == 32 && len(k.Pepper) == 32 && len(k.Altcha) == 32 {
				return k, nil
			}
		}
	}
	k := &Keys{Enc: random(32), Index: random(32), Pepper: random(32), Altcha: random(32)}
	enc := base64.StdEncoding.EncodeToString
	data, _ := json.Marshal(keyFile{Enc: enc(k.Enc), Index: enc(k.Index), Pepper: enc(k.Pepper), Altcha: enc(k.Altcha)})
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return nil, err
	}
	return k, nil
}

func random(n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err) // the system CSPRNG failing is unrecoverable
	}
	return b
}
