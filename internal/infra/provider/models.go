// Package provider provides infrastructure implementations for provider management.
package provider

// ProviderModels defines supported models per provider type.
// This replaces hardcoded model lists in config - models are associated with credential types.
// Following proxypal pattern: models are defined by what the upstream API actually supports.
var ProviderModels = map[string][]string{
	// Codex (OpenAI via chatgpt.com OAuth) - extensive model support including latest
	// Reference: https://platform.openai.com/docs/models
	"codex": {
		// GPT-5 series (latest)
		"gpt-5",
		"gpt-5-mini",
		"gpt-5-turbo",
		// GPT-4 series
		"gpt-4o",
		"gpt-4o-mini",
		"gpt-4-turbo",
		"gpt-4-turbo-preview",
		"gpt-4",
		"gpt-4-32k",
		// GPT-3.5 series
		"gpt-3.5-turbo",
		"gpt-3.5-turbo-16k",
		// O-series (reasoning models)
		"o1",
		"o1-preview",
		"o1-mini",
		"o3",
		"o3-mini",
		// Legacy naming support
		"gpt-4o-2024-05-13",
		"gpt-4o-2024-08-06",
		"gpt-4-0125-preview",
		"gpt-4-1106-preview",
	},

	// GitHub Copilot - supports subset of models via copilot-api
	// Reference: models.ts in proxypal - COPILOT_MODELS
	"copilot": {
		// OpenAI models via Copilot
		"gpt-4o",
		"gpt-4",
		"gpt-4-turbo",
		"gpt-3.5-turbo",
		"gpt-5",
		"gpt-5-mini",
		"o1",
		"o1-mini",
		// Claude models via Copilot
		"claude-3-5-sonnet",
		"claude-3-opus",
		"claude-3-sonnet",
		"claude-3-haiku",
	},

	// Direct OpenAI API (API key based)
	"openai": {
		"gpt-4o",
		"gpt-4o-mini",
		"gpt-4-turbo",
		"gpt-4",
		"gpt-3.5-turbo",
		"o1-preview",
		"o1-mini",
	},

	// Anthropic (API key based)
	"anthropic": {
		"claude-3-5-sonnet-20241022",
		"claude-3-opus-20240229",
		"claude-3-sonnet-20240229",
		"claude-3-haiku-20240307",
	},
}

// ModelAliases maps common aliases to canonical model IDs.
// Clients may use different naming conventions.
var ModelAliases = map[string]string{
	// GPT-4o aliases
	"gpt4o":         "gpt-4o",
	"gpt-4-o":       "gpt-4o",
	"gpt4-o":        "gpt-4o",
	"gpt-4o-latest": "gpt-4o",

	// GPT-5 aliases
	"gpt5":         "gpt-5",
	"gpt-5-latest": "gpt-5",

	// GPT-3.5 aliases
	"gpt-3.5":     "gpt-3.5-turbo",
	"gpt35-turbo": "gpt-3.5-turbo",
	"gpt35":       "gpt-3.5-turbo",

	// O-series aliases
	"o1-latest": "o1",
	"o3-latest": "o3",
}

// GetModelsForProvider returns the supported models for a given provider type.
func GetModelsForProvider(providerType string) []string {
	if models, ok := ProviderModels[providerType]; ok {
		return models
	}
	return nil
}

// ResolveModelAlias returns the canonical model ID for an alias, or the original if not aliased.
func ResolveModelAlias(model string) string {
	if canonical, ok := ModelAliases[model]; ok {
		return canonical
	}
	return model
}

// IsModelSupportedByProvider checks if a model is supported by a specific provider.
func IsModelSupportedByProvider(model string, providerType string) bool {
	// Resolve alias first
	canonicalModel := ResolveModelAlias(model)

	models := GetModelsForProvider(providerType)
	for _, m := range models {
		if m == canonicalModel || m == model {
			return true
		}
	}
	return false
}
