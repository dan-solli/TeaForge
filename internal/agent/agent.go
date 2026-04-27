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
	"sort"
	"strings"

	"github.com/dan-solli/teaforge/internal/memory"
	"github.com/dan-solli/teaforge/internal/ollama"
	"github.com/dan-solli/teaforge/internal/prompt"
	"github.com/dan-solli/teaforge/internal/prompt/guardrails"
	sesslog "github.com/dan-solli/teaforge/internal/session"
	"github.com/dan-solli/teaforge/internal/tools"
	"github.com/dan-solli/teaforge/internal/treesitter"
)

// Event types emitted by the agent during a run.
const (
	EventToken      = "token"       // Partial assistant text chunk
	EventToolCall   = "tool_call"   // The model invoked a tool
	EventToolResult = "tool_result" // A tool returned its output
	EventContext    = "context"     // Prompt budget and compaction stats
	EventDone       = "done"        // The agent turn is complete
	EventError      = "error"       // An unrecoverable error occurred
)

// Event is emitted by the agent during a run and consumed by the TUI.
type Event struct {
	Type    string
	Content string // Token text, tool name, tool result, or error message
}

// Config holds the configuration for an Agent instance.
type Config struct {
	Model        string
	OllamaURL    string
	WorkDir      string
	MemoryFile   string // Path to project memory JSON file
	SessionsDir  string // Directory for session logs; empty disables logging
	SystemPrompt string
	NumCtx       int
}

// Agent is the central orchestrator: it manages memory, tools and the LLM loop.
type Agent struct {
	cfg           Config
	client        *ollama.Client
	session       *memory.SessionMemory
	resumeSummary string
	project       *memory.ProjectMemory
	code          *treesitter.CodeMemory
	tools         *tools.Registry
	pipeline      *prompt.Pipeline
	sessionLog    *sesslog.Log // nil when logging is disabled
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

	var sl *sesslog.Log
	if cfg.SessionsDir != "" {
		sl, err = sesslog.New(cfg.SessionsDir, cfg.Model, cfg.WorkDir)
		if err != nil {
			// Non-fatal: the agent works without logging.
			fmt.Printf("warning: could not create session log: %v\n", err)
			sl = nil
		}
	}

	a := &Agent{
		cfg:        cfg,
		client:     client,
		session:    session,
		project:    project,
		code:       code,
		tools:      registry,
		sessionLog: sl,
	}
	a.pipeline = prompt.NewDefaultPipeline([]prompt.Guardrail{
		guardrails.NewSnapshotGuardrail(a.AppendSessionLog),
	})
	a.pipeline.SetTokenBudget(cfg.NumCtx)
	a.pipeline.SetCompactor(newLLMCompactor(client, cfg.Model, cfg.NumCtx))
	// Register memory-aware tools
	registry.Register(&saveNoteTool{project: project})
	registry.Register(&recallNotesTool{project: project})
	registry.Register(&listNoteCategoriesTool{project: project})
	registry.Register(&searchCodeTool{code: code})
	registry.Register(&indexDirectoryTool{code: code})
	return a, nil
}

// Session returns the session memory (read-only usage from the TUI).
func (a *Agent) Session() *memory.SessionMemory { return a.session }

// ResetSession clears live turns and resume summary context.
func (a *Agent) ResetSession() {
	a.session.Clear()
	a.resumeSummary = ""
}

// Project returns the project memory.
func (a *Agent) Project() *memory.ProjectMemory { return a.project }

// Code returns the code memory.
func (a *Agent) Code() *treesitter.CodeMemory { return a.code }

// Tools returns the tool registry.
func (a *Agent) Tools() *tools.Registry { return a.tools }

