package llm

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
)

// Secrets at rest: provider API keys are the only sensitive fields in the
// llm_settings blob. When KUBONA_SECRET_KEY is configured (32-byte hex or
// base64), Save seals each key with AES-256-GCM; Load opens it. Without the
// variable, legacy plaintext remains readable (and a warning is logged) so
// an existing dev database is not bricked; production should set the key.
const sealedPrefix = "enc:v1:"

func secretKey() ([]byte, error) {
	raw := strings.TrimSpace(os.Getenv("KUBONA_SECRET_KEY"))
	if raw == "" {
		return nil, nil
	}
	if b, err := hex.DecodeString(raw); err == nil && len(b) == 32 {
		return b, nil
	}
	if b, err := base64.StdEncoding.DecodeString(raw); err == nil && len(b) == 32 {
		return b, nil
	}
	if b, err := base64.RawStdEncoding.DecodeString(raw); err == nil && len(b) == 32 {
		return b, nil
	}
	return nil, errors.New("KUBONA_SECRET_KEY must be 32 bytes encoded as hex or base64")
}

func sealSecret(plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	key, err := secretKey()
	if err != nil {
		return "", err
	}
	if key == nil {
		return plain, nil
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	out := gcm.Seal(nonce, nonce, []byte(plain), nil)
	return sealedPrefix + base64.RawStdEncoding.EncodeToString(out), nil
}

func openSecret(stored string) (string, error) {
	if !strings.HasPrefix(stored, sealedPrefix) {
		return stored, nil
	}
	key, err := secretKey()
	if err != nil {
		return "", err
	}
	if key == nil {
		return "", errors.New("encrypted LLM secret found but KUBONA_SECRET_KEY is unset")
	}
	raw, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(stored, sealedPrefix))
	if err != nil {
		return "", fmt.Errorf("invalid sealed secret encoding: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("sealed secret is truncated")
	}
	nonce, ciphertext := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("sealed secret authentication failed: %w", err)
	}
	return string(plain), nil
}
