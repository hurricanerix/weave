package conversation

const (
	// MaxHistorySize is the maximum number of messages allowed in conversation history.
	// When this limit is reached, the oldest messages are removed to make room for new ones.
	// This prevents unbounded memory growth in long-running sessions.
	MaxHistorySize = 100
)

// Manager provides operations for managing a conversation.
// It wraps a Conversation and provides methods for adding messages,
// tracking prompt state, and constructing LLM context.
//
// Manager is not thread-safe. For concurrent access across HTTP requests,
// use SessionManager which provides per-session locking.
type Manager struct {
	conv *Conversation
}

// NewManager creates a new conversation manager with an empty conversation.
func NewManager() *Manager {
	return &Manager{
		conv: NewConversation(),
	}
}

// AddUserMessage adds a user message to the conversation history.
// If the history exceeds MaxHistorySize, the oldest messages are removed.
func (m *Manager) AddUserMessage(content string) {
	m.conv.messages = append(m.conv.messages, Message{
		Role:    RoleUser,
		Content: content,
	})
	m.trimHistory()
}

// AddAssistantMessage adds an assistant message to the conversation history
// and updates the current prompt if provided.
//
// The prompt parameter should be the extracted prompt from the assistant's
// response (via ollama.ExtractPrompt). If empty, the current prompt is unchanged.
// If the history exceeds MaxHistorySize, the oldest messages are removed.
func (m *Manager) AddAssistantMessage(content string, prompt string) {
	m.conv.messages = append(m.conv.messages, Message{
		Role:    RoleAssistant,
		Content: content,
	})

	// Update current prompt if the assistant provided one.
	// Note: promptEdited flag is managed by UpdatePrompt() when users
	// directly edit prompts, not when agents update them via this method.
	if prompt != "" {
		m.conv.previousPrompt = m.conv.currentPrompt
		m.conv.currentPrompt = prompt
	}

	m.trimHistory()
}

// GetHistory returns a copy of the message history.
// The returned slice is safe to modify without affecting the conversation.
func (m *Manager) GetHistory() []Message {
	if len(m.conv.messages) == 0 {
		return nil
	}
	// Return a copy to prevent external modification
	history := make([]Message, len(m.conv.messages))
	copy(history, m.conv.messages)
	return history
}

// GetCurrentPrompt returns the current image generation prompt.
func (m *Manager) GetCurrentPrompt() string {
	return m.conv.currentPrompt
}

// Clear resets the conversation to an empty state.
// All messages are removed and the prompt is cleared.
//
// The underlying message slice capacity is preserved to avoid reallocations
// in active sessions. For sessions that have grown very large, consider
// creating a new Manager instead.
func (m *Manager) Clear() {
	m.conv.messages = m.conv.messages[:0]
	m.conv.currentPrompt = ""
	m.conv.previousPrompt = ""
	m.conv.promptEdited = false
}

// UpdatePrompt updates the current prompt with a user-provided value.
// This is called when the user directly edits the prompt in the UI.
//
// If the new prompt differs from the current prompt, the edited flag is set.
// This flag is used by NotifyPromptEdited to inject a notification into
// the conversation history so the agent knows the user made changes.
func (m *Manager) UpdatePrompt(newPrompt string) {
	if newPrompt != m.conv.currentPrompt {
		m.conv.previousPrompt = m.conv.currentPrompt
		m.conv.currentPrompt = newPrompt
		m.conv.promptEdited = true
	}
}

// IsPromptEdited returns true if the user has edited the prompt since
// the last call to NotifyPromptEdited.
func (m *Manager) IsPromptEdited() bool {
	return m.conv.promptEdited
}

// NotifyPromptEdited injects a user message into the conversation history
// notifying the agent that the user has edited the prompt.
//
// The injected message has the format:
//
//	[user edited prompt to: "<current prompt>"]
//
// This allows the agent to see what the user wrote and incorporate their
// changes into future prompts.
//
// Note: We use RoleUser instead of RoleSystem because ollama requires
// system messages to be first in the conversation. This notification
// represents a user action anyway.
//
// If the prompt hasn't been edited (edited flag is false), this method
// does nothing.
//
// After injection, the edited flag is cleared.
// If the history exceeds MaxHistorySize, the oldest messages are removed.
func (m *Manager) NotifyPromptEdited() {
	if !m.conv.promptEdited {
		return
	}

	// Inject user message with the current prompt (not system - ollama
	// requires system messages to be first in conversation)
	notification := Message{
		Role:    RoleUser,
		Content: `[user edited prompt to: "` + m.conv.currentPrompt + `"]`,
	}
	m.conv.messages = append(m.conv.messages, notification)

	// Clear the edited flag
	m.conv.promptEdited = false

	m.trimHistory()
}

// trimHistory removes the oldest messages if the history exceeds MaxHistorySize.
// This prevents unbounded memory growth in long-running sessions.
// Messages are removed from the beginning of the slice (oldest first).
func (m *Manager) trimHistory() {
	if len(m.conv.messages) <= MaxHistorySize {
		return
	}

	// Calculate how many messages to remove
	excess := len(m.conv.messages) - MaxHistorySize

	// Remove oldest messages by slicing
	m.conv.messages = m.conv.messages[excess:]
}

// BuildLLMContext constructs the full message context for an LLM request.
//
// The returned slice contains:
//  1. System prompt (if provided) as the first message
//  2. All messages from the conversation history
//  3. Trailing context with the current prompt (if set)
//
// The trailing context message has the format:
//
//	[current prompt: "<current prompt>"]
//
// This ensures the agent always knows the current prompt state, even if
// several turns have passed since the last edit or prompt update.
//
// The system prompt is NOT stored in the conversation history. It is
// prepended fresh on each request to allow dynamic system prompts.
//
// Example output structure:
//
//	[system] You help users create images...
//	[user] I want a cat
//	[assistant] Here's a prompt for a cat...
//	[system] [user edited prompt to: "a fluffy cat"]
//	[user] Make it orange
//	[system] [current prompt: "a fluffy cat"]
func (m *Manager) BuildLLMContext(systemPrompt string) []Message {
	// Pre-allocate exact capacity to avoid slice growth during appends.
	// Capacity = history + optional system prompt + optional trailing context.
	capacity := len(m.conv.messages)
	if systemPrompt != "" {
		capacity++
	}
	if m.conv.currentPrompt != "" {
		capacity++
	}

	context := make([]Message, 0, capacity)

	// Prepend system prompt if provided
	if systemPrompt != "" {
		context = append(context, Message{
			Role:    RoleSystem,
			Content: systemPrompt,
		})
	}

	// Add all conversation history
	context = append(context, m.conv.messages...)

	// Append trailing context with current prompt if set
	if m.conv.currentPrompt != "" {
		context = append(context, Message{
			Role:    RoleSystem,
			Content: `[current prompt: "` + m.conv.currentPrompt + `"]`,
		})
	}

	return context
}
