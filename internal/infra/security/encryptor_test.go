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
	sealed, err := enc.Encrypt(input)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	plain, err := enc.Decrypt(sealed)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}

	if plain != input {
		t.Fatalf("expected %q, got %q", input, plain)
	}
}
