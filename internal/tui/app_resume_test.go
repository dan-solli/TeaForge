package tui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListSessionFiles_SortedNewestFirst(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := filepath.Join(dir, "2026-04-01T00-00-00Z.json")
	b := filepath.Join(dir, "2026-04-03T00-00-00Z.json")
	c := filepath.Join(dir, "2026-04-02T00-00-00Z.json")
	_ = os.WriteFile(a, []byte("{}"), 0o644)
	_ = os.WriteFile(b, []byte("{}"), 0o644)
	_ = os.WriteFile(c, []byte("{}"), 0o644)

	files, err := listSessionFiles(dir)
	if err != nil {
		t.Fatalf("listSessionFiles: %v", err)
	}
	if len(files) != 3 {
		t.Fatalf("len=%d want 3", len(files))
	}
	if filepath.Base(files[0]) != filepath.Base(b) {
		t.Fatalf("first=%q want %q", files[0], b)
	}
}
