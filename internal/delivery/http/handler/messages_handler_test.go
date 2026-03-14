package handler

import (
	"testing"

	"github.com/duchoang/llmpool/internal/domain/anthropic"
	usecasecompletion "github.com/duchoang/llmpool/internal/usecase/completion"
)

func TestAnthropicSessionQuotaMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		req  anthropic.MessagesRequest
		want usecasecompletion.SessionQuotaMode
	}{
		{
			name: "empty request consumes",
			req:  anthropic.MessagesRequest{},
			want: usecasecompletion.SessionQuotaConsume,
		},
		{
			name: "single user turn consumes",
			req: anthropic.MessagesRequest{Messages: []anthropic.Message{{
				Role:    "user",
				Content: []anthropic.ContentBlock{{Type: "text", Text: "hello"}},
			}}},
			want: usecasecompletion.SessionQuotaConsume,
		},
		{
			name: "follow up conversation with final user consumes",
			req: anthropic.MessagesRequest{Messages: []anthropic.Message{
				{Role: "user", Content: []anthropic.ContentBlock{{Type: "text", Text: "hello"}}},
				{Role: "assistant", Content: []anthropic.ContentBlock{{Type: "text", Text: "hi"}}},
				{Role: "user", Content: []anthropic.ContentBlock{{Type: "text", Text: "tell me more"}}},
			}},
			want: usecasecompletion.SessionQuotaConsume,
		},
		{
			name: "single user tool result follow up consumes",
			req: anthropic.MessagesRequest{Messages: []anthropic.Message{{
				Role:    "user",
				Content: []anthropic.ContentBlock{{Type: "tool_result", ToolUseID: "tool-1", Content: "done"}},
			}}},
			want: usecasecompletion.SessionQuotaConsume,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := anthropicSessionQuotaMode(tt.req); got != tt.want {
				t.Fatalf("anthropicSessionQuotaMode() = %q, want %q", got, tt.want)
			}
		})
	}
}
