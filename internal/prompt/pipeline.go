package prompt

import (
	"context"
	"fmt"

	"github.com/dan-solli/teaforge/internal/ollama"
)

// Pipeline orchestrates source collection and message assembly.
type Pipeline struct {
	sources    []ContextSource
	assembler  *Assembler
	guardrails []Guardrail
}

func NewPipeline(sources []ContextSource, assembler *Assembler, guardrails []Guardrail) *Pipeline {
	if assembler == nil {
		assembler = NewAssembler()
	}
	return &Pipeline{
		sources:    sources,
		assembler:  assembler,
		guardrails: guardrails,
	}
}

func NewDefaultPipeline(guardrails []Guardrail) *Pipeline {
	return NewPipeline([]ContextSource{
		NewSystemPromptSource(),
		NewResumeSummarySource(),
		NewAgentInstructionsSource(),
		NewEnvironmentSource(),
		NewPinnedNotesSource(),
		NewCodeIndexSummarySource(),
		NewAttachmentsSource(),
	}, NewAssembler(), guardrails)
}

func (p *Pipeline) Build(ctx context.Context, req *Request) ([]ollama.Message, *PromptTrace, error) {
	if p == nil {
		return nil, nil, fmt.Errorf("nil pipeline")
	}
	if len(p.sources) == 0 {
		return nil, nil, fmt.Errorf("pipeline has no sources")
	}
	return p.assembler.Build(ctx, req, p.sources, p.guardrails)
}

func (p *Pipeline) SetTokenBudget(total int) {
	if p == nil || p.assembler == nil {
		return
	}
	p.assembler.SetBudget(total)
}

func (p *Pipeline) SetCompactor(compactor Compactor) {
	if p == nil || p.assembler == nil {
		return
	}
	p.assembler.SetCompactor(compactor)
}
