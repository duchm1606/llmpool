package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/duchoang/llmpool/internal/domain/anthropic"
	domainprovider "github.com/duchoang/llmpool/internal/domain/provider"
	providerinfra "github.com/duchoang/llmpool/internal/infra/provider"
	"github.com/duchoang/llmpool/internal/translator"
	usecasecompletion "github.com/duchoang/llmpool/internal/usecase/completion"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Constants for Copilot API
const (
	copilotChatVersionMsg         = "0.26.7"
	copilotEditorPluginVersionMsg = "copilot-chat/" + copilotChatVersionMsg
	copilotUserAgentMsg           = "GitHubCopilotChat/" + copilotChatVersionMsg
	defaultVSCodeVersionMsg       = "1.104.3"
	copilotGitHubAPIVersionMsg    = "2025-04-01"
)

// MessagesHandler handles Anthropic Claude Messages API requests.
// It proxies requests to GitHub Copilot's /responses endpoint while providing
// an Anthropic-compatible interface.
type MessagesHandler struct {
	router     usecasecompletion.Router
	registry   usecasecompletion.ProviderRegistry
	httpClient *http.Client
	logger     *zap.Logger
}

// NewMessagesHandler creates a new messages handler.
func NewMessagesHandler(
	router usecasecompletion.Router,
	registry usecasecompletion.ProviderRegistry,
	logger *zap.Logger,
) *MessagesHandler {
	return &MessagesHandler{
		router:     router,
		registry:   registry,
		httpClient: &http.Client{},
		logger:     logger,
	}
}

// CreateMessage handles POST /v1/messages
// This endpoint accepts Anthropic Claude format and proxies to Copilot /responses.
func (h *MessagesHandler) CreateMessage(c *gin.Context) {
	// Read raw body
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		h.respondError(c, http.StatusBadRequest, "invalid_request_error", "failed to read request body")
		return
	}

	// Parse Anthropic Messages request
	var req anthropic.MessagesRequest
	if err := json.Unmarshal(body, &req); err != nil {
		h.respondError(c, http.StatusBadRequest, "invalid_request_error", "failed to parse request: "+err.Error())
		return
	}

	h.logger.Info("messages request received",
		zap.String("model", req.Model),
		zap.Bool("stream", req.Stream),
		zap.Int("messages", len(req.Messages)),
		zap.Int("max_tokens", req.MaxTokens),
	)

	// Route to Copilot provider
	ctx := c.Request.Context()
	decision, err := h.router.RouteWithHint(ctx, req.Model, "copilot", nil)
	if err != nil {
		h.logger.Error("routing failed", zap.Error(err))
		h.respondError(c, http.StatusServiceUnavailable, "api_error", "no available provider: "+err.Error())
		return
	}

	// Determine which endpoint to use based on model
	useResponsesAPI := providerinfra.ShouldUseCopilotResponsesAPI(req.Model)

	var copilotBody []byte
	var endpoint string

	if useResponsesAPI {
		// GPT-5+ models (except gpt-5-mini) use /responses endpoint
		copilotBody, err = translator.AnthropicToCopilotResponses(&req)
		endpoint = "/responses"
	} else {
		// Claude models and others use /chat/completions endpoint
		copilotBody, err = translator.AnthropicToChatCompletion(&req)
		endpoint = "/chat/completions"
	}

	if err != nil {
		h.respondError(c, http.StatusBadRequest, "invalid_request_error", "failed to convert request: "+err.Error())
		return
	}

	h.logger.Debug("converted request",
		zap.String("copilot_body", truncateStr(string(copilotBody), 500)),
		zap.String("endpoint", endpoint),
		zap.Bool("use_responses_api", useResponsesAPI),
	)

	// Build Copilot request URL
	url := strings.TrimSuffix(decision.BaseURL, "/") + endpoint

	if req.Stream {
		h.handleStreaming(c, ctx, url, copilotBody, decision, req.Model, useResponsesAPI)
	} else {
		h.handleNonStreaming(c, ctx, url, copilotBody, decision, req.Model, useResponsesAPI)
	}
}

