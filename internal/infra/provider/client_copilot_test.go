package provider

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	domaincompletion "github.com/duchoang/llmpool/internal/domain/completion"
	domainprovider "github.com/duchoang/llmpool/internal/domain/provider"
	"go.uber.org/zap"
)

func TestChatCompletionCopilot_PathAndHeaders(t *testing.T) {
	var gotPath string
	var gotAuth string
	var gotInitiator string
	var gotVision string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotInitiator = r.Header.Get("X-Initiator")
		gotVision = r.Header.Get("Copilot-Vision-Request")

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "chatcmpl-test",
			"object":  "chat.completion",
			"created": 1,
			"model":   "gpt-4o",
			"choices": []map[string]any{{
				"index": 0,
				"message": map[string]any{
					"role":    "assistant",
					"content": "ok",
				},
				"finish_reason": "stop",
			}},
		})
	}))
	defer server.Close()

	c := &client{httpClient: server.Client(), logger: zap.NewNop()}

	decision := domainprovider.RoutingDecision{
		ProviderID: domainprovider.ProviderCopilot,
		BaseURL:    server.URL,
		Token:      "copilot-token",
	}

	request := domaincompletion.ChatCompletionRequest{
		Model: "gpt-4o",
		Messages: []domaincompletion.Message{
			{Role: "user", Content: []any{
				map[string]any{"type": "text", "text": "hello"},
				map[string]any{"type": "image_url", "image_url": map[string]any{"url": "https://example.com/a.png"}},
			}},
		},
	}

	_, err := c.chatCompletionCopilot(t.Context(), decision, request)
	if err != nil {
		t.Fatalf("chatCompletionCopilot failed: %v", err)
	}

	if gotPath != "/chat/completions" {
		t.Fatalf("unexpected path: got %q, want %q", gotPath, "/chat/completions")
	}
	if gotAuth != "Bearer copilot-token" {
		t.Fatalf("unexpected auth header: got %q", gotAuth)
	}
	if gotInitiator != "agent" {
		t.Fatalf("unexpected X-Initiator: got %q, want %q", gotInitiator, "agent")
	}
	if gotVision != "true" {
		t.Fatalf("unexpected Copilot-Vision-Request: got %q, want %q", gotVision, "true")
	}
}

func TestChatCompletionStreamCopilot_PathAndInitiator(t *testing.T) {
	var gotPath string
	var gotInitiator string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotInitiator = r.Header.Get("X-Initiator")

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"id\":\"x\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hi\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	c := &client{httpClient: server.Client(), logger: zap.NewNop()}

	decision := domainprovider.RoutingDecision{
		ProviderID: domainprovider.ProviderCopilot,
		BaseURL:    server.URL,
		Token:      "copilot-token",
	}

	request := domaincompletion.ChatCompletionRequest{
		Model: "gpt-4o",
		Messages: []domaincompletion.Message{
			{Role: "assistant", Content: "previous tool output"},
			{Role: "user", Content: "continue"},
		},
	}

	chunks, err := c.chatCompletionStreamCopilot(t.Context(), decision, request)
	if err != nil {
		t.Fatalf("chatCompletionStreamCopilot failed: %v", err)
	}

	seenDone := false
	for chunk := range chunks {
		if chunk.Error != nil {
			t.Fatalf("unexpected stream error: %v", chunk.Error)
		}
		if chunk.Done {
			seenDone = true
		}
	}

	if !seenDone {
		t.Fatalf("expected done chunk")
	}
	if gotPath != "/chat/completions" {
		t.Fatalf("unexpected path: got %q, want %q", gotPath, "/chat/completions")
	}
	if gotInitiator != "agent" {
		t.Fatalf("unexpected X-Initiator: got %q, want %q", gotInitiator, "agent")
	}
}

func TestHasVisionContent_DetectsTypedContentPart(t *testing.T) {
	req := domaincompletion.ChatCompletionRequest{
		Messages: []domaincompletion.Message{
			{
				Role: "user",
				Content: []any{
					domaincompletion.ContentPart{Type: "text", Text: "hello"},
					domaincompletion.ContentPart{Type: "image_url", ImageURL: &domaincompletion.ImageURL{URL: "https://example.com/img.png"}},
				},
			},
		},
	}

	if !hasVisionContent(req) {
		t.Fatalf("expected typed content-part image to be detected")
	}
}

