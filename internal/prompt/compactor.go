package prompt

import (
	"context"

	"github.com/dan-solli/teaforge/internal/memory"
)

// Compactor summarizes older turns when history no longer fits budget.
type Compactor interface {
	ShouldCompact(turns []memory.Turn, available int) bool
	Compact(ctx context.Context, turns []memory.Turn, keepLatest int) (summary string, kept []memory.Turn, err error)
}
