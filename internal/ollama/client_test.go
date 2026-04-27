package ollama

import (
	"encoding/json"
	"testing"
)

func TestMessageAndToolCallID_JSONRoundTrip(t *testing.T) {
	t.Parallel()

	msg := Message{
		Role:       RoleTool,
		Content:    "ok",
		ToolCallID: "call_1",
	}
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out Message
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.ToolCallID != "call_1" {
		t.Fatalf("tool_call_id = %q, want %q", out.ToolCallID, "call_1")
	}

	toolCall := ToolCall{ID: "tc_123", Function: ToolCallFunction{Name: "read_file", Arguments: map[string]any{"path": "x"}}}
	tcData, err := json.Marshal(toolCall)
	if err != nil {
		t.Fatalf("marshal tool call: %v", err)
	}
	var tcOut ToolCall
	if err := json.Unmarshal(tcData, &tcOut); err != nil {
		t.Fatalf("unmarshal tool call: %v", err)
	}
	if tcOut.ID != "tc_123" {
		t.Fatalf("id = %q, want %q", tcOut.ID, "tc_123")
	}
}
