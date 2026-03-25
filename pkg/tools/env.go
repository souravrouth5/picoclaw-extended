package tools

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/sipeed/picoclaw/pkg/constants"
)

type EnvTool struct{}

func NewEnvTool() *EnvTool {
	return &EnvTool{}
}

func (t *EnvTool) Name() string {
	return "env_get"
}

func (t *EnvTool) Description() string {
	return "Read one or more environment variables by name. Returns their current values."
}

func (t *EnvTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"names": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "List of environment variable names to read",
			},
		},
		"required": []string{"names"},
	}
}

func (t *EnvTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	if !constants.IsInternalChannel(ToolChannel(ctx)) {
		return ErrorResult("env_get is restricted to internal channels")
	}
	rawNames, ok := args["names"].([]any)
	if !ok || len(rawNames) == 0 {
		return ErrorResult("names must be a non-empty array")
	}

	var sb strings.Builder
	for _, raw := range rawNames {
		name, ok := raw.(string)
		if !ok || name == "" {
			continue
		}
		val, set := os.LookupEnv(name)
		if !set {
			sb.WriteString(fmt.Sprintf("%s=(not set)\n", name))
		} else if val == "" {
			sb.WriteString(fmt.Sprintf("%s=(empty)\n", name))
		} else {
			sb.WriteString(fmt.Sprintf("%s=%s\n", name, val))
		}
	}

	return NewToolResult(strings.TrimRight(sb.String(), "\n"))
}
