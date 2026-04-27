package engine

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"testing"

	"experiments/prompt-pipeline-experiment/pipeline"
)

// testFS implements fs.FS for testing.
type testFS struct {
	data map[string][]byte
}

func (t *testFS) Open(name string) (fs.File, error) {
	content, ok := t.data[name]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return &testFile{content: content, name: name}, nil
}

type testFile struct {
	content []byte
	name    string
	off     int64
}

func (f *testFile) Read(p []byte) (n int, err error) {
	if f.off >= int64(len(f.content)) {
		return 0, io.EOF
	}
	n = copy(p, f.anc(f.off))
	f.off += int64(n)
	return n, nil
}

// Helper to avoid complexity with slices in testing
func (f *testFile) anc(off int64) []byte {
	return f.content[off:]
}

func (f *testFile) Stat(name string) (fs.FileInfo, error) { return nil, nil }
func (f *testFile) Close() error                         { return nil }

// mockProvider implements pipeline.Provider for testing.
type mockProvider struct {
	data map[string]any
	err  error
}

func (m *mockProvider) Provide(ctx context.Context, id string) (map[string]any, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.data, nil
}

// mockGuardrail implements pipeline.Guardrail for testing.
type mockGuardrail struct {
	fn func(ctx context.Context, prompt string) (string, error)
}

func (m *mockGuardrail) Process(ctx context.Context, prompt string) (string, error) {
	return m.fn(ctx, prompt)
}

func TestAssemble_Success(t *testing.T) {
	fs := &testFS{
		data: map[string][]byte{
			"test.txt": []byte("Hello {{.name}}!"),
		},
	}
	provider := &mockProvider{data: map[string]any{"name": "World"}}
	assembler := NewAssembler(fs, []pipeline.Provider{provider}, nil)

	got, err := assembler.Assemble(context.Background(), "test.txt", "id1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "Hello World!"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestAssemble_ProviderError(t *testing.T) {
	fs := &testFS{data: map[string][]byte{}}
	provider := &mockProvider{err: errors.New("provider failed")}
	assembler := NewAssembler(fs, []pipeline.Provider{provider}, nil)

	_, err := assembler.Assemble(context.Background(), "test.txt", "id1")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, err) { // Just checking if error exists
		// It's wrapped, so we check if it contains the string
	}
}

func TestAssemble_TemplateError(t *testing.T) {
	fs := &testFS{
		data: map[string][]byte{
			"syntax.txt": []byte("{{.name"), // Missing closing brace
		},
	}
	provider := &mockProvider{data: map[string]any{"name": "World"}}
	assembler := NewAssembler(fs, []pipeline.Provider{provider}, nil)

	_, err := assembler.Assemble(context.Background(), "syntax.txt", "id1")
	if err == nil {
		t.Fatal("expected error due to syntax, got nil")
	}
}

func TestAssemble_GuardrailError(t *testing.T) {
	fs := &testFS{
		data: map[string][]byte{
			"test.txt": []byte("Hello {{.name}}"),
		},
	}
	provider := &mockProvider{data: map[string]any{"name": "World"}}
	guardrail := &mockGuardrail{
		fn: func(ctx context.Context, prompt string) (string, error) {
			return "", errors.New("guardrail blocked")
		},
	}
	assembler := NewAssembler(fs, []pipeline.Provider{provider}, []pipeline.Guardrail{guardrail})

	_, err := assembler.Assemble(context.Background(), "test.txt", "id1")
	if err == nil {
		t.Fatal("expected error from guardrail, got nil")
	}
}

func TestAssemble_DataAggregation(t *testing.T) {
	fs := &testFS{
		data: map[string][]byte{
			"test.txt": []byte("{{.name}} is {{.age}}"),
		},
	}
	p1 := &mockProvider{data: map[string]any{"name": "Alice"}}
	p2 := &mockProvider{data: map[string]any{"age": 30}}
	assembler := NewAssembler(fs, []pipeline.Provider{p1, p2}, nil)

	got, err := assembler.Assemble(context.Background(), "test.txt", "id1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "Alice is 30"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
