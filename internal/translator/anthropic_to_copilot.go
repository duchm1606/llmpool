// Package translator provides format conversion between Anthropic and Copilot API formats.
package translator

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/duchoang/llmpool/internal/domain/anthropic"
)

// AnthropicToCopilotResponses converts an Anthropic Messages request to Copilot Responses API format.
// The Copilot Responses API uses a structure similar to OpenAI's Responses API.
func AnthropicToCopilotResponses(req *anthropic.MessagesRequest) ([]byte, error) {
	payload := map[string]any{
		"model": req.Model,
		"store": false,
	}

	if req.Stream {
		payload["stream"] = true
	}

	// Convert system blocks to instructions
	if len(req.System) > 0 {
		var sb strings.Builder
		for i, block := range req.System {
			if i > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString(block.Text)
		}
		payload["instructions"] = sb.String()
	}

	// Convert messages to input array
	input := make([]any, 0, len(req.Messages)*2) // May expand for tool calls
	for _, msg := range req.Messages {
		switch msg.Role {
		case "user":
			input = append(input, convertUserMessage(msg))
		case "assistant":
			// Assistant messages may contain text and tool_use blocks
			input = append(input, convertAssistantMessage(msg)...)
		}
	}
	payload["input"] = input

	// Convert tools
	if len(req.Tools) > 0 {
		tools := make([]any, 0, len(req.Tools))
		for _, t := range req.Tools {
			tool := map[string]any{
				"type": "function",
				"name": t.Name,
			}
			if t.Description != "" {
				tool["description"] = t.Description
			}
			if t.InputSchema != nil {
				tool["parameters"] = t.InputSchema
			}
			tools = append(tools, tool)
		}
		payload["tools"] = tools
	}

	// Convert tool_choice
	if req.ToolChoice != nil {
		payload["tool_choice"] = convertToolChoice(req.ToolChoice)
	}

	// Temperature and other params
	if req.Temperature != nil {
		payload["temperature"] = *req.Temperature
	}
	if req.TopP != nil {
		payload["top_p"] = *req.TopP
	}
	if req.MaxTokens > 0 {
		payload["max_output_tokens"] = req.MaxTokens
	}
	if req.Metadata != nil && req.Metadata.UserID != "" {
		payload["user"] = req.Metadata.UserID
	}

	return json.Marshal(payload)
}

// convertUserMessage converts an Anthropic user message to Copilot input format.
func convertUserMessage(msg anthropic.Message) map[string]any {
	content := make([]map[string]any, 0, len(msg.Content))

	for _, block := range msg.Content {
		switch block.Type {
		case "text":
			content = append(content, map[string]any{
				"type": "input_text",
				"text": block.Text,
			})
		case "image":
			if block.Source != nil {
				switch block.Source.Type {
				case "base64":
					// Convert base64 to data URL
					dataURL := fmt.Sprintf("data:%s;base64,%s", block.Source.MediaType, block.Source.Data)
					content = append(content, map[string]any{
						"type":      "input_image",
						"image_url": dataURL,
					})
				case "url":
					content = append(content, map[string]any{
						"type":      "input_image",
						"image_url": block.Source.URL,
					})
				}
			}
		case "tool_result":
			// Tool results become function_call_output items at the top level
			// We'll handle these separately
		}
	}

	// Check for tool_result blocks - they need to be converted to function_call_output
	// and returned separately, but for simplicity we include inline
	// Actually, tool_result in user messages should be separate input items
	return map[string]any{
		"type":    "message",
		"role":    "user",
		"content": content,
	}
}

// convertAssistantMessage converts an Anthropic assistant message to Copilot input format.
// This may return multiple input items (message + function_calls).
func convertAssistantMessage(msg anthropic.Message) []any {
	result := make([]any, 0, len(msg.Content))
	textContent := make([]map[string]any, 0)

	for _, block := range msg.Content {
		switch block.Type {
		case "text":
			textContent = append(textContent, map[string]any{
				"type": "output_text",
				"text": block.Text,
			})
		case "tool_use":
			// Add any accumulated text content first
			if len(textContent) > 0 {
				result = append(result, map[string]any{
					"type":    "message",
					"role":    "assistant",
					"content": textContent,
				})
				textContent = make([]map[string]any, 0)
			}

			// Convert tool_use to function_call
			args := ""
			if block.Input != nil {
				if argsBytes, err := json.Marshal(block.Input); err == nil {
					args = string(argsBytes)
				}
			}
			result = append(result, map[string]any{
				"type":      "function_call",
				"call_id":   block.ID,
				"name":      block.Name,
				"arguments": args,
			})
		}
	}

	// Add remaining text content
	if len(textContent) > 0 {
		result = append(result, map[string]any{
			"type":    "message",
			"role":    "assistant",
			"content": textContent,
		})
	}

	return result
}

// convertToolChoice converts Anthropic tool_choice to Copilot format.
func convertToolChoice(tc *anthropic.ToolChoice) any {
	if tc == nil {
		return nil
	}
	switch tc.Type {
	case "auto":
		return "auto"
	case "any":
		return "required"
	case "tool":
		return map[string]any{
			"type": "function",
			"function": map[string]string{
				"name": tc.Name,
			},
		}
	case "none":
		return "none"
	default:
		return "auto"
	}
}

// ConvertToolResultToFunctionOutput converts a tool_result content block to function_call_output.
func ConvertToolResultToFunctionOutput(block anthropic.ContentBlock) map[string]any {
	output := ""
	switch content := block.Content.(type) {
	case string:
		output = content
	case []anthropic.ContentBlock:
		// Extract text from content blocks
		var sb strings.Builder
		for _, cb := range content {
			if cb.Type == "text" {
				sb.WriteString(cb.Text)
			}
		}
		output = sb.String()
	default:
		if content != nil {
			if bytes, err := json.Marshal(content); err == nil {
				output = string(bytes)
			}
		}
	}

	return map[string]any{
		"type":    "function_call_output",
		"call_id": block.ToolUseID,
		"output":  output,
	}
}

// AnthropicRequestFromJSON parses an Anthropic Messages request from JSON.
// This handles the flexible content format where content can be string or array.
func AnthropicRequestFromJSON(data []byte) (*anthropic.MessagesRequest, error) {
	var req anthropic.MessagesRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("parse anthropic request: %w", err)
	}
	return &req, nil
}

// ParseAnthropicMessages parses messages with flexible content handling.
// Anthropic allows content to be either a string or an array of content blocks.
func ParseAnthropicMessages(data []byte) ([]anthropic.Message, error) {
	// First try parsing as structured messages
	var messages []anthropic.Message
	if err := json.Unmarshal(data, &messages); err == nil {
		return messages, nil
	}

	// Try parsing with raw content handling
	var rawMessages []struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(data, &rawMessages); err != nil {
		return nil, err
	}

	messages = make([]anthropic.Message, 0, len(rawMessages))
	for _, raw := range rawMessages {
		msg := anthropic.Message{Role: raw.Role}

		// Try parsing content as string first
		var textContent string
		if err := json.Unmarshal(raw.Content, &textContent); err == nil {
			msg.Content = []anthropic.ContentBlock{{
				Type: "text",
				Text: textContent,
			}}
		} else {
			// Parse as array of content blocks
			var blocks []anthropic.ContentBlock
			if err := json.Unmarshal(raw.Content, &blocks); err != nil {
				return nil, fmt.Errorf("parse content for message: %w", err)
			}
			msg.Content = blocks
		}

		messages = append(messages, msg)
	}

	return messages, nil
}
