package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	domaincompletion "github.com/duchoang/llmpool/internal/domain/completion"
	"github.com/duchoang/llmpool/internal/translator"
	usecasecompletion "github.com/duchoang/llmpool/internal/usecase/completion"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// ResponsesHandler handles OpenAI Responses API requests (used by Cursor IDE).
type ResponsesHandler struct {
	service usecasecompletion.CompletionService
	logger  *zap.Logger
}

// NewResponsesHandler creates a new responses handler.
func NewResponsesHandler(service usecasecompletion.CompletionService, logger *zap.Logger) *ResponsesHandler {
	return &ResponsesHandler{
		service: service,
		logger:  logger,
	}
}

// CreateResponse handles POST /v1/responses
// This endpoint translates OpenAI Responses format to Chat Completions format internally.
func (h *ResponsesHandler) CreateResponse(c *gin.Context) {
	// Read raw body
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		h.respondError(c, domaincompletion.ErrInvalidJSON("failed to read request body"))
		return
	}

	// Check if this is a Codex model and apply transforms
	model := extractModel(body)
	if translator.IsCodexModel(model) {
		h.logger.Debug("applying Codex transforms", zap.String("model", model))
		body = translator.TransformForCodex(body)
	}

	// Parse as Responses request
	var respReq translator.ResponsesRequest
	if err := json.Unmarshal(body, &respReq); err != nil {
		h.respondError(c, domaincompletion.ErrInvalidJSON(err.Error()))
		return
	}

	h.logger.Debug("responses request received",
		zap.String("model", respReq.Model),
		zap.Bool("stream", respReq.Stream),
		zap.String("instructions_preview", truncateString(respReq.Instructions, 100)),
	)

	// Convert to Chat Completions format
	chatReq, err := translator.ConvertResponsesToChat(&respReq)
	if err != nil {
		h.respondError(c, domaincompletion.ErrInvalidJSON("failed to convert request: "+err.Error()))
		return
	}

	// Build domain request
	domainReq := h.buildDomainRequest(chatReq)

	// Handle streaming vs non-streaming
	if domainReq.Stream {
		h.handleStreaming(c, domainReq, body)
		return
	}

	h.handleNonStreaming(c, domainReq, body)
}

// buildDomainRequest converts translator request to domain request.
func (h *ResponsesHandler) buildDomainRequest(chatReq *translator.ChatCompletionsRequest) domaincompletion.ChatCompletionRequest {
	domainReq := domaincompletion.ChatCompletionRequest{
		Model:       chatReq.Model,
		Stream:      chatReq.Stream,
		Temperature: chatReq.Temperature,
		TopP:        chatReq.TopP,
		MaxTokens:   chatReq.MaxTokens,
		User:        chatReq.User,
	}

	// Convert messages
	domainReq.Messages = make([]domaincompletion.Message, 0, len(chatReq.Messages))
	for _, msg := range chatReq.Messages {
		domainMsg := domaincompletion.Message{
			Role:       msg.Role,
			Content:    msg.Content,
			ToolCallID: msg.ToolCallID,
		}
		if len(msg.ToolCalls) > 0 {
			domainMsg.ToolCalls = make([]domaincompletion.ToolCall, 0, len(msg.ToolCalls))
			for _, tc := range msg.ToolCalls {
				domainMsg.ToolCalls = append(domainMsg.ToolCalls, domaincompletion.ToolCall{
					ID:   tc.ID,
					Type: tc.Type,
					Function: domaincompletion.FunctionCall{
						Name:      tc.Function.Name,
						Arguments: tc.Function.Arguments,
					},
				})
			}
		}
		domainReq.Messages = append(domainReq.Messages, domainMsg)
	}

	// Convert tools
	if len(chatReq.Tools) > 0 {
		domainReq.Tools = make([]domaincompletion.Tool, 0, len(chatReq.Tools))
		for _, t := range chatReq.Tools {
			domainReq.Tools = append(domainReq.Tools, domaincompletion.Tool{
				Type: t.Type,
				Function: domaincompletion.Function{
					Name:        t.Function.Name,
					Description: t.Function.Description,
					Parameters:  t.Function.Parameters,
				},
			})
		}
	}

	domainReq.ToolChoice = chatReq.ToolChoice

	return domainReq
}

