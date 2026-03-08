package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	domaincompletion "github.com/duchoang/llmpool/internal/domain/completion"
	domainprovider "github.com/duchoang/llmpool/internal/domain/provider"
	domainusage "github.com/duchoang/llmpool/internal/domain/usage"
	providerinfra "github.com/duchoang/llmpool/internal/infra/provider"
	"github.com/duchoang/llmpool/internal/translator"
	usecasecompletion "github.com/duchoang/llmpool/internal/usecase/completion"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

const (
	copilotChatVersionResp         = "0.26.7"
	copilotEditorPluginVersionResp = "copilot-chat/" + copilotChatVersionResp
	copilotUserAgentResp           = "GitHubCopilotChat/" + copilotChatVersionResp
	defaultVSCodeVersionResp       = "1.104.3"
	copilotGitHubAPIVersionResp    = "2025-04-01"
)

// ResponsesHandler handles OpenAI Responses API requests (used by Cursor IDE).
// It uses a passthrough-first strategy:
//   - Native /responses for models that support it
//   - Fallback conversion path for legacy models
type ResponsesHandler struct {
	service          usecasecompletion.CompletionService
	router           usecasecompletion.Router
	registry         usecasecompletion.ProviderRegistry
	httpClient       *http.Client
	logger           *zap.Logger
	usagePublisher   MessagesUsagePublisher
	responsesRouting bool
}

// NewResponsesHandler creates a new responses handler.
func NewResponsesHandler(
	service usecasecompletion.CompletionService,
	router usecasecompletion.Router,
	registry usecasecompletion.ProviderRegistry,
	logger *zap.Logger,
	responsesRouting bool,
) *ResponsesHandler {
	return &ResponsesHandler{
		service:          service,
		router:           router,
		registry:         registry,
		httpClient:       newResponsesHTTPClient(),
		logger:           logger,
		responsesRouting: responsesRouting,
	}
}

// SetUsagePublisher sets the usage publisher for tracking.
func (h *ResponsesHandler) SetUsagePublisher(publisher MessagesUsagePublisher) {
	h.usagePublisher = publisher
}

func newResponsesHTTPClient() *http.Client {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          200,
		MaxIdleConnsPerHost:   100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ForceAttemptHTTP2:     true,
	}

	return &http.Client{Transport: transport}
}

// CreateResponse handles POST /v1/responses.
// Strategy:
//  1. If model supports native Copilot /responses and routing is enabled, do passthrough.
//  2. Otherwise, fallback to legacy translation path for compatibility.
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

	// Automatic native /responses passthrough when supported.
	if h.shouldUseNativeResponses(respReq.Model) {
		h.handleNativeResponsesPassthrough(c, &respReq, body)
		return
	}

	// Legacy fallback path for models/endpoints without native /responses support.
	h.handleLegacyResponses(c, &respReq, body)
}

func (h *ResponsesHandler) shouldUseNativeResponses(model string) bool {
	if !h.responsesRouting || h.router == nil || h.registry == nil {
		return false
	}

	providers := h.registry.GetProvidersForModel(model)
	if len(providers) == 0 {
		return false
	}

	for _, p := range providers {
		if p == domainprovider.ProviderCopilot {
			return providerinfra.ShouldUseCopilotResponsesAPI(model)
		}
	}

	return false
}

func (h *ResponsesHandler) handleLegacyResponses(c *gin.Context, respReq *translator.ResponsesRequest, originalRequest []byte) {
	// Convert to Chat Completions format
	chatReq, err := translator.ConvertResponsesToChat(respReq)
	if err != nil {
		h.respondError(c, domaincompletion.ErrInvalidJSON("failed to convert request: "+err.Error()))
		return
	}

	// Build domain request
	domainReq := h.buildDomainRequest(chatReq)

	// Handle streaming vs non-streaming
	if domainReq.Stream {
		h.handleStreamingLegacy(c, domainReq, originalRequest)
		return
	}

	h.handleNonStreamingLegacy(c, domainReq, originalRequest)
}

