// Package anthropic defines domain entities for Anthropic Claude API format.
// This package provides types for the Anthropic Messages API which uses a different
// structure than OpenAI's Chat Completions API.
package anthropic

// MessagesRequest represents an Anthropic Claude Messages API request.
// Reference: https://docs.anthropic.com/en/api/messages
type MessagesRequest struct {
	Model         string           `json:"model"`
	MaxTokens     int              `json:"max_tokens"`
	System        []SystemBlock    `json:"system,omitempty"`
	Messages      []Message        `json:"messages"`
	Tools         []Tool           `json:"tools,omitempty"`
	ToolChoice    *ToolChoice      `json:"tool_choice,omitempty"`
	Stream        bool             `json:"stream,omitempty"`
	Temperature   *float64         `json:"temperature,omitempty"`
	TopP          *float64         `json:"top_p,omitempty"`
	TopK          *int             `json:"top_k,omitempty"`
	Metadata      *RequestMetadata `json:"metadata,omitempty"`
	StopSequences []string         `json:"stop_sequences,omitempty"`
}

// SystemBlock represents a system message block with optional cache control.
type SystemBlock struct {
	Type         string        `json:"type"` // "text"
	Text         string        `json:"text"`
	CacheControl *CacheControl `json:"cache_control,omitempty"`
}

// CacheControl specifies caching behavior for content blocks.
type CacheControl struct {
	Type string `json:"type"` // "ephemeral"
}

// Message represents a conversation message.
type Message struct {
	Role    string         `json:"role"` // "user" or "assistant"
	Content []ContentBlock `json:"content"`
}

// ContentBlock represents a content block in a message.
// Can be text, image, tool_use, or tool_result.
type ContentBlock struct {
	Type         string        `json:"type"` // "text", "image", "tool_use", "tool_result"
	Text         string        `json:"text,omitempty"`
	CacheControl *CacheControl `json:"cache_control,omitempty"`

	// For tool_use blocks
	ID    string `json:"id,omitempty"`
	Name  string `json:"name,omitempty"`
	Input any    `json:"input,omitempty"`

	// For tool_result blocks
	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   any    `json:"content,omitempty"` // string or []ContentBlock
	IsError   bool   `json:"is_error,omitempty"`

	// For image blocks
	Source *ImageSource `json:"source,omitempty"`
}

// ImageSource represents an image source in an image content block.
type ImageSource struct {
	Type      string `json:"type"` // "base64" or "url"
	MediaType string `json:"media_type,omitempty"`
	Data      string `json:"data,omitempty"`
	URL       string `json:"url,omitempty"`
}

// Tool represents a tool definition.
type Tool struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	InputSchema any    `json:"input_schema"` // JSON Schema
}

// ToolChoice specifies how tools should be used.
type ToolChoice struct {
	Type string `json:"type"`           // "auto", "any", "tool", "none"
	Name string `json:"name,omitempty"` // Required when type is "tool"
}

// RequestMetadata contains optional metadata for the request.
type RequestMetadata struct {
	UserID string `json:"user_id,omitempty"`
}

// MessagesResponse represents an Anthropic Claude Messages API response.
type MessagesResponse struct {
	ID           string            `json:"id"`
	Type         string            `json:"type"` // "message"
	Role         string            `json:"role"` // "assistant"
	Model        string            `json:"model"`
	Content      []ResponseContent `json:"content"`
	StopReason   string            `json:"stop_reason,omitempty"` // "end_turn", "tool_use", "max_tokens", "stop_sequence"
	StopSequence string            `json:"stop_sequence,omitempty"`
	Usage        *Usage            `json:"usage,omitempty"`
}

// ResponseContent represents a content block in the response.
type ResponseContent struct {
	Type  string `json:"type"` // "text" or "tool_use"
	Text  string `json:"text,omitempty"`
	ID    string `json:"id,omitempty"`    // For tool_use
	Name  string `json:"name,omitempty"`  // For tool_use
	Input any    `json:"input,omitempty"` // For tool_use
}

// Usage represents token usage statistics.
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// StreamEvent represents a streaming SSE event from the Anthropic API.
// Events are sent as: event: {type}\ndata: {json}\n\n
type StreamEvent struct {
	Type string `json:"type"`
	// Fields vary by event type - handled by specific event structs below
}

// MessageStartEvent is sent at the start of a message.
type MessageStartEvent struct {
	Type    string               `json:"type"` // "message_start"
	Message *MessageStartPayload `json:"message"`
}

// MessageStartPayload contains the initial message metadata.
type MessageStartPayload struct {
	ID           string  `json:"id"`
	Type         string  `json:"type"` // "message"
	Role         string  `json:"role"` // "assistant"
	Model        string  `json:"model"`
	Content      []any   `json:"content"`
	StopReason   *string `json:"stop_reason"`
	StopSequence *string `json:"stop_sequence"`
	Usage        *Usage  `json:"usage"`
}

// ContentBlockStartEvent is sent when a new content block starts.
type ContentBlockStartEvent struct {
	Type         string               `json:"type"` // "content_block_start"
	Index        int                  `json:"index"`
	ContentBlock *ContentBlockPayload `json:"content_block"`
}

// ContentBlockPayload represents a content block in streaming.
type ContentBlockPayload struct {
	Type  string `json:"type"` // "text" or "tool_use"
	Text  string `json:"text,omitempty"`
	ID    string `json:"id,omitempty"`
	Name  string `json:"name,omitempty"`
	Input any    `json:"input,omitempty"`
}

// ContentBlockDeltaEvent is sent for incremental content updates.
type ContentBlockDeltaEvent struct {
	Type  string `json:"type"` // "content_block_delta"
	Index int    `json:"index"`
	Delta *Delta `json:"delta"`
}

// Delta represents an incremental update to a content block.
type Delta struct {
	Type        string `json:"type"` // "text_delta" or "input_json_delta"
	Text        string `json:"text,omitempty"`
	PartialJSON string `json:"partial_json,omitempty"`
}

// ContentBlockStopEvent is sent when a content block is complete.
type ContentBlockStopEvent struct {
	Type  string `json:"type"` // "content_block_stop"
	Index int    `json:"index"`
}

// MessageDeltaEvent is sent for updates to the overall message.
type MessageDeltaEvent struct {
	Type  string               `json:"type"` // "message_delta"
	Delta *MessageDeltaPayload `json:"delta"`
	Usage *DeltaUsage          `json:"usage,omitempty"`
}

// MessageDeltaPayload contains the delta updates to the message.
type MessageDeltaPayload struct {
	StopReason   string  `json:"stop_reason,omitempty"`
	StopSequence *string `json:"stop_sequence"` // Always included, null when not set
}

// DeltaUsage contains usage information in delta events.
type DeltaUsage struct {
	InputTokens          int `json:"input_tokens,omitempty"`
	OutputTokens         int `json:"output_tokens"`
	CacheReadInputTokens int `json:"cache_read_input_tokens,omitempty"`
	CacheCreationTokens  int `json:"cache_creation_input_tokens,omitempty"`
}

// MessageStopEvent is sent when the message is complete.
type MessageStopEvent struct {
	Type string `json:"type"` // "message_stop"
}

// ErrorEvent is sent when an error occurs.
type ErrorEvent struct {
	Type  string       `json:"type"` // "error"
	Error *ErrorDetail `json:"error"`
}

// ErrorDetail contains error information.
type ErrorDetail struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// PingEvent is sent periodically to keep the connection alive.
type PingEvent struct {
	Type string `json:"type"` // "ping"
}
