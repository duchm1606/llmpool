// Package translator provides format conversion between API formats.
package translator

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/duchoang/llmpool/internal/domain/anthropic"
	"github.com/google/uuid"
)

// CopilotToAnthropicStreamState maintains state for converting Copilot streaming events
// to Anthropic streaming events.
type CopilotToAnthropicStreamState struct {
	MessageID    string
	Model        string
	Created      int64
	InputTokens  int
	OutputTokens int

	// Cache-related token counts
	CacheReadInputTokens     int
	CacheCreationInputTokens int

	// Track content blocks
	CurrentBlockIndex int
	BlockStarted      map[int]bool
	BlockType         map[int]string // "text" or "tool_use"
	ToolCallIDs       map[int]string
	ToolCallNames     map[int]string
	TextAccumulator   map[int]*strings.Builder
	JSONAccumulator   map[int]*strings.Builder

	// Track overall state
	Started    bool
	Completed  bool
	StopReason string

	// Track if we've emitted final events (message_delta + message_stop)
	FinalEventsEmitted bool
	// Track closed blocks to avoid duplicate content_block_stop
	BlockClosed map[int]bool
}

// NewCopilotToAnthropicStreamState creates a new stream state tracker.
func NewCopilotToAnthropicStreamState(model string) *CopilotToAnthropicStreamState {
	return &CopilotToAnthropicStreamState{
		MessageID:         fmt.Sprintf("msg_%s", uuid.New().String()[:24]),
		Model:             model,
		Created:           time.Now().Unix(),
		CurrentBlockIndex: -1,
		BlockStarted:      make(map[int]bool),
		BlockType:         make(map[int]string),
		ToolCallIDs:       make(map[int]string),
		ToolCallNames:     make(map[int]string),
		TextAccumulator:   make(map[int]*strings.Builder),
		JSONAccumulator:   make(map[int]*strings.Builder),
		BlockClosed:       make(map[int]bool),
	}
}

