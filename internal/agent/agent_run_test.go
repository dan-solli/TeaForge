package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/dan-solli/teaforge/internal/ollama"
)

func TestAgentRun_StreamsAndCompletes(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Fatalf("path=%q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{\"model\":\"m\",\"message\":{\"role\":\"assistant\",\"content\":\"hello\"},\"done\":false}\n"))
		_, _ = w.Write([]byte("{\"model\":\"m\",\"message\":{\"role\":\"assistant\",\"content\":\"\"},\"done\":true}\n"))
	}))
	defer server.Close()

	dir := t.TempDir()
	a, err := New(Config{
		Model:      "m",
		OllamaURL:  server.URL,
		WorkDir:    dir,
		MemoryFile: filepath.Join(dir, "memory.json"),
		NumCtx:     1024,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	events := make(chan Event, 16)
	a.Run(context.Background(), "hello", nil, events)

	var types []string
	for ev := range events {
		types = append(types, ev.Type)
	}
	joined := strings.Join(types, ",")
	for _, want := range []string{EventContext, EventToken, EventDone} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing event %q in %q", want, joined)
		}
	}
	turns := a.Session().Turns()
	if len(turns) == 0 || turns[len(turns)-1].Role != ollama.RoleAssistant {
		t.Fatalf("unexpected session turns: %#v", turns)
	}
}

func TestAgentRun_ToolCallUnknownToolFlow(t *testing.T) {
	t.Parallel()

	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Fatalf("path=%q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		call := atomic.AddInt32(&calls, 1)
		if call == 1 {
			_, _ = w.Write([]byte(`{"model":"m","message":{"role":"assistant","content":"","tool_calls":[{"function":{"name":"unknown_tool","arguments":{"x":"1"}}}]},"done":true}` + "\n"))
			return
		}
		_, _ = w.Write([]byte("{\"model\":\"m\",\"message\":{\"role\":\"assistant\",\"content\":\"final\"},\"done\":false}\n"))
		_, _ = w.Write([]byte("{\"model\":\"m\",\"message\":{\"role\":\"assistant\",\"content\":\"\"},\"done\":true}\n"))
	}))
	defer server.Close()

	dir := t.TempDir()
	a, err := New(Config{
		Model:      "m",
		OllamaURL:  server.URL,
		WorkDir:    dir,
		MemoryFile: filepath.Join(dir, "memory.json"),
		NumCtx:     1024,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	events := make(chan Event, 32)
	a.Run(context.Background(), "hello", nil, events)

	var eventTypes []string
	var toolResultSeen bool
	for ev := range events {
		eventTypes = append(eventTypes, ev.Type)
		if ev.Type == EventToolResult && strings.Contains(ev.Content, "unknown tool") {
			toolResultSeen = true
		}
	}
	joined := strings.Join(eventTypes, ",")
	for _, want := range []string{EventToolCall, EventToolResult, EventDone} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing event %q in %q", want, joined)
		}
	}
	if !toolResultSeen {
		t.Fatalf("expected unknown tool result in events, got %q", joined)
	}

	turns := a.Session().Turns()
	var toolTurnFound bool
	for _, turn := range turns {
		if turn.Role == ollama.RoleTool && strings.Contains(turn.Content, "unknown tool") {
			toolTurnFound = true
			break
		}
	}
	if !toolTurnFound {
		t.Fatalf("expected tool turn in session, got %#v", turns)
	}
}

func TestAgentRun_EmitsErrorOnStreamFailure(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}))
	defer server.Close()

	dir := t.TempDir()
	a, err := New(Config{
		Model:      "m",
		OllamaURL:  server.URL,
		WorkDir:    dir,
		MemoryFile: filepath.Join(dir, "memory.json"),
		NumCtx:     1024,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	events := make(chan Event, 16)
	a.Run(context.Background(), "hello", nil, events)

	var sawError bool
	for ev := range events {
		if ev.Type == EventError {
			sawError = true
			break
		}
	}
	if !sawError {
		t.Fatal("expected EventError from stream failure")
	}
}