// handleNonStreaming handles non-streaming messages requests.
func (h *MessagesHandler) handleNonStreaming(
	c *gin.Context,
	ctx context.Context,
	url string,
	body []byte,
	decision *domainprovider.RoutingDecision,
	model string,
	useResponsesAPI bool,
) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		h.respondError(c, http.StatusInternalServerError, "api_error", "failed to create request")
		return
	}

	h.applyCopilotHeaders(httpReq, decision)

	h.logger.Info("sending non-streaming request to copilot",
		zap.String("url", url),
		zap.String("model", model),
	)

	resp, err := h.httpClient.Do(httpReq)
	if err != nil {
		h.respondError(c, http.StatusBadGateway, "api_error", "upstream request failed: "+err.Error())
		return
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		h.respondError(c, http.StatusBadGateway, "api_error", "failed to read upstream response")
		return
	}

	if resp.StatusCode >= 400 {
		h.logger.Warn("copilot upstream error",
			zap.Int("status", resp.StatusCode),
			zap.String("body", truncateStr(string(respBody), 500)),
		)
		h.respondError(c, resp.StatusCode, "api_error", string(respBody))
		return
	}

	// Convert Copilot response to Anthropic format
	var anthropicResp *anthropic.MessagesResponse
	if useResponsesAPI {
		anthropicResp, err = h.convertResponsesAPIToAnthropic(respBody, model)
	} else {
		anthropicResp, err = h.convertChatCompletionToAnthropic(respBody, model)
	}
	if err != nil {
		h.respondError(c, http.StatusInternalServerError, "api_error", "failed to convert response: "+err.Error())
		return
	}

	c.JSON(http.StatusOK, anthropicResp)
}

// handleStreaming handles streaming messages requests.
func (h *MessagesHandler) handleStreaming(
	c *gin.Context,
	ctx context.Context,
	url string,
	body []byte,
	decision *domainprovider.RoutingDecision,
	model string,
	useResponsesAPI bool,
) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		h.respondError(c, http.StatusInternalServerError, "api_error", "failed to create request")
		return
	}

	h.applyCopilotHeaders(httpReq, decision)
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("Cache-Control", "no-cache")
	httpReq.Header.Set("Connection", "keep-alive")

	h.logger.Info("sending streaming request to copilot",
		zap.String("url", url),
		zap.String("model", model),
	)

	resp, err := h.httpClient.Do(httpReq)
	if err != nil {
		h.respondError(c, http.StatusBadGateway, "api_error", "upstream request failed: "+err.Error())
		return
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		h.logger.Warn("copilot streaming upstream error",
			zap.Int("status", resp.StatusCode),
			zap.String("body", truncateStr(string(respBody), 500)),
		)
		h.respondError(c, resp.StatusCode, "api_error", string(respBody))
		return
	}

	// Set SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Transfer-Encoding", "chunked")
	c.Header("X-Accel-Buffering", "no")

	writer := c.Writer
	writer.Flush()

	// Create stream state for conversion
	streamState := translator.NewCopilotToAnthropicStreamState(model)

	// Read and transform SSE events
	buf := make([]byte, 4096)
	var partial []byte

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			partial = append(partial, buf[:n]...)

			// Process complete SSE events
			for {
				idx := bytes.Index(partial, []byte("\n\n"))
				if idx == -1 {
					break
				}

				eventBlock := partial[:idx]
				partial = partial[idx+2:]

				// Extract data from SSE event block
				data := extractSSEDataFromBlock(eventBlock)
				if data == nil {
					continue
				}

				if bytes.Equal(data, []byte("[DONE]")) {
					// Emit final events
					for _, evt := range streamState.Finalize() {
						_, _ = writer.WriteString(evt)
					}
					writer.Flush()
					return
				}

				// Convert Copilot event to Anthropic events
				var anthropicEvents []string
				if useResponsesAPI {
					anthropicEvents = streamState.ConvertCopilotEventToAnthropic(data)
				} else {
					anthropicEvents = streamState.ConvertChatCompletionEventToAnthropic(data)
				}
				for _, evt := range anthropicEvents {
					if _, err := writer.WriteString(evt); err != nil {
						h.logger.Error("failed to write SSE event", zap.Error(err))
						return
					}
				}
				writer.Flush()
			}
		}

		if readErr != nil {
			if readErr == io.EOF || errors.Is(readErr, context.Canceled) {
				// Emit final events
				for _, evt := range streamState.Finalize() {
					_, _ = writer.WriteString(evt)
				}
				writer.Flush()
			}
			return
		}
	}
}

