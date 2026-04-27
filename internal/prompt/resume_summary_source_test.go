package prompt

import (
	"context"
	"strings"
	"testing"

	"github.com/dan-solli/teaforge/internal/ollama"
)

func TestResumeSummarySourceCollect_WithSummary(t *testing.T) {
	t.Parallel()

	src := NewResumeSummarySource()
	items, err := src.Collect(context.Background(), &Request{ResumeSummary: "important decision"})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].Role != ollama.RoleSystem {
		t.Fatalf("expected system role, got %q", items[0].Role)
	}
	if !strings.Contains(items[0].Body, "<conversation_summary>") {
		t.Fatalf("missing summary wrapper: %q", items[0].Body)
	}
}

func TestResumeSummarySourceCollect_NoSummary(t *testing.T) {
	t.Parallel()

	src := NewResumeSummarySource()
	items, err := src.Collect(context.Background(), &Request{})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected no items, got %d", len(items))
	}
}
