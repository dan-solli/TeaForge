// Package tui implements the Bubble Tea TUI for TeaForge.
package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dan-solli/teaforge/internal/agent"
	"github.com/dan-solli/teaforge/internal/tui/styles"
	"github.com/dan-solli/teaforge/internal/tui/views"
)

// -------------------------------------------------------------------
// Messages
// -------------------------------------------------------------------

// agentEventMsg carries a single agent event into the Bubble Tea event loop.
type agentEventMsg agent.Event

// tickMsg drives the thinking animation.
// type tickMsg time.Time

// indexDoneMsg signals that the background code index is complete.
type indexDoneMsg struct{ err error }

// modelListMsg delivers the available Ollama models.
type modelListMsg struct {
	models []string
	err    error
}

// agentDoneMsg signals that the agent goroutine has closed its channel.
type agentDoneMsg struct{}

// -------------------------------------------------------------------
// View identifiers
// -------------------------------------------------------------------

type viewID int

const (
	viewChat viewID = iota
	viewFiles
	viewMemory
	viewModels
)

// -------------------------------------------------------------------
// Key bindings
// -------------------------------------------------------------------

type keyMap struct {
	Chat       key.Binding
	Files      key.Binding
	Memory     key.Binding
	Models     key.Binding
	Send       key.Binding
	Quit       key.Binding
	Up         key.Binding
	Down       key.Binding
	Toggle     key.Binding
	NextTab    key.Binding
	PrevTab    key.Binding
	Resume     key.Binding
	Reindex    key.Binding
	NewSession key.Binding
}

var defaultKeys = keyMap{
	Chat:       key.NewBinding(key.WithKeys("ctrl+1"), key.WithHelp("ctrl+1", "chat")),
	Files:      key.NewBinding(key.WithKeys("ctrl+2"), key.WithHelp("ctrl+2", "files")),
	Memory:     key.NewBinding(key.WithKeys("ctrl+3"), key.WithHelp("ctrl+3", "memory")),
	Models:     key.NewBinding(key.WithKeys("ctrl+4"), key.WithHelp("ctrl+4", "models")),
	Send:       key.NewBinding(key.WithKeys("ctrl+s"), key.WithHelp("ctrl+s", "send")),
	Quit:       key.NewBinding(key.WithKeys("ctrl+c", "ctrl+q"), key.WithHelp("ctrl+c", "quit")),
	Up:         key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
	Down:       key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
	Toggle:     key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select")),
	NextTab:    key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next tab")),
	PrevTab:    key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "prev tab")),
	Resume:     key.NewBinding(key.WithKeys("ctrl+r"), key.WithHelp("ctrl+r", "resume")),
	Reindex:    key.NewBinding(key.WithKeys("ctrl+shift+r"), key.WithHelp("ctrl+shift+r", "reindex")),
	NewSession: key.NewBinding(key.WithKeys("ctrl+n"), key.WithHelp("ctrl+n", "new session")),
}

// -------------------------------------------------------------------
// App – the root Bubble Tea model
// -------------------------------------------------------------------

// App is the root Bubble Tea model for TeaForge.
type App struct {
	keys              keyMap
	width             int
	height            int
	activeView        viewID
	ag                *agent.Agent
	cfg               agent.Config
	chatView          views.ChatView
	filesView         views.FilesView
	memoryView        views.MemoryView
	sp                spinner.Model
	searchInput       textinput.Model
	searchMode        bool
	models            []string
	modelCursor       int
	statusMsg         string
	thinking          bool
	agentCancel       context.CancelFunc
	agentEvents       chan agent.Event
	sessionPickerOpen bool
	sessionFiles      []string
	sessionCursor     int
	// files selected in Files view to be attached to the next user turn
	pendingAttachments []string
	// accumulates the current assistant response while streaming
	currentResponse *strings.Builder
}

