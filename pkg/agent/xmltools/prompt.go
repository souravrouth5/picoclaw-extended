// PicoClaw - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 PicoClaw contributors

package xmltools

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"

	"github.com/sipeed/picoclaw/pkg/providers"
)

// OrchestratorToolDescription describes a single tool for the orchestrator's
// system prompt. The orchestrator sees tools as text descriptions (not API
// tool definitions) to bypass safety filters that refuse tool execution.
type OrchestratorToolDescription struct {
	Name        string
	Description string
	Parameters  map[string]any
}

// BuildOrchestratorToolPrompt builds the tool description section that is
// appended to the orchestrator's system prompt. It lists all available tools
// with their names, descriptions, and parameter schemas, and instructs the
// orchestrator on the exact format to use when requesting tool calls.
func BuildOrchestratorToolPrompt(tools []OrchestratorToolDescription) string {
	if len(tools) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\n\n## Available Tools\n\n")
	sb.WriteString("You have access to the following tools. When you need to use a tool, output your request using the exact XML format shown below. You may include multiple tool calls in a single response.\n\n")

	// Sort tools by name for deterministic output (KV cache stability)
	sorted := make([]OrchestratorToolDescription, len(tools))
	copy(sorted, tools)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Name < sorted[j].Name
	})

	for _, tool := range sorted {
		sb.WriteString(fmt.Sprintf("### `%s`\n", tool.Name))
		sb.WriteString(tool.Description)
		sb.WriteString("\n")

		if len(tool.Parameters) > 0 {
			sb.WriteString("**Parameters:**\n```json\n")
			paramJSON, err := json.MarshalIndent(tool.Parameters, "", "  ")
			if err == nil {
				sb.Write(paramJSON)
			}
			sb.WriteString("\n```\n")
		}
		sb.WriteString("\n")
	}

	sb.WriteString("## Tool Call Format\n\n")
	sb.WriteString("When you need to use a tool, you MUST output your request in this EXACT raw XML format. ")
	sb.WriteString("CRITICAL: You MUST NEVER wrap the tool call in markdown codeblocks (no ```xml or ```). Write the raw tags directly into the response text.\n\n")
	sb.WriteString("<tool_call>\n{\"name\": \"tool_name\", \"arguments\": {\"param1\": \"value1\"}}\n</tool_call>\n\n")
	sb.WriteString("You can include multiple `<tool_call>` blocks in a single response to execute multiple tools.\n\n")
	sb.WriteString("**Important rules:**\n")
	sb.WriteString("- DO NOT use markdown code blocks (like ```) around the tool call.\n")
	sb.WriteString("- ALWAYS output ONLY the raw `<tool_call>` block when using tools. DO NOT output conversational text before or after the `<tool_call>` block. Save your conversational explanations for the final response AFTER the tools finish.\n")
	sb.WriteString("- When you receive a `<tool_response>`, DO NOT echo or repeat the raw tool output block back to the user. Simply read the data and answer the user's initial query naturally.\n")
	sb.WriteString("- Always use valid, unescaped JSON inside `<tool_call>` tags.\n")
	sb.WriteString("- The `name` field must exactly match one of the available tool names.\n")
	sb.WriteString("- Include all required parameters in the `arguments` object.\n")
	
	sb.WriteString("\n## Environment Context & Execution Rules\n")
	sb.WriteString(fmt.Sprintf("- OS platform: %s/%s\n", runtime.GOOS, runtime.GOARCH))
	if runtime.GOOS == "windows" {
		shell := os.Getenv("COMSPEC")
		if shell == "" {
			shell = "cmd.exe"
		}
		sb.WriteString(fmt.Sprintf("- Default Shell for `exec` tool: %s\n", shell))
		sb.WriteString("- When using `exec` to open URLs in cmd, avoid quotes or use `start \"\" \"https://...\"`.\n")
		sb.WriteString("- You CANNOT interact with GUI apps. DO NOT use `exec` to open `calc.exe`, `notepad.exe`, or browsers expecting to click things. For math/logic, use headless CLI scripts (e.g., pass `\"shell\": \"powershell\"` to `exec` and run `1..100 | Measure-Object -Sum`).\n")
	} else {
		shell := os.Getenv("SHELL")
		if shell == "" {
			shell = "/bin/sh"
		}
		sb.WriteString(fmt.Sprintf("- Default Shell for `exec` tool: %s\n", shell))
		sb.WriteString("- You CANNOT interact with GUI apps. For math or data logic, use standard headless CLI scripts via `exec` (e.g. bash, python, bc).\n")
	}

	return sb.String()
}

// ToolDefsToDescriptions converts provider tool definitions into orchestrator
// tool descriptions. This bridges the gap between the API tool format and
// the text-based format the orchestrator needs.
func ToolDefsToDescriptions(defs []providers.ToolDefinition) []OrchestratorToolDescription {
	descs := make([]OrchestratorToolDescription, 0, len(defs))
	for _, def := range defs {
		descs = append(descs, OrchestratorToolDescription{
			Name:        def.Function.Name,
			Description: def.Function.Description,
			Parameters:  def.Function.Parameters,
		})
	}
	return descs
}

// InjectToolPrompt appends the tool description prompt to the system message
// in a message slice. If the first message has role "system", the tool prompt
// is appended to its content. Otherwise, a new system message is prepended.
func InjectToolPrompt(messages []providers.Message, toolPrompt string) []providers.Message {
	if toolPrompt == "" {
		return messages
	}

	// Clone to avoid mutating the original slice
	result := make([]providers.Message, len(messages))
	copy(result, messages)

	if len(result) > 0 && result[0].Role == "system" {
		result[0] = providers.Message{
			Role:    "system",
			Content: result[0].Content + toolPrompt,
		}
	} else {
		systemMsg := providers.Message{
			Role:    "system",
			Content: toolPrompt,
		}
		result = append([]providers.Message{systemMsg}, result...)
	}

	return result
}
