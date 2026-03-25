package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type DiffTool struct {
	workspace string
	restrict  bool
}

func NewDiffTool(workspace string, restrict bool) *DiffTool {
	return &DiffTool{workspace: workspace, restrict: restrict}
}

func (t *DiffTool) Name() string {
	return "diff_file"
}

func (t *DiffTool) Description() string {
	return "Show a unified diff between two files, or between old_text and new_text strings."
}

func (t *DiffTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"file_a": map[string]any{
				"type":        "string",
				"description": "Path to the first file (mutually exclusive with old_text)",
			},
			"file_b": map[string]any{
				"type":        "string",
				"description": "Path to the second file (mutually exclusive with new_text)",
			},
			"old_text": map[string]any{
				"type":        "string",
				"description": "Original text to diff (mutually exclusive with file_a)",
			},
			"new_text": map[string]any{
				"type":        "string",
				"description": "New text to diff against (mutually exclusive with file_b)",
			},
			"context_lines": map[string]any{
				"type":        "integer",
				"description": "Number of context lines around each change (default: 3)",
				"default":     3,
			},
		},
	}
}

func (t *DiffTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	contextLines := 3
	if cl, ok := args["context_lines"].(float64); ok && int(cl) >= 0 {
		contextLines = int(cl)
	}

	var aLines, bLines []string
	var labelA, labelB string

	fileA, hasFileA := args["file_a"].(string)
	fileB, hasFileB := args["file_b"].(string)
	oldText, hasOldText := args["old_text"].(string)
	newText, hasNewText := args["new_text"].(string)

	switch {
	case hasFileA && hasFileB:
		a, err := t.readFile(fileA)
		if err != nil {
			return ErrorResult(fmt.Sprintf("cannot read file_a: %v", err))
		}
		b, err := t.readFile(fileB)
		if err != nil {
			return ErrorResult(fmt.Sprintf("cannot read file_b: %v", err))
		}
		aLines = splitLines(a)
		bLines = splitLines(b)
		labelA, labelB = fileA, fileB

	case hasOldText && hasNewText:
		aLines = splitLines(oldText)
		bLines = splitLines(newText)
		labelA, labelB = "old", "new"

	default:
		return ErrorResult("provide either (file_a + file_b) or (old_text + new_text)")
	}

	diff := unifiedDiff(aLines, bLines, labelA, labelB, contextLines)
	if diff == "" {
		return NewToolResult("No differences found.")
	}
	return NewToolResult(diff)
}

func (t *DiffTool) readFile(path string) (string, error) {
	absPath := path
	if !filepath.IsAbs(path) && t.workspace != "" {
		absPath = filepath.Join(t.workspace, path)
	}
	if t.restrict && t.workspace != "" {
		rel, err := filepath.Rel(t.workspace, absPath)
		if err != nil || strings.HasPrefix(rel, "..") {
			return "", fmt.Errorf("access denied: path is outside the workspace")
		}
	}
	data, err := os.ReadFile(absPath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func splitLines(s string) []string {
	return strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
}

// unifiedDiff produces a simple unified diff without external dependencies.
func unifiedDiff(a, b []string, labelA, labelB string, ctx int) string {
	edits := diffLines(a, b)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("--- %s\n+++ %s\n", labelA, labelB))

	// Group edits into hunks
	type hunk struct {
		aStart, aLen, bStart, bLen int
		lines                      []diffEdit
	}

	var hunks []hunk
	var cur *hunk
	aLine, bLine := 1, 1

	for i, e := range edits {
		changed := e.kind != ' '
		if changed {
			if cur == nil {
				// Start new hunk with context before
				start := i - ctx
				if start < 0 {
					start = 0
				}
				cur = &hunk{aStart: aLine, bStart: bLine}
				// rewind context
				for j := start; j < i; j++ {
					if edits[j].kind != '+' {
						cur.aStart--
					}
					if edits[j].kind != '-' {
						cur.bStart--
					}
				}
				if cur.aStart < 1 {
					cur.aStart = 1
				}
				if cur.bStart < 1 {
					cur.bStart = 1
				}
				for j := start; j < i; j++ {
					cur.lines = append(cur.lines, diffEdit(edits[j]))
				}
			}
			cur.lines = append(cur.lines, diffEdit(e))
		} else if cur != nil {
			cur.lines = append(cur.lines, e)
			// Check if we've accumulated enough trailing context
			trailingCtx := 0
			for j := len(cur.lines) - 1; j >= 0; j-- {
				if cur.lines[j].kind == ' ' {
					trailingCtx++
				} else {
					break
				}
			}
			if trailingCtx >= ctx*2 {
				// Trim excess trailing context
				cur.lines = cur.lines[:len(cur.lines)-ctx]
				hunks = append(hunks, *cur)
				cur = nil
			}
		}

		if e.kind != '+' {
			aLine++
		}
		if e.kind != '-' {
			bLine++
		}
	}
	if cur != nil {
		// Trim excess trailing context
		trailingCtx := 0
		for j := len(cur.lines) - 1; j >= 0; j-- {
			if cur.lines[j].kind == ' ' {
				trailingCtx++
			} else {
				break
			}
		}
		if trailingCtx > ctx {
			cur.lines = cur.lines[:len(cur.lines)-(trailingCtx-ctx)]
		}
		hunks = append(hunks, *cur)
	}

	for _, h := range hunks {
		for _, l := range h.lines {
			if l.kind != '+' {
				h.aLen++
			}
			if l.kind != '-' {
				h.bLen++
			}
		}
		sb.WriteString(fmt.Sprintf("@@ -%d,%d +%d,%d @@\n", h.aStart, h.aLen, h.bStart, h.bLen))
		for _, l := range h.lines {
			sb.WriteRune(l.kind)
			sb.WriteString(l.line)
			sb.WriteByte('\n')
		}
	}

	return sb.String()
}

type diffEdit struct {
	kind rune
	line string
}

func diffLines(a, b []string) []diffEdit {
	// Simple O(ND) diff using dynamic programming
	n, m := len(a), len(b)
	type cell struct{ x, y int }
	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else if dp[i+1][j] > dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}

	var edits []diffEdit
	i, j := 0, 0
	for i < n || j < m {
		if i < n && j < m && a[i] == b[j] {
			edits = append(edits, diffEdit{' ', a[i]})
			i++
			j++
		} else if j < m && (i >= n || dp[i][j+1] >= dp[i+1][j]) {
			edits = append(edits, diffEdit{'+', b[j]})
			j++
		} else {
			edits = append(edits, diffEdit{'-', a[i]})
			i++
		}
	}
	return edits
}
