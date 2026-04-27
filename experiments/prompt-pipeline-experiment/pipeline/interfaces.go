package pipeline

import (
	"context"
)

// Provider defines the interface for fetching context data for a prompt.
type Provider interface {
	// Provide returns a map of data required by the template.
	// The key could be a context identifier (e.g., "user_context", "retrieved_docs").
	Provide(ctx context.Context, identifier string) (map[string]any, error)
}

// Guardrail defines the interface for inspecting and modifying the assembled prompt.
type Guardrail interface {
	// Process takes the assembled prompt and returns the (potentially modified) prompt.
	// It returns an error if the prompt violates safety or structural rules.
	Process(ctx context.Context, prompt string) (string, error)
}
