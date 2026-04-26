package views

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/dan-solli/teaforge/internal/memory"
	"github.com/dan-solli/teaforge/internal/tui/styles"
	"github.com/dan-solli/teaforge/internal/treesitter"
)

// -------------------------------------------------------------------
// MemoryView
// -------------------------------------------------------------------

// MemoryTab selects which memory panel is displayed.
type MemoryTab int

const (
	MemoryTabSession MemoryTab = iota
	MemoryTabProject
	MemoryTabCode
)

// MemoryView renders all three memory layers: session, project and code.
type MemoryView struct {
	width, height int
	activeTab     MemoryTab
	session       *memory.SessionMemory
	project       *memory.ProjectMemory
	code          *treesitter.CodeMemory
	codeQuery     string
	scrollOffset  int
}

// NewMemoryView creates a MemoryView backed by the provided memory stores.
func NewMemoryView(s *memory.SessionMemory, p *memory.ProjectMemory, c *treesitter.CodeMemory) MemoryView {
	return MemoryView{
		session: s,
		project: p,
		code:    c,
	}
}

// SetSize updates the view dimensions.
func (m *MemoryView) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// NextTab cycles to the next memory tab.
func (m *MemoryView) NextTab() {
	m.activeTab = (m.activeTab + 1) % 3
	m.scrollOffset = 0
}

// PrevTab cycles to the previous memory tab.
func (m *MemoryView) PrevTab() {
	m.activeTab = (m.activeTab + 2) % 3
	m.scrollOffset = 0
}

// ScrollDown scrolls the content down.
func (m *MemoryView) ScrollDown() {
	m.scrollOffset++
}

// ScrollUp scrolls the content up.
func (m *MemoryView) ScrollUp() {
	if m.scrollOffset > 0 {
		m.scrollOffset--
	}
}

// SetCodeQuery sets the search query for the code memory tab.
func (m *MemoryView) SetCodeQuery(q string) {
	m.codeQuery = q
	m.scrollOffset = 0
}

// CodeQuery returns the current code search query.
func (m *MemoryView) CodeQuery() string { return m.codeQuery }

// View renders the memory panel.
func (m *MemoryView) View() string {
	tabBar := m.renderTabBar()
	content := m.renderContent()

	inner := lipgloss.JoinVertical(lipgloss.Left, tabBar, content)
	return styles.Panel.
		Width(m.width - 2).
		Height(m.height - 2).
		Render(inner)
}

func (m *MemoryView) renderTabBar() string {
	tabs := []string{"Session", "Project", "Code"}
	var rendered []string
	for i, t := range tabs {
		if MemoryTab(i) == m.activeTab {
			rendered = append(rendered, styles.TabActive.Render(t))
		} else {
			rendered = append(rendered, styles.TabInactive.Render(t))
		}
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, rendered...)
}

func (m *MemoryView) renderContent() string {
	maxH := m.height - 6
	if maxH < 2 {
		maxH = 2
	}

	var lines []string
	switch m.activeTab {
	case MemoryTabSession:
		lines = m.sessionLines()
	case MemoryTabProject:
		lines = m.projectLines()
	case MemoryTabCode:
		lines = m.codeLines()
	}

	// Apply scroll
	if m.scrollOffset >= len(lines) && len(lines) > 0 {
		m.scrollOffset = len(lines) - 1
	}
	if m.scrollOffset < len(lines) {
		lines = lines[m.scrollOffset:]
	}
	if len(lines) > maxH {
		lines = lines[:maxH]
	}

	return strings.Join(lines, "\n")
}

func (m *MemoryView) sessionLines() []string {
	turns := m.session.Turns()
	ctx := m.session.ContextMap()

	var lines []string
	lines = append(lines, styles.AILabel.Render(fmt.Sprintf("Session: %d turns", len(turns))))

	if len(ctx) > 0 {
		lines = append(lines, styles.MutedText.Render("Context:"))
		for k, v := range ctx {
			lines = append(lines, fmt.Sprintf("  %s = %s", styles.UserLabel.Render(k), v))
		}
	}

	lines = append(lines, "")
	for i, t := range turns {
		ts := t.Timestamp.Format("15:04:05")
		var roleStr string
		switch t.Role {
		case "user":
			roleStr = styles.UserLabel.Render("You")
		case "assistant":
			roleStr = styles.AILabel.Render("AI")
		default:
			roleStr = styles.ToolLabel.Render(t.Role)
		}
		preview := t.Content
		if len(preview) > 80 {
			preview = preview[:77] + "..."
		}
		lines = append(lines, fmt.Sprintf("%d. %s %s: %s", i+1, roleStr, styles.MutedText.Render(ts), preview))
	}
	return lines
}

func (m *MemoryView) projectLines() []string {
	notes := m.project.Notes()
	var lines []string
	lines = append(lines, styles.AILabel.Render(fmt.Sprintf("Project Notes: %d entries", len(notes))))
	lines = append(lines, "")

	if len(notes) == 0 {
		lines = append(lines, styles.MutedText.Render("No project notes yet. Ask the AI to save notes."))
		return lines
	}

	// Group by category
	cats := make(map[string][]memory.Note)
	var catOrder []string
	for _, n := range notes {
		if _, seen := cats[n.Category]; !seen {
			catOrder = append(catOrder, n.Category)
		}
		cats[n.Category] = append(cats[n.Category], n)
	}

	for _, cat := range catOrder {
		lines = append(lines, styles.ToolLabel.Render("["+cat+"]"))
		for _, n := range cats[cat] {
			ts := n.UpdatedAt.Format("2006-01-02 15:04")
			lines = append(lines, fmt.Sprintf("  • %s  %s", n.Content, styles.MutedText.Render(ts)))
		}
		lines = append(lines, "")
	}
	return lines
}

func (m *MemoryView) codeLines() []string {
	var lines []string

	var symbols []treesitter.Symbol
	if m.codeQuery != "" {
		symbols = m.code.Search(m.codeQuery)
		lines = append(lines, styles.AILabel.Render(
			fmt.Sprintf("Code search: %q → %d results", m.codeQuery, len(symbols))))
	} else {
		symbols = m.code.AllSymbols()
		files := m.code.Files()
		lines = append(lines, styles.AILabel.Render(
			fmt.Sprintf("Code index: %d files, %d symbols", len(files), len(symbols))))
		lines = append(lines, styles.MutedText.Render("Type / to search symbols"))
	}

	lines = append(lines, "")
	if len(symbols) == 0 {
		if m.codeQuery != "" {
			lines = append(lines, styles.MutedText.Render("No matches found."))
		} else {
			lines = append(lines, styles.MutedText.Render("No symbols indexed. Ask AI to index a directory."))
		}
		return lines
	}

	for _, s := range symbols {
		kindStyle := styles.MutedText
		switch s.Kind {
		case "function":
			kindStyle = styles.UserLabel
		case "type":
			kindStyle = styles.AILabel
		case "const", "var":
			kindStyle = styles.ToolLabel
		}
		loc := styles.MutedText.Render(fmt.Sprintf("%s:%d", shortenPath(s.File), s.Line))
		lines = append(lines, fmt.Sprintf("  %s %s %s",
			kindStyle.Render(s.Kind), s.Name, loc))
	}
	return lines
}

func shortenPath(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) <= 3 {
		return path
	}
	return ".../" + strings.Join(parts[len(parts)-2:], "/")
}
