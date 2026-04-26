// Package views provides the individual screen views for the TeaForge TUI.
package views

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
	"github.com/dan-solli/teaforge/internal/tui/styles"
)

// -------------------------------------------------------------------
// Chat entry (one rendered line / block in the chat history)
// -------------------------------------------------------------------

// ChatEntry represents a single message in the chat view.
type ChatEntry struct {
	Role      string
	Content   string
	Timestamp time.Time
	IsPartial bool // true while the AI is still streaming
}

// -------------------------------------------------------------------
// ChatView
// -------------------------------------------------------------------

// ChatView renders the conversation and the input text area.
type ChatView struct {
	width, height int
	viewport      viewport.Model
	textarea      textarea.Model
	entries       []ChatEntry
	partial       string // currently-streaming assistant content
	thinking      bool   // agent is running
	thinkDots     int    // animation counter
}

// NewChatView creates and initialises a ChatView.
func NewChatView() ChatView {
	ta := textarea.New()
	ta.Placeholder = "Type your message... (Enter to send, Ctrl+S to submit multi-line)"
	ta.Focus()
	ta.CharLimit = 4096
	ta.ShowLineNumbers = false
	ta.SetHeight(3)

	vp := viewport.New(80, 20)
	vp.SetContent("")

	return ChatView{
		viewport: vp,
		textarea: ta,
	}
}

// SetSize updates the view dimensions.
func (c *ChatView) SetSize(w, h int) {
	c.width = w
	c.height = h
	inputH := 5
	c.viewport.Width = w - 2
	c.viewport.Height = h - inputH - 4
	c.textarea.SetWidth(w - 4)
	c.rebuildViewport()
}

// AddEntry appends a completed message to the chat history.
func (c *ChatView) AddEntry(role, content string) {
	c.entries = append(c.entries, ChatEntry{
		Role:      role,
		Content:   content,
		Timestamp: time.Now(),
	})
	c.partial = ""
	c.rebuildViewport()
	c.viewport.GotoBottom()
}

// AppendPartial extends the currently-streaming assistant message.
func (c *ChatView) AppendPartial(chunk string) {
	c.partial += chunk
	c.rebuildViewport()
	c.viewport.GotoBottom()
}

// AddToolEvent adds a tool call or result line to the chat.
func (c *ChatView) AddToolEvent(eventType, content string) {
	c.entries = append(c.entries, ChatEntry{
		Role:      eventType,
		Content:   content,
		Timestamp: time.Now(),
	})
	c.rebuildViewport()
	c.viewport.GotoBottom()
}

// SetThinking controls the "thinking" spinner state.
func (c *ChatView) SetThinking(t bool) {
	c.thinking = t
	if !t {
		c.thinkDots = 0
	}
}

// TickThinking advances the thinking animation.
func (c *ChatView) TickThinking() {
	c.thinkDots = (c.thinkDots + 1) % 4
}

// TextareaValue returns the current textarea content.
func (c *ChatView) TextareaValue() string {
	return c.textarea.Value()
}

// ClearTextarea resets the textarea.
func (c *ChatView) ClearTextarea() {
	c.textarea.Reset()
}

// Focused returns whether the textarea is focused.
func (c *ChatView) Focused() bool {
	return c.textarea.Focused()
}

// FocusTextarea focuses the input.
func (c *ChatView) FocusTextarea() {
	c.textarea.Focus()
}

// BlurTextarea removes focus from the input.
func (c *ChatView) BlurTextarea() {
	c.textarea.Blur()
}

// Viewport returns the viewport so the caller can forward scroll events.
func (c *ChatView) Viewport() *viewport.Model {
	return &c.viewport
}

// Textarea returns the textarea so the caller can forward key events.
func (c *ChatView) Textarea() *textarea.Model {
	return &c.textarea
}

// View renders the complete chat view as a string.
func (c *ChatView) View() string {
	var parts []string

	// Chat history viewport
	vp := styles.PanelActive.
		Width(c.width - 2).
		Height(c.viewport.Height + 2).
		Render(c.viewport.View())
	parts = append(parts, vp)

	// Thinking indicator
	if c.thinking {
		dots := strings.Repeat(".", c.thinkDots+1)
		indicator := styles.MutedText.Render(fmt.Sprintf(" ⟳ Thinking%s", dots))
		parts = append(parts, indicator)
	}

	// Input box
	inputLabel := styles.MutedText.Render("Message:")
	input := styles.InputStyle.Width(c.width - 4).Render(c.textarea.View())
	parts = append(parts, inputLabel)
	parts = append(parts, input)

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (c *ChatView) rebuildViewport() {
	var sb strings.Builder
	for _, e := range c.entries {
		sb.WriteString(c.renderEntry(e))
		sb.WriteString("\n")
	}
	if c.partial != "" {
		entry := ChatEntry{
			Role:      "assistant",
			Content:   c.partial,
			IsPartial: true,
		}
		sb.WriteString(c.renderEntry(entry))
	}
	c.viewport.SetContent(sb.String())
}

func (c *ChatView) renderEntry(e ChatEntry) string {
	ts := styles.MutedText.Render(e.Timestamp.Format("15:04:05"))
	var label string
	var content string

	switch e.Role {
	case "user":
		label = styles.UserLabel.Render("You")
		content = e.Content
	case "assistant":
		label = styles.AILabel.Render("TeaForge")
		content = e.Content
		if e.IsPartial {
			content += "▌"
		}
	case "tool_call":
		label = styles.ToolLabel.Render("⚙ Tool")
		content = styles.MutedText.Render("calling: " + e.Content)
	case "tool_result":
		label = styles.ToolLabel.Render("⚙ Result")
		content = styles.MutedText.Render(e.Content)
	case "error":
		label = styles.ErrorText.Render("✗ Error")
		content = styles.ErrorText.Render(e.Content)
	default:
		label = styles.MutedText.Render(e.Role)
		content = e.Content
	}

	header := fmt.Sprintf("%s %s", label, ts)
	maxWidth := c.viewport.Width
	if maxWidth < 10 {
		maxWidth = 80
	}

	// Wrap content lines
	var lines []string
	for _, line := range strings.Split(content, "\n") {
		if len(line) > maxWidth {
			for len(line) > maxWidth {
				lines = append(lines, line[:maxWidth])
				line = line[maxWidth:]
			}
		}
		lines = append(lines, line)
	}

	return header + "\n  " + strings.Join(lines, "\n  ") + "\n"
}
