package tools

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

// --- List Files Tool ---

type ListFilesTool struct{}

func (t *ListFilesTool) Name() string { return "ls" }

func (t *ListFilesTool) Description() string {
	return "List files in a directory. Arguments: path (string)."
}

func (t *ListFilesTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{"type": "string", "description": "The directory path to list."},
		},
		"required": []string{"path"},
	}
}

func (t *ListFilesTool) Execute(ctx context.Context, input map[string]any) (any, error) {
	path, ok := input["path"].(string)
	if !ok {
		return nil, fmt.Errorf("argument 'path' is required and must be a string")
	}

	slog.Info("Listing files", "path", path)
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("failed to list directory: %w", err)
	}

	var names []string
	for _, e := range entries {
		suffix := ""
		if e.IsDir() {
			suffix = "/"
		}
		names = append(names, e.Name()+suffix)
	}
	return names, nil
}

// --- Read File Tool ---

type ReadFileTool struct{}

func (t *ReadFileTool) Name() string { return "read_file" }

func (t *ReadFileTool) Description() string {
	return "Read the contents of a file. Arguments: path (string)."
}

func (t *ReadFileTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{"type": "string", "description": "The file path to read."},
		},
		"required": []string{"path"},
	}
}

func (t *ReadFileTool) Execute(ctx context.Context, input map[string]any) (any, error) {
	path, ok := input["path"].(string)
	if !ok {
		return nil, fmt.Errorf("argument 'path' is required and must be a string")
	}

	slog.Info("Reading file", "path", path)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}
	return string(data), nil
}

// --- Write File Tool ---

type WriteFileTool struct{}

func (t *WriteFileTool) Name() string { return "write_file" }

func (t *WriteFileTool) Description() string {
	return "Write content to a file. Arguments: path (string), content (string)."
}

func (t *WriteFileTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path":    map[string]any{"type": "string", "description": "The file path to write to."},
			"content": map[string]any{"type": "string", "description": "The content to write."},
		},
		"required": []string{"path", "content"},
	}
}

func (t *WriteFileTool) Execute(ctx context.Context, input map[string]any) (any, error) {
	path, ok := input["path"].(string)
	if !ok {
		return nil, fmt.Errorf("argument 'path' is required and must be a string")
	}
	content, ok := input["content"].(string)
	if !ok {
		return nil, fmt.Errorf("argument 'content' is required and must be a string")
	}

	slog.Info("Writing file", "path", path, "size", len(content))

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, fmt.Errorf("failed to create directories: %w", err)
	}

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return nil, fmt.Errorf("failed to write file: %w", err)
	}
	return "success", nil
}
