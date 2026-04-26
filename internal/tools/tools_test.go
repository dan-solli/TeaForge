package tools_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dan-solli/teaforge/internal/tools"
)

func TestReadFileTool(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hello.txt")
	if err := os.WriteFile(path, []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}

	tool := tools.ReadFileTool{}
	res := tool.Execute(context.Background(), map[string]any{"path": path})
	if res.IsErr {
		t.Fatalf("unexpected error: %s", res.Error)
	}
	if res.Output != "hello world" {
		t.Errorf("unexpected output: %q", res.Output)
	}
}

func TestReadFileTool_MissingParam(t *testing.T) {
	tool := tools.ReadFileTool{}
	res := tool.Execute(context.Background(), map[string]any{})
	if !res.IsErr {
		t.Error("expected error for missing path")
	}
}

func TestReadFileTool_NotFound(t *testing.T) {
	tool := tools.ReadFileTool{}
	res := tool.Execute(context.Background(), map[string]any{"path": "/nonexistent/file.txt"})
	if !res.IsErr {
		t.Error("expected error for missing file")
	}
}

func TestWriteFileTool(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "out.txt")

	tool := tools.WriteFileTool{}
	res := tool.Execute(context.Background(), map[string]any{
		"path":    path,
		"content": "written content",
	})
	if res.IsErr {
		t.Fatalf("unexpected error: %s", res.Error)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "written content" {
		t.Errorf("unexpected file content: %q", string(data))
	}
}

func TestWriteFileTool_MissingParams(t *testing.T) {
	tool := tools.WriteFileTool{}
	res := tool.Execute(context.Background(), map[string]any{})
	if !res.IsErr {
		t.Error("expected error for missing parameters")
	}
}

func TestEditFileTool(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "edit.txt")
	if err := os.WriteFile(path, []byte("foo bar baz"), 0o644); err != nil {
		t.Fatal(err)
	}

	tool := tools.EditFileTool{}
	res := tool.Execute(context.Background(), map[string]any{
		"path":    path,
		"old_str": "bar",
		"new_str": "qux",
	})
	if res.IsErr {
		t.Fatalf("unexpected error: %s", res.Error)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "foo qux baz" {
		t.Errorf("unexpected result: %q", string(data))
	}
}

func TestEditFileTool_NotFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "edit.txt")
	os.WriteFile(path, []byte("hello"), 0o644) //nolint:errcheck

	tool := tools.EditFileTool{}
	res := tool.Execute(context.Background(), map[string]any{
		"path":    path,
		"old_str": "missing",
		"new_str": "replacement",
	})
	if !res.IsErr {
		t.Error("expected error when old_str not found")
	}
}

func TestEditFileTool_Ambiguous(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "edit.txt")
	os.WriteFile(path, []byte("foo foo"), 0o644) //nolint:errcheck

	tool := tools.EditFileTool{}
	res := tool.Execute(context.Background(), map[string]any{
		"path":    path,
		"old_str": "foo",
		"new_str": "bar",
	})
	if !res.IsErr {
		t.Error("expected error when old_str appears multiple times")
	}
}

func TestListDirectoryTool(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0o644) //nolint:errcheck
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b"), 0o644) //nolint:errcheck
	os.Mkdir(filepath.Join(dir, "subdir"), 0o755)                  //nolint:errcheck

	tool := tools.ListDirectoryTool{}
	res := tool.Execute(context.Background(), map[string]any{"path": dir})
	if res.IsErr {
		t.Fatalf("unexpected error: %s", res.Error)
	}
	if !strings.Contains(res.Output, "a.txt") {
		t.Errorf("expected a.txt in output: %q", res.Output)
	}
	if !strings.Contains(res.Output, "subdir/") {
		t.Errorf("expected subdir/ in output: %q", res.Output)
	}
}

func TestRunCommandTool(t *testing.T) {
	tool := &tools.RunCommandTool{}
	res := tool.Execute(context.Background(), map[string]any{
		"command": "echo hello",
	})
	if res.IsErr {
		t.Fatalf("unexpected error: %s", res.Error)
	}
	if !strings.Contains(res.Output, "hello") {
		t.Errorf("expected 'hello' in output: %q", res.Output)
	}
}

func TestRunCommandTool_Failure(t *testing.T) {
	tool := &tools.RunCommandTool{}
	res := tool.Execute(context.Background(), map[string]any{
		"command": "exit 1",
	})
	if !res.IsErr {
		t.Error("expected error for failing command")
	}
}

func TestRunCommandTool_MissingParam(t *testing.T) {
	tool := &tools.RunCommandTool{}
	res := tool.Execute(context.Background(), map[string]any{})
	if !res.IsErr {
		t.Error("expected error for missing command parameter")
	}
}

func TestRegistry(t *testing.T) {
	r := tools.NewRegistry("/tmp")
	all := r.All()
	if len(all) == 0 {
		t.Fatal("expected at least one tool in registry")
	}
	for _, tool := range all {
		if tool.Name() == "" {
			t.Error("tool has empty name")
		}
		if tool.Description() == "" {
			t.Errorf("tool %q has empty description", tool.Name())
		}
		schema := tool.InputSchema()
		if schema == nil {
			t.Errorf("tool %q has nil schema", tool.Name())
		}
	}
	// Check specific tools exist
	for _, name := range []string{"read_file", "write_file", "edit_file", "list_directory", "run_command"} {
		if _, ok := r.Get(name); !ok {
			t.Errorf("expected tool %q in registry", name)
		}
	}
}
