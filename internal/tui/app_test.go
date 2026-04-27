package tui

import (
	"path/filepath"
	"testing"

	"github.com/dan-solli/teaforge/internal/agent"
)

func newTestApp(t *testing.T) App {
	t.Helper()
	dir := t.TempDir()
	cfg := agent.Config{
		Model:       "test-model",
		OllamaURL:   "http://localhost:11434",
		WorkDir:     dir,
		MemoryFile:  filepath.Join(dir, "memory.json"),
		SessionsDir: filepath.Join(dir, "sessions"),
		NumCtx:      1024,
	}
	ag, err := agent.New(cfg)
	if err != nil {
		t.Fatalf("agent.New: %v", err)
	}
	app := NewApp(cfg, ag)
	app.width = 120
	app.height = 40
	app.updateSizes()
	return app
}

func TestApp_AttachmentHelpersAndWorkDir(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	app.addPendingAttachment("a.go")
	app.addPendingAttachment("a.go")
	app.addPendingAttachment("b.go")
	if len(app.pendingAttachments) != 2 {
		t.Fatalf("attachments=%d want 2", len(app.pendingAttachments))
	}
	got := app.consumePendingAttachments()
	if len(got) != 2 || len(app.pendingAttachments) != 0 {
		t.Fatalf("consume failed: got=%v pending=%v", got, app.pendingAttachments)
	}
	if app.WorkDir() == "" {
		t.Fatal("workdir should not be empty")
	}
}

func TestApp_OpenSessionPickerNoSessions(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	app.cfg.SessionsDir = filepath.Join(t.TempDir(), "empty-sessions")
	if err := app.openSessionPicker(); err == nil {
		t.Fatal("expected error when no sessions exist")
	}
}
