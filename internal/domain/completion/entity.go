// Package completion defines domain entities for OpenAI-compatible completion API.
package completion

import "time"

// ChatCompletionRequest represents an OpenAI-compatible chat completion request.
// This is the exact format expected from clients (Cursor, OpenCode, etc.).
type ChatCompletionRequest struct {
	Model            string         `json:"model"`
	Messages         []Message      `json:"messages"`
	Temperature      *float64       `json:"temperature,omitempty"`
	TopP             *float64       `json:"top_p,omitempty"`
	N                *int           `json:"n,omitempty"`
	Stream           bool           `json:"stream,omitempty"`
	Stop             any            `json:"stop,omitempty"` // string or []string
	MaxTokens        *int           `json:"max_tokens,omitempty"`
	PresencePenalty  *float64       `json:"presence_penalty,omitempty"`
	FrequencyPenalty *float64       `json:"frequency_penalty,omitempty"`
	LogitBias        map[string]int `json:"logit_bias,omitempty"`
	User             string         `json:"user,omitempty"`
	// Additional fields for tool/function calling
	Tools          []Tool          `json:"tools,omitempty"`
	ToolChoice     any             `json:"tool_choice,omitempty"` // string or object
	Functions      []Function      `json:"functions,omitempty"`   // Deprecated but still used
	FunctionCall   any             `json:"function_call,omitempty"`
	ResponseFormat *ResponseFormat `json:"response_format,omitempty"`

	// ProviderHint is an internal field (not from JSON) that forces routing to a specific provider.
	// Set via X-Provider header or provider prefix in model name (e.g., "copilot/gpt-5").
	// This field is populated by the handler before passing to the service.
	ProviderHint string `json:"-"`

	// RequestID is an internal field (not from JSON) for tracking/logging.
	// Set from X-Request-ID header by the handler.
	RequestID string `json:"-"`
}

// Message represents a chat message.
type Message struct {
	Role       string     `json:"role"`
	Content    any        `json:"content"` // string or []ContentPart
	Name       string     `json:"name,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// ContentPart represents a content part (text or image).
type ContentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *ImageURL `json:"image_url,omitempty"`
}

// ImageURL represents an image URL in a content part.
type ImageURL struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"`
}

// Tool represents a tool definition.
type Tool struct {
	Type     string   `json:"type"`
	Function Function `json:"function"`
}

// Function represents a function definition.
type Function struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Parameters  any    `json:"parameters,omitempty"` // JSON Schema object
}

// ToolCall represents a tool call in an assistant message.
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

// FunctionCall represents a function call.
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ResponseFormat specifies the output format.
type ResponseFormat struct {
	Type string `json:"type"` // "text" or "json_object"
}

// ChatCompletionResponse represents an OpenAI-compatible chat completion response.
type ChatCompletionResponse struct {
	ID                string   `json:"id"`
	Object            string   `json:"object"` // "chat.completion"
	Created           int64    `json:"created"`
	Model             string   `json:"model"`
	Choices           []Choice `json:"choices"`
	Usage             *Usage   `json:"usage,omitempty"`
	SystemFingerprint string   `json:"system_fingerprint,omitempty"`
}

// Choice represents a completion choice.
type Choice struct {
	Index        int      `json:"index"`
	Message      *Message `json:"message,omitempty"`       // For non-streaming
	Delta        *Message `json:"delta,omitempty"`         // For streaming
	FinishReason string   `json:"finish_reason,omitempty"` // "stop", "length", "tool_calls", "content_filter"
	Logprobs     any      `json:"logprobs,omitempty"`
}

// Usage represents token usage statistics.
type Usage struct {
	PromptTokens            int                  `json:"prompt_tokens"`
	CompletionTokens        int                  `json:"completion_tokens"`
	TotalTokens             int                  `json:"total_tokens"`
	PromptTokensDetails     *PromptTokensDetails `json:"prompt_tokens_details,omitempty"`
	InputTokensDetails      *PromptTokensDetails `json:"input_tokens_details,omitempty"`
	CompletionTokensDetails *CompletionDetails   `json:"completion_tokens_details,omitempty"`
}

// PromptTokensDetails contains prompt/input token breakdown.
type PromptTokensDetails struct {
	CachedTokens int `json:"cached_tokens,omitempty"`
}

// CompletionDetails contains completion token breakdown.
type CompletionDetails struct {
	ReasoningTokens int `json:"reasoning_tokens,omitempty"`
}

// CachedTokens returns cached input tokens from known provider response fields.
func (u *Usage) CachedTokens() int {
	if u == nil {
		return 0
	}
	if u.PromptTokensDetails != nil {
		return u.PromptTokensDetails.CachedTokens
	}
	if u.InputTokensDetails != nil {
		return u.InputTokensDetails.CachedTokens
	}
	return 0
}

// ChatCompletionChunk represents a streaming chunk.
type ChatCompletionChunk struct {
	ID                string   `json:"id"`
	Object            string   `json:"object"` // "chat.completion.chunk"
	Created           int64    `json:"created"`
	Model             string   `json:"model"`
	Choices           []Choice `json:"choices"`
	SystemFingerprint string   `json:"system_fingerprint,omitempty"`
}

// Model represents a model in the /v1/models response.
type Model struct {
	ID      string `json:"id"`
	Object  string `json:"object"` // "model"
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

// ModelsResponse represents the /v1/models response.
type ModelsResponse struct {
	Object string  `json:"object"` // "list"
	Data   []Model `json:"data"`
}

// ProviderAttempt records an attempt to a provider for debugging/logging.
type ProviderAttempt struct {
	ProviderID string
	StartedAt  time.Time
	EndedAt    time.Time
	StatusCode int
	Error      error
	Success    bool
}

// RoutingContext holds context for a single completion request routing.
type RoutingContext struct {
	RequestID        string
	Model            string
	UserID           string // For future per-user mapping
	Attempts         []ProviderAttempt
	SelectedProvider string
}