// NewApp creates the root App model.
func NewApp(cfg agent.Config, a *agent.Agent) App {
	sp := spinner.New()
	sp.Spinner = spinner.Dot

	si := textinput.New()
	si.Placeholder = "Search code symbols..."
	si.CharLimit = 128

	workDir := cfg.WorkDir
	if workDir == "" {
		workDir = "."
	}

	return App{
		keys:            defaultKeys,
		cfg:             cfg,
		ag:              a,
		activeView:      viewChat,
		chatView:        views.NewChatView(),
		filesView:       views.NewFilesView(workDir),
		memoryView:      views.NewMemoryView(a.Session(), a.Project(), a.Code()),
		sp:              sp,
		searchInput:     si,
		currentResponse: &strings.Builder{},
		statusMsg:       fmt.Sprintf("Ready • model: %s", cfg.Model),
	}
}

// -------------------------------------------------------------------
// Init
// -------------------------------------------------------------------

// Init is called once when the program starts.
func (a App) Init() tea.Cmd {
	return tea.Batch(
		a.sp.Tick,
		fetchModelsCmd(a.cfg.OllamaURL),
		indexWorkDirCmd(a.ag),
	)
}

func fetchModelsCmd(ollamaURL string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		models, err := fetchOllamaModels(ctx, ollamaURL)
		return modelListMsg{models: models, err: err}
	}
}

func indexWorkDirCmd(ag *agent.Agent) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()
		err := ag.IndexWorkDir(ctx)
		return indexDoneMsg{err: err}
	}
}

// -------------------------------------------------------------------
// Update
// -------------------------------------------------------------------

// Update handles all incoming messages and key events.
func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		a.updateSizes()

	case tea.KeyMsg:
		if a.searchMode {
			return a.updateSearchMode(msg)
		}
		return a.updateKeys(msg)

	case agentEventMsg:
		cmd := a.handleAgentEvent(agent.Event(msg))
		if cmd != nil {
			cmds = append(cmds, cmd)
		}

	case agentDoneMsg:
		a.thinking = false
		a.chatView.SetThinking(false)
		// Finalize the streamed response
		resp := a.currentResponse.String()
		a.currentResponse.Reset()
		if resp != "" {
			a.chatView.AddEntry("assistant", resp)
			// Log the assistant response — this path is a safety net for the case
			// where the agent channel closes without a clean EventDone.
			_ = a.ag.AppendSessionLog("assistant", resp)
		}

	case spinner.TickMsg:
		var cmd tea.Cmd
		a.sp, cmd = a.sp.Update(msg)
		if a.thinking {
			a.chatView.TickThinking()
		}
		cmds = append(cmds, cmd)

	case indexDoneMsg:
		if msg.err != nil {
			a.statusMsg = "Index error: " + msg.err.Error()
		} else {
			files := a.ag.Code().Files()
			symbols := a.ag.Code().AllSymbols()
			a.statusMsg = fmt.Sprintf("Indexed %d files • %d symbols • model: %s",
				len(files), len(symbols), a.cfg.Model)
		}

	case modelListMsg:
		if msg.err == nil {
			a.models = msg.models
		} else {
			a.statusMsg = "Ollama: " + msg.err.Error()
		}
	}

	// Forward events to active sub-view components
	var subCmd tea.Cmd
	switch a.activeView {
	case viewChat:
		ta := a.chatView.Textarea()
		*ta, subCmd = ta.Update(msg)
		vp := a.chatView.Viewport()
		*vp, _ = vp.Update(msg)
	}
	if subCmd != nil {
		cmds = append(cmds, subCmd)
	}

	return a, tea.Batch(cmds...)
}

