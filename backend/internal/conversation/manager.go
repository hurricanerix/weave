package conversation

import (
	"fmt"
	"sync"

	"github.com/hurricanerix/weave/internal/ollama"
)

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
// Manager is thread-safe. All methods are protected by a mutex to allow
// concurrent access from multiple HTTP request handlers.
type Manager struct {
	mu       sync.Mutex
	conv     *Conversation
	onChange func() // Called after any mutation to trigger persistence
}

// NewManager creates a new conversation manager with an empty conversation.
func NewManager() *Manager {
	return &Manager{
		conv: NewConversation(),
	}
}

// NewManagerWithConversation creates a manager from an existing conversation.
// This is used for session recovery when loading from persistence.
func NewManagerWithConversation(conv *Conversation) *Manager {
	return &Manager{
		conv: conv,
	}
}

// SetOnChange sets the callback to be invoked after any mutation.
// This is used by SessionManager to trigger persistence saves.
func (m *Manager) SetOnChange(callback func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onChange = callback
}

// triggerOnChangeLocked invokes the onChange callback if set.
// Must be called while holding the mutex (hence the Locked suffix).
func (m *Manager) triggerOnChangeLocked() {
	if m.onChange != nil {
		m.onChange()
	}
}

// AddUserMessage adds a user message to the conversation history.
// If the history exceeds MaxHistorySize, the oldest messages are removed.
// Returns the assigned message ID.
func (m *Manager) AddUserMessage(content string) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	id := m.conv.nextMessageID
	m.conv.nextMessageID++

	m.conv.messages = append(m.conv.messages, ConversationMessage{
		ID:       id,
		Role:     RoleUser,
		Content:  content,
		Snapshot: nil, // User messages don't have snapshots
	})
	m.trimHistoryLocked()
	m.triggerOnChangeLocked()
	return id
}

// AddAssistantMessage adds an assistant message to the conversation history
// and optionally creates a state snapshot if generation parameters changed.
//
// When metadata is provided, this method compares the metadata against the last
// snapshot state. If the prompt or generation settings differ, a StateSnapshot is
// created and attached to the message.
//
// Returns the assigned message ID.
func (m *Manager) AddAssistantMessage(content string, prompt string, metadata *ollama.LLMMetadata) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	id := m.conv.nextMessageID
	m.conv.nextMessageID++

	// Determine if we need to create a snapshot.
	// A snapshot is created if metadata differs from the last snapshot state.
	var snapshot *StateSnapshot
	if metadata != nil && metadata.Prompt != "" {
		lastSnapshot := m.getLastSnapshotLocked()

		// Create snapshot if prompt changed or this is the first prompt
		if lastSnapshot == nil || lastSnapshot.Prompt != metadata.Prompt {
			snapshot = &StateSnapshot{
				Prompt:        metadata.Prompt,
				Steps:         0, // Settings not tracked in LLMMetadata yet
				CFG:           0,
				Seed:          0,
				PreviewStatus: PreviewStatusNone,
				PreviewURL:    "",
			}
		}
	}

	m.conv.messages = append(m.conv.messages, ConversationMessage{
		ID:       id,
		Role:     RoleAssistant,
		Content:  content,
		Snapshot: snapshot,
	})

	// Update current prompt if the assistant provided one.
	// Note: promptEdited flag is managed by UpdatePrompt() when users
	// directly edit prompts, not when agents update them via this method.
	if prompt != "" {
		m.conv.previousPrompt = m.conv.currentPrompt
		m.conv.currentPrompt = prompt
	}

	m.trimHistoryLocked()
	m.triggerOnChangeLocked()
	return id
}

// GetHistory returns a copy of the message history as ollama.Message for LLM API.
// The returned slice is safe to modify without affecting the conversation.
// This converts ConversationMessage to Message by extracting Role, Content, and ToolCalls.
func (m *Manager) GetHistory() []Message {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.conv.messages) == 0 {
		return nil
	}

	// Convert ConversationMessage to Message (for ollama API compatibility)
	history := make([]Message, len(m.conv.messages))
	for i, msg := range m.conv.messages {
		history[i] = Message{
			Role:      msg.Role,
			Content:   msg.Content,
			ToolCalls: msg.ToolCalls,
		}
	}
	return history
}

// GetConversation returns the underlying Conversation.
// This is used by SessionManager to access conversation state for persistence.
// The returned Conversation is NOT thread-safe - caller must hold Manager's mutex.
func (m *Manager) GetConversation() *Conversation {
	return m.conv
}

// GetCurrentPrompt returns the current image generation prompt.
func (m *Manager) GetCurrentPrompt() string {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.conv.currentPrompt
}

// Clear resets the conversation to an empty state.
// All messages are removed and the prompt is cleared.
//
// The underlying message slice capacity is preserved to avoid reallocations
// in active sessions. For sessions that have grown very large, consider
// creating a new Manager instead.
func (m *Manager) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.conv.messages = m.conv.messages[:0]
	m.conv.currentPrompt = ""
	m.conv.previousPrompt = ""
	m.conv.promptEdited = false
	m.conv.nextMessageID = 1 // Reset message ID counter
	m.triggerOnChangeLocked()
}

