package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	domaincompletion "github.com/duchoang/llmpool/internal/domain/completion"
	domainprovider "github.com/duchoang/llmpool/internal/domain/provider"
	usecasecompletion "github.com/duchoang/llmpool/internal/usecase/completion"
	"go.uber.org/zap"
)

const (
	codexClientVersion = "0.101.0"
	codexUserAgent     = "codex_cli_rs/0.101.0 (Mac OS 26.0.1; arm64) Apple_Terminal/464"
)

// ClientConfig configures the HTTP client.
type ClientConfig struct {
	Timeout time.Duration
}

// DefaultClientConfig returns sensible defaults.
func DefaultClientConfig() ClientConfig {
	return ClientConfig{
		Timeout: 120 * time.Second,
	}
}

// client implements ProviderClient.
type client struct {
	httpClient *http.Client
	logger     *zap.Logger
}

// NewClient creates a new provider client.
func NewClient(config ClientConfig, logger *zap.Logger) usecasecompletion.ProviderClient {
	return &client{
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
		logger: logger,
	}
}

// ChatCompletion executes a chat completion request.
func (c *client) ChatCompletion(
	ctx context.Context,
	decision domainprovider.RoutingDecision,
	req domaincompletion.ChatCompletionRequest,
) (*domaincompletion.ChatCompletionResponse, error) {
	if decision.ProviderID == domainprovider.ProviderCodex {
		return c.chatCompletionCodex(ctx, decision, req)
	}
	return c.chatCompletionOpenAICompatible(ctx, decision, req)
}

func (c *client) chatCompletionOpenAICompatible(
	ctx context.Context,
	decision domainprovider.RoutingDecision,
	req domaincompletion.ChatCompletionRequest,
) (*domaincompletion.ChatCompletionResponse, error) {
	req.Stream = false

	body, err := json.Marshal(req)
	if err != nil {
		return nil, domaincompletion.ErrInvalidJSON(err.Error())
	}

	url := strings.TrimSuffix(decision.BaseURL, "/") + "/v1/chat/completions"

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, domaincompletion.ErrInternalServer(fmt.Sprintf("create request: %v", err))
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	if decision.Token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+decision.Token)
	}

	for k, v := range decision.Headers {
		httpReq.Header.Set(k, v)
	}

	c.logger.Debug("sending request to provider",
		zap.String("provider", string(decision.ProviderID)),
		zap.String("url", url),
		zap.String("model", req.Model),
	)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, domaincompletion.ErrInternalServer(fmt.Sprintf("request failed: %v", err))
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, domaincompletion.ErrInternalServer(fmt.Sprintf("read response: %v", err))
	}

	if resp.StatusCode >= 400 {
		return nil, c.parseErrorResponse(resp.StatusCode, respBody)
	}

	var result domaincompletion.ChatCompletionResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, domaincompletion.ErrInternalServer(fmt.Sprintf("parse response: %v", err))
	}

	return &result, nil
}

func (c *client) chatCompletionCodex(
	ctx context.Context,
	decision domainprovider.RoutingDecision,
	req domaincompletion.ChatCompletionRequest,
) (*domaincompletion.ChatCompletionResponse, error) {
	body, err := buildCodexRequestBody(req, true)
	if err != nil {
		return nil, domaincompletion.ErrInvalidJSON(err.Error())
	}

	url := strings.TrimSuffix(codexBaseURL(decision.BaseURL), "/") + "/responses"

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, domaincompletion.ErrInternalServer(fmt.Sprintf("create request: %v", err))
	}

	applyCodexHeaders(httpReq, decision, true)

	c.logger.Info("sending codex completion request",
		zap.String("provider", string(decision.ProviderID)),
		zap.String("url", url),
		zap.String("model", req.Model),
		zap.String("credential_id", decision.CredentialID),
		zap.String("credential_account_id", decision.CredentialAccountID),
	)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, domaincompletion.ErrInternalServer(fmt.Sprintf("request failed: %v", err))
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, domaincompletion.ErrInternalServer(fmt.Sprintf("read response: %v", err))
	}

	if resp.StatusCode >= 400 {
		c.logger.Warn("codex upstream error",
			zap.Int("status", resp.StatusCode),
			zap.String("body", summarizeBody(respBody)),
		)
		return nil, c.parseErrorResponse(resp.StatusCode, respBody)
	}

	completedEvent, extractErr := extractCompletedEvent(respBody)
	if extractErr != nil {
		return nil, domaincompletion.ErrInternalServer(extractErr.Error())
	}

	return parseCodexCompactResponse(completedEvent, req.Model)
}