// ConvertCopilotEventToAnthropic converts a Copilot Responses API SSE event
// to Anthropic Messages API SSE events.
// Returns a list of SSE event strings (each in "event: X\ndata: Y\n\n" format).
func (s *CopilotToAnthropicStreamState) ConvertCopilotEventToAnthropic(eventData []byte) []string {
	var root map[string]any
	if err := json.Unmarshal(eventData, &root); err != nil {
		return nil
	}

	eventType, _ := root["type"].(string)
	events := make([]string, 0)

	switch eventType {
	case "response.created":
		// Emit message_start
		if !s.Started {
			s.Started = true
			events = append(events, s.formatMessageStart())
		}

	case "response.in_progress":
		// Emit message_start if not already done
		if !s.Started {
			s.Started = true
			events = append(events, s.formatMessageStart())
		}

	case "response.output_item.added":
		// New output item (text or function_call)
		item, ok := root["item"].(map[string]any)
		if !ok {
			return events
		}

		itemType, _ := item["type"].(string)
		s.CurrentBlockIndex++
		idx := s.CurrentBlockIndex

		switch itemType {
		case "message":
			// Text content - will be populated by response.output_text.delta
			s.BlockType[idx] = "text"
			s.BlockStarted[idx] = false

		case "function_call":
			// Tool use
			s.BlockType[idx] = "tool_use"
			s.ToolCallIDs[idx], _ = item["call_id"].(string)
			if s.ToolCallIDs[idx] == "" {
				s.ToolCallIDs[idx], _ = item["id"].(string)
			}
			s.ToolCallNames[idx], _ = item["name"].(string)
			s.BlockStarted[idx] = false
		}

	case "response.content_part.added":
		// Content part within a message output item
		// This signals the start of a text block
		s.CurrentBlockIndex++
		idx := s.CurrentBlockIndex
		s.BlockType[idx] = "text"
		s.BlockStarted[idx] = false

	case "response.output_text.delta":
		// Text delta
		delta, _ := root["delta"].(string)
		if delta == "" {
			return events
		}

		// Find or create text block
		idx := s.findOrCreateTextBlock()

		// Emit content_block_start if needed
		if !s.BlockStarted[idx] {
			s.BlockStarted[idx] = true
			events = append(events, s.formatContentBlockStart(idx, "text", "", ""))
		}

		// Emit text_delta
		events = append(events, s.formatTextDelta(idx, delta))
		if s.TextAccumulator[idx] == nil {
			s.TextAccumulator[idx] = &strings.Builder{}
		}
		_, _ = s.TextAccumulator[idx].WriteString(delta)

	case "response.function_call.arguments.delta":
		// Function arguments delta
		delta, _ := root["delta"].(string)
		if delta == "" {
			return events
		}

		// Find current tool_use block
		idx := s.findCurrentToolBlock()
		if idx < 0 {
			return events
		}

		// Emit content_block_start if needed
		if !s.BlockStarted[idx] {
			s.BlockStarted[idx] = true
			events = append(events, s.formatContentBlockStart(idx, "tool_use", s.ToolCallIDs[idx], s.ToolCallNames[idx]))
		}

		// Emit input_json_delta
		events = append(events, s.formatInputJSONDelta(idx, delta))
		if s.JSONAccumulator[idx] == nil {
			s.JSONAccumulator[idx] = &strings.Builder{}
		}
		_, _ = s.JSONAccumulator[idx].WriteString(delta)

	case "response.output_text.done":
		// Text block complete
		idx := s.findCurrentTextBlock()
		if idx >= 0 && s.BlockStarted[idx] && !s.BlockClosed[idx] {
			events = append(events, s.formatContentBlockStop(idx))
		}

	case "response.function_call.arguments.done":
		// Function call complete
		idx := s.findCurrentToolBlock()
		if idx >= 0 && s.BlockStarted[idx] && !s.BlockClosed[idx] {
			events = append(events, s.formatContentBlockStop(idx))
		}

	case "response.output_item.done":
		// Output item complete
		// The block stop is already handled by the specific done events

	case "response.completed":
		// Response complete
		s.Completed = true

		// Extract usage including cache tokens
		if response, ok := root["response"].(map[string]any); ok {
			if usage, ok := response["usage"].(map[string]any); ok {
				if v, ok := usage["input_tokens"].(float64); ok {
					s.InputTokens = int(v)
				}
				if v, ok := usage["output_tokens"].(float64); ok {
					s.OutputTokens = int(v)
				}
				// Extract cache-related token counts
				if v, ok := usage["cache_read_input_tokens"].(float64); ok {
					s.CacheReadInputTokens = int(v)
				}
				if v, ok := usage["cache_creation_input_tokens"].(float64); ok {
					s.CacheCreationInputTokens = int(v)
				}
			}
			// Extract stop reason
			if status, ok := response["status"].(string); ok && status == "completed" {
				// Determine stop reason from output
				if output, ok := response["output"].([]any); ok {
					hasToolUse := false
					for _, item := range output {
						if itemMap, ok := item.(map[string]any); ok {
							if t, _ := itemMap["type"].(string); t == "function_call" {
								hasToolUse = true
								break
							}
						}
					}
					if hasToolUse {
						s.StopReason = "tool_use"
					} else {
						s.StopReason = "end_turn"
					}
				}
			}
		}

		// Close any unclosed blocks (skip already closed ones)
		for idx, started := range s.BlockStarted {
			if started && !s.BlockClosed[idx] {
				events = append(events, s.formatContentBlockStop(idx))
			}
		}

		// Emit message_delta with stop_reason
		events = append(events, s.formatMessageDelta())

		// Emit message_stop
		events = append(events, s.formatMessageStop())
		s.FinalEventsEmitted = true

	case "error":
		// Error event
		if errObj, ok := root["error"].(map[string]any); ok {
			errType, _ := errObj["type"].(string)
			errMsg, _ := errObj["message"].(string)
			events = append(events, s.formatError(errType, errMsg))
		}
	}

	return events
}

// Helper methods for finding blocks
func (s *CopilotToAnthropicStreamState) findOrCreateTextBlock() int {
	// Find existing text block that's not done
	for idx, t := range s.BlockType {
		if t == "text" {
			return idx
		}
	}
	// Create new text block
	s.CurrentBlockIndex++
	idx := s.CurrentBlockIndex
	s.BlockType[idx] = "text"
	s.BlockStarted[idx] = false
	return idx
}

