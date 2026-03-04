package translator

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"time"
)

// StreamState maintains state for converting streaming Chat Completions to Responses format.
type StreamState struct {
	Seq            int
	ResponseID     string
	Created        int64
	Started        bool
	ReasoningID    string
	ReasoningIndex int

	// Aggregation buffers
	MsgTextBuf   map[int]*strings.Builder
	ReasoningBuf strings.Builder
	FuncArgsBuf  map[int]*strings.Builder
	FuncNames    map[int]string
	FuncCallIDs  map[int]string

	// State tracking
	MsgItemAdded    map[int]bool
	MsgContentAdded map[int]bool
	MsgItemDone     map[int]bool
	FuncArgsDone    map[int]bool
	FuncItemDone    map[int]bool

	// Usage aggregation
	PromptTokens     int64
	CachedTokens     int64
	CompletionTokens int64
	TotalTokens      int64
	ReasoningTokens  int64
	UsageSeen        bool

	// Original request for response.completed
	OriginalRequest json.RawMessage
}

// responseIDCounter provides a process-wide unique counter for synthesized response identifiers.
var responseIDCounter uint64

// NewStreamState creates a new stream state for response conversion.
func NewStreamState(originalRequest json.RawMessage) *StreamState {
	return &StreamState{
		MsgTextBuf:      make(map[int]*strings.Builder),
		FuncArgsBuf:     make(map[int]*strings.Builder),
		FuncNames:       make(map[int]string),
		FuncCallIDs:     make(map[int]string),
		MsgItemAdded:    make(map[int]bool),
		MsgContentAdded: make(map[int]bool),
		MsgItemDone:     make(map[int]bool),
		FuncArgsDone:    make(map[int]bool),
		FuncItemDone:    make(map[int]bool),
		OriginalRequest: originalRequest,
	}
}

// ChatCompletionChunk represents a streaming chunk from Chat Completions.
type ChatCompletionChunk struct {
	ID      string        `json:"id"`
	Object  string        `json:"object"`
	Created int64         `json:"created"`
	Model   string        `json:"model"`
	Choices []ChunkChoice `json:"choices"`
	Usage   *ChunkUsage   `json:"usage,omitempty"`
}

// ChunkChoice represents a choice in a streaming chunk.
type ChunkChoice struct {
	Index        int         `json:"index"`
	Delta        *ChunkDelta `json:"delta,omitempty"`
	FinishReason string      `json:"finish_reason,omitempty"`
}

// ChunkDelta represents the delta content in a streaming chunk.
type ChunkDelta struct {
	Role             string          `json:"role,omitempty"`
	Content          string          `json:"content,omitempty"`
	ReasoningContent string          `json:"reasoning_content,omitempty"`
	ToolCalls        []DeltaToolCall `json:"tool_calls,omitempty"`
}

// DeltaToolCall represents a tool call delta.
type DeltaToolCall struct {
	Index    int                `json:"index"`
	ID       string             `json:"id,omitempty"`
	Type     string             `json:"type,omitempty"`
	Function *DeltaFunctionCall `json:"function,omitempty"`
}

// DeltaFunctionCall represents a function call delta.
type DeltaFunctionCall struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

// ChunkUsage represents usage in a streaming chunk.
type ChunkUsage struct {
	PromptTokens            int                      `json:"prompt_tokens"`
	CompletionTokens        int                      `json:"completion_tokens"`
	TotalTokens             int                      `json:"total_tokens"`
	PromptTokensDetails     *PromptTokensDetails     `json:"prompt_tokens_details,omitempty"`
	CompletionTokensDetails *CompletionTokensDetails `json:"completion_tokens_details,omitempty"`
}

// PromptTokensDetails contains prompt token details.
type PromptTokensDetails struct {
	CachedTokens int `json:"cached_tokens,omitempty"`
}

