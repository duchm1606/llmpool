package provider

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	domaincompletion "github.com/duchoang/llmpool/internal/domain/completion"
)

// ChatToResponsesRequest converts a ChatCompletionRequest to a Responses API request payload.
// Reference: opencode convertToOpenAIResponsesInput
func ChatToResponsesRequest(req domaincompletion.ChatCompletionRequest) ([]byte, error) {
	payload := map[string]any{
		"model": GetCopilotModelID(req.Model),
		"store": false,
	}

	if req.Stream {
		payload["stream"] = true
	}

	// Extract system message for instructions
	instructions := ""
	for _, m := range req.Messages {
		if m.Role == "system" {
			if s := stringifyResponsesContent(m.Content); s != "" {
				instructions = s
				break
			}
		}
	}
	if instructions != "" {
		payload["instructions"] = instructions
	}

	// Convert messages to input array
	input := make([]any, 0, len(req.Messages))
	for _, m := range req.Messages {
		if m.Role == "system" {
			continue // Already extracted as instructions
		}

		// Handle tool/function results
		if m.Role == "tool" {
			input = append(input, map[string]any{
				"type":    "function_call_output",
				"call_id": m.ToolCallID,
				"output":  stringifyResponsesContent(m.Content),
			})
			continue
		}

		role := m.Role
		if role == "system" {
			role = "developer"
		}

		// Build message with content
		input = append(input, map[string]any{
			"type":    "message",
			"role":    role,
			"content": buildResponsesContent(role, m.Content),
		})

		// Handle assistant tool calls
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

	// Convert tools
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

	// Temperature and other params
	if req.Temperature != nil {
		payload["temperature"] = *req.Temperature
	}
	if req.TopP != nil {
		payload["top_p"] = *req.TopP
	}
	if req.MaxTokens != nil {
		payload["max_output_tokens"] = *req.MaxTokens
	}

	return json.Marshal(payload)
}

// buildResponsesContent converts message content to responses API format.
func buildResponsesContent(role string, content any) []map[string]any {
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
		parts = append(parts, map[string]any{"type": partType, "text": stringifyResponsesContent(content)})
	}

	return parts
}

func stringifyResponsesContent(content any) string {
	if s, ok := content.(string); ok {
		return s
	}
	b, err := json.Marshal(content)
	if err != nil {
		return ""
	}
	return string(b)
}

// ResponsesToChatResponse converts a Responses API response to ChatCompletionResponse.
// The responses API returns a different structure that we need to normalize.
func ResponsesToChatResponse(raw []byte, fallbackModel string) (*domaincompletion.ChatCompletionResponse, error) {
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, fmt.Errorf("parse responses payload: %w", err)
	}

	// Handle response.completed event wrapper
	responseObj := root
	if t := mapStringVal(root, "type"); t == "response.completed" {
		if nested, ok := root["response"].(map[string]any); ok {
			responseObj = nested
		}
	} else if nested, ok := root["response"].(map[string]any); ok {
		responseObj = nested
	}

	id := mapStringVal(responseObj, "id")
	if id == "" {
		id = fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano())
	}

	created := mapInt64Val(responseObj, "created_at")
	if created == 0 {
		created = time.Now().Unix()
	}

	model := mapStringVal(responseObj, "model")
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

			switch mapStringVal(item, "type") {
			case "message":
				role := mapStringVal(item, "role")
				if role != "" && role != "assistant" {
					continue
				}
				if contentArr, ok := item["content"].([]any); ok {
					for _, partRaw := range contentArr {
						part, ok := partRaw.(map[string]any)
						if !ok {
							continue
						}
						partType := mapStringVal(part, "type")
						if partType == "output_text" || partType == "input_text" || partType == "text" {
							contentBuilder.WriteString(mapStringVal(part, "text"))
						}
					}
				}
			case "function_call":
				callID := mapStringVal(item, "call_id")
				if callID == "" {
					callID = mapStringVal(item, "id")
				}
				toolCalls = append(toolCalls, domaincompletion.ToolCall{
					ID:   callID,
					Type: "function",
					Function: domaincompletion.FunctionCall{
						Name:      mapStringVal(item, "name"),
						Arguments: mapStringVal(item, "arguments"),
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
		prompt := int(mapInt64Val(usageMap, "input_tokens"))
		completion := int(mapInt64Val(usageMap, "output_tokens"))
		total := int(mapInt64Val(usageMap, "total_tokens"))
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

func mapStringVal(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func mapInt64Val(m map[string]any, key string) int64 {
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
