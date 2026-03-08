package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
)

type Encryptor interface {
	Encrypt(plain string) (string, string, string, error)
	Decrypt(cipher, iv, tag string) (string, error)
	ShouldEncrypt() bool
}

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

func (a *AesGCMEncryptor) Encrypt(plain string) (string, string, string, error) {
	if plain == "" {
		return "", "", "", nil
	}

	nonce := make([]byte, a.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", "", "", fmt.Errorf("generate nonce: %w", err)
	}

	sealed := a.aead.Seal(nil, nonce, []byte(plain), nil)
	tagSize := a.aead.Overhead()
	if len(sealed) < tagSize {
		return "", "", "", fmt.Errorf("encrypted payload too short")
	}

	ciphertext := sealed[:len(sealed)-tagSize]
	tag := sealed[len(sealed)-tagSize:]

	return base64.StdEncoding.EncodeToString(ciphertext),
		base64.StdEncoding.EncodeToString(nonce),
		base64.StdEncoding.EncodeToString(tag),
		nil
}

func (a *AesGCMEncryptor) Decrypt(value, iv, tag string) (string, error) {
	if value == "" {
		return "", nil
	}
	if iv == "" || tag == "" {
		return "", fmt.Errorf("missing iv or tag")
	}

	decodedCipher, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return "", fmt.Errorf("decode encrypted payload: %w", err)
	}

	decodedIV, err := base64.StdEncoding.DecodeString(iv)
	if err != nil {
		return "", fmt.Errorf("decode iv: %w", err)
	}
	if len(decodedIV) != a.aead.NonceSize() {
		return "", fmt.Errorf("invalid iv size")
	}

	decodedTag, err := base64.StdEncoding.DecodeString(tag)
	if err != nil {
		return "", fmt.Errorf("decode tag: %w", err)
	}
	if len(decodedTag) != a.aead.Overhead() {
		return "", fmt.Errorf("invalid tag size")
	}

	sealed := make([]byte, 0, len(decodedCipher)+len(decodedTag))
	sealed = append(sealed, decodedCipher...)
	sealed = append(sealed, decodedTag...)

	plain, err := a.aead.Open(nil, decodedIV, sealed, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt payload: %w", err)
	}

	return string(plain), nil
}

func (a *AesGCMEncryptor) ShouldEncrypt() bool {
	return true
}

type NoopEncryptor struct{}

func NewNoopEncryptor() *NoopEncryptor {
	return &NoopEncryptor{}
}

func (n *NoopEncryptor) Encrypt(plain string) (string, string, string, error) {
	return plain, "", "", nil
}

func (n *NoopEncryptor) Decrypt(cipher, iv, tag string) (string, error) {
	return cipher, nil
}

func (n *NoopEncryptor) ShouldEncrypt() bool {
	return false
}