// CompletionTokensDetails contains completion token details.
type CompletionTokensDetails struct {
	ReasoningTokens int `json:"reasoning_tokens,omitempty"`
}

// FormatResponsesSSE formats an event and payload as Responses SSE format.
// Returns: "event: {event}\ndata: {payload}\n\n"
func FormatResponsesSSE(event, payload string) string {
	return fmt.Sprintf("event: %s\ndata: %s\n\n", event, payload)
}

// ConvertChunkToResponsesEvents converts a Chat Completions chunk to Responses SSE events.
// Returns a slice of formatted SSE strings ready to be written to the response.
func (st *StreamState) ConvertChunkToResponsesEvents(chunk *ChatCompletionChunk) []string {
	var out []string

	nextSeq := func() int { st.Seq++; return st.Seq }

	// Track usage if present
	if chunk.Usage != nil {
		st.PromptTokens = int64(chunk.Usage.PromptTokens)
		st.CompletionTokens = int64(chunk.Usage.CompletionTokens)
		st.TotalTokens = int64(chunk.Usage.TotalTokens)
		if chunk.Usage.PromptTokensDetails != nil {
			st.CachedTokens = int64(chunk.Usage.PromptTokensDetails.CachedTokens)
		}
		if chunk.Usage.CompletionTokensDetails != nil {
			st.ReasoningTokens = int64(chunk.Usage.CompletionTokensDetails.ReasoningTokens)
		}
		st.UsageSeen = true
	}

	// Initialize on first chunk
	if !st.Started {
		st.ResponseID = chunk.ID
		if st.ResponseID == "" {
			st.ResponseID = fmt.Sprintf("resp_%x_%d", time.Now().UnixNano(), atomic.AddUint64(&responseIDCounter, 1))
		}
		st.Created = chunk.Created
		if st.Created == 0 {
			st.Created = time.Now().Unix()
		}

		// Reset state
		st.MsgTextBuf = make(map[int]*strings.Builder)
		st.ReasoningBuf.Reset()
		st.ReasoningID = ""
		st.FuncArgsBuf = make(map[int]*strings.Builder)
		st.FuncNames = make(map[int]string)
		st.FuncCallIDs = make(map[int]string)
		st.MsgItemAdded = make(map[int]bool)
		st.MsgContentAdded = make(map[int]bool)
		st.MsgItemDone = make(map[int]bool)
		st.FuncArgsDone = make(map[int]bool)
		st.FuncItemDone = make(map[int]bool)

		// Emit response.created
		created := map[string]any{
			"type":            "response.created",
			"sequence_number": nextSeq(),
			"response": map[string]any{
				"id":         st.ResponseID,
				"object":     "response",
				"created_at": st.Created,
				"status":     "in_progress",
				"background": false,
				"error":      nil,
				"output":     []any{},
			},
		}
		createdJSON, _ := json.Marshal(created)
		out = append(out, FormatResponsesSSE("response.created", string(createdJSON)))

		// Emit response.in_progress
		inProgress := map[string]any{
			"type":            "response.in_progress",
			"sequence_number": nextSeq(),
			"response": map[string]any{
				"id":         st.ResponseID,
				"object":     "response",
				"created_at": st.Created,
				"status":     "in_progress",
			},
		}
		inProgressJSON, _ := json.Marshal(inProgress)
		out = append(out, FormatResponsesSSE("response.in_progress", string(inProgressJSON)))

		st.Started = true
	}

	// Process each choice
	for _, choice := range chunk.Choices {
		idx := choice.Index
		delta := choice.Delta

		if delta != nil {
			// Handle content delta
			if delta.Content != "" {
				// Close reasoning if active
				if st.ReasoningID != "" {
					out = append(out, st.emitReasoningDone(nextSeq)...)
				}

				// Emit output_item.added if not already
				if !st.MsgItemAdded[idx] {
					itemAdded := map[string]any{
						"type":            "response.output_item.added",
						"sequence_number": nextSeq(),
						"output_index":    idx,
						"item": map[string]any{
							"id":      fmt.Sprintf("msg_%s_%d", st.ResponseID, idx),
							"type":    "message",
							"status":  "in_progress",
							"content": []any{},
							"role":    "assistant",
						},
					}
					itemJSON, _ := json.Marshal(itemAdded)
					out = append(out, FormatResponsesSSE("response.output_item.added", string(itemJSON)))
					st.MsgItemAdded[idx] = true
				}

				// Emit content_part.added if not already
				if !st.MsgContentAdded[idx] {
					partAdded := map[string]any{
						"type":            "response.content_part.added",
						"sequence_number": nextSeq(),
						"item_id":         fmt.Sprintf("msg_%s_%d", st.ResponseID, idx),
						"output_index":    idx,
						"content_index":   0,
						"part": map[string]any{
							"type":        "output_text",
							"annotations": []any{},
							"logprobs":    []any{},
							"text":        "",
						},
					}
					partJSON, _ := json.Marshal(partAdded)
					out = append(out, FormatResponsesSSE("response.content_part.added", string(partJSON)))
					st.MsgContentAdded[idx] = true
				}

				// Emit text delta
				textDelta := map[string]any{
					"type":            "response.output_text.delta",
					"sequence_number": nextSeq(),
					"item_id":         fmt.Sprintf("msg_%s_%d", st.ResponseID, idx),
					"output_index":    idx,
					"content_index":   0,
					"delta":           delta.Content,
					"logprobs":        []any{},
				}
				textDeltaJSON, _ := json.Marshal(textDelta)
				out = append(out, FormatResponsesSSE("response.output_text.delta", string(textDeltaJSON)))

				// Aggregate text
				if st.MsgTextBuf[idx] == nil {
					st.MsgTextBuf[idx] = &strings.Builder{}
				}
				_, _ = st.MsgTextBuf[idx].WriteString(delta.Content)
			}

			// Handle reasoning content
			if delta.ReasoningContent != "" {
				if st.ReasoningID == "" {
					st.ReasoningID = fmt.Sprintf("rs_%s_%d", st.ResponseID, idx)
					st.ReasoningIndex = idx

					// Emit reasoning item added
					itemAdded := map[string]any{
						"type":            "response.output_item.added",
						"sequence_number": nextSeq(),
						"output_index":    idx,
						"item": map[string]any{
							"id":      st.ReasoningID,
							"type":    "reasoning",
							"status":  "in_progress",
							"summary": []any{},
						},
					}
					itemJSON, _ := json.Marshal(itemAdded)
					out = append(out, FormatResponsesSSE("response.output_item.added", string(itemJSON)))

					// Emit reasoning part added
					partAdded := map[string]any{
						"type":            "response.reasoning_summary_part.added",
						"sequence_number": nextSeq(),
						"item_id":         st.ReasoningID,
						"output_index":    st.ReasoningIndex,
						"summary_index":   0,
						"part": map[string]any{
							"type": "summary_text",
							"text": "",
						},
					}
					partJSON, _ := json.Marshal(partAdded)
					out = append(out, FormatResponsesSSE("response.reasoning_summary_part.added", string(partJSON)))
				}

				st.ReasoningBuf.WriteString(delta.ReasoningContent)

				// Emit reasoning delta
				reasoningDelta := map[string]any{
					"type":            "response.reasoning_summary_text.delta",
					"sequence_number": nextSeq(),
					"item_id":         st.ReasoningID,
					"output_index":    st.ReasoningIndex,
					"summary_index":   0,
					"delta":           delta.ReasoningContent,
				}
				reasoningJSON, _ := json.Marshal(reasoningDelta)
				out = append(out, FormatResponsesSSE("response.reasoning_summary_text.delta", string(reasoningJSON)))
			}

			// Handle tool calls
			if len(delta.ToolCalls) > 0 {
				// Close reasoning if active
				if st.ReasoningID != "" {
					out = append(out, st.emitReasoningDone(nextSeq)...)
				}

				// Close message if active
				if st.MsgItemAdded[idx] && !st.MsgItemDone[idx] {
					out = append(out, st.emitMessageDone(idx, nextSeq)...)
				}

				for _, tc := range delta.ToolCalls {
					tcIdx := tc.Index
					if tcIdx == 0 {
						tcIdx = idx // Use choice index if tool call index is 0
					}

					// Track name
					if tc.Function != nil && tc.Function.Name != "" {
						st.FuncNames[tcIdx] = tc.Function.Name
					}

					// Track call ID
					if tc.ID != "" {
						if st.FuncCallIDs[tcIdx] == "" {
							st.FuncCallIDs[tcIdx] = tc.ID

							// Emit function call item added
							funcAdded := map[string]any{
								"type":            "response.output_item.added",
								"sequence_number": nextSeq(),
								"output_index":    tcIdx,
								"item": map[string]any{
									"id":        fmt.Sprintf("fc_%s", tc.ID),
									"type":      "function_call",
									"status":    "in_progress",
									"arguments": "",
									"call_id":   tc.ID,
									"name":      st.FuncNames[tcIdx],
								},
							}
							funcJSON, _ := json.Marshal(funcAdded)
							out = append(out, FormatResponsesSSE("response.output_item.added", string(funcJSON)))
						}
					}

					// Ensure args buffer exists
					if st.FuncArgsBuf[tcIdx] == nil {
						st.FuncArgsBuf[tcIdx] = &strings.Builder{}
					}

					// Handle arguments delta
					if tc.Function != nil && tc.Function.Arguments != "" {
						callID := st.FuncCallIDs[tcIdx]
						if callID == "" {
							callID = tc.ID
						}

						if callID != "" {
							argsDelta := map[string]any{
								"type":            "response.function_call_arguments.delta",
								"sequence_number": nextSeq(),
								"item_id":         fmt.Sprintf("fc_%s", callID),
								"output_index":    tcIdx,
								"delta":           tc.Function.Arguments,
							}
							argsJSON, _ := json.Marshal(argsDelta)
							out = append(out, FormatResponsesSSE("response.function_call_arguments.delta", string(argsJSON)))
						}

						_, _ = st.FuncArgsBuf[tcIdx].WriteString(tc.Function.Arguments)
					}
				}
			}
		}

		// Handle finish_reason
		if choice.FinishReason != "" {
			out = append(out, st.emitCompletionEvents(nextSeq)...)
		}
	}

	return out
}

