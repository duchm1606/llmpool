// Package translator provides format conversion between API formats.
package translator

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/duchoang/llmpool/internal/domain/anthropic"
)

// Constants for preprocessing
const (
	// CompactSystemPromptStart is the prefix that identifies compact/summarization requests.
	CompactSystemPromptStart = "You are a helpful AI assistant tasked with summarizing conversations"

	// ThinkingTextPlaceholder is used when thinking text is empty but signature exists.
	// Compatible with opencode which filters out empty thinking blocks.
	ThinkingTextPlaceholder = "Thinking..."

	// SubagentMarkerPrefix identifies subagent markers in system-reminder tags.
	SubagentMarkerPrefix = "__SUBAGENT_MARKER__"
)

// SubagentMarker contains information about a subagent request.
type SubagentMarker struct {
	SessionID string `json:"session_id"`
	AgentID   string `json:"agent_id"`
	AgentType string `json:"agent_type"`
}

// IsCompactRequest detects if a request is a compact/summarization request.
// These requests are typically for context compression and should use a smaller model.
func IsCompactRequest(req *anthropic.MessagesRequest) bool {
	if len(req.System) == 0 {
		return false
	}

	for _, block := range req.System {
		if strings.HasPrefix(block.Text, CompactSystemPromptStart) {
			return true
		}
	}
	return false
}

// ParseSubagentMarker extracts a subagent marker from the first user message.
// Returns nil if no marker is found.
func ParseSubagentMarker(req *anthropic.MessagesRequest) *SubagentMarker {
	// Find first user message
	var firstUserMsg *anthropic.Message
	for i := range req.Messages {
		if req.Messages[i].Role == "user" {
			firstUserMsg = &req.Messages[i]
			break
		}
	}
	if firstUserMsg == nil {
		return nil
	}

	// Search through text blocks for subagent marker
	for _, block := range firstUserMsg.Content {
		if block.Type != "text" {
			continue
		}
		if marker := parseSubagentMarkerFromSystemReminder(block.Text); marker != nil {
			return marker
		}
	}

	return nil
}

// parseSubagentMarkerFromSystemReminder extracts a marker from system-reminder tags.
func parseSubagentMarkerFromSystemReminder(text string) *SubagentMarker {
	startTag := "<system-reminder>"
	endTag := "</system-reminder>"
	searchFrom := 0

	for {
		reminderStart := strings.Index(text[searchFrom:], startTag)
		if reminderStart == -1 {
			break
		}
		reminderStart += searchFrom

		contentStart := reminderStart + len(startTag)
		reminderEnd := strings.Index(text[contentStart:], endTag)
		if reminderEnd == -1 {
			break
		}
		reminderEnd += contentStart

		reminderContent := text[contentStart:reminderEnd]
		markerIndex := strings.Index(reminderContent, SubagentMarkerPrefix)
		if markerIndex == -1 {
			searchFrom = reminderEnd + len(endTag)
			continue
		}

		markerJSON := strings.TrimSpace(reminderContent[markerIndex+len(SubagentMarkerPrefix):])

		var marker SubagentMarker
		if err := json.Unmarshal([]byte(markerJSON), &marker); err != nil {
			searchFrom = reminderEnd + len(endTag)
			continue
		}

		if marker.SessionID == "" || marker.AgentID == "" || marker.AgentType == "" {
			searchFrom = reminderEnd + len(endTag)
			continue
		}

		return &marker
	}

	return nil
}

// MergeToolResultForClaude merges tool_result and text blocks into tool_result blocks.
// This optimization prevents consuming premium requests for certain Claude Code patterns
// like skill invocations, edit hooks, plan reminders, etc.
func MergeToolResultForClaude(req *anthropic.MessagesRequest) {
	for i := range req.Messages {
		msg := &req.Messages[i]
		if msg.Role != "user" || len(msg.Content) == 0 {
			continue
		}

		var toolResults []anthropic.ContentBlock
		var textBlocks []anthropic.ContentBlock
		valid := true

		for _, block := range msg.Content {
			switch block.Type {
			case "tool_result":
				toolResults = append(toolResults, block)
			case "text":
				textBlocks = append(textBlocks, block)
			default:
				// Other block types (like image) break the merge
				valid = false
			}
			if !valid {
				break
			}
		}

		// Only merge if we have both tool_results and text blocks
		if !valid || len(toolResults) == 0 || len(textBlocks) == 0 {
			continue
		}

		msg.Content = mergeToolResult(toolResults, textBlocks)
	}
}

