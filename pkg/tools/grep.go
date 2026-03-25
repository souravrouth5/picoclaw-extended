package tools

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/sipeed/picoclaw/pkg/constants"
)

type GrepTool struct {
	workspace string
	restrict  bool
}

func NewGrepTool(workspace string, restrict bool) *GrepTool {
	return &GrepTool{workspace: workspace, restrict: restrict}
}

func (t *GrepTool) Name() string {
	return "grep_file"
}

func (t *GrepTool) Description() string {
	return "Search for a text pattern in a file or recursively in a directory. Returns matching lines with line numbers."
}

func (t *GrepTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pattern": map[string]any{
				"type":        "string",
				"description": "Text or regex pattern to search for",
			},
			"path": map[string]any{
				"type":        "string",
				"description": "File or directory path to search in",
			},
			"recursive": map[string]any{
				"type":        "boolean",
				"description": "Search recursively in subdirectories (only applies when path is a directory)",
				"default":     false,
			},
			"case_sensitive": map[string]any{
				"type":        "boolean",
				"description": "Whether the search is case-sensitive",
				"default":     true,
			},
			"max_results": map[string]any{
				"type":        "integer",
				"description": "Maximum number of matching lines to return",
				"default":     50,
			},
		},
		"required": []string{"pattern", "path"},
	}
}

func (t *GrepTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	if !constants.IsInternalChannel(ToolChannel(ctx)) {
		return ErrorResult("grep_file is restricted to internal channels")
	}
	pattern, ok := args["pattern"].(string)
	if !ok || pattern == "" {
		return ErrorResult("pattern is required")
	}

	searchPath, ok := args["path"].(string)
	if !ok || searchPath == "" {
		return ErrorResult("path is required")
	}

	recursive, _ := args["recursive"].(bool)
	caseSensitive := true
	if cs, ok := args["case_sensitive"].(bool); ok {
		caseSensitive = cs
	}

	maxResults := 50
	if mr, ok := args["max_results"].(float64); ok && int(mr) > 0 {
		maxResults = int(mr)
	}

	// Resolve path
	absPath := searchPath
	if !filepath.IsAbs(searchPath) && t.workspace != "" {
		absPath = filepath.Join(t.workspace, searchPath)
	}

	if t.restrict && t.workspace != "" {
		rel, err := filepath.Rel(t.workspace, absPath)
		if err != nil || strings.HasPrefix(rel, "..") {
			return ErrorResult("access denied: path is outside the workspace")
		}
	}

	// Compile regex
	regexPattern := pattern
	if !caseSensitive {
		regexPattern = "(?i)" + pattern
	}
	re, err := regexp.Compile(regexPattern)
	if err != nil {
		return ErrorResult(fmt.Sprintf("invalid pattern: %v", err))
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return ErrorResult(fmt.Sprintf("path not found: %v", err))
	}

	var results []string
	count := 0

	if info.IsDir() {
		err = filepath.WalkDir(absPath, func(p string, d os.DirEntry, err error) error {
			if err != nil || count >= maxResults {
				return err
			}
			if d.IsDir() {
				if !recursive && p != absPath {
					return filepath.SkipDir
				}
				return nil
			}
			matches, err := grepFile(p, re, maxResults-count)
			if err != nil {
				return nil // skip unreadable files
			}
			rel, _ := filepath.Rel(absPath, p)
			for _, m := range matches {
				results = append(results, fmt.Sprintf("%s:%s", rel, m))
				count++
			}
			return nil
		})
		if err != nil {
			return ErrorResult(fmt.Sprintf("walk error: %v", err))
		}
	} else {
		matches, err := grepFile(absPath, re, maxResults)
		if err != nil {
			return ErrorResult(fmt.Sprintf("failed to read file: %v", err))
		}
		results = matches
		count = len(matches)
	}

	if len(results) == 0 {
		return NewToolResult(fmt.Sprintf("No matches found for pattern: %s", pattern))
	}

	out := strings.Join(results, "\n")
	if count >= maxResults {
		out += fmt.Sprintf("\n[Results truncated at %d matches]", maxResults)
	}
	return NewToolResult(out)
}

func grepFile(path string, re *regexp.Regexp, limit int) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var matches []string
	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		if re.MatchString(line) {
			matches = append(matches, fmt.Sprintf("%d: %s", lineNum, line))
			if len(matches) >= limit {
				break
			}
		}
	}
	return matches, scanner.Err()
}