// emitReasoningDone emits events for closing reasoning.
func (st *StreamState) emitReasoningDone(nextSeq func() int) []string {
	var out []string
	text := st.ReasoningBuf.String()

	// text.done
	textDone := map[string]any{
		"type":            "response.reasoning_summary_text.done",
		"sequence_number": nextSeq(),
		"item_id":         st.ReasoningID,
		"output_index":    st.ReasoningIndex,
		"summary_index":   0,
		"text":            text,
	}
	textJSON, _ := json.Marshal(textDone)
	out = append(out, FormatResponsesSSE("response.reasoning_summary_text.done", string(textJSON)))

	// part.done
	partDone := map[string]any{
		"type":            "response.reasoning_summary_part.done",
		"sequence_number": nextSeq(),
		"item_id":         st.ReasoningID,
		"output_index":    st.ReasoningIndex,
		"summary_index":   0,
		"part": map[string]any{
			"type": "summary_text",
			"text": text,
		},
	}
	partJSON, _ := json.Marshal(partDone)
	out = append(out, FormatResponsesSSE("response.reasoning_summary_part.done", string(partJSON)))

	// item.done
	itemDone := map[string]any{
		"type":            "response.output_item.done",
		"sequence_number": nextSeq(),
		"output_index":    st.ReasoningIndex,
		"item": map[string]any{
			"id":                st.ReasoningID,
			"type":              "reasoning",
			"encrypted_content": "",
			"summary": []map[string]any{
				{"type": "summary_text", "text": text},
			},
		},
	}
	itemJSON, _ := json.Marshal(itemDone)
	out = append(out, FormatResponsesSSE("response.output_item.done", string(itemJSON)))

	st.ReasoningBuf.Reset()
	st.ReasoningID = ""

	return out
}

