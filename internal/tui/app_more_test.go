package tui

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/dan-solli/teaforge/internal/agent"
	"github.com/dan-solli/teaforge/internal/tui/views"
)

func newTestAppWithURL(t *testing.T, ollamaURL string) App {
	t.Helper()
	dir := t.TempDir()
	cfg := agent.Config{
		Model:       "test-model",
		OllamaURL:   ollamaURL,
		WorkDir:     dir,
		MemoryFile:  filepath.Join(dir, "memory.json"),
		SessionsDir: filepath.Join(dir, ".teaforge", "sessions"),
		NumCtx:      1024,
	}
	ag, err := agent.New(cfg)
	if err != nil {
		t.Fatalf("agent.New: %v", err)
	}
	app := NewApp(cfg, ag)
	app.width = 120
	app.height = 40
	app.updateSizes()
	return app
}

func TestAppUpdate_MessageTypes(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	model, _ := app.Update(tea.WindowSizeMsg{Width: 100, Height: 32})
	a := model.(App)
	if a.width != 100 || a.height != 32 {
		t.Fatalf("window size not applied: %dx%d", a.width, a.height)
	}

	model, _ = a.Update(modelListMsg{models: []string{"m1", "m2"}})
	a = model.(App)
	if len(a.models) != 2 {
		t.Fatalf("models=%d want 2", len(a.models))
	}

	model, _ = a.Update(indexDoneMsg{err: nil})
	a = model.(App)
	if !strings.Contains(a.statusMsg, "Indexed") {
		t.Fatalf("status=%q", a.statusMsg)
	}

	model, _ = a.Update(indexDoneMsg{err: errors.New("boom")})
	a = model.(App)
	if !strings.Contains(a.statusMsg, "Index error") {
		t.Fatalf("status=%q", a.statusMsg)
	}

	a.thinking = true
	model, _ = a.Update(spinner.TickMsg{Time: time.Now()})
	a = model.(App)
	if !a.thinking {
		t.Fatal("expected thinking to remain true on spinner tick")
	}

	a.currentResponse.WriteString("assembled")
	model, _ = a.Update(agentDoneMsg{})
	a = model.(App)
	if a.thinking {
		t.Fatal("expected thinking false after agentDoneMsg")
	}
	if !strings.Contains(a.chatView.View(), "assembled") {
		t.Fatalf("expected assembled response in chat view")
	}
}

func TestAppUpdate_SearchMode(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	app.searchMode = true
	app.searchInput.Focus()

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("A")})
	a := model.(App)
	if !a.searchMode {
		t.Fatal("search mode should stay active while typing")
	}
	if a.searchInput.Value() == "" {
		t.Fatal("search input should update while typing")
	}

	model, _ = a.Update(tea.KeyMsg{Type: tea.KeyEnter})
	a = model.(App)
	if a.searchMode {
		t.Fatal("search mode should close on enter")
	}

	a.searchMode = true
	model, _ = a.Update(tea.KeyMsg{Type: tea.KeyEsc})
	a = model.(App)
	if a.searchMode {
		t.Fatal("search mode should close on esc")
	}
	if a.memoryView.CodeQuery() != "" {
		t.Fatalf("code query should reset on esc, got %q", a.memoryView.CodeQuery())
	}
}

func TestWaitForAgentEvent(t *testing.T) {
	t.Parallel()

	ch := make(chan agent.Event, 1)
	ch <- agent.Event{Type: agent.EventToken, Content: "x"}
	msg := waitForAgentEvent(ch)()
	if _, ok := msg.(agentEventMsg); !ok {
		t.Fatalf("unexpected msg type %T", msg)
	}

	close(ch)
	msg = waitForAgentEvent(ch)()
	if _, ok := msg.(agentDoneMsg); !ok {
		t.Fatalf("unexpected msg type %T", msg)
	}
}

