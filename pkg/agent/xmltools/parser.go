// PicoClaw - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 PicoClaw contributors

package xmltools

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// toolCallTag is the XML-style delimiter the orchestrator uses to express
// tool-call intentions. We parse these from the orchestrator's free-text
// response rather than relying on API-level tool_calls (which trigger
// safety refusals on many models).
const (
	toolCallOpenTag  = "<tool_call>"
	toolCallCloseTag = "</tool_call>"
)

// ParsedToolCall represents a tool call extracted from orchestrator text.
type ParsedToolCall struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// toolCallBlockRe matches <tool_call>...</tool_call> blocks, including
// those wrapped in markdown code fences. The regex is deliberately
// permissive to handle models that might wrap the JSON in backticks.
var toolCallBlockRe = regexp.MustCompile(
	`(?s)<tool_call>\s*` + // opening tag + optional whitespace
		"(?:`{0,3}(?:json)?\\s*)?" + // optional markdown fence
		`(\{.*?\})` + // capture the JSON object (non-greedy)
		"(?:\\s*`{0,3})?" + // optional closing fence
		`\s*</tool_call>`, // closing tag
)

// ParseToolCalls extracts all <tool_call> blocks from the orchestrator's
// text response and returns them as ParsedToolCall structs. Blocks that
// contain invalid JSON or are missing the "name" field are silently
// skipped so that a single malformed block doesn't abort the entire turn.
func ParseToolCalls(text string) []ParsedToolCall {
	if !strings.Contains(text, toolCallOpenTag) {
		return nil
	}

	matches := toolCallBlockRe.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return nil
	}

	calls := make([]ParsedToolCall, 0, len(matches))
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}

		jsonStr := strings.TrimSpace(m[1])

		// Always parse as a raw map for maximum flexibility.
		// This handles alternative field names (args, parameters)
		// that wouldn't match the struct's json:"arguments" tag.
		var raw map[string]any
		if err := json.Unmarshal([]byte(jsonStr), &raw); err != nil {
			continue // skip malformed JSON
		}

		var tc ParsedToolCall
		tc.Name, _ = raw["name"].(string)
		if tc.Name == "" {
			continue
		}

		// Try to extract arguments from various field names models might use
		if args, ok := raw["arguments"].(map[string]any); ok {
			tc.Arguments = args
		} else if args, ok := raw["args"].(map[string]any); ok {
			tc.Arguments = args
		} else if params, ok := raw["parameters"].(map[string]any); ok {
			tc.Arguments = params
		} else {
			// Use all fields except "name" as arguments
			tc.Arguments = make(map[string]any)
			for k, v := range raw {
				if k != "name" {
					tc.Arguments[k] = v
				}
			}
		}

		if tc.Arguments == nil {
			tc.Arguments = make(map[string]any)
		}

		calls = append(calls, tc)
	}

	return calls
}

// StripToolCallBlocks removes all <tool_call>...</tool_call> blocks from
// the text, returning the remaining content. This is used to extract the
// orchestrator's conversational text after parsing tool calls.
func StripToolCallBlocks(text string) string {
	result := toolCallBlockRe.ReplaceAllString(text, "")
	// Clean up leftover whitespace from removed blocks
	result = strings.TrimSpace(result)
	// Collapse multiple blank lines into one
	for strings.Contains(result, "\n\n\n") {
		result = strings.ReplaceAll(result, "\n\n\n", "\n\n")
	}
	return result
}

// HasToolCalls returns true if the text contains any <tool_call> blocks.
func HasToolCalls(text string) bool {
	return strings.Contains(text, toolCallOpenTag) && strings.Contains(text, toolCallCloseTag)
}

// FormatToolCallJSON formats a ParsedToolCall as a JSON string for logging.
func FormatToolCallJSON(tc ParsedToolCall) string {
	b, err := json.Marshal(tc)
	if err != nil {
		return fmt.Sprintf("{name: %q, error: %q}", tc.Name, err.Error())
	}
	return string(b)
}
