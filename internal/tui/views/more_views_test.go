package views

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dan-solli/teaforge/internal/memory"
	"github.com/dan-solli/teaforge/internal/treesitter"
)

func TestMemoryView_ProjectAndCodeTabs(t *testing.T) {
	t.Parallel()
	s := memory.NewSessionMemory()
	pm, err := memory.NewProjectMemory(filepath.Join(t.TempDir(), "memory.json"))
	if err != nil {
		t.Fatalf("NewProjectMemory: %v", err)
	}
	_, _ = pm.AddNote("architecture", "note a")

	cm := treesitter.NewCodeMemory()
	dir := t.TempDir()
	goFile := filepath.Join(dir, "x.go")
	_ = os.WriteFile(goFile, []byte("package main\nfunc Alpha(){}"), 0o644)
	_ = cm.IndexFile(context.Background(), goFile)

	v := NewMemoryView(s, pm, cm)
	v.SetSize(100, 30)
	v.activeTab = MemoryTabProject
	proj := v.View()
	if !strings.Contains(proj, "Project Notes") {
		t.Fatalf("expected project view: %q", proj)
	}

	v.activeTab = MemoryTabCode
	v.SetCodeQuery("Alpha")
	code := v.View()
	if !strings.Contains(code, "Alpha") {
		t.Fatalf("expected code symbol in view: %q", code)
	}
}
