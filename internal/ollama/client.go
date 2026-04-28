// Package ollama provides a client for the Ollama API.
// It supports streaming chat completions and model listing.
package ollama

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

const defaultBaseURL = "http://localhost:11434"

// Role constants for chat messages.
const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
)

// Message represents a chat message.
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// ToolCall represents a request from the model to invoke a tool.
type ToolCall struct {
	ID       string           `json:"id,omitempty"`
	Function ToolCallFunction `json:"function"`
}

// ToolCallFunction holds the function name and arguments for a tool call.
type ToolCallFunction struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// Tool represents a tool definition passed to the model.
type Tool struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

// ToolFunction describes the function a tool exposes.
type ToolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  ToolParameters `json:"parameters"`
}

// ToolParameters describes the JSON Schema for tool input parameters.
type ToolParameters struct {
	Type       string              `json:"type"`
	Properties map[string]Property `json:"properties"`
	Required   []string            `json:"required,omitempty"`
}

// Property describes a single JSON Schema property.
type Property struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

// ChatRequest is sent to the /api/chat endpoint.
type ChatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Tools    []Tool    `json:"tools,omitempty"`
	Stream   bool      `json:"stream"`
	Options  *Options  `json:"options,omitempty"`
}

// Options holds model inference parameters.
type Options struct {
	Temperature float64 `json:"temperature,omitempty"`
	NumCtx      int     `json:"num_ctx,omitempty"`
}

// ChatResponse is a single chunk returned from the /api/chat stream.
type ChatResponse struct {
	Model     string  `json:"model"`
	Message   Message `json:"message"`
	Done      bool    `json:"done"`
	CreatedAt string  `json:"created_at"`
}

// ModelInfo holds basic metadata about an installed model.
type ModelInfo struct {
	Name       string `json:"name"`
	ModifiedAt string `json:"modified_at"`
	Size       int64  `json:"size"`
}

// ModelListResponse is returned by /api/tags.
type ModelListResponse struct {
	Models []ModelInfo `json:"models"`
}

// showModelRequest is sent to /api/show.
type showModelRequest struct {
	Model string `json:"model"`
}

// showModelResponse contains a subset of /api/show fields.
type showModelResponse struct {
	ModelInfo map[string]any `json:"model_info"`
}

// Client is an Ollama API client.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a new Ollama client. If baseURL is empty the default
// localhost address is used.
func NewClient(baseURL string) *Client {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	return &Client{
		baseURL: baseURL,
		// Do not set a global timeout here. Streaming chat requests can
		// legitimately run for a long time; call sites should control
		// deadlines via context.WithTimeout when needed.
		httpClient: &http.Client{},
	}
}

// ListModels returns a list of models available in the local Ollama instance.
func (c *Client) ListModels(ctx context.Context) ([]ModelInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/tags", nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("listing models: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %s", resp.Status)
	}
	var result ModelListResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return result.Models, nil
}

// ModelContextLength returns the model's supported context length from /api/show.
func (c *Client) ModelContextLength(ctx context.Context, model string) (int, error) {
	if strings.TrimSpace(model) == "" {
		return 0, fmt.Errorf("model is required")
	}
	payload, err := json.Marshal(showModelRequest{Model: model})
	if err != nil {
		return 0, fmt.Errorf("marshalling request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/show", bytes.NewReader(payload))
	if err != nil {
		return 0, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("show model: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		msg := string(b)
		if len(b) == 4096 {
			msg += "..."
		}
		return 0, fmt.Errorf("unexpected status %s: %s", resp.Status, msg)
	}

	var out showModelResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return 0, fmt.Errorf("decoding response: %w", err)
	}
	ctxLen, ok := findContextLength(out.ModelInfo)
	if !ok || ctxLen <= 0 {
		return 0, fmt.Errorf("context length not found in model_info")
	}
	return ctxLen, nil
}

// ChatStream sends a chat request to Ollama and streams back response chunks.
// The provided callback is invoked with each partial Message as it arrives.
// When the stream is complete, the callback receives the final message with
// Done == true.
func (c *Client) ChatStream(ctx context.Context, req ChatRequest, onChunk func(ChatResponse) error) error {
	req.Stream = true
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshalling request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		msg := string(b)
		if len(b) == 4096 {
			msg += "..."
		}
		return fmt.Errorf("unexpected status %s: %s", resp.Status, msg)
	}
	scanner := bufio.NewScanner(resp.Body)
	// Default scanner token size is 64K, which can be too small for some
	// streamed JSON chunks. Raise the cap to avoid premature scan failures.
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var chunk ChatResponse
		if err := json.Unmarshal(line, &chunk); err != nil {
			return fmt.Errorf("decoding chunk: %w", err)
		}
		if err := onChunk(chunk); err != nil {
			return err
		}
		if chunk.Done {
			break
		}
	}
	return scanner.Err()
}

// Chat sends a non-streaming chat request and returns the complete response.
func (c *Client) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	req.Stream = false
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshalling request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		msg := string(b)
		if len(b) == 4096 {
			msg += "..."
		}
		return nil, fmt.Errorf("unexpected status %s: %s", resp.Status, msg)
	}
	var result ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return &result, nil
}

func findContextLength(modelInfo map[string]any) (int, bool) {
	if len(modelInfo) == 0 {
		return 0, false
	}

	best := 0
	for k, v := range modelInfo {
		if k != "context_length" && !strings.HasSuffix(k, ".context_length") {
			continue
		}
		n, ok := anyToInt(v)
		if !ok || n <= 0 {
			continue
		}
		if n > best {
			best = n
		}
	}
	if best <= 0 {
		return 0, false
	}
	return best, true
}

func anyToInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int32:
		return int(n), true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			return 0, false
		}
		return int(i), true
	case string:
		i, err := strconv.Atoi(strings.TrimSpace(n))
		if err != nil {
			return 0, false
		}
		return i, true
	default:
		return 0, false
	}
}
