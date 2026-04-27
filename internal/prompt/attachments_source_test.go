package prompt

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dan-solli/teaforge/internal/memory"
	"github.com/dan-solli/teaforge/internal/ollama"
)

func TestParseMentionPaths_SkipsCodeFences(t *testing.T) {
	t.Parallel()

	msg := "use @a.go and ```\nignore @b.go\n``` then @c.md"
	got := parseMentionPaths(msg)
	want := []string{"a.go", "c.md"}
	if len(got) != len(want) {
		t.Fatalf("mentions len=%d want=%d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("mentions[%d]=%q want=%q", i, got[i], want[i])
		}
	}
}

func TestAttachmentsSourceCollect_MentionsAndAttachedPaths(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("A"), 0o644); err != nil {
		t.Fatalf("write a.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("B"), 0o644); err != nil {
		t.Fatalf("write b.txt: %v", err)
	}

	src := NewAttachmentsSource()
	items, err := src.Collect(context.Background(), &Request{
		WorkDir:       dir,
		UserMessage:   "please inspect @a.txt and @a.txt",
		AttachedPaths: []string{"b.txt", "b.txt"},
	})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("items len=%d want=2", len(items))
	}
	if items[0].Role != ollama.RoleUser || items[1].Role != ollama.RoleUser {
		t.Fatalf("expected user role for attachment items")
	}
	if !strings.Contains(items[0].Body, "<file path=\"a.txt\">") {
		t.Fatalf("first item should be a.txt, got %q", items[0].Body)
	}
	if !strings.Contains(items[1].Body, "<file path=\"b.txt\">") {
		t.Fatalf("second item should be b.txt, got %q", items[1].Body)
	}
}

func TestAttachmentsSourceCollect_TruncatesLargeFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	large := strings.Repeat("x", maxAttachmentBytes+10)
	if err := os.WriteFile(filepath.Join(dir, "big.txt"), []byte(large), 0o644); err != nil {
		t.Fatalf("write big.txt: %v", err)
	}

	src := NewAttachmentsSource()
	items, err := src.Collect(context.Background(), &Request{WorkDir: dir, UserMessage: "@big.txt"})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items len=%d want=1", len(items))
	}
	if !strings.Contains(items[0].Body, "...[truncated]") {
		t.Fatalf("expected truncation marker, got %q", items[0].Body)
	}
}

func TestPipelineBuild_AttachmentsAppendedAfterHistory(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("A"), 0o644); err != nil {
		t.Fatalf("write a.txt: %v", err)
	}

	p := NewDefaultPipeline(nil)
	msgs, _, err := p.Build(context.Background(), &Request{
		WorkDir:         dir,
		UserMessage:     "check @a.txt",
		SessionTurns:    []memory.Turn{{Role: ollama.RoleUser, Content: "check @a.txt"}},
		ProjectNotes:    nil,
		CodeSymbolCount: 0,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(msgs) < 3 {
		t.Fatalf("expected at least 3 messages, got %d", len(msgs))
	}
	last := msgs[len(msgs)-1]
	if last.Role != ollama.RoleUser {
		t.Fatalf("last role=%q want user", last.Role)
	}
	if !strings.Contains(last.Content, "<file path=\"a.txt\">") {
		t.Fatalf("last message should contain attached file, got %q", last.Content)
	}
}