func extractCompletedEvent(sseBody []byte) ([]byte, error) {
	lines := bytes.Split(sseBody, []byte("\n"))
	for _, line := range lines {
		line = bytes.TrimSpace(line)
		if len(line) == 0 || !bytes.HasPrefix(line, []byte("data:")) {
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
		if mapString(root, "type") == "response.completed" {
			return payload, nil
		}
	}
	return nil, fmt.Errorf("codex response.completed event not found")
}

// ChatCompletionStream executes a streaming chat completion request.
func (c *client) ChatCompletionStream(
	ctx context.Context,
	decision domainprovider.RoutingDecision,
	req domaincompletion.ChatCompletionRequest,
) (<-chan usecasecompletion.StreamChunk, error) {
	if decision.ProviderID == domainprovider.ProviderCodex {
		return c.chatCompletionStreamCodexCompat(ctx, decision, req)
	}
	return c.chatCompletionStreamOpenAICompatible(ctx, decision, req)
}

func (c *client) chatCompletionStreamOpenAICompatible(
	ctx context.Context,
	decision domainprovider.RoutingDecision,
	req domaincompletion.ChatCompletionRequest,
) (<-chan usecasecompletion.StreamChunk, error) {
	req.Stream = true

	body, err := json.Marshal(req)
	if err != nil {
		return nil, domaincompletion.ErrInvalidJSON(err.Error())
	}

	url := strings.TrimSuffix(decision.BaseURL, "/") + "/v1/chat/completions"

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, domaincompletion.ErrInternalServer(fmt.Sprintf("create request: %v", err))
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("Cache-Control", "no-cache")
	httpReq.Header.Set("Connection", "keep-alive")

	if decision.Token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+decision.Token)
	}

	for k, v := range decision.Headers {
		httpReq.Header.Set(k, v)
	}

	c.logger.Debug("sending streaming request to provider",
		zap.String("provider", string(decision.ProviderID)),
		zap.String("url", url),
		zap.String("model", req.Model),
	)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, domaincompletion.ErrInternalServer(fmt.Sprintf("request failed: %v", err))
	}

	if resp.StatusCode >= 400 {
		defer func() { _ = resp.Body.Close() }()
		respBody, _ := io.ReadAll(resp.Body)
		return nil, c.parseErrorResponse(resp.StatusCode, respBody)
	}

	chunks := make(chan usecasecompletion.StreamChunk, 100)

	go func() {
		defer close(chunks)
		defer func() { _ = resp.Body.Close() }()

		reader := resp.Body
		buf := make([]byte, 4096)
		var partial []byte

		for {
			select {
			case <-ctx.Done():
				chunks <- usecasecompletion.StreamChunk{Error: ctx.Err()}
				return
			default:
			}

			n, readErr := reader.Read(buf)
			if n > 0 {
				partial = append(partial, buf[:n]...)

				for {
					idx := bytes.Index(partial, []byte("\n\n"))
					if idx == -1 {
						break
					}

					event := partial[:idx]
					partial = partial[idx+2:]

					if bytes.HasPrefix(event, []byte("data: ")) {
						data := event[6:]

						if bytes.Equal(data, []byte("[DONE]")) {
							chunks <- usecasecompletion.StreamChunk{Done: true}
							return
						}

						chunks <- usecasecompletion.StreamChunk{Data: data}
					}
				}
			}

			if readErr != nil {
				if readErr == io.EOF {
					chunks <- usecasecompletion.StreamChunk{Done: true}
					return
				}
				chunks <- usecasecompletion.StreamChunk{Error: readErr}
				return
			}
		}
	}()

	return chunks, nil
}

