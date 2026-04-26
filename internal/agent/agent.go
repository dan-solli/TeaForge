// Package agent implements the agentic loop for TeaForge, following the
// conventions established by Anthropic for tool-using agents.
//
// The agent maintains a conversation with an Ollama-hosted LLM, dispatches
// tool calls when the model requests them, and streams partial responses back
// to the caller via callbacks.
package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/dan-solli/teaforge/internal/memory"
	"github.com/dan-solli/teaforge/internal/ollama"
	"github.com/dan-solli/teaforge/internal/tools"
	"github.com/dan-solli/teaforge/internal/treesitter"
)

// Event types emitted by the agent during a run.
const (
	EventToken     = "token"      // Partial assistant text chunk
	EventToolCall  = "tool_call"  // The model invoked a tool
	EventToolResult = "tool_result" // A tool returned its output
	EventDone      = "done"       // The agent turn is complete
	EventError     = "error"      // An unrecoverable error occurred
)

// Event is emitted by the agent during a run and consumed by the TUI.
type Event struct {
	Type    string
	Content string // Token text, tool name, tool result, or error message
}

// Config holds the configuration for an Agent instance.
type Config struct {
	Model       string
	OllamaURL   string
	WorkDir     string
	MemoryFile  string // Path to project memory JSON file
	SystemPrompt string
}

// Agent is the central orchestrator: it manages memory, tools and the LLM loop.
type Agent struct {
	cfg     Config
	client  *ollama.Client
	session *memory.SessionMemory
	project *memory.ProjectMemory
	code    *treesitter.CodeMemory
	tools   *tools.Registry
}

// New creates a new Agent with the provided configuration.
func New(cfg Config) (*Agent, error) {
	client := ollama.NewClient(cfg.OllamaURL)
	session := memory.NewSessionMemory()
	project, err := memory.NewProjectMemory(cfg.MemoryFile)
	if err != nil {
		return nil, fmt.Errorf("loading project memory: %w", err)
	}
	code := treesitter.NewCodeMemory()
	registry := tools.NewRegistry(cfg.WorkDir)

	a := &Agent{
		cfg:     cfg,
		client:  client,
		session: session,
		project: project,
		code:    code,
		tools:   registry,
	}
	// Register memory-aware tools
	registry.Register(&saveNoteTool{project: project})
	registry.Register(&searchCodeTool{code: code})
	registry.Register(&indexDirectoryTool{code: code})
	return a, nil
}

// Session returns the session memory (read-only usage from the TUI).
func (a *Agent) Session() *memory.SessionMemory { return a.session }

// Project returns the project memory.
func (a *Agent) Project() *memory.ProjectMemory { return a.project }

// Code returns the code memory.
func (a *Agent) Code() *treesitter.CodeMemory { return a.code }

// Tools returns the tool registry.
func (a *Agent) Tools() *tools.Registry { return a.tools }

// IndexWorkDir indexes the configured working directory into code memory.
func (a *Agent) IndexWorkDir(ctx context.Context) error {
	if a.cfg.WorkDir == "" {
		return nil
	}
	return a.code.IndexDirectory(ctx, a.cfg.WorkDir)
}

// -------------------------------------------------------------------
// Agent loop
// -------------------------------------------------------------------

