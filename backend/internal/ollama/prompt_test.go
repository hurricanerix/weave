package ollama

import "testing"

func TestParseResponse(t *testing.T) {
	tests := []struct {
		name              string
		response          string
		wantText          string
		wantPrompt        string
		wantGenerateImage bool
		wantSteps         int
		wantCFG           float64
		wantSeed          int64
		wantHasToolCall   bool
		wantErr           bool
	}{
		{
			name:              "function call with generate_image true",
			response:          "Perfect! Generating now.\n__TOOL_CALLS__\n[{\"function\":{\"name\":\"update_generation\",\"arguments\":\"{\\\"prompt\\\":\\\"a cat in space\\\",\\\"steps\\\":28,\\\"cfg\\\":7.5,\\\"seed\\\":42,\\\"generate_image\\\":true}\"}}]",
			wantText:          "Perfect! Generating now.",
			wantPrompt:        "a cat in space",
			wantGenerateImage: true,
			wantSteps:         28,
			wantCFG:           7.5,
			wantSeed:          42,
			wantHasToolCall:   true,
			wantErr:           false,
		},
		{
			name:              "function call with generate_image false",
			response:          "What kind of cat?\n__TOOL_CALLS__\n[{\"function\":{\"name\":\"update_generation\",\"arguments\":\"{\\\"prompt\\\":\\\"\\\",\\\"steps\\\":4,\\\"cfg\\\":1.0,\\\"seed\\\":-1,\\\"generate_image\\\":false}\"}}]",
			wantText:          "What kind of cat?",
			wantPrompt:        "",
			wantGenerateImage: false,
			wantSteps:         4,
			wantCFG:           1.0,
			wantSeed:          -1,
			wantHasToolCall:   true,
			wantErr:           false,
		},
		{
			name:            "no tool calls - pure conversational response",
			response:        "This response has no tool calls",
			wantText:        "This response has no tool calls",
			wantHasToolCall: false,
			wantErr:         false,
		},
		{
			name:            "invalid tool call JSON - falls back to conversational",
			response:        "Text before\n__TOOL_CALLS__\n{invalid json}",
			wantText:        "Text before\n__TOOL_CALLS__\n{invalid json}",
			wantHasToolCall: false,
			wantErr:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotText, gotMetadata, gotHasToolCall, gotErr := parseResponse(tt.response)

			// Check error
			if (gotErr != nil) != tt.wantErr {
				t.Errorf("parseResponse() error = %v, wantErr %v", gotErr, tt.wantErr)
				return
			}

			// If we expected an error, don't check the other return values
			if tt.wantErr {
				return
			}

			// Check hasToolCall
			if gotHasToolCall != tt.wantHasToolCall {
				t.Errorf("parseResponse() hasToolCall = %v, want %v", gotHasToolCall, tt.wantHasToolCall)
			}

			// Check conversational text
			if gotText != tt.wantText {
				t.Errorf("parseResponse() text = %q, want %q", gotText, tt.wantText)
			}

			// Only check metadata if we had a tool call
			if !tt.wantHasToolCall {
				return
			}

			// Check metadata fields
			if gotMetadata.Prompt != tt.wantPrompt {
				t.Errorf("parseResponse() metadata.Prompt = %q, want %q", gotMetadata.Prompt, tt.wantPrompt)
			}
			if gotMetadata.GenerateImage != tt.wantGenerateImage {
				t.Errorf("parseResponse() metadata.GenerateImage = %v, want %v", gotMetadata.GenerateImage, tt.wantGenerateImage)
			}
			if gotMetadata.Steps != tt.wantSteps {
				t.Errorf("parseResponse() metadata.Steps = %d, want %d", gotMetadata.Steps, tt.wantSteps)
			}
			if gotMetadata.CFG != tt.wantCFG {
				t.Errorf("parseResponse() metadata.CFG = %f, want %f", gotMetadata.CFG, tt.wantCFG)
			}
			if gotMetadata.Seed != tt.wantSeed {
				t.Errorf("parseResponse() metadata.Seed = %d, want %d", gotMetadata.Seed, tt.wantSeed)
			}
		})
	}
}

