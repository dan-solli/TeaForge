package treesitter_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dan-solli/teaforge/internal/treesitter"
)

func TestCodeMemory_IndexGoFile(t *testing.T) {
	src := `package main

import "fmt"

type Config struct {
	Model string
}

func Hello(name string) string {
	return fmt.Sprintf("Hello, %s!", name)
}

const Version = "1.0.0"

var Debug = false
`
	dir := t.TempDir()
	path := filepath.Join(dir, "main.go")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	cm := treesitter.NewCodeMemory()
	if err := cm.IndexFile(context.Background(), path); err != nil {
		t.Fatalf("IndexFile: %v", err)
	}

	fi := cm.FileIndex(path)
	if fi == nil {
		t.Fatal("expected file index, got nil")
	}
	if fi.Language != "go" {
		t.Errorf("expected language=go, got %q", fi.Language)
	}

	symbols := fi.Symbols
	// We expect at least: Config (type), Hello (function), Version (const), Debug (var), import
	var kindNames []string
	for _, s := range symbols {
		kindNames = append(kindNames, s.Kind+":"+s.Name)
	}
	t.Logf("symbols: %v", kindNames)

	assertSymbol(t, symbols, "function", "Hello")
	assertSymbol(t, symbols, "type", "Config")
}

func TestCodeMemory_IndexPythonFile(t *testing.T) {
	src := `import os
from pathlib import Path

class Greeter:
    def greet(self, name):
        return f"Hello, {name}"

def standalone(x):
    return x * 2
`
	dir := t.TempDir()
	path := filepath.Join(dir, "hello.py")
	os.WriteFile(path, []byte(src), 0o644) //nolint:errcheck

	cm := treesitter.NewCodeMemory()
	if err := cm.IndexFile(context.Background(), path); err != nil {
		t.Fatalf("IndexFile: %v", err)
	}

	symbols := cm.FileIndex(path).Symbols
	assertSymbol(t, symbols, "type", "Greeter")
	assertSymbol(t, symbols, "function", "standalone")
}

func TestCodeMemory_Search(t *testing.T) {
	src := `package main

func ParseConfig() {}
func ParseFlags() {}
func RunServer() {}
`
	dir := t.TempDir()
	path := filepath.Join(dir, "server.go")
	os.WriteFile(path, []byte(src), 0o644) //nolint:errcheck

	cm := treesitter.NewCodeMemory()
	cm.IndexFile(context.Background(), path) //nolint:errcheck

	results := cm.Search("parse")
	if len(results) < 2 {
		t.Errorf("expected at least 2 results for 'parse', got %d", len(results))
	}
	for _, r := range results {
		if !strings.Contains(strings.ToLower(r.Name), "parse") {
			t.Errorf("unexpected result name: %q", r.Name)
		}
	}
}

func TestCodeMemory_SearchEmpty(t *testing.T) {
	cm := treesitter.NewCodeMemory()
	results := cm.Search("nonexistent")
	if len(results) != 0 {
		t.Errorf("expected no results, got %d", len(results))
	}
}

func TestCodeMemory_UnsupportedFileSkipped(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.csv")
	os.WriteFile(path, []byte("a,b,c"), 0o644) //nolint:errcheck

	cm := treesitter.NewCodeMemory()
	if err := cm.IndexFile(context.Background(), path); err != nil {
		t.Fatalf("expected no error for unsupported file, got %v", err)
	}
	files := cm.Files()
	if len(files) != 0 {
		t.Error("expected unsupported file to be skipped")
	}
}

func TestCodeMemory_IndexDirectory(t *testing.T) {
	dir := t.TempDir()
	goSrc := `package main
func Foo() {}
func Bar() {}
`
	pySrc := `def baz(): pass
`
	os.WriteFile(filepath.Join(dir, "a.go"), []byte(goSrc), 0o644) //nolint:errcheck
	os.WriteFile(filepath.Join(dir, "b.py"), []byte(pySrc), 0o644) //nolint:errcheck

	cm := treesitter.NewCodeMemory()
	if err := cm.IndexDirectory(context.Background(), dir); err != nil {
		t.Fatalf("IndexDirectory: %v", err)
	}
	files := cm.Files()
	if len(files) != 2 {
		t.Errorf("expected 2 indexed files, got %d: %v", len(files), files)
	}
}

func TestCodeMemory_AllSymbols(t *testing.T) {
	src := `package main
func Alpha() {}
func Beta() {}
`
	dir := t.TempDir()
	path := filepath.Join(dir, "x.go")
	os.WriteFile(path, []byte(src), 0o644) //nolint:errcheck

	cm := treesitter.NewCodeMemory()
	cm.IndexFile(context.Background(), path) //nolint:errcheck

	all := cm.AllSymbols()
	// Should contain at least Alpha and Beta
	names := make(map[string]bool)
	for _, s := range all {
		names[s.Name] = true
	}
	if !names["Alpha"] || !names["Beta"] {
		t.Errorf("expected Alpha and Beta in symbols, got %v", all)
	}
}

// assertSymbol checks that symbols contains at least one entry with the given
// kind and name.
func assertSymbol(t *testing.T, symbols []treesitter.Symbol, kind, name string) {
	t.Helper()
	for _, s := range symbols {
		if s.Kind == kind && s.Name == name {
			return
		}
	}
	t.Errorf("expected symbol kind=%q name=%q; got %v", kind, name, symbols)
}
