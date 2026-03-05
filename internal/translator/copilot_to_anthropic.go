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
	BlockType         map[int]string // "text", "tool_use", or "thinking"
	ToolCallIDs       map[int]string
	ToolCallNames     map[int]string
	ToolIndexToBlock  map[int]int
	TextAccumulator   map[int]*strings.Builder
	JSONAccumulator   map[int]*strings.Builder
	JSONEmittedBytes  map[int]int

	// Track overall state
	Started    bool
	Completed  bool
	StopReason string

	// Track if we've emitted final events (message_delta + message_stop)
	FinalEventsEmitted bool
	// Track closed blocks to avoid duplicate content_block_stop
	BlockClosed map[int]bool

	// Thinking block tracking
	ThinkingBlockOpen  bool
	ThinkingBlockIndex int
	ContentBlockOpen   bool // Tracks if a non-thinking content block is open
}

// NewCopilotToAnthropicStreamState creates a new stream state tracker.
func NewCopilotToAnthropicStreamState(model string) *CopilotToAnthropicStreamState {
	return &CopilotToAnthropicStreamState{
		MessageID:          fmt.Sprintf("msg_%s", uuid.New().String()[:24]),
		Model:              model,
		Created:            time.Now().Unix(),
		CurrentBlockIndex:  -1,
		BlockStarted:       make(map[int]bool),
		BlockType:          make(map[int]string),
		ToolCallIDs:        make(map[int]string),
		ToolCallNames:      make(map[int]string),
		ToolIndexToBlock:   make(map[int]int),
		TextAccumulator:    make(map[int]*strings.Builder),
		JSONAccumulator:    make(map[int]*strings.Builder),
		JSONEmittedBytes:   make(map[int]int),
		BlockClosed:        make(map[int]bool),
		ThinkingBlockIndex: -1,
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
		// New output item (text, function_call, or reasoning)
		item, ok := root["item"].(map[string]any)
		if !ok {
			return events
		}

		itemType, _ := item["type"].(string)
		outputIndex := -1
		if idx, ok := asInt(root["output_index"]); ok {
			outputIndex = idx
		}

		switch itemType {
		case "message":
			// Text content - will be populated by response.output_text.delta
			s.CurrentBlockIndex++
			idx := s.CurrentBlockIndex
			s.BlockType[idx] = "text"
			s.BlockStarted[idx] = false

		case "function_call":
			// Tool use
			s.CurrentBlockIndex++
			idx := s.CurrentBlockIndex
			s.BlockType[idx] = "tool_use"
			s.ToolCallIDs[idx], _ = item["call_id"].(string)
			if s.ToolCallIDs[idx] == "" {
				s.ToolCallIDs[idx], _ = item["id"].(string)
			}
			s.ToolCallNames[idx], _ = item["name"].(string)
			s.BlockStarted[idx] = false
			if outputIndex >= 0 {
				s.ToolIndexToBlock[outputIndex] = idx
			}

		case "reasoning":
			// Reasoning/thinking output - handled differently from message/function_call
			// The actual content comes via response.reasoning_summary_text.delta
			// We'll create the block when we receive the first delta
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

		// Find current tool_use block. Prefer explicit output_index mapping when available.
		idx := -1
		if outputIndex, ok := asInt(root["output_index"]); ok {
			if mapped, exists := s.ToolIndexToBlock[outputIndex]; exists {
				idx = mapped
			}
		}
		if idx < 0 {
			idx = s.findCurrentToolBlock()
		}
		if idx < 0 {
			return events
		}

		if s.JSONAccumulator[idx] == nil {
			s.JSONAccumulator[idx] = &strings.Builder{}
		}
		_, _ = s.JSONAccumulator[idx].WriteString(delta)

		// Emit content_block_start if needed
		if !s.BlockStarted[idx] {
			s.BlockStarted[idx] = true
			events = append(events, s.formatContentBlockStart(idx, "tool_use", s.ToolCallIDs[idx], s.ToolCallNames[idx]))
		}

		if jsonDelta := s.flushPendingInputJSONDelta(idx); jsonDelta != "" {
			events = append(events, jsonDelta)
		}

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

	case "response.reasoning_summary_text.delta":
		// Reasoning text delta - equivalent to thinking_delta
		delta, _ := root["delta"].(string)
		if delta == "" {
			return events
		}

		// Find or create thinking block
		if !s.ThinkingBlockOpen {
			s.CurrentBlockIndex++
			idx := s.CurrentBlockIndex
			s.BlockType[idx] = "thinking"
			s.BlockStarted[idx] = true
			s.ThinkingBlockIndex = idx
			s.ThinkingBlockOpen = true
			events = append(events, s.formatThinkingBlockStart(idx))
		}

		// Emit thinking_delta
		events = append(events, s.formatThinkingDelta(s.ThinkingBlockIndex, delta))

	case "response.reasoning_summary_text.done":
		// Reasoning text complete - no action needed, block will be closed by output_item.done

	case "response.output_item.done":
		// Output item complete - handle reasoning signature
		item, ok := root["item"].(map[string]any)
		if !ok {
			return events
		}

		itemType, _ := item["type"].(string)
		if itemType == "reasoning" {
			// Build signature from encrypted_content and id
			encryptedContent, _ := item["encrypted_content"].(string)
			id, _ := item["id"].(string)
			signature := encryptedContent + "@" + id

			// Find or create thinking block if not yet open
			if !s.ThinkingBlockOpen {
				s.CurrentBlockIndex++
				idx := s.CurrentBlockIndex
				s.BlockType[idx] = "thinking"
				s.BlockStarted[idx] = true
				s.ThinkingBlockIndex = idx
				s.ThinkingBlockOpen = true
				events = append(events, s.formatThinkingBlockStart(idx))

				// Add placeholder thinking text (compatible with opencode)
				events = append(events, s.formatThinkingDelta(idx, ThinkingTextPlaceholder))
			}

			// Emit signature_delta and close block
			if signature != "@" { // Only if there's actual content
				events = append(events, s.formatSignatureDelta(s.ThinkingBlockIndex, signature))
			}
			events = append(events, s.formatContentBlockStop(s.ThinkingBlockIndex))
			s.ThinkingBlockOpen = false
		}

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
	// Build content_block as a map to control exactly which fields appear.
	// Using a struct with omitempty would drop required fields like "text":"".
	var contentBlock any
	switch blockType {
	case "text":
		contentBlock = map[string]any{
			"type": "text",
			"text": "",
		}
	case "tool_use":
		contentBlock = map[string]any{
			"type":  "tool_use",
			"id":    toolID,
			"name":  toolName,
			"input": map[string]any{},
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

	// Close any open thinking block first
	events = append(events, s.closeThinkingBlockIfOpen()...)

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
				Role            string `json:"role,omitempty"`
				Content         string `json:"content,omitempty"`
				ReasoningText   string `json:"reasoning_text,omitempty"`   // Extended thinking text
				ReasoningOpaque string `json:"reasoning_opaque,omitempty"` // Signature for thinking
				ToolCalls       []struct {
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

	// Handle reasoning_text (thinking deltas)
	if delta.ReasoningText != "" {
		// Edge case: if content block is already open (abnormal server behavior),
		// treat reasoning_text as regular content
		if s.ContentBlockOpen {
			delta.Content = delta.ReasoningText
			delta.ReasoningText = ""
		} else {
			events = append(events, s.handleThinkingText(delta.ReasoningText)...)
		}
	}

	// Handle text content
	if delta.Content != "" {
		// Close thinking block if open before starting text content
		events = append(events, s.closeThinkingBlockIfOpen()...)

		// Check if a tool block was open - close it first
		if s.isToolBlockOpen() {
			events = append(events, s.closeCurrentBlock()...)
		}

		idx := s.findOrCreateTextBlock()

		// Emit content_block_start if needed
		if !s.BlockStarted[idx] {
			s.BlockStarted[idx] = true
			s.ContentBlockOpen = true
			events = append(events, s.formatContentBlockStart(idx, "text", "", ""))
		}

		// Emit text_delta
		events = append(events, s.formatTextDelta(idx, delta.Content))
		if s.TextAccumulator[idx] == nil {
			s.TextAccumulator[idx] = &strings.Builder{}
		}
		_, _ = s.TextAccumulator[idx].WriteString(delta.Content)
	}

	// Handle signature at end of thinking (when content is empty but reasoning_opaque exists)
	if delta.Content == "" && delta.ReasoningOpaque != "" && s.ThinkingBlockOpen {
		events = append(events, s.formatSignatureDelta(s.ThinkingBlockIndex, delta.ReasoningOpaque))
		events = append(events, s.formatContentBlockStop(s.ThinkingBlockIndex))
		s.ThinkingBlockOpen = false
	}

	// Handle tool calls
	if len(delta.ToolCalls) > 0 {
		// Close thinking block before tool calls
		events = append(events, s.closeThinkingBlockIfOpen()...)

		// Close non-tool content block if open
		if s.ContentBlockOpen && !s.isToolBlockOpen() {
			events = append(events, s.closeCurrentBlock()...)
		}

		// Handle reasoning_opaque within tool calls context (opaque thinking block)
		events = append(events, s.handleReasoningOpaque(delta.ReasoningOpaque)...)
	}

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

		if tc.Function.Arguments != "" {
			if s.JSONAccumulator[toolBlockIdx] == nil {
				s.JSONAccumulator[toolBlockIdx] = &strings.Builder{}
			}
			_, _ = s.JSONAccumulator[toolBlockIdx].WriteString(tc.Function.Arguments)
		}

		// Emit content_block_start if we have ID and name and haven't started
		if !s.BlockStarted[toolBlockIdx] && s.ToolCallIDs[toolBlockIdx] != "" && s.ToolCallNames[toolBlockIdx] != "" {
			s.BlockStarted[toolBlockIdx] = true
			s.ContentBlockOpen = true
			events = append(events, s.formatContentBlockStart(toolBlockIdx, "tool_use", s.ToolCallIDs[toolBlockIdx], s.ToolCallNames[toolBlockIdx]))
		}

		if s.BlockStarted[toolBlockIdx] {
			if jsonDelta := s.flushPendingInputJSONDelta(toolBlockIdx); jsonDelta != "" {
				events = append(events, jsonDelta)
			}
		}
	}

	// Handle finish_reason
	if choice.FinishReason != nil {
		s.Completed = true

		// Close any open content blocks first
		if s.ContentBlockOpen {
			events = append(events, s.closeCurrentBlock()...)
			// Handle reasoning_opaque after closing non-tool block
			if !s.isToolBlockOpen() {
				events = append(events, s.handleReasoningOpaque(delta.ReasoningOpaque)...)
			}
		}

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

		// Close any open thinking blocks that aren't already closed
		for idx, started := range s.BlockStarted {
			if started && s.BlockType[idx] == "thinking" && !s.BlockClosed[idx] {
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

// handleThinkingText handles reasoning_text deltas and emits thinking block events.
func (s *CopilotToAnthropicStreamState) handleThinkingText(text string) []string {
	events := make([]string, 0)

	if !s.ThinkingBlockOpen {
		// Start a new thinking block
		s.CurrentBlockIndex++
		idx := s.CurrentBlockIndex
		s.BlockType[idx] = "thinking"
		s.BlockStarted[idx] = true
		s.ThinkingBlockIndex = idx
		s.ThinkingBlockOpen = true

		events = append(events, s.formatThinkingBlockStart(idx))
	}

	// Emit thinking_delta
	events = append(events, s.formatThinkingDelta(s.ThinkingBlockIndex, text))
	return events
}

// handleReasoningOpaque emits an opaque thinking block (with placeholder text and signature).
func (s *CopilotToAnthropicStreamState) handleReasoningOpaque(opaque string) []string {
	if opaque == "" {
		return nil
	}

	events := make([]string, 0)

	// Create a complete thinking block with placeholder text and signature
	s.CurrentBlockIndex++
	idx := s.CurrentBlockIndex
	s.BlockType[idx] = "thinking"
	s.BlockStarted[idx] = true

	events = append(events, s.formatThinkingBlockStart(idx))
	events = append(events, s.formatThinkingDelta(idx, ThinkingTextPlaceholder))
	events = append(events, s.formatSignatureDelta(idx, opaque))
	events = append(events, s.formatContentBlockStop(idx))

	return events
}

// closeThinkingBlockIfOpen closes the current thinking block if one is open.
func (s *CopilotToAnthropicStreamState) closeThinkingBlockIfOpen() []string {
	if !s.ThinkingBlockOpen {
		return nil
	}

	events := make([]string, 0)
	// Emit empty signature_delta before closing (for completeness)
	events = append(events, s.formatSignatureDelta(s.ThinkingBlockIndex, ""))
	events = append(events, s.formatContentBlockStop(s.ThinkingBlockIndex))
	s.ThinkingBlockOpen = false

	return events
}

// closeCurrentBlock closes the current content block (non-thinking).
func (s *CopilotToAnthropicStreamState) closeCurrentBlock() []string {
	if !s.ContentBlockOpen {
		return nil
	}

	events := make([]string, 0)
	events = append(events, s.formatContentBlockStop(s.CurrentBlockIndex))
	s.ContentBlockOpen = false
	s.CurrentBlockIndex++

	return events
}

// isToolBlockOpen checks if the current open block is a tool block.
func (s *CopilotToAnthropicStreamState) isToolBlockOpen() bool {
	if !s.ContentBlockOpen {
		return false
	}
	// Check if any tool call maps to the current block index
	for _, toolIdx := range s.ToolIndexToBlock {
		if toolIdx == s.CurrentBlockIndex {
			return true
		}
	}
	return false
}

// formatThinkingBlockStart formats a thinking block start event.
func (s *CopilotToAnthropicStreamState) formatThinkingBlockStart(index int) string {
	contentBlock := map[string]any{
		"type":     "thinking",
		"thinking": "",
	}

	event := anthropic.ContentBlockStartEvent{
		Type:         "content_block_start",
		Index:        index,
		ContentBlock: contentBlock,
	}
	data, _ := json.Marshal(event)
	return fmt.Sprintf("event: content_block_start\ndata: %s\n\n", data)
}

// formatThinkingDelta formats a thinking delta event.
func (s *CopilotToAnthropicStreamState) formatThinkingDelta(index int, thinking string) string {
	event := anthropic.ContentBlockDeltaEvent{
		Type:  "content_block_delta",
		Index: index,
		Delta: &anthropic.Delta{
			Type:     "thinking_delta",
			Thinking: thinking,
		},
	}
	data, _ := json.Marshal(event)
	return fmt.Sprintf("event: content_block_delta\ndata: %s\n\n", data)
}

// formatSignatureDelta formats a signature delta event.
func (s *CopilotToAnthropicStreamState) formatSignatureDelta(index int, signature string) string {
	event := anthropic.ContentBlockDeltaEvent{
		Type:  "content_block_delta",
		Index: index,
		Delta: &anthropic.Delta{
			Type:      "signature_delta",
			Signature: signature,
		},
	}
	data, _ := json.Marshal(event)
	return fmt.Sprintf("event: content_block_delta\ndata: %s\n\n", data)
}

// findOrCreateToolBlockByIndex finds or creates a tool block for a specific tool call index.
func (s *CopilotToAnthropicStreamState) findOrCreateToolBlockByIndex(toolIndex int) int {
	if idx, ok := s.ToolIndexToBlock[toolIndex]; ok {
		return idx
	}

	// Create new tool block
	s.CurrentBlockIndex++
	idx := s.CurrentBlockIndex
	s.BlockType[idx] = "tool_use"
	s.BlockStarted[idx] = false
	s.ToolIndexToBlock[toolIndex] = idx
	return idx
}

func (s *CopilotToAnthropicStreamState) flushPendingInputJSONDelta(index int) string {
	b := s.JSONAccumulator[index]
	if b == nil {
		return ""
	}

	all := b.String()
	sent := s.JSONEmittedBytes[index]
	if sent >= len(all) {
		return ""
	}

	pending := all[sent:]
	s.JSONEmittedBytes[index] = len(all)
	return s.formatInputJSONDelta(index, pending)
}

func asInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	default:
		return 0, false
	}
}
