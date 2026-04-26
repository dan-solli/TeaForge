// Package treesitter provides code-level memory powered by tree-sitter.
// It parses source files and extracts structural information (symbols,
// functions, types, imports) that the agent can consult as reference material.
package treesitter

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/python"
	"github.com/smacker/go-tree-sitter/typescript/typescript"
	"github.com/smacker/go-tree-sitter/javascript"
)

// Symbol represents a named entity extracted from source code.
type Symbol struct {
	Name     string `json:"name"`
	Kind     string `json:"kind"` // "function", "type", "const", "var", "import"
	File     string `json:"file"`
	Line     uint32 `json:"line"`
	Language string `json:"language"`
	Snippet  string `json:"snippet"`
}

// FileIndex stores the parsed symbols for a single source file.
type FileIndex struct {
	Path     string   `json:"path"`
	Language string   `json:"language"`
	Symbols  []Symbol `json:"symbols"`
}

// CodeMemory is the tree-sitter backed code index.
type CodeMemory struct {
	mu      sync.RWMutex
	index   map[string]*FileIndex // keyed by absolute file path
	parsers map[string]*sitter.Parser
}

// NewCodeMemory creates a CodeMemory with parsers for supported languages.
func NewCodeMemory() *CodeMemory {
	cm := &CodeMemory{
		index:   make(map[string]*FileIndex),
		parsers: make(map[string]*sitter.Parser),
	}

	langs := map[string]func() *sitter.Language{
		"go":         golang.GetLanguage,
		"python":     python.GetLanguage,
		"typescript": typescript.GetLanguage,
		"javascript": javascript.GetLanguage,
	}

	for name, getLang := range langs {
		p := sitter.NewParser()
		p.SetLanguage(getLang())
		cm.parsers[name] = p
	}

	return cm
}

// languageForFile returns the language name for a given file path based on
// its extension, or an empty string if unsupported.
func languageForFile(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		return "go"
	case ".py":
		return "python"
	case ".ts", ".tsx":
		return "typescript"
	case ".js", ".jsx", ".mjs":
		return "javascript"
	default:
		return ""
	}
}

// IndexFile parses the given file and stores its symbols in the code index.
func (cm *CodeMemory) IndexFile(ctx context.Context, path string) error {
	lang := languageForFile(path)
	if lang == "" {
		return nil // unsupported language, skip silently
	}

	src, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}

	cm.mu.Lock()
	parser, ok := cm.parsers[lang]
	cm.mu.Unlock()
	if !ok {
		return nil
	}

	tree, err := parser.ParseCtx(ctx, nil, src)
	if err != nil {
		return fmt.Errorf("parsing %s: %w", path, err)
	}
	defer tree.Close()

	symbols := extractSymbols(tree.RootNode(), src, path, lang)

	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.index[path] = &FileIndex{
		Path:     path,
		Language: lang,
		Symbols:  symbols,
	}
	return nil
}

// IndexDirectory walks dir recursively and indexes all supported source files.
func (cm *CodeMemory) IndexDirectory(ctx context.Context, dir string) error {
	return filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		if d.IsDir() {
			// Skip hidden and vendor directories
			base := d.Name()
			if strings.HasPrefix(base, ".") || base == "vendor" || base == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		return cm.IndexFile(ctx, path)
	})
}

// Files returns paths of all indexed files.
func (cm *CodeMemory) Files() []string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	paths := make([]string, 0, len(cm.index))
	for p := range cm.index {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths
}

// FileIndex returns the index for a specific file, or nil if not indexed.
func (cm *CodeMemory) FileIndex(path string) *FileIndex {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	fi := cm.index[path]
	if fi == nil {
		return nil
	}
	// Return a shallow copy to avoid data races on slices
	cp := *fi
	return &cp
}

// Search returns all symbols whose name contains the query string
// (case-insensitive).
func (cm *CodeMemory) Search(query string) []Symbol {
	q := strings.ToLower(query)
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	var out []Symbol
	for _, fi := range cm.index {
		for _, s := range fi.Symbols {
			if strings.Contains(strings.ToLower(s.Name), q) {
				out = append(out, s)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}
		return out[i].Line < out[j].Line
	})
	return out
}

// AllSymbols returns every symbol in the index.
func (cm *CodeMemory) AllSymbols() []Symbol {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	var out []Symbol
	for _, fi := range cm.index {
		out = append(out, fi.Symbols...)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}
		return out[i].Line < out[j].Line
	})
	return out
}

// -------------------------------------------------------------------
// Symbol extraction helpers
// -------------------------------------------------------------------

func extractSymbols(node *sitter.Node, src []byte, file, lang string) []Symbol {
	switch lang {
	case "go":
		return extractGoSymbols(node, src, file)
	case "python":
		return extractPythonSymbols(node, src, file)
	case "typescript", "javascript":
		return extractJSSymbols(node, src, file, lang)
	}
	return nil
}

func nodeText(node *sitter.Node, src []byte) string {
	if node == nil {
		return ""
	}
	return string(src[node.StartByte():node.EndByte()])
}

func snippetLine(node *sitter.Node, src []byte) string {
	text := nodeText(node, src)
	lines := strings.SplitN(text, "\n", 2)
	if len(lines) == 0 {
		return text
	}
	return strings.TrimSpace(lines[0])
}