func TestHasVisionContent_DetectsMapContentPart(t *testing.T) {
	req := domaincompletion.ChatCompletionRequest{
		Messages: []domaincompletion.Message{
			{
				Role: "user",
				Content: []any{
					map[string]any{"type": "text", "text": "hello"},
					map[string]any{"type": "image_url", "image_url": map[string]any{"url": "https://example.com/img.png"}},
				},
			},
		},
	}

	if !hasVisionContent(req) {
		t.Fatalf("expected map content-part image to be detected")
	}
}

func TestChatCompletionCopilot_ResponsesRouting_FlagDisabled(t *testing.T) {
	// When flag is disabled, GPT-5 should still use /chat/completions
	var gotPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "chatcmpl-test",
			"object":  "chat.completion",
			"created": 1,
			"model":   "gpt-5",
			"choices": []map[string]any{{
				"index": 0,
				"message": map[string]any{
					"role":    "assistant",
					"content": "ok",
				},
				"finish_reason": "stop",
			}},
		})
	}))
	defer server.Close()

	// Feature flag disabled (default)
	c := &client{
		httpClient:                    server.Client(),
		logger:                        zap.NewNop(),
		enableCopilotResponsesRouting: false,
	}

	decision := domainprovider.RoutingDecision{
		ProviderID: domainprovider.ProviderCopilot,
		BaseURL:    server.URL,
		Token:      "copilot-token",
	}

	request := domaincompletion.ChatCompletionRequest{
		Model:    "gpt-5",
		Messages: []domaincompletion.Message{{Role: "user", Content: "hello"}},
	}

	_, err := c.chatCompletionCopilot(t.Context(), decision, request)
	if err != nil {
		t.Fatalf("chatCompletionCopilot failed: %v", err)
	}

	if gotPath != "/chat/completions" {
		t.Fatalf("with flag disabled, GPT-5 should use /chat/completions, got %q", gotPath)
	}
}

func TestChatCompletionCopilot_ResponsesRouting_FlagEnabled_GPT5(t *testing.T) {
	// When flag is enabled, GPT-5 should use /responses
	var gotPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "text/event-stream")
		// Responses API returns SSE events
		_, _ = w.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp-1\",\"model\":\"gpt-5\",\"output\":[{\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"hello\"}]}]}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	// Feature flag enabled
	c := &client{
		httpClient:                    server.Client(),
		logger:                        zap.NewNop(),
		enableCopilotResponsesRouting: true,
	}

	decision := domainprovider.RoutingDecision{
		ProviderID: domainprovider.ProviderCopilot,
		BaseURL:    server.URL,
		Token:      "copilot-token",
	}

	request := domaincompletion.ChatCompletionRequest{
		Model:    "gpt-5",
		Messages: []domaincompletion.Message{{Role: "user", Content: "hello"}},
	}

	resp, err := c.chatCompletionCopilot(t.Context(), decision, request)
	if err != nil {
		t.Fatalf("chatCompletionCopilot failed: %v", err)
	}

	if gotPath != "/responses" {
		t.Fatalf("with flag enabled, GPT-5 should use /responses, got %q", gotPath)
	}

	if len(resp.Choices) == 0 {
		t.Fatalf("expected at least one choice")
	}
}

func TestChatCompletionCopilot_ResponsesRouting_FlagEnabled_GPT5Mini(t *testing.T) {
	// When flag is enabled, gpt-5-mini should still use /chat/completions
	var gotPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "chatcmpl-test",
			"object":  "chat.completion",
			"created": 1,
			"model":   "gpt-5-mini",
			"choices": []map[string]any{{
				"index": 0,
				"message": map[string]any{
					"role":    "assistant",
					"content": "ok",
				},
				"finish_reason": "stop",
			}},
		})
	}))
	defer server.Close()

	// Feature flag enabled
	c := &client{
		httpClient:                    server.Client(),
		logger:                        zap.NewNop(),
		enableCopilotResponsesRouting: true,
	}

	decision := domainprovider.RoutingDecision{
		ProviderID: domainprovider.ProviderCopilot,
		BaseURL:    server.URL,
		Token:      "copilot-token",
	}

	request := domaincompletion.ChatCompletionRequest{
		Model:    "gpt-5-mini",
		Messages: []domaincompletion.Message{{Role: "user", Content: "hello"}},
	}

	_, err := c.chatCompletionCopilot(t.Context(), decision, request)
	if err != nil {
		t.Fatalf("chatCompletionCopilot failed: %v", err)
	}

	if gotPath != "/chat/completions" {
		t.Fatalf("gpt-5-mini should always use /chat/completions, got %q", gotPath)
	}
}

