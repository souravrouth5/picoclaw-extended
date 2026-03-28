// PicoClaw - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 PicoClaw contributors

package xmltools

import (
	"strings"
	"testing"

	"github.com/sipeed/picoclaw/pkg/providers"
)

// --- Parser tests ---

func TestParseToolCalls_SingleCall(t *testing.T) {
	text := `I'll search for that information.

<tool_call>
{"name": "exec", "arguments": {"command": "ls -la"}}
</tool_call>

Let me check the results.`

	calls := ParseToolCalls(text)
	if len(calls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(calls))
	}
	if calls[0].Name != "exec" {
		t.Errorf("expected name 'exec', got %q", calls[0].Name)
	}
	cmd, ok := calls[0].Arguments["command"].(string)
	if !ok || cmd != "ls -la" {
		t.Errorf("expected command 'ls -la', got %v", calls[0].Arguments["command"])
	}
}

func TestParseToolCalls_MultipleCalls(t *testing.T) {
	text := `I need to do two things:

<tool_call>
{"name": "read_file", "arguments": {"path": "/tmp/test.txt"}}
</tool_call>

<tool_call>
{"name": "exec", "arguments": {"command": "echo hello"}}
</tool_call>`

	calls := ParseToolCalls(text)
	if len(calls) != 2 {
		t.Fatalf("expected 2 tool calls, got %d", len(calls))
	}
	if calls[0].Name != "read_file" {
		t.Errorf("call 0: expected 'read_file', got %q", calls[0].Name)
	}
	if calls[1].Name != "exec" {
		t.Errorf("call 1: expected 'exec', got %q", calls[1].Name)
	}
}

func TestParseToolCalls_WithMarkdownFence(t *testing.T) {
	text := `<tool_call>
` + "```json" + `
{"name": "exec", "arguments": {"command": "pwd"}}
` + "```" + `
</tool_call>`

	calls := ParseToolCalls(text)
	if len(calls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(calls))
	}
	if calls[0].Name != "exec" {
		t.Errorf("expected name 'exec', got %q", calls[0].Name)
	}
}

func TestParseToolCalls_NoToolCalls(t *testing.T) {
	text := "Just a regular response with no tool calls."
	calls := ParseToolCalls(text)
	if len(calls) != 0 {
		t.Fatalf("expected 0 tool calls, got %d", len(calls))
	}
}

func TestParseToolCalls_MalformedJSON(t *testing.T) {
	text := `<tool_call>
{not valid json}
</tool_call>`

	calls := ParseToolCalls(text)
	if len(calls) != 0 {
		t.Fatalf("expected 0 tool calls (malformed), got %d", len(calls))
	}
}

func TestParseToolCalls_MissingName(t *testing.T) {
	text := `<tool_call>
{"arguments": {"command": "ls"}}
</tool_call>`

	calls := ParseToolCalls(text)
	if len(calls) != 0 {
		t.Fatalf("expected 0 tool calls (no name), got %d", len(calls))
	}
}