func (h *ResponsesHandler) handleNativeResponsesPassthrough(c *gin.Context, req *translator.ResponsesRequest, rawBody []byte) {
	ctx := c.Request.Context()

	decision, err := h.router.RouteWithHint(ctx, req.Model, "copilot", nil)
	if err != nil {
		var apiErr *domaincompletion.APIError
		if errors.As(err, &apiErr) {
			h.respondError(c, apiErr)
			return
		}
		h.respondError(c, domaincompletion.ErrInternalServer(err.Error()))
		return
	}

	url := strings.TrimSuffix(decision.BaseURL, "/") + providerinfra.CopilotEndpointResponses.Path()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(rawBody))
	if err != nil {
		h.respondError(c, domaincompletion.ErrInternalServer("failed to create upstream request: "+err.Error()))
		return
	}

	applyResponsesCopilotHeaders(httpReq, decision)
	httpReq.Header.Set("Content-Type", "application/json")

	requestID := c.GetHeader("X-Request-ID")
	if requestID == "" {
		requestID = c.Writer.Header().Get("X-Request-ID")
	}
	start := time.Now()

	h.logger.Info("sending responses passthrough request to copilot",
		zap.String("url", url),
		zap.String("model", req.Model),
		zap.String("request_id", requestID),
		zap.Bool("stream", req.Stream),
	)

	resp, err := h.httpClient.Do(httpReq)
	if err != nil {
		h.publishResponsesUsage(requestID, req.Model, decision, nil, domainusage.StatusFailed, err.Error(), start, req.Stream)
		h.respondError(c, domaincompletion.NewAPIError(http.StatusBadGateway, domaincompletion.ErrorTypeServer, "upstream request failed: "+err.Error()))
		return
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		h.publishResponsesUsage(requestID, req.Model, decision, nil, domainusage.StatusFailed, truncateString(string(respBody), 200), start, req.Stream)
		h.respondError(c, parseProviderErrorToAPIError(resp.StatusCode, respBody))
		return
	}

	if req.Stream {
		h.streamResponsesPassthrough(c, resp.Body, requestID, req.Model, decision, start)
		return
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		h.publishResponsesUsage(requestID, req.Model, decision, nil, domainusage.StatusFailed, "failed to read upstream response", start, false)
		h.respondError(c, domaincompletion.ErrInternalServer("failed to read upstream response"))
		return
	}

	usage := extractResponsesUsageFromPayload(respBody)
	h.publishResponsesUsage(requestID, req.Model, decision, usage, domainusage.StatusDone, "", start, false)

	c.Data(resp.StatusCode, "application/json", respBody)
}

func (h *ResponsesHandler) streamResponsesPassthrough(
	c *gin.Context,
	src io.Reader,
	requestID string,
	model string,
	decision *domainprovider.RoutingDecision,
	startedAt time.Time,
) {
	// Set SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Transfer-Encoding", "chunked")
	c.Header("X-Accel-Buffering", "no")

	writer := c.Writer
	writer.Flush()

	buf := make([]byte, 4096)
	var partial []byte
	var usage *responsesUsage
	status := domainusage.StatusDone
	errorMsg := ""

	for {
		select {
		case <-c.Request.Context().Done():
			status = domainusage.StatusCanceled
			errorMsg = c.Request.Context().Err().Error()
			h.publishResponsesUsage(requestID, model, decision, usage, status, errorMsg, startedAt, true)
			return
		default:
		}

		n, readErr := src.Read(buf)
		if n > 0 {
			chunk := buf[:n]
			partial = append(partial, chunk...)

			if _, writeErr := writer.Write(chunk); writeErr != nil {
				status = domainusage.StatusFailed
				errorMsg = "write error: " + writeErr.Error()
				h.publishResponsesUsage(requestID, model, decision, usage, status, errorMsg, startedAt, true)
				return
			}
			writer.Flush()

			usage = mergeResponsesUsage(usage, extractResponsesUsageFromSSEBuffer(partial))
		}

		if readErr != nil {
			if readErr != io.EOF {
				status = domainusage.StatusFailed
				errorMsg = "read error: " + readErr.Error()
			}
			h.publishResponsesUsage(requestID, model, decision, usage, status, errorMsg, startedAt, true)
			return
		}
	}
}

