package prompt

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnvironmentSourceCollect_Basic(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "recent.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write recent file: %v", err)
	}

	src := NewEnvironmentSource()
	items, err := src.Collect(context.Background(), &Request{WorkDir: dir, Model: "m1"})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	body := items[0].Body
	if !strings.Contains(body, "<env>") || !strings.Contains(body, "</env>") {
		t.Fatalf("expected env wrapper, got: %q", body)
	}
	if !strings.Contains(body, "model: m1") {
		t.Fatalf("expected model line, got: %q", body)
	}
	if !strings.Contains(body, "working_directory:") {
		t.Fatalf("expected working directory line, got: %q", body)
	}
}

func TestEnvironmentSourceCollect_GitContext(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v (%s)", args, err, string(out))
		}
	}
	run("init")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("a"), 0o644); err != nil {
		t.Fatalf("write tracked file: %v", err)
	}
	run("add", "tracked.txt")
	run("commit", "-m", "init")
	if err := os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("b"), 0o644); err != nil {
		t.Fatalf("modify tracked file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "untracked.txt"), []byte("u"), 0o644); err != nil {
		t.Fatalf("write untracked file: %v", err)
	}

	src := NewEnvironmentSource()
	items, err := src.Collect(context.Background(), &Request{WorkDir: dir, Model: "m1"})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	body := items[0].Body
	if !strings.Contains(body, "git_branch:") {
		t.Fatalf("expected git branch line, got: %q", body)
	}
	if !strings.Contains(body, "git_status: modified=1 untracked=1") {
		t.Fatalf("expected git status counts, got: %q", body)
	}
}