func (s *CopilotToAnthropicStreamState) findCurrentTextBlock() int {
	for idx, t := range s.BlockType {
		if t == "text" && s.BlockStarted[idx] {
			return idx
		}
	}
	return -1
}

func (s *CopilotToAnthropicStreamState) findCurrentToolBlock() int {
	// Find the most recent tool_use block
	maxIdx := -1
	for idx, t := range s.BlockType {
		if t == "tool_use" && idx > maxIdx {
			maxIdx = idx
		}
	}
	return maxIdx
}

// Formatting methods
func (s *CopilotToAnthropicStreamState) formatMessageStart() string {
	event := anthropic.MessageStartEvent{
		Type: "message_start",
		Message: &anthropic.MessageStartPayload{
			ID:      s.MessageID,
			Type:    "message",
			Role:    "assistant",
			Model:   s.Model,
			Content: []any{},
			Usage:   &anthropic.Usage{InputTokens: 0, OutputTokens: 0},
		},
	}
	data, _ := json.Marshal(event)
	return fmt.Sprintf("event: message_start\ndata: %s\n\n", data)
}

func (s *CopilotToAnthropicStreamState) formatContentBlockStart(index int, blockType, toolID, toolName string) string {
	var contentBlock *anthropic.ContentBlockPayload
	switch blockType {
	case "text":
		contentBlock = &anthropic.ContentBlockPayload{
			Type: "text",
			Text: "",
		}
	case "tool_use":
		contentBlock = &anthropic.ContentBlockPayload{
			Type:  "tool_use",
			ID:    toolID,
			Name:  toolName,
			Input: map[string]any{},
		}
	}

	event := anthropic.ContentBlockStartEvent{
		Type:         "content_block_start",
		Index:        index,
		ContentBlock: contentBlock,
	}
	data, _ := json.Marshal(event)
	return fmt.Sprintf("event: content_block_start\ndata: %s\n\n", data)
}

func (s *CopilotToAnthropicStreamState) formatTextDelta(index int, text string) string {
	event := anthropic.ContentBlockDeltaEvent{
		Type:  "content_block_delta",
		Index: index,
		Delta: &anthropic.Delta{
			Type: "text_delta",
			Text: text,
		},
	}
	data, _ := json.Marshal(event)
	return fmt.Sprintf("event: content_block_delta\ndata: %s\n\n", data)
}

func (s *CopilotToAnthropicStreamState) formatInputJSONDelta(index int, partialJSON string) string {
	event := anthropic.ContentBlockDeltaEvent{
		Type:  "content_block_delta",
		Index: index,
		Delta: &anthropic.Delta{
			Type:        "input_json_delta",
			PartialJSON: partialJSON,
		},
	}
	data, _ := json.Marshal(event)
	return fmt.Sprintf("event: content_block_delta\ndata: %s\n\n", data)
}

func (s *CopilotToAnthropicStreamState) formatContentBlockStop(index int) string {
	// Mark block as closed to avoid duplicate stops
	s.BlockClosed[index] = true

	event := anthropic.ContentBlockStopEvent{
		Type:  "content_block_stop",
		Index: index,
	}
	data, _ := json.Marshal(event)
	return fmt.Sprintf("event: content_block_stop\ndata: %s\n\n", data)
}

func (s *CopilotToAnthropicStreamState) formatMessageDelta() string {
	stopReason := s.StopReason
	if stopReason == "" {
		stopReason = "end_turn"
	}

	// Build usage with all available fields
	usage := &anthropic.DeltaUsage{
		InputTokens:  s.InputTokens,
		OutputTokens: s.OutputTokens,
	}
	if s.CacheReadInputTokens > 0 {
		usage.CacheReadInputTokens = s.CacheReadInputTokens
	}
	if s.CacheCreationInputTokens > 0 {
		usage.CacheCreationTokens = s.CacheCreationInputTokens
	}

	event := anthropic.MessageDeltaEvent{
		Type: "message_delta",
		Delta: &anthropic.MessageDeltaPayload{
			StopReason:   stopReason,
			StopSequence: nil, // Explicitly nil, serializes as null
		},
		Usage: usage,
	}
	data, _ := json.Marshal(event)
	return fmt.Sprintf("event: message_delta\ndata: %s\n\n", data)
}