// Run executes one user turn through the agentic loop.
// It emits Events on the provided channel until the turn completes or fails.
// The channel is closed when Run returns.
func (a *Agent) Run(ctx context.Context, userMessage string, events chan<- Event) {
	defer close(events)

	// Add the user turn to session memory
	a.session.AddTurn(ollama.RoleUser, userMessage)

	// Build messages from session history
	messages := a.buildMessages()

	// Build tool descriptors from registry
	ollamaTools := a.buildToolDescriptors()

	// Agentic loop: continue until the model stops requesting tool calls
	for {
		var assistantContent strings.Builder
		var toolCalls []ollama.ToolCall

		req := ollama.ChatRequest{
			Model:    a.cfg.Model,
			Messages: messages,
			Tools:    ollamaTools,
		}

		streamErr := a.client.ChatStream(ctx, req, func(chunk ollama.ChatResponse) error {
			if chunk.Message.Content != "" {
				assistantContent.WriteString(chunk.Message.Content)
				select {
				case events <- Event{Type: EventToken, Content: chunk.Message.Content}:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			if len(chunk.Message.ToolCalls) > 0 {
				toolCalls = append(toolCalls, chunk.Message.ToolCalls...)
			}
			return nil
		})

		if streamErr != nil {
			select {
			case events <- Event{Type: EventError, Content: streamErr.Error()}:
			case <-ctx.Done():
			}
			return
		}

		// Record assistant's message
		assistantMsg := assistantContent.String()
		if assistantMsg != "" || len(toolCalls) > 0 {
			a.session.AddTurn(ollama.RoleAssistant, assistantMsg)
			messages = append(messages, ollama.Message{
				Role:      ollama.RoleAssistant,
				Content:   assistantMsg,
				ToolCalls: toolCalls,
			})
		}

		// If no tool calls, the turn is complete
		if len(toolCalls) == 0 {
			select {
			case events <- Event{Type: EventDone}:
			case <-ctx.Done():
			}
			return
		}

		// Dispatch tool calls
		for _, tc := range toolCalls {
			toolName := tc.Function.Name
			args := tc.Function.Arguments

			select {
			case events <- Event{Type: EventToolCall, Content: toolName}:
			case <-ctx.Done():
				return
			}

			tool, ok := a.tools.Get(toolName)
			var resultContent string
			if !ok {
				resultContent = fmt.Sprintf("Error: unknown tool %q", toolName)
			} else {
				result := tool.Execute(ctx, args)
				if result.IsErr {
					resultContent = "Error: " + result.Error
					if result.Output != "" {
						resultContent += "\n" + result.Output
					}
				} else {
					resultContent = result.Output
				}
			}

			select {
			case events <- Event{Type: EventToolResult, Content: fmt.Sprintf("[%s] %s", toolName, resultContent)}:
			case <-ctx.Done():
				return
			}

			// Append tool result to conversation
			a.session.AddTurn(ollama.RoleTool, resultContent)
			messages = append(messages, ollama.Message{
				Role:    ollama.RoleTool,
				Content: resultContent,
			})
		}
		// Continue the loop so the model can respond to the tool results
	}
}

// -------------------------------------------------------------------
// Helpers
// -------------------------------------------------------------------

func (a *Agent) buildMessages() []ollama.Message {
	systemPrompt := a.cfg.SystemPrompt
	if systemPrompt == "" {
		systemPrompt = defaultSystemPrompt(a)
	}

	msgs := []ollama.Message{
		{Role: ollama.RoleSystem, Content: systemPrompt},
	}

	for _, t := range a.session.Turns() {
		msgs = append(msgs, ollama.Message{
			Role:    t.Role,
			Content: t.Content,
		})
	}
	return msgs
}

func (a *Agent) buildToolDescriptors() []ollama.Tool {
	var out []ollama.Tool
	for _, t := range a.tools.All() {
		schema := t.InputSchema()
		props, _ := schema["properties"].(map[string]any)
		required, _ := schema["required"].([]string)

		ollamaPropMap := make(map[string]ollama.Property)
		for k, v := range props {
			if vm, ok := v.(map[string]any); ok {
				prop := ollama.Property{}
				if typ, ok := vm["type"].(string); ok {
					prop.Type = typ
				}
				if desc, ok := vm["description"].(string); ok {
					prop.Description = desc
				}
				ollamaPropMap[k] = prop
			}
		}

		out = append(out, ollama.Tool{
			Type: "function",
			Function: ollama.ToolFunction{
				Name:        t.Name(),
				Description: t.Description(),
				Parameters: ollama.ToolParameters{
					Type:       "object",
					Properties: ollamaPropMap,
					Required:   required,
				},
			},
		})
	}
	return out
}

func defaultSystemPrompt(a *Agent) string {
	var sb strings.Builder
	sb.WriteString(`You are TeaForge, an expert software development AI assistant running locally.
You help developers understand, write, and improve code.
You have access to tools that let you read files, write files, edit code,
run commands, search the codebase, and save project notes.

When you need to look at code or project structure, use the available tools.
When you make decisions or discover important information, save it as a project note.
Always explain what you are doing and why.

`)
	// Add context from project notes
	notes := a.project.Notes()
	if len(notes) > 0 {
		sb.WriteString("## Project Memory (decisions and notes)\n")
		for _, n := range notes {
			sb.WriteString(fmt.Sprintf("- [%s] %s\n", n.Category, n.Content))
		}
		sb.WriteString("\n")
	}
	// Add code index summary
	symbols := a.code.AllSymbols()
	if len(symbols) > 0 {
		sb.WriteString(fmt.Sprintf("## Code Index (%d symbols indexed)\n", len(symbols)))
		sb.WriteString("Use the search_code tool to look up specific symbols.\n\n")
	}
	return sb.String()
}

// -------------------------------------------------------------------
// Memory-aware tools
// -------------------------------------------------------------------

type saveNoteTool struct {
	project *memory.ProjectMemory
}

func (t *saveNoteTool) Name() string { return "save_note" }
func (t *saveNoteTool) Description() string {
	return "Save a note or decision to the persistent project memory. " +
		"Use this to record important decisions, discoveries, or TODOs."
}
func (t *saveNoteTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"category": map[string]any{
				"type":        "string",
				"description": "Category for the note (e.g. 'decision', 'todo', 'discovery', 'architecture').",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "The content of the note.",
			},
		},
		"required": []string{"category", "content"},
	}
}
func (t *saveNoteTool) Execute(_ context.Context, params map[string]any) tools.Result {
	category, _ := params["category"].(string)
	content, _ := params["content"].(string)
	if content == "" {
		return tools.Result{IsErr: true, Error: "parameter 'content' is required"}
	}
	note, err := t.project.AddNote(category, content)
	if err != nil {
		return tools.Result{IsErr: true, Error: fmt.Sprintf("saving note: %v", err)}
	}
	return tools.Result{Output: fmt.Sprintf("Note saved with ID %s", note.ID)}
}

