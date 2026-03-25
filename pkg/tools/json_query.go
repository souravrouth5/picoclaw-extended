package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tidwall/gjson"
)

type JSONQueryTool struct {
	workspace string
	restrict  bool
}

func NewJSONQueryTool(workspace string, restrict bool) *JSONQueryTool {
	return &JSONQueryTool{workspace: workspace, restrict: restrict}
}

func (t *JSONQueryTool) Name() string {
	return "json_query"
}

func (t *JSONQueryTool) Description() string {
	return "Query a JSON file or JSON string using a dot-notation path (e.g. 'user.name', 'items.0.id', 'users.#.name'). Avoids loading large JSON into context just to extract one field."
}

func (t *JSONQueryTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "GJSON path query (e.g. 'user.name', 'items.#.id', 'users.@reverse')",
			},
			"file": map[string]any{
				"type":        "string",
				"description": "Path to a JSON file to query (mutually exclusive with json)",
			},
			"json": map[string]any{
				"type":        "string",
				"description": "Raw JSON string to query (mutually exclusive with file)",
			},
		},
		"required": []string{"query"},
	}
}

func (t *JSONQueryTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	query, ok := args["query"].(string)
	if !ok || strings.TrimSpace(query) == "" {
		return ErrorResult("query is required")
	}

	var jsonStr string

	if file, ok := args["file"].(string); ok && file != "" {
		absPath := file
		if !filepath.IsAbs(file) && t.workspace != "" {
			absPath = filepath.Join(t.workspace, file)
		}
		if t.restrict && t.workspace != "" {
			rel, err := filepath.Rel(t.workspace, absPath)
			if err != nil || strings.HasPrefix(rel, "..") {
				return ErrorResult("access denied: path is outside the workspace")
			}
		}
		data, err := os.ReadFile(absPath)
		if err != nil {
			return ErrorResult(fmt.Sprintf("failed to read file: %v", err))
		}
		jsonStr = string(data)
	} else if raw, ok := args["json"].(string); ok && raw != "" {
		jsonStr = raw
	} else {
		return ErrorResult("provide either 'file' or 'json'")
	}

	if !gjson.Valid(jsonStr) {
		return ErrorResult("invalid JSON")
	}

	result := gjson.Get(jsonStr, query)
	if !result.Exists() {
		return NewToolResult(fmt.Sprintf("No value found at path: %s", query))
	}

	return NewToolResult(result.String())
}