// extractSSEDataFromBlock extracts the data payload from an SSE event block.
func extractSSEDataFromBlock(eventBlock []byte) []byte {
	lines := bytes.Split(eventBlock, []byte("\n"))
	var dataLines [][]byte

	for _, line := range lines {
		line = bytes.TrimSpace(line)
		if bytes.HasPrefix(line, []byte("data:")) {
			payload := bytes.TrimPrefix(line, []byte("data:"))
			payload = bytes.TrimSpace(payload)
			dataLines = append(dataLines, payload)
		}
	}

	if len(dataLines) == 0 {
		return nil
	}

	return bytes.Join(dataLines, []byte("\n"))
}

// applyCopilotHeaders applies Copilot-specific headers to the HTTP request.
// Always sets X-Initiator: agent for all requests.
func (h *MessagesHandler) applyCopilotHeaders(req *http.Request, decision *domainprovider.RoutingDecision) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+decision.Token)
	req.Header.Set("Copilot-Integration-Id", "vscode-chat")
	req.Header.Set("Editor-Version", "vscode/"+defaultVSCodeVersionMsg)
	req.Header.Set("Editor-Plugin-Version", copilotEditorPluginVersionMsg)
	req.Header.Set("User-Agent", copilotUserAgentMsg)
	req.Header.Set("Openai-Intent", "conversation-edits")
	req.Header.Set("X-Github-Api-Version", copilotGitHubAPIVersionMsg)
	req.Header.Set("X-Request-Id", uuid.New().String())
	req.Header.Set("X-Vscode-User-Agent-Library-Version", "electron-fetch")

	// Always set X-Initiator to "agent" for all requests
	req.Header.Set("X-Initiator", "agent")

	// Apply any additional headers from routing decision
	for k, v := range decision.Headers {
		req.Header.Set(k, v)
	}
}

// convertResponsesAPIToAnthropic converts a Copilot Responses API response to Anthropic format.
func (h *MessagesHandler) convertResponsesAPIToAnthropic(respBody []byte, model string) (*anthropic.MessagesResponse, error) {
	// Extract the actual response payload (may be wrapped in SSE or response.completed)
	payload, err := extractResponsePayload(respBody)
	if err != nil {
		return nil, err
	}

	var root map[string]any
	if err := json.Unmarshal(payload, &root); err != nil {
		return nil, fmt.Errorf("parse response payload: %w", err)
	}

	// Handle response.completed wrapper
	responseObj := root
	if t, _ := root["type"].(string); t == "response.completed" {
		if nested, ok := root["response"].(map[string]any); ok {
			responseObj = nested
		}
	} else if nested, ok := root["response"].(map[string]any); ok {
		responseObj = nested
	}

	// Extract ID
	id, _ := responseObj["id"].(string)
	if id == "" {
		id = fmt.Sprintf("msg_%s", uuid.New().String()[:24])
	}

	// Build content array
	content := make([]anthropic.ResponseContent, 0)
	stopReason := "end_turn"

	if output, ok := responseObj["output"].([]any); ok {
		for _, itemRaw := range output {
			item, ok := itemRaw.(map[string]any)
			if !ok {
				continue
			}

			itemType, _ := item["type"].(string)
			switch itemType {
			case "message":
				// Extract text content
				if contentArr, ok := item["content"].([]any); ok {
					for _, partRaw := range contentArr {
						part, ok := partRaw.(map[string]any)
						if !ok {
							continue
						}
						partType, _ := part["type"].(string)
						if partType == "output_text" || partType == "text" {
							text, _ := part["text"].(string)
							content = append(content, anthropic.ResponseContent{
								Type: "text",
								Text: text,
							})
						}
					}
				}

			case "function_call":
				// Tool use
				callID, _ := item["call_id"].(string)
				if callID == "" {
					callID, _ = item["id"].(string)
				}
				name, _ := item["name"].(string)
				args, _ := item["arguments"].(string)

				var input any
				if args != "" {
					_ = json.Unmarshal([]byte(args), &input)
				}
				if input == nil {
					input = map[string]any{}
				}

				content = append(content, anthropic.ResponseContent{
					Type:  "tool_use",
					ID:    callID,
					Name:  name,
					Input: input,
				})
				stopReason = "tool_use"
			}
		}
	}

	// Extract usage
	var usage *anthropic.Usage
	if usageMap, ok := responseObj["usage"].(map[string]any); ok {
		inputTokens, _ := usageMap["input_tokens"].(float64)
		outputTokens, _ := usageMap["output_tokens"].(float64)
		usage = &anthropic.Usage{
			InputTokens:  int(inputTokens),
			OutputTokens: int(outputTokens),
		}
	}

	return &anthropic.MessagesResponse{
		ID:         id,
		Type:       "message",
		Role:       "assistant",
		Model:      model,
		Content:    content,
		StopReason: stopReason,
		Usage:      usage,
	}, nil
}