// chatCompletionStreamCodexCompat provides SSE compatibility for codex.
// It performs a non-stream compact request and emits OpenAI-style chunks.
func (c *client) chatCompletionStreamCodexCompat(
	ctx context.Context,
	decision domainprovider.RoutingDecision,
	req domaincompletion.ChatCompletionRequest,
) (<-chan usecasecompletion.StreamChunk, error) {
	resp, err := c.chatCompletionCodex(ctx, decision, req)
	if err != nil {
		return nil, err
	}

	chunks := make(chan usecasecompletion.StreamChunk, 3)

	go func() {
		defer close(chunks)

		if len(resp.Choices) == 0 || resp.Choices[0].Message == nil {
			chunks <- usecasecompletion.StreamChunk{Done: true}
			return
		}

		msg := resp.Choices[0].Message
		chunk1 := domaincompletion.ChatCompletionChunk{
			ID:      resp.ID,
			Object:  "chat.completion.chunk",
			Created: resp.Created,
			Model:   resp.Model,
			Choices: []domaincompletion.Choice{{
				Index: 0,
				Delta: &domaincompletion.Message{Role: "assistant"},
			}},
		}
		if data, marshalErr := json.Marshal(chunk1); marshalErr == nil {
			chunks <- usecasecompletion.StreamChunk{Data: data}
		}

		if content, ok := msg.Content.(string); ok && content != "" {
			chunk2 := domaincompletion.ChatCompletionChunk{
				ID:      resp.ID,
				Object:  "chat.completion.chunk",
				Created: resp.Created,
				Model:   resp.Model,
				Choices: []domaincompletion.Choice{{
					Index: 0,
					Delta: &domaincompletion.Message{Content: content},
				}},
			}
			if data, marshalErr := json.Marshal(chunk2); marshalErr == nil {
				chunks <- usecasecompletion.StreamChunk{Data: data}
			}
		}

		finish := resp.Choices[0].FinishReason
		if finish == "" {
			finish = "stop"
		}

		chunk3 := domaincompletion.ChatCompletionChunk{
			ID:      resp.ID,
			Object:  "chat.completion.chunk",
			Created: resp.Created,
			Model:   resp.Model,
			Choices: []domaincompletion.Choice{{
				Index:        0,
				Delta:        &domaincompletion.Message{},
				FinishReason: finish,
			}},
		}
		if data, marshalErr := json.Marshal(chunk3); marshalErr == nil {
			chunks <- usecasecompletion.StreamChunk{Data: data}
		}

		chunks <- usecasecompletion.StreamChunk{Done: true}
	}()

	return chunks, nil
}

