// Package ollama provides a client for communicating with ollama LLM server.
// It handles streaming chat completions and prompt extraction for image generation.
package ollama

// Default configuration constants
const (
	DefaultEndpoint = "http://localhost:11434"
	DefaultModel    = "llama3.2:1b"
	DefaultTimeout  = 60 // seconds
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
// then outputs a prompt line when ready to generate.
const SystemPrompt = `You help users create images. Your job is to ask clarifying questions to understand exactly what they want, then provide a prompt for the image generator.

IMPORTANT: Keep prompts VERY SHORT - under 200 characters. Use simple words. Focus only on: subject, style, setting. No flowery language.

When the user describes something, ask about key details:
- Style (realistic, cartoon, painting, etc.)
- Subject details (what kind of cat? what color?)
- Setting/background
- Mood/lighting

Do not assume or inject your own artistic interpretation. Ask the user.

When you have enough information to generate, include exactly one line starting with "Prompt: " followed by the prompt. Only include this line when you're ready to generate.

When you see "[user edited prompt to: ...]" in the conversation, the user has manually edited the prompt. Preserve their changes in your next prompt—do not remove or override their edits unless they explicitly ask you to. Build on what they wrote.

Example:
User: I want a cat wearing a hat
Assistant: A cat in a hat! Let me ask a few questions:
- What kind of cat? (tabby, black, Persian, etc.)
- What style of hat? (wizard hat, top hat, beanie, etc.)
- What's the setting? (indoors, outdoors, plain background?)
- What style? (photo-realistic, illustration, painting?)

Example prompt (good - concise):
Prompt: A tabby cat wearing a blue wizard hat, sitting in a cozy library, realistic photo style, warm lighting

Example prompt (bad - too long):
Prompt: A majestic tabby cat with striking amber eyes gracefully perched upon an antique mahogany desk wearing an elaborate sapphire blue wizard hat adorned with silver stars and moons, surrounded by towering bookshelves filled with ancient leather-bound tomes in a warmly lit Victorian library...`

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

// ChatResult represents the complete result of a chat request.
type ChatResult struct {
	Response string // Full response text
	Prompt   string // Extracted prompt (empty if no "Prompt: " line found)
}
