package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dan-solli/teaforge/internal/agent"
	"github.com/dan-solli/teaforge/internal/tui"
)

type fakeProgram struct {
	err error
}

func (p fakeProgram) Run() (tea.Model, error) {
	return nil, p.err
}

func restoreMainDeps() func() {
	origNewAgent := newAgent
	origGetwd := getwd
	origNewProgram := newProgram
	return func() {
		newAgent = origNewAgent
		getwd = origGetwd
		newProgram = origNewProgram
	}
}

func TestModelFromEnv(t *testing.T) {
	t.Setenv("TEAFORGE_MODEL", "")
	if got := modelFromEnv(); got != "gemma4:26b" {
		t.Fatalf("default model=%q", got)
	}
	t.Setenv("TEAFORGE_MODEL", "custom:model")
	if got := modelFromEnv(); got != "custom:model" {
		t.Fatalf("model=%q", got)
	}
}

func TestOllamaURLFromEnv(t *testing.T) {
	t.Setenv("OLLAMA_HOST", "")
	if got := ollamaURLFromEnv(); got != "http://localhost:11434" {
		t.Fatalf("default url=%q", got)
	}
	t.Setenv("OLLAMA_HOST", "http://example")
	if got := ollamaURLFromEnv(); got != "http://example" {
		t.Fatalf("url=%q", got)
	}
}

func TestNumCtxFromEnv(t *testing.T) {
	t.Setenv("TEAFORGE_NUM_CTX", "")
	if got := numCtxFromEnv(); got != 8192 {
		t.Fatalf("default num_ctx=%d", got)
	}
	t.Setenv("TEAFORGE_NUM_CTX", "invalid")
	if got := numCtxFromEnv(); got != 8192 {
		t.Fatalf("invalid num_ctx should default, got=%d", got)
	}
	t.Setenv("TEAFORGE_NUM_CTX", "16384")
	if got := numCtxFromEnv(); got != 16384 {
		t.Fatalf("num_ctx=%d", got)
	}
}

func TestResolveResumePath_ByID(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "abc.json")
	if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := resolveResumePath(dir, "abc", false)
	if err != nil {
		t.Fatalf("resolveResumePath: %v", err)
	}
	if got != path {
		t.Fatalf("path=%q want %q", got, path)
	}
}

func TestResolveResumePath_Latest(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	older := filepath.Join(dir, "2026-04-01T00-00-00Z.json")
	newer := filepath.Join(dir, "2026-04-02T00-00-00Z.json")
	_ = os.WriteFile(older, []byte("{}"), 0o644)
	_ = os.WriteFile(newer, []byte("{}"), 0o644)

	got, err := resolveResumePath(dir, "", true)
	if err != nil {
		t.Fatalf("resolveResumePath latest: %v", err)
	}
	if got != newer {
		t.Fatalf("path=%q want %q", got, newer)
	}
}

func TestRun_Success(t *testing.T) {
	restore := restoreMainDeps()
	defer restore()

	wd := t.TempDir()
	getwd = func() (string, error) { return wd, nil }
	newProgram = func(app tui.App) teaProgram { return fakeProgram{} }

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := run(nil, &stdout, &stderr); code != 0 {
		t.Fatalf("run code=%d stderr=%q", code, stderr.String())
	}
}

func TestRun_ParseError(t *testing.T) {
	restore := restoreMainDeps()
	defer restore()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := run([]string{"--unknown"}, &stdout, &stderr); code != 2 {
		t.Fatalf("run code=%d want 2", code)
	}
	if !strings.Contains(stderr.String(), "flag provided") {
		t.Fatalf("stderr=%q", stderr.String())
	}
}

func TestRun_VersionFlag(t *testing.T) {
	restore := restoreMainDeps()
	defer restore()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := run([]string{"--version"}, &stdout, &stderr); code != 0 {
		t.Fatalf("run code=%d want 0", code)
	}
	if !strings.Contains(stdout.String(), "teaforge") {
		t.Fatalf("stdout=%q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr=%q", stderr.String())
	}
}

func TestRun_MutuallyExclusiveResumeFlags(t *testing.T) {
	restore := restoreMainDeps()
	defer restore()

	wd := t.TempDir()
	getwd = func() (string, error) { return wd, nil }
	newProgram = func(app tui.App) teaProgram { return fakeProgram{} }

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run([]string{"--resume", "x", "--resume-latest"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("run code=%d want 1", code)
	}
	if !strings.Contains(stderr.String(), "mutually exclusive") {
		t.Fatalf("stderr=%q", stderr.String())
	}
}

func TestRun_AgentInitError(t *testing.T) {
	restore := restoreMainDeps()
	defer restore()

	wd := t.TempDir()
	getwd = func() (string, error) { return wd, nil }
	newAgent = func(cfg agent.Config) (*agent.Agent, error) {
		return nil, errors.New("init failed")
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := run(nil, &stdout, &stderr); code != 1 {
		t.Fatalf("run code=%d want 1", code)
	}
	if !strings.Contains(stderr.String(), "failed to initialise agent") {
		t.Fatalf("stderr=%q", stderr.String())
	}
}

func TestRun_ProgramError(t *testing.T) {
	restore := restoreMainDeps()
	defer restore()

	wd := t.TempDir()
	getwd = func() (string, error) { return wd, nil }
	newProgram = func(app tui.App) teaProgram { return fakeProgram{err: errors.New("boom")} }

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := run(nil, &stdout, &stderr); code != 1 {
		t.Fatalf("run code=%d want 1", code)
	}
	if !strings.Contains(stderr.String(), "boom") {
		t.Fatalf("stderr=%q", stderr.String())
	}
}

func TestRun_ResumeLatestSuccess(t *testing.T) {
	restore := restoreMainDeps()
	defer restore()

	wd := t.TempDir()
	getwd = func() (string, error) { return wd, nil }
	newProgram = func(app tui.App) teaProgram { return fakeProgram{} }

	sessionsDir := filepath.Join(wd, ".teaforge", "sessions")
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	logPath := filepath.Join(sessionsDir, "2026-04-27T00-00-00Z.json")
	logBody := `{"id":"2026-04-27T00-00-00Z","started_at":"2026-04-27T00:00:00Z","model":"m","work_dir":"` + wd + `","turns":[]}`
	if err := os.WriteFile(logPath, []byte(logBody), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := run([]string{"--resume-latest"}, &stdout, &stderr); code != 0 {
		t.Fatalf("run code=%d stderr=%q", code, stderr.String())
	}
}
