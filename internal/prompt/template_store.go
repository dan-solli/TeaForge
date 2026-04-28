package prompt

import (
	"bytes"
	"embed"
	"fmt"
	"text/template"
)

//go:embed templates/*
var promptTemplateFS embed.FS

func mustLoadPromptTemplate(name string) string {
	content, err := loadPromptTemplate(name)
	if err != nil {
		panic(err)
	}
	return content
}

// MustLoadTemplate loads an embedded prompt template and panics on failure.
func MustLoadTemplate(name string) string {
	return mustLoadPromptTemplate(name)
}

func loadPromptTemplate(name string) (string, error) {
	path := "templates/" + name
	data, err := promptTemplateFS.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read prompt template %q: %w", name, err)
	}
	return string(data), nil
}

func renderPromptTemplate(name string, data any) (string, error) {
	path := "templates/" + name
	tmpl, err := template.ParseFS(promptTemplateFS, path)
	if err != nil {
		return "", fmt.Errorf("parse prompt template %q: %w", name, err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute prompt template %q: %w", name, err)
	}
	return buf.String(), nil
}

// RenderTemplate renders an embedded prompt template with the provided data.
func RenderTemplate(name string, data any) (string, error) {
	return renderPromptTemplate(name, data)
}
