package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	domaincompletion "github.com/duchoang/llmpool/internal/domain/completion"
	"github.com/duchoang/llmpool/internal/infra/provider"
	usecasecompletion "github.com/duchoang/llmpool/internal/usecase/completion"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// ProviderHeader is the HTTP header used to force routing to a specific provider.
// Example: X-Provider: copilot
const ProviderHeader = "X-Provider"

// ChatHandler handles chat completion requests.
type ChatHandler struct {
	service usecasecompletion.CompletionService
	logger  *zap.Logger
}

// NewChatHandler creates a new chat handler.
func NewChatHandler(service usecasecompletion.CompletionService, logger *zap.Logger) *ChatHandler {
	return &ChatHandler{
		service: service,
		logger:  logger,
	}
}

// ChatCompletion handles POST /v1/chat/completions
func (h *ChatHandler) ChatCompletion(c *gin.Context) {
	// Read raw body first (for logging/debugging)
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		h.respondError(c, domaincompletion.ErrInvalidJSON("failed to read request body"))
		return
	}

	// Parse request
	var req domaincompletion.ChatCompletionRequest
	if err := json.Unmarshal(body, &req); err != nil {
		h.respondError(c, domaincompletion.ErrInvalidJSON(err.Error()))
		return
	}

	// Extract provider hint from header or model prefix
	req.ProviderHint = h.extractProviderHint(c, &req)

	h.logger.Debug("chat completion request",
		zap.String("model", req.Model),
		zap.String("provider_hint", req.ProviderHint),
		zap.Bool("stream", req.Stream),
		zap.Int("messages", len(req.Messages)),
	)

	// Handle streaming vs non-streaming
	if req.Stream {
		h.handleStreaming(c, req)
		return
	}

	h.handleNonStreaming(c, req)
}

// extractProviderHint extracts the provider hint from header or model prefix.
// Priority: X-Provider header > model prefix (e.g., "copilot/gpt-5")
func (h *ChatHandler) extractProviderHint(c *gin.Context, req *domaincompletion.ChatCompletionRequest) string {
	// Check X-Provider header first (highest priority)
	if headerProvider := c.GetHeader(ProviderHeader); headerProvider != "" {
		h.logger.Debug("provider hint from header",
			zap.String("provider", headerProvider),
		)
		return headerProvider
	}

	// Check for provider prefix in model name (e.g., "copilot/gpt-5")
	parsed := provider.ParseModelWithProvider(req.Model)
	if parsed.Provider != "" {
		h.logger.Debug("provider hint from model prefix",
			zap.String("original_model", req.Model),
			zap.String("provider", parsed.Provider),
			zap.String("model", parsed.Model),
		)
		// Update the model in request to the unprefixed version
		req.Model = parsed.Model
		return parsed.Provider
	}

	return ""
}

// handleNonStreaming handles non-streaming chat completion.
func (h *ChatHandler) handleNonStreaming(c *gin.Context, req domaincompletion.ChatCompletionRequest) {
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

	c.JSON(http.StatusOK, resp)
}

// handleStreaming handles streaming chat completion.
func (h *ChatHandler) handleStreaming(c *gin.Context, req domaincompletion.ChatCompletionRequest) {
	ctx := c.Request.Context()

	// Set SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Transfer-Encoding", "chunked")
	c.Header("X-Accel-Buffering", "no") // Disable nginx buffering

	// Get the response writer
	writer := c.Writer

	// Flush headers
	writer.Flush()

	// Stream completion
	err := h.service.ChatCompletionStream(ctx, req, writer)
	if err != nil {
		var apiErr *domaincompletion.APIError
		if errors.As(err, &apiErr) {
			// For streaming, we can only write error if headers haven't been sent
			// In practice, we should have already started streaming, so we log the error
			h.logger.Error("streaming error",
				zap.Error(err),
				zap.String("model", req.Model),
			)
			// Try to send error as SSE event
			errData, _ := json.Marshal(domaincompletion.APIErrorResponse{Error: *apiErr})
			_, _ = c.Writer.WriteString("data: " + string(errData) + "\n\n")
			_, _ = c.Writer.WriteString("data: [DONE]\n\n")
			c.Writer.Flush()
			return
		}
		h.logger.Error("streaming error",
			zap.Error(err),
			zap.String("model", req.Model),
		)
	}
}

// respondError writes an OpenAI-compatible error response.
func (h *ChatHandler) respondError(c *gin.Context, apiErr *domaincompletion.APIError) {
	c.JSON(apiErr.HTTPStatus, domaincompletion.APIErrorResponse{
		Error: *apiErr,
	})
}

// Completion handles POST /v1/completions (legacy text completions)
// This is less commonly used but included for compatibility.
func (h *ChatHandler) Completion(c *gin.Context) {
	// For now, return an error indicating this endpoint is not yet implemented
	// Most modern clients use /v1/chat/completions
	h.respondError(c, domaincompletion.NewAPIError(
		http.StatusNotImplemented,
		domaincompletion.ErrorTypeInvalidRequest,
		"The /v1/completions endpoint is not yet implemented. Please use /v1/chat/completions instead.",
	))
}
