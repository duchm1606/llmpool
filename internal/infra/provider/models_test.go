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
