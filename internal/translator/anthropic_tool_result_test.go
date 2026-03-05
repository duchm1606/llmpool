package translator

import (
	"encoding/json"
	"testing"

	"github.com/duchoang/llmpool/internal/domain/anthropic"
)

func TestAnthropicToChatCompletion_ToolResultBecomesToolMessage(t *testing.T) {
	req := &anthropic.MessagesRequest{
		Model: "claude-opus-4.6",
		Messages: []anthropic.Message{
			{
				Role: "assistant",
				Content: []anthropic.ContentBlock{
					{Type: "tool_use", ID: "toolu_1", Name: "fetch_weather", Input: map[string]any{"city": "Hanoi"}},
				},
			},
			{
				Role: "user",
				Content: []anthropic.ContentBlock{
					{Type: "tool_result", ToolUseID: "toolu_1", Content: "sunny"},
				},
			},
		},
	}

	body, err := AnthropicToChatCompletion(req)
	if err != nil {
		t.Fatalf("AnthropicToChatCompletion failed: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("unmarshal payload failed: %v", err)
	}

	messages, ok := payload["messages"].([]any)
	if !ok {
		t.Fatalf("messages missing or wrong type")
	}
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}

	assistant, _ := messages[0].(map[string]any)
	if assistant["role"] != "assistant" {
		t.Fatalf("expected first message role=assistant, got %v", assistant["role"])
	}

	toolCalls, ok := assistant["tool_calls"].([]any)
	if !ok || len(toolCalls) != 1 {
		t.Fatalf("expected one tool_call in assistant message")
	}
	call, _ := toolCalls[0].(map[string]any)
	if call["id"] != "toolu_1" {
		t.Fatalf("expected tool_call id toolu_1, got %v", call["id"])
	}

	tool, _ := messages[1].(map[string]any)
	if tool["role"] != "tool" {
		t.Fatalf("expected second message role=tool, got %v", tool["role"])
	}
	if tool["tool_call_id"] != "toolu_1" {
		t.Fatalf("expected tool_call_id toolu_1, got %v", tool["tool_call_id"])
	}
	if tool["content"] != "sunny" {
		t.Fatalf("expected tool content sunny, got %v", tool["content"])
	}
}

func TestAnthropicToChatCompletion_ToolResultBeforeUserText(t *testing.T) {
	req := &anthropic.MessagesRequest{
		Model: "claude-opus-4.6",
		Messages: []anthropic.Message{
			{
				Role: "assistant",
				Content: []anthropic.ContentBlock{
					{Type: "tool_use", ID: "toolu_2", Name: "fetch_weather", Input: map[string]any{"city": "Saigon"}},
				},
			},
			{
				Role: "user",
				Content: []anthropic.ContentBlock{
					{Type: "tool_result", ToolUseID: "toolu_2", Content: "rainy"},
					{Type: "text", Text: "continue"},
				},
			},
		},
	}

	body, err := AnthropicToChatCompletion(req)
	if err != nil {
		t.Fatalf("AnthropicToChatCompletion failed: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("unmarshal payload failed: %v", err)
	}

	messages, ok := payload["messages"].([]any)
	if !ok {
		t.Fatalf("messages missing or wrong type")
	}
	if len(messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(messages))
	}

	tool, _ := messages[1].(map[string]any)
	if tool["role"] != "tool" {
		t.Fatalf("expected second message role=tool, got %v", tool["role"])
	}

	user, _ := messages[2].(map[string]any)
	if user["role"] != "user" {
		t.Fatalf("expected third message role=user, got %v", user["role"])
	}
	if user["content"] != "continue" {
		t.Fatalf("expected user content continue, got %v", user["content"])
	}
}

func TestAnthropicToCopilotResponses_ToolResultBecomesFunctionCallOutput(t *testing.T) {
	req := &anthropic.MessagesRequest{
		Model: "claude-opus-4.6",
		Messages: []anthropic.Message{
			{
				Role: "assistant",
				Content: []anthropic.ContentBlock{
					{Type: "tool_use", ID: "toolu_3", Name: "fetch_weather", Input: map[string]any{"city": "Danang"}},
				},
			},
			{
				Role: "user",
				Content: []anthropic.ContentBlock{
					{Type: "tool_result", ToolUseID: "toolu_3", Content: "windy"},
				},
			},
		},
	}

	body, err := AnthropicToCopilotResponses(req)
	if err != nil {
		t.Fatalf("AnthropicToCopilotResponses failed: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("unmarshal payload failed: %v", err)
	}

	input, ok := payload["input"].([]any)
	if !ok {
		t.Fatalf("input missing or wrong type")
	}
	if len(input) != 2 {
		t.Fatalf("expected 2 input items, got %d", len(input))
	}

	call, _ := input[0].(map[string]any)
	if call["type"] != "function_call" {
		t.Fatalf("expected first item type=function_call, got %v", call["type"])
	}

	out, _ := input[1].(map[string]any)
	if out["type"] != "function_call_output" {
		t.Fatalf("expected second item type=function_call_output, got %v", out["type"])
	}
	if out["call_id"] != "toolu_3" {
		t.Fatalf("expected call_id toolu_3, got %v", out["call_id"])
	}
	if out["output"] != "windy" {
		t.Fatalf("expected output windy, got %v", out["output"])
	}
}
