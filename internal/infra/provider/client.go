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

	"github.com/google/uuid"

	domaincompletion "github.com/duchoang/llmpool/internal/domain/completion"
	domainprovider "github.com/duchoang/llmpool/internal/domain/provider"
	usecasecompletion "github.com/duchoang/llmpool/internal/usecase/completion"
	"go.uber.org/zap"
)

const (
	codexClientVersion = "0.101.0"
	codexUserAgent     = "codex_cli_rs/0.101.0 (Mac OS 26.0.1; arm64) Apple_Terminal/464"

	// Copilot API version constants - aligned with reference implementation
	copilotChatVersion         = "0.26.7"
	copilotEditorPluginVersion = "copilot-chat/" + copilotChatVersion
	copilotUserAgent           = "GitHubCopilotChat/" + copilotChatVersion
	defaultVSCodeVersion       = "1.104.3"
	copilotGitHubAPIVersion    = "2025-04-01"
)

// ClientConfig configures the HTTP client.
type ClientConfig struct {
	Timeout time.Duration
	// EnableCopilotResponsesRouting enables /responses endpoint for GPT-5+ models.
	// When true, GPT-5+ models (except gpt-5-mini) use /responses instead of /chat/completions.
	// Default: false for safe rollout / backward compatibility.
	EnableCopilotResponsesRouting bool
}

// DefaultClientConfig returns sensible defaults.
func DefaultClientConfig() ClientConfig {
	return ClientConfig{
		Timeout:                       120 * time.Second,
		EnableCopilotResponsesRouting: false, // Safe default for backward compatibility
	}
}

// client implements ProviderClient.
type client struct {
	httpClient *http.Client
	logger     *zap.Logger
	// enableCopilotResponsesRouting enables /responses endpoint for GPT-5+ models (except gpt-5-mini).
	// When false (default), all Copilot requests use /chat/completions for backward compatibility.
	enableCopilotResponsesRouting bool
}

// NewClient creates a new provider client.
func NewClient(config ClientConfig, logger *zap.Logger) usecasecompletion.ProviderClient {
	return &client{
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
		logger:                        logger,
		enableCopilotResponsesRouting: config.EnableCopilotResponsesRouting,
	}
}

// ChatCompletion executes a chat completion request.
func (c *client) ChatCompletion(
	ctx context.Context,
	decision domainprovider.RoutingDecision,
	req domaincompletion.ChatCompletionRequest,
) (*domaincompletion.ChatCompletionResponse, error) {
	switch decision.ProviderID {
	case domainprovider.ProviderCodex:
		return c.chatCompletionCodex(ctx, decision, req)
	case domainprovider.ProviderCopilot:
		return c.chatCompletionCopilot(ctx, decision, req)
	default:
		return c.chatCompletionOpenAICompatible(ctx, decision, req)
	}
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

// chatCompletionCopilot executes a chat completion request to GitHub Copilot.
// Headers and endpoint aligned with reference implementation (copilot-api).
// Supports both /chat/completions and /responses endpoints based on model and feature flag.
func (c *client) chatCompletionCopilot(
	ctx context.Context,
	decision domainprovider.RoutingDecision,
	req domaincompletion.ChatCompletionRequest,
) (*domaincompletion.ChatCompletionResponse, error) {
	// Determine which endpoint to use based on model and feature flag
	endpoint := ResolveCopilotEndpoint(req.Model, c.enableCopilotResponsesRouting)

	if endpoint == CopilotEndpointResponses {
		return c.chatCompletionCopilotResponses(ctx, decision, req)
	}
	return c.chatCompletionCopilotChat(ctx, decision, req)
}

// chatCompletionCopilotChat executes a chat completion via /chat/completions endpoint.
func (c *client) chatCompletionCopilotChat(
	ctx context.Context,
	decision domainprovider.RoutingDecision,
	req domaincompletion.ChatCompletionRequest,
) (*domaincompletion.ChatCompletionResponse, error) {
	// Map model ID to Copilot-specific format
	copilotReq := req
	copilotReq.Model = GetCopilotModelID(req.Model)

	body, err := json.Marshal(copilotReq)
	if err != nil {
		return nil, domaincompletion.ErrInvalidJSON(err.Error())
	}

	// Copilot uses /chat/completions without /v1 prefix
	url := strings.TrimSuffix(decision.BaseURL, "/") + "/chat/completions"

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, domaincompletion.ErrInternalServer(fmt.Sprintf("create request: %v", err))
	}

	// Apply Copilot-specific headers
	applyCopilotHeaders(httpReq, decision, req)

	c.logger.Info("sending copilot chat completion request",
		zap.String("provider", string(decision.ProviderID)),
		zap.String("url", url),
		zap.String("model", copilotReq.Model),
		zap.String("original_model", req.Model),
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
		c.logger.Warn("copilot upstream error",
			zap.Int("status", resp.StatusCode),
			zap.String("body", summarizeBody(respBody)),
		)
		return nil, c.parseErrorResponse(resp.StatusCode, respBody)
	}

	var result domaincompletion.ChatCompletionResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, domaincompletion.ErrInternalServer(fmt.Sprintf("parse response: %v", err))
	}

	return &result, nil
}

