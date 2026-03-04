// Package translator provides format conversion between OpenAI API formats.
// This file contains tool name shortening utilities for Codex compatibility.
package translator

import (
	"strconv"
	"strings"
)

const maxToolNameLength = 64

// ShortenNameIfNeeded applies the shortening rule for a tool name.
// If the name length exceeds 64 characters, it will try to preserve the "mcp__" prefix
// and last segment. Otherwise it truncates to 64 characters.
func ShortenNameIfNeeded(name string) string {
	if len(name) <= maxToolNameLength {
		return name
	}

	if strings.HasPrefix(name, "mcp__") {
		// Keep prefix and last segment after '__'
		idx := strings.LastIndex(name, "__")
		if idx > 0 {
			candidate := "mcp__" + name[idx+2:]
			if len(candidate) > maxToolNameLength {
				return candidate[:maxToolNameLength]
			}
			return candidate
		}
	}

	return name[:maxToolNameLength]
}

// BuildShortNameMap generates unique short names (<=64 chars) for the given list of names.
// It preserves the "mcp__" prefix with the last segment when possible and ensures uniqueness
// by appending suffixes like "_1", "_2" if needed.
// Returns a map from original name to shortened name.
func BuildShortNameMap(names []string) map[string]string {
	used := make(map[string]struct{})
	result := make(map[string]string)

	baseCandidate := func(n string) string {
		if len(n) <= maxToolNameLength {
			return n
		}
		if strings.HasPrefix(n, "mcp__") {
			idx := strings.LastIndex(n, "__")
			if idx > 0 {
				cand := "mcp__" + n[idx+2:]
				if len(cand) > maxToolNameLength {
					cand = cand[:maxToolNameLength]
				}
				return cand
			}
		}
		return n[:maxToolNameLength]
	}

	makeUnique := func(cand string) string {
		if _, ok := used[cand]; !ok {
			return cand
		}
		base := cand
		for i := 1; ; i++ {
			suffix := "_" + strconv.Itoa(i)
			allowed := maxToolNameLength - len(suffix)
			if allowed < 0 {
				allowed = 0
			}
			tmp := base
			if len(tmp) > allowed {
				tmp = tmp[:allowed]
			}
			tmp = tmp + suffix
			if _, ok := used[tmp]; !ok {
				return tmp
			}
		}
	}

	for _, n := range names {
		cand := baseCandidate(n)
		uniq := makeUnique(cand)
		used[uniq] = struct{}{}
		result[n] = uniq
	}

	return result
}

// ReverseNameMap creates a reverse mapping from short name to original name.
func ReverseNameMap(m map[string]string) map[string]string {
	reverse := make(map[string]string, len(m))
	for orig, short := range m {
		reverse[short] = orig
	}
	return reverse
}
