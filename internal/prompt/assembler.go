package prompt

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/dan-solli/teaforge/internal/ollama"
)

// Assembler converts context items into chat messages.
type Assembler struct {
	budget    TokenBudget
	compactor Compactor
}

func NewAssembler() *Assembler {
	return &Assembler{budget: DefaultTokenBudget(0)}
}

func (a *Assembler) SetBudget(total int) {
	if a == nil {
		return
	}
	a.budget = DefaultTokenBudget(total)
}

func (a *Assembler) SetCompactor(compactor Compactor) {
	if a == nil {
		return
	}
	a.compactor = compactor
}

func (a *Assembler) Build(
	ctx context.Context,
	req *Request,
	sources []ContextSource,
	guardrails []Guardrail,
) ([]ollama.Message, *PromptTrace, error) {
	if req == nil {
		return nil, nil, fmt.Errorf("nil request")
	}

	budget := a.budget
	if req.NumCtx > 0 {
		budget = DefaultTokenBudget(req.NumCtx)
	}

	trace := NewPromptTrace()
	trace.Budget = budget
	messages := make([]ollama.Message, 0, 1+len(req.SessionTurns))
	var systemPrompt strings.Builder
	var summaryBlock string

	var sourceItems []ContextItem
	for _, src := range sources {
		if src.Mode() != ModePinned {
			continue
		}
		items, err := src.Collect(ctx, req)
		if err != nil {
			return nil, nil, fmt.Errorf("collect source %q: %w", src.Name(), err)
		}
		for _, item := range items {
			trace.AddItem(item)
			sourceItems = append(sourceItems, item)
		}
	}

	keptSourceItems, sourceMessages := applySourceBudget(sourceItems, budget.LiveBudget, trace)
	for _, item := range keptSourceItems {
		role := item.Role
		if role == "" {
			role = ollama.RoleSystem
		}
		if role == ollama.RoleSystem {
			systemPrompt.WriteString(item.Body)
		}
	}

	historyTurns := req.SessionTurns
	historyTokens := EstimateTurnsTokens(historyTurns)
	if historyTokens > budget.HistoryBudget && a.compactor != nil && a.compactor.ShouldCompact(historyTurns, budget.HistoryBudget) {
		summary, kept, err := a.compactor.Compact(ctx, historyTurns, 12)
		if err != nil {
			trace.Evictions = append(trace.Evictions, fmt.Sprintf("compactor_error: %v", err))
		} else if strings.TrimSpace(summary) != "" {
			trace.Compacted = true
			trace.Summary = strings.TrimSpace(summary)
			summaryBlock = "<conversation_summary>\n" + trace.Summary + "\n</conversation_summary>\n\n"
			historyTurns = kept
			historyTokens = EstimateTurnsTokens(historyTurns)
		}
	}

	historyLimit := budget.HistoryBudget - EstimateTokens(summaryBlock)
	for len(historyTurns) > 0 && historyTokens > historyLimit {
		dropped := historyTurns[0]
		historyTurns = historyTurns[1:]
		historyTokens -= EstimateTokens(dropped.Role) + EstimateTokens(dropped.Content)
		trace.Evictions = append(trace.Evictions, fmt.Sprintf("history:%s", truncateForTrace(dropped.Content)))
	}

	if systemPrompt.Len() > 0 {
		messages = append(messages, ollama.Message{Role: ollama.RoleSystem, Content: systemPrompt.String()})
	}
	if summaryBlock != "" {
		messages = append(messages, ollama.Message{Role: ollama.RoleSystem, Content: summaryBlock})
	}
	for _, t := range historyTurns {
		messages = append(messages, ollama.Message{Role: t.Role, Content: t.Content})
	}
	messages = append(messages, sourceMessages...)

	used := 0
	for _, msg := range messages {
		used += EstimateMessageTokens(msg)
	}
	trace.UsedTokens = used
	if budget.Total > 0 {
		trace.FillPercent = used * 100 / budget.Total
		if trace.FillPercent > 100 {
			trace.FillPercent = 100
		}
	}

	for _, g := range guardrails {
		var err error
		messages, err = g.Apply(ctx, messages, trace)
		if err != nil {
			return nil, nil, fmt.Errorf("apply guardrail: %w", err)
		}
	}

	return messages, trace, nil
}

func applySourceBudget(items []ContextItem, liveBudget int, trace *PromptTrace) ([]ContextItem, []ollama.Message) {
	if len(items) == 0 {
		return nil, nil
	}

	kept := append([]ContextItem(nil), items...)
	for estimateContextItemsTokens(kept) > liveBudget {
		dropIdx := indexOfEvictableLowPriorityItem(kept)
		if dropIdx < 0 {
			break
		}
		dropped := kept[dropIdx]
		kept = append(kept[:dropIdx], kept[dropIdx+1:]...)
		trace.Evictions = append(trace.Evictions, fmt.Sprintf("source:%s", dropped.Source))
	}

	nonSystem := make([]struct {
		idx int
		msg ollama.Message
	}, 0, len(kept))
	for i, item := range kept {
		role := item.Role
		if role == "" {
			role = ollama.RoleSystem
		}
		if role == ollama.RoleSystem {
			continue
		}
		nonSystem = append(nonSystem, struct {
			idx int
			msg ollama.Message
		}{idx: i, msg: ollama.Message{Role: role, Content: item.Body}})
	}
	sort.Slice(nonSystem, func(i, j int) bool { return nonSystem[i].idx < nonSystem[j].idx })
	messages := make([]ollama.Message, 0, len(nonSystem))
	for _, it := range nonSystem {
		messages = append(messages, it.msg)
	}
	return kept, messages
}

func estimateContextItemsTokens(items []ContextItem) int {
	total := 0
	for _, item := range items {
		tokens := item.Tokens
		if tokens <= 0 {
			tokens = EstimateTokens(item.Body)
		}
		total += tokens
	}
	return total
}

func indexOfEvictableLowPriorityItem(items []ContextItem) int {
	idx := -1
	for i := range items {
		if items[i].PinKey == "system_prompt" {
			continue
		}
		if idx == -1 || items[i].Priority < items[idx].Priority {
			idx = i
		}
	}
	return idx
}

func truncateForTrace(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 60 {
		return s[:60] + "..."
	}
	return s
}