func TestChatCompletionCopilot_ResponsesRouting_FlagEnabled_GPT4o(t *testing.T) {
	// When flag is enabled, GPT-4o should still use /chat/completions
	var gotPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "chatcmpl-test",
			"object":  "chat.completion",
			"created": 1,
			"model":   "gpt-4o",
			"choices": []map[string]any{{
				"index": 0,
				"message": map[string]any{
					"role":    "assistant",
					"content": "ok",
				},
				"finish_reason": "stop",
			}},
		})
	}))
	defer server.Close()

	// Feature flag enabled
	c := &client{
		httpClient:                    server.Client(),
		logger:                        zap.NewNop(),
		enableCopilotResponsesRouting: true,
	}

	decision := domainprovider.RoutingDecision{
		ProviderID: domainprovider.ProviderCopilot,
		BaseURL:    server.URL,
		Token:      "copilot-token",
	}

	request := domaincompletion.ChatCompletionRequest{
		Model:    "gpt-4o",
		Messages: []domaincompletion.Message{{Role: "user", Content: "hello"}},
	}

	_, err := c.chatCompletionCopilot(t.Context(), decision, request)
	if err != nil {
		t.Fatalf("chatCompletionCopilot failed: %v", err)
	}

	if gotPath != "/chat/completions" {
		t.Fatalf("gpt-4o should always use /chat/completions, got %q", gotPath)
	}
}

func TestChatCompletionStreamCopilot_ResponsesRouting_FlagEnabled_GPT5(t *testing.T) {
	// When flag is enabled, GPT-5 streaming should use /responses
	var gotPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"Hello\"}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"response.output_text.done\"}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	// Feature flag enabled
	c := &client{
		httpClient:                    server.Client(),
		logger:                        zap.NewNop(),
		enableCopilotResponsesRouting: true,
	}

	decision := domainprovider.RoutingDecision{
		ProviderID: domainprovider.ProviderCopilot,
		BaseURL:    server.URL,
		Token:      "copilot-token",
	}

	request := domaincompletion.ChatCompletionRequest{
		Model:    "gpt-5",
		Messages: []domaincompletion.Message{{Role: "user", Content: "hello"}},
	}

	chunks, err := c.chatCompletionStreamCopilot(t.Context(), decision, request)
	if err != nil {
		t.Fatalf("chatCompletionStreamCopilot failed: %v", err)
	}

	seenContent := false
	seenDone := false
	for chunk := range chunks {
		if chunk.Error != nil {
			t.Fatalf("unexpected stream error: %v", chunk.Error)
		}
		if chunk.Done {
			seenDone = true
		}
		if len(chunk.Data) > 0 {
			seenContent = true
		}
	}

	if gotPath != "/responses" {
		t.Fatalf("with flag enabled, GPT-5 streaming should use /responses, got %q", gotPath)
	}
	if !seenContent {
		t.Fatalf("expected to see content chunks")
	}
	if !seenDone {
		t.Fatalf("expected done chunk")
	}
}

