//go:build integration
// +build integration

package persistence

import (
	"path/filepath"
	"testing"

	"github.com/hurricanerix/weave/internal/conversation"
)

// TestSessionRecoveryIntegration tests the full session recovery flow
// with the actual persistence layer (not mocked).
func TestSessionRecoveryIntegration(t *testing.T) {
	// Create temp directory for sessions
	tmpDir := t.TempDir()
	sessionPath := filepath.Join(tmpDir, "sessions")

	// Create persistence store
	store := NewSessionStore(sessionPath)

	// Create session manager with persistence
	sm := conversation.NewSessionManagerWithPersistence(store)
	defer sm.Shutdown()

	sessionID := "00000000000000000000000000000099"

	// Get session and add some messages
	session1 := sm.GetSession(sessionID)
	manager1 := session1.Manager()
	manager1.AddUserMessage("Hello from integration test")
	manager1.AddAssistantMessage("Hi there! This is a test.", "a beautiful landscape", nil)
	manager1.UpdatePrompt("a beautiful sunset landscape")

	// Verify conversation state
	if count := len(manager1.GetHistory()); count != 2 {
		t.Errorf("Initial message count = %d, want 2", count)
	}
	if prompt := manager1.GetCurrentPrompt(); prompt != "a beautiful sunset landscape" {
		t.Errorf("Initial prompt = %q, want %q", prompt, "a beautiful sunset landscape")
	}

	// Create a new session manager (simulating server restart)
	sm2 := conversation.NewSessionManagerWithPersistence(store)
	defer sm2.Shutdown()

	// Get the same session - should recover from disk
	session2 := sm2.GetSession(sessionID)
	manager2 := session2.Manager()

	// Verify conversation was recovered
	history := manager2.GetHistory()
	if count := len(history); count != 2 {
		t.Errorf("Recovered message count = %d, want 2", count)
	}
	if history[0].Content != "Hello from integration test" {
		t.Errorf("First message content = %q, want %q", history[0].Content, "Hello from integration test")
	}
	if history[1].Content != "Hi there! This is a test." {
		t.Errorf("Second message content = %q, want %q", history[1].Content, "Hi there! This is a test.")
	}
	if prompt := manager2.GetCurrentPrompt(); prompt != "a beautiful sunset landscape" {
		t.Errorf("Recovered prompt = %q, want %q", prompt, "a beautiful sunset landscape")
	}

	// Add more messages to the recovered session
	manager2.AddUserMessage("Can you make it more dramatic?")
	manager2.AddAssistantMessage("Sure! How about a stormy sunset?", "a dramatic stormy sunset landscape", nil)

	// Verify the new messages are there
	history = manager2.GetHistory()
	if count := len(history); count != 4 {
		t.Errorf("After adding messages, count = %d, want 4", count)
	}

	// Recover again to verify the new messages were persisted
	sm3 := conversation.NewSessionManagerWithPersistence(store)
	defer sm3.Shutdown()
	session3 := sm3.GetSession(sessionID)
	manager3 := session3.Manager()

	history = manager3.GetHistory()
	if count := len(history); count != 4 {
		t.Errorf("Final recovered message count = %d, want 4", count)
	}
	if prompt := manager3.GetCurrentPrompt(); prompt != "a dramatic stormy sunset landscape" {
		t.Errorf("Final recovered prompt = %q, want %q", prompt, "a dramatic stormy sunset landscape")
	}
}
