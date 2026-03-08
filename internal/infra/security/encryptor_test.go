package security

import (
	"encoding/base64"
	"testing"
)

func TestAesGCMEncryptor_RoundTrip(t *testing.T) {
	key := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))

	enc, err := NewAesGCMEncryptor(key)
	if err != nil {
		t.Fatalf("new encryptor: %v", err)
	}

	input := "secret-token"
	sealed, iv, tag, err := enc.Encrypt(input)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	plain, err := enc.Decrypt(sealed, iv, tag)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}

	if plain != input {
		t.Fatalf("expected %q, got %q", input, plain)
	}
}

func TestNoopEncryptor_RoundTrip(t *testing.T) {
	enc := NewNoopEncryptor()
	input := "secret-token"

	cipher, iv, tag, err := enc.Encrypt(input)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if iv != "" || tag != "" {
		t.Fatalf("expected empty iv/tag for noop encryptor")
	}

	plain, err := enc.Decrypt(cipher, iv, tag)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}

	if plain != input {
		t.Fatalf("expected %q, got %q", input, plain)
	}
}
