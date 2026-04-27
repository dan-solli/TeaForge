// TeaForge – a TUI-based agentic software development environment.
//
// It connects to a locally running Ollama instance to provide an AI coding
// assistant with three memory layers (session, project and code), built-in
// file-editing tools, command execution, and tree-sitter powered code analysis.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dan-solli/teaforge/internal/agent"
	"github.com/dan-solli/teaforge/internal/tui"
)

func main() {
	// Determine working directory
	workDir, err := os.Getwd()
	if err != nil {
		workDir = "."
	}

	// Project memory file lives alongside the source tree
	memoryFile := filepath.Join(workDir, ".teaforge", "memory.json")
	sessionsDir := filepath.Join(workDir, ".teaforge", "sessions")

	// Build agent configuration
	cfg := agent.Config{
		Model:       modelFromEnv(),
		OllamaURL:   ollamaURLFromEnv(),
		WorkDir:     workDir,
		MemoryFile:  memoryFile,
		SessionsDir: sessionsDir,
	}

	// Create the agent
	ag, err := agent.New(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "teaforge: failed to initialise agent: %v\n", err)
		os.Exit(1)
	}

	// Create and run the Bubble Tea program
	app := tui.NewApp(cfg, ag)
	p := tea.NewProgram(
		app,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "teaforge: %v\n", err)
		os.Exit(1)
	}
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
