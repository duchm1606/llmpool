package translator

import (
	"testing"

	"github.com/duchoang/llmpool/internal/domain/anthropic"
)

func TestIsCompactRequest(t *testing.T) {
	tests := []struct {
		name   string
		system []anthropic.SystemBlock
		want   bool
	}{
		{
			name:   "empty system",
			system: nil,
			want:   false,
		},
		{
			name: "compact system prompt",
			system: []anthropic.SystemBlock{
				{Type: "text", Text: "You are a helpful AI assistant tasked with summarizing conversations between a user and an AI assistant."},
			},
			want: true,
		},
		{
			name: "compact system prompt exact prefix",
			system: []anthropic.SystemBlock{
				{Type: "text", Text: CompactSystemPromptStart},
			},
			want: true,
		},
		{
			name: "non-compact system prompt",
			system: []anthropic.SystemBlock{
				{Type: "text", Text: "You are a helpful coding assistant."},
			},
			want: false,
		},
		{
			name: "multiple blocks with compact",
			system: []anthropic.SystemBlock{
				{Type: "text", Text: "Some prefix"},
				{Type: "text", Text: "You are a helpful AI assistant tasked with summarizing conversations..."},
			},
			want: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := &anthropic.MessagesRequest{System: tc.system}
			got := IsCompactRequest(req)
			if got != tc.want {
				t.Fatalf("IsCompactRequest() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestParseSubagentMarker(t *testing.T) {
	tests := []struct {
		name     string
		messages []anthropic.Message
		want     *SubagentMarker
	}{
		{
			name:     "no messages",
			messages: nil,
			want:     nil,
		},
		{
			name: "no user message",
			messages: []anthropic.Message{
				{Role: "assistant", Content: []anthropic.ContentBlock{{Type: "text", Text: "Hello"}}},
			},
			want: nil,
		},
		{
			name: "user message without marker",
			messages: []anthropic.Message{
				{Role: "user", Content: []anthropic.ContentBlock{{Type: "text", Text: "Hello"}}},
			},
			want: nil,
		},
		{
			name: "user message with valid marker",
			messages: []anthropic.Message{
				{Role: "user", Content: []anthropic.ContentBlock{
					{Type: "text", Text: `Some text <system-reminder>__SUBAGENT_MARKER__{"session_id":"sess1","agent_id":"agent1","agent_type":"task"}</system-reminder> more text`},
				}},
			},
			want: &SubagentMarker{SessionID: "sess1", AgentID: "agent1", AgentType: "task"},
		},
		{
			name: "user message with incomplete marker",
			messages: []anthropic.Message{
				{Role: "user", Content: []anthropic.ContentBlock{
					{Type: "text", Text: `<system-reminder>__SUBAGENT_MARKER__{"session_id":"sess1"}</system-reminder>`},
				}},
			},
			want: nil, // Missing required fields
		},
		{
			name: "marker in second user message ignored",
			messages: []anthropic.Message{
				{Role: "user", Content: []anthropic.ContentBlock{{Type: "text", Text: "First"}}},
				{Role: "assistant", Content: []anthropic.ContentBlock{{Type: "text", Text: "Response"}}},
				{Role: "user", Content: []anthropic.ContentBlock{
					{Type: "text", Text: `<system-reminder>__SUBAGENT_MARKER__{"session_id":"sess2","agent_id":"agent2","agent_type":"task"}</system-reminder>`},
				}},
			},
			want: nil, // Only first user message is checked
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := &anthropic.MessagesRequest{Messages: tc.messages}
			got := ParseSubagentMarker(req)
			if tc.want == nil {
				if got != nil {
					t.Fatalf("ParseSubagentMarker() = %+v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("ParseSubagentMarker() = nil, want %+v", tc.want)
			}
			if got.SessionID != tc.want.SessionID || got.AgentID != tc.want.AgentID || got.AgentType != tc.want.AgentType {
				t.Fatalf("ParseSubagentMarker() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestMergeToolResultForClaude(t *testing.T) {
	tests := []struct {
		name         string
		messages     []anthropic.Message
		wantMsgCount int
		wantMerged   bool
	}{
		{
			name: "no tool_result",
			messages: []anthropic.Message{
				{Role: "user", Content: []anthropic.ContentBlock{{Type: "text", Text: "Hello"}}},
			},
			wantMsgCount: 1,
			wantMerged:   false,
		},
		{
			name: "tool_result only",
			messages: []anthropic.Message{
				{Role: "user", Content: []anthropic.ContentBlock{
					{Type: "tool_result", ToolUseID: "t1", Content: "result"},
				}},
			},
			wantMsgCount: 1,
			wantMerged:   false, // No text to merge
		},
		{
			name: "tool_result with text - should merge",
			messages: []anthropic.Message{
				{Role: "user", Content: []anthropic.ContentBlock{
					{Type: "tool_result", ToolUseID: "t1", Content: "result"},
					{Type: "text", Text: "continue"},
				}},
			},
			wantMsgCount: 1,
			wantMerged:   true,
		},
		{
			name: "assistant message untouched",
			messages: []anthropic.Message{
				{Role: "assistant", Content: []anthropic.ContentBlock{
					{Type: "tool_use", ID: "t1", Name: "test"},
				}},
			},
			wantMsgCount: 1,
			wantMerged:   false,
		},
		{
			name: "image block breaks merge",
			messages: []anthropic.Message{
				{Role: "user", Content: []anthropic.ContentBlock{
					{Type: "tool_result", ToolUseID: "t1", Content: "result"},
					{Type: "image", Source: &anthropic.ImageSource{Type: "base64"}},
					{Type: "text", Text: "continue"},
				}},
			},
			wantMsgCount: 1,
			wantMerged:   false, // Image block prevents merge
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := &anthropic.MessagesRequest{Messages: tc.messages}
			originalContent := make([]anthropic.ContentBlock, len(req.Messages[0].Content))
			copy(originalContent, req.Messages[0].Content)

			MergeToolResultForClaude(req)

			if len(req.Messages) != tc.wantMsgCount {
				t.Fatalf("MergeToolResultForClaude() message count = %d, want %d", len(req.Messages), tc.wantMsgCount)
			}

			if tc.wantMerged {
				// After merge, should have fewer content blocks
				if len(req.Messages[0].Content) >= len(originalContent) {
					t.Fatalf("MergeToolResultForClaude() should have reduced content blocks")
				}
				// First block should be tool_result with merged content
				if req.Messages[0].Content[0].Type != "tool_result" {
					t.Fatalf("MergeToolResultForClaude() first block should be tool_result")
				}
			}
		})
	}
}

func TestFilterThinkingBlocks(t *testing.T) {
	tests := []struct {
		name      string
		modelID   string
		content   []anthropic.ContentBlock
		wantCount int
		wantTypes []string
	}{
		{
			name:    "non-claude model unchanged",
			modelID: "gpt-5",
			content: []anthropic.ContentBlock{
				{Type: "thinking", Thinking: "test", Signature: "sig@123"},
				{Type: "text", Text: "hello"},
			},
			wantCount: 2,
			wantTypes: []string{"thinking", "text"},
		},
		{
			name:    "claude filters GPT signature",
			modelID: "claude-sonnet-4.5",
			content: []anthropic.ContentBlock{
				{Type: "thinking", Thinking: "test", Signature: "sig@123"}, // GPT signature with @
				{Type: "text", Text: "hello"},
			},
			wantCount: 1,
			wantTypes: []string{"text"},
		},
		{
			name:    "claude keeps valid thinking",
			modelID: "claude-opus-4.5",
			content: []anthropic.ContentBlock{
				{Type: "thinking", Thinking: "deep thought", Signature: "validClaudeSignature"},
				{Type: "text", Text: "hello"},
			},
			wantCount: 2,
			wantTypes: []string{"thinking", "text"},
		},
		{
			name:    "claude filters empty thinking",
			modelID: "claude-sonnet-4",
			content: []anthropic.ContentBlock{
				{Type: "thinking", Thinking: "", Signature: "validSig"},
				{Type: "text", Text: "hello"},
			},
			wantCount: 1,
			wantTypes: []string{"text"},
		},
		{
			name:    "claude filters placeholder thinking",
			modelID: "claude-opus-4",
			content: []anthropic.ContentBlock{
				{Type: "thinking", Thinking: ThinkingTextPlaceholder, Signature: "validSig"},
				{Type: "text", Text: "hello"},
			},
			wantCount: 1,
			wantTypes: []string{"text"},
		},
		{
			name:    "claude filters no signature",
			modelID: "claude-sonnet-4.5",
			content: []anthropic.ContentBlock{
				{Type: "thinking", Thinking: "test", Signature: ""},
				{Type: "text", Text: "hello"},
			},
			wantCount: 1,
			wantTypes: []string{"text"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := &anthropic.MessagesRequest{
				Messages: []anthropic.Message{
					{Role: "assistant", Content: tc.content},
				},
			}

			FilterThinkingBlocks(req, tc.modelID)

			got := req.Messages[0].Content
			if len(got) != tc.wantCount {
				t.Fatalf("FilterThinkingBlocks() content count = %d, want %d", len(got), tc.wantCount)
			}

			for i, wantType := range tc.wantTypes {
				if got[i].Type != wantType {
					t.Fatalf("FilterThinkingBlocks() content[%d].Type = %q, want %q", i, got[i].Type, wantType)
				}
			}
		})
	}
}

func TestGetEffortForModel(t *testing.T) {
	tests := []struct {
		effort string
		want   string
	}{
		{"xhigh", "max"},
		{"none", "low"},
		{"minimal", "low"},
		{"low", "low"},
		{"medium", "medium"},
		{"high", "high"},
		{"", "medium"},
		{"invalid", "medium"},
	}

	for _, tc := range tests {
		t.Run(tc.effort, func(t *testing.T) {
			got := GetEffortForModel(tc.effort)
			if got != tc.want {
				t.Fatalf("GetEffortForModel(%q) = %q, want %q", tc.effort, got, tc.want)
			}
		})
	}
}

func TestIsGPT5OrLater_Preprocessing(t *testing.T) {
	// Test the preprocessing version (mirrors provider version)
	tests := []struct {
		modelID string
		want    bool
	}{
		{"gpt-5", true},
		{"gpt-5-mini", true},
		{"gpt-5.3-codex", true},
		{"gpt-4o", false},
		{"claude-sonnet-4.5", false},
	}

	for _, tc := range tests {
		t.Run(tc.modelID, func(t *testing.T) {
			got := IsGPT5OrLater(tc.modelID)
			if got != tc.want {
				t.Fatalf("IsGPT5OrLater(%q) = %v, want %v", tc.modelID, got, tc.want)
			}
		})
	}
}

func TestTranslateModelName(t *testing.T) {
	tests := []struct {
		model string
		want  string
	}{
		{"claude-sonnet-4-20250514", "claude-sonnet-4"},
		{"claude-opus-4-20250514", "claude-opus-4"},
		{"claude-sonnet-4.5", "claude-sonnet-4.5"},
		{"gpt-5", "gpt-5"},
	}

	for _, tc := range tests {
		t.Run(tc.model, func(t *testing.T) {
			got := TranslateModelName(tc.model)
			if got != tc.want {
				t.Fatalf("TranslateModelName(%q) = %q, want %q", tc.model, got, tc.want)
			}
		})
	}
}
