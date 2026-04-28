package prompt

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/dan-solli/teaforge/internal/ollama"
)

const maxAttachmentBytes = 32 * 1024

var (
	fencedBlockRE  = regexp.MustCompile("(?s)```.*?```")
	attachmentAtRE = regexp.MustCompile(`@([^\s\x60]+)`)
)

// AttachmentsSource emits file contents from @mentions and explicit TUI attachments.
type AttachmentsSource struct{}

func NewAttachmentsSource() *AttachmentsSource {
	return &AttachmentsSource{}
}

func (s *AttachmentsSource) Name() string { return "attachments" }

func (s *AttachmentsSource) Mode() ContextMode { return ModePinned }

func (s *AttachmentsSource) Priority() int { return 70 }

func (s *AttachmentsSource) Collect(_ context.Context, req *Request) ([]ContextItem, error) {
	if req == nil || req.WorkDir == "" {
		return nil, nil
	}

	candidates := collectAttachmentCandidates(req.UserMessage, req.AttachedPaths)
	if len(candidates) == 0 {
		return nil, nil
	}

	workDir, err := filepath.Abs(req.WorkDir)
	if err != nil {
		workDir = req.WorkDir
	}

	items := make([]ContextItem, 0, len(candidates))
	for _, p := range candidates {
		absPath, relPath, ok := resolveAttachmentPath(workDir, p)
		if !ok {
			continue
		}
		data, err := os.ReadFile(absPath)
		if err != nil {
			continue
		}
		truncated := false
		if len(data) > maxAttachmentBytes {
			data = data[:maxAttachmentBytes]
			truncated = true
		}

		content := string(data)
		if len(data) > 0 && data[len(data)-1] != '\n' {
			content += "\n"
		}
		body, err := renderPromptTemplate("attachment_file.tmpl", struct {
			RelPath   string
			Content   string
			Truncated bool
		}{
			RelPath:   filepath.ToSlash(relPath),
			Content:   content,
			Truncated: truncated,
		})
		if err != nil {
			return nil, err
		}

		items = append(items, ContextItem{
			Source:   s.Name(),
			Kind:     "attachment",
			Role:     ollama.RoleUser,
			Body:     body,
			Priority: s.Priority(),
			PinKey:   "attachment:" + filepath.ToSlash(relPath),
		})
	}

	return items, nil
}

func collectAttachmentCandidates(userMessage string, attachedPaths []string) []string {
	var out []string
	seen := make(map[string]struct{})

	for _, p := range parseMentionPaths(userMessage) {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	for _, p := range attachedPaths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}

func parseMentionPaths(message string) []string {
	if strings.TrimSpace(message) == "" {
		return nil
	}
	sanitized := fencedBlockRE.ReplaceAllString(message, "")
	matches := attachmentAtRE.FindAllStringSubmatch(sanitized, -1)
	if len(matches) == 0 {
		return nil
	}
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		candidate := strings.TrimRight(m[1], ".,;:!?)]}")
		if candidate == "" {
			continue
		}
		out = append(out, candidate)
	}
	return out
}

func resolveAttachmentPath(workDir, candidate string) (absPath string, relPath string, ok bool) {
	clean := filepath.Clean(candidate)
	if clean == "." || clean == string(filepath.Separator) {
		return "", "", false
	}

	if filepath.IsAbs(clean) {
		absPath = clean
	} else {
		absPath = filepath.Join(workDir, clean)
	}

	absPath, err := filepath.Abs(absPath)
	if err != nil {
		return "", "", false
	}
	relPath, err = filepath.Rel(workDir, absPath)
	if err != nil {
		return "", "", false
	}
	if strings.HasPrefix(relPath, "..") || relPath == "." {
		return "", "", false
	}
	info, err := os.Stat(absPath)
	if err != nil || info.IsDir() {
		return "", "", false
	}
	return absPath, relPath, true
}
