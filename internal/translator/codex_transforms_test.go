package translator

import (
	"encoding/json"
	"testing"

	"github.com/tidwall/gjson"
)

func TestTransformForCodex(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		checks []func(result []byte) bool
	}{
		{
			name:  "sets required fields",
			input: `{"model":"o1","input":"hello"}`,
			checks: []func(result []byte) bool{
				func(r []byte) bool { return gjson.GetBytes(r, "stream").Bool() == true },
				func(r []byte) bool { return gjson.GetBytes(r, "store").Bool() == false },
				func(r []byte) bool { return gjson.GetBytes(r, "parallel_tool_calls").Bool() == true },
				func(r []byte) bool {
					inc := gjson.GetBytes(r, "include").Array()
					return len(inc) == 1 && inc[0].String() == "reasoning.encrypted_content"
				},
			},
		},
		{
			name:  "strips unsupported fields",
			input: `{"model":"o1","input":"hello","max_output_tokens":100,"temperature":0.7,"top_p":0.9,"service_tier":"default","truncation":"auto","context_management":{},"user":"test"}`,
			checks: []func(result []byte) bool{
				func(r []byte) bool { return !gjson.GetBytes(r, "max_output_tokens").Exists() },
				func(r []byte) bool { return !gjson.GetBytes(r, "temperature").Exists() },
				func(r []byte) bool { return !gjson.GetBytes(r, "top_p").Exists() },
				func(r []byte) bool { return !gjson.GetBytes(r, "service_tier").Exists() },
				func(r []byte) bool { return !gjson.GetBytes(r, "truncation").Exists() },
				func(r []byte) bool { return !gjson.GetBytes(r, "context_management").Exists() },
				func(r []byte) bool { return !gjson.GetBytes(r, "user").Exists() },
			},
		},
		{
			name:  "converts string input to structured array",
			input: `{"model":"o1","input":"hello world"}`,
			checks: []func(result []byte) bool{
				func(r []byte) bool {
					return gjson.GetBytes(r, "input").IsArray()
				},
				func(r []byte) bool {
					return gjson.GetBytes(r, "input.0.type").String() == "message"
				},
				func(r []byte) bool {
					return gjson.GetBytes(r, "input.0.role").String() == "user"
				},
				func(r []byte) bool {
					return gjson.GetBytes(r, "input.0.content.0.text").String() == "hello world"
				},
			},
		},
		{
			name:  "converts system role to developer",
			input: `{"model":"o1","input":[{"type":"message","role":"system","content":"you are helpful"},{"type":"message","role":"user","content":"hi"}]}`,
			checks: []func(result []byte) bool{
				func(r []byte) bool {
					return gjson.GetBytes(r, "input.0.role").String() == "developer"
				},
				func(r []byte) bool {
					return gjson.GetBytes(r, "input.1.role").String() == "user"
				},
			},
		},
		{
			name:  "preserves other fields",
			input: `{"model":"o1","input":"hi","instructions":"be helpful","tools":[{"type":"function","name":"test"}],"metadata":{"cursor_id":"123"}}`,
			checks: []func(result []byte) bool{
				func(r []byte) bool { return gjson.GetBytes(r, "model").String() == "o1" },
				func(r []byte) bool { return gjson.GetBytes(r, "instructions").String() == "be helpful" },
				func(r []byte) bool { return gjson.GetBytes(r, "tools.0.name").String() == "test" },
				func(r []byte) bool { return gjson.GetBytes(r, "metadata.cursor_id").String() == "123" },
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TransformForCodex([]byte(tt.input))

			// Verify it's valid JSON
			if !json.Valid(result) {
				t.Fatalf("result is not valid JSON: %s", string(result))
			}

			for i, check := range tt.checks {
				if !check(result) {
					t.Errorf("check %d failed, result: %s", i, string(result))
				}
			}
		})
	}
}

func TestIsCodexModel(t *testing.T) {
	tests := []struct {
		model    string
		expected bool
	}{
		{"o1", true},
		{"o1-preview", true},
		{"o3", true},
		{"o3-mini", true},
		{"o4-mini", true},
		{"gpt-5", true},
		{"gpt-5-turbo", true},
		{"gpt-4.1", true},
		{"gpt-4.1-preview", true},
		{"gpt-4o", false},
		{"gpt-4o-mini", false},
		{"gpt-4-turbo", false},
		{"claude-3-opus", false},
		{"gemini-pro", false},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			result := IsCodexModel(tt.model)
			if result != tt.expected {
				t.Errorf("IsCodexModel(%q) = %v, want %v", tt.model, result, tt.expected)
			}
		})
	}
}