func (a App) updateKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	if a.sessionPickerOpen {
		switch {
		case key.Matches(msg, a.keys.Quit):
			if a.agentCancel != nil {
				a.agentCancel()
			}
			return a, tea.Quit
		case msg.String() == "esc":
			a.sessionPickerOpen = false
			return a, nil
		case key.Matches(msg, a.keys.Up):
			if a.sessionCursor > 0 {
				a.sessionCursor--
			}
			return a, nil
		case key.Matches(msg, a.keys.Down):
			if a.sessionCursor < len(a.sessionFiles)-1 {
				a.sessionCursor++
			}
			return a, nil
		case key.Matches(msg, a.keys.Toggle):
			if err := a.resumeSelectedSession(); err != nil {
				a.statusMsg = "Resume error: " + err.Error()
			} else {
				a.statusMsg = "Session resumed"
			}
			a.sessionPickerOpen = false
			a.activeView = viewChat
			a.chatView.FocusTextarea()
			return a, nil
		default:
			return a, nil
		}
	}

	switch {
	case key.Matches(msg, a.keys.Quit):
		if a.agentCancel != nil {
			a.agentCancel()
		}
		return a, tea.Quit

	case key.Matches(msg, a.keys.Chat):
		a.activeView = viewChat
		a.chatView.FocusTextarea()

	case key.Matches(msg, a.keys.Files):
		a.activeView = viewFiles
		a.chatView.BlurTextarea()

	case key.Matches(msg, a.keys.Memory):
		a.activeView = viewMemory
		a.chatView.BlurTextarea()

	case key.Matches(msg, a.keys.Models):
		a.activeView = viewModels
		a.chatView.BlurTextarea()

	case key.Matches(msg, a.keys.NewSession):
		a.ag.ResetSession()
		a.chatView = views.NewChatView()
		a.pendingAttachments = nil
		a.updateSizes()
		a.statusMsg = "New session started"

	case key.Matches(msg, a.keys.Send):
		if a.activeView == viewChat && !a.thinking {
			text := strings.TrimSpace(a.chatView.TextareaValue())
			if text != "" {
				attachments := a.consumePendingAttachments()
				a.chatView.ClearTextarea()
				cmd := a.startAgentRun(text, attachments)
				if cmd != nil {
					cmds = append(cmds, cmd)
				}
			}
		}

	case a.activeView != viewChat && key.Matches(msg, a.keys.Up):
		switch a.activeView {
		case viewFiles:
			a.filesView.MoveUp()
		case viewMemory:
			a.memoryView.ScrollUp()
		case viewModels:
			if a.modelCursor > 0 {
				a.modelCursor--
			}
		}

	case a.activeView != viewChat && key.Matches(msg, a.keys.Down):
		switch a.activeView {
		case viewFiles:
			a.filesView.MoveDown()
		case viewMemory:
			a.memoryView.ScrollDown()
		case viewModels:
			if a.modelCursor < len(a.models)-1 {
				a.modelCursor++
			}
		}

	case key.Matches(msg, a.keys.Toggle):
		switch a.activeView {
		case viewFiles:
			path := a.filesView.Toggle()
			if path != "" {
				a.addPendingAttachment(path)
				a.activeView = viewChat
				a.chatView.FocusTextarea()
				a.statusMsg = fmt.Sprintf("Attached file for next turn: %s", path)
			}
		case viewModels:
			if a.modelCursor < len(a.models) {
				a.cfg.Model = a.models[a.modelCursor]
				a.statusMsg = fmt.Sprintf("Model switched to: %s", a.cfg.Model)
			}
		case viewChat:
			text := strings.TrimSpace(a.chatView.TextareaValue())
			if text != "" && !strings.Contains(text, "\n") && !a.thinking {
				attachments := a.consumePendingAttachments()
				a.chatView.ClearTextarea()
				cmd := a.startAgentRun(text, attachments)
				if cmd != nil {
					cmds = append(cmds, cmd)
				}
			}
		}

	case key.Matches(msg, a.keys.NextTab):
		if a.activeView == viewMemory {
			a.memoryView.NextTab()
		}

	case key.Matches(msg, a.keys.PrevTab):
		if a.activeView == viewMemory {
			a.memoryView.PrevTab()
		}

	case key.Matches(msg, a.keys.Resume):
		if err := a.openSessionPicker(); err != nil {
			a.statusMsg = "Resume error: " + err.Error()
		}

	case key.Matches(msg, a.keys.Reindex):
		cmds = append(cmds, indexWorkDirCmd(a.ag))

	default:
		if a.activeView == viewChat {
			ta := a.chatView.Textarea()
			var cmd tea.Cmd
			*ta, cmd = ta.Update(msg)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
		if a.activeView == viewMemory && msg.String() == "/" {
			a.searchMode = true
			a.searchInput.SetValue("")
			a.searchInput.Focus()
		}
	}

	return a, tea.Batch(cmds...)
}

