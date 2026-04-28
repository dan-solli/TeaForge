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
			{Category: "postmortem", Content: "include postmortem"},
			{Category: "debugging", Content: "include debugging"},
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
	for _, want := range []string{"must include", "include arch", "include always", "include postmortem", "include debugging"} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected %q in body: %q", want, body)
		}
	}
}
