package provider

import (
	"testing"
)

func TestProviderModels(t *testing.T) {
	tests := []struct {
		name         string
		providerType string
		wantModels   bool
	}{
		{
			name:         "codex provider has models",
			providerType: "codex",
			wantModels:   true,
		},
		{
			name:         "copilot provider has models",
			providerType: "copilot",
			wantModels:   true,
		},
		{
			name:         "openai provider has models",
			providerType: "openai",
			wantModels:   true,
		},
		{
			name:         "anthropic provider has models",
			providerType: "anthropic",
			wantModels:   true,
		},
		{
			name:         "unknown provider has no models",
			providerType: "unknown",
			wantModels:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			models := GetModelsForProvider(tt.providerType)
			hasModels := len(models) > 0
			if hasModels != tt.wantModels {
				t.Errorf("GetModelsForProvider(%s) returned %d models, wantModels=%v", tt.providerType, len(models), tt.wantModels)
			}
		})
	}
}

func TestCodexModelsIncludeGPT5(t *testing.T) {
	models := GetModelsForProvider("codex")
	found := false
	for _, m := range models {
		if m == "gpt-5" {
			found = true
			break
		}
	}
	if !found {
		t.Error("codex provider should include gpt-5 model")
	}
}

func TestCodexModelsIncludeGPT4o(t *testing.T) {
	models := GetModelsForProvider("codex")
	found := false
	for _, m := range models {
		if m == "gpt-4o" {
			found = true
			break
		}
	}
	if !found {
		t.Error("codex provider should include gpt-4o model")
	}
}

func TestResolveModelAlias(t *testing.T) {
	tests := []struct {
		alias    string
		expected string
	}{
		{"gpt4o", "gpt-4o"},
		{"gpt-4-o", "gpt-4o"},
		{"gpt5", "gpt-5"},
		{"gpt-3.5", "gpt-3.5-turbo"},
		{"gpt-4o", "gpt-4o"}, // Not an alias, should return as-is
		{"unknown-model", "unknown-model"},
	}

	for _, tt := range tests {
		t.Run(tt.alias, func(t *testing.T) {
			result := ResolveModelAlias(tt.alias)
			if result != tt.expected {
				t.Errorf("ResolveModelAlias(%s) = %s, want %s", tt.alias, result, tt.expected)
			}
		})
	}
}

func TestIsModelSupportedByProvider(t *testing.T) {
	tests := []struct {
		model        string
		providerType string
		expected     bool
	}{
		{"gpt-5", "codex", true},
		{"gpt-4o", "codex", true},
		{"gpt-4o", "copilot", true},
		{"gpt4o", "codex", true}, // Alias
		{"gpt5", "codex", true},  // Alias
		{"claude-3-5-sonnet-20241022", "anthropic", true},
		{"gpt-5", "anthropic", false},
		{"unknown-model", "codex", false},
	}

	for _, tt := range tests {
		t.Run(tt.model+"_"+tt.providerType, func(t *testing.T) {
			result := IsModelSupportedByProvider(tt.model, tt.providerType)
			if result != tt.expected {
				t.Errorf("IsModelSupportedByProvider(%s, %s) = %v, want %v", tt.model, tt.providerType, result, tt.expected)
			}
		})
	}
}

func TestMaskEmail(t *testing.T) {
	tests := []struct {
		email    string
		expected string
	}{
		{"user@example.com", "u***@example.com"},
		{"a@b.com", "a@b.com"},
		{"test", "***"},
		{"", ""},
		{"ab@domain.org", "a***@domain.org"},
	}

	for _, tt := range tests {
		t.Run(tt.email, func(t *testing.T) {
			result := maskEmail(tt.email)
			if result != tt.expected {
				t.Errorf("maskEmail(%s) = %s, want %s", tt.email, result, tt.expected)
			}
		})
	}
}