func (a App) updateSearchMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+c":
		a.searchMode = false
		a.searchInput.Blur()
		a.memoryView.SetCodeQuery("")
	case "enter":
		a.searchMode = false
		a.searchInput.Blur()
		a.memoryView.SetCodeQuery(a.searchInput.Value())
	default:
		var cmd tea.Cmd
		a.searchInput, cmd = a.searchInput.Update(msg)
		a.memoryView.SetCodeQuery(a.searchInput.Value())
		return a, cmd
	}
	return a, nil
}

// -------------------------------------------------------------------
// Agent execution
// -------------------------------------------------------------------

// startAgentRun kicks off the agent loop for userMsg. It returns a Cmd
// that reads the first event from the agent channel.
func (a *App) startAgentRun(userMsg string, attachments []string) tea.Cmd {
	if a.thinking {
		return nil
	}
	a.thinking = true
	a.chatView.SetThinking(true)
	a.chatView.AddEntry("user", userMsg)
	if len(attachments) > 0 {
		a.chatView.AddToolEvent("tool_call", fmt.Sprintf("attaching %d file(s)", len(attachments)))
	}
	a.currentResponse.Reset()

	ctx, cancel := context.WithCancel(context.Background())
	a.agentCancel = cancel

	events := make(chan agent.Event, 64)
	a.agentEvents = events

	go a.ag.Run(ctx, userMsg, attachments, events)

	return waitForAgentEvent(events)
}

// waitForAgentEvent is a Cmd factory that waits for the next event from the
// agent channel and returns it as a tea.Msg.
func waitForAgentEvent(events chan agent.Event) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-events
		if !ok {
			return agentDoneMsg{}
		}
		return agentEventMsg(ev)
	}
}

// handleAgentEvent processes a single agent event and returns a Cmd that
// will read the next event (or nil when done).
func (a *App) handleAgentEvent(ev agent.Event) tea.Cmd {
	switch ev.Type {
	case agent.EventToken:
		a.currentResponse.WriteString(ev.Content)
		a.chatView.AppendPartial(ev.Content)

	case agent.EventToolCall:
		a.chatView.AddToolEvent("tool_call", ev.Content)

	case agent.EventToolResult:
		a.chatView.AddToolEvent("tool_result", ev.Content)

	case agent.EventContext:
		a.statusMsg = fmt.Sprintf("Context %s • model: %s", ev.Content, a.cfg.Model)

	case agent.EventDone:
		a.thinking = false
		a.chatView.SetThinking(false)
		resp := a.currentResponse.String()
		a.currentResponse.Reset()
		if resp != "" {
			a.chatView.AddEntry("assistant", resp)
			// Log the assembled response — this is the definitive version:
			// exactly what was streamed token-by-token and displayed to the user.
			_ = a.ag.AppendSessionLog("assistant", resp)
		}
		if a.agentCancel != nil {
			a.agentCancel()
			a.agentCancel = nil
		}
		return nil

	case agent.EventError:
		a.chatView.AddToolEvent("error", ev.Content)
		a.thinking = false
		a.chatView.SetThinking(false)
		a.currentResponse.Reset()
		if a.agentCancel != nil {
			a.agentCancel()
			a.agentCancel = nil
		}
		return nil
	}

	// Continue reading from the agent channel
	if a.agentEvents != nil {
		return waitForAgentEvent(a.agentEvents)
	}
	return nil
}

// -------------------------------------------------------------------
// View
// -------------------------------------------------------------------

// View renders the full TUI.
func (a App) View() string {
	if a.width == 0 {
		return "Loading TeaForge..."
	}

	header := a.renderHeader()
	body := a.renderBody()
	statusBar := a.renderStatusBar()

	return lipgloss.JoinVertical(lipgloss.Left, header, body, statusBar)
}

func (a App) renderHeader() string {
	title := styles.TitleBar.Render(" ⚒  TeaForge ")

	type tabDef struct {
		id    viewID
		label string
		key   string
	}
	tabs := []tabDef{
		{viewChat, "Chat", "^1"},
		{viewFiles, "Files", "^2"},
		{viewMemory, "Memory", "^3"},
		{viewModels, "Models", "^4"},
	}

	var tabStrs []string
	for _, t := range tabs {
		label := fmt.Sprintf("%s %s", t.label, styles.MutedText.Render(t.key))
		if a.activeView == t.id {
			tabStrs = append(tabStrs, styles.TabActive.Render(label))
		} else {
			tabStrs = append(tabStrs, styles.TabInactive.Render(label))
		}
	}
	tabBar := lipgloss.JoinHorizontal(lipgloss.Top, tabStrs...)

	gap := a.width - lipgloss.Width(title) - lipgloss.Width(tabBar)
	if gap < 0 {
		gap = 0
	}
	return lipgloss.JoinHorizontal(lipgloss.Top,
		title,
		strings.Repeat(" ", gap),
		tabBar,
	)
}

