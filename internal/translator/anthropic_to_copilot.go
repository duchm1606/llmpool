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
		"model":               req.Model,
		"store":               false,
		"parallel_tool_calls": true,
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
			input = append(input, convertUserMessage(msg)...)
		case "assistant":
			// Assistant messages may contain text, tool_use, and thinking blocks
			input = append(input, convertAssistantMessage(msg)...)
		}
	}
	payload["input"] = input

	// Convert tools
	if len(req.Tools) > 0 {
		tools := make([]any, 0, len(req.Tools))
		for _, t := range req.Tools {
			tool := map[string]any{
				"type":   "function",
				"name":   t.Name,
				"strict": false,
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

	// Temperature - for reasoning models, fix to 1
	if req.Thinking != nil && (req.Thinking.Type == "enabled" || req.Thinking.Type == "adaptive") {
		payload["temperature"] = 1
	} else if req.Temperature != nil {
		payload["temperature"] = *req.Temperature
	}

	if req.TopP != nil {
		payload["top_p"] = *req.TopP
	}

	// Max output tokens - ensure minimum for reasoning
	maxTokens := req.MaxTokens
	if maxTokens > 0 {
		if maxTokens < 12800 {
			maxTokens = 12800 // Minimum for reasoning models
		}
		payload["max_output_tokens"] = maxTokens
	}

	if req.Metadata != nil && req.Metadata.UserID != "" {
		payload["user"] = req.Metadata.UserID
	}

	// Add reasoning config for models that support it
	if req.Thinking != nil || req.OutputConfig != nil {
		reasoning := map[string]any{
			"summary": "detailed",
		}

		// Determine effort level
		var effort string
		if req.OutputConfig != nil && req.OutputConfig.Effort != "" {
			effort = req.OutputConfig.Effort
		} else {
			effort = GetEffortForModel(req.Model)
		}
		reasoning["effort"] = effort

		payload["reasoning"] = reasoning
		payload["include"] = []string{"reasoning.encrypted_content"}
	}

	return json.Marshal(payload)
}

// convertUserMessage converts an Anthropic user message to Copilot input format.
// Tool results are emitted as function_call_output items before any user message item.
func convertUserMessage(msg anthropic.Message) []any {
	content := make([]map[string]any, 0, len(msg.Content))
	result := make([]any, 0, len(msg.Content)+1)

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
			result = append(result, ConvertToolResultToFunctionOutput(block))
		}
	}

	if len(content) > 0 {
		result = append(result, map[string]any{
			"type":    "message",
			"role":    "user",
			"content": content,
		})
	}

	if len(result) == 0 {
		result = append(result, map[string]any{
			"type":    "message",
			"role":    "user",
			"content": []map[string]any{},
		})
	}

	return result
}

// convertAssistantMessage converts an Anthropic assistant message to Copilot input format.
// This may return multiple input items (message + function_calls + reasoning).
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
		case "thinking":
			// Handle thinking blocks - only if signature contains "@" (GPT format)
			if block.Signature != "" && strings.Contains(block.Signature, "@") {
				// Flush any accumulated text content first
				if len(textContent) > 0 {
					result = append(result, map[string]any{
						"type":    "message",
						"role":    "assistant",
						"content": textContent,
					})
					textContent = make([]map[string]any, 0)
				}

				// Convert thinking to reasoning content
				result = append(result, convertThinkingToReasoning(block))
			}
			// Skip non-GPT thinking blocks (Claude native thinking)
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
				"status":    "completed",
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

// convertThinkingToReasoning converts an Anthropic thinking block to Copilot reasoning format.
// The signature format is "encrypted_content@id" for GPT models.
func convertThinkingToReasoning(block anthropic.ContentBlock) map[string]any {
	// Parse signature@id format
	parts := strings.SplitN(block.Signature, "@", 2)
	encryptedContent := parts[0]
	var id string
	if len(parts) > 1 {
		id = parts[1]
	}

	// Clean up thinking text - if it's the placeholder, use empty
	thinking := block.Thinking
	if thinking == ThinkingTextPlaceholder {
		thinking = ""
	}

	reasoning := map[string]any{
		"type":              "reasoning",
		"encrypted_content": encryptedContent,
	}

	if id != "" {
		reasoning["id"] = id
	}

	// Add summary if thinking text is present
	if thinking != "" {
		reasoning["summary"] = []map[string]any{
			{
				"type": "summary_text",
				"text": thinking,
			},
		}
	} else {
		reasoning["summary"] = []map[string]any{}
	}

	return reasoning
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
