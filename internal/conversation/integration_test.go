package conversation

import (
	"strings"
	"sync"
	"testing"
)

// TestMultiTurnConversationFlow tests a complete multi-turn conversation
// with user edits and LLM context construction.
func TestMultiTurnConversationFlow(t *testing.T) {
	m := NewManager()
	systemPrompt := "You help users create images by writing detailed prompts."

	// Turn 1: User requests an image
	m.AddUserMessage("I want a picture of a cat")

	// Agent responds with initial prompt
	m.AddAssistantMessage(
		"A cat! I can help with that. Here's a starting prompt:\n\nPrompt: a cute cat sitting on a windowsill",
		"a cute cat sitting on a windowsill",
	)

	// Verify state after turn 1
	if m.GetCurrentPrompt() != "a cute cat sitting on a windowsill" {
		t.Errorf("Prompt after turn 1 = %q, want %q",
			m.GetCurrentPrompt(), "a cute cat sitting on a windowsill")
	}

	// User edits the prompt
	m.UpdatePrompt("a fluffy tabby cat sitting on a sunny windowsill")

	if !m.IsPromptEdited() {
		t.Error("Prompt should be marked as edited")
	}

	// Notify the edit (this would happen before the next LLM call)
	m.NotifyPromptEdited()

	if m.IsPromptEdited() {
		t.Error("Edited flag should be cleared after notify")
	}

	// Turn 2: User asks for more changes
	m.AddUserMessage("Can you make it more magical?")

	// Build LLM context - should include edit notification
	context := m.BuildLLMContext(systemPrompt)

	// Verify context structure
	// Expected: system, user, assistant, edit notification, user, trailing
	if len(context) != 6 {
		t.Fatalf("Expected 6 messages in context, got %d", len(context))
	}

	// Verify system prompt is first
	if context[0].Role != RoleSystem || context[0].Content != systemPrompt {
		t.Errorf("First message should be system prompt, got {%s, %q}",
			context[0].Role, context[0].Content)
	}

	// Verify edit notification is present (uses RoleUser, not RoleSystem,
	// because ollama requires system messages to be first in conversation)
	editNotification := context[3]
	expectedEdit := `[user edited prompt to: "a fluffy tabby cat sitting on a sunny windowsill"]`
	if editNotification.Role != RoleUser || editNotification.Content != expectedEdit {
		t.Errorf("Edit notification = {%s, %q}, want {user, %q}",
			editNotification.Role, editNotification.Content, expectedEdit)
	}

	// Verify trailing context (uses RoleUser, not RoleSystem, because
	// ollama requires system messages to be first in conversation)
	trailing := context[5]
	expectedTrailing := `[current prompt: "a fluffy tabby cat sitting on a sunny windowsill"]`
	if trailing.Role != RoleUser || trailing.Content != expectedTrailing {
		t.Errorf("Trailing context = {%s, %q}, want {user, %q}",
			trailing.Role, trailing.Content, expectedTrailing)
	}

	// Agent responds with updated prompt
	m.AddAssistantMessage(
		"I've made it more magical:\n\nPrompt: a fluffy tabby cat sitting on a sunny windowsill, magical sparkles, fantasy style",
		"a fluffy tabby cat sitting on a sunny windowsill, magical sparkles, fantasy style",
	)

	// Build context again - edit notification should still be in history
	// but trailing context should have new prompt
	context2 := m.BuildLLMContext(systemPrompt)

	// Expected: system, user, assistant, edit, user, assistant, trailing
	if len(context2) != 7 {
		t.Fatalf("Expected 7 messages in context after turn 2, got %d", len(context2))
	}

	// Verify new trailing context
	newTrailing := context2[6]
	newExpectedTrailing := `[current prompt: "a fluffy tabby cat sitting on a sunny windowsill, magical sparkles, fantasy style"]`
	if newTrailing.Content != newExpectedTrailing {
		t.Errorf("New trailing context = %q, want %q",
			newTrailing.Content, newExpectedTrailing)
	}
}

