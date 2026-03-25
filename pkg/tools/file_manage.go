package tools

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/sipeed/picoclaw/pkg/constants"
)

type FileManageTool struct {
	workspace string
	restrict  bool
}

func NewFileManageTool(workspace string, restrict bool) *FileManageTool {
	return &FileManageTool{workspace: workspace, restrict: restrict}
}

func (t *FileManageTool) Name() string {
	return "file_manage"
}

func (t *FileManageTool) Description() string {
	return "Move, copy, or delete files and directories."
}

func (t *FileManageTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"move", "copy", "delete"},
				"description": "Action to perform: move, copy, or delete",
			},
			"path": map[string]any{
				"type":        "string",
				"description": "Source file or directory path",
			},
			"destination": map[string]any{
				"type":        "string",
				"description": "Destination path (required for move and copy)",
			},
		},
		"required": []string{"action", "path"},
	}
}

func (t *FileManageTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	if !constants.IsInternalChannel(ToolChannel(ctx)) {
		return ErrorResult("file_manage is restricted to internal channels")
	}
	action, ok := args["action"].(string)
	if !ok {
		return ErrorResult("action is required")
	}

	path, ok := args["path"].(string)
	if !ok || path == "" {
		return ErrorResult("path is required")
	}

	absSrc := t.resolvePath(path)
	if err := t.checkAccess(absSrc); err != nil {
		return ErrorResult(err.Error())
	}

	switch action {
	case "delete":
		if err := os.RemoveAll(absSrc); err != nil {
			return ErrorResult(fmt.Sprintf("delete failed: %v", err))
		}
		return SilentResult(fmt.Sprintf("Deleted: %s", path))

	case "move", "copy":
		dest, ok := args["destination"].(string)
		if !ok || dest == "" {
			return ErrorResult("destination is required for " + action)
		}
		absDst := t.resolvePath(dest)
		if err := t.checkAccess(absDst); err != nil {
			return ErrorResult(err.Error())
		}
		// Ensure parent dir exists
		if err := os.MkdirAll(filepath.Dir(absDst), 0o755); err != nil {
			return ErrorResult(fmt.Sprintf("failed to create destination directory: %v", err))
		}
		if action == "move" {
			if err := os.Rename(absSrc, absDst); err != nil {
				return ErrorResult(fmt.Sprintf("move failed: %v", err))
			}
			return SilentResult(fmt.Sprintf("Moved: %s → %s", path, dest))
		}
		if err := copyPath(absSrc, absDst); err != nil {
			return ErrorResult(fmt.Sprintf("copy failed: %v", err))
		}
		return SilentResult(fmt.Sprintf("Copied: %s → %s", path, dest))

	default:
		return ErrorResult(fmt.Sprintf("unknown action: %s", action))
	}
}

func (t *FileManageTool) resolvePath(path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	if t.workspace != "" {
		return filepath.Join(t.workspace, path)
	}
	return filepath.Clean(path)
}

func (t *FileManageTool) checkAccess(absPath string) error {
	if !t.restrict || t.workspace == "" {
		return nil
	}
	rel, err := filepath.Rel(t.workspace, absPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		return fmt.Errorf("access denied: path is outside the workspace")
	}
	return nil
}

func copyPath(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return copyDir(src, dst)
	}
	return copyFile(src, dst)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err = io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

func copyDir(src, dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := copyPath(filepath.Join(src, entry.Name()), filepath.Join(dst, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}
