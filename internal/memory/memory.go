// Package memory implements three layers of memory for the TeaForge agent:
//   - Session memory: the current conversation history and working context.
//   - Project memory: persistent notes, decisions and documentation written to disk.
//   - Code memory: a structural index of the source code built by tree-sitter.
package memory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// -------------------------------------------------------------------
// Session memory
// -------------------------------------------------------------------

// Turn represents a single exchange in the conversation.
type Turn struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

// SessionMemory holds the live conversation history and transient context.
type SessionMemory struct {
	mu      sync.RWMutex
	turns   []Turn
	context map[string]string
}

// NewSessionMemory creates an empty SessionMemory.
func NewSessionMemory() *SessionMemory {
	return &SessionMemory{
		context: make(map[string]string),
	}
}

// AddTurn appends a turn to the conversation history.
func (s *SessionMemory) AddTurn(role, content string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.turns = append(s.turns, Turn{
		Role:      role,
		Content:   content,
		Timestamp: time.Now(),
	})
}

// Turns returns a copy of the current conversation history.
func (s *SessionMemory) Turns() []Turn {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Turn, len(s.turns))
	copy(out, s.turns)
	return out
}

// SetContext stores an arbitrary key-value pair in the session context.
func (s *SessionMemory) SetContext(key, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.context[key] = value
}

// GetContext retrieves a key from the session context.
func (s *SessionMemory) GetContext(key string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.context[key]
	return v, ok
}

// ContextMap returns a snapshot of all session context entries.
func (s *SessionMemory) ContextMap() map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]string, len(s.context))
	for k, v := range s.context {
		out[k] = v
	}
	return out
}

// Clear resets the session, removing all turns and context entries.
func (s *SessionMemory) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.turns = nil
	s.context = make(map[string]string)
}

// -------------------------------------------------------------------
// Project memory
// -------------------------------------------------------------------

// Note is a persistent project-level memory entry.
type Note struct {
	ID        string    `json:"id"`
	Category  string    `json:"category"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ProjectMemory persists notes and decisions to a JSON file in the project.
type ProjectMemory struct {
	mu       sync.RWMutex
	filePath string
	notes    []Note
}

// NewProjectMemory loads (or creates) the project memory file at path.
func NewProjectMemory(path string) (*ProjectMemory, error) {
	pm := &ProjectMemory{filePath: path}
	if err := pm.load(); err != nil {
		return nil, err
	}
	return pm, nil
}

func (pm *ProjectMemory) load() error {
	data, err := os.ReadFile(pm.filePath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading project memory: %w", err)
	}
	return json.Unmarshal(data, &pm.notes)
}

func (pm *ProjectMemory) save() error {
	if err := os.MkdirAll(filepath.Dir(pm.filePath), 0o755); err != nil {
		return fmt.Errorf("creating memory dir: %w", err)
	}
	data, err := json.MarshalIndent(pm.notes, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling project memory: %w", err)
	}
	return os.WriteFile(pm.filePath, data, 0o644)
}

// AddNote creates a new note and persists it.
func (pm *ProjectMemory) AddNote(category, content string) (Note, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	note := Note{
		ID:        fmt.Sprintf("%d", time.Now().UnixNano()),
		Category:  category,
		Content:   content,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	pm.notes = append(pm.notes, note)
	return note, pm.save()
}

// UpdateNote replaces the content of an existing note by ID.
func (pm *ProjectMemory) UpdateNote(id, content string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	for i := range pm.notes {
		if pm.notes[i].ID == id {
			pm.notes[i].Content = content
			pm.notes[i].UpdatedAt = time.Now()
			return pm.save()
		}
	}
	return fmt.Errorf("note %s not found", id)
}

// DeleteNote removes a note by ID.
func (pm *ProjectMemory) DeleteNote(id string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	for i, n := range pm.notes {
		if n.ID == id {
			pm.notes = append(pm.notes[:i], pm.notes[i+1:]...)
			return pm.save()
		}
	}
	return fmt.Errorf("note %s not found", id)
}

// Notes returns all stored notes.
func (pm *ProjectMemory) Notes() []Note {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	out := make([]Note, len(pm.notes))
	copy(out, pm.notes)
	return out
}

// NotesByCategory returns notes matching the given category.
func (pm *ProjectMemory) NotesByCategory(category string) []Note {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	var out []Note
	for _, n := range pm.notes {
		if n.Category == category {
			out = append(out, n)
		}
	}
	return out
}
