package oauth

import (
	"strings"
	"testing"
)

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
			name:        "ends with ..",
			state:       strings.Repeat("a", 30) + "..",
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
			name:        "contains tab",
			state:       "abc\tdef" + strings.Repeat("a", 25),
			wantErrType: "control character",
		},
		{
			name:        "contains special char !",
			state:       "abc!def" + strings.Repeat("a", 25),
			wantErrType: "invalid character",
		},
		{
			name:        "contains special char @",
			state:       "abc@def" + strings.Repeat("a", 25),
			wantErrType: "invalid character",
		},
		{
			name:        "contains special char #",
			state:       "abc#def" + strings.Repeat("a", 25),
			wantErrType: "invalid character",
		},
		{
			name:        "contains special char $",
			state:       "abc$def" + strings.Repeat("a", 25),
			wantErrType: "invalid character",
		},
		{
			name:        "contains special char %",
			state:       "abc%def" + strings.Repeat("a", 25),
			wantErrType: "invalid character",
		},
		{
			name:        "contains special char ^",
			state:       "abc^def" + strings.Repeat("a", 25),
			wantErrType: "invalid character",
		},
		{
			name:        "contains special char &",
			state:       "abc&def" + strings.Repeat("a", 25),
			wantErrType: "invalid character",
		},
		{
			name:        "contains special char *",
			state:       "abc*def" + strings.Repeat("a", 25),
			wantErrType: "invalid character",
		},
		{
			name:        "contains special char +",
			state:       "abc+def" + strings.Repeat("a", 25),
			wantErrType: "invalid character",
		},
		{
			name:        "contains special char =",
			state:       "abc=def" + strings.Repeat("a", 25),
			wantErrType: "invalid character",
		},
		{
			name:        "contains unicode emoji",
			state:       "abc😀def" + strings.Repeat("a", 22),
			wantErrType: "unicode",
		},
		{
			name:        "contains non-ASCII character",
			state:       "abcédef" + strings.Repeat("a", 25),
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

func TestValidateState_TrimWhitespace(t *testing.T) {
	tests := []struct {
		name  string
		state string
	}{
		{
			name:  "leading whitespace",
			state: "   " + strings.Repeat("a", 32),
		},
		{
			name:  "trailing whitespace",
			state: strings.Repeat("a", 32) + "   ",
		},
		{
			name:  "both leading and trailing",
			state: "   " + strings.Repeat("a", 32) + "   ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateState(tt.state)
			if err != nil {
				t.Errorf("ValidateState(%q) should trim whitespace, got error: %v", tt.state, err)
			}
		})
	}
}

func BenchmarkGenerateState(b *testing.B) {
	for i := 0; i < b.N; i++ {
		GenerateState()
	}
}

func BenchmarkValidateState(b *testing.B) {
	state := GenerateState()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ValidateState(state)
	}
}
