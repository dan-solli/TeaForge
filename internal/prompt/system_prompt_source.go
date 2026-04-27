package prompt

import (
	"context"
	"fmt"

	"github.com/dan-solli/teaforge/internal/ollama"
)

const baseSystemPrompt = `You are TeaForge, an expert software development AI assistant running locally.
You help developers understand, write, and improve code.
You have access to tools that let you read files, write files, edit code,
run commands, search the codebase, and save project notes.

When you need to look at code or project structure, use the available tools.
When you make decisions or discover important information, save it as a project note.
Always explain what you are doing and why.

`

// SystemPromptSource emits the base or configured system prompt.
type SystemPromptSource struct{}

func NewSystemPromptSource() *SystemPromptSource {
	return &SystemPromptSource{}
}

func (s *SystemPromptSource) Name() string { return "system_prompt" }

func (s *SystemPromptSource) Mode() ContextMode { return ModePinned }

func (s *SystemPromptSource) Priority() int { return 100 }

func (s *SystemPromptSource) Collect(_ context.Context, req *Request) ([]ContextItem, error) {
	if req == nil {
		return nil, fmt.Errorf("nil request")
	}
	content := req.SystemPrompt
	if content == "" {
		content = baseSystemPrompt
	}
	return []ContextItem{{
		Source:   s.Name(),
		Kind:     "instruction",
		Role:     ollama.RoleSystem,
		Body:     content,
		Priority: s.Priority(),
		PinKey:   "system_prompt",
	}}, nil
}
