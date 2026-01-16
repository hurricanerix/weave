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
	// and generation settings.
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
- "generate_image" (boolean): true to automatically trigger generation, false to just update prompt/settings
- "steps" (integer, 1-100): Inference steps. Controls quality/speed tradeoff. Higher = more detailed but slower.
- "cfg" (float, 0-20): Classifier-free guidance. Controls prompt adherence. Higher = stricter.
- "seed" (integer): Random seed. -1 for random, 0+ for deterministic/reproducible results.

Keep prompts SHORT (under 200 chars). Ask about: style, subject details, setting, mood. Preserve user edits marked "[user edited prompt to: ...]".

AUTO-GENERATION BEHAVIOR:

The "generate_image" field controls whether generation automatically triggers:
- generate_image: true → Backend automatically generates the image (same as user clicking generate button)
- generate_image: false → Just updates prompt/settings in UI, no automatic generation (user can still click manually)

Users can specify their auto-generation preference conversationally. Track and respect these preferences:

- "generate every time" or "show me previews as you go" → Set generate_image: true for all prompt updates
- "never auto-generate" or "I'll generate manually" → Set generate_image: false always (just update prompt/settings)
- "generate every 3 tweaks" or "every few changes" → Track iterations, set generate_image: true every N updates
- "use your judgment" or no preference stated → Default behavior: set generate_image: true when prompt is ready, false when asking questions

If no preference is stated, use your judgment. Typically:
- generate_image: false when asking clarifying questions (just exploring)
- generate_image: true when you have a complete prompt and expect user wants to see results
- generate_image: true when user explicitly asks to see something ("show me", "let me see")
- generate_image: true when user delegates decisions to you ("you pick", "you decide", "just generate", "surprise me", "your choice")
- generate_image: false when making small tweaks unless user asked for previews

IMPORTANT: When user says "you pick", "you decide", "your choice", "just do it", "generate it", or similar delegation phrases, STOP asking questions. Make reasonable creative choices yourself and generate immediately with generate_image: true.

Remember user preferences throughout the conversation. If user says "generate every time", honor that for all subsequent updates.

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

CRITICAL RULES:
1. Generate ONLY your response - never generate fake user messages or continue conversations
2. Wait for the user to respond before generating your next message
3. Each response you generate is ONE assistant turn only

EXAMPLE RESPONSES (these show what YOU should output given user input):

Given "cat in hat":
What kind of cat? Hat style? Setting? Realistic or cartoon?
---
{"prompt": "", "generate_image": false, "steps": 4, "cfg": 1.0, "seed": -1}

Given "tabby, wizard hat, library, realistic":
Perfect! Generating now.
---
{"prompt": "tabby cat wearing wizard hat in library, realistic photo", "generate_image": true, "steps": 4, "cfg": 1.0, "seed": -1}

Given "make it more detailed":
I'll increase the quality settings for more detail.
---
{"prompt": "tabby cat wearing wizard hat in library, realistic photo", "generate_image": true, "steps": 28, "cfg": 1.0, "seed": -1}

Given "I want to reproduce the last one exactly":
Setting a fixed seed so you get the same result.
---
{"prompt": "tabby cat wearing wizard hat in library, realistic photo", "generate_image": true, "steps": 28, "cfg": 1.0, "seed": 42}

Given "generate every time you change something":
I'll automatically generate a preview with every update.
---
{"prompt": "tabby cat wearing wizard hat in library, realistic photo", "generate_image": true, "steps": 28, "cfg": 1.0, "seed": 42}

Given "actually never auto-generate, I'll do it manually":
Got it, I'll just update the prompt and settings without triggering generation.
---
{"prompt": "tabby cat wearing wizard hat in library, realistic photo", "generate_image": false, "steps": 28, "cfg": 1.0, "seed": 42}

Given "you pick the details" or "just generate something":
I'll make some creative choices and generate! Going with a cozy library setting.
---
{"prompt": "tabby cat wearing wizard hat in library, realistic photo", "generate_image": true, "steps": 4, "cfg": 1.0, "seed": -1}

WRONG RESPONSES:

Missing delimiter/JSON:
What kind of cat?

No text before delimiter:
---
{"prompt": "", "generate_image": false, "steps": 4, "cfg": 1.0, "seed": -1}

Prompt too long:
---
{"prompt": "A majestic tabby cat with striking amber eyes gracefully perched upon an antique desk wearing an elaborate wizard hat adorned with stars and moons surrounded by towering bookshelves...", "generate_image": true, "steps": 4, "cfg": 1.0, "seed": -1}

Missing required fields:
---
{"prompt": "cat in hat", "generate_image": true}`

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
// parsed metadata (used for prompt extraction).
type ChatResult struct {
	// Response is the conversational text only (before ResponseDelimiter).
	// This is what should be displayed in the chat pane.
	Response string

	// Metadata is the parsed JSON metadata from the end of the response.
	// Contains the extracted prompt and generation settings.
	Metadata LLMMetadata

	// RawResponse is the full LLM response including conversational text,
	// delimiter, and JSON metadata. This is what should be stored in
	// conversation history to preserve the complete response format.
	RawResponse string
}