func extractGoSymbols(root *sitter.Node, src []byte, file string) []Symbol {
	var symbols []Symbol
	var walk func(n *sitter.Node)
	walk = func(n *sitter.Node) {
		switch n.Type() {
		case "function_declaration", "method_declaration":
			nameNode := n.ChildByFieldName("name")
			if nameNode != nil {
				symbols = append(symbols, Symbol{
					Name:     nodeText(nameNode, src),
					Kind:     "function",
					File:     file,
					Line:     n.StartPoint().Row + 1,
					Language: "go",
					Snippet:  snippetLine(n, src),
				})
			}
		case "type_declaration":
			for i := 0; i < int(n.ChildCount()); i++ {
				spec := n.Child(i)
				if spec.Type() == "type_spec" {
					nameNode := spec.ChildByFieldName("name")
					if nameNode != nil {
						symbols = append(symbols, Symbol{
							Name:     nodeText(nameNode, src),
							Kind:     "type",
							File:     file,
							Line:     n.StartPoint().Row + 1,
							Language: "go",
							Snippet:  snippetLine(n, src),
						})
					}
				}
			}
		case "const_declaration", "var_declaration":
			kind := "const"
			if n.Type() == "var_declaration" {
				kind = "var"
			}
			for i := 0; i < int(n.ChildCount()); i++ {
				spec := n.Child(i)
				if spec.Type() == "const_spec" || spec.Type() == "var_spec" {
					for j := 0; j < int(spec.ChildCount()); j++ {
						child := spec.Child(j)
						if child.Type() == "identifier" {
							symbols = append(symbols, Symbol{
								Name:     nodeText(child, src),
								Kind:     kind,
								File:     file,
								Line:     n.StartPoint().Row + 1,
								Language: "go",
								Snippet:  snippetLine(spec, src),
							})
						}
					}
				}
			}
		case "import_declaration":
			symbols = append(symbols, Symbol{
				Name:     snippetLine(n, src),
				Kind:     "import",
				File:     file,
				Line:     n.StartPoint().Row + 1,
				Language: "go",
				Snippet:  snippetLine(n, src),
			})
		}
		for i := 0; i < int(n.ChildCount()); i++ {
			walk(n.Child(i))
		}
	}
	walk(root)
	return symbols
}

func extractPythonSymbols(root *sitter.Node, src []byte, file string) []Symbol {
	var symbols []Symbol
	var walk func(n *sitter.Node)
	walk = func(n *sitter.Node) {
		switch n.Type() {
		case "function_definition":
			nameNode := n.ChildByFieldName("name")
			if nameNode != nil {
				symbols = append(symbols, Symbol{
					Name:     nodeText(nameNode, src),
					Kind:     "function",
					File:     file,
					Line:     n.StartPoint().Row + 1,
					Language: "python",
					Snippet:  snippetLine(n, src),
				})
			}
		case "class_definition":
			nameNode := n.ChildByFieldName("name")
			if nameNode != nil {
				symbols = append(symbols, Symbol{
					Name:     nodeText(nameNode, src),
					Kind:     "type",
					File:     file,
					Line:     n.StartPoint().Row + 1,
					Language: "python",
					Snippet:  snippetLine(n, src),
				})
			}
		case "import_statement", "import_from_statement":
			symbols = append(symbols, Symbol{
				Name:     snippetLine(n, src),
				Kind:     "import",
				File:     file,
				Line:     n.StartPoint().Row + 1,
				Language: "python",
				Snippet:  snippetLine(n, src),
			})
		}
		for i := 0; i < int(n.ChildCount()); i++ {
			walk(n.Child(i))
		}
	}
	walk(root)
	return symbols
}

func extractJSSymbols(root *sitter.Node, src []byte, file, lang string) []Symbol {
	var symbols []Symbol
	var walk func(n *sitter.Node)
	walk = func(n *sitter.Node) {
		switch n.Type() {
		case "function_declaration", "arrow_function":
			nameNode := n.ChildByFieldName("name")
			if nameNode != nil {
				symbols = append(symbols, Symbol{
					Name:     nodeText(nameNode, src),
					Kind:     "function",
					File:     file,
					Line:     n.StartPoint().Row + 1,
					Language: lang,
					Snippet:  snippetLine(n, src),
				})
			}
		case "class_declaration":
			nameNode := n.ChildByFieldName("name")
			if nameNode != nil {
				symbols = append(symbols, Symbol{
					Name:     nodeText(nameNode, src),
					Kind:     "type",
					File:     file,
					Line:     n.StartPoint().Row + 1,
					Language: lang,
					Snippet:  snippetLine(n, src),
				})
			}
		case "import_statement":
			symbols = append(symbols, Symbol{
				Name:     snippetLine(n, src),
				Kind:     "import",
				File:     file,
				Line:     n.StartPoint().Row + 1,
				Language: lang,
				Snippet:  snippetLine(n, src),
			})
		case "lexical_declaration", "variable_declaration":
			for i := 0; i < int(n.ChildCount()); i++ {
				child := n.Child(i)
				if child.Type() == "variable_declarator" {
					nameNode := child.ChildByFieldName("name")
					if nameNode != nil {
						symbols = append(symbols, Symbol{
							Name:     nodeText(nameNode, src),
							Kind:     "var",
							File:     file,
							Line:     n.StartPoint().Row + 1,
							Language: lang,
							Snippet:  snippetLine(n, src),
						})
					}
				}
			}
		}
		for i := 0; i < int(n.ChildCount()); i++ {
			walk(n.Child(i))
		}
	}
	walk(root)
	return symbols
}
