// Package translator provides format conversion between Anthropic and Copilot API formats.
package translator

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/duchoang/llmpool/internal/domain/anthropic"
	domaincompletion "github.com/duchoang/llmpool/internal/domain/completion"
)

// AnthropicToChatCompletion converts an Anthropic Messages request to OpenAI Chat Completion format.
// This is used for Claude models and other models that require /chat/completions endpoint.
func AnthropicToChatCompletion(req *anthropic.MessagesRequest) ([]byte, error) {
	chatReq := domaincompletion.ChatCompletionRequest{
		Model:  req.Model,
		Stream: req.Stream,
	}

	// Build messages array
	messages := make([]domaincompletion.Message, 0, len(req.Messages)+1)

	// Add system message if present
	if len(req.System) > 0 {
		var sb strings.Builder
		for i, block := range req.System {
			if i > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString(block.Text)
		}
		messages = append(messages, domaincompletion.Message{
			Role:    "system",
			Content: sb.String(),
		})
	}

	// Convert Anthropic messages to OpenAI format
	for _, msg := range req.Messages {
		switch msg.Role {
		case "user":
			chatMsg := convertUserMessageToChat(msg)
			messages = append(messages, chatMsg)
		case "assistant":
			// Assistant messages may contain text and tool_use blocks
			chatMsgs := convertAssistantMessageToChat(msg)
			messages = append(messages, chatMsgs...)
		}
	}

	chatReq.Messages = messages

	// Convert tools
	if len(req.Tools) > 0 {
		tools := make([]domaincompletion.Tool, 0, len(req.Tools))
		for _, t := range req.Tools {
			tool := domaincompletion.Tool{
				Type: "function",
				Function: domaincompletion.Function{
					Name:        t.Name,
					Description: t.Description,
					Parameters:  t.InputSchema,
				},
			}
			tools = append(tools, tool)
		}
		chatReq.Tools = tools
	}

	// Convert tool_choice
	if req.ToolChoice != nil {
		chatReq.ToolChoice = convertToolChoiceToChat(req.ToolChoice)
	}

	// Temperature and other params
	if req.Temperature != nil {
		chatReq.Temperature = req.Temperature
	}
	if req.TopP != nil {
		chatReq.TopP = req.TopP
	}
	if req.MaxTokens > 0 {
		chatReq.MaxTokens = &req.MaxTokens
	}
	if req.Metadata != nil && req.Metadata.UserID != "" {
		chatReq.User = req.Metadata.UserID
	}

	return json.Marshal(chatReq)
}

// convertUserMessageToChat converts an Anthropic user message to OpenAI chat format.
func convertUserMessageToChat(msg anthropic.Message) domaincompletion.Message {
	// Check if any content blocks are non-text (images, tool_result)
	hasComplexContent := false
	for _, block := range msg.Content {
		if block.Type != "text" {
			hasComplexContent = true
			break
		}
	}

	// Simple text-only message
	if !hasComplexContent && len(msg.Content) == 1 {
		return domaincompletion.Message{
			Role:    "user",
			Content: msg.Content[0].Text,
		}
	}

	// Complex content - build content parts array
	contentParts := make([]domaincompletion.ContentPart, 0, len(msg.Content))
	var toolResults []domaincompletion.Message

	for _, block := range msg.Content {
		switch block.Type {
		case "text":
			contentParts = append(contentParts, domaincompletion.ContentPart{
				Type: "text",
				Text: block.Text,
			})
		case "image":
			if block.Source != nil {
				var imageURL string
				switch block.Source.Type {
				case "base64":
					imageURL = fmt.Sprintf("data:%s;base64,%s", block.Source.MediaType, block.Source.Data)
				case "url":
					imageURL = block.Source.URL
				}
				if imageURL != "" {
					contentParts = append(contentParts, domaincompletion.ContentPart{
						Type: "image_url",
						ImageURL: &domaincompletion.ImageURL{
							URL: imageURL,
						},
					})
				}
			}
		case "tool_result":
			// Tool results become separate "tool" role messages in OpenAI format
			toolResult := convertToolResultToChat(block)
			toolResults = append(toolResults, toolResult)
		}
	}

	// If we only have tool results, return a placeholder and the tool results will be handled separately
	if len(contentParts) == 0 && len(toolResults) > 0 {
		// This shouldn't happen normally - tool_result should be the only content
		// Return the first tool result as the message
		if len(toolResults) > 0 {
			return toolResults[0]
		}
	}

	// Build the message with content parts
	chatMsg := domaincompletion.Message{
		Role: "user",
	}

	if len(contentParts) == 1 && contentParts[0].Type == "text" {
		chatMsg.Content = contentParts[0].Text
	} else if len(contentParts) > 0 {
		chatMsg.Content = contentParts
	}

	return chatMsg
}

// convertToolResultToChat converts an Anthropic tool_result to OpenAI tool message.
func convertToolResultToChat(block anthropic.ContentBlock) domaincompletion.Message {
	output := ""
	switch content := block.Content.(type) {
	case string:
		output = content
	case []anthropic.ContentBlock:
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

	return domaincompletion.Message{
		Role:       "tool",
		Content:    output,
		ToolCallID: block.ToolUseID,
	}
}

// convertAssistantMessageToChat converts an Anthropic assistant message to OpenAI chat format.
// May return multiple messages if there are tool_use blocks.
func convertAssistantMessageToChat(msg anthropic.Message) []domaincompletion.Message {
	var textContent strings.Builder
	var toolCalls []domaincompletion.ToolCall

	for _, block := range msg.Content {
		switch block.Type {
		case "text":
			textContent.WriteString(block.Text)
		case "tool_use":
			// Convert tool_use to tool_call
			args := ""
			if block.Input != nil {
				if argsBytes, err := json.Marshal(block.Input); err == nil {
					args = string(argsBytes)
				}
			}
			toolCalls = append(toolCalls, domaincompletion.ToolCall{
				ID:   block.ID,
				Type: "function",
				Function: domaincompletion.FunctionCall{
					Name:      block.Name,
					Arguments: args,
				},
			})
		}
	}

	// Build the assistant message
	chatMsg := domaincompletion.Message{
		Role: "assistant",
	}

	text := textContent.String()
	if text != "" {
		chatMsg.Content = text
	}

	if len(toolCalls) > 0 {
		chatMsg.ToolCalls = toolCalls
	}

	return []domaincompletion.Message{chatMsg}
}

// convertToolChoiceToChat converts Anthropic tool_choice to OpenAI format.
func convertToolChoiceToChat(tc *anthropic.ToolChoice) any {
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