type searchCodeTool struct {
	code *treesitter.CodeMemory
}

func (t *searchCodeTool) Name() string { return "search_code" }
func (t *searchCodeTool) Description() string {
	return "Search the code index for symbols matching a query string. " +
		"Returns matching function names, types, variables etc. with their file locations."
}
func (t *searchCodeTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "Symbol name or partial name to search for.",
			},
		},
		"required": []string{"query"},
	}
}
func (t *searchCodeTool) Execute(_ context.Context, params map[string]any) tools.Result {
	query, _ := params["query"].(string)
	if query == "" {
		return tools.Result{IsErr: true, Error: "parameter 'query' is required"}
	}
	symbols := t.code.Search(query)
	if len(symbols) == 0 {
		return tools.Result{Output: "No symbols found matching: " + query}
	}
	var sb strings.Builder
	for _, s := range symbols {
		sb.WriteString(fmt.Sprintf("%s %s (%s:%d) - %s\n", s.Kind, s.Name, s.File, s.Line, s.Snippet))
	}
	return tools.Result{Output: sb.String()}
}

type indexDirectoryTool struct {
	code *treesitter.CodeMemory
}

func (t *indexDirectoryTool) Name() string { return "index_directory" }
func (t *indexDirectoryTool) Description() string {
	return "Index a directory with tree-sitter to build or refresh the code memory. " +
		"Call this when you want to analyse a new project directory."
}
func (t *indexDirectoryTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Directory path to index.",
			},
		},
		"required": []string{"path"},
	}
}
func (t *indexDirectoryTool) Execute(ctx context.Context, params map[string]any) tools.Result {
	path, _ := params["path"].(string)
	if path == "" {
		return tools.Result{IsErr: true, Error: "parameter 'path' is required"}
	}
	if err := t.code.IndexDirectory(ctx, path); err != nil {
		return tools.Result{IsErr: true, Error: fmt.Sprintf("indexing directory: %v", err)}
	}
	files := t.code.Files()
	return tools.Result{Output: fmt.Sprintf("Indexed %d files in %s", len(files), path)}
}

