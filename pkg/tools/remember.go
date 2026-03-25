package tools

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// MemoryBackend is the interface the remember tool needs from the agent's MemoryStore.
type MemoryBackend interface {
	ReadLongTerm() string
	WriteLongTerm(content string) error
	AppendToday(content string) error
}

type RememberTool struct {
	memory MemoryBackend
}

func NewRememberTool(memory MemoryBackend) *RememberTool {
	return &RememberTool{memory: memory}
}

func (t *RememberTool) Name() string {
	return "remember"
}

func (t *RememberTool) Description() string {
	return "Save or recall information from persistent memory. Use 'save' to store a fact for later, 'recall' to read all saved memories, or 'forget' to remove a specific entry."
}

func (t *RememberTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"save", "recall", "forget"},
				"description": "Action: 'save' stores a new memory, 'recall' reads all memories, 'forget' removes a specific entry by its exact text",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "The information to save (for 'save') or the exact text to remove (for 'forget')",
			},
		},
		"required": []string{"action"},
	}
}

func (t *RememberTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	action, ok := args["action"].(string)
	if !ok {
		return ErrorResult("action is required")
	}

	switch action {
	case "save":
		content, ok := args["content"].(string)
		if !ok || strings.TrimSpace(content) == "" {
			return ErrorResult("content is required for save")
		}
		entry := fmt.Sprintf("- [%s] %s\n", time.Now().Format("2006-01-02"), strings.TrimSpace(content))

		existing := t.memory.ReadLongTerm()
		if err := t.memory.WriteLongTerm(existing + entry); err != nil {
			return ErrorResult(fmt.Sprintf("failed to save memory: %v", err))
		}
		return SilentResult("Memory saved: " + strings.TrimSpace(content))

	case "recall":
		mem := t.memory.ReadLongTerm()
		if strings.TrimSpace(mem) == "" {
			return NewToolResult("No memories saved yet.")
		}
		return NewToolResult("Saved memories:\n\n" + mem)

	case "forget":
		content, ok := args["content"].(string)
		if !ok || strings.TrimSpace(content) == "" {
			return ErrorResult("content is required for forget")
		}
		existing := t.memory.ReadLongTerm()
		lines := strings.Split(existing, "\n")
		var kept []string
		removed := false
		for _, line := range lines {
			if strings.Contains(line, strings.TrimSpace(content)) && !removed {
				removed = true
				continue
			}
			kept = append(kept, line)
		}
		if !removed {
			return ErrorResult("memory entry not found")
		}
		if err := t.memory.WriteLongTerm(strings.Join(kept, "\n")); err != nil {
			return ErrorResult(fmt.Sprintf("failed to update memory: %v", err))
		}
		return SilentResult("Memory entry removed.")

	default:
		return ErrorResult(fmt.Sprintf("unknown action: %s", action))
	}
}
