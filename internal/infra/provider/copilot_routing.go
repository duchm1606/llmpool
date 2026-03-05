package provider

import (
	"regexp"
	"strings"
)

// CopilotEndpoint represents the endpoint type for Copilot API calls.
type CopilotEndpoint string

const (
	// CopilotEndpointChat is the /chat/completions endpoint.
	CopilotEndpointChat CopilotEndpoint = "chat"
	// CopilotEndpointResponses is the /responses endpoint.
	CopilotEndpointResponses CopilotEndpoint = "responses"
	// CopilotEndpointMessages is the /v1/messages endpoint (native Anthropic-style).
	CopilotEndpointMessages CopilotEndpoint = "messages"
)

var gptVersionRegex = regexp.MustCompile(`^gpt-(\d+)`)

// IsGPT5OrLater checks if a model ID is GPT-5 or a later version.
// Reference: opencode provider.ts isGpt5OrLater function.
// Examples:
//   - "gpt-5" -> true
//   - "gpt-5-mini" -> true
//   - "gpt-5-codex" -> true
//   - "gpt-5.1-codex" -> true
//   - "gpt-4o" -> false
//   - "claude-3.5-sonnet" -> false
func IsGPT5OrLater(modelID string) bool {
	matches := gptVersionRegex.FindStringSubmatch(modelID)
	if len(matches) < 2 {
		return false
	}
	// Parse the major version number
	// matches[1] is the first capture group (the version number)
	version := 0
	for _, c := range matches[1] {
		if c >= '0' && c <= '9' {
			version = version*10 + int(c-'0')
		} else {
			break
		}
	}
	return version >= 5
}

// ShouldUseCopilotResponsesAPI determines if a model should use the /responses endpoint.
// Reference: opencode provider.ts shouldUseCopilotResponsesApi function.
//
// Logic:
//   - GPT-5+ models should use /responses EXCEPT gpt-5-mini
//   - All other models use /chat/completions
//
// Examples:
//   - "gpt-5" -> true (use responses)
//   - "gpt-5-codex" -> true (use responses)
//   - "gpt-5.1-codex" -> true (use responses)
//   - "gpt-5-mini" -> false (use chat)
//   - "gpt-4o" -> false (use chat)
//   - "claude-3.5-sonnet" -> false (use chat)
func ShouldUseCopilotResponsesAPI(modelID string) bool {
	return IsGPT5OrLater(modelID) && !strings.HasPrefix(modelID, "gpt-5-mini")
}

// ResolveCopilotEndpoint determines which Copilot API endpoint to use for a model.
// When enableResponsesRouting is false, always returns chat endpoint for backward compatibility.
// When enableResponsesRouting is true, uses the shouldUseCopilotResponsesAPI logic.
func ResolveCopilotEndpoint(modelID string, enableResponsesRouting bool) CopilotEndpoint {
	if !enableResponsesRouting {
		return CopilotEndpointChat
	}
	if ShouldUseCopilotResponsesAPI(modelID) {
		return CopilotEndpointResponses
	}
	return CopilotEndpointChat
}

// CopilotEndpointPath returns the URL path for the given endpoint.
func (e CopilotEndpoint) Path() string {
	switch e {
	case CopilotEndpointResponses:
		return "/responses"
	case CopilotEndpointMessages:
		return "/v1/messages"
	default:
		return "/chat/completions"
	}
}

// ShouldUseCopilotMessagesAPI determines if a model should use the /v1/messages endpoint.
// The Messages API is the native Anthropic-style endpoint, preferred for Claude models
// that support adaptive thinking and extended features.
//
// Currently enabled for Claude 4.x models (sonnet-4.5, opus-4.5, opus-4.6, etc.)
// which support features like adaptive_thinking through the Messages API.
//
// Examples:
//   - "claude-sonnet-4.5" -> true (use messages)
//   - "claude-opus-4.5" -> true (use messages)
//   - "claude-3.5-sonnet" -> false (older model, use chat)
//   - "gpt-5" -> false (not Claude, use responses or chat)
func ShouldUseCopilotMessagesAPI(modelID string) bool {
	// Claude 4.x models with adaptive thinking support
	// These models have been verified to support /v1/messages on Copilot
	messagesModels := []string{
		"claude-sonnet-4.5",
		"claude-opus-4.5",
		"claude-opus-4.6",
		"claude-haiku-4.5",
		// Also support the base versions without minor version
		"claude-sonnet-4",
		"claude-opus-4",
	}

	for _, m := range messagesModels {
		if strings.HasPrefix(modelID, m) {
			return true
		}
	}

	return false
}

// SupportsAdaptiveThinking checks if a model supports adaptive thinking config.
// Adaptive thinking enables dynamic reasoning effort for Claude models via the Messages API.
//
// IMPORTANT: "Adaptive thinking" here refers to models that support reasoning with
// configurable effort (via output_config.effort). The thinking.type MUST be set to
// "enabled" (NOT "adaptive") - the downstream API only accepts "enabled" or "disabled".
func SupportsAdaptiveThinking(modelID string) bool {
	// Models that support adaptive_thinking in their capabilities
	adaptiveModels := []string{
		"claude-sonnet-4.5",
		"claude-opus-4.5",
		"claude-opus-4.6",
	}

	for _, m := range adaptiveModels {
		if strings.HasPrefix(modelID, m) {
			return true
		}
	}

	return false
}
