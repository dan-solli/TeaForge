package views

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/dan-solli/teaforge/internal/tui/styles"
)

// -------------------------------------------------------------------
// FilesView
// -------------------------------------------------------------------

// FileNode represents an entry in the file tree.
type FileNode struct {
	Name     string
	Path     string
	IsDir    bool
	Children []*FileNode
	Expanded bool
	Depth    int
}

// FilesView renders a simple file tree explorer.
type FilesView struct {
	width, height int
	rootDir       string
	root          *FileNode
	flat          []*FileNode // flattened visible nodes
	cursor        int
	selectedPath  string
}

// NewFilesView creates a FilesView rooted at dir.
func NewFilesView(dir string) FilesView {
	fv := FilesView{rootDir: dir}
	fv.refresh()
	return fv
}

// SetSize updates the view dimensions.
func (f *FilesView) SetSize(w, h int) {
	f.width = w
	f.height = h
}

// SelectedPath returns the currently highlighted file path.
func (f *FilesView) SelectedPath() string {
	return f.selectedPath
}

// MoveDown moves the cursor down.
func (f *FilesView) MoveDown() {
	if f.cursor < len(f.flat)-1 {
		f.cursor++
	}
	f.updateSelected()
}

// MoveUp moves the cursor up.
func (f *FilesView) MoveUp() {
	if f.cursor > 0 {
		f.cursor--
	}
	f.updateSelected()
}

// Toggle expands or collapses the selected directory, or returns the
// path if a file is selected.
func (f *FilesView) Toggle() string {
	if f.cursor >= len(f.flat) {
		return ""
	}
	node := f.flat[f.cursor]
	if node.IsDir {
		node.Expanded = !node.Expanded
		f.buildFlat()
		return ""
	}
	return node.Path
}

// Refresh re-reads the directory from disk.
func (f *FilesView) refresh() {
	f.root = f.buildTree(f.rootDir, 0)
	f.buildFlat()
}

func (f *FilesView) buildTree(path string, depth int) *FileNode {
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}
	node := &FileNode{
		Name:  info.Name(),
		Path:  path,
		IsDir: info.IsDir(),
		Depth: depth,
	}
	if !info.IsDir() || depth > 6 {
		return node
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return node
	}
	// Directories first, then files, both alphabetical
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir() != entries[j].IsDir() {
			return entries[i].IsDir()
		}
		return entries[i].Name() < entries[j].Name()
	})
	for _, e := range entries {
		name := e.Name()
		// Skip hidden files and common build artifacts at root level
		if strings.HasPrefix(name, ".") {
			continue
		}
		if depth == 0 && (name == "vendor" || name == "node_modules" || name == "dist") {
			continue
		}
		child := f.buildTree(filepath.Join(path, name), depth+1)
		if child != nil {
			node.Children = append(node.Children, child)
		}
	}
	return node
}

func (f *FilesView) buildFlat() {
	f.flat = nil
	if f.root != nil {
		f.flattenNode(f.root)
	}
	if f.cursor >= len(f.flat) && len(f.flat) > 0 {
		f.cursor = len(f.flat) - 1
	}
	f.updateSelected()
}

func (f *FilesView) flattenNode(node *FileNode) {
	f.flat = append(f.flat, node)
	if node.IsDir && node.Expanded {
		for _, child := range node.Children {
			f.flattenNode(child)
		}
	}
}

func (f *FilesView) updateSelected() {
	if f.cursor < len(f.flat) {
		f.selectedPath = f.flat[f.cursor].Path
	}
}

// View renders the file tree.
func (f *FilesView) View() string {
	maxH := f.height - 4
	if maxH < 1 {
		maxH = 1
	}

	// Window the visible range around the cursor
	start := 0
	if f.cursor >= maxH {
		start = f.cursor - maxH + 1
	}
	end := start + maxH
	if end > len(f.flat) {
		end = len(f.flat)
	}

	var lines []string
	for i := start; i < end; i++ {
		node := f.flat[i]
		indent := strings.Repeat("  ", node.Depth)
		var icon string
		if node.IsDir {
			if node.Expanded {
				icon = "▾ "
			} else {
				icon = "▸ "
			}
		} else {
			icon = "  "
		}
		name := node.Name
		line := indent + icon + name
		if i == f.cursor {
			line = styles.ListItemSelected.Render(line)
		} else if node.IsDir {
			line = styles.AILabel.Render(line)
		}
		lines = append(lines, line)
	}

	title := styles.TitleBar.Render(fmt.Sprintf(" Files: %s ", shortenPath(f.rootDir)))
	content := strings.Join(lines, "\n")

	inner := lipgloss.JoinVertical(lipgloss.Left, title, content)
	return styles.Panel.
		Width(f.width - 2).
		Height(f.height - 2).
		Render(inner)
}