func TestCopilotHeaders_OpenaiIntent(t *testing.T) {
	// Verify Openai-Intent header is set to conversation-edits
	var gotIntent string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotIntent = r.Header.Get("Openai-Intent")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "chatcmpl-test",
			"object":  "chat.completion",
			"created": 1,
			"model":   "gpt-4o",
			"choices": []map[string]any{{
				"index": 0,
				"message": map[string]any{
					"role":    "assistant",
					"content": "ok",
				},
				"finish_reason": "stop",
			}},
		})
	}))
	defer server.Close()

	c := &client{httpClient: server.Client(), logger: zap.NewNop()}

	decision := domainprovider.RoutingDecision{
		ProviderID: domainprovider.ProviderCopilot,
		BaseURL:    server.URL,
		Token:      "copilot-token",
	}

	request := domaincompletion.ChatCompletionRequest{
		Model:    "gpt-4o",
		Messages: []domaincompletion.Message{{Role: "user", Content: "hello"}},
	}

	_, err := c.chatCompletionCopilot(t.Context(), decision, request)
	if err != nil {
		t.Fatalf("chatCompletionCopilot failed: %v", err)
	}

	if gotIntent != "conversation-edits" {
		t.Fatalf("expected Openai-Intent header to be 'conversation-edits', got %q", gotIntent)
	}
}

func TestChatCompletionCopilot_ResponsesRouting_DirectJSON(t *testing.T) {
	// Test that non-stream /responses handles direct JSON response (not SSE-wrapped)
	var gotPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		// Direct JSON response without SSE wrapper (opencode-like behavior)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":    "resp-direct-json",
			"model": "gpt-5.3-codex",
			"output": []map[string]any{
				{
					"type": "message",
					"role": "assistant",
					"content": []map[string]any{
						{"type": "output_text", "text": "Hello from direct JSON"},
					},
				},
			},
			"usage": map[string]any{
				"input_tokens":  10,
				"output_tokens": 5,
				"total_tokens":  15,
			},
		})
	}))
	defer server.Close()

	c := &client{
		httpClient:                    server.Client(),
		logger:                        zap.NewNop(),
		enableCopilotResponsesRouting: true,
	}

	decision := domainprovider.RoutingDecision{
		ProviderID: domainprovider.ProviderCopilot,
		BaseURL:    server.URL,
		Token:      "copilot-token",
	}

	request := domaincompletion.ChatCompletionRequest{
		Model:    "gpt-5.3-codex",
		Messages: []domaincompletion.Message{{Role: "user", Content: "hello"}},
	}

	resp, err := c.chatCompletionCopilot(t.Context(), decision, request)
	if err != nil {
		t.Fatalf("chatCompletionCopilot failed: %v", err)
	}

	if gotPath != "/responses" {
		t.Fatalf("gpt-5.3-codex should use /responses, got %q", gotPath)
	}

	if len(resp.Choices) == 0 {
		t.Fatalf("expected at least one choice")
	}

	msg := resp.Choices[0].Message
	if msg == nil {
		t.Fatalf("expected message in choice")
	}

	content, ok := msg.Content.(string)
	if !ok {
		t.Fatalf("expected string content, got %T", msg.Content)
	}

	if content != "Hello from direct JSON" {
		t.Fatalf("unexpected content: %q", content)
	}

	if resp.Usage == nil {
		t.Fatalf("expected usage in response")
	}
	if resp.Usage.TotalTokens != 15 {
		t.Fatalf("unexpected total tokens: %d", resp.Usage.TotalTokens)
	}
}

func TestChatCompletionCopilot_ResponsesRouting_SSEFallback(t *testing.T) {
	// Test that non-stream /responses still handles SSE-wrapped response.completed
	var gotPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "text/event-stream")
		// SSE-wrapped response (some implementations may still use this for non-stream)
		_, _ = w.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp-sse\",\"model\":\"gpt-5.3-codex\",\"output\":[{\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"Hello from SSE\"}]}]}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	c := &client{
		httpClient:                    server.Client(),
		logger:                        zap.NewNop(),
		enableCopilotResponsesRouting: true,
	}

	decision := domainprovider.RoutingDecision{
		ProviderID: domainprovider.ProviderCopilot,
		BaseURL:    server.URL,
		Token:      "copilot-token",
	}

	request := domaincompletion.ChatCompletionRequest{
		Model:    "gpt-5.3-codex",
		Messages: []domaincompletion.Message{{Role: "user", Content: "hello"}},
	}

	resp, err := c.chatCompletionCopilot(t.Context(), decision, request)
	if err != nil {
		t.Fatalf("chatCompletionCopilot failed: %v", err)
	}

	if gotPath != "/responses" {
		t.Fatalf("gpt-5.3-codex should use /responses, got %q", gotPath)
	}

	if len(resp.Choices) == 0 {
		t.Fatalf("expected at least one choice")
	}

	msg := resp.Choices[0].Message
	if msg == nil {
		t.Fatalf("expected message in choice")
	}

	content, ok := msg.Content.(string)
	if !ok {
		t.Fatalf("expected string content, got %T", msg.Content)
	}

	if content != "Hello from SSE" {
		t.Fatalf("unexpected content: %q", content)
	}
}