func buildCodexRequestBody(req domaincompletion.ChatCompletionRequest, stream bool) ([]byte, error) {
	instructions := "You are a helpful assistant."
	for _, m := range req.Messages {
		if m.Role == "system" {
			candidate := strings.TrimSpace(stringifyContent(m.Content))
			if candidate != "" {
				instructions = candidate
			}
			break
		}
	}

	payload := map[string]any{
		"model":        req.Model,
		"instructions": instructions,
		"store":        false,
	}
	if stream {
		payload["stream"] = true
	}

	input := make([]any, 0, len(req.Messages))
	for _, m := range req.Messages {
		if m.Role == "system" {
			continue
		}

		if m.Role == "tool" {
			input = append(input, map[string]any{
				"type":    "function_call_output",
				"call_id": m.ToolCallID,
				"output":  stringifyContent(m.Content),
			})
			continue
		}

		role := m.Role
		if role == "system" {
			role = "developer"
		}

		input = append(input, map[string]any{
			"type":    "message",
			"role":    role,
			"content": buildCodexContent(role, m.Content),
		})

		if m.Role == "assistant" && len(m.ToolCalls) > 0 {
			for _, tc := range m.ToolCalls {
				input = append(input, map[string]any{
					"type":      "function_call",
					"call_id":   tc.ID,
					"name":      tc.Function.Name,
					"arguments": tc.Function.Arguments,
				})
			}
		}
	}
	payload["input"] = input

	if len(req.Tools) > 0 {
		tools := make([]any, 0, len(req.Tools))
		for _, t := range req.Tools {
			if t.Type != "function" {
				continue
			}
			tool := map[string]any{
				"type": "function",
				"name": t.Function.Name,
			}
			if t.Function.Description != "" {
				tool["description"] = t.Function.Description
			}
			if t.Function.Parameters != nil {
				tool["parameters"] = t.Function.Parameters
			}
			tools = append(tools, tool)
		}
		if len(tools) > 0 {
			payload["tools"] = tools
		}
	}

	if req.ToolChoice != nil {
		payload["tool_choice"] = req.ToolChoice
	}

	if req.User != "" {
		payload["user"] = req.User
	}

	return json.Marshal(payload)
}

func buildCodexContent(role string, content any) []map[string]any {
	partType := "input_text"
	if role == "assistant" {
		partType = "output_text"
	}

	parts := []map[string]any{}

	switch v := content.(type) {
	case string:
		if strings.TrimSpace(v) != "" {
			parts = append(parts, map[string]any{"type": partType, "text": v})
		}
	case []any:
		for _, raw := range v {
			item, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			typeVal, _ := item["type"].(string)
			switch typeVal {
			case "text":
				if txt, ok := item["text"].(string); ok && strings.TrimSpace(txt) != "" {
					parts = append(parts, map[string]any{"type": partType, "text": txt})
				}
			case "image_url":
				if role != "user" {
					continue
				}
				if image, ok := item["image_url"].(map[string]any); ok {
					if url, ok := image["url"].(string); ok && strings.TrimSpace(url) != "" {
						parts = append(parts, map[string]any{"type": "input_image", "image_url": url})
					}
				}
			}
		}
	}

	if len(parts) == 0 {
		parts = append(parts, map[string]any{"type": partType, "text": stringifyContent(content)})
	}

	return parts
}

func stringifyContent(content any) string {
	if s, ok := content.(string); ok {
		return s
	}
	b, err := json.Marshal(content)
	if err != nil {
		return ""
	}
	return string(b)
}

func codexBaseURL(baseURL string) string {
	trimmed := strings.TrimSpace(strings.TrimSuffix(baseURL, "/"))
	if trimmed == "" || strings.Contains(trimmed, "api.openai.com") {
		return "https://chatgpt.com/backend-api/codex"
	}
	if strings.Contains(trimmed, "chatgpt.com") && !strings.Contains(trimmed, "/backend-api/codex") {
		return trimmed + "/backend-api/codex"
	}
	return trimmed
}

func applyCodexHeaders(req *http.Request, decision domainprovider.RoutingDecision, stream bool) {
	req.Header.Set("Content-Type", "application/json")
	if stream {
		req.Header.Set("Accept", "text/event-stream")
	} else {
		req.Header.Set("Accept", "application/json")
	}
	req.Header.Set("Connection", "Keep-Alive")
	req.Header.Set("Version", codexClientVersion)
	req.Header.Set("User-Agent", codexUserAgent)
	req.Header.Set("Originator", "codex_cli_rs")

	if decision.Token != "" {
		req.Header.Set("Authorization", "Bearer "+decision.Token)
	}
	if decision.CredentialAccountID != "" {
		req.Header.Set("Chatgpt-Account-Id", decision.CredentialAccountID)
	}

	for k, v := range decision.Headers {
		req.Header.Set(k, v)
	}
}

