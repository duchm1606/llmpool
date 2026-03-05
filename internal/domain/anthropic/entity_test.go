package anthropic

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNormalizeThinkingType(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "enabled stays enabled",
			input: "enabled",
			want:  "enabled",
		},
		{
			name:  "disabled stays disabled",
			input: "disabled",
			want:  "disabled",
		},
		{
			name:  "adaptive converts to enabled",
			input: "adaptive",
			want:  "enabled",
		},
		{
			name:  "unknown converts to disabled",
			input: "unknown",
			want:  "disabled",
		},
		{
			name:  "empty string converts to disabled",
			input: "",
			want:  "disabled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeThinkingType(tt.input)
			if got != tt.want {
				t.Errorf("NormalizeThinkingType(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestNormalizeThinkingConfig(t *testing.T) {
	t.Run("nil config returns nil", func(t *testing.T) {
		got := NormalizeThinkingConfig(nil)
		if got != nil {
			t.Errorf("NormalizeThinkingConfig(nil) = %v, want nil", got)
		}
	})

	t.Run("enabled without budget_tokens gets default", func(t *testing.T) {
		cfg := &ThinkingConfig{Type: "enabled"}
		got := NormalizeThinkingConfig(cfg)

		if got.Type != "enabled" {
			t.Errorf("Type = %q, want %q", got.Type, "enabled")
		}
		if got.BudgetTokens == nil {
			t.Error("BudgetTokens is nil, want non-nil")
		} else if *got.BudgetTokens != DefaultBudgetTokens {
			t.Errorf("BudgetTokens = %d, want %d", *got.BudgetTokens, DefaultBudgetTokens)
		}
	})

	t.Run("enabled with budget_tokens preserves value", func(t *testing.T) {
		budget := 8000
		cfg := &ThinkingConfig{Type: "enabled", BudgetTokens: &budget}
		got := NormalizeThinkingConfig(cfg)

		if got.Type != "enabled" {
			t.Errorf("Type = %q, want %q", got.Type, "enabled")
		}
		if got.BudgetTokens == nil {
			t.Error("BudgetTokens is nil, want non-nil")
		} else if *got.BudgetTokens != 8000 {
			t.Errorf("BudgetTokens = %d, want %d", *got.BudgetTokens, 8000)
		}
	})

	t.Run("adaptive converts to enabled and gets budget_tokens", func(t *testing.T) {
		cfg := &ThinkingConfig{Type: "adaptive"}
		got := NormalizeThinkingConfig(cfg)

		if got.Type != "enabled" {
			t.Errorf("Type = %q, want %q", got.Type, "enabled")
		}
		if got.BudgetTokens == nil {
			t.Error("BudgetTokens is nil, want non-nil")
		} else if *got.BudgetTokens != DefaultBudgetTokens {
			t.Errorf("BudgetTokens = %d, want %d", *got.BudgetTokens, DefaultBudgetTokens)
		}
	})

	t.Run("disabled does not add budget_tokens", func(t *testing.T) {
		cfg := &ThinkingConfig{Type: "disabled"}
		got := NormalizeThinkingConfig(cfg)

		if got.Type != "disabled" {
			t.Errorf("Type = %q, want %q", got.Type, "disabled")
		}
		if got.BudgetTokens != nil {
			t.Errorf("BudgetTokens = %d, want nil", *got.BudgetTokens)
		}
	})
}

func TestNewThinkingConfigEnabled(t *testing.T) {
	t.Run("zero budget uses default", func(t *testing.T) {
		got := NewThinkingConfigEnabled(0)

		if got.Type != "enabled" {
			t.Errorf("Type = %q, want %q", got.Type, "enabled")
		}
		if got.BudgetTokens == nil {
			t.Error("BudgetTokens is nil, want non-nil")
		} else if *got.BudgetTokens != DefaultBudgetTokens {
			t.Errorf("BudgetTokens = %d, want %d", *got.BudgetTokens, DefaultBudgetTokens)
		}
	})

	t.Run("negative budget uses default", func(t *testing.T) {
		got := NewThinkingConfigEnabled(-100)

		if got.BudgetTokens == nil {
			t.Error("BudgetTokens is nil, want non-nil")
		} else if *got.BudgetTokens != DefaultBudgetTokens {
			t.Errorf("BudgetTokens = %d, want %d", *got.BudgetTokens, DefaultBudgetTokens)
		}
	})

	t.Run("positive budget is used", func(t *testing.T) {
		got := NewThinkingConfigEnabled(32000)

		if got.Type != "enabled" {
			t.Errorf("Type = %q, want %q", got.Type, "enabled")
		}
		if got.BudgetTokens == nil {
			t.Error("BudgetTokens is nil, want non-nil")
		} else if *got.BudgetTokens != 32000 {
			t.Errorf("BudgetTokens = %d, want %d", *got.BudgetTokens, 32000)
		}
	})
}

// TestThinkingConfigJSONSerialization verifies the exact JSON output format
// that will be sent to the upstream /v1/messages endpoint.
func TestThinkingConfigJSONSerialization(t *testing.T) {
	t.Run("enabled with budget_tokens serializes correctly", func(t *testing.T) {
		cfg := NewThinkingConfigEnabled(16000)
		jsonBytes, err := json.Marshal(cfg)
		if err != nil {
			t.Fatalf("json.Marshal failed: %v", err)
		}

		jsonStr := string(jsonBytes)
		// Must contain type and budget_tokens
		if !strings.Contains(jsonStr, `"type":"enabled"`) {
			t.Errorf("JSON missing type:enabled, got: %s", jsonStr)
		}
		if !strings.Contains(jsonStr, `"budget_tokens":16000`) {
			t.Errorf("JSON missing budget_tokens:16000, got: %s", jsonStr)
		}

		// Verify exact structure
		expected := `{"type":"enabled","budget_tokens":16000}`
		if jsonStr != expected {
			t.Errorf("JSON mismatch\ngot:  %s\nwant: %s", jsonStr, expected)
		}
	})

	t.Run("disabled without budget_tokens serializes correctly", func(t *testing.T) {
		cfg := &ThinkingConfig{Type: "disabled"}
		jsonBytes, err := json.Marshal(cfg)
		if err != nil {
			t.Fatalf("json.Marshal failed: %v", err)
		}

		jsonStr := string(jsonBytes)
		// Should NOT contain budget_tokens
		if strings.Contains(jsonStr, "budget_tokens") {
			t.Errorf("JSON should not contain budget_tokens for disabled, got: %s", jsonStr)
		}

		expected := `{"type":"disabled"}`
		if jsonStr != expected {
			t.Errorf("JSON mismatch\ngot:  %s\nwant: %s", jsonStr, expected)
		}
	})

	t.Run("full request with thinking serializes correctly", func(t *testing.T) {
		budget := 16000
		req := MessagesRequest{
			Model:        "claude-sonnet-4.5",
			MaxTokens:    4096,
			Messages:     []Message{{Role: "user", Content: []ContentBlock{{Type: "text", Text: "Hello"}}}},
			Thinking:     &ThinkingConfig{Type: "enabled", BudgetTokens: &budget},
			OutputConfig: &OutputConfig{Effort: "medium"},
		}

		jsonBytes, err := json.Marshal(req)
		if err != nil {
			t.Fatalf("json.Marshal failed: %v", err)
		}

		jsonStr := string(jsonBytes)
		// Verify thinking block structure
		if !strings.Contains(jsonStr, `"thinking":{"type":"enabled","budget_tokens":16000}`) {
			t.Errorf("Request JSON missing correct thinking block, got: %s", jsonStr)
		}
		// Verify output_config block
		if !strings.Contains(jsonStr, `"output_config":{"effort":"medium"}`) {
			t.Errorf("Request JSON missing correct output_config block, got: %s", jsonStr)
		}
	})
}
