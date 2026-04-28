package agent

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dan-solli/teaforge/internal/memory"
	"github.com/dan-solli/teaforge/internal/ollama"
)

func makeTurns(n int, content string) []memory.Turn {
	turns := make([]memory.Turn, 0, n)
	for i := 0; i < n; i++ {
		role := ollama.RoleUser
		if i%2 == 1 {
			role = ollama.RoleAssistant
		}
		turns = append(turns, memory.Turn{Role: role, Content: fmt.Sprintf("%s-%d", content, i)})
	}
	return turns
}

func TestLLMCompactor_ShouldCompact(t *testing.T) {
	t.Parallel()

	var nilCompactor *llmCompactor
	if nilCompactor.ShouldCompact(makeTurns(20, "x"), 1) {
		t.Fatal("nil compactor should never compact")
	}

	c := &llmCompactor{}
	if c.ShouldCompact(makeTurns(12, "x"), 1) {
		t.Fatal("<=12 turns should not compact")
	}
	if !c.ShouldCompact(makeTurns(13, strings.Repeat("x", 200)), 1) {
		t.Fatal("expected compaction when turns exceed budget")
	}
}

func TestLLMCompactor_CompactBoundaries(t *testing.T) {
	t.Parallel()

	turns := makeTurns(4, "hello")
	if summary, kept, err := ((*llmCompactor)(nil)).Compact(context.Background(), turns, 2); err != nil || summary != "" || len(kept) != 4 {
		t.Fatalf("nil compactor: summary=%q kept=%d err=%v", summary, len(kept), err)
	}

	c := &llmCompactor{}
	if summary, kept, err := c.Compact(context.Background(), turns, 2); err != nil || summary != "" || len(kept) != 4 {
		t.Fatalf("nil client: summary=%q kept=%d err=%v", summary, len(kept), err)
	}

	if summary, kept, err := c.Compact(context.Background(), turns, 10); err != nil || summary != "" || len(kept) != 4 {
		t.Fatalf("short transcript: summary=%q kept=%d err=%v", summary, len(kept), err)
	}
}

func TestLLMCompactor_CompactSuccess(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Fatalf("path=%q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"m","message":{"role":"assistant","content":"- summary"},"done":true}`))
	}))
	defer server.Close()

	c := newLLMCompactor(ollama.NewClient(server.URL), "m", 1024)
	turns := makeTurns(20, strings.Repeat("x", 50))

	summary, kept, err := c.Compact(context.Background(), turns, 6)
	if err != nil {
		t.Fatalf("Compact: %v", err)
	}
	if summary != "- summary" {
		t.Fatalf("summary=%q", summary)
	}
	if len(kept) != 6 {
		t.Fatalf("kept=%d want 6", len(kept))
	}
}

func TestLLMCompactor_CompactFallbackOnError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}))
	defer server.Close()

	c := newLLMCompactor(ollama.NewClient(server.URL), "m", 1024)
	turns := makeTurns(16, strings.Repeat("x", 80))

	summary, kept, err := c.Compact(context.Background(), turns, 4)
	if err != nil {
		t.Fatalf("Compact: %v", err)
	}
	if !strings.Contains(summary, "Conversation summary fallback") {
		t.Fatalf("expected fallback summary, got %q", summary)
	}
	if len(kept) != 4 {
		t.Fatalf("kept=%d want 4", len(kept))
	}
}

func TestCompactorHelpers(t *testing.T) {
	t.Parallel()

	rendered := renderTurnsForCompaction([]memory.Turn{{Role: "user", Content: strings.Repeat("a", 1300)}})
	if !strings.Contains(rendered, "[1] user:") {
		t.Fatalf("rendered=%q", rendered)
	}
	if !strings.Contains(rendered, "...") {
		t.Fatalf("expected truncation marker in %q", rendered)
	}

	if got := fallbackCompactionSummary(nil); got != "No prior turns to summarize." {
		t.Fatalf("fallback=nil: %q", got)
	}

	fallback := fallbackCompactionSummary(makeTurns(20, strings.Repeat("b", 220)))
	if !strings.Contains(fallback, "additional turns omitted") {
		t.Fatalf("fallback=%q", fallback)
	}

	sections := splitTurnsIntoSections(makeTurns(55, "z"), 20)
	if len(sections) != 3 {
		t.Fatalf("sections=%d want 3", len(sections))
	}
	if len(sections[0]) != 20 || len(sections[1]) != 20 || len(sections[2]) != 15 {
		t.Fatalf("unexpected section sizes: %d %d %d", len(sections[0]), len(sections[1]), len(sections[2]))
	}
}
