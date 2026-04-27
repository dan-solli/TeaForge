package prompt

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/dan-solli/teaforge/internal/ollama"
)

var htmlCommentRE = regexp.MustCompile(`(?s)<!--.*?-->`)

// AgentInstructionsSource loads AGENTS.md from workdir up to repo root.
type AgentInstructionsSource struct{}

func NewAgentInstructionsSource() *AgentInstructionsSource {
	return &AgentInstructionsSource{}
}

func (s *AgentInstructionsSource) Name() string { return "agents_md" }

func (s *AgentInstructionsSource) Mode() ContextMode { return ModePinned }

func (s *AgentInstructionsSource) Priority() int { return 90 }

func (s *AgentInstructionsSource) Collect(_ context.Context, req *Request) ([]ContextItem, error) {
	if req == nil || req.SystemPrompt != "" || req.WorkDir == "" {
		return nil, nil
	}

	paths := discoverAgentInstructionFiles(req.WorkDir)
	if len(paths) == 0 {
		return nil, nil
	}

	var sb strings.Builder
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		content := strings.TrimSpace(htmlCommentRE.ReplaceAllString(string(data), ""))
		if content == "" {
			continue
		}
		if sb.Len() > 0 {
			sb.WriteString("\n\n")
		}
		rel, err := filepath.Rel(req.WorkDir, p)
		if err != nil {
			rel = p
		}
		sb.WriteString("## From ")
		sb.WriteString(filepath.ToSlash(rel))
		sb.WriteString("\n")
		sb.WriteString(content)
	}
	if sb.Len() == 0 {
		return nil, nil
	}
	sb.WriteString("\n\n")

	return []ContextItem{{
		Source:   s.Name(),
		Kind:     "instruction",
		Role:     ollama.RoleSystem,
		Body:     sb.String(),
		Priority: s.Priority(),
		PinKey:   "agents_md",
	}}, nil
}

func discoverAgentInstructionFiles(workDir string) []string {
	start, err := filepath.Abs(workDir)
	if err != nil {
		return nil
	}

	var out []string
	for dir := start; ; dir = filepath.Dir(dir) {
		candidate := filepath.Join(dir, "AGENTS.md")
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			out = append(out, candidate)
		}
		if isRepoRoot(dir) {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
	}
	return out
}

func isRepoRoot(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, ".git"))
	if err != nil {
		return false
	}
	return info.IsDir() || !info.IsDir()
}