// extractResponsePayload extracts the response payload from SSE or direct JSON.
func extractResponsePayload(body []byte) ([]byte, error) {
	trimmed := bytes.TrimSpace(body)

	// Try direct JSON first
	if len(trimmed) > 0 && trimmed[0] == '{' {
		var root map[string]any
		if err := json.Unmarshal(trimmed, &root); err == nil {
			if _, hasID := root["id"]; hasID {
				return trimmed, nil
			}
			if _, hasOutput := root["output"]; hasOutput {
				return trimmed, nil
			}
			if t, _ := root["type"].(string); t == "response.completed" {
				return trimmed, nil
			}
		}
	}

	// Parse SSE events
	lines := bytes.Split(body, []byte("\n"))
	for _, line := range lines {
		line = bytes.TrimSpace(line)
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		payload := bytes.TrimSpace(line[len("data:"):])
		if bytes.Equal(payload, []byte("[DONE]")) {
			continue
		}
		var root map[string]any
		if err := json.Unmarshal(payload, &root); err != nil {
			continue
		}
		if t, _ := root["type"].(string); t == "response.completed" {
			return payload, nil
		}
	}

	return nil, fmt.Errorf("no valid response payload found")
}

// convertChatCompletionToAnthropic converts an OpenAI Chat Completion response to Anthropic format.
func (h *MessagesHandler) convertChatCompletionToAnthropic(respBody []byte, model string) (*anthropic.MessagesResponse, error) {
	var chatResp struct {
		ID      string `json:"id"`
		Model   string `json:"model"`
		Choices []struct {
			Index   int `json:"index"`
			Message struct {
				Role      string `json:"role"`
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage *struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}

	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, fmt.Errorf("parse chat completion response: %w", err)
	}

	// Use response ID or generate one
	id := chatResp.ID
	if id == "" {
		id = fmt.Sprintf("msg_%s", uuid.New().String()[:24])
	}

	// Build content array
	content := make([]anthropic.ResponseContent, 0)
	stopReason := "end_turn"

	if len(chatResp.Choices) > 0 {
		choice := chatResp.Choices[0]

		// Add text content if present
		if choice.Message.Content != "" {
			content = append(content, anthropic.ResponseContent{
				Type: "text",
				Text: choice.Message.Content,
			})
		}

		// Add tool calls if present
		for _, tc := range choice.Message.ToolCalls {
			var input any
			if tc.Function.Arguments != "" {
				_ = json.Unmarshal([]byte(tc.Function.Arguments), &input)
			}
			if input == nil {
				input = map[string]any{}
			}

			content = append(content, anthropic.ResponseContent{
				Type:  "tool_use",
				ID:    tc.ID,
				Name:  tc.Function.Name,
				Input: input,
			})
			stopReason = "tool_use"
		}

		// Convert finish_reason to Anthropic stop_reason
		if stopReason != "tool_use" {
			switch choice.FinishReason {
			case "stop":
				stopReason = "end_turn"
			case "length":
				stopReason = "max_tokens"
			case "tool_calls":
				stopReason = "tool_use"
			case "content_filter":
				stopReason = "end_turn"
			default:
				stopReason = "end_turn"
			}
		}
	}

	// Convert usage
	var usage *anthropic.Usage
	if chatResp.Usage != nil {
		usage = &anthropic.Usage{
			InputTokens:  chatResp.Usage.PromptTokens,
			OutputTokens: chatResp.Usage.CompletionTokens,
		}
	}

	return &anthropic.MessagesResponse{
		ID:         id,
		Type:       "message",
		Role:       "assistant",
		Model:      model,
		Content:    content,
		StopReason: stopReason,
		Usage:      usage,
	}, nil
}

// respondError writes an Anthropic-compatible error response.
func (h *MessagesHandler) respondError(c *gin.Context, status int, errType, message string) {
	c.JSON(status, map[string]any{
		"type": "error",
		"error": map[string]any{
			"type":    errType,
			"message": message,
		},
	})
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// HealthCheck handles GET /v1/messages/health (optional)
func (h *MessagesHandler) HealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