// emitMessageDone emits events for closing a message.
func (st *StreamState) emitMessageDone(idx int, nextSeq func() int) []string {
	var out []string
	fullText := ""
	if b := st.MsgTextBuf[idx]; b != nil {
		fullText = b.String()
	}

	// text.done
	textDone := map[string]any{
		"type":            "response.output_text.done",
		"sequence_number": nextSeq(),
		"item_id":         fmt.Sprintf("msg_%s_%d", st.ResponseID, idx),
		"output_index":    idx,
		"content_index":   0,
		"text":            fullText,
		"logprobs":        []any{},
	}
	textJSON, _ := json.Marshal(textDone)
	out = append(out, FormatResponsesSSE("response.output_text.done", string(textJSON)))

	// content_part.done
	partDone := map[string]any{
		"type":            "response.content_part.done",
		"sequence_number": nextSeq(),
		"item_id":         fmt.Sprintf("msg_%s_%d", st.ResponseID, idx),
		"output_index":    idx,
		"content_index":   0,
		"part": map[string]any{
			"type":        "output_text",
			"annotations": []any{},
			"logprobs":    []any{},
			"text":        fullText,
		},
	}
	partJSON, _ := json.Marshal(partDone)
	out = append(out, FormatResponsesSSE("response.content_part.done", string(partJSON)))

	// output_item.done
	itemDone := map[string]any{
		"type":            "response.output_item.done",
		"sequence_number": nextSeq(),
		"output_index":    idx,
		"item": map[string]any{
			"id":     fmt.Sprintf("msg_%s_%d", st.ResponseID, idx),
			"type":   "message",
			"status": "completed",
			"content": []map[string]any{
				{
					"type":        "output_text",
					"annotations": []any{},
					"logprobs":    []any{},
					"text":        fullText,
				},
			},
			"role": "assistant",
		},
	}
	itemJSON, _ := json.Marshal(itemDone)
	out = append(out, FormatResponsesSSE("response.output_item.done", string(itemJSON)))

	st.MsgItemDone[idx] = true
	return out
}