func TestHandleAgentEventPaths(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	if cmd := app.handleAgentEvent(agent.Event{Type: agent.EventToken, Content: "hel"}); cmd != nil {
		t.Fatal("expected nil cmd without agentEvents channel")
	}
	if !strings.Contains(app.chatView.View(), "hel") {
		t.Fatal("expected partial token in chat view")
	}

	_ = app.handleAgentEvent(agent.Event{Type: agent.EventToolCall, Content: "read_file"})
	_ = app.handleAgentEvent(agent.Event{Type: agent.EventToolResult, Content: "[read_file] ok"})
	if !strings.Contains(app.chatView.View(), "Tool") {
		t.Fatal("expected tool events in chat view")
	}

	_ = app.handleAgentEvent(agent.Event{Type: agent.EventContext, Content: "55% (10/20 tokens)"})
	if !strings.Contains(app.statusMsg, "Context 55%") {
		t.Fatalf("status=%q", app.statusMsg)
	}

	app.thinking = true
	app.currentResponse.WriteString("done")
	if cmd := app.handleAgentEvent(agent.Event{Type: agent.EventDone}); cmd != nil {
		t.Fatal("expected nil cmd on done")
	}
	if app.thinking {
		t.Fatal("thinking should be false after done")
	}
	if !strings.Contains(app.chatView.View(), "done") {
		t.Fatal("assistant response should be committed on done")
	}

	app.thinking = true
	app.currentResponse.WriteString("temp")
	if cmd := app.handleAgentEvent(agent.Event{Type: agent.EventError, Content: "failed"}); cmd != nil {
		t.Fatal("expected nil cmd on error")
	}
	if app.thinking {
		t.Fatal("thinking should be false on error")
	}
	if app.currentResponse.Len() != 0 {
		t.Fatal("partial response should reset on error")
	}
}

func TestStartAgentRun(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Fatalf("path=%q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{\"model\":\"m\",\"message\":{\"role\":\"assistant\",\"content\":\"ok\"},\"done\":false}\n"))
		_, _ = w.Write([]byte("{\"model\":\"m\",\"message\":{\"role\":\"assistant\",\"content\":\"\"},\"done\":true}\n"))
	}))
	defer server.Close()

	app := newTestAppWithURL(t, server.URL)
	if app.startAgentRun("x", nil) == nil {
		t.Fatal("expected command to start agent run")
	}
	if !app.thinking {
		t.Fatal("app should enter thinking state")
	}

	app.thinking = true
	if app.startAgentRun("x", nil) != nil {
		t.Fatal("startAgentRun should no-op while already thinking")
	}
}

func TestAppRenderPaths(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	app.width = 0
	if got := app.View(); !strings.Contains(got, "Loading TeaForge") {
		t.Fatalf("View=%q", got)
	}

	app.width = 120
	app.height = 40
	app.updateSizes()
	app.pendingAttachments = []string{"a.go"}
	if got := app.View(); !strings.Contains(got, "TeaForge") {
		t.Fatalf("expected full view, got %q", got)
	}

	app.sessionPickerOpen = true
	app.sessionFiles = []string{"/tmp/s1.json"}
	if got := app.renderSessionPicker(20); !strings.Contains(got, "Resume Session") {
		t.Fatalf("session picker=%q", got)
	}
	if got := app.renderStatusBar(); !strings.Contains(got, "Select a session") {
		t.Fatalf("status=%q", got)
	}

	app.sessionPickerOpen = false
	app.activeView = viewModels
	app.models = nil
	if got := app.renderModelsView(20); !strings.Contains(got, "No models found") {
		t.Fatalf("models view=%q", got)
	}

	app.activeView = viewMemory
	app.searchMode = true
	if got := app.renderBody(); !strings.Contains(got, "/ ") {
		t.Fatalf("memory search body=%q", got)
	}
}