func TestParseModelWithProvider(t *testing.T) {
	tests := []struct {
		input        string
		wantProvider string
		wantModel    string
		wantOriginal string
	}{
		// No prefix - routes by priority
		{
			input:        "gpt-5.3-codex",
			wantProvider: "",
			wantModel:    "gpt-5.3-codex",
			wantOriginal: "gpt-5.3-codex",
		},
		{
			input:        "gpt-4o",
			wantProvider: "",
			wantModel:    "gpt-4o",
			wantOriginal: "gpt-4o",
		},
		// Provider prefix - forces provider
		{
			input:        "copilot/gpt-5.3-codex",
			wantProvider: "copilot",
			wantModel:    "gpt-5.3-codex",
			wantOriginal: "copilot/gpt-5.3-codex",
		},
		{
			input:        "codex/gpt-5",
			wantProvider: "codex",
			wantModel:    "gpt-5",
			wantOriginal: "codex/gpt-5",
		},
		{
			input:        "openai/gpt-4o",
			wantProvider: "openai",
			wantModel:    "gpt-4o",
			wantOriginal: "openai/gpt-4o",
		},
		{
			input:        "anthropic/claude-3-opus",
			wantProvider: "anthropic",
			wantModel:    "claude-3-opus",
			wantOriginal: "anthropic/claude-3-opus",
		},
		// Unknown prefix - not treated as provider
		{
			input:        "unknown/some-model",
			wantProvider: "",
			wantModel:    "unknown/some-model",
			wantOriginal: "unknown/some-model",
		},
		// Edge cases
		{
			input:        "copilot/",
			wantProvider: "",
			wantModel:    "copilot/",
			wantOriginal: "copilot/",
		},
		{
			input:        "/gpt-5",
			wantProvider: "",
			wantModel:    "/gpt-5",
			wantOriginal: "/gpt-5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := ParseModelWithProvider(tt.input)
			if result.Provider != tt.wantProvider {
				t.Errorf("ParseModelWithProvider(%q).Provider = %q, want %q", tt.input, result.Provider, tt.wantProvider)
			}
			if result.Model != tt.wantModel {
				t.Errorf("ParseModelWithProvider(%q).Model = %q, want %q", tt.input, result.Model, tt.wantModel)
			}
			if result.Original != tt.wantOriginal {
				t.Errorf("ParseModelWithProvider(%q).Original = %q, want %q", tt.input, result.Original, tt.wantOriginal)
			}
		})
	}
}

func TestIsKnownProvider(t *testing.T) {
	known := []string{"codex", "copilot", "openai", "anthropic"}
	unknown := []string{"unknown", "google", "azure", ""}

	for _, p := range known {
		if !isKnownProvider(p) {
			t.Errorf("isKnownProvider(%q) = false, want true", p)
		}
	}

	for _, p := range unknown {
		if isKnownProvider(p) {
			t.Errorf("isKnownProvider(%q) = true, want false", p)
		}
	}
}

func TestGetProviderPrefixedModels(t *testing.T) {
	prefixedModels := GetProviderPrefixedModels("codex")
	if len(prefixedModels) == 0 {
		t.Fatal("expected prefixed models for codex provider")
	}

	// Check that all models are properly prefixed
	for _, m := range prefixedModels {
		if len(m) <= len("codex/") {
			t.Errorf("expected model to be prefixed with 'codex/', got %q", m)
		}
		if m[:6] != "codex/" {
			t.Errorf("expected model to start with 'codex/', got %q", m)
		}
	}

	// Check one known model is present
	found := false
	for _, m := range prefixedModels {
		if m == "codex/gpt-5" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'codex/gpt-5' to be in prefixed models")
	}
}

