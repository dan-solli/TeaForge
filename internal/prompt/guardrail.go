package prompt

import (
	"context"

	"github.com/dan-solli/teaforge/internal/ollama"
)

// Guardrail transforms or validates assembled messages before they are sent.
type Guardrail interface {
	Apply(ctx context.Context, messages []ollama.Message, trace *PromptTrace) ([]ollama.Message, error)
}
