// Package ollama provides a client for communicating with ollama LLM server.
// It handles streaming chat completions and prompt extraction for image generation.
package ollama

// Default configuration constants
const (
	DefaultEndpoint = "http://localhost:11434"
	DefaultModel    = "mistral:7b"
	DefaultTimeout  = 60 // seconds
)

// Response format constants
const (
	// ResponseDelimiter is the delimiter that separates conversational text
	// from JSON metadata in LLM responses. The LLM is instructed to end
	// every message with this delimiter followed by JSON containing prompt
	// and ready status.
	ResponseDelimiter = "---"
)

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

// SystemPrompt defines the agent's behavior for conversational image generation.
// The agent asks clarifying questions to understand what the user wants,
// then outputs a prompt when ready to generate.
//
// CRITICAL: Every response MUST end with the delimiter "---" followed by JSON metadata.
// This format is required for reliable parsing and prompt extraction.
const SystemPrompt = `You help users create images. Ask clarifying questions, then provide a prompt for the generator.

FORMAT REQUIRED: End EVERY response with "---" on its own line, then JSON with these fields:

- "prompt" (string): Generation prompt (empty if still asking questions)
- "ready" (boolean): true if ready to generate, false if clarifying
- "steps" (integer, 1-100): Inference steps. Controls quality/speed tradeoff. Higher = more detailed but slower.
- "cfg" (float, 0-20): Classifier-free guidance. Controls prompt adherence. Higher = stricter.
- "seed" (integer): Random seed. -1 for random, 0+ for deterministic/reproducible results.

Keep prompts SHORT (under 200 chars). Ask about: style, subject details, setting, mood. Preserve user edits marked "[user edited prompt to: ...]".

If user rejects after ready=true, set ready back to false and continue asking.

GENERATION SETTINGS GUIDANCE:

Be conservative with settings - only change when user explicitly asks or implies different quality/speed needs.

Steps (1-100):
- Default: 4 (fast iteration, good for exploring ideas)
- Quality: 20-30 (more detailed, slower)
- User says "more detailed" or "higher quality" → increase steps to 20-30
- User says "faster" or "quick preview" → decrease steps to 4-8

CFG (0-20):
- Default: 1.0 (balanced)
- Strict adherence: 3-7 (when results don't match prompt well)
- User says results don't match their description → suggest increasing cfg to 3-7

Seed:
- Default: -1 (random, for exploring variations)
- Deterministic: 0+ (when user wants reproducibility or to iterate on specific result)
- Keep at -1 unless user explicitly wants "same result" or "reproduce this"

Invalid values will be clamped to valid ranges and you will receive feedback.

CORRECT:
User: cat in hat
Assistant: What kind of cat? Hat style? Setting? Realistic or cartoon?
---
{"prompt": "", "ready": false, "steps": 4, "cfg": 1.0, "seed": -1}

User: tabby, wizard hat, library, realistic
Assistant: Perfect! Generating now.
---
{"prompt": "tabby cat wearing wizard hat in library, realistic photo", "ready": true, "steps": 4, "cfg": 1.0, "seed": -1}

User: make it more detailed
Assistant: I'll increase the quality settings for more detail.
---
{"prompt": "tabby cat wearing wizard hat in library, realistic photo", "ready": true, "steps": 28, "cfg": 1.0, "seed": -1}

User: I want to reproduce the last one exactly
Assistant: Setting a fixed seed so you get the same result.
---
{"prompt": "tabby cat wearing wizard hat in library, realistic photo", "ready": true, "steps": 28, "cfg": 1.0, "seed": 42}

WRONG - Missing delimiter/JSON:
Assistant: What kind of cat?

WRONG - No text before delimiter:
---
{"prompt": "", "ready": false, "steps": 4, "cfg": 1.0, "seed": -1}

WRONG - Prompt too long:
---
{"prompt": "A majestic tabby cat with striking amber eyes gracefully perched upon an antique desk wearing an elaborate wizard hat adorned with stars and moons surrounded by towering bookshelves...", "ready": true, "steps": 4, "cfg": 1.0, "seed": -1}

WRONG - Missing required fields:
---
{"prompt": "cat in hat", "ready": true}`

// Message represents a single message in a conversation.
type Message struct {
	Role    string `json:"role"`    // "system", "user", or "assistant"
	Content string `json:"content"` // Message text
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

// LLMMetadata represents the structured JSON metadata that the LLM includes
// at the end of every response after the ResponseDelimiter.
// This replaces text-based prompt extraction with reliable JSON parsing.
type LLMMetadata struct {
	// Prompt is the image generation prompt. Empty string if the LLM is still
	// asking questions and not ready to generate.
	Prompt string `json:"prompt"`

	// Ready indicates whether the LLM has enough information to generate.
	// When true and Prompt is non-empty, the prompt should be used for generation.
	Ready bool `json:"ready"`

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
// parsed metadata (used for prompt extraction).
type ChatResult struct {
	// Response is the conversational text only (before ResponseDelimiter).
	// This is what should be displayed in the chat pane.
	Response string

	// Metadata is the parsed JSON metadata from the end of the response.
	// Contains the extracted prompt and ready status.
	Metadata LLMMetadata

	// RawResponse is the full LLM response including conversational text,
	// delimiter, and JSON metadata. This is what should be stored in
	// conversation history to preserve the complete response format.
	RawResponse string
}