// chatCompletionCopilotResponses executes a chat completion via /responses endpoint.
// Used for GPT-5+ models (except gpt-5-mini) when responses routing is enabled.
func (c *client) chatCompletionCopilotResponses(
	ctx context.Context,
	decision domainprovider.RoutingDecision,
	req domaincompletion.ChatCompletionRequest,
) (*domaincompletion.ChatCompletionResponse, error) {
	// Convert chat completion request to responses API format
	body, err := ChatToResponsesRequest(req)
	if err != nil {
		return nil, domaincompletion.ErrInvalidJSON(err.Error())
	}

	// Responses endpoint
	url := strings.TrimSuffix(decision.BaseURL, "/") + "/responses"

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, domaincompletion.ErrInternalServer(fmt.Sprintf("create request: %v", err))
	}

	// Apply Copilot-specific headers
	applyCopilotHeaders(httpReq, decision, req)

	c.logger.Info("sending copilot responses request",
		zap.String("provider", string(decision.ProviderID)),
		zap.String("url", url),
		zap.String("model", GetCopilotModelID(req.Model)),
		zap.String("original_model", req.Model),
		zap.String("credential_id", decision.CredentialID),
		zap.String("credential_account_id", decision.CredentialAccountID),
		zap.Bool("responses_routing", true),
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
		c.logger.Warn("copilot responses upstream error",
			zap.Int("status", resp.StatusCode),
			zap.String("body", summarizeBody(respBody)),
		)
		return nil, c.parseErrorResponse(resp.StatusCode, respBody)
	}

	// For non-streaming, the responses API returns SSE events.
	// Extract the response.completed event.
	responsesPayload, extractErr := extractResponsesPayload(respBody)
	if extractErr != nil {
		return nil, domaincompletion.ErrInternalServer(extractErr.Error())
	}

	// Convert responses format to chat completion format
	return ResponsesToChatResponse(responsesPayload, req.Model)
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

	responsesPayload, extractErr := extractResponsesPayload(respBody)
	if extractErr != nil {
		return nil, domaincompletion.ErrInternalServer(extractErr.Error())
	}

	return parseCodexCompactResponse(responsesPayload, req.Model)
}

// extractResponsesPayload extracts the response payload from either:
// 1. A direct JSON response object (non-stream mode)
// 2. An SSE-wrapped response.completed event (stream mode or some non-stream implementations)
//
// The function is robust to both formats for compatibility with different API behaviors.
func extractResponsesPayload(body []byte) ([]byte, error) {
	trimmed := bytes.TrimSpace(body)

	// First, try parsing as direct JSON response (non-SSE format).
	// The /responses API may return a direct JSON object when stream=false.
	if len(trimmed) > 0 && trimmed[0] == '{' {
		var root map[string]any
		if err := json.Unmarshal(trimmed, &root); err == nil {
			// Check if it's a valid responses API object (has "id" or "output" field)
			if _, hasID := root["id"]; hasID {
				return trimmed, nil
			}
			if _, hasOutput := root["output"]; hasOutput {
				return trimmed, nil
			}
			// Check if it's wrapped in response.completed event
			if mapString(root, "type") == "response.completed" {
				return trimmed, nil
			}
		}
	}

	// Fall back to SSE parsing - look for response.completed event
	lines := bytes.Split(body, []byte("\n"))
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
	return nil, fmt.Errorf("responses API: no valid response payload found")
}

