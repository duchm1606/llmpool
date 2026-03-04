package provider

import "testing"

func TestIsGPT5OrLater(t *testing.T) {
	tests := []struct {
		name    string
		modelID string
		want    bool
	}{
		// GPT-5+ models (should return true)
		{name: "gpt-5", modelID: "gpt-5", want: true},
		{name: "gpt-5-mini", modelID: "gpt-5-mini", want: true},
		{name: "gpt-5-codex", modelID: "gpt-5-codex", want: true},
		{name: "gpt-5-turbo", modelID: "gpt-5-turbo", want: true},
		{name: "gpt-5.1", modelID: "gpt-5.1", want: true},
		{name: "gpt-5.1-codex", modelID: "gpt-5.1-codex", want: true},
		{name: "gpt-5.1-codex-mini", modelID: "gpt-5.1-codex-mini", want: true},
		{name: "gpt-5.2-codex", modelID: "gpt-5.2-codex", want: true},
		{name: "gpt-5.3-codex", modelID: "gpt-5.3-codex", want: true},
		{name: "gpt-6", modelID: "gpt-6", want: true},
		{name: "gpt-10", modelID: "gpt-10", want: true},

		// GPT-4 and earlier (should return false)
		{name: "gpt-4o", modelID: "gpt-4o", want: false},
		{name: "gpt-4o-mini", modelID: "gpt-4o-mini", want: false},
		{name: "gpt-4-turbo", modelID: "gpt-4-turbo", want: false},
		{name: "gpt-4", modelID: "gpt-4", want: false},
		{name: "gpt-4.1", modelID: "gpt-4.1", want: false},
		{name: "gpt-3.5-turbo", modelID: "gpt-3.5-turbo", want: false},

		// Non-GPT models (should return false)
		{name: "claude-3.5-sonnet", modelID: "claude-3.5-sonnet", want: false},
		{name: "claude-sonnet-4", modelID: "claude-sonnet-4", want: false},
		{name: "o1", modelID: "o1", want: false},
		{name: "o1-mini", modelID: "o1-mini", want: false},
		{name: "o3", modelID: "o3", want: false},
		{name: "gemini-2.5-pro", modelID: "gemini-2.5-pro", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := IsGPT5OrLater(tc.modelID)
			if got != tc.want {
				t.Fatalf("IsGPT5OrLater(%q) = %v, want %v", tc.modelID, got, tc.want)
			}
		})
	}
}

func TestShouldUseCopilotResponsesAPI(t *testing.T) {
	tests := []struct {
		name    string
		modelID string
		want    bool
	}{
		// GPT-5+ models that SHOULD use responses
		{name: "gpt-5", modelID: "gpt-5", want: true},
		{name: "gpt-5-codex", modelID: "gpt-5-codex", want: true},
		{name: "gpt-5-turbo", modelID: "gpt-5-turbo", want: true},
		{name: "gpt-5.1", modelID: "gpt-5.1", want: true},
		{name: "gpt-5.1-codex", modelID: "gpt-5.1-codex", want: true},
		{name: "gpt-5.2-codex", modelID: "gpt-5.2-codex", want: true},
		{name: "gpt-5.3-codex", modelID: "gpt-5.3-codex", want: true},

		// gpt-5-mini variants should NOT use responses (exception)
		{name: "gpt-5-mini", modelID: "gpt-5-mini", want: false},
		{name: "gpt-5-mini-test", modelID: "gpt-5-mini-test", want: false},

		// GPT-4 and earlier should NOT use responses
		{name: "gpt-4o", modelID: "gpt-4o", want: false},
		{name: "gpt-4o-mini", modelID: "gpt-4o-mini", want: false},
		{name: "gpt-4-turbo", modelID: "gpt-4-turbo", want: false},
		{name: "gpt-4", modelID: "gpt-4", want: false},
		{name: "gpt-3.5-turbo", modelID: "gpt-3.5-turbo", want: false},

		// Non-GPT models should NOT use responses
		{name: "claude-3.5-sonnet", modelID: "claude-3.5-sonnet", want: false},
		{name: "claude-sonnet-4", modelID: "claude-sonnet-4", want: false},
		{name: "o1", modelID: "o1", want: false},
		{name: "gemini-2.5-pro", modelID: "gemini-2.5-pro", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ShouldUseCopilotResponsesAPI(tc.modelID)
			if got != tc.want {
				t.Fatalf("ShouldUseCopilotResponsesAPI(%q) = %v, want %v", tc.modelID, got, tc.want)
			}
		})
	}
}

func TestResolveCopilotEndpoint(t *testing.T) {
	tests := []struct {
		name                   string
		modelID                string
		enableResponsesRouting bool
		want                   CopilotEndpoint
	}{
		// Flag disabled - always chat
		{
			name:                   "flag off gpt-5",
			modelID:                "gpt-5",
			enableResponsesRouting: false,
			want:                   CopilotEndpointChat,
		},
		{
			name:                   "flag off gpt-5.3-codex",
			modelID:                "gpt-5.3-codex",
			enableResponsesRouting: false,
			want:                   CopilotEndpointChat,
		},

		// Flag enabled - GPT-5+ uses responses
		{
			name:                   "flag on gpt-5",
			modelID:                "gpt-5",
			enableResponsesRouting: true,
			want:                   CopilotEndpointResponses,
		},
		{
			name:                   "flag on gpt-5.3-codex",
			modelID:                "gpt-5.3-codex",
			enableResponsesRouting: true,
			want:                   CopilotEndpointResponses,
		},
		{
			name:                   "flag on gpt-5.1-codex",
			modelID:                "gpt-5.1-codex",
			enableResponsesRouting: true,
			want:                   CopilotEndpointResponses,
		},

		// Flag enabled - gpt-5-mini uses chat (exception)
		{
			name:                   "flag on gpt-5-mini",
			modelID:                "gpt-5-mini",
			enableResponsesRouting: true,
			want:                   CopilotEndpointChat,
		},

		// Flag enabled - GPT-4 and earlier use chat
		{
			name:                   "flag on gpt-4o",
			modelID:                "gpt-4o",
			enableResponsesRouting: true,
			want:                   CopilotEndpointChat,
		},
		{
			name:                   "flag on gpt-3.5-turbo",
			modelID:                "gpt-3.5-turbo",
			enableResponsesRouting: true,
			want:                   CopilotEndpointChat,
		},

		// Non-GPT models always use chat
		{
			name:                   "flag on claude-3.5-sonnet",
			modelID:                "claude-3.5-sonnet",
			enableResponsesRouting: true,
			want:                   CopilotEndpointChat,
		},
		{
			name:                   "flag on o1",
			modelID:                "o1",
			enableResponsesRouting: true,
			want:                   CopilotEndpointChat,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveCopilotEndpoint(tc.modelID, tc.enableResponsesRouting)
			if got != tc.want {
				t.Fatalf("ResolveCopilotEndpoint(%q, %v) = %v, want %v", tc.modelID, tc.enableResponsesRouting, got, tc.want)
			}
		})
	}
}

func TestCopilotEndpointPath(t *testing.T) {
	tests := []struct {
		endpoint CopilotEndpoint
		want     string
	}{
		{CopilotEndpointChat, "/chat/completions"},
		{CopilotEndpointResponses, "/responses"},
	}

	for _, tc := range tests {
		t.Run(string(tc.endpoint), func(t *testing.T) {
			got := tc.endpoint.Path()
			if got != tc.want {
				t.Fatalf("CopilotEndpoint(%q).Path() = %q, want %q", tc.endpoint, got, tc.want)
			}
		})
	}
}