// handleNonStreaming handles non-streaming responses requests.
func (h *ResponsesHandler) handleNonStreaming(c *gin.Context, req domaincompletion.ChatCompletionRequest, originalRequest []byte) {
	ctx := c.Request.Context()

	resp, err := h.service.ChatCompletion(ctx, req)
	if err != nil {
		var apiErr *domaincompletion.APIError
		if errors.As(err, &apiErr) {
			h.respondError(c, apiErr)
			return
		}
		h.respondError(c, domaincompletion.ErrInternalServer(err.Error()))
		return
	}

	// Convert response to Responses format
	responsesResp := convertToResponsesFormat(resp, originalRequest)
	c.JSON(http.StatusOK, responsesResp)
}

// handleStreaming handles streaming responses requests.
func (h *ResponsesHandler) handleStreaming(c *gin.Context, req domaincompletion.ChatCompletionRequest, originalRequest []byte) {
	ctx := c.Request.Context()

	// Set SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Transfer-Encoding", "chunked")
	c.Header("X-Accel-Buffering", "no")

	writer := c.Writer
	writer.Flush()

	// Create a transformer that intercepts Chat Completions SSE and converts to Responses SSE
	transformer := NewResponsesStreamTransformer(writer, originalRequest, h.logger)

	// Stream completion through transformer
	err := h.service.ChatCompletionStream(ctx, req, transformer)
	if err != nil {
		var apiErr *domaincompletion.APIError
		if errors.As(err, &apiErr) {
			h.logger.Error("streaming error", zap.Error(err))
			// Try to send error as Responses SSE event
			errResp := map[string]any{
				"type":  "error",
				"error": map[string]any{"message": apiErr.Message, "type": apiErr.Type},
			}
			errJSON, _ := json.Marshal(errResp)
			_, _ = writer.WriteString(translator.FormatResponsesSSE("error", string(errJSON)))
			writer.Flush()
		}
	}

	// Ensure completion event was emitted
	transformer.Finalize()
}

// ResponsesStreamTransformer transforms Chat Completions SSE to Responses SSE.
type ResponsesStreamTransformer struct {
	writer      http.Flusher
	ginWriter   gin.ResponseWriter
	streamState *translator.StreamState
	logger      *zap.Logger
	buffer      bytes.Buffer
	finalized   bool
}

// NewResponsesStreamTransformer creates a new stream transformer.
func NewResponsesStreamTransformer(w gin.ResponseWriter, originalRequest []byte, logger *zap.Logger) *ResponsesStreamTransformer {
	return &ResponsesStreamTransformer{
		ginWriter:   w,
		writer:      w,
		streamState: translator.NewStreamState(originalRequest),
		logger:      logger,
	}
}

// Write implements io.Writer, intercepting Chat Completions SSE and converting to Responses SSE.
func (t *ResponsesStreamTransformer) Write(p []byte) (n int, err error) {
	// Accumulate data in buffer
	t.buffer.Write(p)

	// Process complete SSE events from buffer
	for {
		// Look for complete SSE event (ends with \n\n)
		data := t.buffer.String()
		idx := strings.Index(data, "\n\n")
		if idx == -1 {
			break
		}

		event := data[:idx]
		t.buffer.Reset()
		t.buffer.WriteString(data[idx+2:])

		// Parse SSE event
		if strings.HasPrefix(event, "data: ") {
			payload := strings.TrimPrefix(event, "data: ")

			// Handle [DONE]
			if payload == "[DONE]" {
				// Don't emit anything - Finalize will emit response.completed
				continue
			}

			// Parse as Chat Completions chunk
			var chunk translator.ChatCompletionChunk
			if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
				t.logger.Warn("failed to parse chunk", zap.Error(err), zap.String("payload", truncateString(payload, 200)))
				continue
			}

			// Convert to Responses events
			events := t.streamState.ConvertChunkToResponsesEvents(&chunk)
			for _, evt := range events {
				if _, err := t.ginWriter.WriteString(evt); err != nil {
					return len(p), err
				}
			}
			t.writer.Flush()
		}
	}

	return len(p), nil
}

