package conversation

import (
	"testing"

	"github.com/hurricanerix/weave/internal/ollama"
)

// mockPersistence is a simple in-memory persistence implementation for testing.
type mockPersistence struct {
	data map[string]*Conversation
}

func newMockPersistence() *mockPersistence {
	return &mockPersistence{
		data: make(map[string]*Conversation),
	}
}

func (m *mockPersistence) Save(sessionID string, conv *Conversation) error {
	// Make a copy to simulate disk persistence
	m.data[sessionID] = &Conversation{
		messages:       append([]ConversationMessage(nil), conv.GetMessages()...),
		nextMessageID:  conv.GetNextMessageID(),
		currentPrompt:  conv.GetCurrentPrompt(),
		previousPrompt: conv.GetPreviousPrompt(),
		promptEdited:   conv.IsPromptEdited(),
	}
	return nil
}

func (m *mockPersistence) Load(sessionID string) (*Conversation, error) {
	conv, exists := m.data[sessionID]
	if !exists {
		return NewConversation(), nil
	}
	// Return a copy
	return &Conversation{
		messages:       append([]ConversationMessage(nil), conv.messages...),
		nextMessageID:  conv.nextMessageID,
		currentPrompt:  conv.currentPrompt,
		previousPrompt: conv.previousPrompt,
		promptEdited:   conv.promptEdited,
	}, nil
}

func TestSessionRecovery(t *testing.T) {
	// Create mock persistence
	store := newMockPersistence()

	// Create session manager with persistence
	sm := NewSessionManagerWithPersistence(store)
	defer sm.Shutdown()

	sessionID := "test-session-123"

	// Get session and add some messages
	session1 := sm.GetSession(sessionID)
	manager1 := session1.Manager()
	manager1.AddUserMessage("Hello")
	manager1.AddAssistantMessage("Hi there!", "a cat", nil)

	// Verify conversation state
	if count := len(manager1.GetHistory()); count != 2 {
		t.Errorf("Initial message count = %d, want 2", count)
	}
	if prompt := manager1.GetCurrentPrompt(); prompt != "a cat" {
		t.Errorf("Initial prompt = %q, want %q", prompt, "a cat")
	}

	// Create a new session manager (simulating server restart)
	sm2 := NewSessionManagerWithPersistence(store)
	defer sm2.Shutdown()

	// Get the same session - should recover from "disk"
	session2 := sm2.GetSession(sessionID)
	manager2 := session2.Manager()

	// Verify conversation was recovered
	history := manager2.GetHistory()
	if count := len(history); count != 2 {
		t.Errorf("Recovered message count = %d, want 2", count)
	}
	if history[0].Content != "Hello" {
		t.Errorf("First message content = %q, want %q", history[0].Content, "Hello")
	}
	if history[1].Content != "Hi there!" {
		t.Errorf("Second message content = %q, want %q", history[1].Content, "Hi there!")
	}
	if prompt := manager2.GetCurrentPrompt(); prompt != "a cat" {
		t.Errorf("Recovered prompt = %q, want %q", prompt, "a cat")
	}
}

func TestSessionRecovery_NonExistent(t *testing.T) {
	// Create mock persistence
	store := newMockPersistence()

	// Create session manager with persistence
	sm := NewSessionManagerWithPersistence(store)
	defer sm.Shutdown()

	// Get non-existent session - should create empty one
	session := sm.GetSession("non-existent-session")
	manager := session.Manager()

	// Verify it's empty
	if count := len(manager.GetHistory()); count != 0 {
		t.Errorf("New session message count = %d, want 0", count)
	}
	if prompt := manager.GetCurrentPrompt(); prompt != "" {
		t.Errorf("New session prompt = %q, want empty", prompt)
	}
}

