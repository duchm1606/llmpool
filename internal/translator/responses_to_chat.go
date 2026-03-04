// Package translator provides format conversion between OpenAI API formats.
// It supports converting between OpenAI Responses API format (used by Cursor IDE)
// and standard OpenAI Chat Completions format.
package translator

import (
	"encoding/json"
	"strings"
)

// ResponsesRequest represents an OpenAI Responses API request (used by Cursor).
type ResponsesRequest struct {
	Model             string          `json:"model"`
	Input             json.RawMessage `json:"input"` // Can be string or []InputItem
	Instructions      string          `json:"instructions,omitempty"`
	Tools             []ResponsesTool `json:"tools,omitempty"`
	ToolChoice        any             `json:"tool_choice,omitempty"`
	MaxOutputTokens   *int            `json:"max_output_tokens,omitempty"`
	ParallelToolCalls *bool           `json:"parallel_tool_calls,omitempty"`
	Stream            bool            `json:"stream,omitempty"`
	Store             bool            `json:"store,omitempty"`
	Reasoning         *Reasoning      `json:"reasoning,omitempty"`
	Metadata          map[string]any  `json:"metadata,omitempty"`
	User              string          `json:"user,omitempty"`
	Temperature       *float64        `json:"temperature,omitempty"`
	TopP              *float64        `json:"top_p,omitempty"`
	// Additional fields for Codex compatibility
	Include           []string        `json:"include,omitempty"`            // e.g., ["reasoning.encrypted_content"]
	ContextManagement json.RawMessage `json:"context_management,omitempty"` // Stripped for Codex
	ServiceTier       string          `json:"service_tier,omitempty"`       // Stripped for Codex
	Truncation        string          `json:"truncation,omitempty"`         // Stripped for Codex
}

// Reasoning represents reasoning configuration in Responses format.
type Reasoning struct {
	Effort string `json:"effort,omitempty"` // "low", "medium", "high"
}

// InputItem represents an item in the input array.
type InputItem struct {
	Type      string          `json:"type,omitempty"` // "message", "function_call", "function_call_output"
	Role      string          `json:"role,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"` // Can be string or []ContentPart
	CallID    string          `json:"call_id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Arguments string          `json:"arguments,omitempty"`
	Output    string          `json:"output,omitempty"`
}

// ContentPart represents a content part in the input.
type ContentPart struct {
	Type     string `json:"type"` // "input_text", "output_text", "input_image"
	Text     string `json:"text,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
}

// ResponsesTool represents a tool in Responses format.
type ResponsesTool struct {
	Type        string `json:"type"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Parameters  any    `json:"parameters,omitempty"`
}

// ChatCompletionsRequest represents a standard OpenAI Chat Completions request.
type ChatCompletionsRequest struct {
	Model             string        `json:"model"`
	Messages          []ChatMessage `json:"messages"`
	Stream            bool          `json:"stream,omitempty"`
	MaxTokens         *int          `json:"max_tokens,omitempty"`
	Temperature       *float64      `json:"temperature,omitempty"`
	TopP              *float64      `json:"top_p,omitempty"`
	Tools             []ChatTool    `json:"tools,omitempty"`
	ToolChoice        any           `json:"tool_choice,omitempty"`
	ParallelToolCalls *bool         `json:"parallel_tool_calls,omitempty"`
	ReasoningEffort   string        `json:"reasoning_effort,omitempty"`
	User              string        `json:"user,omitempty"`
}

// ChatMessage represents a message in Chat Completions format.
type ChatMessage struct {
	Role       string         `json:"role"`
	Content    any            `json:"content,omitempty"` // string or []ChatContentPart
	ToolCalls  []ChatToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
}

// ChatContentPart represents a content part in Chat Completions format.
type ChatContentPart struct {
	Type     string        `json:"type"`
	Text     string        `json:"text,omitempty"`
	ImageURL *ChatImageURL `json:"image_url,omitempty"`
}

// ChatImageURL represents an image URL in Chat Completions format.
type ChatImageURL struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"`
}

// ChatTool represents a tool in Chat Completions format.
type ChatTool struct {
	Type     string       `json:"type"`
	Function ChatFunction `json:"function"`
}

// ChatFunction represents a function definition in Chat Completions format.
type ChatFunction struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Parameters  any    `json:"parameters,omitempty"`
}

// ChatToolCall represents a tool call in Chat Completions format.
type ChatToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function ChatFunctionCall `json:"function"`
}

