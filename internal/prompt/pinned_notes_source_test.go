package prompt

import (
	"context"
	"strings"
	"testing"

	"github.com/dan-solli/teaforge/internal/memory"
)

func TestPinnedNotesSourceCollect_FiltersToPinnedCategories(t *testing.T) {
	t.Parallel()

	src := NewPinnedNotesSource()
	items, err := src.Collect(context.Background(), &Request{
		ProjectNotes: []memory.Note{
			{Category: "decision", Content: "not pinned"},
			{Category: "pinned", Content: "must include"},
			{Category: "architecture", Content: "include arch"},
			{Category: "always", Content: "include always"},
		},
	})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 context item, got %d", len(items))
	}
	body := items[0].Body
	if strings.Contains(body, "not pinned") {
		t.Fatalf("unexpected non-pinned note in body: %q", body)
	}
	for _, want := range []string{"must include", "include arch", "include always"} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected %q in body: %q", want, body)
		}
	}
}

func TestPinnedNotesSourceCollect_LegacyIncludesAll(t *testing.T) {
	t.Parallel()

	src := newLegacyPinnedNotesSource()
	items, err := src.Collect(context.Background(), &Request{
		ProjectNotes: []memory.Note{
			{Category: "decision", Content: "legacy include"},
		},
	})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 context item, got %d", len(items))
	}
	if !strings.Contains(items[0].Body, "legacy include") {
		t.Fatalf("expected note in legacy output: %q", items[0].Body)
	}
}
