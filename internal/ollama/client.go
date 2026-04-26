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
	"time"
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
	Role      string     `json:"role"`
	Content   string     `json:"content"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

// ToolCall represents a request from the model to invoke a tool.
type ToolCall struct {
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
		httpClient: &http.Client{
			Timeout: 5 * time.Minute,
		},
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
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status %s: %s", resp.Status, string(b))
	}
	scanner := bufio.NewScanner(resp.Body)
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
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status %s: %s", resp.Status, string(b))
	}
	var result ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return &result, nil
}
