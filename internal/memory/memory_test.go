package memory_test

import (
	"path/filepath"
	"testing"

	"github.com/dan-solli/teaforge/internal/memory"
)

// -------------------------------------------------------------------
// SessionMemory tests
// -------------------------------------------------------------------

func TestSessionMemory_AddAndRetrieveTurns(t *testing.T) {
	s := memory.NewSessionMemory()

	s.AddTurn("user", "hello")
	s.AddTurn("assistant", "hi there")

	turns := s.Turns()
	if len(turns) != 2 {
		t.Fatalf("expected 2 turns, got %d", len(turns))
	}
	if turns[0].Role != "user" || turns[0].Content != "hello" {
		t.Errorf("unexpected first turn: %+v", turns[0])
	}
	if turns[1].Role != "assistant" || turns[1].Content != "hi there" {
		t.Errorf("unexpected second turn: %+v", turns[1])
	}
}

func TestSessionMemory_Context(t *testing.T) {
	s := memory.NewSessionMemory()

	s.SetContext("project", "teaforge")
	s.SetContext("language", "go")

	v, ok := s.GetContext("project")
	if !ok || v != "teaforge" {
		t.Errorf("expected project=teaforge, got %q ok=%v", v, ok)
	}

	cm := s.ContextMap()
	if len(cm) != 2 {
		t.Errorf("expected 2 context entries, got %d", len(cm))
	}
}

func TestSessionMemory_Clear(t *testing.T) {
	s := memory.NewSessionMemory()
	s.AddTurn("user", "test")
	s.SetContext("k", "v")

	s.Clear()

	if len(s.Turns()) != 0 {
		t.Error("expected zero turns after Clear()")
	}
	if len(s.ContextMap()) != 0 {
		t.Error("expected empty context after Clear()")
	}
}

// -------------------------------------------------------------------
// ProjectMemory tests
// -------------------------------------------------------------------

func TestProjectMemory_AddAndRetrieveNotes(t *testing.T) {
	dir := t.TempDir()
	pm, err := memory.NewProjectMemory(filepath.Join(dir, "memory.json"))
	if err != nil {
		t.Fatal(err)
	}

	note, err := pm.AddNote("decision", "use Bubble Tea for TUI")
	if err != nil {
		t.Fatal(err)
	}
	if note.ID == "" {
		t.Error("note ID should not be empty")
	}
	if note.Category != "decision" {
		t.Errorf("expected category=decision, got %q", note.Category)
	}

	notes := pm.Notes()
	if len(notes) != 1 {
		t.Fatalf("expected 1 note, got %d", len(notes))
	}
}

func TestProjectMemory_Persistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "memory.json")

	pm1, err := memory.NewProjectMemory(path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pm1.AddNote("arch", "hexagonal architecture")
	if err != nil {
		t.Fatal(err)
	}

	// Reload from disk
	pm2, err := memory.NewProjectMemory(path)
	if err != nil {
		t.Fatal(err)
	}
	notes := pm2.Notes()
	if len(notes) != 1 {
		t.Fatalf("expected 1 note after reload, got %d", len(notes))
	}
	if notes[0].Content != "hexagonal architecture" {
		t.Errorf("unexpected content: %q", notes[0].Content)
	}
}

func TestProjectMemory_UpdateNote(t *testing.T) {
	dir := t.TempDir()
	pm, _ := memory.NewProjectMemory(filepath.Join(dir, "memory.json"))

	note, _ := pm.AddNote("todo", "write tests")
	if err := pm.UpdateNote(note.ID, "write more tests"); err != nil {
		t.Fatal(err)
	}

	notes := pm.Notes()
	if notes[0].Content != "write more tests" {
		t.Errorf("update failed: got %q", notes[0].Content)
	}
}

func TestProjectMemory_DeleteNote(t *testing.T) {
	dir := t.TempDir()
	pm, _ := memory.NewProjectMemory(filepath.Join(dir, "memory.json"))

	note, _ := pm.AddNote("temp", "temporary")
	if err := pm.DeleteNote(note.ID); err != nil {
		t.Fatal(err)
	}
	if len(pm.Notes()) != 0 {
		t.Error("expected zero notes after delete")
	}
}

func TestProjectMemory_NotesByCategory(t *testing.T) {
	dir := t.TempDir()
	pm, _ := memory.NewProjectMemory(filepath.Join(dir, "memory.json"))

	pm.AddNote("decision", "use tree-sitter") //nolint:errcheck
	pm.AddNote("todo", "add Python parser")   //nolint:errcheck
	pm.AddNote("decision", "use Ollama")       //nolint:errcheck

	decisions := pm.NotesByCategory("decision")
	if len(decisions) != 2 {
		t.Errorf("expected 2 decisions, got %d", len(decisions))
	}
	todos := pm.NotesByCategory("todo")
	if len(todos) != 1 {
		t.Errorf("expected 1 todo, got %d", len(todos))
	}
}

func TestProjectMemory_MissingFile(t *testing.T) {
	dir := t.TempDir()
	// File does not exist yet – should succeed (empty memory)
	pm, err := memory.NewProjectMemory(filepath.Join(dir, "new.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(pm.Notes()) != 0 {
		t.Error("expected empty notes for new file")
	}
}

func TestProjectMemory_UpdateNonExistent(t *testing.T) {
	dir := t.TempDir()
	pm, _ := memory.NewProjectMemory(filepath.Join(dir, "memory.json"))
	err := pm.UpdateNote("nonexistent", "content")
	if err == nil {
		t.Error("expected error updating non-existent note")
	}
}

func TestProjectMemory_DeleteNonExistent(t *testing.T) {
	dir := t.TempDir()
	pm, _ := memory.NewProjectMemory(filepath.Join(dir, "memory.json"))
	err := pm.DeleteNote("nonexistent")
	if err == nil {
		t.Error("expected error deleting non-existent note")
	}
}
