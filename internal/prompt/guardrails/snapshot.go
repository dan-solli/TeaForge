package guardrails

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dan-solli/teaforge/internal/ollama"
	"github.com/dan-solli/teaforge/internal/prompt"
	sesslog "github.com/dan-solli/teaforge/internal/session"
)

// SnapshotGuardrail persists the fully assembled message list before send.
type SnapshotGuardrail struct {
	appendFn func(role, content string) error
}

func NewSnapshotGuardrail(appendFn func(role, content string) error) *SnapshotGuardrail {
	return &SnapshotGuardrail{appendFn: appendFn}
}

func (g *SnapshotGuardrail) Apply(_ context.Context, messages []ollama.Message, _ *prompt.PromptTrace) ([]ollama.Message, error) {
	if g == nil || g.appendFn == nil {
		return messages, nil
	}

	data, err := json.Marshal(messages)
	if err != nil {
		return nil, fmt.Errorf("marshal prompt snapshot: %w", err)
	}
	if err := g.appendFn(sesslog.RolePromptSnapshot, string(data)); err != nil {
		return nil, fmt.Errorf("append prompt snapshot: %w", err)
	}
	return messages, nil
}
