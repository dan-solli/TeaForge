// Package tools provides built-in tools that the agent can call.
// Tools follow the Anthropic tool-use convention: each tool has a Name,
// Description, InputSchema and an Execute method.
package tools

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Result is the output returned by a tool.
type Result struct {
	Output string
	Error  string
	IsErr  bool
}

// Tool is the interface implemented by all built-in tools.
type Tool interface {
	Name() string
	Description() string
	// InputSchema returns a JSON-schema-compatible description of the parameters.
	InputSchema() map[string]any
	// Execute runs the tool with the given parameters.
	Execute(ctx context.Context, params map[string]any) Result
}

// -------------------------------------------------------------------
// ReadFileTool
// -------------------------------------------------------------------

// ReadFileTool reads the contents of a file from the filesystem.
type ReadFileTool struct{}

func (t ReadFileTool) Name() string { return "read_file" }
func (t ReadFileTool) Description() string {
	return "Read the contents of a file. Returns the file content as a string."
}
func (t ReadFileTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Absolute or relative path to the file to read.",
			},
		},
		"required": []string{"path"},
	}
}
func (t ReadFileTool) Execute(_ context.Context, params map[string]any) Result {
	path, ok := params["path"].(string)
	if !ok || path == "" {
		return Result{IsErr: true, Error: "parameter 'path' is required"}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Result{IsErr: true, Error: fmt.Sprintf("reading file: %v", err)}
	}
	return Result{Output: string(data)}
}

// -------------------------------------------------------------------
// WriteFileTool
// -------------------------------------------------------------------

// WriteFileTool writes or creates a file with the given content.
type WriteFileTool struct{}

func (t WriteFileTool) Name() string { return "write_file" }
func (t WriteFileTool) Description() string {
	return "Write content to a file. Creates the file (and any parent directories) if it does not exist. Overwrites existing content."
}
func (t WriteFileTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Path to the file to write.",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "Content to write into the file.",
			},
		},
		"required": []string{"path", "content"},
	}
}
func (t WriteFileTool) Execute(_ context.Context, params map[string]any) Result {
	path, ok := params["path"].(string)
	if !ok || path == "" {
		return Result{IsErr: true, Error: "parameter 'path' is required"}
	}
	content, ok := params["content"].(string)
	if !ok {
		return Result{IsErr: true, Error: "parameter 'content' is required"}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return Result{IsErr: true, Error: fmt.Sprintf("creating directories: %v", err)}
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return Result{IsErr: true, Error: fmt.Sprintf("writing file: %v", err)}
	}
	return Result{Output: fmt.Sprintf("Successfully wrote %d bytes to %s", len(content), path)}
}

// -------------------------------------------------------------------
// EditFileTool
// -------------------------------------------------------------------

// EditFileTool performs a precise string-replacement edit on a file.
type EditFileTool struct{}

func (t EditFileTool) Name() string { return "edit_file" }
func (t EditFileTool) Description() string {
	return "Replace an exact string in a file with new content. The old_str must match exactly once in the file."
}
func (t EditFileTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Path to the file to edit.",
			},
			"old_str": map[string]any{
				"type":        "string",
				"description": "The exact string to replace (must appear exactly once).",
			},
			"new_str": map[string]any{
				"type":        "string",
				"description": "The replacement string.",
			},
		},
		"required": []string{"path", "old_str", "new_str"},
	}
}
func (t EditFileTool) Execute(_ context.Context, params map[string]any) Result {
	path, _ := params["path"].(string)
	oldStr, _ := params["old_str"].(string)
	newStr, _ := params["new_str"].(string)
	if path == "" {
		return Result{IsErr: true, Error: "parameter 'path' is required"}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Result{IsErr: true, Error: fmt.Sprintf("reading file: %v", err)}
	}
	content := string(data)
	count := strings.Count(content, oldStr)
	if count == 0 {
		return Result{IsErr: true, Error: "old_str not found in file"}
	}
	if count > 1 {
		return Result{IsErr: true, Error: fmt.Sprintf("old_str found %d times; must match exactly once", count)}
	}
	newContent := strings.Replace(content, oldStr, newStr, 1)
	if err := os.WriteFile(path, []byte(newContent), 0o644); err != nil {
		return Result{IsErr: true, Error: fmt.Sprintf("writing file: %v", err)}
	}
	return Result{Output: fmt.Sprintf("Successfully edited %s", path)}
}

