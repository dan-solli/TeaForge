package guardrails

import (
	"context"
	"errors"
	"testing"

	"github.com/dan-solli/teaforge/internal/ollama"
	"github.com/dan-solli/teaforge/internal/prompt"
	sesslog "github.com/dan-solli/teaforge/internal/session"
)

func TestSnapshotGuardrailApply_NoAppendFn(t *testing.T) {
	t.Parallel()
	g := NewSnapshotGuardrail(nil)
	in := []ollama.Message{{Role: ollama.RoleUser, Content: "hello"}}
	out, err := g.Apply(context.Background(), in, prompt.NewPromptTrace())
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(out) != 1 || out[0].Content != "hello" {
		t.Fatalf("unexpected output: %#v", out)
	}
}

func TestSnapshotGuardrailApply_AppendsSnapshot(t *testing.T) {
	t.Parallel()
	var gotRole string
	var gotContent string
	g := NewSnapshotGuardrail(func(role, content string) error {
		gotRole = role
		gotContent = content
		return nil
	})
	in := []ollama.Message{{Role: ollama.RoleSystem, Content: "sys"}}
	out, err := g.Apply(context.Background(), in, prompt.NewPromptTrace())
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(out) != 1 || out[0].Role != ollama.RoleSystem {
		t.Fatalf("unexpected output: %#v", out)
	}
	if gotRole != sesslog.RolePromptSnapshot {
		t.Fatalf("role=%q want %q", gotRole, sesslog.RolePromptSnapshot)
	}
	if gotContent == "" {
		t.Fatal("expected non-empty serialized snapshot")
	}
}

func TestSnapshotGuardrailApply_AppendError(t *testing.T) {
	t.Parallel()
	g := NewSnapshotGuardrail(func(role, content string) error {
		return errors.New("boom")
	})
	_, err := g.Apply(context.Background(), []ollama.Message{{Role: ollama.RoleUser, Content: "x"}}, prompt.NewPromptTrace())
	if err == nil {
		t.Fatal("expected error")
	}
}
