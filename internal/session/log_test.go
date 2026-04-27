package session_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/dan-solli/teaforge/internal/session"
)

func TestLog_Append(t *testing.T) {
	dir := t.TempDir()

	l, err := session.New(dir, "test-model", "/tmp/work")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	turns := []struct{ role, content string }{
		{"system", "You are TeaForge."},
		{"user", "Hello!"},
		{"assistant", "Hi there!"},
		{"tool_call", `read_file: {"path":"main.go"}`},
		{"tool", "package main..."},
	}
	for _, tr := range turns {
		if err := l.Append(tr.role, tr.content); err != nil {
			t.Fatalf("Append(%q): %v", tr.role, err)
		}
	}

	data, err := os.ReadFile(l.Path())
	if err != nil {
		t.Fatalf("reading log file: %v", err)
	}

	var out struct {
		Model string `json:"model"`
		Turns []struct {
			Seq     int    `json:"seq"`
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"turns"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshalling: %v", err)
	}
	if out.Model != "test-model" {
		t.Errorf("model = %q, want %q", out.Model, "test-model")
	}
	if len(out.Turns) != len(turns) {
		t.Fatalf("turn count = %d, want %d", len(out.Turns), len(turns))
	}
	for i, want := range turns {
		got := out.Turns[i]
		if got.Role != want.role {
			t.Errorf("turn[%d].role = %q, want %q", i, got.Role, want.role)
		}
		if got.Content != want.content {
			t.Errorf("turn[%d].content = %q, want %q", i, got.Content, want.content)
		}
		if got.Seq != i+1 {
			t.Errorf("turn[%d].seq = %d, want %d", i, got.Seq, i+1)
		}
	}
}

func TestLog_NilSafe(t *testing.T) {
	var l *session.Log
	if err := l.Append("user", "should not panic"); err != nil {
		t.Errorf("nil Append returned unexpected error: %v", err)
	}
	if l.Path() != "" {
		t.Errorf("nil Path should be empty")
	}
}
