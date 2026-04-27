package engine

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"text/template"

	"experiments/prompt-pipeline-experiment/pipeline"
)

// Assembler is responsible for constructing the final prompt from templates and data.
type Assembler struct {
	templateFS fs.FS
	providers  []pipeline.Provider
	guardrails []pipeline.Guardrail
	// templateStr is used for testing purposes to avoid FS dependency in simple tests.
	templateStr string
}

// NewAssembler creates a new Assembler with the provided embedded filesystem and configuration.
func NewAssembler(templateFS fs.FS, providers []pipeline.Provider, guardrails []pipeline.Guardrail) *Assembler {
	return &Assembler{
		templateFS: templateFS,
		providers:  providers,
		guardrails: guardrails,
	}
}

// Assemble takes a template name and a context identifier, and returns the interpolated string.
func (a *Assembler) Assemble(ctx context.Context, templateName string, contextID string) (string, error) {
	// 1. Gather data from all providers
	allData := make(map[string]any)
	for _, p := range a.providers {
		data, err := p.Provide(ctx, contextID)
		if err != nil {
			return "", fmt.Errorf("provider error: %w", err)
		}
		for k, v := range data {
			allData[k] = v
		}
	}

	// 2. Get template content
	var templateContent string
	if a.templateStr != "" {
		templateContent = a.templateStr
	} else {
		// In a real app, the templateName would be used to find the template in the FS.
		// For this experiment, we'll use the templateName provided.
		path := templateName
		templateData, err := fs.ReadFile(a.templateFS, path)
		if err != nil {
			return "", fmt.Errorf("failed to read template %s: %w", path, err)
		}
		templateContent = string(templateData)
	}

	// 3. Parse the template
	tmpl, err := template.New("prompt").Parse(templateContent)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	// 4. Execute the template with the gathered data
	var buf bytes.Buffer
	err = tmpl.Execute(&buf, allData)
	if err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	prompt := buf.String()

	// 5. Run through guardrails
	for _, g := range a.guardrails {
		prompt, err = g.Process(ctx, prompt)
		if err != nil {
			return "", fmt.Errorf("guardrail error: %w", err)
		}
	}

	return prompt, nil
}
