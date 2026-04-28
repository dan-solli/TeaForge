package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
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

func TestAgentRun_FiltersNonPinnedNotesInSystemPrompt(t *testing.T) {
	t.Parallel()

	var captured ollama.ChatRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Fatalf("path=%q", r.URL.Path)
		}
		var req ollama.ChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		captured = req

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{\"model\":\"m\",\"message\":{\"role\":\"assistant\",\"content\":\"ok\"},\"done\":true}\n"))
	}))
	defer server.Close()

	dir := t.TempDir()
	a, err := New(Config{
		Model:      "m",
		OllamaURL:  server.URL,
		WorkDir:    dir,
		MemoryFile: filepath.Join(dir, "memory.json"),
		NumCtx:     16384,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := a.Project().AddNote("decision", "should-not-be-included"); err != nil {
		t.Fatalf("AddNote decision: %v", err)
	}
	if _, err := a.Project().AddNote("postmortem", "should-be-included"); err != nil {
		t.Fatalf("AddNote postmortem: %v", err)
	}

	events := make(chan Event, 16)
	a.Run(context.Background(), "hello", nil, events)
	for range events {
	}

	if len(captured.Messages) == 0 {
		t.Fatal("expected captured chat request")
	}
	sys := captured.Messages[0].Content
	if strings.Contains(sys, "should-not-be-included") {
		t.Fatalf("non-pinned note leaked into prompt\ncontent=%q", sys)
	}
	if !strings.Contains(sys, "should-be-included") {
		t.Fatalf("postmortem note missing from prompt\ncontent=%q", sys)
	}
}

func TestAgentRun_ToolLoopGuardForcesFinalNoToolResponse(t *testing.T) {
	t.Parallel()

	var calls int32
	var sawNoToolsRequest atomic.Bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Fatalf("path=%q", r.URL.Path)
		}
		var req ollama.ChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		if len(req.Tools) == 0 {
			sawNoToolsRequest.Store(true)
			_, _ = w.Write([]byte(`{"model":"m","message":{"role":"assistant","content":"Final summary after loop guard."},"done":false}` + "\n"))
			_, _ = w.Write([]byte(`{"model":"m","message":{"role":"assistant","content":""},"done":true}` + "\n"))
			return
		}

		atomic.AddInt32(&calls, 1)
		_, _ = w.Write([]byte(`{"model":"m","message":{"role":"assistant","content":"","tool_calls":[{"function":{"name":"unknown_tool","arguments":{"path":"x"}}}]},"done":true}` + "\n"))
	}))
	defer server.Close()

	dir := t.TempDir()
	a, err := New(Config{
		Model:                            "m",
		OllamaURL:                        server.URL,
		WorkDir:                          dir,
		MemoryFile:                       filepath.Join(dir, "memory.json"),
		NumCtx:                           2048,
		MaxToolIterations:                2,
		MaxConsecutiveDuplicateToolCalls: 10,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	events := make(chan Event, 64)
	a.Run(context.Background(), "please fix this", nil, events)

	var sawDone bool
	var sawFinalToken bool
	for ev := range events {
		if ev.Type == EventDone {
			sawDone = true
		}
		if ev.Type == EventToken && strings.Contains(ev.Content, "Final summary after loop guard") {
			sawFinalToken = true
		}
	}

	if !sawDone {
		t.Fatal("expected done event")
	}
	if !sawNoToolsRequest.Load() {
		t.Fatal("expected final request without tools after loop guard")
	}
	if !sawFinalToken {
		t.Fatal("expected final summary token after loop guard")
	}
	if atomic.LoadInt32(&calls) < 2 {
		t.Fatalf("expected at least 2 tool rounds, got %d", calls)
	}
}

func TestAgentRun_ToolEventsAreSummarized(t *testing.T) {
	t.Parallel()

	secret := "TOP_SECRET_FILE_CONTENT"
	dir := t.TempDir()
	path := filepath.Join(dir, "secret.txt")
	if err := os.WriteFile(path, []byte(secret), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var call int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Fatalf("path=%q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		if atomic.AddInt32(&call, 1) == 1 {
			payload := `{"model":"m","message":{"role":"assistant","content":"","tool_calls":[{"function":{"name":"read_file","arguments":{"path":"` + path + `"}}}]},"done":true}` + "\n"
			_, _ = w.Write([]byte(payload))
			return
		}
		_, _ = w.Write([]byte(`{"model":"m","message":{"role":"assistant","content":"done"},"done":false}` + "\n"))
		_, _ = w.Write([]byte(`{"model":"m","message":{"role":"assistant","content":""},"done":true}` + "\n"))
	}))
	defer server.Close()

	a, err := New(Config{
		Model:      "m",
		OllamaURL:  server.URL,
		WorkDir:    dir,
		MemoryFile: filepath.Join(dir, "memory.json"),
		NumCtx:     2048,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	events := make(chan Event, 64)
	a.Run(context.Background(), "inspect", nil, events)

	var callSummary string
	var resultSummary string
	for ev := range events {
		if ev.Type == EventToolCall {
			callSummary = ev.Content
		}
		if ev.Type == EventToolResult {
			resultSummary = ev.Content
		}
		if strings.Contains(ev.Content, secret) {
			t.Fatalf("tool event leaked raw file content: %q", ev.Content)
		}
	}

	if !strings.Contains(callSummary, "Reading file") {
		t.Fatalf("expected summarized tool call, got %q", callSummary)
	}
	if !strings.Contains(resultSummary, "Read file") {
		t.Fatalf("expected summarized tool result, got %q", resultSummary)
	}
}

func TestAgentRun_NoProgressToolLoopGuardForcesFinalNoToolResponse(t *testing.T) {
	t.Parallel()

	var calls int32
	var sawNoToolsRequest atomic.Bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Fatalf("path=%q", r.URL.Path)
		}
		var req ollama.ChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		if len(req.Tools) == 0 {
			sawNoToolsRequest.Store(true)
			_, _ = w.Write([]byte(`{"model":"m","message":{"role":"assistant","content":"Final response after no-progress guard."},"done":false}` + "\n"))
			_, _ = w.Write([]byte(`{"model":"m","message":{"role":"assistant","content":""},"done":true}` + "\n"))
			return
		}

		idx := atomic.AddInt32(&calls, 1)
		missingPath := fmt.Sprintf("/definitely/missing-%d.txt", idx)
		payload := `{"model":"m","message":{"role":"assistant","content":"","tool_calls":[{"function":{"name":"read_file","arguments":{"path":"` + missingPath + `"}}}]},"done":true}` + "\n"
		_, _ = w.Write([]byte(payload))
	}))
	defer server.Close()

	dir := t.TempDir()
	a, err := New(Config{
		Model:                            "m",
		OllamaURL:                        server.URL,
		WorkDir:                          dir,
		MemoryFile:                       filepath.Join(dir, "memory.json"),
		NumCtx:                           2048,
		MaxToolIterations:                20,
		MaxConsecutiveDuplicateToolCalls: 20,
		MaxNoProgressToolRounds:          3,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	events := make(chan Event, 128)
	a.Run(context.Background(), "investigate", nil, events)

	var sawDone bool
	var sawFinalToken bool
	var sawError string
	var eventTypes []string
	for ev := range events {
		eventTypes = append(eventTypes, ev.Type)
		if ev.Type == EventDone {
			sawDone = true
		}
		if ev.Type == EventError {
			sawError = ev.Content
		}
		if ev.Type == EventToken && strings.Contains(ev.Content, "Final response after no-progress guard") {
			sawFinalToken = true
		}
	}

	if !sawDone {
		t.Fatalf("expected done event, got events=%v error=%q", eventTypes, sawError)
	}
	if !sawNoToolsRequest.Load() {
		t.Fatalf("expected final request without tools after no-progress guard, got calls=%d events=%v error=%q", atomic.LoadInt32(&calls), eventTypes, sawError)
	}
	if !sawFinalToken {
		t.Fatalf("expected final summary token after no-progress guard, got events=%v error=%q", eventTypes, sawError)
	}
	if got := atomic.LoadInt32(&calls); got > 3 {
		t.Fatalf("expected no-progress guard to stop within 3 tool rounds, got %d", got)
	}
}
