// Package ollama provides a client for communicating with ollama LLM server.
// It handles streaming chat completions and prompt extraction for image generation.
package ollama

import "encoding/json"

// Default configuration constants
const (
	DefaultEndpoint = "http://localhost:11434"
	DefaultModel    = "llama3.1:8b"
	DefaultTimeout  = 60 // seconds
)

// Response format constants (removed - function calling is now the only supported format)

// API endpoints
const (
	EndpointTags = "/api/tags"
	EndpointChat = "/api/chat"
)

// Message roles
const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
)

// Tool represents a function that can be called by the model.
// Ollama's function calling API allows models to request function calls
// as part of their response.
type Tool struct {
	Type     string       `json:"type"`     // Always "function"
	Function ToolFunction `json:"function"` // Function definition
}

// ToolFunction describes a callable function.
// The schema matches OpenAI's function calling format, which ollama supports.
type ToolFunction struct {
	Name        string                 `json:"name"`        // Function name
	Description string                 `json:"description"` // Human-readable description
	Parameters  map[string]interface{} `json:"parameters"`  // JSON Schema for parameters
}

// ToolCall represents a function call made by the model.
// When the model decides to call a function, it returns a ToolCall
// in the response message.
type ToolCall struct {
	Function ToolCallFunction `json:"function"` // Function call details
}

// ToolCallFunction contains the function name and arguments.
type ToolCallFunction struct {
	Name      string          `json:"name"`      // Function name
	Arguments json.RawMessage `json:"arguments"` // Raw JSON arguments (parse later)
}

// Message represents a single message in a conversation.
type Message struct {
	Role      string     `json:"role"`                 // "system", "user", or "assistant"
	Content   string     `json:"content"`              // Message text
	ToolCalls []ToolCall `json:"tool_calls,omitempty"` // Function calls made by the model
}

// ChatOptions contains optional parameters for chat requests.
type ChatOptions struct {
	// Seed for deterministic responses.
	// If nil, ollama uses random seed (non-deterministic).
	// If non-nil (including 0), produces deterministic output with that seed.
	Seed *int64 `json:"seed,omitempty"`
}

// ChatRequest represents a request to ollama's /api/chat endpoint.
type ChatRequest struct {
	Model    string       `json:"model"`             // Model name (e.g., "llama3.2:1b")
	Messages []Message    `json:"messages"`          // Conversation history
	Stream   bool         `json:"stream"`            // Whether to stream response
	Options  *ChatOptions `json:"options,omitempty"` // Optional parameters
	Tools    []Tool       `json:"tools,omitempty"`   // Available tools for function calling
}

// ChatResponse represents a streaming response from ollama's /api/chat endpoint.
// Each line of the streaming response is a JSON object with these fields.
type ChatResponse struct {
	Model      string  `json:"model"`                 // Model that generated response
	CreatedAt  string  `json:"created_at"`            // Timestamp
	Message    Message `json:"message"`               // Response message (partial in stream)
	Done       bool    `json:"done"`                  // True if this is the final response
	DoneReason string  `json:"done_reason,omitempty"` // Reason for completion (e.g., "stop")

	// Final response fields (only present when Done is true)
	TotalDuration      int64 `json:"total_duration,omitempty"`       // Total time in nanoseconds
	LoadDuration       int64 `json:"load_duration,omitempty"`        // Model load time
	PromptEvalCount    int   `json:"prompt_eval_count,omitempty"`    // Tokens in prompt
	PromptEvalDuration int64 `json:"prompt_eval_duration,omitempty"` // Prompt eval time
	EvalCount          int   `json:"eval_count,omitempty"`           // Tokens generated
	EvalDuration       int64 `json:"eval_duration,omitempty"`        // Generation time
}

// TagsResponse represents the response from ollama's /api/tags endpoint.
// Used to verify ollama is running and check available models.
type TagsResponse struct {
	Models []ModelInfo `json:"models"`
}