// emitCompletionEvents emits all completion events including response.completed.
func (st *StreamState) emitCompletionEvents(nextSeq func() int) []string {
	var out []string

	// Close any open messages
	for idx := range st.MsgItemAdded {
		if !st.MsgItemDone[idx] {
			out = append(out, st.emitMessageDone(idx, nextSeq)...)
		}
	}

	// Close reasoning if active
	if st.ReasoningID != "" {
		out = append(out, st.emitReasoningDone(nextSeq)...)
	}

	// Close any open function calls
	for idx, callID := range st.FuncCallIDs {
		if callID == "" || st.FuncItemDone[idx] {
			continue
		}

		args := "{}"
		if b := st.FuncArgsBuf[idx]; b != nil && b.Len() > 0 {
			args = b.String()
		}

		// args.done
		argsDone := map[string]any{
			"type":            "response.function_call_arguments.done",
			"sequence_number": nextSeq(),
			"item_id":         fmt.Sprintf("fc_%s", callID),
			"output_index":    idx,
			"arguments":       args,
		}
		argsJSON, _ := json.Marshal(argsDone)
		out = append(out, FormatResponsesSSE("response.function_call_arguments.done", string(argsJSON)))

		// item.done
		itemDone := map[string]any{
			"type":            "response.output_item.done",
			"sequence_number": nextSeq(),
			"output_index":    idx,
			"item": map[string]any{
				"id":        fmt.Sprintf("fc_%s", callID),
				"type":      "function_call",
				"status":    "completed",
				"arguments": args,
				"call_id":   callID,
				"name":      st.FuncNames[idx],
			},
		}
		itemJSON, _ := json.Marshal(itemDone)
		out = append(out, FormatResponsesSSE("response.output_item.done", string(itemJSON)))

		st.FuncItemDone[idx] = true
	}

	// Build response.completed
	completed := st.buildCompletedEvent(nextSeq)
	completedJSON, _ := json.Marshal(completed)
	out = append(out, FormatResponsesSSE("response.completed", string(completedJSON)))

	return out
}

