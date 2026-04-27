package main

import (
	"context"
	"embed"
	"fmt"
	"log"
	"time"

	"prompt-pipeline/engine"
	"prompt-pipeline/pipeline"
)

//go:embed templates/*.txt
var templateFS embed.FS

// MockProvider implements pipeline.Provider
type MockProvider struct{}

func (m *MockProvider) Provide(ctx context.Context, identifier string) (map[string]any, error) {
	// In a real scenario, this would fetch data from a DB, API, etc.
	// For now, we just return some static data.
	return map[string]any{
		"AssistantName": "TeaForge",
		"CurrentTime":   time.Now().Format(time.RFC1123),
		"UserQuery":     "How do I build a robust prompt pipeline?",
		"Instructions":  "Be alright, be concise and provide a structured answer.",
	}, nil
}

// PrintGuardrail implements pipeline.Guardrail
type PrintGuardrail struct{}

func (p *PrintGuardrail) Process(ctx context.Context, prompt string) (string, error) {
	fmt.Println("[Guardrail] Inspecting prompt...")
	// In a real scenario, this might check for PII or forbidden content.
	return prompt, nil
}

func main() {
	ctx := context.Background()

	// Initialize the components
	provider := &MockProvider{}
	guardrail := &PrintGuardrail{}

	// Initialize the Assembler with our embedded templates, providers, and guardrails
	assembler := engine.NewAssembler(
		templateFS,
		[]pipeline.Provider{provider},
		[]pipeline.Guardrail{guardrail},
	)

	// Assemble the prompt using a context ID (which our MockProvider will use)
	// In this case, we'll just pass "user_session_123"
	prompt, err := assembler.Assemble(ctx, "system_prompt", "user_session_123")
	if err != nil {
		log.Fatalf("Failed to assemble prompt: %v", err)
	}

	// Output the final assembled prompt (The Audit step)
	fmt.Println("\n--- ASSEMBLED PROMPT ---")
	fmt.Println(prompt)
	fmt.Println("--- END OF PROMPT ---")
}
