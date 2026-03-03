// Package oauth provides OAuth2 and PKCE utilities for authentication flows.
package oauth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

// GenerateVerifier creates a cryptographically secure random PKCE code verifier.
// The verifier is a high-entropy string (43-128 characters) used to prove possession
// of the client that initiated the authorization request, as specified in RFC 7636.
//
// The verifier uses URL-safe base64 encoding without padding and contains only
// characters from the unreserved character set: [A-Za-z0-9\-._~]
//
// Returns:
//   - verifier: The generated code verifier (43-128 characters)
//   - error: Non-nil if random generation fails
func GenerateVerifier() (string, error) {
	// Generate 96 random bytes which will encode to 128 base64 characters (96 * 4/3 = 128)
	// This maximizes entropy within RFC 7636 bounds
	bytes := make([]byte, 96)
	_, err := rand.Read(bytes)
	if err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}

	// Encode to URL-safe base64 without padding
	verifier := base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(bytes)
	return verifier, nil
}

// GenerateChallenge derives a PKCE code challenge from a given code verifier using
// the S256 method (SHA-256). The challenge is computed by taking the SHA256 hash of
// the verifier and then Base64 URL-encoding the result without padding.
//
// This function is deterministic: the same verifier will always produce the same challenge.
// It is safe to call with empty strings or any arbitrary input.
//
// Parameters:
//   - verifier: The PKCE code verifier (typically from GenerateVerifier)
//
// Returns:
//   - challenge: The S256 code challenge (43 characters, base64url encoded)
func GenerateChallenge(verifier string) string {
	// Compute SHA256 hash of the verifier
	hash := sha256.Sum256([]byte(verifier))

	// Encode hash to URL-safe base64 without padding
	challenge := base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(hash[:])
	return challenge
}
