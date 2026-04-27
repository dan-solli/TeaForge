package prompt_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/dan-solli/teaforge/internal/memory"
	"github.com/dan-solli/teaforge/internal/ollama"
	"github.com/dan-solli/teaforge/internal/prompt"
	"github.com/dan-solli/teaforge/internal/prompt/guardrails"
	"github.com/dan-solli/teaforge/internal/session"
)

func TestDefaultPipelineBuild_WritesPromptSnapshotToSessionLog(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	log, err := session.New(dir, "test-model", "/tmp/work")
	if err != nil {
		t.Fatalf("session.New: %v", err)
	}

	p := prompt.NewDefaultPipeline([]prompt.Guardrail{
		guardrails.NewSnapshotGuardrail(log.Append),
	})

	req := &prompt.Request{
		SessionTurns:    []memory.Turn{{Role: ollama.RoleUser, Content: "hello"}},
		ProjectNotes:    []memory.Note{{Category: "architecture", Content: "keep snapshots"}},
		CodeSymbolCount: 2,
	}
	_, trace, err := p.Build(context.Background(), req)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if trace == nil {
		t.Fatal("expected non-nil trace")
	}
	if len(trace.Items) != 3 {
		t.Fatalf("expected 3 context items from split sources, got %d", len(trace.Items))
	}

	data, err := os.ReadFile(log.Path())
	if err != nil {
		t.Fatalf("os.ReadFile: %v", err)
	}
	var out struct {
		Turns []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"turns"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	found := false
	for _, tr := range out.Turns {
		if tr.Role == session.RolePromptSnapshot {
			if tr.Content == "" {
				t.Fatal("prompt_snapshot content should not be empty")
			}
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected prompt_snapshot role in session log")
	}
}