func TestExtractResponsesPayload_DirectJSON(t *testing.T) {
	// Test extractResponsesPayload with direct JSON response
	directJSON := []byte(`{"id":"resp-1","model":"gpt-5","output":[{"type":"message"}]}`)

	payload, err := extractResponsesPayload(directJSON)
	if err != nil {
		t.Fatalf("extractResponsesPayload failed: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(payload, &parsed); err != nil {
		t.Fatalf("failed to parse extracted payload: %v", err)
	}

	if parsed["id"] != "resp-1" {
		t.Fatalf("unexpected id: %v", parsed["id"])
	}
}

func TestExtractResponsesPayload_SSE(t *testing.T) {
	// Test extractResponsesPayload with SSE-wrapped response
	sseBody := []byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"Hello\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp-2\"}}\n\ndata: [DONE]\n\n")

	payload, err := extractResponsesPayload(sseBody)
	if err != nil {
		t.Fatalf("extractResponsesPayload failed: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(payload, &parsed); err != nil {
		t.Fatalf("failed to parse extracted payload: %v", err)
	}

	if parsed["type"] != "response.completed" {
		t.Fatalf("expected response.completed event, got type: %v", parsed["type"])
	}
}

func TestExtractResponsesPayload_InvalidBody(t *testing.T) {
	// Test extractResponsesPayload with invalid body
	invalidBody := []byte("not valid json or sse")

	_, err := extractResponsesPayload(invalidBody)
	if err == nil {
		t.Fatalf("expected error for invalid body")
	}
}

func TestExtractSSEData(t *testing.T) {
	tests := []struct {
		name       string
		eventBlock []byte
		want       []byte
	}{
		{
			name:       "simple data line",
			eventBlock: []byte("data: {\"type\":\"test\"}"),
			want:       []byte("{\"type\":\"test\"}"),
		},
		{
			name:       "data line without space after colon",
			eventBlock: []byte("data:{\"type\":\"test\"}"),
			want:       []byte("{\"type\":\"test\"}"),
		},
		{
			name:       "event + data lines (Copilot /responses format)",
			eventBlock: []byte("event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"Hello\"}"),
			want:       []byte("{\"type\":\"response.output_text.delta\",\"delta\":\"Hello\"}"),
		},
		{
			name:       "id + data lines",
			eventBlock: []byte("id: abc123\ndata: {\"message\":\"test\"}"),
			want:       []byte("{\"message\":\"test\"}"),
		},
		{
			name:       "retry + data lines",
			eventBlock: []byte("retry: 1500\ndata: {\"message\":\"test\"}"),
			want:       []byte("{\"message\":\"test\"}"),
		},
		{
			name:       "event + id + data lines",
			eventBlock: []byte("event: response.created\nid: resp-123\ndata: {\"type\":\"response.created\"}"),
			want:       []byte("{\"type\":\"response.created\"}"),
		},
		{
			name:       "DONE marker",
			eventBlock: []byte("data: [DONE]"),
			want:       []byte("[DONE]"),
		},
		{
			name:       "empty event block",
			eventBlock: []byte(""),
			want:       nil,
		},
		{
			name:       "only event line without data",
			eventBlock: []byte("event: ping"),
			want:       nil,
		},
		{
			name:       "multi-line data (SSE spec allows this)",
			eventBlock: []byte("data: line1\ndata: line2"),
			want:       []byte("line1\nline2"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractSSEData(tt.eventBlock)
			if string(got) != string(tt.want) {
				t.Errorf("extractSSEData() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestChatCompletionStreamCopilotResponses_RawPassthrough(t *testing.T) {
	// Test that streaming forwards upstream event payloads as-is (raw passthrough)
	// without transformation - preserving all event types including usage
	var gotPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "text/event-stream")

		// Simulate real Copilot /responses format with event: prefix
		_, _ = w.Write([]byte("event: response.created\ndata: {\"type\":\"response.created\",\"response\":{\"id\":\"resp-1\",\"model\":\"gpt-5\"}}\n\n"))
		_, _ = w.Write([]byte("event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"item_id\":\"item-1\",\"delta\":\"Hello \"}\n\n"))
		_, _ = w.Write([]byte("event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"item_id\":\"item-1\",\"delta\":\"World!\"}\n\n"))
		_, _ = w.Write([]byte("event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp-1\",\"usage\":{\"input_tokens\":10,\"output_tokens\":5,\"total_tokens\":15}}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	// Feature flag enabled
	c := &client{
		httpClient:                    server.Client(),
		logger:                        zap.NewNop(),
		enableCopilotResponsesRouting: true,
	}

	decision := domainprovider.RoutingDecision{
		ProviderID: domainprovider.ProviderCopilot,
		BaseURL:    server.URL,
		Token:      "copilot-token",
	}

	request := domaincompletion.ChatCompletionRequest{
		Model:    "gpt-5.3-codex",
		Messages: []domaincompletion.Message{{Role: "user", Content: "hello"}},
	}

	chunks, err := c.chatCompletionStreamCopilot(t.Context(), decision, request)
	if err != nil {
		t.Fatalf("chatCompletionStreamCopilot failed: %v", err)
	}

	var receivedEvents []map[string]any
	seenDone := false
	for chunk := range chunks {
		if chunk.Error != nil {
			t.Fatalf("unexpected stream error: %v", chunk.Error)
		}
		if chunk.Done {
			seenDone = true
			continue
		}
		if len(chunk.Data) > 0 {
			var parsed map[string]any
			if err := json.Unmarshal(chunk.Data, &parsed); err != nil {
				t.Fatalf("failed to parse chunk data: %v", err)
			}
			receivedEvents = append(receivedEvents, parsed)
		}
	}

	if gotPath != "/responses" {
		t.Fatalf("expected /responses path, got %q", gotPath)
	}

	if !seenDone {
		t.Fatalf("expected done chunk")
	}

	// Verify we received all 4 events (response.created, 2x delta, response.completed)
	if len(receivedEvents) != 4 {
		t.Fatalf("expected 4 events, got %d", len(receivedEvents))
	}

	// Verify event types are preserved (raw passthrough, not transformed)
	expectedTypes := []string{
		"response.created",
		"response.output_text.delta",
		"response.output_text.delta",
		"response.completed",
	}
	for i, evt := range receivedEvents {
		gotType, _ := evt["type"].(string)
		if gotType != expectedTypes[i] {
			t.Errorf("event %d: expected type %q, got %q", i, expectedTypes[i], gotType)
		}
	}

	// Verify delta content is preserved
	delta1, _ := receivedEvents[1]["delta"].(string)
	delta2, _ := receivedEvents[2]["delta"].(string)
	if delta1 != "Hello " {
		t.Errorf("expected delta1 to be 'Hello ', got %q", delta1)
	}
	if delta2 != "World!" {
		t.Errorf("expected delta2 to be 'World!', got %q", delta2)
	}
}

func TestChatCompletionStreamCopilotResponses_UsageInCompletedEvent(t *testing.T) {
	// Test that usage information is preserved in the response.completed event
	// (not stripped during streaming)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"Hi\"}\n\n"))
		_, _ = w.Write([]byte("event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp-usage\",\"usage\":{\"input_tokens\":100,\"output_tokens\":50,\"total_tokens\":150}}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	c := &client{
		httpClient:                    server.Client(),
		logger:                        zap.NewNop(),
		enableCopilotResponsesRouting: true,
	}

	decision := domainprovider.RoutingDecision{
		ProviderID: domainprovider.ProviderCopilot,
		BaseURL:    server.URL,
		Token:      "copilot-token",
	}

	request := domaincompletion.ChatCompletionRequest{
		Model:    "gpt-5",
		Messages: []domaincompletion.Message{{Role: "user", Content: "hello"}},
	}

	chunks, err := c.chatCompletionStreamCopilot(t.Context(), decision, request)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}

	var completedEvent map[string]any
	for chunk := range chunks {
		if chunk.Error != nil {
			t.Fatalf("unexpected error: %v", chunk.Error)
		}
		if chunk.Done {
			continue
		}
		if len(chunk.Data) > 0 {
			var parsed map[string]any
			if err := json.Unmarshal(chunk.Data, &parsed); err != nil {
				continue
			}
			if parsed["type"] == "response.completed" {
				completedEvent = parsed
			}
		}
	}

	if completedEvent == nil {
		t.Fatalf("expected to receive response.completed event")
	}

	// Verify usage is present in the completed event
	response, ok := completedEvent["response"].(map[string]any)
	if !ok {
		t.Fatalf("expected response object in completed event")
	}

	usage, ok := response["usage"].(map[string]any)
	if !ok {
		t.Fatalf("expected usage in response.completed event")
	}

	inputTokens, _ := usage["input_tokens"].(float64)
	outputTokens, _ := usage["output_tokens"].(float64)
	totalTokens, _ := usage["total_tokens"].(float64)

	if inputTokens != 100 {
		t.Errorf("expected input_tokens=100, got %v", inputTokens)
	}
	if outputTokens != 50 {
		t.Errorf("expected output_tokens=50, got %v", outputTokens)
	}
	if totalTokens != 150 {
		t.Errorf("expected total_tokens=150, got %v", totalTokens)
	}
}

func TestChatCompletionStreamCopilotResponses_AllEventTypesForwarded(t *testing.T) {
	// Test that ALL upstream event types are forwarded, not just known ones
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		// Include various event types including some that might be future/unknown
		_, _ = w.Write([]byte("event: response.created\ndata: {\"type\":\"response.created\",\"response\":{\"id\":\"resp-1\"}}\n\n"))
		_, _ = w.Write([]byte("event: response.in_progress\ndata: {\"type\":\"response.in_progress\"}\n\n"))
		_, _ = w.Write([]byte("event: response.output_item.added\ndata: {\"type\":\"response.output_item.added\",\"item\":{\"id\":\"item-1\"}}\n\n"))
		_, _ = w.Write([]byte("event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"test\"}\n\n"))
		_, _ = w.Write([]byte("event: response.output_item.done\ndata: {\"type\":\"response.output_item.done\",\"item\":{\"id\":\"item-1\"}}\n\n"))
		_, _ = w.Write([]byte("event: response.function_call.arguments.delta\ndata: {\"type\":\"response.function_call.arguments.delta\",\"call_id\":\"call-1\",\"delta\":\"{\\\"arg\\\":1}\"}\n\n"))
		_, _ = w.Write([]byte("event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp-1\"}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	c := &client{
		httpClient:                    server.Client(),
		logger:                        zap.NewNop(),
		enableCopilotResponsesRouting: true,
	}

	decision := domainprovider.RoutingDecision{
		ProviderID: domainprovider.ProviderCopilot,
		BaseURL:    server.URL,
		Token:      "copilot-token",
	}

	request := domaincompletion.ChatCompletionRequest{
		Model:    "gpt-5",
		Messages: []domaincompletion.Message{{Role: "user", Content: "hello"}},
	}

	chunks, err := c.chatCompletionStreamCopilot(t.Context(), decision, request)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}

	var eventTypes []string
	for chunk := range chunks {
		if chunk.Error != nil {
			t.Fatalf("unexpected error: %v", chunk.Error)
		}
		if chunk.Done {
			continue
		}
		if len(chunk.Data) > 0 {
			var parsed map[string]any
			if err := json.Unmarshal(chunk.Data, &parsed); err != nil {
				continue
			}
			if eventType, ok := parsed["type"].(string); ok {
				eventTypes = append(eventTypes, eventType)
			}
		}
	}

	// All 7 event types should be forwarded
	expected := []string{
		"response.created",
		"response.in_progress",
		"response.output_item.added",
		"response.output_text.delta",
		"response.output_item.done",
		"response.function_call.arguments.delta",
		"response.completed",
	}

	if len(eventTypes) != len(expected) {
		t.Fatalf("expected %d events, got %d: %v", len(expected), len(eventTypes), eventTypes)
	}

	for i, et := range expected {
		if eventTypes[i] != et {
			t.Errorf("event %d: expected %q, got %q", i, et, eventTypes[i])
		}
	}
}
