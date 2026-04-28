package prompt

import (
	"context"
	"fmt"

	"github.com/dan-solli/teaforge/internal/ollama"
)

var baseSystemPrompt = mustLoadPromptTemplate("system_prompt.txt")

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