// UpdatePrompt updates the current prompt with a user-provided value.
// This is called when the user directly edits the prompt in the UI.
//
// If the new prompt differs from the current prompt, the edited flag is set.
// This flag is used by NotifyPromptEdited to inject a notification into
// the conversation history so the agent knows the user made changes.
func (m *Manager) UpdatePrompt(newPrompt string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if newPrompt != m.conv.currentPrompt {
		m.conv.previousPrompt = m.conv.currentPrompt
		m.conv.currentPrompt = newPrompt
		m.conv.promptEdited = true
		m.triggerOnChangeLocked()
	}
}

// IsPromptEdited returns true if the user has edited the prompt since
// the last call to NotifyPromptEdited.
func (m *Manager) IsPromptEdited() bool {
	m.mu.Lock()
	defer m.mu.Unlock()

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
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.conv.promptEdited {
		return
	}

	id := m.conv.nextMessageID
	m.conv.nextMessageID++

	// Inject user message with the current prompt (not system - ollama
	// requires system messages to be first in conversation)
	notification := ConversationMessage{
		ID:       id,
		Role:     RoleUser,
		Content:  `[user edited prompt to: "` + m.conv.currentPrompt + `"]`,
		Snapshot: nil, // Edit notifications don't have snapshots
	}
	m.conv.messages = append(m.conv.messages, notification)

	// Clear the edited flag
	m.conv.promptEdited = false

	m.trimHistoryLocked()
	m.triggerOnChangeLocked()
}

// trimHistoryLocked removes the oldest messages if the history exceeds MaxHistorySize.
// This prevents unbounded memory growth in long-running sessions.
// Messages are removed from the beginning of the slice (oldest first).
//
// This method must be called while holding the mutex (hence the Locked suffix).
func (m *Manager) trimHistoryLocked() {
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
//  2. Current generation settings (if any are non-zero) as a user message
//  3. All messages from the conversation history
//  4. Trailing context with the current prompt (if set)
//
// The settings message has the format:
//
//	[Current generation settings: steps=X, cfg=Y, seed=Z]
//
// This appears after the system prompt but before conversation history so
// the agent sees current UI values when generating responses.
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
//	[user] [Current generation settings: steps=20, cfg=7.5, seed=42]
//	[user] I want a cat
//	[assistant] Here's a prompt for a cat...
//	[user] [user edited prompt to: "a fluffy cat"]
//	[user] Make it orange
//	[user] [current prompt: "a fluffy cat"]
func (m *Manager) BuildLLMContext(systemPrompt string, currentSteps int, currentCFG float64, currentSeed int64) []Message {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Pre-allocate exact capacity to avoid slice growth during appends.
	// Capacity = history + optional system prompt + optional settings + optional trailing context.
	capacity := len(m.conv.messages)
	if systemPrompt != "" {
		capacity++
	}
	// Check if any settings are non-zero (settings are set)
	hasSettings := currentSteps != 0 || currentCFG != 0 || currentSeed != 0
	if hasSettings {
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

	// Inject current generation settings if any are non-zero.
	// This appears after the system prompt but before conversation history
	// so the agent sees current UI values when generating responses.
	// Note: We use RoleUser instead of RoleSystem because Ollama requires
	// system messages to be first in the conversation (only one allowed).
	if hasSettings {
		// Format: [Current generation settings: steps=20, cfg=7.5, seed=42]
		settingsMsg := fmt.Sprintf("[Current generation settings: steps=%d, cfg=%.1f, seed=%d]",
			currentSteps, currentCFG, currentSeed)
		context = append(context, Message{
			Role:    RoleUser,
			Content: settingsMsg,
		})
	}

	// Add all conversation history (convert ConversationMessage to Message)
	for _, msg := range m.conv.messages {
		context = append(context, Message{
			Role:      msg.Role,
			Content:   msg.Content,
			ToolCalls: msg.ToolCalls,
		})
	}

	// Append trailing context with current prompt if set.
	// Note: We use RoleUser instead of RoleSystem because Ollama requires
	// system messages to be first in the conversation. This context message
	// represents state information for the agent, similar to edit notifications.
	if m.conv.currentPrompt != "" {
		context = append(context, Message{
			Role:    RoleUser,
			Content: `[current prompt: "` + m.conv.currentPrompt + `"]`,
		})
	}

	return context
}

// getLastSnapshotLocked returns the most recent state snapshot from the conversation.
// Returns nil if no snapshots exist.
//
// This method must be called while holding the mutex (hence the Locked suffix).
func (m *Manager) getLastSnapshotLocked() *StateSnapshot {
	// Iterate backwards to find the most recent snapshot
	for i := len(m.conv.messages) - 1; i >= 0; i-- {
		if m.conv.messages[i].Snapshot != nil {
			return m.conv.messages[i].Snapshot
		}
	}
	return nil
}

// GetMessage returns the message with the specified ID.
// Returns nil if no message with that ID exists.
func (m *Manager) GetMessage(id int) *ConversationMessage {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i := range m.conv.messages {
		if m.conv.messages[i].ID == id {
			return &m.conv.messages[i]
		}
	}
	return nil
}

// UpdateMessagePreview updates the preview status and URL for a message with a snapshot.
// This is called when a preview image is generated or generation completes.
//
// If the message doesn't exist or has no snapshot, this method does nothing.
func (m *Manager) UpdateMessagePreview(id int, status string, url string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i := range m.conv.messages {
		if m.conv.messages[i].ID == id && m.conv.messages[i].Snapshot != nil {
			m.conv.messages[i].Snapshot.PreviewStatus = status
			m.conv.messages[i].Snapshot.PreviewURL = url
			m.triggerOnChangeLocked()
			return
		}
	}
}