func (s *CopilotToAnthropicStreamState) formatMessageStop() string {
	event := anthropic.MessageStopEvent{
		Type: "message_stop",
	}
	data, _ := json.Marshal(event)
	return fmt.Sprintf("event: message_stop\ndata: %s\n\n", data)
}

func (s *CopilotToAnthropicStreamState) formatError(errType, errMsg string) string {
	event := anthropic.ErrorEvent{
		Type: "error",
		Error: &anthropic.ErrorDetail{
			Type:    errType,
			Message: errMsg,
		},
	}
	data, _ := json.Marshal(event)
	return fmt.Sprintf("event: error\ndata: %s\n\n", data)
}

// Finalize ensures all necessary closing events are emitted.
func (s *CopilotToAnthropicStreamState) Finalize() []string {
	// Don't emit if already completed or final events already emitted
	if s.Completed || s.FinalEventsEmitted {
		return nil
	}

	events := make([]string, 0)

	// Close any open blocks that haven't been closed yet
	for idx, started := range s.BlockStarted {
		if started && !s.BlockClosed[idx] {
			events = append(events, s.formatContentBlockStop(idx))
		}
	}

	// Emit message_delta
	events = append(events, s.formatMessageDelta())

	// Emit message_stop
	events = append(events, s.formatMessageStop())

	s.Completed = true
	s.FinalEventsEmitted = true
	return events
}

