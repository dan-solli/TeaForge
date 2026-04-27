package prompt

import (
	"context"
	"testing"

	"github.com/dan-solli/teaforge/internal/memory"
	"github.com/dan-solli/teaforge/internal/ollama"
)

func TestLegacyPipelineBuild_DefaultPromptAndHistory(t *testing.T) {
	t.Parallel()

	p := NewLegacyPipeline()
	req := &Request{
		SessionTurns: []memory.Turn{
			{Role: ollama.RoleUser, Content: "hello"},
			{Role: ollama.RoleAssistant, Content: "hi"},
		},
		ProjectNotes: []memory.Note{
			{Category: "decision", Content: "Use refactor-first"},
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

	expectedPrompt := `You are TeaForge, an expert software development AI assistant running locally.
You help developers understand, write, and improve code.
You have access to tools that let you read files, write files, edit code,
run commands, search the codebase, and save project notes.

When you need to look at code or project structure, use the available tools.
When you make decisions or discover important information, save it as a project note.
Always explain what you are doing and why.

## Project Memory (decisions and notes)
- [decision] Use refactor-first
- [todo] Add tests

## Code Index (3 symbols indexed)
Use the search_code tool to look up specific symbols.

`

	if msgs[0].Content != expectedPrompt {
		t.Fatalf("unexpected system prompt\nwant:\n%q\ngot:\n%q", expectedPrompt, msgs[0].Content)
	}

	if msgs[1].Role != ollama.RoleUser || msgs[1].Content != "hello" {
		t.Fatalf("unexpected second message: %#v", msgs[1])
	}
	if msgs[2].Role != ollama.RoleAssistant || msgs[2].Content != "hi" {
		t.Fatalf("unexpected third message: %#v", msgs[2])
	}
}

func TestLegacyPipelineBuild_OverrideSystemPrompt(t *testing.T) {
	t.Parallel()

	p := NewLegacyPipeline()
	req := &Request{
		SystemPrompt:    "custom prompt",
		ProjectNotes:    []memory.Note{{Category: "decision", Content: "ignored"}},
		CodeSymbolCount: 42,
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

func TestLegacyPipelineBuild_NilRequest(t *testing.T) {
	t.Parallel()

	p := NewLegacyPipeline()
	_, _, err := p.Build(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil request")
	}
}
