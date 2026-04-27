package views

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/dan-solli/teaforge/internal/memory"
	"github.com/dan-solli/teaforge/internal/treesitter"
)

func TestMemoryView_TabsScrollAndView(t *testing.T) {
	t.Parallel()
	s := memory.NewSessionMemory()
	s.AddTurn("user", "hello")
	s.SetContext("k", "v")

	pm, err := memory.NewProjectMemory(filepath.Join(t.TempDir(), "memory.json"))
	if err != nil {
		t.Fatalf("NewProjectMemory: %v", err)
	}
	_, _ = pm.AddNote("architecture", "use source pipeline")

	cm := treesitter.NewCodeMemory()
	v := NewMemoryView(s, pm, cm)
	v.SetSize(100, 30)

	v.NextTab()
	v.PrevTab()
	v.ScrollDown()
	v.ScrollUp()
	v.SetCodeQuery("foo")
	if v.CodeQuery() != "foo" {
		t.Fatalf("code query=%q", v.CodeQuery())
	}

	out := v.View()
	if !strings.Contains(out, "Session") {
		t.Fatalf("expected tab text in view: %q", out)
	}
}

func TestShortenPath(t *testing.T) {
	t.Parallel()
	if got := shortenPath("a/b"); got != "a/b" {
		t.Fatalf("got=%q", got)
	}
	got := shortenPath("a/b/c/d/e")
	if !strings.Contains(got, "...") {
		t.Fatalf("expected shortened path, got=%q", got)
	}
}
