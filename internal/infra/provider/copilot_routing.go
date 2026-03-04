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
	default:
		return "/chat/completions"
	}
}
