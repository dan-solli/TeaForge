package session

// Package session provides persistent logging for TeaForge chat sessions.
//
// Each session is written as a single JSON file under a configurable directory
// (default: <workdir>/.teaforge/sessions/).  Files are named by their start
// timestamp so they sort chronologically and can be loaded for replay or
// analysis without any database.
//
// File format
//
//	{
//	  "id":           "2026-04-27T14-05-00Z",
//	  "started_at":  "2026-04-27T14:05:00.000Z",
//	  "model":       "gemma4:26b",
//	  "work_dir":    "/path/to/project",
//	  "turns": [
//	    {"seq":1, "role":"system",    "content":"...", "timestamp":"..."},
//	    {"seq":2, "role":"user",      "content":"...", "timestamp":"..."},
//	    {"seq":3, "role":"assistant", "content":"...", "timestamp":"..."},
//	    {"seq":4, "role":"tool_call", "content":"tool_name: {args}", "timestamp":"..."},
//	    {"seq":5, "role":"tool",      "content":"...", "timestamp":"..."}
//	  ]
//	}
//
// Writes are atomic (write-to-temp + rename) so a crash mid-write never
// corrupts a previous good state.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Turn is one entry in the conversation transcript.
type Turn struct {
	Seq       int       `json:"seq"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

// Log is a persistent, append-safe record of one chat session.
// A nil *Log is safe to use — all methods become no-ops, which lets the
// agent run without logging when no SessionsDir is configured.
type Log struct {
	mu sync.Mutex

	// Exported so they serialise directly.
	ID        string    `json:"id"`
	StartedAt time.Time `json:"started_at"`
	Model     string    `json:"model"`
	WorkDir   string    `json:"work_dir"`
	Turns     []Turn    `json:"turns"`

	path string
}

// New creates a new session log file in dir.
// The file is named <id>.json where id is derived from the current UTC time.
// The directory is created if it does not exist.
func New(dir, model, workDir string) (*Log, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("session: creating sessions dir: %w", err)
	}

	now := time.Now().UTC()
	// Use a filesystem-safe timestamp as the session ID.
	id := now.Format("2006-01-02T15-04-05Z")
	path := filepath.Join(dir, id+".json")

	l := &Log{
		ID:        id,
		StartedAt: now,
		Model:     model,
		WorkDir:   workDir,
		path:      path,
	}
	if err := l.flush(); err != nil {
		return nil, err
	}
	return l, nil
}

// Append adds a turn to the log and persists it to disk.
// Calling Append on a nil *Log is a no-op.
func (l *Log) Append(role, content string) error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	l.Turns = append(l.Turns, Turn{
		Seq:       len(l.Turns) + 1,
		Role:      role,
		Content:   content,
		Timestamp: time.Now().UTC(),
	})
	return l.flush()
}

// Path returns the absolute path to the log file on disk.
func (l *Log) Path() string {
	if l == nil {
		return ""
	}
	return l.path
}

// flush writes the log atomically: it serialises to a .tmp file and then
// renames it over the target path.  Must be called with l.mu held.
func (l *Log) flush() error {
	data, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return fmt.Errorf("session: marshalling log: %w", err)
	}

	tmp := l.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("session: writing log: %w", err)
	}
	if err := os.Rename(tmp, l.path); err != nil {
		return fmt.Errorf("session: renaming log: %w", err)
	}
	return nil
}
