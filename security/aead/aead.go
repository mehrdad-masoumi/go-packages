// Package aead provides versioned AES-GCM envelope encryption for secrets at rest.
package aead

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	prefixV1 = "enc:v1:"
	keySize  = 32
)

var (
	ErrMissingKey     = errors.New("aead: encryption key is required")
	ErrInvalidKey     = errors.New("aead: encryption key must be 32 bytes (hex or base64)")
	ErrInvalidPayload = errors.New("aead: invalid ciphertext")
)

// Crypter is a versioned AES-GCM envelope. Existing plaintext values are left
// untouched on Decrypt so operators can introduce encryption without a silent
// rewrite of historical rows.
type Crypter struct {
	key []byte
}

func New(keyMaterial string) (*Crypter, error) {
	key, err := parseKey(keyMaterial)
	if err != nil {
		return nil, err
	}
	return &Crypter{key: key}, nil
}

func parseKey(raw string) ([]byte, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, ErrMissingKey
	}
	if decoded, err := hex.DecodeString(raw); err == nil && len(decoded) == keySize {
		return decoded, nil
	}
	if decoded, err := base64.StdEncoding.DecodeString(raw); err == nil && len(decoded) == keySize {
		return decoded, nil
	}
	if decoded, err := base64.RawStdEncoding.DecodeString(raw); err == nil && len(decoded) == keySize {
		return decoded, nil
	}
	if len(raw) == keySize {
		return []byte(raw), nil
	}
	return nil, ErrInvalidKey
}

func (c *Crypter) Encrypt(plaintext string) (string, error) {
	if c == nil || len(c.key) != keySize {
		return "", ErrMissingKey
	}
	block, err := aes.NewCipher(c.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return prefixV1 + base64.RawURLEncoding.EncodeToString(sealed), nil
}

// Decrypt returns plaintext. Values without the versioned prefix are treated
// as legacy plaintext and returned unchanged.
func (c *Crypter) Decrypt(value string) (string, error) {
	if !IsEnvelope(value) {
		return value, nil
	}
	if c == nil || len(c.key) != keySize {
		return "", ErrMissingKey
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(value, prefixV1))
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidPayload, err)
	}
	block, err := aes.NewCipher(c.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	ns := gcm.NonceSize()
	if len(raw) < ns {
		return "", ErrInvalidPayload
	}
	plain, err := gcm.Open(nil, raw[:ns], raw[ns:], nil)
	if err != nil {
		return "", ErrInvalidPayload
	}
	return string(plain), nil
}

func IsEnvelope(value string) bool {
	return strings.HasPrefix(value, prefixV1)
}

func (c *Crypter) HMACSHA256(msg string) (string, error) {
	if c == nil || len(c.key) != keySize {
		return "", ErrMissingKey
	}
	mac := hmac.New(sha256.New, c.key)
	_, _ = mac.Write([]byte(msg))
	return hex.EncodeToString(mac.Sum(nil)), nil
}

func (c *Crypter) HMACEqual(msg, digestHex string) (bool, error) {
	want, err := c.HMACSHA256(msg)
	if err != nil {
		return false, err
	}
	return hmac.Equal([]byte(want), []byte(strings.TrimSpace(digestHex))), nil
}
