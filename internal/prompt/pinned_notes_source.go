package prompt

import (
	"context"
	"fmt"
	"strings"

	"github.com/dan-solli/teaforge/internal/memory"
	"github.com/dan-solli/teaforge/internal/ollama"
)

// PinnedNotesSource emits the project notes block.
// In Phase 6 default behavior includes only categories important to prompt context.
type PinnedNotesSource struct {
	includeAll bool
}

func NewPinnedNotesSource() *PinnedNotesSource {
	return &PinnedNotesSource{includeAll: false}
}

func newLegacyPinnedNotesSource() *PinnedNotesSource {
	return &PinnedNotesSource{includeAll: true}
}

func (s *PinnedNotesSource) Name() string { return "pinned_notes" }

func (s *PinnedNotesSource) Mode() ContextMode { return ModePinned }

func (s *PinnedNotesSource) Priority() int { return 80 }

func (s *PinnedNotesSource) Collect(_ context.Context, req *Request) ([]ContextItem, error) {
	if req == nil {
		return nil, fmt.Errorf("nil request")
	}
	if req.SystemPrompt != "" || len(req.ProjectNotes) == 0 {
		return nil, nil
	}

	selected := req.ProjectNotes
	if !s.includeAll {
		selected = filterPinnedNotes(req.ProjectNotes)
		if len(selected) == 0 {
			return nil, nil
		}
	}

	var sb strings.Builder
	sb.WriteString("## Project Memory (decisions and notes)\n")
	for _, n := range selected {
		sb.WriteString(fmt.Sprintf("- [%s] %s\n", n.Category, n.Content))
	}
	sb.WriteString("\n")

	return []ContextItem{{
		Source:   s.Name(),
		Kind:     "project_memory",
		Role:     ollama.RoleSystem,
		Body:     sb.String(),
		Priority: s.Priority(),
		PinKey:   "project_notes",
	}}, nil
}

func filterPinnedNotes(in []memory.Note) []memory.Note {
	out := make([]memory.Note, 0, len(in))
	for _, n := range in {
		switch strings.ToLower(strings.TrimSpace(n.Category)) {
		case "pinned", "architecture", "always":
			out = append(out, n)
		}
	}
	return out
}