// -------------------------------------------------------------------
// ListDirectoryTool
// -------------------------------------------------------------------

// ListDirectoryTool lists the files in a directory.
type ListDirectoryTool struct{}

func (t ListDirectoryTool) Name() string { return "list_directory" }
func (t ListDirectoryTool) Description() string {
	return "List files and directories at the given path."
}
func (t ListDirectoryTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Directory path to list.",
			},
		},
		"required": []string{"path"},
	}
}
func (t ListDirectoryTool) Execute(_ context.Context, params map[string]any) Result {
	path, _ := params["path"].(string)
	if path == "" {
		path = "."
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return Result{IsErr: true, Error: fmt.Sprintf("listing directory: %v", err)}
	}
	var lines []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			name += "/"
		}
		lines = append(lines, name)
	}
	return Result{Output: strings.Join(lines, "\n")}
}

// -------------------------------------------------------------------
// RunCommandTool
// -------------------------------------------------------------------

const maxOutputBytes = 32 * 1024 // 32 KB cap on command output

// RunCommandTool executes a shell command and returns its output.
type RunCommandTool struct {
	// WorkDir is the working directory for commands; defaults to current dir.
	WorkDir string
	// Timeout limits execution time (default 60s).
	Timeout time.Duration
}

func (t *RunCommandTool) Name() string { return "run_command" }
func (t *RunCommandTool) Description() string {
	return "Execute a shell command and return its stdout/stderr output. " +
		"Use with caution: prefer safe, non-destructive commands."
}
func (t *RunCommandTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"description": "The shell command to execute.",
			},
			"working_dir": map[string]any{
				"type":        "string",
				"description": "Optional working directory for the command.",
			},
		},
		"required": []string{"command"},
	}
}
func (t *RunCommandTool) Execute(ctx context.Context, params map[string]any) Result {
	command, ok := params["command"].(string)
	if !ok || command == "" {
		return Result{IsErr: true, Error: "parameter 'command' is required"}
	}
	timeout := t.Timeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	workDir := t.WorkDir
	if wd, ok := params["working_dir"].(string); ok && wd != "" {
		workDir = wd
	}

	cmd := exec.CommandContext(ctx, "sh", "-c", command) //nolint:gosec
	cmd.Dir = workDir
	out, err := cmd.CombinedOutput()
	if len(out) > maxOutputBytes {
		out = append(out[:maxOutputBytes], []byte("\n...(output truncated)")...)
	}
	if err != nil {
		return Result{
			IsErr:  true,
			Error:  fmt.Sprintf("command failed: %v", err),
			Output: string(out),
		}
	}
	return Result{Output: string(out)}
}

// -------------------------------------------------------------------
// Registry
// -------------------------------------------------------------------

// Registry holds all available tools keyed by name.
type Registry struct {
	tools map[string]Tool
}

// NewRegistry creates a registry pre-populated with the default built-in tools.
func NewRegistry(workDir string) *Registry {
	r := &Registry{tools: make(map[string]Tool)}
	timeout := 60 * time.Second
	r.Register(ReadFileTool{})
	r.Register(WriteFileTool{})
	r.Register(EditFileTool{})
	r.Register(ListDirectoryTool{})
	r.Register(&RunCommandTool{WorkDir: workDir, Timeout: timeout})
	return r
}

// Register adds a tool to the registry.
func (r *Registry) Register(t Tool) {
	r.tools[t.Name()] = t
}

// Get returns a tool by name.
func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

// All returns all registered tools.
func (r *Registry) All() []Tool {
	out := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, t)
	}
	return out
}
