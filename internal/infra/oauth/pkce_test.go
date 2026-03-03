package oauth

import (
	"crypto/sha256"
	"encoding/base64"
	"regexp"
	"strings"
	"testing"
)

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

func TestGenerateState(t *testing.T) {
	tests := []struct {
		name      string
		wantLen   int
		wantValid bool
	}{
		{
			name:      "happy path - generates valid state",
			wantLen:   43, // base64url encoding of 32 bytes (no padding)
			wantValid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := GenerateState()

			// Check length
			if len(state) != tt.wantLen {
				t.Errorf("GenerateState() length = %d, want %d", len(state), tt.wantLen)
			}

			// Check it's valid
			if err := ValidateState(state); err != nil {
				t.Errorf("GenerateState() produced invalid state: %v", err)
			}

			// Check randomness (two calls should produce different states)
			state2 := GenerateState()
			if state == state2 {
				t.Error("GenerateState() produced identical states (not random)")
			}
		})
	}
}

func TestValidateState_ValidCases(t *testing.T) {
	tests := []struct {
		name  string
		state string
	}{
		{
			name:  "alphanumeric",
			state: "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKL",
		},
		{
			name:  "with hyphen",
			state: "abc-def-ghi-jkl-mno-pqr-stu-vwx-yz0123",
		},
		{
			name:  "with underscore",
			state: "abc_def_ghi_jkl_mno_pqr_stu_vwx_yz0123",
		},
		{
			name:  "with dot",
			state: "abc.def.ghi.jkl.mno.pqr.stu.vwx.yz0123",
		},
		{
			name:  "min length (32 chars)",
			state: "a" + strings.Repeat("b", 31),
		},
		{
			name:  "max length (256 chars)",
			state: strings.Repeat("a", 256),
		},
		{
			name:  "mixed safe characters",
			state: "abc-123_def.456_ghi-789abcdefghijklmnop",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateState(tt.state)
			if err != nil {
				t.Errorf("ValidateState(%q) = %v, want nil", tt.state, err)
			}
		})
	}
}

func TestValidateState_InvalidCases(t *testing.T) {
	tests := []struct {
		name        string
		state       string
		wantErrType string
	}{
		{
			name:        "empty string",
			state:       "",
			wantErrType: "empty",
		},
		{
			name:        "whitespace only",
			state:       "   ",
			wantErrType: "empty",
		},
		{
			name:        "too short (31 chars)",
			state:       strings.Repeat("a", 31),
			wantErrType: "too short",
		},
		{
			name:        "too long (257 chars)",
			state:       strings.Repeat("a", 257),
			wantErrType: "too long",
		},
		{
			name:        "contains forward slash",
			state:       "abc/def" + strings.Repeat("a", 25),
			wantErrType: "path separator",
		},
		{
			name:        "contains backslash",
			state:       "abc\\def" + strings.Repeat("a", 25),
			wantErrType: "path separator",
		},
		{
			name:        "contains path traversal ..",
			state:       "abc..def" + strings.Repeat("a", 24),
			wantErrType: "path traversal",
		},
		{
			name:        "contains space",
			state:       "abc def" + strings.Repeat("a", 25),
			wantErrType: "invalid character",
		},
		{
			name:        "contains control character (newline)",
			state:       "abc\ndef" + strings.Repeat("a", 25),
			wantErrType: "control character",
		},
		{
			name:        "contains special char !",
			state:       "abc!def" + strings.Repeat("a", 25),
			wantErrType: "invalid character",
		},
		{
			name:        "contains unicode emoji",
			state:       "abc😀def" + strings.Repeat("a", 22),
			wantErrType: "unicode",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateState(tt.state)
			if err == nil {
				t.Errorf("ValidateState(%q) = nil, want error", tt.state)
			}
			if err != nil && !strings.Contains(err.Error(), tt.wantErrType) {
				t.Errorf("ValidateState(%q) error = %v, want to contain %q", tt.state, err, tt.wantErrType)
			}
		})
	}
}

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