func TestGetCopilotModelID(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		// Core GPT models pass through unchanged (real upstream model IDs)
		{"gpt-5", "gpt-5"},
		{"gpt-5-mini", "gpt-5-mini"},
		{"gpt-5-codex", "gpt-5-codex"},
		{"gpt-4o", "gpt-4o"},
		{"gpt-4", "gpt-4"},
		{"gpt-4.1", "gpt-4.1"},
		{"o1", "o1"},
		{"o1-mini", "o1-mini"},
		{"gpt-3.5-turbo", "gpt-3.5-turbo"},

		// GPT-5.x Codex variants - PASS THROUGH (real Copilot model IDs)
		// These ARE available in Copilot /models endpoint
		{"gpt-5.3-codex", "gpt-5.3-codex"},             // Real model - pass through
		{"gpt-5.3-codex-spark", "gpt-5.3-codex-spark"}, // Real model - pass through
		{"gpt-5.2-codex", "gpt-5.2-codex"},             // Real model - pass through
		{"gpt-5.2", "gpt-5.2"},                         // Real model - pass through
		{"gpt-5.1", "gpt-5.1"},                         // Real model - pass through
		{"gpt-5.1-codex", "gpt-5.1-codex"},             // Real model - pass through
		{"gpt-5.1-codex-mini", "gpt-5.1-codex-mini"},   // Real model - pass through
		{"gpt-5-codex-mini", "gpt-5-codex-mini"},       // Real model - pass through

		// Alias: gpt-5.1-codex-max -> gpt-5.1-codex (no "max" variant exists)
		{"gpt-5.1-codex-max", "gpt-5.1-codex"},

		// Claude models with dot notation transformation (alias)
		{"claude-3-5-sonnet", "claude-3.5-sonnet"},

		// Claude models with date suffix stripping
		{"claude-sonnet-4-5-20241022", "claude-sonnet-4.5"},
		{"claude-opus-4-6-20260205", "claude-opus-4.6"},
		{"claude-opus-4-5-20251101", "claude-opus-4.5"},
		{"claude-haiku-4-5-20251001", "claude-haiku-4.5"},

		// Claude hyphen-to-dot aliases
		{"claude-sonnet-4-5", "claude-sonnet-4.5"},
		{"claude-opus-4-5", "claude-opus-4.5"},
		{"claude-opus-4-6", "claude-opus-4.6"},
		{"claude-haiku-4-5", "claude-haiku-4.5"},

		// Claude models without transformation (already correct format)
		{"claude-sonnet-4.5", "claude-sonnet-4.5"},
		{"claude-opus-4.5", "claude-opus-4.5"},
		{"claude-haiku-4.5", "claude-haiku-4.5"},

		// Gemini models - real IDs pass through
		{"gemini-2.5-pro", "gemini-2.5-pro"},
		{"gemini-3-pro", "gemini-3-pro"},
		{"gemini-3.1-pro-preview", "gemini-3.1-pro-preview"},

		// Gemini preview aliases
		{"gemini-3-pro-preview", "gemini-3-pro"},
		{"gemini-3.1-pro", "gemini-3.1-pro-preview"},
		{"gemini-3-flash-preview", "gemini-3-flash"},

		// Unknown models should pass through unchanged
		{"unknown-model", "unknown-model"},
		{"custom/model", "custom/model"},
		{"grok-code-fast-1", "grok-code-fast-1"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := GetCopilotModelID(tt.input)
			if result != tt.expected {
				t.Errorf("GetCopilotModelID(%s) = %s, want %s", tt.input, result, tt.expected)
			}
		})
	}
}

func TestCopilotModelSupportIncludesGPT5Codex(t *testing.T) {
	models := GetModelsForProvider("copilot")

	// Check that various GPT-5 related models are supported
	expectedModels := []string{
		"gpt-5",
		"gpt-5-mini",
		"gpt-5.3-codex",
		"gpt-5.4",
		"gpt-5-codex",
	}

	for _, expected := range expectedModels {
		found := false
		for _, m := range models {
			if m == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("copilot provider should include model %q", expected)
		}
	}
}

func TestCopilotModelSupportIncludesClaude(t *testing.T) {
	models := GetModelsForProvider("copilot")

	// Check Claude models are supported
	expectedModels := []string{
		"claude-3-5-sonnet",
		"claude-opus-4.5",
		"claude-sonnet-4.5",
	}

	for _, expected := range expectedModels {
		found := false
		for _, m := range models {
			if m == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("copilot provider should include model %q", expected)
		}
	}
}

func TestTransformClaudeModel(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		// Models with date suffix should be stripped
		{"claude-sonnet-4-5-20241022", "claude-sonnet-4-5"},
		{"claude-opus-4-6-20260205", "claude-opus-4-6"},
		{"claude-opus-4-5-20251101", "claude-opus-4-5"},
		{"claude-haiku-4-5-20251001", "claude-haiku-4-5"},

		// Models without date suffix should pass through
		{"claude-sonnet-4.5", "claude-sonnet-4.5"},
		{"claude-opus-4.6", "claude-opus-4.6"},
		{"claude-3.5-sonnet", "claude-3.5-sonnet"},

		// Non-Claude models should pass through
		{"gpt-5.3-codex", "gpt-5.3-codex"},
		{"gpt-4o", "gpt-4o"},
		{"o1", "o1"},

		// Short strings should pass through
		{"claude", "claude"},
		{"gpt", "gpt"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := transformClaudeModel(tt.input)
			if result != tt.expected {
				t.Errorf("transformClaudeModel(%s) = %s, want %s", tt.input, result, tt.expected)
			}
		})
	}
}

