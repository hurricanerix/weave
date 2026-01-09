// Package conversation provides state management for multi-turn LLM conversations.
// It tracks chat history, current prompt state, and user edits to prompts.
//
// # Design Overview
//
// The conversation package manages state for an image generation assistant where
// users interact with an LLM to refine prompts. The key design decisions are:
//
// 1. Current Prompt is Separate from Message History
//
// The current prompt is tracked as a separate field rather than derived from
// message history. This separation exists because:
//   - The agent may output multiple prompts during a single response
//   - The user may edit the prompt directly without sending a message
//   - The LLM needs to see the current prompt on every request, even if many
//     turns have passed since it was last mentioned
//
// 2. Edit Notification Flow
//
// When users edit the prompt directly (not via chat), the system needs to
// inform the LLM so it can incorporate the changes. The flow is:
//
//	UpdatePrompt("new prompt")  // Sets edited flag
//	NotifyPromptEdited()        // Injects "[user edited prompt to: ...]" into history
//	BuildLLMContext(...)        // LLM sees the notification and current prompt
//
// The notification is injected into the message history so the LLM sees it as
// part of the conversation context. Multiple edits before notification result
// in a single notification with the final value.
//
// 3. LLM Context Construction
//
// BuildLLMContext assembles messages for the LLM in this order:
//   - System prompt (prepended, not stored in history)
//   - All conversation messages (user, assistant, edit notifications)
//   - Trailing context with current prompt (always shows current state)
//
// The trailing context ensures the LLM knows the current prompt even if many
// turns have passed since it was last updated.
//
// 4. Session Management
//
// SessionManager provides thread-safe session isolation using double-check
// locking with RWMutex. Each browser session gets its own conversation state.
// For MVP, sessions are never cleaned up (memory leak is acceptable).
//
// # Thread Safety
//
// Manager is NOT thread-safe. Use SessionManager for concurrent access, which
// provides per-session locking via GetOrCreate.
//
// # Usage Example
//
//	sm := NewSessionManager()
//	m := sm.GetOrCreate(sessionID)
//
//	m.AddUserMessage("I want a cat picture")
//	m.AddAssistantMessage("Here's a prompt:\n\nPrompt: a cat", "a cat")
//
//	// User edits prompt in UI
//	m.UpdatePrompt("a fluffy cat")
//	m.NotifyPromptEdited()
//
//	// Before LLM call
//	context := m.BuildLLMContext(systemPrompt)
package conversation

import "github.com/hurricanerix/weave/internal/ollama"

// Message represents a single message in a conversation.
// This is an alias for ollama.Message to ensure type compatibility
// when constructing LLM context.
type Message = ollama.Message

// Role constants for message roles.
// These are re-exported from ollama for convenience.
const (
	RoleSystem    = ollama.RoleSystem
	RoleUser      = ollama.RoleUser
	RoleAssistant = ollama.RoleAssistant
)

// GenerationSettings holds the current generation parameters for a session.
// These settings control how images are generated (quality, speed, reproducibility).
type GenerationSettings struct {
	// Steps controls the number of generation steps (1-100).
	// Higher values produce more detailed images but take longer.
	Steps int

	// CFG (Classifier-Free Guidance) controls prompt adherence (0-20).
	// Higher values make the image follow the prompt more strictly.
	CFG float64

	// Seed controls reproducibility.
	// -1 means random (new seed each time), 0+ means deterministic.
	Seed int64
}

// Conversation holds the state for a single conversation session.
// It tracks the message history, current prompt, and whether the user
// has edited the prompt since the last agent update.
//
// The current prompt is stored separately from the message history because:
// 1. The agent may output multiple prompts during a conversation
// 2. The user may edit the prompt independently of sending messages
// 3. The LLM needs to see the current prompt state on every request
type Conversation struct {
	// messages is the ordered history of user and assistant messages.
	// System messages for edit notifications are also stored here.
	messages []Message

	// currentPrompt is the current image generation prompt.
	// Updated when the agent outputs a "Prompt: " line or when the user edits.
	currentPrompt string

	// promptEdited is true if the user has edited the prompt since the last
	// time NotifyPromptEdited() was called. Used to inject edit notifications
	// into the conversation history.
	promptEdited bool

	// previousPrompt tracks the prompt value before the last UpdatePrompt call.
	// Used to detect whether the prompt actually changed.
	previousPrompt string
}

// NewConversation creates a new empty conversation.
func NewConversation() *Conversation {
	return &Conversation{
		messages: make([]Message, 0),
	}
}
