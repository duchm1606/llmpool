// Package translator provides format conversion between OpenAI API formats.
// This file contains Codex-specific request transformations.
package translator

import (
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// TransformForCodex transforms an OpenAI Responses API request for Codex compatibility.
// Codex has specific requirements:
// - Rejects certain fields: context_management, service_tier, truncation, user, max_output_tokens, temperature, top_p
// - Requires include: ["reasoning.encrypted_content"]
// - Requires system role to be converted to developer
// - Forces stream: true, store: false, parallel_tool_calls: true
func TransformForCodex(rawJSON []byte) []byte {
	result := rawJSON

	// Handle string input by converting to structured array
	inputResult := gjson.GetBytes(result, "input")
	if inputResult.Type == gjson.String {
		input, _ := sjson.Set(`[{"type":"message","role":"user","content":[{"type":"input_text","text":""}]}]`, "0.content.0.text", inputResult.String())
		result, _ = sjson.SetRawBytes(result, "input", []byte(input))
	}

	// Set required fields
	result, _ = sjson.SetBytes(result, "stream", true)
	result, _ = sjson.SetBytes(result, "store", false)
	result, _ = sjson.SetBytes(result, "parallel_tool_calls", true)
	result, _ = sjson.SetBytes(result, "include", []string{"reasoning.encrypted_content"})

	// Delete unsupported fields that Codex rejects
	result, _ = sjson.DeleteBytes(result, "max_output_tokens")
	result, _ = sjson.DeleteBytes(result, "max_completion_tokens")
	result, _ = sjson.DeleteBytes(result, "temperature")
	result, _ = sjson.DeleteBytes(result, "top_p")
	result, _ = sjson.DeleteBytes(result, "service_tier")
	result, _ = sjson.DeleteBytes(result, "truncation")
	result, _ = sjson.DeleteBytes(result, "context_management")
	result, _ = sjson.DeleteBytes(result, "user")

	// Convert system role to developer in input array
	result = convertSystemRoleToDeveloper(result)

	return result
}

// convertSystemRoleToDeveloper traverses the input array and converts any message items
// with role "system" to role "developer". This is necessary because Codex API does not
// accept "system" role in the input array.
func convertSystemRoleToDeveloper(rawJSON []byte) []byte {
	inputResult := gjson.GetBytes(rawJSON, "input")
	if !inputResult.IsArray() {
		return rawJSON
	}

	inputArray := inputResult.Array()
	result := rawJSON

	for i := range inputArray {
		rolePath := "input." + itoa(i) + ".role"
		if gjson.GetBytes(result, rolePath).String() == "system" {
			result, _ = sjson.SetBytes(result, rolePath, "developer")
		}
	}

	return result
}

// itoa converts int to string without importing strconv (simple implementation for small numbers).
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	if n < 0 {
		return "-" + itoa(-n)
	}
	digits := ""
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	return digits
}

// IsCodexModel returns true if the model name indicates a Codex-compatible model.
// This helps determine when to apply Codex-specific transformations.
func IsCodexModel(model string) bool {
	// Codex models include o1, o3, o4-mini, gpt-4.1, gpt-5, etc.
	// For now, we'll check for specific prefixes that indicate Codex routing
	switch {
	case len(model) >= 2 && model[0] == 'o' && (model[1] >= '0' && model[1] <= '9'):
		// o1, o3, o4-mini, etc.
		return true
	case len(model) >= 5 && model[:5] == "gpt-5":
		return true
	case len(model) >= 7 && model[:7] == "gpt-4.1":
		return true
	default:
		return false
	}
}