// TestMultipleEditsBeforeNotify tests that multiple edits only produce
// one notification with the final value.
func TestMultipleEditsBeforeNotify(t *testing.T) {
	m := NewManager()

	m.AddAssistantMessage("Initial prompt", "prompt v1")

	// User edits multiple times before we notify
	m.UpdatePrompt("prompt v2")
	m.UpdatePrompt("prompt v3")
	m.UpdatePrompt("prompt v4")

	// Should still be marked as edited
	if !m.IsPromptEdited() {
		t.Error("Should be marked as edited")
	}

	// Notify once
	m.NotifyPromptEdited()

	// Should have exactly one notification in history
	// Notification uses RoleUser and contains "[user edited prompt to:"
	history := m.GetHistory()
	notificationCount := 0
	for _, msg := range history {
		if msg.Role == RoleUser && strings.Contains(msg.Content, "[user edited prompt to:") {
			notificationCount++
		}
	}

	if notificationCount != 1 {
		t.Errorf("Expected 1 notification, got %d", notificationCount)
	}

	// Notification should have final value
	lastMsg := history[len(history)-1]
	expected := `[user edited prompt to: "prompt v4"]`
	if lastMsg.Content != expected {
		t.Errorf("Notification = %q, want %q", lastMsg.Content, expected)
	}
}

// TestClearResetsAllState tests that Clear() properly resets all conversation state.
func TestClearResetsAllState(t *testing.T) {
	m := NewManager()

	// Build up state
	m.AddUserMessage("message 1")
	m.AddAssistantMessage("response 1", "prompt 1")
	m.UpdatePrompt("edited prompt")

	// Verify state exists
	if len(m.GetHistory()) != 2 {
		t.Fatal("Should have 2 messages before clear")
	}
	if m.GetCurrentPrompt() != "edited prompt" {
		t.Fatal("Should have edited prompt before clear")
	}
	if !m.IsPromptEdited() {
		t.Fatal("Should be marked as edited before clear")
	}

	// Clear
	m.Clear()

	// Verify all state is reset
	if len(m.GetHistory()) != 0 {
		t.Error("History should be empty after clear")
	}
	if m.GetCurrentPrompt() != "" {
		t.Error("Prompt should be empty after clear")
	}
	if m.IsPromptEdited() {
		t.Error("Edited flag should be false after clear")
	}

	// Verify LLM context is minimal
	context := m.BuildLLMContext("system prompt")
	if len(context) != 1 {
		t.Errorf("Context should have only system prompt after clear, got %d messages", len(context))
	}
}

// TestSessionIsolationIntegration tests that multiple sessions remain isolated
// during concurrent operations.
func TestSessionIsolationIntegration(t *testing.T) {
	sm := NewSessionManager()

	const sessions = 10
	const messagesPerSession = 5

	var wg sync.WaitGroup
	wg.Add(sessions)

	// Simulate concurrent users building conversations
	for i := 0; i < sessions; i++ {
		go func(sessionNum int) {
			defer wg.Done()

			sessionID := string(rune('A' + sessionNum))
			m := sm.GetOrCreate(sessionID)

			// Each session has unique messages
			for j := 0; j < messagesPerSession; j++ {
				m.AddUserMessage("user " + sessionID)
				m.AddAssistantMessage("assistant "+sessionID, "prompt "+sessionID)
			}

			// Update prompt uniquely per session
			m.UpdatePrompt("final prompt for " + sessionID)
		}(i)
	}

	wg.Wait()

	// Verify each session is isolated
	for i := 0; i < sessions; i++ {
		sessionID := string(rune('A' + i))
		m := sm.Get(sessionID)

		if m == nil {
			t.Errorf("Session %s should exist", sessionID)
			continue
		}

		// Each session should have 2*messagesPerSession messages
		expectedMessages := messagesPerSession * 2
		history := m.GetHistory()
		if len(history) != expectedMessages {
			t.Errorf("Session %s has %d messages, want %d",
				sessionID, len(history), expectedMessages)
		}

		// Verify messages belong to this session
		expectedPrompt := "final prompt for " + sessionID
		if m.GetCurrentPrompt() != expectedPrompt {
			t.Errorf("Session %s prompt = %q, want %q",
				sessionID, m.GetCurrentPrompt(), expectedPrompt)
		}
	}
}