// mergeToolResult combines tool_result blocks with text blocks.
func mergeToolResult(toolResults, textBlocks []anthropic.ContentBlock) []anthropic.ContentBlock {
	// Equal lengths: pairwise merge
	if len(toolResults) == len(textBlocks) {
		result := make([]anthropic.ContentBlock, len(toolResults))
		for i, tr := range toolResults {
			result[i] = mergeContentWithText(tr, textBlocks[i])
		}
		return result
	}

	// Unequal lengths: append all text blocks to the last tool_result
	result := make([]anthropic.ContentBlock, len(toolResults))
	copy(result, toolResults)
	lastIdx := len(result) - 1
	result[lastIdx] = mergeContentWithTexts(toolResults[lastIdx], textBlocks)
	return result
}

// mergeContentWithText merges a single text block into a tool_result.
func mergeContentWithText(tr, textBlock anthropic.ContentBlock) anthropic.ContentBlock {
	result := tr
	switch content := tr.Content.(type) {
	case string:
		result.Content = content + "\n\n" + textBlock.Text
	case []anthropic.ContentBlock:
		result.Content = append(content, anthropic.ContentBlock{
			Type: "text",
			Text: textBlock.Text,
		})
	default:
		// If content is nil or other type, just set text
		result.Content = textBlock.Text
	}
	return result
}

// mergeContentWithTexts merges multiple text blocks into a tool_result.
func mergeContentWithTexts(tr anthropic.ContentBlock, textBlocks []anthropic.ContentBlock) anthropic.ContentBlock {
	result := tr
	switch content := tr.Content.(type) {
	case string:
		var texts []string
		for _, tb := range textBlocks {
			texts = append(texts, tb.Text)
		}
		result.Content = content + "\n\n" + strings.Join(texts, "\n\n")
	case []anthropic.ContentBlock:
		for _, tb := range textBlocks {
			content = append(content, anthropic.ContentBlock{
				Type: "text",
				Text: tb.Text,
			})
		}
		result.Content = content
	default:
		// If content is nil or other type, combine texts
		var texts []string
		for _, tb := range textBlocks {
			texts = append(texts, tb.Text)
		}
		result.Content = strings.Join(texts, "\n\n")
	}
	return result
}

// FilterThinkingBlocks filters invalid thinking blocks from assistant messages.
// For Claude models, removes thinking blocks with:
// - Empty thinking text (except placeholder)
// - Invalid signatures (containing @, which indicates GPT format)
func FilterThinkingBlocks(req *anthropic.MessagesRequest, modelID string) {
	if !strings.HasPrefix(modelID, "claude") {
		return
	}

	for i := range req.Messages {
		msg := &req.Messages[i]
		if msg.Role != "assistant" || len(msg.Content) == 0 {
			continue
		}

		filtered := make([]anthropic.ContentBlock, 0, len(msg.Content))
		for _, block := range msg.Content {
			if block.Type != "thinking" {
				filtered = append(filtered, block)
				continue
			}

			// Filter out invalid thinking blocks for Claude
			if block.Thinking == "" || block.Thinking == ThinkingTextPlaceholder {
				continue
			}
			if block.Signature == "" {
				continue
			}
			// GPT signatures contain @, Claude signatures don't
			if strings.Contains(block.Signature, "@") {
				continue
			}

			filtered = append(filtered, block)
		}
		msg.Content = filtered
	}
}

// gptVersionRegex matches GPT model version numbers.
var gptVersionRegex = regexp.MustCompile(`^gpt-(\d+)`)

// IsGPT5OrLater checks if a model ID is GPT-5 or a later version.
func IsGPT5OrLater(modelID string) bool {
	matches := gptVersionRegex.FindStringSubmatch(modelID)
	if len(matches) < 2 {
		return false
	}
	version := 0
	for _, c := range matches[1] {
		if c >= '0' && c <= '9' {
			version = version*10 + int(c-'0')
		} else {
			break
		}
	}
	return version >= 5
}

// TranslateModelName normalizes model names for Copilot API.
// Subagent requests may use specific model numbers that Copilot doesn't support.
func TranslateModelName(model string) string {
	// claude-sonnet-4-20250514 -> claude-sonnet-4
	if strings.HasPrefix(model, "claude-sonnet-4-") {
		return "claude-sonnet-4"
	}
	// claude-opus-4-20250514 -> claude-opus-4
	if strings.HasPrefix(model, "claude-opus-4-") {
		return "claude-opus-4"
	}
	return model
}

// GetEffortForModel converts reasoning effort to Anthropic effort level.
func GetEffortForModel(reasoningEffort string) string {
	switch reasoningEffort {
	case "xhigh":
		return "max"
	case "none", "minimal":
		return "low"
	case "low", "medium", "high":
		return reasoningEffort
	default:
		return "medium"
	}
}