func TestSessionRecovery_PersistenceOnChange(t *testing.T) {
	// Create mock persistence
	store := newMockPersistence()

	// Create session manager with persistence
	sm := NewSessionManagerWithPersistence(store)
	defer sm.Shutdown()

	sessionID := "test-persist-on-change"

	// Get session and add message
	session := sm.GetSession(sessionID)
	manager := session.Manager()
	manager.AddUserMessage("First message")

	// Verify data was saved to mock store
	if _, exists := store.data[sessionID]; !exists {
		t.Error("Session was not saved after AddUserMessage")
	}

	// Add another message
	manager.AddAssistantMessage("Response", "a dog", nil)

	// Verify data is updated
	if conv, exists := store.data[sessionID]; !exists {
		t.Error("Session disappeared after AddAssistantMessage")
	} else if len(conv.GetMessages()) != 2 {
		t.Errorf("Saved message count = %d, want 2", len(conv.GetMessages()))
	}

	// Load and verify
	sm2 := NewSessionManagerWithPersistence(store)
	defer sm2.Shutdown()
	session2 := sm2.GetSession(sessionID)
	manager2 := session2.Manager()

	history := manager2.GetHistory()
	if count := len(history); count != 2 {
		t.Errorf("Persisted message count = %d, want 2", count)
	}
}

func TestSessionManager_WithoutPersistence(t *testing.T) {
	// Create session manager without persistence (nil store)
	sm := NewSessionManagerWithPersistence(nil)
	defer sm.Shutdown()

	sessionID := "no-persist-session"

	// Get session and add messages
	session := sm.GetSession(sessionID)
	manager := session.Manager()
	manager.AddUserMessage("Hello")
	manager.AddAssistantMessage("Hi!", "test", nil)

	// Should work fine in-memory
	if count := len(manager.GetHistory()); count != 2 {
		t.Errorf("In-memory message count = %d, want 2", count)
	}

	// Create new session manager - should not recover (no persistence)
	sm2 := NewSessionManagerWithPersistence(nil)
	defer sm2.Shutdown()
	session2 := sm2.GetSession(sessionID)
	manager2 := session2.Manager()

	// Should be empty (no persistence)
	if count := len(manager2.GetHistory()); count != 0 {
		t.Errorf("Non-persisted session count = %d, want 0", count)
	}
}

func TestOnChangeCallback(t *testing.T) {
	// Test that onChange callback is triggered on mutations
	callCount := 0
	manager := NewManager()
	manager.SetOnChange(func() {
		callCount++
	})

	// Test AddUserMessage triggers callback
	manager.AddUserMessage("Hello")
	if callCount != 1 {
		t.Errorf("AddUserMessage: callback count = %d, want 1", callCount)
	}

	// Test AddAssistantMessage triggers callback
	manager.AddAssistantMessage("Hi", "prompt", nil)
	if callCount != 2 {
		t.Errorf("AddAssistantMessage: callback count = %d, want 2", callCount)
	}

	// Test UpdatePrompt triggers callback (when changed)
	manager.UpdatePrompt("new prompt")
	if callCount != 3 {
		t.Errorf("UpdatePrompt: callback count = %d, want 3", callCount)
	}

	// Test UpdatePrompt doesn't trigger when unchanged
	manager.UpdatePrompt("new prompt")
	if callCount != 3 {
		t.Errorf("UpdatePrompt (unchanged): callback count = %d, want 3", callCount)
	}

	// Test NotifyPromptEdited triggers callback
	manager.NotifyPromptEdited()
	if callCount != 4 {
		t.Errorf("NotifyPromptEdited: callback count = %d, want 4", callCount)
	}

	// Test Clear triggers callback
	manager.Clear()
	if callCount != 5 {
		t.Errorf("Clear: callback count = %d, want 5", callCount)
	}

	// Test UpdateMessagePreview triggers callback
	// First, add a message with a snapshot
	msgID := manager.AddAssistantMessage("Test", "test prompt", &ollama.LLMMetadata{Prompt: "test prompt"})
	callCount = 0 // Reset counter after adding message
	// Now update the preview - should trigger callback
	manager.UpdateMessagePreview(msgID, "complete", "url")
	if callCount != 1 {
		t.Errorf("UpdateMessagePreview: callback count = %d, want 1", callCount)
	}
}