func mergeResponsesUsage(current, incoming *responsesUsage) *responsesUsage {
	if incoming == nil {
		return current
	}
	if current == nil {
		copy := *incoming
		return &copy
	}
	if incoming.InputTokens > 0 {
		current.InputTokens = incoming.InputTokens
	}
	if incoming.OutputTokens > 0 {
		current.OutputTokens = incoming.OutputTokens
	}
	if incoming.CachedTokens > 0 {
		current.CachedTokens = incoming.CachedTokens
	}
	if incoming.CacheCreationTokens > 0 {
		current.CacheCreationTokens = incoming.CacheCreationTokens
	}
	if incoming.ReasoningTokens > 0 {
		current.ReasoningTokens = incoming.ReasoningTokens
	}
	return current
}

type responsesUsage struct {
	InputTokens         int
	OutputTokens        int
	CachedTokens        int
	CacheCreationTokens int
	ReasoningTokens     int
}

func extractResponsesUsageFromPayload(payload []byte) *responsesUsage {
	var root map[string]any
	if err := json.Unmarshal(payload, &root); err != nil {
		return nil
	}

	responseObj := root
	if t, _ := root["type"].(string); t == "response.completed" {
		if nested, ok := root["response"].(map[string]any); ok {
			responseObj = nested
		}
	} else if nested, ok := root["response"].(map[string]any); ok {
		responseObj = nested
	}

	usageMap, _ := responseObj["usage"].(map[string]any)
	if usageMap == nil {
		return nil
	}

	usage := &responsesUsage{
		InputTokens:         toInt(usageMap["input_tokens"]),
		OutputTokens:        toInt(usageMap["output_tokens"]),
		CacheCreationTokens: toInt(usageMap["cache_creation_input_tokens"]),
	}

	if usage.CachedTokens == 0 {
		if inputDetails, ok := usageMap["input_tokens_details"].(map[string]any); ok {
			usage.CachedTokens = toInt(inputDetails["cached_tokens"])
		}
	}
	if usage.CachedTokens == 0 {
		if promptDetails, ok := usageMap["prompt_tokens_details"].(map[string]any); ok {
			usage.CachedTokens = toInt(promptDetails["cached_tokens"])
		}
	}
	if outputDetails, ok := usageMap["output_tokens_details"].(map[string]any); ok {
		usage.ReasoningTokens = toInt(outputDetails["reasoning_tokens"])
	}

	if usage.CachedTokens > 0 && usage.InputTokens >= usage.CachedTokens {
		usage.InputTokens -= usage.CachedTokens
	}

	return usage
}

func extractResponsesUsageFromSSEBuffer(buf []byte) *responsesUsage {
	idx := bytes.LastIndex(buf, []byte("\n\n"))
	if idx > 0 {
		buf = buf[:idx+2]
	}

	blocks := bytes.Split(buf, []byte("\n\n"))
	var latest *responsesUsage
	for _, block := range blocks {
		data := extractSSEDataFromBlock(block)
		if data == nil || bytes.Equal(data, []byte("[DONE]")) {
			continue
		}
		parsed := extractResponsesUsageFromPayload(data)
		if parsed != nil {
			latest = mergeResponsesUsage(latest, parsed)
		}
	}
	return latest
}

func toInt(v any) int {
	switch x := v.(type) {
	case float64:
		return int(x)
	case int:
		return x
	case int64:
		return int(x)
	default:
		return 0
	}
}

func (h *ResponsesHandler) publishResponsesUsage(
	requestID string,
	model string,
	decision *domainprovider.RoutingDecision,
	usage *responsesUsage,
	status domainusage.Status,
	errorMsg string,
	startedAt time.Time,
	stream bool,
) {
	if h.usagePublisher == nil {
		return
	}

	promptTokens := 0
	completionTokens := 0
	cachedTokens := 0
	if usage != nil {
		promptTokens = usage.InputTokens
		completionTokens = usage.OutputTokens
		cachedTokens = usage.CachedTokens + usage.CacheCreationTokens
	}

	provider := ""
	credentialID := ""
	credentialType := ""
	credentialAccountID := ""
	if decision != nil {
		provider = string(decision.ProviderID)
		credentialID = decision.CredentialID
		credentialType = decision.CredentialType
		credentialAccountID = decision.CredentialAccountID
	}

	// TODO: persist provider metadata (for example server_tool_use) into usage audit logs.

	record := domainusage.UsageRecord{
		RequestID:           requestID,
		Model:               model,
		Provider:            provider,
		CredentialID:        credentialID,
		CredentialType:      credentialType,
		CredentialAccountID: credentialAccountID,
		PromptTokens:        promptTokens,
		CachedTokens:        cachedTokens,
		CompletionTokens:    completionTokens,
		Status:              status,
		ErrorMessage:        errorMsg,
		StartedAt:           startedAt,
		CompletedAt:         time.Now(),
		Stream:              stream,
	}

	h.usagePublisher.Publish(record)
}