// ModelInfo represents information about an available model.
type ModelInfo struct {
	Name       string `json:"name"`        // Model name (e.g., "llama3.2:1b")
	ModifiedAt string `json:"modified_at"` // Last modification time
	Size       int64  `json:"size"`        // Model size in bytes
}

// StreamToken represents a token received during streaming.
// Used by the streaming callback to receive tokens as they arrive.
type StreamToken struct {
	Content string // Token text
	Done    bool   // True if this is the final token
}

// LLMMetadata represents the structured metadata extracted from function calls.
// The LLM calls the update_generation function with these parameters instead of
// outputting formatted text. This provides reliable structured output through
// the model's native function calling capability.
type LLMMetadata struct {
	// Prompt is the image generation prompt. Empty string if the LLM is still
	// asking questions and not ready to generate.
	Prompt string `json:"prompt"`

	// GenerateImage controls whether to automatically trigger image generation.
	// When true, the backend invokes generation directly (same path as POST /generate).
	// When false, no automatic generation occurs (user can still click manually).
	// Defaults to false if the field is missing in the JSON.
	GenerateImage bool `json:"generate_image"`

	// Steps is the number of inference steps to use for generation.
	// The web layer clamps this to valid ranges for the selected model.
	Steps int `json:"steps"`

	// CFG is the classifier-free guidance scale.
	// Higher values make the image adhere more strictly to the prompt.
	// The web layer clamps this to valid ranges for the selected model.
	CFG float64 `json:"cfg"`

	// Seed is the random seed for deterministic generation.
	// Using the same seed with the same prompt and settings produces identical images.
	Seed int64 `json:"seed"`
}

// ChatResult represents the complete result of a chat request.
// It contains both the conversational text (displayed to user) and the
// parsed metadata (extracted from function calls).
type ChatResult struct {
	// Response is the conversational text only (before tool call marker).
	// This is what should be displayed in the chat pane.
	Response string

	// Metadata is the parsed metadata from function call arguments.
	// Contains the extracted prompt and generation settings.
	// Only valid when HasToolCall is true.
	Metadata LLMMetadata

	// HasToolCall indicates whether the LLM called the update_generation function.
	// If false, this is a pure conversational response with no generation metadata.
	HasToolCall bool

	// RawResponse is the full LLM response including conversational text
	// and tool call marker/data. This is what should be stored in
	// conversation history to preserve the complete response format.
	RawResponse string
}

// UpdateGenerationTool returns the tool definition for the update_generation function.
// This function allows the LLM to update generation parameters and optionally trigger
// image generation through structured function calling.
func UpdateGenerationTool() Tool {
	return Tool{
		Type: "function",
		Function: ToolFunction{
			Name:        "update_generation",
			Description: "Update image generation prompt and settings. Use this to set or modify the generation parameters and optionally trigger image generation.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"prompt": map[string]interface{}{
						"type":        "string",
						"description": "Image generation prompt. Should be concise (under 200 characters). Empty string if still asking questions.",
					},
					"steps": map[string]interface{}{
						"type":        "integer",
						"description": "Number of inference steps (1-100). Higher values produce more detailed images but take longer. Default: 4 for fast iteration.",
						"minimum":     1,
						"maximum":     100,
					},
					"cfg": map[string]interface{}{
						"type":        "number",
						"description": "Classifier-free guidance scale (0-20). Controls how strictly the image adheres to the prompt. Default: 1.0 for balanced results.",
						"minimum":     0,
						"maximum":     20,
					},
					"seed": map[string]interface{}{
						"type":        "integer",
						"description": "Random seed for generation. Use -1 for random (exploring variations), or a specific value (0+) for deterministic/reproducible results.",
					},
					"generate_image": map[string]interface{}{
						"type":        "boolean",
						"description": "Whether to automatically trigger image generation. Set to true to generate immediately, false to just update settings without generating.",
					},
				},
				"required": []string{"prompt", "steps", "cfg", "seed", "generate_image"},
			},
		},
	}
}
