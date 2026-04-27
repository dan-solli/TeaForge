package prompt

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/dan-solli/teaforge/internal/ollama"
)

var nowUTC = func() time.Time {
	return time.Now().UTC()
}

// EnvironmentSource emits current runtime and repository context.
type EnvironmentSource struct{}

func NewEnvironmentSource() *EnvironmentSource {
	return &EnvironmentSource{}
}

func (s *EnvironmentSource) Name() string { return "environment" }

func (s *EnvironmentSource) Mode() ContextMode { return ModePinned }

func (s *EnvironmentSource) Priority() int { return 85 }

func (s *EnvironmentSource) Collect(ctx context.Context, req *Request) ([]ContextItem, error) {
	if req == nil || req.SystemPrompt != "" || req.WorkDir == "" {
		return nil, nil
	}

	workDir, err := filepath.Abs(req.WorkDir)
	if err != nil {
		workDir = req.WorkDir
	}
	branch, modified, untracked := gitStatus(ctx, workDir)
	recent := recentlyModifiedFiles(workDir, nowUTC().Add(-1*time.Hour), 20)

	var sb strings.Builder
	sb.WriteString("<env>\n")
	sb.WriteString(fmt.Sprintf("timestamp: %s\n", nowUTC().Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("working_directory: %s\n", filepath.ToSlash(workDir)))
	if req.Model != "" {
		sb.WriteString(fmt.Sprintf("model: %s\n", req.Model))
	}
	if branch != "" {
		sb.WriteString(fmt.Sprintf("git_branch: %s\n", branch))
	}
	if modified >= 0 && untracked >= 0 {
		sb.WriteString(fmt.Sprintf("git_status: modified=%d untracked=%d\n", modified, untracked))
	} else {
		sb.WriteString("git_status: unavailable\n")
	}
	if len(recent) == 0 {
		sb.WriteString("recent_files: none\n")
	} else {
		sb.WriteString("recent_files:\n")
		for _, f := range recent {
			sb.WriteString("- ")
			sb.WriteString(f)
			sb.WriteString("\n")
		}
	}
	sb.WriteString("</env>\n\n")

	return []ContextItem{{
		Source:   s.Name(),
		Kind:     "environment",
		Role:     ollama.RoleSystem,
		Body:     sb.String(),
		Priority: s.Priority(),
		PinKey:   "environment",
	}}, nil
}

func gitStatus(ctx context.Context, workDir string) (branch string, modified int, untracked int) {
	branchOut, err := runGit(ctx, workDir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", -1, -1
	}
	statusOut, err := runGit(ctx, workDir, "status", "--porcelain")
	if err != nil {
		return strings.TrimSpace(branchOut), -1, -1
	}

	modified = 0
	untracked = 0
	for _, line := range strings.Split(statusOut, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "??") {
			untracked++
			continue
		}
		modified++
	}
	return strings.TrimSpace(branchOut), modified, untracked
}

func runGit(ctx context.Context, workDir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = workDir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func recentlyModifiedFiles(workDir string, since time.Time, limit int) []string {
	type entry struct {
		path string
		mod  time.Time
	}
	var entries []entry

	_ = filepath.WalkDir(workDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if info.ModTime().Before(since) {
			return nil
		}
		rel, err := filepath.Rel(workDir, path)
		if err != nil {
			rel = path
		}
		entries = append(entries, entry{path: filepath.ToSlash(rel), mod: info.ModTime()})
		return nil
	})

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].mod.After(entries[j].mod)
	})
	if len(entries) > limit {
		entries = entries[:limit]
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.path)
	}
	return out
}
