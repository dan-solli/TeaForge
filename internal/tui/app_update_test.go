package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestAppUpdate_HandlesBasicKeys(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	app.width = 120
	app.height = 40
	app.updateSizes()
	app.ag.Session().AddTurn("user", "hello")

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyCtrlN})
	a := model.(App)
	if len(a.ag.Session().Turns()) != 0 {
		t.Fatalf("expected session cleared on ctrl+n")
	}

	a.activeView = viewModels
	a.models = []string{"m1", "m2"}
	a.modelCursor = 0
	model, _ = a.Update(tea.KeyMsg{Type: tea.KeyDown})
	a = model.(App)
	if a.modelCursor != 1 {
		t.Fatalf("modelCursor=%d want 1", a.modelCursor)
	}

	model, _ = a.Update(tea.KeyMsg{Type: tea.KeyUp})
	a = model.(App)
	if a.modelCursor != 0 {
		t.Fatalf("modelCursor=%d want 0", a.modelCursor)
	}
}

func TestAppUpdate_ChatAllowsLowercaseJK(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	app.activeView = viewChat
	app.chatView.FocusTextarea()

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	a := model.(App)
	if got := a.chatView.TextareaValue(); got != "j" {
		t.Fatalf("textarea=%q want %q", got, "j")
	}

	model, _ = a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	a = model.(App)
	if got := a.chatView.TextareaValue(); got != "jk" {
		t.Fatalf("textarea=%q want %q", got, "jk")
	}
}
