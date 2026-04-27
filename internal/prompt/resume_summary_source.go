package prompt

import (
	"context"
	"fmt"
	"strings"

	"github.com/dan-solli/teaforge/internal/ollama"
)

// ResumeSummarySource injects persisted summary context when resuming a session.
type ResumeSummarySource struct{}

func NewResumeSummarySource() *ResumeSummarySource {
	return &ResumeSummarySource{}
}

func (s *ResumeSummarySource) Name() string { return "resume_summary" }

func (s *ResumeSummarySource) Mode() ContextMode { return ModePinned }

func (s *ResumeSummarySource) Priority() int { return 95 }

func (s *ResumeSummarySource) Collect(_ context.Context, req *Request) ([]ContextItem, error) {
	if req == nil {
		return nil, fmt.Errorf("nil request")
	}
	if req.SystemPrompt != "" {
		return nil, nil
	}
	summary := strings.TrimSpace(req.ResumeSummary)
	if summary == "" {
		return nil, nil
	}
	body := "<conversation_summary>\n" + summary + "\n</conversation_summary>\n\n"
	return []ContextItem{{
		Source:   s.Name(),
		Kind:     "summary",
		Role:     ollama.RoleSystem,
		Body:     body,
		Priority: s.Priority(),
		PinKey:   "resume_summary",
	}}, nil
}