// ChatCompletionStream executes a streaming chat completion request.
func (c *client) ChatCompletionStream(
	ctx context.Context,
	decision domainprovider.RoutingDecision,
	req domaincompletion.ChatCompletionRequest,
) (<-chan usecasecompletion.StreamChunk, error) {
	switch decision.ProviderID {
	case domainprovider.ProviderCodex:
		return c.chatCompletionStreamCodexCompat(ctx, decision, req)
	case domainprovider.ProviderCopilot:
		return c.chatCompletionStreamCopilot(ctx, decision, req)
	default:
		return c.chatCompletionStreamOpenAICompatible(ctx, decision, req)
	}
}

func (c *client) chatCompletionStreamCopilot(
	ctx context.Context,
	decision domainprovider.RoutingDecision,
	req domaincompletion.ChatCompletionRequest,
) (<-chan usecasecompletion.StreamChunk, error) {
	// Determine which endpoint to use based on model and feature flag
	endpoint := ResolveCopilotEndpoint(req.Model, c.enableCopilotResponsesRouting)

	if endpoint == CopilotEndpointResponses {
		return c.chatCompletionStreamCopilotResponses(ctx, decision, req)
	}
	return c.chatCompletionStreamCopilotChat(ctx, decision, req)
}

// chatCompletionStreamCopilotChat streams via /chat/completions endpoint.
func (c *client) chatCompletionStreamCopilotChat(
	ctx context.Context,
	decision domainprovider.RoutingDecision,
	req domaincompletion.ChatCompletionRequest,
) (<-chan usecasecompletion.StreamChunk, error) {
	// Map model ID to Copilot-specific format
	copilotReq := req
	copilotReq.Model = GetCopilotModelID(req.Model)
	copilotReq.Stream = true

	body, err := json.Marshal(copilotReq)
	if err != nil {
		return nil, domaincompletion.ErrInvalidJSON(err.Error())
	}

	// Copilot uses /chat/completions without /v1 prefix.
	url := strings.TrimSuffix(decision.BaseURL, "/") + "/chat/completions"

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, domaincompletion.ErrInternalServer(fmt.Sprintf("create request: %v", err))
	}

	applyCopilotHeaders(httpReq, decision, req)
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("Cache-Control", "no-cache")
	httpReq.Header.Set("Connection", "keep-alive")

	c.logger.Debug("sending streaming request to copilot chat",
		zap.String("provider", string(decision.ProviderID)),
		zap.String("url", url),
		zap.String("model", copilotReq.Model),
		zap.String("original_model", req.Model),
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

// chatCompletionStreamCopilotResponses streams via /responses endpoint.
// Used for GPT-5+ models when responses routing is enabled.
// Forwards upstream SSE event payloads as-is (raw passthrough) to preserve all event types
// (response.created, response.output_text.delta, response.completed with usage, etc.).
func (c *client) chatCompletionStreamCopilotResponses(
	ctx context.Context,
	decision domainprovider.RoutingDecision,
	req domaincompletion.ChatCompletionRequest,
) (<-chan usecasecompletion.StreamChunk, error) {
	// Convert to responses API format with streaming enabled
	streamReq := req
	streamReq.Stream = true
	body, err := ChatToResponsesRequest(streamReq)
	if err != nil {
		return nil, domaincompletion.ErrInvalidJSON(err.Error())
	}

	url := strings.TrimSuffix(decision.BaseURL, "/") + "/responses"

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, domaincompletion.ErrInternalServer(fmt.Sprintf("create request: %v", err))
	}

	applyCopilotHeaders(httpReq, decision, req)
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("Cache-Control", "no-cache")
	httpReq.Header.Set("Connection", "keep-alive")

	c.logger.Debug("sending streaming request to copilot responses",
		zap.String("provider", string(decision.ProviderID)),
		zap.String("url", url),
		zap.String("model", GetCopilotModelID(req.Model)),
		zap.String("original_model", req.Model),
		zap.Bool("responses_routing", true),
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

					eventBlock := partial[:idx]
					partial = partial[idx+2:]

					// Parse SSE event block - may contain multiple lines:
					// "event: response.output_text.delta\ndata: {...}"
					// or just "data: {...}"
					data := extractSSEData(eventBlock)
					if data == nil {
						continue
					}

					if bytes.Equal(data, []byte("[DONE]")) {
						chunks <- usecasecompletion.StreamChunk{Done: true}
						return
					}

					// Raw passthrough: forward upstream event payload as-is.
					// This preserves all event types (response.created, response.output_text.delta,
					// response.completed with usage, response.function_call.arguments.delta, etc.)
					// without transformation, allowing clients to receive the original upstream events.
					chunks <- usecasecompletion.StreamChunk{Data: data}
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

// extractSSEData extracts the data payload from an SSE event block.
// SSE event blocks may contain multiple lines like:
//   - "event: response.output_text.delta\ndata: {...}"
//   - "data: {...}"
//   - "id: ...\ndata: {...}"
//
// This function finds and returns the data payload, supporting multi-line data.
func extractSSEData(eventBlock []byte) []byte {
	lines := bytes.Split(eventBlock, []byte("\n"))
	var dataLines [][]byte

	for _, line := range lines {
		line = bytes.TrimSpace(line)
		if bytes.HasPrefix(line, []byte("data:")) {
			// Handle both "data: {...}" and "data:{...}" (with or without space)
			payload := bytes.TrimPrefix(line, []byte("data:"))
			payload = bytes.TrimSpace(payload)
			dataLines = append(dataLines, payload)
		}
		// Ignore "event:", "id:", "retry:" lines - we only need the data
	}

	if len(dataLines) == 0 {
		return nil
	}

	// Join multiple data lines (rare but allowed by SSE spec)
	return bytes.Join(dataLines, []byte("\n"))
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

// applyCopilotHeaders applies Copilot-specific headers to the HTTP request.
// Headers are aligned with reference implementation (copilot-api api-config.ts copilotHeaders).
// Always sets X-Initiator: agent for all requests to enable full agent capabilities.
func applyCopilotHeaders(req *http.Request, decision domainprovider.RoutingDecision, chatReq domaincompletion.ChatCompletionRequest) {
	// Check if vision is enabled (any message has image_url content)
	enableVision := hasVisionContent(chatReq)

	// Set headers aligned with copilotHeaders in api-config.ts
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+decision.Token)
	req.Header.Set("Copilot-Integration-Id", "vscode-chat")
	req.Header.Set("Editor-Version", "vscode/"+defaultVSCodeVersion)
	req.Header.Set("Editor-Plugin-Version", copilotEditorPluginVersion)
	req.Header.Set("User-Agent", copilotUserAgent)
	req.Header.Set("Openai-Intent", "conversation-edits")
	req.Header.Set("X-Github-Api-Version", copilotGitHubAPIVersion)
	req.Header.Set("X-Request-Id", uuid.New().String())
	req.Header.Set("X-Vscode-User-Agent-Library-Version", "electron-fetch")

	// Always set X-Initiator to "agent" for all requests
	// This enables full agent capabilities regardless of message content
	req.Header.Set("X-Initiator", "agent")

	// Set vision header if needed
	if enableVision {
		req.Header.Set("Copilot-Vision-Request", "true")
	}

	// Apply any additional headers from routing decision
	for k, v := range decision.Headers {
		req.Header.Set(k, v)
	}
}

// hasVisionContent checks if any message in the request contains image_url content.
func hasVisionContent(req domaincompletion.ChatCompletionRequest) bool {
	for _, msg := range req.Messages {
		switch parts := msg.Content.(type) {
		case []any:
			for _, part := range parts {
				if isVisionContentPart(part) {
					return true
				}
			}
		case []domaincompletion.ContentPart:
			for _, part := range parts {
				if part.Type == "image_url" && part.ImageURL != nil && strings.TrimSpace(part.ImageURL.URL) != "" {
					return true
				}
			}
		}
	}
	return false
}

func isVisionContentPart(part any) bool {
	switch p := part.(type) {
	case map[string]any:
		typeValue, _ := p["type"].(string)
		return typeValue == "image_url"
	case domaincompletion.ContentPart:
		return p.Type == "image_url" && p.ImageURL != nil && strings.TrimSpace(p.ImageURL.URL) != ""
	default:
		return false
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