// AppendSessionLog appends a turn to the persistent session log.
// It is a no-op when session logging is disabled (no SessionsDir configured).
func (a *Agent) AppendSessionLog(role, content string) error {
	return a.sessionLog.Append(role, content)
}

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
func (a *Agent) Run(ctx context.Context, userMessage string, attachedPaths []string, events chan<- Event) {
	defer close(events)

	// Add the user turn to session memory
	a.session.AddTurn(ollama.RoleUser, userMessage)

	// Build messages through the prompt pipeline.
	messages, trace, buildErr := a.pipeline.Build(ctx, &prompt.Request{
		SystemPrompt:    a.cfg.SystemPrompt,
		WorkDir:         a.cfg.WorkDir,
		Model:           a.cfg.Model,
		UserMessage:     userMessage,
		AttachedPaths:   attachedPaths,
		ResumeSummary:   a.resumeSummary,
		NumCtx:          a.cfg.NumCtx,
		SessionTurns:    a.session.Turns(),
		ProjectNotes:    a.project.Notes(),
		CodeSymbolCount: len(a.code.AllSymbols()),
	})
	if buildErr != nil {
		select {
		case events <- Event{Type: EventError, Content: buildErr.Error()}:
		case <-ctx.Done():
		}
		return
	}
	if trace != nil {
		if trace.Summary != "" {
			_ = a.sessionLog.Append(sesslog.RoleSummary, trace.Summary)
		}
		contextStats := fmt.Sprintf("%d%% (%d/%d tokens)", trace.FillPercent, trace.UsedTokens, trace.Budget.Total)
		if trace.Compacted {
			contextStats += " • compacted"
		}
		select {
		case events <- Event{Type: EventContext, Content: contextStats}:
		case <-ctx.Done():
			return
		}
	}

	// Log the system prompt (first element) and the user message.
	// The system prompt is rebuilt each run so we record the exact version used.
	if len(messages) > 0 && messages[0].Role == ollama.RoleSystem {
		_ = a.sessionLog.Append(sesslog.RoleSystem, messages[0].Content)
	}
	_ = a.sessionLog.Append(sesslog.RoleUser, userMessage)

	// Build tool descriptors from registry
	ollamaTools := a.buildToolDescriptors()

	// Agentic loop: continue until the model stops requesting tool calls
	for {
		var assistantContent strings.Builder
		var toolCalls []ollama.ToolCall
		var opts *ollama.Options
		if a.cfg.NumCtx > 0 {
			opts = &ollama.Options{NumCtx: a.cfg.NumCtx}
		}

		req := ollama.ChatRequest{
			Model:    a.cfg.Model,
			Messages: messages,
			Tools:    ollamaTools,
			Options:  opts,
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
		for i := range toolCalls {
			if toolCalls[i].ID == "" {
				toolCalls[i].ID = fmt.Sprintf("tool_call_%d", i+1)
			}
		}
		if assistantMsg != "" || len(toolCalls) > 0 {
			a.session.AddTurn(ollama.RoleAssistant, assistantMsg)
			// Note: assistant response is logged by the TUI in handleAgentEvent/agentDoneMsg
			// so that the logged text matches exactly what was displayed.
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

			_ = a.sessionLog.Append(sesslog.RoleToolCall, fmt.Sprintf("id=%s %s: %v", tc.ID, toolName, args))
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
			_ = a.sessionLog.Append(sesslog.RoleTool, resultContent)
			messages = append(messages, ollama.Message{
				Role:       ollama.RoleTool,
				Content:    resultContent,
				ToolCallID: tc.ID,
			})
		}
		// Continue the loop so the model can respond to the tool results
	}
}

// ResumeFromLog reconstructs session memory from a prior session log.
func (a *Agent) ResumeFromLog(path string) error {
	log, err := sesslog.LoadFromFile(path)
	if err != nil {
		return err
	}

	a.ResetSession()
	for _, turn := range log.Turns {
		switch turn.Role {
		case sesslog.RoleSummary:
			a.resumeSummary = turn.Content
		case sesslog.RoleUser:
			a.session.AddTurn(ollama.RoleUser, turn.Content)
		case sesslog.RoleAssistant:
			a.session.AddTurn(ollama.RoleAssistant, turn.Content)
		case sesslog.RoleTool:
			a.session.AddTurn(ollama.RoleTool, turn.Content)
		}
	}

	if log.Model != "" {
		a.cfg.Model = log.Model
	}
	return nil
}

// -------------------------------------------------------------------
// Helpers
// -------------------------------------------------------------------

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

type recallNotesTool struct {
	project *memory.ProjectMemory
}

func (t *recallNotesTool) Name() string { return "recall_notes" }
func (t *recallNotesTool) Description() string {
	return "Recall notes from project memory by query and optional category. " +
		"Use this to fetch relevant prior decisions and discoveries on demand."
}
func (t *recallNotesTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "Text to match against note content and category.",
			},
			"category": map[string]any{
				"type":        "string",
				"description": "Optional category filter (e.g. architecture, pinned, decision).",
			},
		},
		"required": []string{"query"},
	}
}
func (t *recallNotesTool) Execute(_ context.Context, params map[string]any) tools.Result {
	query, _ := params["query"].(string)
	category, _ := params["category"].(string)
	query = strings.TrimSpace(query)
	category = strings.TrimSpace(category)
	if query == "" {
		return tools.Result{IsErr: true, Error: "parameter 'query' is required"}
	}

	queryLC := strings.ToLower(query)
	catLC := strings.ToLower(category)
	matches := make([]memory.Note, 0)
	for _, n := range t.project.Notes() {
		noteCat := strings.ToLower(strings.TrimSpace(n.Category))
		if catLC != "" && noteCat != catLC {
			continue
		}
		contentLC := strings.ToLower(n.Content)
		if strings.Contains(contentLC, queryLC) || strings.Contains(noteCat, queryLC) {
			matches = append(matches, n)
		}
	}

	if len(matches) == 0 {
		return tools.Result{Output: fmt.Sprintf("No notes found matching query: %s", query)}
	}

	if len(matches) > 20 {
		matches = matches[:20]
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d note(s):\n", len(matches)))
	for _, n := range matches {
		sb.WriteString(fmt.Sprintf("- [%s] %s (id=%s)\n", n.Category, n.Content, n.ID))
	}
	return tools.Result{Output: strings.TrimSpace(sb.String())}
}

type listNoteCategoriesTool struct {
	project *memory.ProjectMemory
}

func (t *listNoteCategoriesTool) Name() string { return "list_note_categories" }
func (t *listNoteCategoriesTool) Description() string {
	return "List available project note categories with counts."
}
func (t *listNoteCategoriesTool) InputSchema() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}
func (t *listNoteCategoriesTool) Execute(_ context.Context, _ map[string]any) tools.Result {
	counts := make(map[string]int)
	for _, n := range t.project.Notes() {
		cat := strings.TrimSpace(n.Category)
		if cat == "" {
			cat = "uncategorized"
		}
		counts[cat]++
	}
	if len(counts) == 0 {
		return tools.Result{Output: "No note categories available."}
	}

	cats := make([]string, 0, len(counts))
	for cat := range counts {
		cats = append(cats, cat)
	}
	sort.Strings(cats)

	var sb strings.Builder
	sb.WriteString("Note categories:\n")
	for _, cat := range cats {
		sb.WriteString(fmt.Sprintf("- %s (%d)\n", cat, counts[cat]))
	}
	return tools.Result{Output: strings.TrimSpace(sb.String())}
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