func parseProviderErrorToAPIError(statusCode int, body []byte) *domaincompletion.APIError {
	var errResp domaincompletion.APIErrorResponse
	if err := json.Unmarshal(body, &errResp); err == nil && errResp.Error.Message != "" {
		apiErr := &errResp.Error
		apiErr.HTTPStatus = statusCode
		return apiErr
	}

	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err == nil {
		if errObj, ok := raw["error"].(map[string]any); ok {
			if msg, ok := errObj["message"].(string); ok && msg != "" {
				return domaincompletion.NewAPIError(statusCode, domaincompletion.MapHTTPStatusToErrorType(statusCode), msg)
			}
		}
		if msg, ok := raw["message"].(string); ok && msg != "" {
			return domaincompletion.NewAPIError(statusCode, domaincompletion.MapHTTPStatusToErrorType(statusCode), msg)
		}
	}

	errType := domaincompletion.MapHTTPStatusToErrorType(statusCode)
	message := string(body)
	if message == "" {
		message = http.StatusText(statusCode)
	}

	return domaincompletion.NewAPIError(statusCode, errType, message)
}

func applyResponsesCopilotHeaders(req *http.Request, decision *domainprovider.RoutingDecision) {
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+decision.Token)
	req.Header.Set("Copilot-Integration-Id", "vscode-chat")
	req.Header.Set("Editor-Version", "vscode/"+defaultVSCodeVersionResp)
	req.Header.Set("Editor-Plugin-Version", copilotEditorPluginVersionResp)
	req.Header.Set("User-Agent", copilotUserAgentResp)
	req.Header.Set("Openai-Intent", "conversation-edits")
	req.Header.Set("X-Github-Api-Version", copilotGitHubAPIVersionResp)
	req.Header.Set("X-Vscode-User-Agent-Library-Version", "electron-fetch")
	req.Header.Set("X-Initiator", "agent")

	for k, v := range decision.Headers {
		req.Header.Set(k, v)
	}
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

// handleNonStreamingLegacy handles non-streaming responses requests.
func (h *ResponsesHandler) handleNonStreamingLegacy(c *gin.Context, req domaincompletion.ChatCompletionRequest, originalRequest []byte) {
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

// handleStreamingLegacy handles streaming responses requests.
func (h *ResponsesHandler) handleStreamingLegacy(c *gin.Context, req domaincompletion.ChatCompletionRequest, originalRequest []byte) {
	ctx := c.Request.Context()

	// Preflight validation/routing before committing streaming headers.
	if err := h.service.ValidateRequest(ctx, req); err != nil {
		var apiErr *domaincompletion.APIError
		if errors.As(err, &apiErr) {
			h.respondError(c, apiErr)
			return
		}
		h.respondError(c, domaincompletion.ErrInternalServer(err.Error()))
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
		usage := map[string]any{
			"input_tokens":  resp.Usage.PromptTokens,
			"output_tokens": resp.Usage.CompletionTokens,
			"total_tokens":  resp.Usage.TotalTokens,
		}
		usage["input_tokens_details"] = map[string]any{
			"cached_tokens": resp.Usage.CachedTokens(),
		}
		if resp.Usage.CompletionTokensDetails != nil && resp.Usage.CompletionTokensDetails.ReasoningTokens > 0 {
			usage["output_tokens_details"] = map[string]any{
				"reasoning_tokens": resp.Usage.CompletionTokensDetails.ReasoningTokens,
			}
		} else {
			usage["output_tokens_details"] = map[string]any{
				"reasoning_tokens": 0,
			}
		}
		result["usage"] = usage
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
