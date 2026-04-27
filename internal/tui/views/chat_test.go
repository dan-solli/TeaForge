package views

import (
	"strings"
	"testing"
)

func TestChatView_BasicFlow(t *testing.T) {
	t.Parallel()
	v := NewChatView()
	v.SetSize(100, 30)
	v.AddEntry("user", "hello")
	v.AppendPartial("stream")
	v.AddToolEvent("tool_call", "read_file")
	v.SetThinking(true)
	v.TickThinking()

	if !v.Focused() {
		t.Fatal("textarea should start focused")
	}
	v.BlurTextarea()
	if v.Focused() {
		t.Fatal("textarea should be blurred")
	}
	v.FocusTextarea()

	out := v.View()
	for _, want := range []string{"You", "hello", "calling: read_file", "Thinking"} {
		if !strings.Contains(out, want) {
			t.Fatalf("view missing %q", want)
		}
	}
}

func TestChatView_RenderEntryAndTextareaHelpers(t *testing.T) {
	t.Parallel()
	v := NewChatView()
	v.SetSize(80, 20)
	v.AddEntry("assistant", "ok")
	if got := v.TextareaValue(); got != "" {
		t.Fatalf("initial textarea value=%q", got)
	}
	v.Textarea().SetValue("hello")
	if got := v.TextareaValue(); got != "hello" {
		t.Fatalf("textarea value=%q", got)
	}
	v.ClearTextarea()
	if got := v.TextareaValue(); got != "" {
		t.Fatalf("cleared textarea value=%q", got)
	}

	entry := ChatEntry{Role: "error", Content: "boom"}
	rendered := v.renderEntry(entry)
	if !strings.Contains(rendered, "boom") {
		t.Fatalf("rendered missing content: %q", rendered)
	}
}
