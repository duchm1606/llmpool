package oauth

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"
	"unicode"
)

const (
	// minStateLength is the minimum allowed state length (32 bytes -> 43 base64url chars)
	minStateLength = 32
	// maxStateLength is the maximum allowed state length
	maxStateLength = 256
	// stateRandomBytes is the number of random bytes to generate
	stateRandomBytes = 32
)

// GenerateState generates a cryptographically secure OAuth state parameter.
// Returns a base64url-encoded random string suitable for CSRF protection.
// The generated state is always valid per ValidateState rules.
func GenerateState() string {
	// Generate 32 random bytes (256 bits) of entropy
	randomBytes := make([]byte, stateRandomBytes)
	if _, err := rand.Read(randomBytes); err != nil {
		// If random generation fails, this is a critical error.
		// In production, this should panic or be handled at a higher level.
		panic(fmt.Sprintf("failed to generate random state: %v", err))
	}

	// Encode to base64url (RFC 4648) without padding
	// This produces a 43-character string from 32 bytes
	return base64.RawURLEncoding.EncodeToString(randomBytes)
}

// ValidateState validates an OAuth state parameter with strict security constraints.
// Validation rules:
//   - State must not be empty (after trimming whitespace)
//   - State must be between minStateLength (32) and maxStateLength (256) characters
//   - State must only contain alphanumeric characters plus: - _ .
//   - State must not contain path separators (/ \)
//   - State must not contain path traversal patterns (..)
//   - State must not contain control characters or unicode characters
//
// This function rejects dangerous patterns that could be used for:
//   - Path traversal attacks (../, ..\)
//   - Null byte injection
//   - Control character injection (newline, tab, etc.)
//   - Unicode normalization attacks
func ValidateState(state string) error {
	// Trim whitespace
	trimmed := strings.TrimSpace(state)

	// Check empty
	if trimmed == "" {
		return fmt.Errorf("invalid oauth state: empty")
	}

	// Check length bounds
	if len(trimmed) < minStateLength {
		return fmt.Errorf("invalid oauth state: too short (minimum %d characters)", minStateLength)
	}
	if len(trimmed) > maxStateLength {
		return fmt.Errorf("invalid oauth state: too long (maximum %d characters)", maxStateLength)
	}

	// Check for path separators
	if strings.Contains(trimmed, "/") {
		return fmt.Errorf("invalid oauth state: contains path separator /")
	}
	if strings.Contains(trimmed, "\\") {
		return fmt.Errorf("invalid oauth state: contains path separator \\")
	}

	// Check for path traversal patterns
	if strings.Contains(trimmed, "..") {
		return fmt.Errorf("invalid oauth state: contains path traversal pattern ..")
	}

	// Validate each character
	for i, r := range trimmed {
		switch {
		// Allow alphanumeric
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9'):
			// Valid
		// Allow safe symbols (-, _, .)
		case r == '-' || r == '_' || r == '.':
			// Valid
		// Reject control characters
		case unicode.IsControl(r):
			return fmt.Errorf("invalid oauth state: contains control character at position %d", i)
		// Reject non-ASCII/unicode
		case r > 127:
			return fmt.Errorf("invalid oauth state: contains unicode character at position %d", i)
		// Reject everything else
		default:
			return fmt.Errorf("invalid oauth state: contains invalid character %q at position %d", r, i)
		}
	}

	return nil
}
