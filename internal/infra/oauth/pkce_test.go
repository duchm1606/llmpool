// Package oauth provides OAuth2 and PKCE utilities for authentication flows.
package oauth

import (
	"crypto/sha256"
	"encoding/base64"
	"regexp"
	"strings"
	"testing"
)

// TestGenerateVerifier tests the verifier generation with happy path and constraints.
func TestGenerateVerifier(t *testing.T) {
	tests := []struct {
		name   string
		testFn func(*testing.T)
	}{
		{
			name: "generates non-empty verifier",
			testFn: func(t *testing.T) {
				verifier, err := GenerateVerifier()
				if err != nil {
					t.Fatalf("GenerateVerifier() error = %v, want nil", err)
				}
				if verifier == "" {
					t.Error("GenerateVerifier() returned empty string")
				}
			},
		},
		{
			name: "verifier length is within RFC 7636 bounds (43-128)",
			testFn: func(t *testing.T) {
				verifier, err := GenerateVerifier()
				if err != nil {
					t.Fatalf("GenerateVerifier() error = %v, want nil", err)
				}
				if len(verifier) < 43 || len(verifier) > 128 {
					t.Errorf("GenerateVerifier() length = %d, want between 43-128", len(verifier))
				}
			},
		},
		{
			name: "verifier contains only valid RFC 7636 characters",
			testFn: func(t *testing.T) {
				verifier, err := GenerateVerifier()
				if err != nil {
					t.Fatalf("GenerateVerifier() error = %v, want nil", err)
				}
				// RFC 7636: [A-Z] / [a-z] / [0-9] / "-" / "." / "_" / "~"
				pattern := regexp.MustCompile(`^[A-Za-z0-9\-._~]+$`)
				if !pattern.MatchString(verifier) {
					t.Errorf("GenerateVerifier() contains invalid characters: %s", verifier)
				}
			},
		},
		{
			name: "multiple verifiers are unique",
			testFn: func(t *testing.T) {
				verifiers := make(map[string]bool)
				for i := 0; i < 10; i++ {
					verifier, err := GenerateVerifier()
					if err != nil {
						t.Fatalf("GenerateVerifier() error = %v, want nil", err)
					}
					if verifiers[verifier] {
						t.Errorf("GenerateVerifier() generated duplicate: %s", verifier)
					}
					verifiers[verifier] = true
				}
			},
		},
		{
			name: "verifier is base64url encoded (no padding)",
			testFn: func(t *testing.T) {
				verifier, err := GenerateVerifier()
				if err != nil {
					t.Fatalf("GenerateVerifier() error = %v, want nil", err)
				}
				// Should not contain padding
				if strings.Contains(verifier, "=") {
					t.Errorf("GenerateVerifier() contains padding: %s", verifier)
				}
				// Should be decodable as base64url
				_, err = base64.URLEncoding.WithPadding(base64.NoPadding).DecodeString(verifier)
				if err != nil {
					t.Errorf("GenerateVerifier() not valid base64url: %v", err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.testFn)
	}
}

// TestGenerateChallenge tests challenge generation from verifier.
func TestGenerateChallenge(t *testing.T) {
	tests := []struct {
		name   string
		testFn func(*testing.T)
	}{
		{
			name: "challenge is non-empty",
			testFn: func(t *testing.T) {
				verifier, err := GenerateVerifier()
				if err != nil {
					t.Fatalf("GenerateVerifier() error = %v, want nil", err)
				}
				challenge := GenerateChallenge(verifier)
				if challenge == "" {
					t.Error("GenerateChallenge() returned empty string")
				}
			},
		},
		{
			name: "same verifier generates same challenge (deterministic)",
			testFn: func(t *testing.T) {
				testVerifier := "E9Mrozoa2owUednlCZ_lXvJwV2ChwuCHofo37Zv7h8~"
				challenge1 := GenerateChallenge(testVerifier)
				challenge2 := GenerateChallenge(testVerifier)
				if challenge1 != challenge2 {
					t.Errorf("GenerateChallenge() not deterministic: got %s and %s", challenge1, challenge2)
				}
			},
		},
		{
			name: "challenge is base64url encoded (no padding)",
			testFn: func(t *testing.T) {
				verifier, err := GenerateVerifier()
				if err != nil {
					t.Fatalf("GenerateVerifier() error = %v, want nil", err)
				}
				challenge := GenerateChallenge(verifier)
				// Should not contain padding
				if strings.Contains(challenge, "=") {
					t.Errorf("GenerateChallenge() contains padding: %s", challenge)
				}
				// Should be decodable as base64url
				_, err = base64.URLEncoding.WithPadding(base64.NoPadding).DecodeString(challenge)
				if err != nil {
					t.Errorf("GenerateChallenge() not valid base64url: %v", err)
				}
			},
		},
		{
			name: "challenge is SHA256 hash of verifier",
			testFn: func(t *testing.T) {
				testVerifier := "E9Mrozoa2owUednlCZ_lXvJwV2ChwuCHofo37Zv7h8"
				challenge := GenerateChallenge(testVerifier)
				// Manually compute expected challenge
				hash := sha256.Sum256([]byte(testVerifier))
				expected := base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(hash[:])
				if challenge != expected {
					t.Errorf("GenerateChallenge() = %s, want %s", challenge, expected)
				}
			},
		},
		{
			name: "different verifiers generate different challenges",
			testFn: func(t *testing.T) {
				verifier1, _ := GenerateVerifier()
				verifier2, _ := GenerateVerifier()
				challenge1 := GenerateChallenge(verifier1)
				challenge2 := GenerateChallenge(verifier2)
				if challenge1 == challenge2 {
					t.Error("GenerateChallenge() generated same challenge for different verifiers")
				}
			},
		},
		{
			name: "challenge length is 43 characters (SHA256 base64url)",
			testFn: func(t *testing.T) {
				verifier, _ := GenerateVerifier()
				challenge := GenerateChallenge(verifier)
				// SHA256 hash (32 bytes) encoded in base64url without padding = 43 chars
				if len(challenge) != 43 {
					t.Errorf("GenerateChallenge() length = %d, want 43", len(challenge))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.testFn)
	}
}

// TestPKCEFlow tests the complete PKCE flow.
func TestPKCEFlow(t *testing.T) {
	t.Run("complete PKCE flow", func(t *testing.T) {
		// Step 1: Client generates verifier
		verifier, err := GenerateVerifier()
		if err != nil {
			t.Fatalf("GenerateVerifier() error = %v, want nil", err)
		}

		// Step 2: Client generates challenge
		challenge := GenerateChallenge(verifier)

		// Verify both are non-empty
		if verifier == "" {
			t.Error("verifier is empty")
		}
		if challenge == "" {
			t.Error("challenge is empty")
		}

		// Verify constraints
		if len(verifier) < 43 || len(verifier) > 128 {
			t.Errorf("verifier length = %d, want 43-128", len(verifier))
		}
		if len(challenge) != 43 {
			t.Errorf("challenge length = %d, want 43", len(challenge))
		}

		// Verify challenge matches manual computation
		hash := sha256.Sum256([]byte(verifier))
		expectedChallenge := base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(hash[:])
		if challenge != expectedChallenge {
			t.Errorf("challenge = %s, want %s", challenge, expectedChallenge)
		}
	})
}

// TestMalformedInputs tests edge cases and malformed inputs.
func TestMalformedInputs(t *testing.T) {
	tests := []struct {
		name     string
		verifier string
		testFn   func(*testing.T, string)
	}{
		{
			name:     "empty verifier",
			verifier: "",
			testFn: func(t *testing.T, verifier string) {
				challenge := GenerateChallenge(verifier)
				if challenge == "" {
					t.Error("GenerateChallenge() should not return empty for empty verifier")
				}
				// Should still compute valid challenge for empty input
				hash := sha256.Sum256([]byte(verifier))
				expected := base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(hash[:])
				if challenge != expected {
					t.Errorf("challenge = %s, want %s", challenge, expected)
				}
			},
		},
		{
			name:     "verifier with special characters",
			verifier: "ABC123-._~",
			testFn: func(t *testing.T, verifier string) {
				challenge := GenerateChallenge(verifier)
				// Should process without error
				hash := sha256.Sum256([]byte(verifier))
				expected := base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(hash[:])
				if challenge != expected {
					t.Errorf("challenge = %s, want %s", challenge, expected)
				}
			},
		},
		{
			name:     "long verifier",
			verifier: strings.Repeat("a", 128),
			testFn: func(t *testing.T, verifier string) {
				challenge := GenerateChallenge(verifier)
				hash := sha256.Sum256([]byte(verifier))
				expected := base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(hash[:])
				if challenge != expected {
					t.Errorf("challenge = %s, want %s", challenge, expected)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.testFn(t, tt.verifier)
		})
	}
}