func (a App) renderBody() string {
	bodyH := a.height - 3
	if bodyH < 1 {
		bodyH = 1
	}
	if a.sessionPickerOpen {
		return a.renderSessionPicker(bodyH)
	}

	switch a.activeView {
	case viewChat:
		chat := a.chatView.View()
		if len(a.pendingAttachments) == 0 {
			return chat
		}
		lines := []string{styles.MutedText.Render("Next turn attachments:")}
		for _, p := range a.pendingAttachments {
			lines = append(lines, "- "+styles.AILabel.Render(p))
		}
		attachments := styles.Panel.
			Width(a.width - 2).
			Render(strings.Join(lines, "\n"))
		return lipgloss.JoinVertical(lipgloss.Left, chat, attachments)
	case viewFiles:
		return a.filesView.View()
	case viewMemory:
		if a.searchMode {
			search := styles.InputStyle.
				Width(a.width - 4).
				Render("/ " + a.searchInput.View())
			return lipgloss.JoinVertical(lipgloss.Left, a.memoryView.View(), search)
		}
		return a.memoryView.View()
	case viewModels:
		return a.renderModelsView(bodyH)
	}
	return ""
}

func (a App) renderSessionPicker(h int) string {
	_ = h
	lines := []string{
		styles.AILabel.Render(fmt.Sprintf("Resume Session (%d)", len(a.sessionFiles))),
		styles.MutedText.Render("Enter: resume  Esc: cancel"),
		"",
	}
	if len(a.sessionFiles) == 0 {
		lines = append(lines, styles.ErrorText.Render("No sessions found."))
	} else {
		for i, p := range a.sessionFiles {
			name := strings.TrimSuffix(filepath.Base(p), ".json")
			line := "  " + name
			if i == a.sessionCursor {
				line = styles.ListItemSelected.Render(line)
			}
			lines = append(lines, line)
		}
	}
	content := strings.Join(lines, "\n")
	return styles.Panel.
		Width(a.width - 2).
		Height(a.height - 6).
		Render(content)
}

func (a App) renderModelsView(h int) string {
	_ = h
	var lines []string
	lines = append(lines, styles.AILabel.Render(fmt.Sprintf("Available Models (%d)", len(a.models))))
	lines = append(lines, styles.MutedText.Render("Press Enter to select, ↑↓ to navigate"))
	lines = append(lines, "")

	if len(a.models) == 0 {
		lines = append(lines, styles.ErrorText.Render("No models found. Is Ollama running?"))
		lines = append(lines, styles.MutedText.Render("Start Ollama with: ollama serve"))
	}

	for i, m := range a.models {
		label := m
		if m == a.cfg.Model {
			label += " " + styles.UserLabel.Render("(active)")
		}
		if i == a.modelCursor {
			lines = append(lines, styles.ListItemSelected.Render("  "+label))
		} else {
			lines = append(lines, "  "+label)
		}
	}

	content := strings.Join(lines, "\n")
	return styles.Panel.
		Width(a.width - 2).
		Render(content)
}

