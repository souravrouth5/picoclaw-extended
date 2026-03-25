package tools

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/sipeed/picoclaw/pkg/constants"
)

type GitTool struct {
	workspace string
}

func NewGitTool(workspace string) *GitTool {
	return &GitTool{workspace: workspace}
}

func (t *GitTool) Name() string {
	return "git"
}

func (t *GitTool) Description() string {
	return "Run read-only git commands: status, diff, log, show, branch, stash list. Write operations (push, commit, reset) are not allowed."
}

func (t *GitTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"enum":        []string{"status", "diff", "log", "show", "branch", "stash list", "diff --staged", "diff HEAD"},
				"description": "The git command to run",
			},
			"args": map[string]any{
				"type":        "string",
				"description": "Optional extra arguments (e.g. '--oneline -10' for log, or a file path for diff)",
			},
		},
		"required": []string{"command"},
	}
}

var allowedGitCommands = map[string]bool{
	"status":      true,
	"diff":        true,
	"log":         true,
	"show":        true,
	"branch":      true,
	"stash list":  true,
	"diff --staged": true,
	"diff HEAD":   true,
}

func (t *GitTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	if !constants.IsInternalChannel(ToolChannel(ctx)) {
		return ErrorResult("git is restricted to internal channels")
	}
	command, ok := args["command"].(string)
	if !ok || command == "" {
		return ErrorResult("command is required")
	}

	if !allowedGitCommands[command] {
		return ErrorResult(fmt.Sprintf("command not allowed: %s", command))
	}

	extraArgs, _ := args["args"].(string)

	// Build full arg list
	parts := strings.Fields(command)
	if extraArgs != "" {
		parts = append(parts, strings.Fields(extraArgs)...)
	}

	cmd := exec.CommandContext(ctx, "git", parts...)
	if t.workspace != "" {
		cmd.Dir = t.workspace
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	out := stdout.String()
	if stderr.Len() > 0 {
		out += "\nSTDERR:\n" + stderr.String()
	}

	if out == "" {
		out = "(no output)"
	}

	// Truncate large diffs
	const maxLen = 8000
	if len(out) > maxLen {
		out = out[:maxLen] + fmt.Sprintf("\n... (truncated, %d more chars)", len(out)-maxLen)
	}

	if err != nil {
		return &ToolResult{ForLLM: out, ForUser: out, IsError: true}
	}
	return UserResult(out)
}
