package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
)

type AesGCMEncryptor struct {
	aead cipher.AEAD
}

func NewAesGCMEncryptor(key string) (*AesGCMEncryptor, error) {
	decoded, err := base64.StdEncoding.DecodeString(key)
	if err != nil {
		return nil, fmt.Errorf("decode encryption key: %w", err)
	}

	if len(decoded) != 32 {
		return nil, fmt.Errorf("encryption key must be 32 bytes after base64 decode")
	}

	block, err := aes.NewCipher(decoded)
	if err != nil {
		return nil, fmt.Errorf("create aes cipher: %w", err)
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create gcm: %w", err)
	}

	return &AesGCMEncryptor{aead: aead}, nil
}

func (a *AesGCMEncryptor) Encrypt(plain string) (string, error) {
	if plain == "" {
		return "", nil
	}

	nonce := make([]byte, a.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}

	ciphertext := a.aead.Seal(nil, nonce, []byte(plain), nil)
	payload := append(nonce, ciphertext...)

	return base64.StdEncoding.EncodeToString(payload), nil
}

func (a *AesGCMEncryptor) Decrypt(value string) (string, error) {
	if value == "" {
		return "", nil
	}

	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return "", fmt.Errorf("decode encrypted payload: %w", err)
	}

	nonceSize := a.aead.NonceSize()
	if len(decoded) <= nonceSize {
		return "", fmt.Errorf("encrypted payload too short")
	}

	nonce := decoded[:nonceSize]
	ciphertext := decoded[nonceSize:]

	plain, err := a.aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt payload: %w", err)
	}

	return string(plain), nil
}
