package agent

import (
	"context"
	"path/filepath"
	"testing"
)

func newTestAgent(t *testing.T) *Agent {
	t.Helper()
	dir := t.TempDir()
	a, err := New(Config{
		Model:      "m",
		OllamaURL:  "http://localhost:11434",
		WorkDir:    dir,
		MemoryFile: filepath.Join(dir, "memory.json"),
		NumCtx:     2048,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return a
}

func TestAgentAccessorsAndBuildToolDescriptors(t *testing.T) {
	t.Parallel()
	a := newTestAgent(t)
	if a.Project() == nil || a.Code() == nil || a.Tools() == nil {
		t.Fatal("accessors returned nil")
	}
	desc := a.buildToolDescriptors()
	if len(desc) == 0 {
		t.Fatal("expected tool descriptors")
	}
}

func TestAgentAppendAndIndexNoop(t *testing.T) {
	t.Parallel()
	a := newTestAgent(t)
	if err := a.AppendSessionLog("user", "x"); err != nil {
		t.Fatalf("AppendSessionLog: %v", err)
	}
	if err := a.IndexWorkDir(context.Background()); err != nil {
		t.Fatalf("IndexWorkDir: %v", err)
	}
}
