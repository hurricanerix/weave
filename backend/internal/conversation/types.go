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

// StateSnapshot captures generation parameters at a specific point in conversation.
// This is attached to assistant messages that change the prompt or generation settings,
// allowing users to see and restore historical generation states.
type StateSnapshot struct {
	// Prompt is the image generation prompt at this point.
	Prompt string `json:"prompt"`

	// Steps is the number of inference steps (1-100).
	Steps int `json:"steps"`

	// CFG is the classifier-free guidance scale (0-20).
	CFG float64 `json:"cfg"`

	// Seed is the random seed for generation.
	// -1 means random, 0+ means deterministic.
	Seed int64 `json:"seed"`

	// PreviewStatus indicates the state of the preview image for this message.
	// Values: "none" (no preview generated), "generating" (in progress), "complete" (done).
	PreviewStatus string `json:"preview_status"`

	// PreviewURL is the URL or path to the preview image.
	// Empty if PreviewStatus is "none" or "generating".
	PreviewURL string `json:"preview_url"`
}

// ConversationMessage represents a message with additional metadata for conversation history.
// This wraps the basic message concept with stable IDs and optional state snapshots.
// Unlike the ollama.Message which is used for LLM API communication, this type is for
// internal conversation history storage and persistence.
type ConversationMessage struct {
	// ID is a stable integer identifier within the session.
	// IDs are assigned sequentially starting from 1.
	ID int `json:"id"`

	// Role is the message role: "user", "assistant", or "system".
	Role string `json:"role"`

	// Content is the message text.
	Content string `json:"content"`

	// ToolCalls preserves compatibility with ollama.Message.
	// Contains function calls made by the LLM.
	ToolCalls []ollama.ToolCall `json:"tool_calls,omitempty"`

	// Snapshot is the generation state at this point in the conversation.
	// Only set for assistant messages that changed the prompt or settings.
	// Nil for user messages and assistant messages that are pure conversation.
	Snapshot *StateSnapshot `json:"snapshot,omitempty"`
}

// Role constants for message roles.
// These are re-exported from ollama for convenience.
const (
	RoleSystem    = ollama.RoleSystem
	RoleUser      = ollama.RoleUser
	RoleAssistant = ollama.RoleAssistant
)

// Preview status constants for StateSnapshot.PreviewStatus.
const (
	PreviewStatusNone       = "none"       // No preview generated yet
	PreviewStatusGenerating = "generating" // Generation in progress
	PreviewStatusComplete   = "complete"   // Generation complete
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
	// Each message has a stable ID and optional state snapshot.
	messages []ConversationMessage

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

	// nextMessageID is the next ID to assign to a new message.
	// IDs start at 1 and increment sequentially.
	nextMessageID int
}

// NewConversation creates a new empty conversation.
func NewConversation() *Conversation {
	return &Conversation{
		messages:      make([]ConversationMessage, 0),
		nextMessageID: 1, // Start IDs at 1
	}
}

// GetMessages returns a copy of all messages in the conversation.
// This is used for persistence and serialization.
func (c *Conversation) GetMessages() []ConversationMessage {
	return append([]ConversationMessage(nil), c.messages...)
}

// SetMessages replaces all messages in the conversation.
// This is used when deserializing from persistence.
func (c *Conversation) SetMessages(messages []ConversationMessage) {
	c.messages = messages
}

// GetNextMessageID returns the next message ID that will be assigned.
func (c *Conversation) GetNextMessageID() int {
	return c.nextMessageID
}

// SetNextMessageID sets the next message ID to be assigned.
// This is used when deserializing from persistence.
func (c *Conversation) SetNextMessageID(id int) {
	c.nextMessageID = id
}

// GetCurrentPrompt returns the current prompt.
func (c *Conversation) GetCurrentPrompt() string {
	return c.currentPrompt
}

// SetCurrentPrompt sets the current prompt.
// This is used when deserializing from persistence.
func (c *Conversation) SetCurrentPrompt(prompt string) {
	c.currentPrompt = prompt
}

// GetPreviousPrompt returns the previous prompt.
func (c *Conversation) GetPreviousPrompt() string {
	return c.previousPrompt
}

// SetPreviousPrompt sets the previous prompt.
// This is used when deserializing from persistence.
func (c *Conversation) SetPreviousPrompt(prompt string) {
	c.previousPrompt = prompt
}

// IsPromptEdited returns whether the prompt has been edited.
func (c *Conversation) IsPromptEdited() bool {
	return c.promptEdited
}

// SetPromptEdited sets the prompt edited flag.
// This is used when deserializing from persistence.
func (c *Conversation) SetPromptEdited(edited bool) {
	c.promptEdited = edited
}
