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
		"gpt-5.3-codex",
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

	// GitHub Copilot - supports extensive model list via copilot-api
	// Reference: proxypal COPILOT_MODELS and GitHub Copilot /models endpoint
	// NOTE: These are user-facing model IDs; CopilotModelMapping translates to upstream IDs
	"copilot": {
		// OpenAI GPT models via Copilot
		"gpt-4o",
		"gpt-4",
		"gpt-4-turbo",
		"gpt-4.1",
		"gpt-3.5-turbo",
		"gpt-5",
		"gpt-5-mini",
		"gpt-5-codex",
		"gpt-5-codex-mini",
		"gpt-5.1",
		"gpt-5.1-codex",
		"gpt-5.1-codex-mini",
		// Client aliases that map to gpt-5-codex (Amp uses these)
		"gpt-5.2",
		"gpt-5.2-codex",
		"gpt-5.3-codex",
		"gpt-5.3-codex-spark",
		"gpt-5.4",
		// O-series reasoning models
		"o1",
		"o1-mini",
		"o1-preview",
		// Claude models via Copilot
		"claude-3.5-sonnet",
		"claude-3-5-sonnet",
		"claude-3-opus",
		"claude-3-sonnet",
		"claude-3-haiku",
		"claude-sonnet-4",
		"claude-sonnet-4.5",
		"claude-opus-4",
		"claude-opus-4.1",
		"claude-opus-4.5",
		"claude-opus-4.6",
		"claude-haiku-4.5",
		// Gemini models via Copilot
		"gemini-2.5-pro",
		"gemini-3-pro",
		"gemini-3.1-pro-preview",
		// Other models
		"grok-code-fast-1",
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

// CopilotModelAliases maps compatibility aliases to Copilot upstream model IDs.
// These are ONLY for known aliases where the client uses a different naming convention.
//
// IMPORTANT: If a model ID is a real upstream Copilot model (from /models endpoint),
// it should NOT be in this map - it will pass through unchanged via GetCopilotModelID.
//
// Reference: GitHub Copilot /models endpoint returns real model IDs.
var CopilotModelAliases = map[string]string{
	// Claude models - normalize hyphen vs dot notation (claude-3-5-sonnet -> claude-3.5-sonnet)
	"claude-3-5-sonnet": "claude-3.5-sonnet",

	// Claude 4.x models - normalize hyphen notation to dot notation
	"claude-sonnet-4-5": "claude-sonnet-4.5",
	"claude-opus-4-5":   "claude-opus-4.5",
	"claude-opus-4-6":   "claude-opus-4.6",
	"claude-haiku-4-5":  "claude-haiku-4.5",

	// Claude models with date suffixes - strip to base model
	// Reference: copilot-api translateModelName strips date suffixes
	"claude-sonnet-4-5-20241022": "claude-sonnet-4.5",
	"claude-opus-4-5-20251101":   "claude-opus-4.5",
	"claude-opus-4-6-20260205":   "claude-opus-4.6",
	"claude-haiku-4-5-20251001":  "claude-haiku-4.5",

	// Gemini preview aliases
	"gemini-3-pro-preview":   "gemini-3-pro",
	"gemini-3.1-pro":         "gemini-3.1-pro-preview",
	"gemini-3-flash-preview": "gemini-3-flash",

	// GPT-5.1-codex-max is an alias for gpt-5.1-codex (no "max" variant exists)
	"gpt-5.1-codex-max": "gpt-5.1-codex",
}

// CopilotUnsupportedModels lists model IDs that are known to be unsupported by Copilot.
// When these are requested, we return a clear error with alternatives.
var CopilotUnsupportedModels = map[string]string{
	// These models don't exist in Copilot; suggest alternatives
	"gpt-5-turbo": "gpt-5 or gpt-5-codex",
	"o3":          "o1 or o1-mini",
	"o3-mini":     "o1-mini",
}

// GetCopilotModelID returns the Copilot-specific model ID for a given model.
// Strategy (pass-through first):
//  1. Check if model is a known alias -> return mapped value
//  2. Apply Claude date-stripping transformation -> check alias again
//  3. Otherwise, return the original model unchanged (pass-through)
//
// IMPORTANT: Real upstream Copilot model IDs (like gpt-5.3-codex) pass through unchanged.
// Only compatibility aliases are transformed.
func GetCopilotModelID(model string) string {
	// Check alias mapping first (known compatibility aliases only)
	if mapped, ok := CopilotModelAliases[model]; ok {
		return mapped
	}

	// Apply Claude date-stripping transformation for unmapped models
	// This handles dynamic models like claude-opus-4-7-20260501 that aren't in our alias list
	if transformed := transformClaudeModel(model); transformed != model {
		// Check if transformed model has an alias
		if mapped, ok := CopilotModelAliases[transformed]; ok {
			return mapped
		}
		return transformed
	}

	// Pass through unchanged - real upstream model IDs work as-is
	return model
}

// transformClaudeModel applies Claude model naming transformations.
// Copilot doesn't support date-suffixed Claude models; this strips the suffix.
func transformClaudeModel(model string) string {
	// Match patterns like claude-{variant}-4-{version}-YYYYMMDD
	// e.g., claude-sonnet-4-5-20241022, claude-opus-4-6-20260205
	if len(model) < 20 {
		return model
	}

	// Check if it starts with claude- and ends with -YYYYMMDD pattern
	if len(model) > 9 && model[:7] == "claude-" {
		// Check for date suffix (8 digits at the end after a hyphen)
		lastHyphen := -1
		for i := len(model) - 1; i >= 0; i-- {
			if model[i] == '-' {
				lastHyphen = i
				break
			}
		}

		if lastHyphen > 0 && len(model)-lastHyphen-1 == 8 {
			suffix := model[lastHyphen+1:]
			allDigits := true
			for _, c := range suffix {
				if c < '0' || c > '9' {
					allDigits = false
					break
				}
			}
			if allDigits {
				// Strip the date suffix
				return model[:lastHyphen]
			}
		}
	}

	return model
}

// IsCopilotModelSupported checks if a model is supported by Copilot.
// Returns the mapped model ID if supported, empty string and error message if not.
func IsCopilotModelSupported(model string) (mappedModel string, unsupportedReason string) {
	// Check if it's in the unsupported list
	if alternative, isUnsupported := CopilotUnsupportedModels[model]; isUnsupported {
		return "", "model '" + model + "' is not available via Copilot. Try: " + alternative
	}

	// Get the mapped model ID
	mappedModel = GetCopilotModelID(model)

	// Check if the mapped model is in our known supported list
	models := GetModelsForProvider("copilot")
	for _, m := range models {
		if m == mappedModel || m == model {
			return mappedModel, ""
		}
	}

	// Model not in our list but might still work (Copilot /models is dynamic)
	// Return the mapped model without error - upstream will validate
	return mappedModel, ""
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

// ParsedModel represents a parsed model string that may include a provider prefix.
// Formats supported:
//   - "gpt-5.3-codex"           -> Provider: "", Model: "gpt-5.3-codex"
//   - "copilot/gpt-5.3-codex"   -> Provider: "copilot", Model: "gpt-5.3-codex"
//   - "codex/gpt-5"             -> Provider: "codex", Model: "gpt-5"
type ParsedModel struct {
	// Provider is the explicitly requested provider (empty if not specified)
	Provider string
	// Model is the actual model ID to use
	Model string
	// Original is the original model string before parsing
	Original string
}

// ParseModelWithProvider parses a model string that may contain a provider prefix.
// Provider prefixes allow forcing requests to specific providers.
//
// Supported formats:
//   - "gpt-5.3-codex"           -> No prefix, route by priority
//   - "copilot/gpt-5.3-codex"   -> Force copilot provider
//   - "codex/gpt-5"             -> Force codex provider
//
// Known provider prefixes: copilot
func ParseModelWithProvider(model string) ParsedModel {
	result := ParsedModel{
		Original: model,
		Model:    model,
	}

	// Check for provider prefix (format: "provider/model")
	// Only split on first "/" to handle models that might have "/" in their name
	idx := -1
	for i, c := range model {
		if c == '/' {
			idx = i
			break
		}
	}

	if idx > 0 && idx < len(model)-1 {
		prefix := model[:idx]
		suffix := model[idx+1:]

		// Only treat as provider prefix if it's a known provider
		if isKnownProvider(prefix) {
			result.Provider = prefix
			result.Model = suffix
		}
	}

	return result
}

// isKnownProvider checks if a string is a known provider ID.
func isKnownProvider(s string) bool {
	switch s {
	case "copilot":
		return true
	default:
		return false
	}
}

// GetProviderPrefixedModels returns models with provider prefix for /v1/models endpoint.
// This allows clients to see available provider-specific routing options.
// For example, if "gpt-5" is supported by both codex and copilot, this returns:
//   - "gpt-5" (routes by priority)
//   - "copilot/gpt-5" (forces copilot)
//   - "codex/gpt-5" (forces codex)
func GetProviderPrefixedModels(providerType string) []string {
	models := GetModelsForProvider(providerType)
	result := make([]string, 0, len(models))
	for _, m := range models {
		result = append(result, providerType+"/"+m)
	}
	return result
}