// buildCompletedEvent builds the response.completed event payload.
func (st *StreamState) buildCompletedEvent(nextSeq func() int) map[string]any {
	response := map[string]any{
		"id":         st.ResponseID,
		"object":     "response",
		"created_at": st.Created,
		"status":     "completed",
		"background": false,
		"error":      nil,
	}

	// Add original request fields
	if len(st.OriginalRequest) > 0 {
		var req map[string]any
		if err := json.Unmarshal(st.OriginalRequest, &req); err == nil {
			if v, ok := req["instructions"]; ok {
				response["instructions"] = v
			}
			if v, ok := req["max_output_tokens"]; ok {
				response["max_output_tokens"] = v
			}
			if v, ok := req["model"]; ok {
				response["model"] = v
			}
			if v, ok := req["temperature"]; ok {
				response["temperature"] = v
			}
			if v, ok := req["tools"]; ok {
				response["tools"] = v
			}
			if v, ok := req["tool_choice"]; ok {
				response["tool_choice"] = v
			}
			if v, ok := req["store"]; ok {
				response["store"] = v
			}
			if v, ok := req["metadata"]; ok {
				response["metadata"] = v
			}
			if v, ok := req["reasoning"]; ok {
				response["reasoning"] = v
			}
		}
	}

	// Build output array
	outputArr := make([]any, 0)

	// Add messages
	for idx := range st.MsgItemAdded {
		txt := ""
		if b := st.MsgTextBuf[idx]; b != nil {
			txt = b.String()
		}
		outputArr = append(outputArr, map[string]any{
			"id":     fmt.Sprintf("msg_%s_%d", st.ResponseID, idx),
			"type":   "message",
			"status": "completed",
			"content": []map[string]any{
				{
					"type":        "output_text",
					"annotations": []any{},
					"logprobs":    []any{},
					"text":        txt,
				},
			},
			"role": "assistant",
		})
	}

	// Add function calls
	for idx, callID := range st.FuncCallIDs {
		if callID == "" {
			continue
		}
		args := ""
		if b := st.FuncArgsBuf[idx]; b != nil {
			args = b.String()
		}
		outputArr = append(outputArr, map[string]any{
			"id":        fmt.Sprintf("fc_%s", callID),
			"type":      "function_call",
			"status":    "completed",
			"arguments": args,
			"call_id":   callID,
			"name":      st.FuncNames[idx],
		})
	}

	if len(outputArr) > 0 {
		response["output"] = outputArr
	}

	// Add usage
	if st.UsageSeen {
		usage := map[string]any{
			"input_tokens":  st.PromptTokens,
			"output_tokens": st.CompletionTokens,
			"total_tokens":  st.TotalTokens,
		}
		if st.CachedTokens > 0 {
			usage["input_tokens_details"] = map[string]any{
				"cached_tokens": st.CachedTokens,
			}
		}
		if st.ReasoningTokens > 0 {
			usage["output_tokens_details"] = map[string]any{
				"reasoning_tokens": st.ReasoningTokens,
			}
		}
		response["usage"] = usage
	}

	return map[string]any{
		"type":            "response.completed",
		"sequence_number": nextSeq(),
		"response":        response,
	}
}
