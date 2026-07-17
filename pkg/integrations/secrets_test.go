package integrations_test

import (
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"github.com/pletka-io/pletka/pkg/integrations"
	"github.com/google/go-cmp/cmp"
)

const testKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func newCipher(t *testing.T) *integrations.Cipher {
	t.Helper()
	c, err := integrations.NewCipher(testKey)
	if err != nil {
		t.Fatalf("NewCipher: %v", err)
	}
	return c
}

func TestNewCipher_RejectsShortKey(t *testing.T) {
	_, err := integrations.NewCipher("deadbeef")
	if err == nil {
		t.Fatalf("want error for short key, got nil")
	}
}

func TestNewCipher_RejectsInvalidEncoding(t *testing.T) {
	_, err := integrations.NewCipher("not-hex-not-base64-!!")
	if err == nil {
		t.Fatalf("want decode error, got nil")
	}
}

func TestEncryptDecrypt_RoundTrip(t *testing.T) {
	c := newCipher(t)
	plaintext := []byte("password123")
	enc, err := c.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if enc == "" || strings.Contains(enc, string(plaintext)) {
		t.Fatalf("ciphertext leaks plaintext: %q", enc)
	}
	got, err := c.Decrypt(enc)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if diff := cmp.Diff(plaintext, got); diff != "" {
		t.Fatalf("plaintext mismatch (-want +got):\n%s", diff)
	}
}

func TestEncrypt_RandomNonce(t *testing.T) {
	c := newCipher(t)
	a, _ := c.Encrypt([]byte("same"))
	b, _ := c.Encrypt([]byte("same"))
	if a == b {
		t.Fatalf("expected different ciphertexts for same plaintext (random nonce); both = %q", a)
	}
}

func TestDecrypt_WrongKey(t *testing.T) {
	a := newCipher(t)
	enc, err := a.Encrypt([]byte("password"))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	otherKey := make([]byte, 32)
	for i := range otherKey {
		otherKey[i] = 0xAA
	}
	b, err := integrations.NewCipher(hex.EncodeToString(otherKey))
	if err != nil {
		t.Fatalf("other cipher: %v", err)
	}
	if _, err := b.Decrypt(enc); err == nil {
		t.Fatalf("want decrypt failure with wrong key, got nil")
	}
}

func TestDecrypt_TamperedCiphertext(t *testing.T) {
	c := newCipher(t)
	enc, _ := c.Encrypt([]byte("hello"))
	// Flip a byte (decode → mutate → re-encode is overkill; mutating
	// the base64 string still produces garbage AEAD input).
	tampered := "AAAA" + enc[4:]
	if _, err := c.Decrypt(tampered); err == nil {
		t.Fatalf("want auth failure on tamper, got nil")
	}
}

func TestEncryptFields_OnlyTouchesNamedKeys(t *testing.T) {
	c := newCipher(t)
	cfg := map[string]any{
		"username": "alice",
		"password": "secret",
		"timeout":  30,
		"empty":    "",
		"absent":   nil,
	}
	if err := integrations.EncryptFields(c, cfg, []string{"password", "empty", "absent", "missing"}); err != nil {
		t.Fatalf("EncryptFields: %v", err)
	}
	if cfg["username"] != "alice" {
		t.Fatalf("non-secret field mutated: %v", cfg["username"])
	}
	if cfg["timeout"] != 30 {
		t.Fatalf("non-string secret field mutated: %v", cfg["timeout"])
	}
	if cfg["empty"] != "" {
		t.Fatalf("empty secret value mutated: %v", cfg["empty"])
	}
	enc, ok := cfg["password"].(string)
	if !ok || enc == "secret" {
		t.Fatalf("password not encrypted: %v", cfg["password"])
	}
	if err := integrations.DecryptFields(c, cfg, []string{"password"}); err != nil {
		t.Fatalf("DecryptFields: %v", err)
	}
	if cfg["password"] != "secret" {
		t.Fatalf("decrypt round-trip failed: %v", cfg["password"])
	}
}

func TestEncryptFields_NilCipherWithSecrets(t *testing.T) {
	cfg := map[string]any{"password": "secret"}
	err := integrations.EncryptFields(nil, cfg, []string{"password"})
	if !errors.Is(err, integrations.ErrCipherUnavailable) {
		t.Fatalf("want ErrCipherUnavailable, got %v", err)
	}
}

func TestEncryptFields_NilCipherNoSecrets(t *testing.T) {
	cfg := map[string]any{"username": "alice"}
	if err := integrations.EncryptFields(nil, cfg, nil); err != nil {
		t.Fatalf("nil cipher with no secret fields should be allowed, got: %v", err)
	}
}

func TestRedactFields_ZerosNamedKeys(t *testing.T) {
	cfg := map[string]any{
		"username": "alice",
		"password": "ciphertext-blob",
	}
	integrations.RedactFields(cfg, []string{"password"})
	if cfg["password"] != "" {
		t.Fatalf("password not redacted: %v", cfg["password"])
	}
	if cfg["username"] != "alice" {
		t.Fatalf("non-secret field touched: %v", cfg["username"])
	}
}
