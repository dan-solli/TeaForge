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
type PinnedNotesSource struct{}

func NewPinnedNotesSource() *PinnedNotesSource {
	return &PinnedNotesSource{}
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

	selected := filterPinnedNotes(req.ProjectNotes)
	if len(selected) == 0 {
		return nil, nil
	}

	body, err := renderPromptTemplate("pinned_notes.tmpl", struct {
		Notes []memory.Note
	}{Notes: selected})
	if err != nil {
		return nil, err
	}

	return []ContextItem{{
		Source:   s.Name(),
		Kind:     "project_memory",
		Role:     ollama.RoleSystem,
		Body:     body,
		Priority: s.Priority(),
		PinKey:   "project_notes",
	}}, nil
}

func filterPinnedNotes(in []memory.Note) []memory.Note {
	out := make([]memory.Note, 0, len(in))
	for _, n := range in {
		switch strings.ToLower(strings.TrimSpace(n.Category)) {
		case "pinned", "architecture", "always", "postmortem", "debugging":
			out = append(out, n)
		}
	}
	return out
}
