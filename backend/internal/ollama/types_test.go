package ollama

import (
	"encoding/json"
	"testing"
)

func TestToolSerialization(t *testing.T) {
	tool := Tool{
		Type: "function",
		Function: ToolFunction{
			Name:        "test_function",
			Description: "A test function",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"param1": map[string]interface{}{
						"type":        "string",
						"description": "First parameter",
					},
				},
				"required": []string{"param1"},
			},
		},
	}

	data, err := json.Marshal(tool)
	if err != nil {
		t.Fatalf("failed to marshal tool: %v", err)
	}

	var decoded Tool
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal tool: %v", err)
	}

	if decoded.Type != tool.Type {
		t.Errorf("Type = %q, want %q", decoded.Type, tool.Type)
	}

	if decoded.Function.Name != tool.Function.Name {
		t.Errorf("Function.Name = %q, want %q", decoded.Function.Name, tool.Function.Name)
	}

	if decoded.Function.Description != tool.Function.Description {
		t.Errorf("Function.Description = %q, want %q", decoded.Function.Description, tool.Function.Description)
	}
}

func TestToolCallSerialization(t *testing.T) {
	// Create a tool call with raw JSON arguments
	args := json.RawMessage(`{"prompt": "a cat", "steps": 4, "cfg": 1.0, "seed": -1, "generate": true}`)
	toolCall := ToolCall{
		Function: ToolCallFunction{
			Name:      "update_generation",
			Arguments: args,
		},
	}

	data, err := json.Marshal(toolCall)
	if err != nil {
		t.Fatalf("failed to marshal tool call: %v", err)
	}

	var decoded ToolCall
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal tool call: %v", err)
	}

	if decoded.Function.Name != toolCall.Function.Name {
		t.Errorf("Function.Name = %q, want %q", decoded.Function.Name, toolCall.Function.Name)
	}

	// Verify arguments can be parsed
	var parsedArgs map[string]interface{}
	if err := json.Unmarshal(decoded.Function.Arguments, &parsedArgs); err != nil {
		t.Fatalf("failed to parse arguments: %v", err)
	}

	if parsedArgs["prompt"] != "a cat" {
		t.Errorf("prompt = %v, want %q", parsedArgs["prompt"], "a cat")
	}

	if parsedArgs["generate"] != true {
		t.Errorf("generate = %v, want true", parsedArgs["generate"])
	}
}

func TestChatRequestWithTools(t *testing.T) {
	req := ChatRequest{
		Model: "llama3.1:8b",
		Messages: []Message{
			{Role: RoleUser, Content: "generate a cat"},
		},
		Stream: true,
		Tools: []Tool{
			UpdateGenerationTool(),
		},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal chat request: %v", err)
	}

	var decoded ChatRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal chat request: %v", err)
	}

	if len(decoded.Tools) != 1 {
		t.Fatalf("len(Tools) = %d, want 1", len(decoded.Tools))
	}

	if decoded.Tools[0].Function.Name != "update_generation" {
		t.Errorf("Tools[0].Function.Name = %q, want %q", decoded.Tools[0].Function.Name, "update_generation")
	}
}

func TestMessageWithToolCalls(t *testing.T) {
	args := json.RawMessage(`{"prompt": "a dog", "steps": 8, "cfg": 2.0, "seed": 42, "generate": false}`)
	msg := Message{
		Role:    RoleAssistant,
		Content: "Let me update those settings.",
		ToolCalls: []ToolCall{
			{
				Function: ToolCallFunction{
					Name:      "update_generation",
					Arguments: args,
				},
			},
		},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("failed to marshal message: %v", err)
	}

	var decoded Message
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal message: %v", err)
	}

	if decoded.Role != msg.Role {
		t.Errorf("Role = %q, want %q", decoded.Role, msg.Role)
	}

	if decoded.Content != msg.Content {
		t.Errorf("Content = %q, want %q", decoded.Content, msg.Content)
	}

	if len(decoded.ToolCalls) != 1 {
		t.Fatalf("len(ToolCalls) = %d, want 1", len(decoded.ToolCalls))
	}

	if decoded.ToolCalls[0].Function.Name != "update_generation" {
		t.Errorf("ToolCalls[0].Function.Name = %q, want %q", decoded.ToolCalls[0].Function.Name, "update_generation")
	}

	// Verify arguments
	var parsedArgs map[string]interface{}
	if err := json.Unmarshal(decoded.ToolCalls[0].Function.Arguments, &parsedArgs); err != nil {
		t.Fatalf("failed to parse arguments: %v", err)
	}

	if parsedArgs["prompt"] != "a dog" {
		t.Errorf("prompt = %v, want %q", parsedArgs["prompt"], "a dog")
	}

	if parsedArgs["steps"] != float64(8) {
		t.Errorf("steps = %v, want 8", parsedArgs["steps"])
	}
}

func TestUpdateGenerationTool(t *testing.T) {
	tool := UpdateGenerationTool()

	if tool.Type != "function" {
		t.Errorf("Type = %q, want %q", tool.Type, "function")
	}

	if tool.Function.Name != "update_generation" {
		t.Errorf("Function.Name = %q, want %q", tool.Function.Name, "update_generation")
	}

	if tool.Function.Description == "" {
		t.Error("Function.Description is empty")
	}

	// Verify parameters structure
	params, ok := tool.Function.Parameters["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("Parameters.properties is not a map")
	}

	// Check required parameters exist
	requiredParams := []string{"prompt", "steps", "cfg", "seed", "generate_image"}
	for _, param := range requiredParams {
		if _, exists := params[param]; !exists {
			t.Errorf("missing required parameter: %s", param)
		}
	}

	// Verify required array
	required, ok := tool.Function.Parameters["required"].([]string)
	if !ok {
		t.Fatal("Parameters.required is not a string slice")
	}

	if len(required) != len(requiredParams) {
		t.Errorf("len(required) = %d, want %d", len(required), len(requiredParams))
	}
}

func TestToolCallArgumentsParsing(t *testing.T) {
	tests := []struct {
		name     string
		argsJSON string
		wantErr  bool
	}{
		{
			name:     "valid arguments",
			argsJSON: `{"prompt": "test", "steps": 4, "cfg": 1.0, "seed": -1, "generate": true}`,
			wantErr:  false,
		},
		{
			name:     "invalid json",
			argsJSON: `{invalid}`,
			wantErr:  true,
		},
		{
			name:     "empty object",
			argsJSON: `{}`,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			toolCall := ToolCall{
				Function: ToolCallFunction{
					Name:      "update_generation",
					Arguments: json.RawMessage(tt.argsJSON),
				},
			}

			var args map[string]interface{}
			err := json.Unmarshal(toolCall.Function.Arguments, &args)

			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}

			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}
