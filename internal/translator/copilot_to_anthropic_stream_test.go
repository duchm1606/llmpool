package translator

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestConvertChatCompletionEventToAnthropic_ToolArgsStayIsolated(t *testing.T) {
	s := NewCopilotToAnthropicStreamState("claude-opus-4.6")

	chunks := [][]byte{
		mustJSON(t, map[string]any{
			"id":      "chatcmpl_1",
			"created": float64(1),
			"model":   "claude-opus-4.6",
			"choices": []any{map[string]any{
				"index": float64(0),
				"delta": map[string]any{
					"tool_calls": []any{map[string]any{
						"index": float64(0),
						"id":    "call_1",
						"type":  "function",
						"function": map[string]any{
							"name":      "task",
							"arguments": `{"description":"one"`,
						},
					}},
				},
			}},
		}),
		mustJSON(t, map[string]any{
			"id":      "chatcmpl_1",
			"created": float64(1),
			"model":   "claude-opus-4.6",
			"choices": []any{map[string]any{
				"index": float64(0),
				"delta": map[string]any{
					"tool_calls": []any{map[string]any{
						"index": float64(1),
						"id":    "call_2",
						"type":  "function",
						"function": map[string]any{
							"name":      "task",
							"arguments": `{"description":"two"`,
						},
					}},
				},
			}},
		}),
		mustJSON(t, map[string]any{
			"id":      "chatcmpl_1",
			"created": float64(1),
			"model":   "claude-opus-4.6",
			"choices": []any{map[string]any{
				"index": float64(0),
				"delta": map[string]any{
					"tool_calls": []any{map[string]any{
						"index": float64(0),
						"function": map[string]any{
							"arguments": `,"prompt":"p1","subagent_type":"explore"}`,
						},
					}},
				},
			}},
		}),
		mustJSON(t, map[string]any{
			"id":      "chatcmpl_1",
			"created": float64(1),
			"model":   "claude-opus-4.6",
			"choices": []any{map[string]any{
				"index": float64(0),
				"delta": map[string]any{
					"tool_calls": []any{map[string]any{
						"index": float64(1),
						"function": map[string]any{
							"arguments": `,"prompt":"p2","subagent_type":"explore"}`,
						},
					}},
				},
			}},
		}),
	}

	parts := map[int]string{}
	for _, c := range chunks {
		events := s.ConvertChatCompletionEventToAnthropic(c)
		collectInputJSONDeltas(t, events, parts)
	}

	if got := parts[0]; got != `{"description":"one","prompt":"p1","subagent_type":"explore"}` {
		t.Fatalf("tool index 0 arguments mismatch: %s", got)
	}
	if got := parts[1]; got != `{"description":"two","prompt":"p2","subagent_type":"explore"}` {
		t.Fatalf("tool index 1 arguments mismatch: %s", got)
	}
}

func TestConvertCopilotEventToAnthropic_OutputIndexRouting(t *testing.T) {
	s := NewCopilotToAnthropicStreamState("claude-opus-4.6")

	stream := [][]byte{
		mustJSON(t, map[string]any{"type": "response.created"}),
		mustJSON(t, map[string]any{
			"type":         "response.output_item.added",
			"output_index": float64(0),
			"item": map[string]any{
				"type":    "function_call",
				"call_id": "call_a",
				"name":    "task",
			},
		}),
		mustJSON(t, map[string]any{
			"type":         "response.output_item.added",
			"output_index": float64(1),
			"item": map[string]any{
				"type":    "function_call",
				"call_id": "call_b",
				"name":    "task",
			},
		}),
		mustJSON(t, map[string]any{
			"type":         "response.function_call.arguments.delta",
			"output_index": float64(0),
			"delta":        `{"description":"A"`,
		}),
		mustJSON(t, map[string]any{
			"type":         "response.function_call.arguments.delta",
			"output_index": float64(1),
			"delta":        `{"description":"B"`,
		}),
		mustJSON(t, map[string]any{
			"type":         "response.function_call.arguments.delta",
			"output_index": float64(0),
			"delta":        `,"prompt":"P1","subagent_type":"explore"}`,
		}),
		mustJSON(t, map[string]any{
			"type":         "response.function_call.arguments.delta",
			"output_index": float64(1),
			"delta":        `,"prompt":"P2","subagent_type":"explore"}`,
		}),
	}

	parts := map[int]string{}
	for _, event := range stream {
		collectInputJSONDeltas(t, s.ConvertCopilotEventToAnthropic(event), parts)
	}

	if got := parts[0]; got != `{"description":"A","prompt":"P1","subagent_type":"explore"}` {
		t.Fatalf("output_index 0 arguments mismatch: %s", got)
	}
	if got := parts[1]; got != `{"description":"B","prompt":"P2","subagent_type":"explore"}` {
		t.Fatalf("output_index 1 arguments mismatch: %s", got)
	}
}

func collectInputJSONDeltas(t *testing.T, events []string, out map[int]string) {
	t.Helper()
	for _, evt := range events {
		if !strings.HasPrefix(evt, "event: content_block_delta\n") {
			continue
		}
		data := sseData(evt)
		var msg struct {
			Type  string `json:"type"`
			Index int    `json:"index"`
			Delta struct {
				Type        string `json:"type"`
				PartialJSON string `json:"partial_json"`
			} `json:"delta"`
		}
		if err := json.Unmarshal([]byte(data), &msg); err != nil {
			t.Fatalf("unmarshal delta event failed: %v", err)
		}
		if msg.Delta.Type != "input_json_delta" {
			continue
		}
		out[msg.Index] += msg.Delta.PartialJSON
	}
}

func sseData(evt string) string {
	for _, line := range strings.Split(evt, "\n") {
		if strings.HasPrefix(line, "data: ") {
			return strings.TrimPrefix(line, "data: ")
		}
	}
	return ""
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	return b
}

func TestSSEDataHelper(t *testing.T) {
	raw := "event: test\ndata: {\"a\":1}\n\n"
	if got := sseData(raw); got != `{"a":1}` {
		t.Fatalf("unexpected data extraction: %s", fmt.Sprintf("%q", got))
	}
}
