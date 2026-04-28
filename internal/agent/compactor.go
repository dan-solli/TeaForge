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

const compactionSectionSize = 24

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
	sections := splitTurnsIntoSections(older, compactionSectionSize)
	if len(sections) <= 1 {
		summary := c.summarizeTranscript(ctx, renderTurnsForCompaction(older))
		if summary == "" {
			summary = fallbackCompactionSummary(older)
		}
		return summary, kept, nil
	}

	sectionSummaries := make([]string, 0, len(sections))
	for i, section := range sections {
		transcript := renderTurnsForCompaction(section)
		headered := fmt.Sprintf("Section %d/%d\n%s", i+1, len(sections), transcript)
		summary := c.summarizeTranscript(ctx, headered)
		if summary == "" {
			summary = fallbackCompactionSummary(section)
		}
		sectionSummaries = append(sectionSummaries, fmt.Sprintf("Section %d:\n%s", i+1, strings.TrimSpace(summary)))
	}

	synthesisPrompt := "Synthesize these section summaries into one concise bullet list. Preserve concrete file names, decisions, open questions, and unresolved risks.\n\n" + strings.Join(sectionSummaries, "\n\n")
	final := c.summarizeTranscript(ctx, synthesisPrompt)
	if strings.TrimSpace(final) == "" {
		final = strings.Join(sectionSummaries, "\n\n")
	}
	return strings.TrimSpace(final), kept, nil
}

func (c *llmCompactor) summarizeTranscript(ctx context.Context, transcript string) string {
	userPrompt, err := prompt.RenderTemplate("compactor_user_prompt.tmpl", struct {
		Transcript string
	}{Transcript: transcript})
	if err != nil {
		return ""
	}
	chatReq := ollama.ChatRequest{
		Model: c.model,
		Messages: []ollama.Message{
			{Role: ollama.RoleSystem, Content: compactorSystemPrompt},
			{Role: ollama.RoleUser, Content: userPrompt},
		},
		Options: &ollama.Options{Temperature: 0.1, NumCtx: c.numCtx},
	}
	resp, err := c.client.Chat(ctx, chatReq)
	if err != nil || resp == nil {
		return ""
	}
	return strings.TrimSpace(resp.Message.Content)
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

func splitTurnsIntoSections(turns []memory.Turn, sectionSize int) [][]memory.Turn {
	if len(turns) == 0 {
		return nil
	}
	if sectionSize <= 0 {
		sectionSize = compactionSectionSize
	}
	sections := make([][]memory.Turn, 0, (len(turns)+sectionSize-1)/sectionSize)
	for start := 0; start < len(turns); start += sectionSize {
		end := start + sectionSize
		if end > len(turns) {
			end = len(turns)
		}
		sections = append(sections, turns[start:end])
	}
	return sections
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
