// Package integrations is the wiring layer that composes per-project
// integrations into the hub. The public contract integrations implement
// lives in pkg/integrations/registry; this package owns secret-field
// encryption and public core integration composition.
package integrations

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
)

// ErrCipherUnavailable is returned by EncryptFields / DecryptFields when
// the integration declares secret fields but the hub was started without
// a configured cipher (e.g. integrations.secret_key absent from config).
// Callers translate it into a clear "configure integrations.secret_key"
// error at the API boundary.
var ErrCipherUnavailable = errors.New("integrations secret cipher unavailable")

// Cipher does AES-256-GCM encryption with a random per-encrypt nonce
// prepended to the ciphertext. Output is base64-stdandard-encoded so
// the entire blob fits inside the JSONB config column as a string
// value. Cipher is goroutine-safe: cipher.AEAD has no mutable state
// across Seal/Open calls.
type Cipher struct {
	aead cipher.AEAD
}

// NewCipher builds a Cipher from a 32-byte AES-256 key. The key may be
// supplied as hex (64 chars) or as base64-standard (44 chars). Any
// other length is rejected; the application key must be configured
// explicitly to a 32-byte value.
func NewCipher(key string) (*Cipher, error) {
	raw, err := decodeKey(key)
	if err != nil {
		return nil, fmt.Errorf("decode key: %w", err)
	}
	if len(raw) != 32 {
		return nil, fmt.Errorf("integrations cipher key must be 32 bytes (got %d)", len(raw))
	}
	block, err := aes.NewCipher(raw)
	if err != nil {
		return nil, fmt.Errorf("aes cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("aes-gcm: %w", err)
	}
	return &Cipher{aead: aead}, nil
}

func decodeKey(s string) ([]byte, error) {
	if raw, err := hex.DecodeString(s); err == nil && len(raw) == 32 {
		return raw, nil
	}
	return base64.StdEncoding.DecodeString(s)
}

// Encrypt seals plaintext under a fresh random nonce, returning
// base64-encoded (nonce || ciphertext || tag).
func (c *Cipher) Encrypt(plaintext []byte) (string, error) {
	if c == nil {
		return "", ErrCipherUnavailable
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("read nonce: %w", err)
	}
	sealed := c.aead.Seal(nonce, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt reverses Encrypt. Tampered or truncated input returns an
// error from cipher.AEAD.Open; the caller does not need to distinguish
// the variants.
func (c *Cipher) Decrypt(encoded string) ([]byte, error) {
	if c == nil {
		return nil, ErrCipherUnavailable
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decode ciphertext: %w", err)
	}
	ns := c.aead.NonceSize()
	if len(raw) < ns {
		return nil, fmt.Errorf("ciphertext too short")
	}
	nonce, body := raw[:ns], raw[ns:]
	pt, err := c.aead.Open(nil, nonce, body, nil)
	if err != nil {
		return nil, fmt.Errorf("aead open: %w", err)
	}
	return pt, nil
}

// EncryptFields walks the named keys of config and replaces each string
// value with its ciphertext. Non-string values and missing keys are
// left untouched. Returns ErrCipherUnavailable when fields is non-empty
// but c is nil — encrypting nothing through a nil cipher is allowed.
func EncryptFields(c *Cipher, config map[string]any, fields []string) error {
	if len(fields) == 0 {
		return nil
	}
	if c == nil {
		return ErrCipherUnavailable
	}
	for _, k := range fields {
		v, ok := config[k]
		if !ok {
			continue
		}
		s, ok := v.(string)
		if !ok || s == "" {
			continue
		}
		enc, err := c.Encrypt([]byte(s))
		if err != nil {
			return fmt.Errorf("encrypt field %q: %w", k, err)
		}
		config[k] = enc
	}
	return nil
}

// DecryptFields is the inverse of EncryptFields.
func DecryptFields(c *Cipher, config map[string]any, fields []string) error {
	if len(fields) == 0 {
		return nil
	}
	if c == nil {
		return ErrCipherUnavailable
	}
	for _, k := range fields {
		v, ok := config[k]
		if !ok {
			continue
		}
		s, ok := v.(string)
		if !ok || s == "" {
			continue
		}
		pt, err := c.Decrypt(s)
		if err != nil {
			return fmt.Errorf("decrypt field %q: %w", k, err)
		}
		config[k] = string(pt)
	}
	return nil
}

// RedactFields zeros the named secret fields in a config copy. Used
// when echoing the stored config back through the API so plaintext
// secrets never leave the server.
func RedactFields(config map[string]any, fields []string) {
	for _, k := range fields {
		if _, ok := config[k]; ok {
			config[k] = ""
		}
	}
}
