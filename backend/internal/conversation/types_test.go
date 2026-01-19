package conversation

import (
	"encoding/json"
	"testing"

	"github.com/hurricanerix/weave/internal/ollama"
)

func TestStateSnapshot_JSON(t *testing.T) {
	tests := []struct {
		name     string
		snapshot StateSnapshot
	}{
		{
			name: "complete snapshot",
			snapshot: StateSnapshot{
				Prompt:        "a cat in space",
				Steps:         28,
				CFG:           7.5,
				Seed:          12345,
				PreviewStatus: PreviewStatusComplete,
				PreviewURL:    "/images/1.png",
			},
		},
		{
			name: "generating snapshot",
			snapshot: StateSnapshot{
				Prompt:        "a dog on the moon",
				Steps:         4,
				CFG:           1.0,
				Seed:          -1,
				PreviewStatus: PreviewStatusGenerating,
				PreviewURL:    "",
			},
		},
		{
			name: "no preview snapshot",
			snapshot: StateSnapshot{
				Prompt:        "a bird flying",
				Steps:         10,
				CFG:           5.0,
				Seed:          0,
				PreviewStatus: PreviewStatusNone,
				PreviewURL:    "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Marshal to JSON
			data, err := json.Marshal(tt.snapshot)
			if err != nil {
				t.Fatalf("failed to marshal: %v", err)
			}

			// Unmarshal back
			var decoded StateSnapshot
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("failed to unmarshal: %v", err)
			}

			// Verify fields match
			if decoded.Prompt != tt.snapshot.Prompt {
				t.Errorf("Prompt: got %q, want %q", decoded.Prompt, tt.snapshot.Prompt)
			}
			if decoded.Steps != tt.snapshot.Steps {
				t.Errorf("Steps: got %d, want %d", decoded.Steps, tt.snapshot.Steps)
			}
			if decoded.CFG != tt.snapshot.CFG {
				t.Errorf("CFG: got %f, want %f", decoded.CFG, tt.snapshot.CFG)
			}
			if decoded.Seed != tt.snapshot.Seed {
				t.Errorf("Seed: got %d, want %d", decoded.Seed, tt.snapshot.Seed)
			}
			if decoded.PreviewStatus != tt.snapshot.PreviewStatus {
				t.Errorf("PreviewStatus: got %q, want %q", decoded.PreviewStatus, tt.snapshot.PreviewStatus)
			}
			if decoded.PreviewURL != tt.snapshot.PreviewURL {
				t.Errorf("PreviewURL: got %q, want %q", decoded.PreviewURL, tt.snapshot.PreviewURL)
			}
		})
	}
}

func TestConversationMessage_JSON(t *testing.T) {
	tests := []struct {
		name string
		msg  ConversationMessage
	}{
		{
			name: "user message without snapshot",
			msg: ConversationMessage{
				ID:        1,
				Role:      RoleUser,
				Content:   "I want a cat picture",
				ToolCalls: nil,
				Snapshot:  nil,
			},
		},
		{
			name: "assistant message with snapshot",
			msg: ConversationMessage{
				ID:      2,
				Role:    RoleAssistant,
				Content: "Here's a prompt for you:\n\nPrompt: a cat",
				Snapshot: &StateSnapshot{
					Prompt:        "a cat",
					Steps:         4,
					CFG:           1.0,
					Seed:          -1,
					PreviewStatus: PreviewStatusNone,
					PreviewURL:    "",
				},
			},
		},
		{
			name: "assistant message with tool calls",
			msg: ConversationMessage{
				ID:      3,
				Role:    RoleAssistant,
				Content: "Updating the prompt...",
				ToolCalls: []ollama.ToolCall{
					{
						Function: ollama.ToolCallFunction{
							Name:      "update_generation",
							Arguments: json.RawMessage(`{"prompt":"a cat","steps":4,"cfg":1.0,"seed":-1,"generate_image":false}`),
						},
					},
				},
				Snapshot: &StateSnapshot{
					Prompt:        "a cat",
					Steps:         4,
					CFG:           1.0,
					Seed:          -1,
					PreviewStatus: PreviewStatusNone,
					PreviewURL:    "",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Marshal to JSON
			data, err := json.Marshal(tt.msg)
			if err != nil {
				t.Fatalf("failed to marshal: %v", err)
			}

			// Unmarshal back
			var decoded ConversationMessage
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("failed to unmarshal: %v", err)
			}

			// Verify basic fields
			if decoded.ID != tt.msg.ID {
				t.Errorf("ID: got %d, want %d", decoded.ID, tt.msg.ID)
			}
			if decoded.Role != tt.msg.Role {
				t.Errorf("Role: got %q, want %q", decoded.Role, tt.msg.Role)
			}
			if decoded.Content != tt.msg.Content {
				t.Errorf("Content: got %q, want %q", decoded.Content, tt.msg.Content)
			}

			// Verify snapshot
			if tt.msg.Snapshot == nil && decoded.Snapshot != nil {
				t.Errorf("Snapshot: got non-nil, want nil")
			}
			if tt.msg.Snapshot != nil && decoded.Snapshot == nil {
				t.Errorf("Snapshot: got nil, want non-nil")
			}
			if tt.msg.Snapshot != nil && decoded.Snapshot != nil {
				if decoded.Snapshot.Prompt != tt.msg.Snapshot.Prompt {
					t.Errorf("Snapshot.Prompt: got %q, want %q", decoded.Snapshot.Prompt, tt.msg.Snapshot.Prompt)
				}
			}
		})
	}
}

func TestConversationMessage_OmitEmpty(t *testing.T) {
	// Verify that nil snapshot is omitted from JSON
	msg := ConversationMessage{
		ID:       1,
		Role:     RoleUser,
		Content:  "test",
		Snapshot: nil,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	// The JSON should not contain "snapshot" key
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("failed to unmarshal to map: %v", err)
	}

	if _, exists := raw["snapshot"]; exists {
		t.Errorf("snapshot key should be omitted when nil, got: %s", string(data))
	}
}

func TestPreviewStatusConstants(t *testing.T) {
	// Verify constants are defined and have expected values
	tests := []struct {
		constant string
		value    string
	}{
		{"PreviewStatusNone", "none"},
		{"PreviewStatusGenerating", "generating"},
		{"PreviewStatusComplete", "complete"},
	}

	for _, tt := range tests {
		t.Run(tt.constant, func(t *testing.T) {
			var actual string
			switch tt.constant {
			case "PreviewStatusNone":
				actual = PreviewStatusNone
			case "PreviewStatusGenerating":
				actual = PreviewStatusGenerating
			case "PreviewStatusComplete":
				actual = PreviewStatusComplete
			}

			if actual != tt.value {
				t.Errorf("%s: got %q, want %q", tt.constant, actual, tt.value)
			}
		})
	}
}
