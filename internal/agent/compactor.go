package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/dan-solli/teaforge/internal/memory"
	"github.com/dan-solli/teaforge/internal/ollama"
	"github.com/dan-solli/teaforge/internal/prompt"
)

type llmCompactor struct {
	client *ollama.Client
	model  string
	numCtx int
}

var compactorSystemPrompt = prompt.MustLoadTemplate("compactor_system_prompt.txt")

func newLLMCompactor(client *ollama.Client, model string, numCtx int) *llmCompactor {
	return &llmCompactor{client: client, model: model, numCtx: numCtx}
}

func (c *llmCompactor) ShouldCompact(turns []memory.Turn, available int) bool {
	if c == nil {
		return false
	}
	if len(turns) <= 12 {
		return false
	}
	return prompt.EstimateTurnsTokens(turns) > available
}

func (c *llmCompactor) Compact(ctx context.Context, turns []memory.Turn, keepLatest int) (string, []memory.Turn, error) {
	if c == nil || c.client == nil || len(turns) == 0 {
		return "", turns, nil
	}
	if keepLatest <= 0 {
		keepLatest = 12
	}
	if len(turns) <= keepLatest {
		return "", turns, nil
	}

	split := len(turns) - keepLatest
	older := turns[:split]
	kept := append([]memory.Turn(nil), turns[split:]...)

	transcript := renderTurnsForCompaction(older)
	userPrompt, err := prompt.RenderTemplate("compactor_user_prompt.tmpl", struct {
		Transcript string
	}{Transcript: transcript})
	if err != nil {
		return fallbackCompactionSummary(older), kept, nil
	}
	chatReq := ollama.ChatRequest{
		Model: c.model,
		Messages: []ollama.Message{
			{
				Role:    ollama.RoleSystem,
				Content: compactorSystemPrompt,
			},
			{
				Role:    ollama.RoleUser,
				Content: userPrompt,
			},
		},
		Options: &ollama.Options{Temperature: 0.1, NumCtx: c.numCtx},
	}

	resp, err := c.client.Chat(ctx, chatReq)
	if err != nil {
		return fallbackCompactionSummary(older), kept, nil
	}
	summary := strings.TrimSpace(resp.Message.Content)
	if summary == "" {
		summary = fallbackCompactionSummary(older)
	}
	return summary, kept, nil
}

func renderTurnsForCompaction(turns []memory.Turn) string {
	var sb strings.Builder
	for i, t := range turns {
		content := strings.TrimSpace(t.Content)
		if len(content) > 1200 {
			content = content[:1200] + "..."
		}
		sb.WriteString(fmt.Sprintf("[%d] %s: %s\n", i+1, t.Role, content))
	}
	return sb.String()
}

func fallbackCompactionSummary(turns []memory.Turn) string {
	type summaryLine struct {
		Role    string
		Content string
	}
	data := struct {
		HasTurns bool
		Lines    []summaryLine
		Omitted  bool
	}{
		HasTurns: len(turns) > 0,
		Lines:    make([]summaryLine, 0, minInt(len(turns), 12)),
	}
	for i, t := range turns {
		if i >= 12 {
			data.Omitted = true
			break
		}
		content := strings.TrimSpace(t.Content)
		if len(content) > 180 {
			content = content[:180] + "..."
		}
		data.Lines = append(data.Lines, summaryLine{Role: t.Role, Content: content})
	}

	rendered, err := prompt.RenderTemplate("compactor_fallback_summary.tmpl", data)
	if err != nil {
		return fallbackCompactionSummaryInline(turns)
	}
	return strings.TrimSpace(rendered)
}

func fallbackCompactionSummaryInline(turns []memory.Turn) string {
	if len(turns) == 0 {
		return "No prior turns to summarize."
	}
	var sb strings.Builder
	sb.WriteString("- Conversation summary fallback (LLM compaction unavailable):\n")
	for i, t := range turns {
		if i >= 12 {
			sb.WriteString("- ... additional turns omitted\n")
			break
		}
		content := strings.TrimSpace(t.Content)
		if len(content) > 180 {
			content = content[:180] + "..."
		}
		sb.WriteString(fmt.Sprintf("- %s: %s\n", t.Role, content))
	}
	return strings.TrimSpace(sb.String())
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
