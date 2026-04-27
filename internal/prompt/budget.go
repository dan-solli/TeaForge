package prompt

import (
	"math"
	"strings"
	"unicode/utf8"

	"github.com/dan-solli/teaforge/internal/memory"
	"github.com/dan-solli/teaforge/internal/ollama"
)

// TokenBudget reserves space for history, live context, and model response.
type TokenBudget struct {
	Total           int
	HistoryBudget   int
	LiveBudget      int
	ResponseReserve int
}

func DefaultTokenBudget(total int) TokenBudget {
	if total <= 0 {
		total = 8192
	}
	return TokenBudget{
		Total:           total,
		HistoryBudget:   total * 50 / 100,
		LiveBudget:      total * 20 / 100,
		ResponseReserve: total * 30 / 100,
	}
}

// EstimateTokens returns an approximate token count for text.
// It uses a conservative heuristic for code-like content and a lighter one for prose.
func EstimateTokens(text string) int {
	if text == "" {
		return 0
	}
	runes := utf8.RuneCountInString(text)
	if runes == 0 {
		return 0
	}

	if looksLikeCode(text) {
		return runes
	}

	tokens := int(math.Ceil(float64(runes) / 3.5))
	if tokens < 1 {
		return 1
	}
	return tokens
}

func EstimateMessageTokens(msg ollama.Message) int {
	total := EstimateTokens(msg.Role) + EstimateTokens(msg.Content)
	for _, tc := range msg.ToolCalls {
		total += EstimateTokens(tc.ID)
		total += EstimateTokens(tc.Function.Name)
	}
	total += EstimateTokens(msg.ToolCallID)
	return total
}

func EstimateTurnsTokens(turns []memory.Turn) int {
	total := 0
	for _, t := range turns {
		total += EstimateTokens(t.Role)
		total += EstimateTokens(t.Content)
	}
	return total
}

func looksLikeCode(text string) bool {
	if strings.Contains(text, "\n\t") || strings.ContainsAny(text, "{}[]();<>=`$") {
		return true
	}
	for _, marker := range []string{"func ", "package ", "import ", "class ", "def ", "return ", "</"} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}
