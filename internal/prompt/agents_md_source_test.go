package prompt

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAgentInstructionsSourceCollect_ChildFirstAndCommentStripped(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	child := filepath.Join(root, "child")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatalf("mkdir child: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("parent\n<!-- hidden -->"), 0o644); err != nil {
		t.Fatalf("write parent AGENTS.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(child, "AGENTS.md"), []byte("child"), 0o644); err != nil {
		t.Fatalf("write child AGENTS.md: %v", err)
	}

	src := NewAgentInstructionsSource()
	items, err := src.Collect(context.Background(), &Request{WorkDir: child})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	got := items[0].Body
	if strings.Contains(got, "hidden") {
		t.Fatalf("expected HTML comments stripped, got: %q", got)
	}
	childPos := strings.Index(got, "## From AGENTS.md")
	parentPos := strings.Index(got, "## From ../AGENTS.md")
	if childPos == -1 || parentPos == -1 {
		t.Fatalf("expected child and parent headers in output, got: %q", got)
	}
	if childPos > parentPos {
		t.Fatalf("expected child-first ordering, got: %q", got)
	}
}

func TestAgentInstructionsSourceCollect_NoFiles(t *testing.T) {
	t.Parallel()

	src := NewAgentInstructionsSource()
	items, err := src.Collect(context.Background(), &Request{WorkDir: t.TempDir()})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected no items, got %d", len(items))
	}
}
