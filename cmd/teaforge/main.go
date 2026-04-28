// TeaForge – a TUI-based agentic software development environment.
//
// It connects to a locally running Ollama instance to provide an AI coding
// assistant with three memory layers (session, project and code), built-in
// file-editing tools, command execution, and tree-sitter powered code analysis.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dan-solli/teaforge/internal/agent"
	"github.com/dan-solli/teaforge/internal/buildinfo"
	"github.com/dan-solli/teaforge/internal/ollama"
	"github.com/dan-solli/teaforge/internal/tui"
)

type teaProgram interface {
	Run() (tea.Model, error)
}

var newAgent = agent.New
var getwd = os.Getwd
var detectModelContextLength = func(ollamaURL, model string) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	return ollama.NewClient(ollamaURL).ModelContextLength(ctx, model)
}
var newProgram = func(app tui.App) teaProgram {
	return tea.NewProgram(
		app,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)
}

const (
	defaultModelContextLength = 262144
	defaultContextUsagePct    = 80
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet("teaforge", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	resumeID := fs.String("resume", "", "Resume a prior session by ID (filename without .json)")
	resumeLatest := fs.Bool("resume-latest", false, "Resume the most recent session")
	showVersion := fs.Bool("version", false, "Print version information and exit")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(stderr, "teaforge: %v\n", err)
		return 2
	}
	if *showVersion {
		fmt.Fprintln(stdout, buildinfo.String())
		return 0
	}

	// Determine working directory
	workDir, err := getwd()
	if err != nil {
		workDir = "."
	}

	// Project memory file lives alongside the source tree
	memoryFile := filepath.Join(workDir, ".teaforge", "memory.json")
	sessionsDir := filepath.Join(workDir, ".teaforge", "sessions")

	model := modelFromEnv()
	ollamaURL := ollamaURLFromEnv()
	modelCtx := modelContextFromEnvOrDetect(model, ollamaURL)
	promptBudget := usablePromptBudget(modelCtx)

	// Build agent configuration
	cfg := agent.Config{
		Model:        model,
		OllamaURL:    ollamaURL,
		WorkDir:      workDir,
		MemoryFile:   memoryFile,
		SessionsDir:  sessionsDir,
		NumCtx:       modelCtx,
		PromptBudget: promptBudget,
	}

	// Create the agent
	ag, err := newAgent(cfg)
	if err != nil {
		fmt.Fprintf(stderr, "teaforge: failed to initialise agent: %v\n", err)
		return 1
	}

	if *resumeID != "" && *resumeLatest {
		fmt.Fprintln(stderr, "teaforge: --resume and --resume-latest are mutually exclusive")
		return 1
	}
	if *resumeID != "" || *resumeLatest {
		path, err := resolveResumePath(sessionsDir, *resumeID, *resumeLatest)
		if err != nil {
			fmt.Fprintf(stderr, "teaforge: %v\n", err)
			return 1
		}
		if err := ag.ResumeFromLog(path); err != nil {
			fmt.Fprintf(stderr, "teaforge: resume failed: %v\n", err)
			return 1
		}
	}

	// Create and run the Bubble Tea program
	app := tui.NewApp(cfg, ag)
	p := newProgram(app)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(stderr, "teaforge: %v\n", err)
		return 1
	}
	return 0
}

// modelFromEnv returns the Ollama model name, falling back to a sensible
// default if TEAFORGE_MODEL is not set.
func modelFromEnv() string {
	if m := os.Getenv("TEAFORGE_MODEL"); m != "" {
		return m
	}
	return "gemma4:26b"
}

// ollamaURLFromEnv returns the Ollama base URL, falling back to localhost
// if OLLAMA_HOST is not set.
func ollamaURLFromEnv() string {
	if h := os.Getenv("OLLAMA_HOST"); h != "" {
		return h
	}
	return "http://localhost:11434"
}

// numCtxFromEnv returns the context window token budget.
// It defaults to 262144 when TEAFORGE_NUM_CTX is unset or invalid.
func numCtxFromEnv() int {
	if n, ok := numCtxOverrideFromEnv(); ok {
		return n
	}
	return defaultModelContextLength
}

func numCtxOverrideFromEnv() (int, bool) {
	raw := os.Getenv("TEAFORGE_NUM_CTX")
	if raw == "" {
		return 0, false
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

func modelContextFromEnvOrDetect(model, ollamaURL string) int {
	if n, ok := numCtxOverrideFromEnv(); ok {
		return n
	}
	n, err := detectModelContextLength(ollamaURL, model)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not detect context length for model %q: %v; omitting num_ctx and using Ollama default\n", model, err)
		return 0
	}
	if n <= 0 {
		fmt.Fprintf(os.Stderr, "warning: detected non-positive context length for model %q; omitting num_ctx and using Ollama default\n", model)
		return 0
	}
	return n
}

func contextUsagePercentFromEnv() int {
	raw := strings.TrimSpace(os.Getenv("TEAFORGE_CTX_USAGE_PERCENT"))
	if raw == "" {
		return defaultContextUsagePct
	}
	pct, err := strconv.Atoi(raw)
	if err != nil || pct < 50 || pct > 95 {
		return defaultContextUsagePct
	}
	return pct
}

func usablePromptBudget(totalCtx int) int {
	if totalCtx <= 0 {
		totalCtx = defaultModelContextLength
	}
	usable := totalCtx * contextUsagePercentFromEnv() / 100
	if usable < 1024 {
		return totalCtx
	}
	return usable
}

func resolveResumePath(sessionsDir, id string, latest bool) (string, error) {
	if latest {
		return latestSessionPath(sessionsDir)
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return "", fmt.Errorf("empty resume id")
	}
	if filepath.IsAbs(id) {
		return id, nil
	}
	if filepath.Ext(id) != ".json" {
		id += ".json"
	}
	path := filepath.Join(sessionsDir, id)
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("resume session not found: %s", path)
	}
	return path, nil
}

func latestSessionPath(sessionsDir string) (string, error) {
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		return "", fmt.Errorf("reading sessions dir: %w", err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if filepath.Ext(e.Name()) == ".json" {
			names = append(names, e.Name())
		}
	}
	if len(names) == 0 {
		return "", fmt.Errorf("no session logs found in %s", sessionsDir)
	}
	sort.Strings(names)
	return filepath.Join(sessionsDir, names[len(names)-1]), nil
}
