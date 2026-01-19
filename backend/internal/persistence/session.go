package persistence

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/hurricanerix/weave/internal/conversation"
)

const (
	// MaxConversationSizeBytes is the maximum size allowed for a conversation file.
	// This prevents disk exhaustion from malicious or corrupted data.
	// 10MB is enough for very large conversations with thousands of messages.
	MaxConversationSizeBytes = 10 * 1024 * 1024 // 10MB
)

// SessionStore manages persisting conversation sessions to disk.
// It handles saving and loading conversation state including messages,
// snapshots, and generation parameters.
//
// Storage structure:
//
//	config/sessions/{session_id}/
//	  conversation.json
//	  images/
type SessionStore struct {
	basePath string // Base directory for all sessions (e.g., "config/sessions")
}

// NewSessionStore creates a new session store rooted at the specified base path.
// The base path is typically "config/sessions".
//
// The directory structure will be created as needed when Save is called.
func NewSessionStore(basePath string) *SessionStore {
	return &SessionStore{
		basePath: basePath,
	}
}

// Save persists a conversation to disk.
// The conversation is serialized to JSON and written to:
// {basePath}/{sessionID}/conversation.json
//
// The session directory and images subdirectory are created if they don't exist.
// If the conversation.json file exists, it is overwritten atomically.
func (s *SessionStore) Save(sessionID string, conv *conversation.Conversation) error {
	if err := validateSessionID(sessionID); err != nil {
		return fmt.Errorf("invalid session ID: %w", err)
	}
	if conv == nil {
		return fmt.Errorf("conversation cannot be nil")
	}

	// Create session directory structure (0700: owner-only access)
	sessionDir := filepath.Join(s.basePath, sessionID)
	if err := os.MkdirAll(sessionDir, 0700); err != nil {
		return fmt.Errorf("failed to create session directory: %w", err)
	}

	// Create images subdirectory (0700: owner-only access)
	imagesDir := filepath.Join(sessionDir, "images")
	if err := os.MkdirAll(imagesDir, 0700); err != nil {
		return fmt.Errorf("failed to create images directory: %w", err)
	}

	// Serialize conversation to JSON
	data, err := serializeConversation(conv)
	if err != nil {
		return fmt.Errorf("failed to serialize conversation: %w", err)
	}

	// Check size limit to prevent disk exhaustion
	if len(data) > MaxConversationSizeBytes {
		return fmt.Errorf("conversation size %d bytes exceeds maximum %d bytes", len(data), MaxConversationSizeBytes)
	}

	// Write to temp file first, then rename (atomic write)
	// 0600: owner read/write only
	conversationPath := filepath.Join(sessionDir, "conversation.json")
	tempPath := conversationPath + ".tmp"

	if err := os.WriteFile(tempPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write conversation file: %w", err)
	}

	if err := os.Rename(tempPath, conversationPath); err != nil {
		// Clean up temp file if rename fails
		_ = os.Remove(tempPath)
		return fmt.Errorf("failed to commit conversation file: %w", err)
	}

	return nil
}

// Load reads a conversation from disk and returns it.
// Returns an empty conversation if the file doesn't exist.
// Returns an error if the file exists but is corrupt or unreadable.
func (s *SessionStore) Load(sessionID string) (*conversation.Conversation, error) {
	if err := validateSessionID(sessionID); err != nil {
		return nil, fmt.Errorf("invalid session ID: %w", err)
	}

	conversationPath := filepath.Join(s.basePath, sessionID, "conversation.json")

	// If file doesn't exist, return empty conversation (not an error)
	if _, err := os.Stat(conversationPath); os.IsNotExist(err) {
		return conversation.NewConversation(), nil
	}

	// Read the file
	data, err := os.ReadFile(conversationPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read conversation file: %w", err)
	}

	// Deserialize from JSON
	conv, err := deserializeConversation(data)
	if err != nil {
		// Log corrupt file but return empty conversation to allow recovery
		fmt.Fprintf(os.Stderr, "WARNING: corrupt conversation file for session %s: %v\n", sessionID, err)
		return conversation.NewConversation(), nil
	}

	return conv, nil
}

// Exists checks if a session exists on disk.
// Returns true if the session directory and conversation.json file exist.
func (s *SessionStore) Exists(sessionID string) bool {
	if err := validateSessionID(sessionID); err != nil {
		return false
	}

	conversationPath := filepath.Join(s.basePath, sessionID, "conversation.json")
	_, err := os.Stat(conversationPath)
	return err == nil
}

// ListSessions returns a list of all session IDs found in the base directory.
// Each session is identified by its directory name.
// Returns an empty slice if no sessions exist or if the base directory doesn't exist.
func (s *SessionStore) ListSessions() ([]string, error) {
	// If base path doesn't exist, return empty slice (not an error)
	if _, err := os.Stat(s.basePath); os.IsNotExist(err) {
		return []string{}, nil
	}

	entries, err := os.ReadDir(s.basePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read sessions directory: %w", err)
	}

	sessions := make([]string, 0)
	for _, entry := range entries {
		// Only include directories that contain conversation.json
		if entry.IsDir() {
			sessionID := entry.Name()
			if s.Exists(sessionID) {
				sessions = append(sessions, sessionID)
			}
		}
	}

	return sessions, nil
}

// conversationJSON is the JSON representation of a Conversation.
// This struct defines the exact format written to disk.
type conversationJSON struct {
	Messages       []conversation.ConversationMessage `json:"messages"`
	NextMessageID  int                                `json:"next_message_id"`
	CurrentPrompt  string                             `json:"current_prompt"`
	PreviousPrompt string                             `json:"previous_prompt,omitempty"`
	PromptEdited   bool                               `json:"prompt_edited,omitempty"`
}

// serializeConversation converts a Conversation to JSON bytes.
func serializeConversation(conv *conversation.Conversation) ([]byte, error) {
	data := conversationJSON{
		Messages:       conv.GetMessages(),
		NextMessageID:  conv.GetNextMessageID(),
		CurrentPrompt:  conv.GetCurrentPrompt(),
		PreviousPrompt: conv.GetPreviousPrompt(),
		PromptEdited:   conv.IsPromptEdited(),
	}

	return json.MarshalIndent(data, "", "  ")
}

// deserializeConversation creates a Conversation from JSON bytes.
func deserializeConversation(data []byte) (*conversation.Conversation, error) {
	var jsonData conversationJSON
	if err := json.Unmarshal(data, &jsonData); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	// Create new conversation and restore state
	conv := conversation.NewConversation()
	conv.SetMessages(jsonData.Messages)
	conv.SetNextMessageID(jsonData.NextMessageID)
	conv.SetCurrentPrompt(jsonData.CurrentPrompt)
	conv.SetPreviousPrompt(jsonData.PreviousPrompt)
	conv.SetPromptEdited(jsonData.PromptEdited)

	return conv, nil
}