// TestContextWithoutPrompt verifies that LLM context works correctly
// when no prompt has been set.
func TestContextWithoutPrompt(t *testing.T) {
	m := NewManager()
	systemPrompt := "You are helpful."

	m.AddUserMessage("Hello")
	m.AddAssistantMessage("Hi there! How can I help?", "") // No prompt

	context := m.BuildLLMContext(systemPrompt)

	// Should have: system, user, assistant (no trailing context)
	if len(context) != 3 {
		t.Fatalf("Expected 3 messages (no trailing), got %d", len(context))
	}

	// Last message should be assistant, not trailing context
	if context[2].Role != RoleAssistant {
		t.Errorf("Last message should be assistant, got %s", context[2].Role)
	}
}

// TestEditNotificationFlow tests the complete flow of detecting edits,
// notifying, and verifying the notification appears in LLM context.
func TestEditNotificationFlow(t *testing.T) {
	m := NewManager()

	// Initial conversation
	m.AddUserMessage("Make me a cat")
	m.AddAssistantMessage("Here's a cat prompt", "a cat")

	// No edit yet - context has: system, user, assistant, trailing (3 non-system)
	// Note: trailing context is RoleUser, not RoleSystem, because ollama
	// requires system messages to be first in conversation
	context1 := m.BuildLLMContext("system")
	nonSystemMessages := 0
	for _, msg := range context1 {
		if msg.Role != RoleSystem {
			nonSystemMessages++
		}
	}
	// Expect 3: user message, assistant message, trailing context (all RoleUser or RoleAssistant)
	if nonSystemMessages != 3 {
		t.Errorf("Expected 3 non-system messages before edit (user, assistant, trailing), got %d", nonSystemMessages)
	}

	// User edits
	m.UpdatePrompt("a fluffy cat")
	m.NotifyPromptEdited()

	// Context should now include edit notification in history
	context2 := m.BuildLLMContext("system")

	// Find the edit notification
	found := false
	for _, msg := range context2 {
		if msg.Content == `[user edited prompt to: "a fluffy cat"]` {
			found = true
			break
		}
	}
	if !found {
		t.Error("Edit notification should be in context")
	}
}

// TestPromptEvolution tests that the prompt evolves correctly through
// multiple agent and user updates.
func TestPromptEvolution(t *testing.T) {
	m := NewManager()

	// Agent sets initial prompt
	m.AddAssistantMessage("Initial", "v1")
	if m.GetCurrentPrompt() != "v1" {
		t.Errorf("After agent v1: got %q", m.GetCurrentPrompt())
	}

	// User edits
	m.UpdatePrompt("v2-user")
	if m.GetCurrentPrompt() != "v2-user" {
		t.Errorf("After user edit: got %q", m.GetCurrentPrompt())
	}

	// Agent updates (should replace user edit)
	m.AddAssistantMessage("Updated", "v3")
	if m.GetCurrentPrompt() != "v3" {
		t.Errorf("After agent v3: got %q", m.GetCurrentPrompt())
	}

	// User edits again
	m.UpdatePrompt("v4-user")
	if m.GetCurrentPrompt() != "v4-user" {
		t.Errorf("After user edit v4: got %q", m.GetCurrentPrompt())
	}

	// Final context should have v4-user
	context := m.BuildLLMContext("system")
	trailing := context[len(context)-1]
	expected := `[current prompt: "v4-user"]`
	if trailing.Content != expected {
		t.Errorf("Trailing context = %q, want %q", trailing.Content, expected)
	}
}