func parseCodexCompactResponse(raw []byte, fallbackModel string) (*domaincompletion.ChatCompletionResponse, error) {
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, domaincompletion.ErrInternalServer(fmt.Sprintf("parse codex response: %v", err))
	}

	responseObj := root
	if t := mapString(root, "type"); t == "response.completed" {
		if nested, ok := root["response"].(map[string]any); ok {
			responseObj = nested
		}
	} else if nested, ok := root["response"].(map[string]any); ok {
		responseObj = nested
	}

	id := mapString(responseObj, "id")
	if id == "" {
		id = fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano())
	}

	created := mapInt64(responseObj, "created_at")
	if created == 0 {
		created = time.Now().Unix()
	}

	model := mapString(responseObj, "model")
	if model == "" {
		model = fallbackModel
	}

	message := domaincompletion.Message{Role: "assistant", Content: ""}
	finishReason := "stop"

	if output, ok := responseObj["output"].([]any); ok {
		var contentBuilder strings.Builder
		toolCalls := make([]domaincompletion.ToolCall, 0)

		for _, itemRaw := range output {
			item, ok := itemRaw.(map[string]any)
			if !ok {
				continue
			}

			switch mapString(item, "type") {
			case "message":
				role := mapString(item, "role")
				if role != "" && role != "assistant" {
					continue
				}
				if contentArr, ok := item["content"].([]any); ok {
					for _, partRaw := range contentArr {
						part, ok := partRaw.(map[string]any)
						if !ok {
							continue
						}
						partType := mapString(part, "type")
						if partType == "output_text" || partType == "input_text" || partType == "text" {
							contentBuilder.WriteString(mapString(part, "text"))
						}
					}
				}
			case "function_call":
				callID := mapString(item, "call_id")
				if callID == "" {
					callID = mapString(item, "id")
				}
				toolCalls = append(toolCalls, domaincompletion.ToolCall{
					ID:   callID,
					Type: "function",
					Function: domaincompletion.FunctionCall{
						Name:      mapString(item, "name"),
						Arguments: mapString(item, "arguments"),
					},
				})
			}
		}

		if contentBuilder.Len() > 0 {
			message.Content = contentBuilder.String()
		}
		if len(toolCalls) > 0 {
			message.ToolCalls = toolCalls
			finishReason = "tool_calls"
		}
	}

	choice := domaincompletion.Choice{
		Index:        0,
		Message:      &message,
		FinishReason: finishReason,
	}

	var usage *domaincompletion.Usage
	if usageMap, ok := responseObj["usage"].(map[string]any); ok {
		prompt := int(mapInt64(usageMap, "input_tokens"))
		completion := int(mapInt64(usageMap, "output_tokens"))
		total := int(mapInt64(usageMap, "total_tokens"))
		if total == 0 {
			total = prompt + completion
		}
		usage = &domaincompletion.Usage{
			PromptTokens:     prompt,
			CompletionTokens: completion,
			TotalTokens:      total,
		}
	}

	return &domaincompletion.ChatCompletionResponse{
		ID:      id,
		Object:  "chat.completion",
		Created: created,
		Model:   model,
		Choices: []domaincompletion.Choice{choice},
		Usage:   usage,
	}, nil
}

func mapString(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func mapInt64(m map[string]any, key string) int64 {
	v, ok := m[key]
	if !ok {
		return 0
	}
	switch t := v.(type) {
	case int:
		return int64(t)
	case int64:
		return t
	case float64:
		return int64(t)
	default:
		return 0
	}
}

func summarizeBody(body []byte) string {
	trimmed := strings.TrimSpace(string(body))
	if len(trimmed) > 512 {
		return trimmed[:512] + "..."
	}
	return trimmed
}

// parseErrorResponse parses an error response from the provider.
func (c *client) parseErrorResponse(statusCode int, body []byte) *domaincompletion.APIError {
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