func TestAppUpdateKeys_FileAttachAndModelSelect(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	filePath := filepath.Join(app.cfg.WorkDir, "attach.go")
	if err := os.WriteFile(filePath, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	app.filesView = views.NewFilesView(app.cfg.WorkDir)
	app.filesView.SetSize(app.width/3, app.height-3)

	app.activeView = viewFiles
	_ = app.filesView.Toggle() // expand root
	for i := 0; i < 20; i++ {
		selected := app.filesView.SelectedPath()
		if info, err := os.Stat(selected); err == nil && !info.IsDir() {
			break
		}
		app.filesView.MoveDown()
	}

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	a := model.(App)
	if a.activeView != viewChat {
		t.Fatalf("activeView=%v want chat", a.activeView)
	}
	if len(a.pendingAttachments) == 0 {
		t.Fatal("expected pending attachment after file toggle")
	}

	a.activeView = viewModels
	a.models = []string{"m1", "m2"}
	a.modelCursor = 1
	model, _ = a.Update(tea.KeyMsg{Type: tea.KeyEnter})
	a = model.(App)
	if a.cfg.Model != "m2" {
		t.Fatalf("model=%q want m2", a.cfg.Model)
	}
}

func TestSessionPickerAndResumeFlow(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	if err := app.ag.AppendSessionLog("user", "u1"); err != nil {
		t.Fatalf("AppendSessionLog user: %v", err)
	}
	if err := app.ag.AppendSessionLog("assistant", "a1"); err != nil {
		t.Fatalf("AppendSessionLog assistant: %v", err)
	}
	if err := app.ag.AppendSessionLog("tool", "t1"); err != nil {
		t.Fatalf("AppendSessionLog tool: %v", err)
	}

	if err := app.openSessionPicker(); err != nil {
		t.Fatalf("openSessionPicker: %v", err)
	}
	if !app.sessionPickerOpen || len(app.sessionFiles) == 0 {
		t.Fatalf("unexpected picker state: open=%v files=%d", app.sessionPickerOpen, len(app.sessionFiles))
	}

	if err := app.resumeSelectedSession(); err != nil {
		t.Fatalf("resumeSelectedSession: %v", err)
	}
	view := app.chatView.View()
	if !strings.Contains(view, "u1") || !strings.Contains(view, "a1") || !strings.Contains(view, "t1") {
		t.Fatalf("chat was not hydrated from resumed session: %q", view)
	}

	app.sessionFiles = nil
	if err := app.resumeSelectedSession(); err == nil {
		t.Fatal("expected resumeSelectedSession error with no sessions")
	}
}

func TestListSessionFilesAndModelFetchErrors(t *testing.T) {
	t.Parallel()

	if _, err := listSessionFiles(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("expected no sessions found error for missing dir")
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "note.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := listSessionFiles(dir); err == nil {
		t.Fatal("expected no sessions found error for dir without json logs")
	}

	badServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not-json"))
	}))
	defer badServer.Close()

	if _, err := fetchOllamaModels(context.Background(), badServer.URL); err == nil {
		t.Fatal("expected decode error for invalid model response")
	}

	goodServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"models":[{"name":"m1"},{"name":"m2"}]}`))
	}))
	defer goodServer.Close()

	models, err := fetchOllamaModels(context.Background(), goodServer.URL)
	if err != nil {
		t.Fatalf("fetchOllamaModels: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("models=%v", models)
	}
}

func TestInitAndCommandFactories(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			_, _ = w.Write([]byte(`{"models":[{"name":"m1"}]}`))
			return
		}
		if r.URL.Path == "/api/chat" {
			_, _ = w.Write([]byte(`{"model":"m1","message":{"role":"assistant","content":"ok"},"done":true}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	app := newTestAppWithURL(t, server.URL)
	if app.Init() == nil {
		t.Fatal("Init should return a batched startup command")
	}

	msg := fetchModelsCmd(server.URL)()
	ml, ok := msg.(modelListMsg)
	if !ok {
		t.Fatalf("unexpected message type: %T", msg)
	}
	if ml.err != nil || len(ml.models) != 1 || ml.models[0] != "m1" {
		t.Fatalf("unexpected model list: %+v", ml)
	}

	idxMsg := indexWorkDirCmd(app.ag)()
	id, ok := idxMsg.(indexDoneMsg)
	if !ok {
		t.Fatalf("unexpected message type: %T", idxMsg)
	}
	if id.err != nil {
		t.Fatalf("indexWorkDirCmd returned error: %v", id.err)
	}
}