func TestParseToolCalls(t *testing.T) {
	tests := []struct {
		name              string
		toolCalls         []ToolCall
		wantPrompt        string
		wantGenerateImage bool
		wantSteps         int
		wantCFG           float64
		wantSeed          int64
		wantErr           bool
		wantErrMsg        string
	}{
		{
			name: "valid update_generation call",
			toolCalls: []ToolCall{
				{
					Function: ToolCallFunction{
						Name:      "update_generation",
						Arguments: []byte(`{"prompt": "a cat in space", "steps": 28, "cfg": 7.5, "seed": 42, "generate_image": true}`),
					},
				},
			},
			wantPrompt:        "a cat in space",
			wantGenerateImage: true,
			wantSteps:         28,
			wantCFG:           7.5,
			wantSeed:          42,
			wantErr:           false,
		},
		{
			name: "valid update_generation call with generate_image false",
			toolCalls: []ToolCall{
				{
					Function: ToolCallFunction{
						Name:      "update_generation",
						Arguments: []byte(`{"prompt": "", "steps": 4, "cfg": 1.0, "seed": -1, "generate_image": false}`),
					},
				},
			},
			wantPrompt:        "",
			wantGenerateImage: false,
			wantSteps:         4,
			wantCFG:           1.0,
			wantSeed:          -1,
			wantErr:           false,
		},
		{
			name: "multiple tool calls, first is update_generation",
			toolCalls: []ToolCall{
				{
					Function: ToolCallFunction{
						Name:      "update_generation",
						Arguments: []byte(`{"prompt": "test prompt", "steps": 20, "cfg": 5.0, "seed": 100, "generate_image": true}`),
					},
				},
				{
					Function: ToolCallFunction{
						Name:      "other_function",
						Arguments: []byte(`{"data": "value"}`),
					},
				},
			},
			wantPrompt:        "test prompt",
			wantGenerateImage: true,
			wantSteps:         20,
			wantCFG:           5.0,
			wantSeed:          100,
			wantErr:           false,
		},
		{
			name: "stringified values (LLM type coercion)",
			toolCalls: []ToolCall{
				{
					Function: ToolCallFunction{
						Name:      "update_generation",
						Arguments: []byte(`{"prompt": "cat dancing on corner", "steps": "20", "cfg": "4.3", "seed": "-1", "generate_image": "true"}`),
					},
				},
			},
			wantPrompt:        "cat dancing on corner",
			wantGenerateImage: true,
			wantSteps:         20,
			wantCFG:           4.3,
			wantSeed:          -1,
			wantErr:           false,
		},
		{
			name: "mixed types (some string, some proper)",
			toolCalls: []ToolCall{
				{
					Function: ToolCallFunction{
						Name:      "update_generation",
						Arguments: []byte(`{"prompt": "a cat", "steps": 28, "cfg": "7.5", "seed": -1, "generate_image": "false"}`),
					},
				},
			},
			wantPrompt:        "a cat",
			wantGenerateImage: false,
			wantSteps:         28,
			wantCFG:           7.5,
			wantSeed:          -1,
			wantErr:           false,
		},
		{
			name:       "empty tool calls",
			toolCalls:  []ToolCall{},
			wantErr:    true,
			wantErrMsg: "no tool calls found",
		},
		{
			name: "wrong function name",
			toolCalls: []ToolCall{
				{
					Function: ToolCallFunction{
						Name:      "other_function",
						Arguments: []byte(`{"data": "value"}`),
					},
				},
			},
			wantErr:    true,
			wantErrMsg: "update_generation tool call not found",
		},
		{
			name: "invalid JSON in arguments",
			toolCalls: []ToolCall{
				{
					Function: ToolCallFunction{
						Name:      "update_generation",
						Arguments: []byte(`{invalid json}`),
					},
				},
			},
			wantErr:    true,
			wantErrMsg: "failed to parse tool call arguments",
		},
		{
			name: "missing required field prompt",
			toolCalls: []ToolCall{
				{
					Function: ToolCallFunction{
						Name:      "update_generation",
						Arguments: []byte(`{"steps": 28, "cfg": 7.5, "seed": 42, "generate_image": true}`),
					},
				},
			},
			wantErr:    true,
			wantErrMsg: "JSON missing required fields",
		},
		{
			name: "missing required field steps",
			toolCalls: []ToolCall{
				{
					Function: ToolCallFunction{
						Name:      "update_generation",
						Arguments: []byte(`{"prompt": "a cat", "cfg": 7.5, "seed": 42, "generate_image": true}`),
					},
				},
			},
			wantErr:    true,
			wantErrMsg: "JSON missing required fields",
		},
		{
			name: "missing required field cfg",
			toolCalls: []ToolCall{
				{
					Function: ToolCallFunction{
						Name:      "update_generation",
						Arguments: []byte(`{"prompt": "a cat", "steps": 28, "seed": 42, "generate_image": true}`),
					},
				},
			},
			wantErr:    true,
			wantErrMsg: "JSON missing required fields",
		},
		{
			name: "missing required field seed",
			toolCalls: []ToolCall{
				{
					Function: ToolCallFunction{
						Name:      "update_generation",
						Arguments: []byte(`{"prompt": "a cat", "steps": 28, "cfg": 7.5, "generate_image": true}`),
					},
				},
			},
			wantErr:    true,
			wantErrMsg: "JSON missing required fields",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metadata, err := parseToolCalls(tt.toolCalls)

			if tt.wantErr {
				if err == nil {
					t.Errorf("parseToolCalls() expected error, got nil")
					return
				}
				if tt.wantErrMsg != "" && !contains(err.Error(), tt.wantErrMsg) {
					t.Errorf("parseToolCalls() error = %q, want containing %q", err.Error(), tt.wantErrMsg)
				}
				return
			}

			if err != nil {
				t.Errorf("parseToolCalls() unexpected error: %v", err)
				return
			}

			if metadata.Prompt != tt.wantPrompt {
				t.Errorf("Prompt = %q, want %q", metadata.Prompt, tt.wantPrompt)
			}
			if metadata.GenerateImage != tt.wantGenerateImage {
				t.Errorf("GenerateImage = %v, want %v", metadata.GenerateImage, tt.wantGenerateImage)
			}
			if metadata.Steps != tt.wantSteps {
				t.Errorf("Steps = %d, want %d", metadata.Steps, tt.wantSteps)
			}
			if metadata.CFG != tt.wantCFG {
				t.Errorf("CFG = %f, want %f", metadata.CFG, tt.wantCFG)
			}
			if metadata.Seed != tt.wantSeed {
				t.Errorf("Seed = %d, want %d", metadata.Seed, tt.wantSeed)
			}
		})
	}
}

