package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	domaincompletion "github.com/duchoang/llmpool/internal/domain/completion"
	usecasecompletion "github.com/duchoang/llmpool/internal/usecase/completion"
)

// mockCompletionService is a mock implementation of CompletionService for testing.
type mockCompletionService struct {
	lastRequest domaincompletion.ChatCompletionRequest
	response    *domaincompletion.ChatCompletionResponse
	err         error
}

func (m *mockCompletionService) ChatCompletion(
	_ context.Context,
	req domaincompletion.ChatCompletionRequest,
) (*domaincompletion.ChatCompletionResponse, error) {
	m.lastRequest = req
	return m.response, m.err
}

func (m *mockCompletionService) ValidateRequest(_ context.Context, req domaincompletion.ChatCompletionRequest) error {
	m.lastRequest = req
	return m.err
}

func (m *mockCompletionService) ChatCompletionStream(
	_ context.Context,
	req domaincompletion.ChatCompletionRequest,
	_ io.Writer,
) error {
	m.lastRequest = req
	return m.err
}

func (m *mockCompletionService) ListModels(_ context.Context) (*domaincompletion.ModelsResponse, error) {
	return nil, nil
}

func (m *mockCompletionService) SetUsagePublisher(_ usecasecompletion.UsagePublisher) {
	// no-op for mock
}

// Compile-time check that mockCompletionService implements the interface
var _ usecasecompletion.CompletionService = (*mockCompletionService)(nil)

func TestChatHandler_StreamBehavior(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name         string
		body         map[string]any
		expectStream bool
	}{
		{
			name: "stream true triggers streaming",
			body: map[string]any{
				"model":    "gpt-4",
				"messages": []map[string]any{{"role": "user", "content": "hello"}},
				"stream":   true,
			},
			expectStream: true,
		},
		{
			name: "streaming alias does NOT trigger streaming (removed support)",
			body: map[string]any{
				"model":     "gpt-4",
				"messages":  []map[string]any{{"role": "user", "content": "hello"}},
				"streaming": true,
			},
			expectStream: false, // streaming alias no longer supported
		},
		{
			name: "no stream uses non-streaming",
			body: map[string]any{
				"model":    "gpt-4",
				"messages": []map[string]any{{"role": "user", "content": "hello"}},
			},
			expectStream: false,
		},
		{
			name: "stream false uses non-streaming regardless of streaming alias",
			body: map[string]any{
				"model":     "gpt-4",
				"messages":  []map[string]any{{"role": "user", "content": "hello"}},
				"stream":    false,
				"streaming": true,
			},
			expectStream: false, // streaming alias no longer supported
		},
		{
			name: "only stream field is honored",
			body: map[string]any{
				"model":    "gpt-4",
				"messages": []map[string]any{{"role": "user", "content": "hello"}},
				"stream":   true,
			},
			expectStream: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockService := &mockCompletionService{
				response: &domaincompletion.ChatCompletionResponse{
					ID:      "chatcmpl-test",
					Object:  "chat.completion",
					Created: 1,
					Model:   "gpt-4",
					Choices: []domaincompletion.Choice{{
						Index:        0,
						Message:      &domaincompletion.Message{Role: "assistant", Content: "ok"},
						FinishReason: "stop",
					}},
				},
			}

			handler := NewChatHandler(mockService, zap.NewNop())

			bodyBytes, _ := json.Marshal(tt.body)

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(bodyBytes))
			c.Request.Header.Set("Content-Type", "application/json")

			handler.ChatCompletion(c)

			// For streaming requests, the handler sets SSE headers
			// For non-streaming, it returns JSON
			if tt.expectStream {
				// Streaming path - check Content-Type is text/event-stream
				contentType := w.Header().Get("Content-Type")
				if contentType != "text/event-stream" {
					t.Errorf("expected text/event-stream Content-Type for streaming, got %q", contentType)
				}
			} else {
				// Non-streaming path - check that the request was handled (status 200)
				if w.Code != http.StatusOK {
					t.Errorf("expected status 200, got %d", w.Code)
				}
			}
		})
	}
}

func TestChatHandler_ProviderPrefix(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mockService := &mockCompletionService{
		response: &domaincompletion.ChatCompletionResponse{
			ID:      "chatcmpl-test",
			Object:  "chat.completion",
			Created: 1,
			Model:   "gpt-5",
			Choices: []domaincompletion.Choice{{
				Index:        0,
				Message:      &domaincompletion.Message{Role: "assistant", Content: "ok"},
				FinishReason: "stop",
			}},
		},
	}

	handler := NewChatHandler(mockService, zap.NewNop())

	body := map[string]any{
		"model":    "copilot/gpt-5.3-codex",
		"messages": []map[string]any{{"role": "user", "content": "hello"}},
	}
	bodyBytes, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(bodyBytes))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.ChatCompletion(c)

	// Check that the model was stripped of the provider prefix
	if mockService.lastRequest.Model != "gpt-5.3-codex" {
		t.Errorf("expected model to be stripped to 'gpt-5.3-codex', got %q", mockService.lastRequest.Model)
	}

	// Check that provider hint was extracted
	if mockService.lastRequest.ProviderHint != "copilot" {
		t.Errorf("expected provider hint to be 'copilot', got %q", mockService.lastRequest.ProviderHint)
	}
}

func TestChatHandler_StreamingValidationErrorReturnsNon200(t *testing.T) {
	gin.SetMode(gin.TestMode)

	modelNotFound := domaincompletion.ErrModelNotFound("claude-4.5-opus-high")
	mockService := &mockCompletionService{err: modelNotFound}
	handler := NewChatHandler(mockService, zap.NewNop())

	body := map[string]any{
		"model":    "claude-4.5-opus-high",
		"messages": []map[string]any{{"role": "user", "content": "hello"}},
		"stream":   true,
	}
	bodyBytes, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(bodyBytes))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.ChatCompletion(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", w.Code)
	}

	if got := w.Header().Get("Content-Type"); got == "text/event-stream" {
		t.Fatalf("expected non-SSE error response content-type, got %q", got)
	}

	var errResp domaincompletion.APIErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if errResp.Error.Message != modelNotFound.Message {
		t.Fatalf("expected error message %q, got %q", modelNotFound.Message, errResp.Error.Message)
	}
}
