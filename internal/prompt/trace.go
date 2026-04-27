package prompt

// PromptTrace captures source-level details for one assembled prompt.
type PromptTrace struct {
	Items       []ContextItem
	Evictions   []string
	Budget      TokenBudget
	UsedTokens  int
	FillPercent int
	Compacted   bool
	Summary     string
}

func NewPromptTrace() *PromptTrace {
	return &PromptTrace{}
}

func (t *PromptTrace) AddItem(item ContextItem) {
	if t == nil {
		return
	}
	t.Items = append(t.Items, item)
}
