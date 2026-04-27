package prompt

import (
	"context"

	"github.com/dan-solli/teaforge/internal/memory"
)

// ContextMode controls whether a source is always included or fetched on demand.
type ContextMode int

const (
	ModePinned ContextMode = iota
	ModeOnDemand
)

// ContextItem is the atomic contribution a source can add to a prompt.
type ContextItem struct {
	Source   string
	Kind     string
	Role     string
	Body     string
	Tokens   int
	Priority int
	PinKey   string
}

// Request is the input to prompt assembly for one user turn.
type Request struct {
	SystemPrompt    string
	WorkDir         string
	Model           string
	UserMessage     string
	AttachedPaths   []string
	ResumeSummary   string
	NumCtx          int
	SessionTurns    []memory.Turn
	ProjectNotes    []memory.Note
	CodeSymbolCount int
}

// ContextSource provides prompt context from one source of truth.
type ContextSource interface {
	Name() string
	Mode() ContextMode
	Priority() int
	Collect(ctx context.Context, req *Request) ([]ContextItem, error)
}