func TestIsCopilotModelSupported(t *testing.T) {
	tests := []struct {
		input             string
		expectedMapped    string
		expectUnsupported bool
	}{
		// Supported models - pass through unchanged
		{"gpt-5", "gpt-5", false},
		{"gpt-5-codex", "gpt-5-codex", false},
		{"gpt-5.3-codex", "gpt-5.3-codex", false}, // Real model - passes through unchanged
		{"gpt-5.2-codex", "gpt-5.2-codex", false}, // Real model - passes through unchanged
		{"gemini-3.1-pro", "gemini-3.1-pro-preview", false},
		{"claude-sonnet-4.5", "claude-sonnet-4.5", false},

		// Alias transformations
		{"claude-3-5-sonnet", "claude-3.5-sonnet", false}, // Alias maps to dot notation

		// Unsupported models with clear alternatives
		{"gpt-5-turbo", "", true},
		{"o3", "", true},
		{"o3-mini", "", true},

		// Unknown models (not in our list, but may work dynamically) - pass through
		{"some-new-model-2026", "some-new-model-2026", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			mapped, reason := IsCopilotModelSupported(tt.input)
			if tt.expectUnsupported {
				if reason == "" {
					t.Errorf("IsCopilotModelSupported(%s) should return unsupported reason", tt.input)
				}
				if mapped != "" {
					t.Errorf("IsCopilotModelSupported(%s) mapped = %s, want empty for unsupported", tt.input, mapped)
				}
			} else {
				if reason != "" {
					t.Errorf("IsCopilotModelSupported(%s) unexpected reason: %s", tt.input, reason)
				}
				if mapped != tt.expectedMapped {
					t.Errorf("IsCopilotModelSupported(%s) mapped = %s, want %s", tt.input, mapped, tt.expectedMapped)
				}
			}
		})
	}
}

func TestCopilotModelPassThrough_GPT53Codex(t *testing.T) {
	// Verify that gpt-5.3-codex (a real Copilot model ID) passes through unchanged
	// This is critical: real upstream model IDs should NOT be rewritten
	mapped := GetCopilotModelID("gpt-5.3-codex")
	if mapped != "gpt-5.3-codex" {
		t.Errorf("gpt-5.3-codex should pass through unchanged for Copilot, got %s", mapped)
	}

	// Also verify gpt-5.3-codex-spark passes through
	mapped = GetCopilotModelID("gpt-5.3-codex-spark")
	if mapped != "gpt-5.3-codex-spark" {
		t.Errorf("gpt-5.3-codex-spark should pass through unchanged for Copilot, got %s", mapped)
	}

	// Verify gpt-5.3-codex is in the supported list (for user-facing model selection)
	if !IsModelSupportedByProvider("gpt-5.3-codex", "copilot") {
		t.Error("gpt-5.3-codex should be listed as supported by copilot provider")
	}
}

func TestCopilotProviderPrefix_GPT53Codex(t *testing.T) {
	// Test that copilot/gpt-5.3-codex is correctly parsed and model passes through
	parsed := ParseModelWithProvider("copilot/gpt-5.3-codex")

	if parsed.Provider != "copilot" {
		t.Errorf("ParseModelWithProvider(copilot/gpt-5.3-codex).Provider = %q, want %q", parsed.Provider, "copilot")
	}
	if parsed.Model != "gpt-5.3-codex" {
		t.Errorf("ParseModelWithProvider(copilot/gpt-5.3-codex).Model = %q, want %q", parsed.Model, "gpt-5.3-codex")
	}

	// Verify the model is passed through unchanged when sent to Copilot
	copilotModel := GetCopilotModelID(parsed.Model)
	if copilotModel != "gpt-5.3-codex" {
		t.Errorf("GetCopilotModelID(%s) = %s, want %s (should pass through unchanged)", parsed.Model, copilotModel, "gpt-5.3-codex")
	}
}

func TestCopilotAliasesOnly_NoFalseRewrites(t *testing.T) {
	// These are real Copilot model IDs that should NEVER be rewritten
	realModels := []string{
		"gpt-5.3-codex",
		"gpt-5.3-codex-spark",
		"gpt-5.2-codex",
		"gpt-5.2",
		"gpt-5.1",
		"gpt-5.1-codex",
		"gpt-5.1-codex-mini",
		"gpt-5-codex",
		"gpt-5-codex-mini",
		"gpt-5",
		"gpt-5-mini",
		"gpt-4o",
		"gpt-4",
		"gpt-4.1",
		"gpt-4-turbo",
		"gpt-3.5-turbo",
		"o1",
		"o1-mini",
		"o1-preview",
		"claude-sonnet-4",
		"claude-sonnet-4.5",
		"claude-opus-4",
		"claude-opus-4.1",
		"claude-opus-4.5",
		"claude-opus-4.6",
		"claude-haiku-4.5",
		"claude-3.5-sonnet",
		"claude-3-opus",
		"claude-3-sonnet",
		"claude-3-haiku",
		"gemini-2.5-pro",
		"gemini-3-pro",
		"gemini-3.1-pro-preview",
		"grok-code-fast-1",
	}

	for _, model := range realModels {
		t.Run(model, func(t *testing.T) {
			result := GetCopilotModelID(model)
			if result != model {
				t.Errorf("GetCopilotModelID(%s) = %s, want %s (real model should pass through)", model, result, model)
			}
		})
	}
}