func TestUpdateKeys_AdditionalBranches(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)

	app.sessionPickerOpen = true
	app.sessionFiles = []string{"a", "b"}
	app.sessionCursor = 1
	model, _ := app.updateKeys(tea.KeyMsg{Type: tea.KeyUp})
	a := model.(App)
	if a.sessionCursor != 0 {
		t.Fatalf("sessionCursor=%d want 0", a.sessionCursor)
	}

	model, _ = a.updateKeys(tea.KeyMsg{Type: tea.KeyDown})
	a = model.(App)
	if a.sessionCursor != 1 {
		t.Fatalf("sessionCursor=%d want 1", a.sessionCursor)
	}

	model, _ = a.updateKeys(tea.KeyMsg{Type: tea.KeyEsc})
	a = model.(App)
	if a.sessionPickerOpen {
		t.Fatal("session picker should close on esc")
	}

	a.sessionPickerOpen = true
	a.sessionFiles = nil
	model, _ = a.updateKeys(tea.KeyMsg{Type: tea.KeyEnter})
	a = model.(App)
	if !strings.Contains(a.statusMsg, "Resume error") {
		t.Fatalf("expected resume error status, got %q", a.statusMsg)
	}

	a.sessionPickerOpen = false
	a.activeView = viewMemory
	model, _ = a.updateKeys(tea.KeyMsg{Type: tea.KeyTab})
	a = model.(App)
	if !strings.Contains(a.renderBody(), "Project Notes") {
		t.Fatalf("expected memory tab to advance to project")
	}

	model, _ = a.updateKeys(tea.KeyMsg{Type: tea.KeyShiftTab})
	a = model.(App)
	if !strings.Contains(a.renderBody(), "Session:") {
		t.Fatalf("expected memory tab to return to session")
	}

	model, _ = a.updateKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	a = model.(App)
	if !a.searchMode {
		t.Fatal("expected search mode to open from memory view")
	}

	a.searchMode = false
	a.activeView = viewChat
	ta := a.chatView.Textarea()
	ta.SetValue("draft")
	model, _ = a.updateKeys(tea.KeyMsg{Type: tea.KeyCtrlN})
	a = model.(App)
	if len(a.ag.Session().Turns()) != 0 {
		t.Fatal("expected reset session on ctrl+n")
	}
}

func TestRenderBodyAndStatus_AllViews(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	app.width = 120
	app.height = 40
	app.updateSizes()

	app.activeView = viewChat
	app.pendingAttachments = nil
	if body := app.renderBody(); !strings.Contains(body, "Message:") {
		t.Fatalf("chat body=%q", body)
	}
	if status := app.renderStatusBar(); !strings.Contains(status, "ctrl+s") {
		t.Fatalf("chat status=%q", status)
	}

	app.activeView = viewFiles
	if body := app.renderBody(); !strings.Contains(body, "Files:") {
		t.Fatalf("files body=%q", body)
	}
	if status := app.renderStatusBar(); !strings.Contains(status, "navigate") {
		t.Fatalf("files status=%q", status)
	}

	app.activeView = viewMemory
	app.searchMode = false
	if body := app.renderBody(); !strings.Contains(body, "Session") {
		t.Fatalf("memory body=%q", body)
	}
	if status := app.renderStatusBar(); !strings.Contains(status, "search") {
		t.Fatalf("memory status=%q", status)
	}

	app.activeView = viewModels
	app.models = []string{"m1", "m2"}
	app.cfg.Model = "m1"
	app.modelCursor = 1
	if body := app.renderBody(); !strings.Contains(body, "Available Models") {
		t.Fatalf("models body=%q", body)
	}
	if status := app.renderStatusBar(); !strings.Contains(status, "select") {
		t.Fatalf("models status=%q", status)
	}
}

func TestUpdateKeys_SendQuitAndResume(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{\"model\":\"m\",\"message\":{\"role\":\"assistant\",\"content\":\"ok\"},\"done\":false}\n"))
		_, _ = w.Write([]byte("{\"model\":\"m\",\"message\":{\"role\":\"assistant\",\"content\":\"\"},\"done\":true}\n"))
	}))
	defer server.Close()

	app := newTestAppWithURL(t, server.URL)
	app.activeView = viewChat
	ta := app.chatView.Textarea()
	ta.SetValue("hello")

	model, cmd := app.updateKeys(tea.KeyMsg{Type: tea.KeyCtrlS})
	a := model.(App)
	if !a.thinking {
		t.Fatal("expected send key to start agent run")
	}
	if cmd == nil {
		t.Fatal("expected command from send key")
	}
	if a.agentCancel != nil {
		a.agentCancel()
	}
	if a.agentEvents != nil {
		for range a.agentEvents {
		}
	}

	a.cfg.SessionsDir = filepath.Join(t.TempDir(), "missing-sessions")
	model, _ = a.updateKeys(tea.KeyMsg{Type: tea.KeyCtrlR})
	a = model.(App)
	if !strings.Contains(a.statusMsg, "Resume error") {
		t.Fatalf("expected resume error status, got %q", a.statusMsg)
	}

	model, quitCmd := a.updateKeys(tea.KeyMsg{Type: tea.KeyCtrlC})
	a = model.(App)
	if quitCmd == nil {
		t.Fatal("expected quit command")
	}
	if _, ok := quitCmd().(tea.QuitMsg); !ok {
		t.Fatal("quit command should emit tea.QuitMsg")
	}
}