// ConvertChatCompletionEventToAnthropic converts an OpenAI Chat Completion SSE event
// to Anthropic Messages API SSE events.
// Chat Completion uses a different streaming format than Responses API.
func (s *CopilotToAnthropicStreamState) ConvertChatCompletionEventToAnthropic(eventData []byte) []string {
	var chunk struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		Created int64  `json:"created"`
		Model   string `json:"model"`
		Choices []struct {
			Index int `json:"index"`
			Delta struct {
				Role      string `json:"role,omitempty"`
				Content   string `json:"content,omitempty"`
				ToolCalls []struct {
					Index    int    `json:"index"`
					ID       string `json:"id,omitempty"`
					Type     string `json:"type,omitempty"`
					Function struct {
						Name      string `json:"name,omitempty"`
						Arguments string `json:"arguments,omitempty"`
					} `json:"function,omitempty"`
				} `json:"tool_calls,omitempty"`
			} `json:"delta"`
			FinishReason *string `json:"finish_reason"`
		} `json:"choices"`
		Usage *struct {
			PromptTokens        int `json:"prompt_tokens"`
			CompletionTokens    int `json:"completion_tokens"`
			TotalTokens         int `json:"total_tokens"`
			PromptTokensDetails *struct {
				CachedTokens int `json:"cached_tokens"`
			} `json:"prompt_tokens_details,omitempty"`
		} `json:"usage,omitempty"`
	}

	if err := json.Unmarshal(eventData, &chunk); err != nil {
		return nil
	}

	events := make([]string, 0)

	// Update message ID and created if we have them
	if chunk.ID != "" && s.MessageID == "" {
		s.MessageID = "msg_" + chunk.ID
	}
	if chunk.Created != 0 && s.Created == 0 {
		s.Created = chunk.Created
	}
	if chunk.Model != "" && s.Model == "" {
		s.Model = chunk.Model
	}

	// Capture usage if present (may come in a separate final chunk)
	if chunk.Usage != nil {
		s.InputTokens = chunk.Usage.PromptTokens
		s.OutputTokens = chunk.Usage.CompletionTokens
		// Extract cached tokens if available
		if chunk.Usage.PromptTokensDetails != nil {
			s.CacheReadInputTokens = chunk.Usage.PromptTokensDetails.CachedTokens
		}
	}

	// Emit message_start on first chunk
	if !s.Started {
		s.Started = true
		events = append(events, s.formatMessageStart())
	}

	// Process choices
	if len(chunk.Choices) == 0 {
		// Usage-only chunk (common at end of stream)
		// If we've already marked completed but haven't emitted final events, emit them now
		if s.Completed && !s.FinalEventsEmitted {
			events = append(events, s.formatMessageDelta())
			events = append(events, s.formatMessageStop())
			s.FinalEventsEmitted = true
		}
		return events
	}

	choice := chunk.Choices[0]
	delta := choice.Delta

	// Handle text content
	if delta.Content != "" {
		idx := s.findOrCreateTextBlock()

		// Emit content_block_start if needed
		if !s.BlockStarted[idx] {
			s.BlockStarted[idx] = true
			events = append(events, s.formatContentBlockStart(idx, "text", "", ""))
		}

		// Emit text_delta
		events = append(events, s.formatTextDelta(idx, delta.Content))
		if s.TextAccumulator[idx] == nil {
			s.TextAccumulator[idx] = &strings.Builder{}
		}
		_, _ = s.TextAccumulator[idx].WriteString(delta.Content)
	}

	// Handle tool calls
	for _, tc := range delta.ToolCalls {
		// Find or create tool block for this index
		toolBlockIdx := s.findOrCreateToolBlockByIndex(tc.Index)

		// Store tool ID and name if provided (usually in first delta)
		if tc.ID != "" {
			s.ToolCallIDs[toolBlockIdx] = tc.ID
		}
		if tc.Function.Name != "" {
			s.ToolCallNames[toolBlockIdx] = tc.Function.Name
		}

		// Emit content_block_start if we have ID and name and haven't started
		if !s.BlockStarted[toolBlockIdx] && s.ToolCallIDs[toolBlockIdx] != "" && s.ToolCallNames[toolBlockIdx] != "" {
			s.BlockStarted[toolBlockIdx] = true
			events = append(events, s.formatContentBlockStart(toolBlockIdx, "tool_use", s.ToolCallIDs[toolBlockIdx], s.ToolCallNames[toolBlockIdx]))
		}

		// Emit argument delta if present
		if tc.Function.Arguments != "" && s.BlockStarted[toolBlockIdx] {
			events = append(events, s.formatInputJSONDelta(toolBlockIdx, tc.Function.Arguments))
			if s.JSONAccumulator[toolBlockIdx] == nil {
				s.JSONAccumulator[toolBlockIdx] = &strings.Builder{}
			}
			_, _ = s.JSONAccumulator[toolBlockIdx].WriteString(tc.Function.Arguments)
		}
	}

	// Handle finish_reason
	if choice.FinishReason != nil {
		s.Completed = true

		// Close any open text blocks that aren't already closed
		for idx, started := range s.BlockStarted {
			if started && s.BlockType[idx] == "text" && !s.BlockClosed[idx] {
				events = append(events, s.formatContentBlockStop(idx))
			}
		}

		// Close any open tool blocks that aren't already closed
		for idx, started := range s.BlockStarted {
			if started && s.BlockType[idx] == "tool_use" && !s.BlockClosed[idx] {
				events = append(events, s.formatContentBlockStop(idx))
			}
		}

		// Convert finish_reason to Anthropic stop_reason
		switch *choice.FinishReason {
		case "stop":
			s.StopReason = "end_turn"
		case "length":
			s.StopReason = "max_tokens"
		case "tool_calls":
			s.StopReason = "tool_use"
		case "content_filter":
			s.StopReason = "end_turn"
		default:
			s.StopReason = "end_turn"
		}

		// If we already have usage data, emit final events now
		// Otherwise, defer until we get a usage chunk
		if s.InputTokens > 0 || s.OutputTokens > 0 {
			events = append(events, s.formatMessageDelta())
			events = append(events, s.formatMessageStop())
			s.FinalEventsEmitted = true
		}
		// If no usage yet, final events will be emitted when usage chunk arrives
		// or in Finalize()
	}

	return events
}

// findOrCreateToolBlockByIndex finds or creates a tool block for a specific tool call index.
func (s *CopilotToAnthropicStreamState) findOrCreateToolBlockByIndex(toolIndex int) int {
	// Map tool index to block index (offset by number of text blocks + 1)
	// This is a simple mapping strategy
	for idx, t := range s.BlockType {
		if t == "tool_use" {
			// Check if this is the right tool block based on position
			toolCount := 0
			for i := 0; i <= idx; i++ {
				if s.BlockType[i] == "tool_use" {
					if toolCount == toolIndex {
						return i
					}
					toolCount++
				}
			}
		}
	}

	// Create new tool block
	s.CurrentBlockIndex++
	idx := s.CurrentBlockIndex
	s.BlockType[idx] = "tool_use"
	s.BlockStarted[idx] = false
	return idx
}