// ChatFunctionCall represents a function call in Chat Completions format.
type ChatFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ConvertResponsesToChat converts an OpenAI Responses request to Chat Completions format.
func ConvertResponsesToChat(req *ResponsesRequest) (*ChatCompletionsRequest, error) {
	out := &ChatCompletionsRequest{
		Model:       req.Model,
		Stream:      req.Stream,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		User:        req.User,
	}

	if req.MaxOutputTokens != nil {
		out.MaxTokens = req.MaxOutputTokens
	}

	if req.ParallelToolCalls != nil {
		out.ParallelToolCalls = req.ParallelToolCalls
	}

	if req.Reasoning != nil && req.Reasoning.Effort != "" {
		out.ReasoningEffort = strings.ToLower(strings.TrimSpace(req.Reasoning.Effort))
	}

	// Convert instructions to system message
	if req.Instructions != "" {
		out.Messages = append(out.Messages, ChatMessage{
			Role:    "system",
			Content: req.Instructions,
		})
	}

	// Parse and convert input
	if err := convertInput(req.Input, &out.Messages); err != nil {
		return nil, err
	}

	// Convert tools
	if len(req.Tools) > 0 {
		out.Tools = convertTools(req.Tools)
	}

	if req.ToolChoice != nil {
		out.ToolChoice = req.ToolChoice
	}

	return out, nil
}

func convertInput(inputRaw json.RawMessage, messages *[]ChatMessage) error {
	if len(inputRaw) == 0 {
		return nil
	}

	// Try parsing as string first
	var inputStr string
	if err := json.Unmarshal(inputRaw, &inputStr); err == nil {
		*messages = append(*messages, ChatMessage{
			Role:    "user",
			Content: inputStr,
		})
		return nil
	}

	// Parse as array
	var items []InputItem
	if err := json.Unmarshal(inputRaw, &items); err != nil {
		return err
	}

	for _, item := range items {
		itemType := item.Type
		if itemType == "" && item.Role != "" {
			itemType = "message"
		}

		switch itemType {
		case "message", "":
			msg := convertMessageItem(item)
			*messages = append(*messages, msg)

		case "function_call":
			// Convert to assistant message with tool_calls
			*messages = append(*messages, ChatMessage{
				Role: "assistant",
				ToolCalls: []ChatToolCall{{
					ID:   item.CallID,
					Type: "function",
					Function: ChatFunctionCall{
						Name:      item.Name,
						Arguments: item.Arguments,
					},
				}},
			})

		case "function_call_output":
			// Convert to tool message
			*messages = append(*messages, ChatMessage{
				Role:       "tool",
				ToolCallID: item.CallID,
				Content:    item.Output,
			})
		}
	}

	return nil
}

func convertMessageItem(item InputItem) ChatMessage {
	role := item.Role
	// Map developer role to user for providers that don't support it
	if role == "developer" {
		role = "user"
	}

	msg := ChatMessage{Role: role}

	// Parse content
	if len(item.Content) == 0 {
		return msg
	}

	// Try parsing as string
	var contentStr string
	if err := json.Unmarshal(item.Content, &contentStr); err == nil {
		msg.Content = contentStr
		return msg
	}

	// Parse as array of content parts
	var parts []ContentPart
	if err := json.Unmarshal(item.Content, &parts); err != nil {
		return msg
	}

	// Convert content parts
	chatParts := make([]ChatContentPart, 0, len(parts))
	for _, part := range parts {
		switch part.Type {
		case "input_text", "output_text", "text":
			chatParts = append(chatParts, ChatContentPart{
				Type: "text",
				Text: part.Text,
			})
		case "input_image":
			chatParts = append(chatParts, ChatContentPart{
				Type: "image_url",
				ImageURL: &ChatImageURL{
					URL: part.ImageURL,
				},
			})
		}
	}

	if len(chatParts) == 1 && chatParts[0].Type == "text" {
		// Simplify to string if only one text part
		msg.Content = chatParts[0].Text
	} else if len(chatParts) > 0 {
		msg.Content = chatParts
	}

	return msg
}

func convertTools(tools []ResponsesTool) []ChatTool {
	result := make([]ChatTool, 0, len(tools))
	for _, tool := range tools {
		// Only convert function tools; skip built-in tools like web_search
		if tool.Type != "function" && tool.Type != "" {
			continue
		}

		result = append(result, ChatTool{
			Type: "function",
			Function: ChatFunction{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  tool.Parameters,
			},
		})
	}
	return result
}
