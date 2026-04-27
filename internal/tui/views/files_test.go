package views

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFilesView_NavigationToggleAndView(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	sub := filepath.Join(dir, "src")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	f := filepath.Join(sub, "main.go")
	if err := os.WriteFile(f, []byte("package main"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	v := NewFilesView(dir)
	v.SetSize(80, 20)
	if v.SelectedPath() == "" {
		t.Fatal("expected selected path")
	}
	v.MoveDown()
	v.MoveUp()
	_ = v.Toggle() // expand or collapse depending on cursor target

	out := v.View()
	if !strings.Contains(out, "Files:") {
		t.Fatalf("missing header in view: %q", out)
	}
}

func TestFilesView_ToggleFileReturnsPath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	f := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	v := NewFilesView(dir)
	v.SetSize(80, 20)
	_ = v.Toggle() // expand root directory
	if len(v.flat) == 0 {
		t.Fatal("expected flat entries")
	}
	for i, node := range v.flat {
		if !node.IsDir {
			v.cursor = i
			break
		}
	}
	path := v.Toggle()
	if path == "" {
		t.Fatal("expected file path on toggle")
	}
}
