package agent

import (
	"path/filepath"
	"testing"

	sesslog "github.com/dan-solli/teaforge/internal/session"
)

func TestResumeFromLog_ReconstructsSessionAndSummary(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfg := Config{
		Model:       "test-model",
		OllamaURL:   "http://localhost:11434",
		WorkDir:     dir,
		MemoryFile:  filepath.Join(dir, "memory.json"),
		SessionsDir: filepath.Join(dir, "sessions"),
	}
	a, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	log, err := sesslog.New(cfg.SessionsDir, cfg.Model, cfg.WorkDir)
	if err != nil {
		t.Fatalf("session.New: %v", err)
	}
	_ = log.Append(sesslog.RoleSystem, "ignored system")
	_ = log.Append(sesslog.RoleSummary, "resumed summary")
	_ = log.Append(sesslog.RoleUser, "hello")
	_ = log.Append(sesslog.RoleAssistant, "hi")
	_ = log.Append(sesslog.RoleTool, "tool output")
	_ = log.Append(sesslog.RolePromptSnapshot, "ignored snapshot")

	if err := a.ResumeFromLog(log.Path()); err != nil {
		t.Fatalf("ResumeFromLog: %v", err)
	}

	if a.resumeSummary != "resumed summary" {
		t.Fatalf("resumeSummary=%q want %q", a.resumeSummary, "resumed summary")
	}
	turns := a.Session().Turns()
	if len(turns) != 3 {
		t.Fatalf("turn count=%d want 3", len(turns))
	}
	if turns[0].Role != "user" || turns[0].Content != "hello" {
		t.Fatalf("unexpected turn0: %+v", turns[0])
	}
	if turns[1].Role != "assistant" || turns[1].Content != "hi" {
		t.Fatalf("unexpected turn1: %+v", turns[1])
	}
	if turns[2].Role != "tool" || turns[2].Content != "tool output" {
		t.Fatalf("unexpected turn2: %+v", turns[2])
	}
}