func TestParseToolCalls_AlternativeArgFields(t *testing.T) {
	// Some models might use "args" or "parameters" instead of "arguments"
	tests := []struct {
		name string
		json string
	}{
		{"args field", `{"name": "exec", "args": {"command": "ls"}}`},
		{"parameters field", `{"name": "exec", "parameters": {"command": "ls"}}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text := "<tool_call>\n" + tt.json + "\n</tool_call>"
			calls := ParseToolCalls(text)
			if len(calls) != 1 {
				t.Fatalf("expected 1 tool call, got %d", len(calls))
			}
			if calls[0].Name != "exec" {
				t.Errorf("expected name 'exec', got %q", calls[0].Name)
			}
			cmd, ok := calls[0].Arguments["command"].(string)
			if !ok || cmd != "ls" {
				t.Errorf("expected command 'ls', got %v", calls[0].Arguments["command"])
			}
		})
	}
}

func TestParseToolCalls_EmptyArguments(t *testing.T) {
	text := `<tool_call>
{"name": "list_dir"}
</tool_call>`

	calls := ParseToolCalls(text)
	if len(calls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(calls))
	}
	if calls[0].Arguments == nil {
		t.Error("expected non-nil arguments map")
	}
}

func TestHasToolCalls(t *testing.T) {
	tests := []struct {
		text     string
		expected bool
	}{
		{"no tool calls here", false},
		{"<tool_call>{}</tool_call>", true},
		{"<tool_call> only open tag", false},
		{"only close </tool_call>", false},
		{"text <tool_call>{\"name\":\"x\"}</tool_call> more text", true},
	}

	for _, tt := range tests {
		if HasToolCalls(tt.text) != tt.expected {
			t.Errorf("HasToolCalls(%q) = %v, want %v", tt.text[:min(40, len(tt.text))], !tt.expected, tt.expected)
		}
	}
}

func TestStripToolCallBlocks(t *testing.T) {
	text := `Here is my analysis.

<tool_call>
{"name": "exec", "arguments": {"command": "ls"}}
</tool_call>

And here is more text.`

	result := StripToolCallBlocks(text)
	if strings.Contains(result, "<tool_call>") {
		t.Error("result should not contain <tool_call> tags")
	}
	if !strings.Contains(result, "Here is my analysis") {
		t.Error("result should contain surrounding text")
	}
	if !strings.Contains(result, "And here is more text") {
		t.Error("result should contain trailing text")
	}
}

// --- Orchestrator prompt tests ---

func TestBuildOrchestratorToolPrompt_Empty(t *testing.T) {
	result := BuildOrchestratorToolPrompt(nil)
	if result != "" {
		t.Errorf("expected empty string for nil tools, got %q", result)
	}
}

func TestBuildOrchestratorToolPrompt_ContainsToolNames(t *testing.T) {
	tools := []OrchestratorToolDescription{
		{Name: "exec", Description: "Execute a command", Parameters: map[string]any{"type": "object"}},
		{Name: "read_file", Description: "Read a file", Parameters: map[string]any{"type": "object"}},
	}

	result := BuildOrchestratorToolPrompt(tools)

	if !strings.Contains(result, "`exec`") {
		t.Error("prompt should contain exec tool name")
	}
	if !strings.Contains(result, "`read_file`") {
		t.Error("prompt should contain read_file tool name")
	}
	if !strings.Contains(result, "<tool_call>") {
		t.Error("prompt should contain format example with <tool_call> tag")
	}
	if !strings.Contains(result, "## Available Tools") {
		t.Error("prompt should contain 'Available Tools' header")
	}
}

func TestBuildOrchestratorToolPrompt_SortedOrder(t *testing.T) {
	tools := []OrchestratorToolDescription{
		{Name: "z_tool", Description: "Z tool"},
		{Name: "a_tool", Description: "A tool"},
		{Name: "m_tool", Description: "M tool"},
	}

	result := BuildOrchestratorToolPrompt(tools)

	aIdx := strings.Index(result, "`a_tool`")
	mIdx := strings.Index(result, "`m_tool`")
	zIdx := strings.Index(result, "`z_tool`")

	if aIdx > mIdx || mIdx > zIdx {
		t.Errorf("tools should be sorted alphabetically: a=%d, m=%d, z=%d", aIdx, mIdx, zIdx)
	}
}

func TestToolDefsToDescriptions(t *testing.T) {
	defs := []providers.ToolDefinition{
		{
			Type: "function",
			Function: providers.ToolFunctionDefinition{
				Name:        "exec",
				Description: "Execute a command",
				Parameters:  map[string]any{"type": "object"},
			},
		},
	}

	descs := ToolDefsToDescriptions(defs)
	if len(descs) != 1 {
		t.Fatalf("expected 1 description, got %d", len(descs))
	}
	if descs[0].Name != "exec" {
		t.Errorf("expected name 'exec', got %q", descs[0].Name)
	}
}

func TestInjectToolPrompt_WithSystemMessage(t *testing.T) {
	messages := []providers.Message{
		{Role: "system", Content: "You are a helpful assistant."},
		{Role: "user", Content: "Hello"},
	}

	result := InjectToolPrompt(messages, "\n\n## Tools\ntest tools")

	if len(result) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result))
	}
	if !strings.Contains(result[0].Content, "You are a helpful assistant.") {
		t.Error("system message should preserve original content")
	}
	if !strings.Contains(result[0].Content, "## Tools") {
		t.Error("system message should contain injected tool prompt")
	}
}

func TestInjectToolPrompt_WithoutSystemMessage(t *testing.T) {
	messages := []providers.Message{
		{Role: "user", Content: "Hello"},
	}

	result := InjectToolPrompt(messages, "\n\n## Tools\ntest tools")

	if len(result) != 2 {
		t.Fatalf("expected 2 messages (new system + user), got %d", len(result))
	}
	if result[0].Role != "system" {
		t.Error("first message should be system")
	}
}

func TestInjectToolPrompt_EmptyPrompt(t *testing.T) {
	messages := []providers.Message{
		{Role: "user", Content: "Hello"},
	}

	result := InjectToolPrompt(messages, "")
	if len(result) != 1 {
		t.Fatalf("expected 1 message unchanged, got %d", len(result))
	}
}

func TestInjectToolPrompt_DoesNotMutateOriginal(t *testing.T) {
	messages := []providers.Message{
		{Role: "system", Content: "Original"},
		{Role: "user", Content: "Hello"},
	}

	InjectToolPrompt(messages, " INJECTED")

	if strings.Contains(messages[0].Content, "INJECTED") {
		t.Error("original messages should not be mutated")
	}
}

// Executor tests removed

// End of xmltools tests

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
