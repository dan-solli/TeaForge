package prompt

import (
	"context"
	"fmt"

	"github.com/dan-solli/teaforge/internal/ollama"
)

// CodeIndexSummarySource emits a summary of indexed code symbols.
type CodeIndexSummarySource struct{}

func NewCodeIndexSummarySource() *CodeIndexSummarySource {
	return &CodeIndexSummarySource{}
}

func (s *CodeIndexSummarySource) Name() string { return "code_index_summary" }

func (s *CodeIndexSummarySource) Mode() ContextMode { return ModePinned }

func (s *CodeIndexSummarySource) Priority() int { return 60 }

func (s *CodeIndexSummarySource) Collect(_ context.Context, req *Request) ([]ContextItem, error) {
	if req == nil {
		return nil, fmt.Errorf("nil request")
	}
	if req.SystemPrompt != "" || req.CodeSymbolCount == 0 {
		return nil, nil
	}

	body := fmt.Sprintf("## Code Index (%d symbols indexed)\n", req.CodeSymbolCount) +
		"Use the search_code tool to look up specific symbols.\n\n"

	return []ContextItem{{
		Source:   s.Name(),
		Kind:     "code_index",
		Role:     ollama.RoleSystem,
		Body:     body,
		Priority: s.Priority(),
		PinKey:   "code_index_summary",
	}}, nil
}
