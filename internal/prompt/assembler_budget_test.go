package prompt

import (
	"context"
	"strings"
	"testing"

	"github.com/dan-solli/teaforge/internal/memory"
	"github.com/dan-solli/teaforge/internal/ollama"
)

type staticSource struct {
	name     string
	priority int
	items    []ContextItem
}

func (s staticSource) Name() string      { return s.name }
func (s staticSource) Mode() ContextMode { return ModePinned }
func (s staticSource) Priority() int     { return s.priority }
func (s staticSource) Collect(_ context.Context, _ *Request) ([]ContextItem, error) {
	return append([]ContextItem(nil), s.items...), nil
}

type fakeCompactor struct {
	called bool
}

func (f *fakeCompactor) ShouldCompact(turns []memory.Turn, available int) bool {
	return len(turns) > 0 && EstimateTurnsTokens(turns) > available
}

func (f *fakeCompactor) Compact(_ context.Context, turns []memory.Turn, keepLatest int) (string, []memory.Turn, error) {
	f.called = true
	if keepLatest <= 0 {
		keepLatest = 2
	}
	if len(turns) <= keepLatest {
		return "", turns, nil
	}
	return "summary block", append([]memory.Turn(nil), turns[len(turns)-keepLatest:]...), nil
}

func TestAssemblerBuild_EvictsLowPrioritySourceItems(t *testing.T) {
	t.Parallel()

	assembler := NewAssembler()
	assembler.SetBudget(100)

	src := staticSource{
		name:     "test",
		priority: 100,
		items: []ContextItem{
			{
				Source:   "system",
				Role:     ollama.RoleSystem,
				Body:     "base",
				Priority: 100,
				PinKey:   "system_prompt",
			},
			{
				Source:   "low_priority",
				Role:     ollama.RoleSystem,
				Body:     strings.Repeat("x", 600),
				Priority: 1,
				PinKey:   "low",
			},
		},
	}

	msgs, trace, err := assembler.Build(context.Background(), &Request{}, []ContextSource{src}, nil)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(msgs) == 0 || msgs[0].Role != ollama.RoleSystem {
		t.Fatalf("expected first message to be system, got %#v", msgs)
	}
	if strings.Contains(msgs[0].Content, strings.Repeat("x", 40)) {
		t.Fatalf("expected low-priority source to be evicted")
	}
	if len(trace.Evictions) == 0 {
		t.Fatal("expected at least one eviction")
	}
}

func TestAssemblerBuild_CompactsHistoryAndTracksFill(t *testing.T) {
	t.Parallel()

	assembler := NewAssembler()
	assembler.SetBudget(200)
	fc := &fakeCompactor{}
	assembler.SetCompactor(fc)

	turns := make([]memory.Turn, 0, 20)
	for i := 0; i < 20; i++ {
		role := ollama.RoleUser
		if i%2 == 1 {
			role = ollama.RoleAssistant
		}
		turns = append(turns, memory.Turn{Role: role, Content: strings.Repeat("history text ", 10)})
	}

	src := staticSource{
		name: "system",
		items: []ContextItem{{
			Source:   "system",
			Role:     ollama.RoleSystem,
			Body:     "base",
			Priority: 100,
			PinKey:   "system_prompt",
		}},
	}

	msgs, trace, err := assembler.Build(context.Background(), &Request{SessionTurns: turns}, []ContextSource{src}, nil)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if !fc.called {
		t.Fatal("expected compactor to be called")
	}
	if !trace.Compacted {
		t.Fatal("expected trace.Compacted to be true")
	}
	if trace.Summary == "" {
		t.Fatal("expected non-empty summary")
	}
	if trace.FillPercent <= 0 {
		t.Fatalf("expected fill percent > 0, got %d", trace.FillPercent)
	}
	foundSummary := false
	for _, m := range msgs {
		if m.Role == ollama.RoleSystem && strings.Contains(m.Content, "<conversation_summary>") {
			foundSummary = true
			break
		}
	}
	if !foundSummary {
		t.Fatal("expected summary system message in assembled prompt")
	}
}
