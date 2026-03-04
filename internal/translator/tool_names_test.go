package translator

import (
	"testing"
)

func TestShortenNameIfNeeded(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "short name unchanged",
			input:    "my_tool",
			expected: "my_tool",
		},
		{
			name:     "exactly 64 chars unchanged",
			input:    "this_is_a_very_long_name_that_exactly_reaches_sixty_four_charact",
			expected: "this_is_a_very_long_name_that_exactly_reaches_sixty_four_charact",
		},
		{
			name:     "long name truncated to 64",
			input:    "this_is_a_very_long_tool_name_that_exceeds_sixty_four_characters_limit_by_a_lot",
			expected: "this_is_a_very_long_tool_name_that_exceeds_sixty_four_characters",
		},
		{
			name:     "mcp prefix under 64 chars unchanged",
			input:    "mcp__server__tool_name",
			expected: "mcp__server__tool_name",
		},
		{
			name:     "mcp prefix over 64 chars preserves last segment",
			input:    "mcp__very_long_server_name_that_makes_total_over_64__another_segment__final_tool",
			expected: "mcp__final_tool",
		},
		{
			name:     "mcp prefix with long last segment truncates",
			input:    "mcp__server__this_is_a_very_long_final_segment_name_that_exceeds_the_limit_significantly",
			expected: "mcp__this_is_a_very_long_final_segment_name_that_exceeds_the_lim",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ShortenNameIfNeeded(tt.input)
			if result != tt.expected {
				t.Errorf("ShortenNameIfNeeded(%q) = %q, want %q", tt.input, result, tt.expected)
			}
			if len(result) > 64 {
				t.Errorf("result length %d exceeds 64 chars", len(result))
			}
		})
	}
}

func TestBuildShortNameMap(t *testing.T) {
	tests := []struct {
		name   string
		names  []string
		checks map[string]func(result map[string]string) bool
	}{
		{
			name:  "all short names unchanged",
			names: []string{"tool1", "tool2", "tool3"},
			checks: map[string]func(result map[string]string) bool{
				"tool1 unchanged": func(r map[string]string) bool { return r["tool1"] == "tool1" },
				"tool2 unchanged": func(r map[string]string) bool { return r["tool2"] == "tool2" },
				"tool3 unchanged": func(r map[string]string) bool { return r["tool3"] == "tool3" },
			},
		},
		{
			name: "duplicate long names get suffix for uniqueness",
			names: []string{
				"mcp__server1__this_is_a_very_long_tool_name_that_exceeds_64_chars_limit",
				"mcp__server2__this_is_a_very_long_tool_name_that_exceeds_64_chars_limit",
			},
			checks: map[string]func(result map[string]string) bool{
				"all unique": func(r map[string]string) bool {
					seen := make(map[string]bool)
					for _, v := range r {
						if seen[v] {
							return false
						}
						seen[v] = true
					}
					return true
				},
				"all <= 64 chars": func(r map[string]string) bool {
					for _, v := range r {
						if len(v) > 64 {
							return false
						}
					}
					return true
				},
			},
		},
		{
			name: "long names truncated and made unique",
			names: []string{
				"this_is_a_very_long_tool_name_that_exceeds_the_sixty_four_character_limit_v1",
				"this_is_a_very_long_tool_name_that_exceeds_the_sixty_four_character_limit_v2",
			},
			checks: map[string]func(result map[string]string) bool{
				"all <= 64 chars": func(r map[string]string) bool {
					for _, v := range r {
						if len(v) > 64 {
							return false
						}
					}
					return true
				},
				"all unique": func(r map[string]string) bool {
					seen := make(map[string]bool)
					for _, v := range r {
						if seen[v] {
							return false
						}
						seen[v] = true
					}
					return true
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildShortNameMap(tt.names)

			// Check all input names are in result
			for _, n := range tt.names {
				if _, ok := result[n]; !ok {
					t.Errorf("missing mapping for %q", n)
				}
			}

			// Run checks
			for checkName, checkFn := range tt.checks {
				if !checkFn(result) {
					t.Errorf("check %q failed, result: %v", checkName, result)
				}
			}
		})
	}
}

func TestReverseNameMap(t *testing.T) {
	m := map[string]string{
		"original1": "short1",
		"original2": "short2",
	}

	reversed := ReverseNameMap(m)

	if reversed["short1"] != "original1" {
		t.Errorf("expected reversed[short1] = original1, got %s", reversed["short1"])
	}
	if reversed["short2"] != "original2" {
		t.Errorf("expected reversed[short2] = original2, got %s", reversed["short2"])
	}
}