func (a App) renderStatusBar() string {
	if a.sessionPickerOpen {
		status := styles.StatusBar.Render("Select a session to resume")
		help := styles.StatusKey.Render("↑↓") + " navigate  " +
			styles.StatusKey.Render("enter") + " resume  " +
			styles.StatusKey.Render("esc") + " cancel"
		gap := a.width - lipgloss.Width(status) - lipgloss.Width(help)
		if gap < 0 {
			gap = 0
		}
		return lipgloss.JoinHorizontal(lipgloss.Top,
			status,
			strings.Repeat(" ", gap),
			help,
		)
	}

	status := styles.StatusBar.Render(a.statusMsg)
	var help string
	switch a.activeView {
	case viewChat:
		help = styles.StatusKey.Render("ctrl+s") + " send  " +
			styles.StatusKey.Render("ctrl+r") + " resume  " +
			styles.StatusKey.Render("ctrl+n") + " new session  " +
			styles.StatusKey.Render("ctrl+c") + " quit"
	case viewFiles:
		help = styles.StatusKey.Render("↑↓") + " navigate  " +
			styles.StatusKey.Render("enter") + " attach  " +
			styles.StatusKey.Render("ctrl+c") + " quit"
	case viewMemory:
		help = styles.StatusKey.Render("tab") + " next  " +
			styles.StatusKey.Render("/") + " search  " +
			styles.StatusKey.Render("ctrl+shift+r") + " reindex"
	case viewModels:
		help = styles.StatusKey.Render("↑↓") + " navigate  " +
			styles.StatusKey.Render("enter") + " select"
	}

	gap := a.width - lipgloss.Width(status) - lipgloss.Width(help)
	if gap < 0 {
		gap = 0
	}
	return lipgloss.JoinHorizontal(lipgloss.Top,
		status,
		strings.Repeat(" ", gap),
		help,
	)
}

func (a *App) addPendingAttachment(path string) {
	for _, p := range a.pendingAttachments {
		if p == path {
			return
		}
	}
	a.pendingAttachments = append(a.pendingAttachments, path)
}

func (a *App) consumePendingAttachments() []string {
	if len(a.pendingAttachments) == 0 {
		return nil
	}
	out := make([]string, len(a.pendingAttachments))
	copy(out, a.pendingAttachments)
	a.pendingAttachments = nil
	return out
}

// -------------------------------------------------------------------
// Size propagation
// -------------------------------------------------------------------

func (a *App) updateSizes() {
	bodyH := a.height - 3
	if bodyH < 4 {
		bodyH = 4
	}
	a.chatView.SetSize(a.width, bodyH)
	a.filesView.SetSize(a.width/3, bodyH)
	a.memoryView.SetSize(a.width, bodyH)
}

func (a *App) openSessionPicker() error {
	files, err := listSessionFiles(a.cfg.SessionsDir)
	if err != nil {
		return err
	}
	a.sessionFiles = files
	a.sessionCursor = 0
	a.sessionPickerOpen = true
	return nil
}

func (a *App) resumeSelectedSession() error {
	if len(a.sessionFiles) == 0 {
		return fmt.Errorf("no sessions available")
	}
	path := a.sessionFiles[a.sessionCursor]
	if err := a.ag.ResumeFromLog(path); err != nil {
		return err
	}
	a.chatView = views.NewChatView()
	a.pendingAttachments = nil
	a.updateSizes()
	a.hydrateChatFromSession()
	a.statusMsg = fmt.Sprintf("Resumed %s", strings.TrimSuffix(filepath.Base(path), ".json"))
	return nil
}

func (a *App) hydrateChatFromSession() {
	for _, t := range a.ag.Session().Turns() {
		switch t.Role {
		case "user", "assistant":
			a.chatView.AddEntry(t.Role, t.Content)
		case "tool":
			a.chatView.AddToolEvent("tool_result", t.Content)
		}
	}
}

func listSessionFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no sessions found")
		}
		return nil, err
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if filepath.Ext(e.Name()) == ".json" {
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no sessions found")
	}
	sort.Slice(out, func(i, j int) bool {
		return filepath.Base(out[i]) > filepath.Base(out[j])
	})
	return out, nil
}

// -------------------------------------------------------------------
// WorkDir helper
// -------------------------------------------------------------------

// WorkDir returns the configured working directory.
func (a App) WorkDir() string {
	return filepath.Clean(a.cfg.WorkDir)
}

// -------------------------------------------------------------------
// Ollama model list helper
// -------------------------------------------------------------------

// fetchOllamaModels retrieves model names from the Ollama REST API.
func fetchOllamaModels(ctx context.Context, baseURL string) ([]string, error) {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/tags", nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	names := make([]string, len(result.Models))
	for i, m := range result.Models {
		names[i] = m.Name
	}
	return names, nil
}