// Finalize emits the response.completed event if not already done.
func (t *ResponsesStreamTransformer) Finalize() {
	if t.finalized {
		return
	}
	t.finalized = true

	// If stream state hasn't emitted completion yet, emit it now
	if t.streamState.Started {
		// Check if we need to emit completion events
		hasUnclosedContent := false
		for idx := range t.streamState.MsgItemAdded {
			if !t.streamState.MsgItemDone[idx] {
				hasUnclosedContent = true
				break
			}
		}
		for _, callID := range t.streamState.FuncCallIDs {
			if callID != "" {
				hasUnclosedContent = true
				break
			}
		}

		if hasUnclosedContent || len(t.streamState.MsgItemAdded) == 0 {
			// Build completion manually
			events := t.streamState.ConvertChunkToResponsesEvents(&translator.ChatCompletionChunk{
				ID:      t.streamState.ResponseID,
				Created: t.streamState.Created,
				Choices: []translator.ChunkChoice{{
					Index:        0,
					FinishReason: "stop",
				}},
			})
			for _, evt := range events {
				_, _ = t.ginWriter.WriteString(evt)
			}
			t.writer.Flush()
		}
	}
}

// convertToResponsesFormat converts a Chat Completions response to Responses format.
func convertToResponsesFormat(resp *domaincompletion.ChatCompletionResponse, originalRequest []byte) map[string]any {
	result := map[string]any{
		"id":                 resp.ID,
		"object":             "response",
		"created_at":         resp.Created,
		"status":             "completed",
		"background":         false,
		"error":              nil,
		"incomplete_details": nil,
	}

	// Echo request fields
	if len(originalRequest) > 0 {
		var req map[string]any
		if err := json.Unmarshal(originalRequest, &req); err == nil {
			if v, ok := req["instructions"]; ok {
				result["instructions"] = v
			}
			if v, ok := req["max_output_tokens"]; ok {
				result["max_output_tokens"] = v
			}
			if v, ok := req["model"]; ok {
				result["model"] = v
			}
			if v, ok := req["temperature"]; ok {
				result["temperature"] = v
			}
			if v, ok := req["tools"]; ok {
				result["tools"] = v
			}
			if v, ok := req["tool_choice"]; ok {
				result["tool_choice"] = v
			}
			if v, ok := req["store"]; ok {
				result["store"] = v
			}
			if v, ok := req["metadata"]; ok {
				result["metadata"] = v
			}
		}
	}

	// Build output array
	outputArr := make([]any, 0)

	for i, choice := range resp.Choices {
		if choice.Message != nil {
			// Text content
			if content, ok := choice.Message.Content.(string); ok && content != "" {
				outputArr = append(outputArr, map[string]any{
					"id":     fmt.Sprintf("msg_%s_%d", resp.ID, i),
					"type":   "message",
					"status": "completed",
					"content": []map[string]any{
						{
							"type":        "output_text",
							"annotations": []any{},
							"logprobs":    []any{},
							"text":        content,
						},
					},
					"role": "assistant",
				})
			}

			// Tool calls
			for _, tc := range choice.Message.ToolCalls {
				outputArr = append(outputArr, map[string]any{
					"id":        "fc_" + tc.ID,
					"type":      "function_call",
					"status":    "completed",
					"arguments": tc.Function.Arguments,
					"call_id":   tc.ID,
					"name":      tc.Function.Name,
				})
			}
		}
	}

	if len(outputArr) > 0 {
		result["output"] = outputArr
	}

	// Usage mapping
	if resp.Usage != nil {
		result["usage"] = map[string]any{
			"input_tokens":  resp.Usage.PromptTokens,
			"output_tokens": resp.Usage.CompletionTokens,
			"total_tokens":  resp.Usage.TotalTokens,
		}
	}

	return result
}

// respondError writes an OpenAI-compatible error response.
func (h *ResponsesHandler) respondError(c *gin.Context, apiErr *domaincompletion.APIError) {
	c.JSON(apiErr.HTTPStatus, domaincompletion.APIErrorResponse{
		Error: *apiErr,
	})
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// extractModel extracts the model name from a raw JSON request body.
func extractModel(body []byte) string {
	var m struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &m); err != nil {
		return ""
	}
	return m.Model
}
