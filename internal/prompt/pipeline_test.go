package prompt

import (
	"context"
	"strings"
	"testing"

	"github.com/dan-solli/teaforge/internal/memory"
	"github.com/dan-solli/teaforge/internal/ollama"
)

func TestDefaultPipelineBuild_DefaultPromptAndHistory(t *testing.T) {
	t.Parallel()

	p := NewDefaultPipeline(nil)
	req := &Request{
		SessionTurns: []memory.Turn{
			{Role: ollama.RoleUser, Content: "hello"},
			{Role: ollama.RoleAssistant, Content: "hi"},
		},
		ProjectNotes: []memory.Note{
			{Category: "pinned", Content: "Use refactor-first"},
			{Category: "todo", Content: "Add tests"},
		},
		CodeSymbolCount: 3,
	}

	msgs, trace, err := p.Build(context.Background(), req)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if trace == nil {
		t.Fatal("Build returned nil trace")
	}
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(msgs))
	}
	if msgs[0].Role != ollama.RoleSystem {
		t.Fatalf("expected first message role %q, got %q", ollama.RoleSystem, msgs[0].Role)
	}

	system := msgs[0].Content
	for _, want := range []string{
		"You are TeaForge, an expert software development AI assistant running locally.",
		"## Project Memory (decisions and notes)",
		"Use refactor-first",
		"## Code Index (3 symbols indexed)",
	} {
		if !strings.Contains(system, want) {
			t.Fatalf("expected system prompt to contain %q\ncontent=%q", want, system)
		}
	}
	if strings.Contains(system, "Add tests") {
		t.Fatalf("expected non-pinned note to be filtered from prompt\ncontent=%q", system)
	}

	if msgs[1].Role != ollama.RoleUser || msgs[1].Content != "hello" {
		t.Fatalf("unexpected second message: %#v", msgs[1])
	}
	if msgs[2].Role != ollama.RoleAssistant || msgs[2].Content != "hi" {
		t.Fatalf("unexpected third message: %#v", msgs[2])
	}
}

func TestDefaultPipelineBuild_OverrideSystemPrompt(t *testing.T) {
	t.Parallel()

	p := NewDefaultPipeline(nil)
	req := &Request{
		SystemPrompt:    "custom prompt",
		ProjectNotes:    []memory.Note{{Category: "pinned", Content: "ignored"}},
		CodeSymbolCount: 42,
		WorkDir:         ".",
		UserMessage:     "@README.md",
		ResumeSummary:   "ignored",
	}

	msgs, _, err := p.Build(context.Background(), req)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Role != ollama.RoleSystem {
		t.Fatalf("expected role %q, got %q", ollama.RoleSystem, msgs[0].Role)
	}
	if msgs[0].Content != "custom prompt" {
		t.Fatalf("expected custom system prompt, got %q", msgs[0].Content)
	}
}

func TestDefaultPipelineBuild_NilRequest(t *testing.T) {
	t.Parallel()

	p := NewDefaultPipeline(nil)
	_, _, err := p.Build(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil request")
	}
}