func TestExtractToolCallsFromResponse(t *testing.T) {
	tests := []struct {
		name                  string
		response              string
		wantConversational    string
		wantHasToolCalls      bool
		wantToolCallCount     int
		wantFirstFunctionName string
	}{
		{
			name:                  "response with tool calls",
			response:              "Here's the image!\n__TOOL_CALLS__\n[{\"function\":{\"name\":\"update_generation\",\"arguments\":\"{\\\"prompt\\\":\\\"a cat\\\",\\\"steps\\\":28,\\\"cfg\\\":7.5,\\\"seed\\\":42,\\\"generate_image\\\":true}\"}}]",
			wantConversational:    "Here's the image!",
			wantHasToolCalls:      true,
			wantToolCallCount:     1,
			wantFirstFunctionName: "update_generation",
		},
		{
			name:               "response without tool calls",
			response:           "Just a regular response",
			wantConversational: "Just a regular response",
			wantHasToolCalls:   false,
			wantToolCallCount:  0,
		},
		{
			name:               "response with marker but invalid JSON",
			response:           "Text before\n__TOOL_CALLS__\n{invalid json}",
			wantConversational: "Text before\n__TOOL_CALLS__\n{invalid json}",
			wantHasToolCalls:   false,
			wantToolCallCount:  0,
		},
		{
			name:               "empty response",
			response:           "",
			wantConversational: "",
			wantHasToolCalls:   false,
			wantToolCallCount:  0,
		},
		{
			name:               "only marker, no JSON",
			response:           "Text\n__TOOL_CALLS__\n",
			wantConversational: "Text\n__TOOL_CALLS__\n",
			wantHasToolCalls:   false,
			wantToolCallCount:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conversational, toolCalls, hasToolCalls := extractToolCallsFromResponse(tt.response)

			if conversational != tt.wantConversational {
				t.Errorf("conversational = %q, want %q", conversational, tt.wantConversational)
			}
			if hasToolCalls != tt.wantHasToolCalls {
				t.Errorf("hasToolCalls = %v, want %v", hasToolCalls, tt.wantHasToolCalls)
			}
			if len(toolCalls) != tt.wantToolCallCount {
				t.Errorf("toolCallCount = %d, want %d", len(toolCalls), tt.wantToolCallCount)
			}
			if tt.wantToolCallCount > 0 && toolCalls[0].Function.Name != tt.wantFirstFunctionName {
				t.Errorf("first function name = %q, want %q", toolCalls[0].Function.Name, tt.wantFirstFunctionName)
			}
		})
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
