package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dan-solli/teaforge/internal/memory"
	"github.com/dan-solli/teaforge/internal/treesitter"
)

func newTestProjectMemory(t *testing.T) *memory.ProjectMemory {
	t.Helper()
	pm, err := memory.NewProjectMemory(filepath.Join(t.TempDir(), "memory.json"))
	if err != nil {
		t.Fatalf("NewProjectMemory: %v", err)
	}
	return pm
}

func TestRecallNotesTool_Execute(t *testing.T) {
	t.Parallel()

	pm := newTestProjectMemory(t)
	_, _ = pm.AddNote("architecture", "use source-driven pipeline")
	_, _ = pm.AddNote("decision", "switch to AGENTS.md")

	tool := &recallNotesTool{project: pm}
	res := tool.Execute(context.Background(), map[string]any{"query": "pipeline"})
	if res.IsErr {
		t.Fatalf("unexpected error: %s", res.Error)
	}
	if !strings.Contains(res.Output, "source-driven pipeline") {
		t.Fatalf("expected matching note in output: %q", res.Output)
	}

	filtered := tool.Execute(context.Background(), map[string]any{"query": "switch", "category": "decision"})
	if filtered.IsErr {
		t.Fatalf("unexpected error: %s", filtered.Error)
	}
	if !strings.Contains(filtered.Output, "switch to AGENTS.md") {
		t.Fatalf("expected category filtered note in output: %q", filtered.Output)
	}
}

func TestRecallNotesTool_NoMatches(t *testing.T) {
	t.Parallel()

	pm := newTestProjectMemory(t)
	_, _ = pm.AddNote("architecture", "use source-driven pipeline")
	tool := &recallNotesTool{project: pm}
	res := tool.Execute(context.Background(), map[string]any{"query": "nonexistent"})
	if res.IsErr {
		t.Fatalf("unexpected error: %s", res.Error)
	}
	if !strings.Contains(res.Output, "No notes found") {
		t.Fatalf("unexpected output: %q", res.Output)
	}
}

func TestListNoteCategoriesTool_Execute(t *testing.T) {
	t.Parallel()

	pm := newTestProjectMemory(t)
	_, _ = pm.AddNote("architecture", "a")
	_, _ = pm.AddNote("architecture", "b")
	_, _ = pm.AddNote("decision", "c")

	tool := &listNoteCategoriesTool{project: pm}
	res := tool.Execute(context.Background(), nil)
	if res.IsErr {
		t.Fatalf("unexpected error: %s", res.Error)
	}
	if !strings.Contains(res.Output, "architecture (2)") {
		t.Fatalf("missing architecture count: %q", res.Output)
	}
	if !strings.Contains(res.Output, "decision (1)") {
		t.Fatalf("missing decision count: %q", res.Output)
	}
}

func TestListNoteCategoriesTool_Empty(t *testing.T) {
	t.Parallel()

	pm := newTestProjectMemory(t)
	tool := &listNoteCategoriesTool{project: pm}
	res := tool.Execute(context.Background(), map[string]any{})
	if res.IsErr {
		t.Fatalf("unexpected error: %s", res.Error)
	}
	if !strings.Contains(res.Output, "No note categories") {
		t.Fatalf("unexpected output: %q", res.Output)
	}
}

func TestSaveNoteTool_Execute(t *testing.T) {
	t.Parallel()

	pm := newTestProjectMemory(t)
	tool := &saveNoteTool{project: pm}

	res := tool.Execute(context.Background(), map[string]any{"category": "decision"})
	if !res.IsErr || !strings.Contains(res.Error, "content") {
		t.Fatalf("expected missing content error, got: %+v", res)
	}

	res = tool.Execute(context.Background(), map[string]any{"category": "decision", "content": "ship it"})
	if res.IsErr {
		t.Fatalf("unexpected error: %s", res.Error)
	}
	if !strings.Contains(res.Output, "Note saved with ID") {
		t.Fatalf("unexpected output: %q", res.Output)
	}
}

func TestSearchCodeTool_Execute(t *testing.T) {
	t.Parallel()

	cm := treesitter.NewCodeMemory()
	goFile := filepath.Join(t.TempDir(), "alpha.go")
	if err := os.WriteFile(goFile, []byte("package main\nfunc AlphaTool() {}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := cm.IndexFile(context.Background(), goFile); err != nil {
		t.Fatalf("IndexFile: %v", err)
	}

	tool := &searchCodeTool{code: cm}
	res := tool.Execute(context.Background(), map[string]any{})
	if !res.IsErr || !strings.Contains(res.Error, "query") {
		t.Fatalf("expected query error, got %+v", res)
	}

	res = tool.Execute(context.Background(), map[string]any{"query": "missing"})
	if res.IsErr {
		t.Fatalf("unexpected error: %s", res.Error)
	}
	if !strings.Contains(res.Output, "No symbols found") {
		t.Fatalf("unexpected output: %q", res.Output)
	}

	res = tool.Execute(context.Background(), map[string]any{"query": "AlphaTool"})
	if res.IsErr {
		t.Fatalf("unexpected error: %s", res.Error)
	}
	if !strings.Contains(res.Output, "AlphaTool") {
		t.Fatalf("expected symbol output, got %q", res.Output)
	}
}

func TestIndexDirectoryTool_Execute(t *testing.T) {
	t.Parallel()

	cm := treesitter.NewCodeMemory()
	tool := &indexDirectoryTool{code: cm}

	res := tool.Execute(context.Background(), map[string]any{})
	if !res.IsErr || !strings.Contains(res.Error, "path") {
		t.Fatalf("expected missing path error, got %+v", res)
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc Main(){}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	res = tool.Execute(context.Background(), map[string]any{"path": dir})
	if res.IsErr {
		t.Fatalf("unexpected error: %s", res.Error)
	}
	if !strings.Contains(res.Output, "Indexed") {
		t.Fatalf("unexpected output: %q", res.Output)
	}
}
