package conversation

import (
	"testing"
)

func TestNewManager(t *testing.T) {
	m := NewManager()

	if m == nil {
		t.Fatal("NewManager() returned nil")
	}

	if m.conv == nil {
		t.Fatal("Manager.conv is nil")
	}

	if len(m.GetHistory()) != 0 {
		t.Errorf("New manager should have empty history, got %d messages", len(m.GetHistory()))
	}

	if m.GetCurrentPrompt() != "" {
		t.Errorf("New manager should have empty prompt, got %q", m.GetCurrentPrompt())
	}
}

func TestAddUserMessage(t *testing.T) {
	m := NewManager()

	m.AddUserMessage("hello")
	m.AddUserMessage("world")

	history := m.GetHistory()
	if len(history) != 2 {
		t.Fatalf("Expected 2 messages, got %d", len(history))
	}

	if history[0].Role != RoleUser || history[0].Content != "hello" {
		t.Errorf("First message = {%s, %s}, want {user, hello}", history[0].Role, history[0].Content)
	}

	if history[1].Role != RoleUser || history[1].Content != "world" {
		t.Errorf("Second message = {%s, %s}, want {user, world}", history[1].Role, history[1].Content)
	}
}

func TestAddAssistantMessage(t *testing.T) {
	m := NewManager()

	m.AddUserMessage("I want a cat")
	m.AddAssistantMessage("What kind of cat?", "")

	history := m.GetHistory()
	if len(history) != 2 {
		t.Fatalf("Expected 2 messages, got %d", len(history))
	}

	if history[1].Role != RoleAssistant || history[1].Content != "What kind of cat?" {
		t.Errorf("Assistant message = {%s, %s}, want {assistant, What kind of cat?}",
			history[1].Role, history[1].Content)
	}

	// Prompt should still be empty (no prompt provided)
	if m.GetCurrentPrompt() != "" {
		t.Errorf("Prompt should be empty, got %q", m.GetCurrentPrompt())
	}
}

func TestAddAssistantMessageWithPrompt(t *testing.T) {
	m := NewManager()

	m.AddUserMessage("I want a cat")
	m.AddAssistantMessage("Here's your prompt:\n\nPrompt: a cute tabby cat", "a cute tabby cat")

	if m.GetCurrentPrompt() != "a cute tabby cat" {
		t.Errorf("Prompt = %q, want %q", m.GetCurrentPrompt(), "a cute tabby cat")
	}

	// Add another message with updated prompt
	m.AddAssistantMessage("Updated prompt:\n\nPrompt: a fluffy tabby cat", "a fluffy tabby cat")

	if m.GetCurrentPrompt() != "a fluffy tabby cat" {
		t.Errorf("Prompt = %q, want %q", m.GetCurrentPrompt(), "a fluffy tabby cat")
	}
}

func TestGetHistoryMaintainsOrder(t *testing.T) {
	m := NewManager()

	m.AddUserMessage("first")
	m.AddAssistantMessage("second", "")
	m.AddUserMessage("third")
	m.AddAssistantMessage("fourth", "")

	history := m.GetHistory()
	if len(history) != 4 {
		t.Fatalf("Expected 4 messages, got %d", len(history))
	}

	expected := []struct {
		role    string
		content string
	}{
		{RoleUser, "first"},
		{RoleAssistant, "second"},
		{RoleUser, "third"},
		{RoleAssistant, "fourth"},
	}

	for i, exp := range expected {
		if history[i].Role != exp.role || history[i].Content != exp.content {
			t.Errorf("Message %d = {%s, %s}, want {%s, %s}",
				i, history[i].Role, history[i].Content, exp.role, exp.content)
		}
	}
}

func TestGetHistoryReturnsEmptyForNewManager(t *testing.T) {
	m := NewManager()

	history := m.GetHistory()
	if history != nil {
		t.Errorf("Expected nil for empty history, got %v", history)
	}
}

func TestGetHistoryReturnsCopy(t *testing.T) {
	m := NewManager()
	m.AddUserMessage("original")

	history := m.GetHistory()
	history[0].Content = "modified"

	// Original should be unchanged
	originalHistory := m.GetHistory()
	if originalHistory[0].Content != "original" {
		t.Errorf("Modifying returned history affected original: got %q", originalHistory[0].Content)
	}
}

func TestClear(t *testing.T) {
	m := NewManager()

	m.AddUserMessage("hello")
	m.AddAssistantMessage("hi", "test prompt")

	if len(m.GetHistory()) != 2 {
		t.Fatalf("Expected 2 messages before clear, got %d", len(m.GetHistory()))
	}

	m.Clear()

	if len(m.GetHistory()) != 0 {
		t.Errorf("Expected 0 messages after clear, got %d", len(m.GetHistory()))
	}

	if m.GetCurrentPrompt() != "" {
		t.Errorf("Expected empty prompt after clear, got %q", m.GetCurrentPrompt())
	}
}

func TestClearPreservesCapacity(t *testing.T) {
	m := NewManager()

	// Add messages to grow the slice
	for i := 0; i < 10; i++ {
		m.AddUserMessage("test")
	}

	m.Clear()

	// After clear, adding messages should work
	m.AddUserMessage("new message")
	history := m.GetHistory()

	if len(history) != 1 {
		t.Errorf("Expected 1 message after clear and add, got %d", len(history))
	}

	if history[0].Content != "new message" {
		t.Errorf("Message content = %q, want %q", history[0].Content, "new message")
	}
}

func TestUpdatePromptSetsEditedFlag(t *testing.T) {
	m := NewManager()

	// Initially not edited
	if m.IsPromptEdited() {
		t.Error("New manager should not have edited flag set")
	}

	// Update prompt
	m.UpdatePrompt("new prompt")

	if !m.IsPromptEdited() {
		t.Error("After UpdatePrompt, edited flag should be set")
	}

	if m.GetCurrentPrompt() != "new prompt" {
		t.Errorf("Prompt = %q, want %q", m.GetCurrentPrompt(), "new prompt")
	}
}

func TestUpdatePromptNoChangeDoesNotSetFlag(t *testing.T) {
	m := NewManager()

	// Set initial prompt via assistant
	m.AddAssistantMessage("Here's your prompt", "initial prompt")

	// Update with same value
	m.UpdatePrompt("initial prompt")

	if m.IsPromptEdited() {
		t.Error("UpdatePrompt with same value should not set edited flag")
	}
}

func TestUpdatePromptDetectsChange(t *testing.T) {
	m := NewManager()

	// Set initial prompt
	m.AddAssistantMessage("Here's your prompt", "initial prompt")

	// Update with different value
	m.UpdatePrompt("modified prompt")

	if !m.IsPromptEdited() {
		t.Error("UpdatePrompt with different value should set edited flag")
	}

	if m.GetCurrentPrompt() != "modified prompt" {
		t.Errorf("Prompt = %q, want %q", m.GetCurrentPrompt(), "modified prompt")
	}
}

func TestNotifyPromptEditedInjectsMessage(t *testing.T) {
	m := NewManager()

	m.AddUserMessage("I want a cat")
	m.AddAssistantMessage("Here's your prompt", "a cat")
	m.UpdatePrompt("a fluffy cat")

	m.NotifyPromptEdited()

	history := m.GetHistory()
	if len(history) != 3 {
		t.Fatalf("Expected 3 messages, got %d", len(history))
	}

	notification := history[2]
	if notification.Role != RoleUser {
		t.Errorf("Notification role = %q, want %q", notification.Role, RoleUser)
	}

	expected := `[user edited prompt to: "a fluffy cat"]`
	if notification.Content != expected {
		t.Errorf("Notification content = %q, want %q", notification.Content, expected)
	}
}

func TestNotifyPromptEditedClearsFlag(t *testing.T) {
	m := NewManager()

	m.UpdatePrompt("test prompt")

	if !m.IsPromptEdited() {
		t.Fatal("Edited flag should be set before notify")
	}

	m.NotifyPromptEdited()

	if m.IsPromptEdited() {
		t.Error("Edited flag should be cleared after notify")
	}
}

func TestNotifyPromptEditedNoOpWhenNotEdited(t *testing.T) {
	m := NewManager()

	m.AddUserMessage("hello")

	// Not edited, so notify should be no-op
	m.NotifyPromptEdited()

	history := m.GetHistory()
	if len(history) != 1 {
		t.Errorf("Expected 1 message (no notification), got %d", len(history))
	}
}

func TestNotifyPromptEditedOnlyOnce(t *testing.T) {
	m := NewManager()

	m.UpdatePrompt("test prompt")
	m.NotifyPromptEdited()
	m.NotifyPromptEdited() // Should be no-op

	history := m.GetHistory()
	if len(history) != 1 {
		t.Errorf("Expected 1 notification message, got %d", len(history))
	}
}

func TestUpdatePromptMultipleTimes(t *testing.T) {
	m := NewManager()

	m.UpdatePrompt("first")
	m.UpdatePrompt("second")
	m.UpdatePrompt("third")

	// Only one notification should be injected
	m.NotifyPromptEdited()

	history := m.GetHistory()
	if len(history) != 1 {
		t.Errorf("Expected 1 notification, got %d", len(history))
	}

	// Notification should have the final prompt value
	expected := `[user edited prompt to: "third"]`
	if history[0].Content != expected {
		t.Errorf("Notification = %q, want %q", history[0].Content, expected)
	}
}

func TestClearResetsEditedFlag(t *testing.T) {
	m := NewManager()

	m.UpdatePrompt("test")

	if !m.IsPromptEdited() {
		t.Fatal("Edited flag should be set")
	}

	m.Clear()

	if m.IsPromptEdited() {
		t.Error("Edited flag should be cleared after Clear()")
	}
}

func TestBuildLLMContextEmpty(t *testing.T) {
	m := NewManager()

	// No system prompt, no history, no current prompt
	context := m.BuildLLMContext("")

	if len(context) != 0 {
		t.Errorf("Expected empty context, got %d messages", len(context))
	}
}

func TestBuildLLMContextSystemPromptOnly(t *testing.T) {
	m := NewManager()

	context := m.BuildLLMContext("You are a helpful assistant.")

	if len(context) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(context))
	}

	if context[0].Role != RoleSystem {
		t.Errorf("First message role = %q, want %q", context[0].Role, RoleSystem)
	}

	if context[0].Content != "You are a helpful assistant." {
		t.Errorf("First message content = %q, want %q", context[0].Content, "You are a helpful assistant.")
	}
}

func TestBuildLLMContextHistoryOnly(t *testing.T) {
	m := NewManager()

	m.AddUserMessage("hello")
	m.AddAssistantMessage("hi there", "")

	// No system prompt
	context := m.BuildLLMContext("")

	if len(context) != 2 {
		t.Fatalf("Expected 2 messages, got %d", len(context))
	}

	if context[0].Role != RoleUser || context[0].Content != "hello" {
		t.Errorf("First message = {%s, %s}, want {user, hello}", context[0].Role, context[0].Content)
	}

	if context[1].Role != RoleAssistant || context[1].Content != "hi there" {
		t.Errorf("Second message = {%s, %s}, want {assistant, hi there}", context[1].Role, context[1].Content)
	}
}

func TestBuildLLMContextCurrentPromptOnly(t *testing.T) {
	m := NewManager()

	m.AddAssistantMessage("Here's a prompt", "a cute cat")

	// No system prompt, but has current prompt
	context := m.BuildLLMContext("")

	if len(context) != 2 {
		t.Fatalf("Expected 2 messages (history + trailing), got %d", len(context))
	}

	// Last message should be trailing context
	trailing := context[len(context)-1]
	if trailing.Role != RoleSystem {
		t.Errorf("Trailing message role = %q, want %q", trailing.Role, RoleSystem)
	}

	expected := `[current prompt: "a cute cat"]`
	if trailing.Content != expected {
		t.Errorf("Trailing content = %q, want %q", trailing.Content, expected)
	}
}

func TestBuildLLMContextFull(t *testing.T) {
	m := NewManager()

	m.AddUserMessage("I want a cat")
	m.AddAssistantMessage("Here's a cat prompt", "a cute cat")
	m.AddUserMessage("Make it fluffy")

	context := m.BuildLLMContext("You help users create images.")

	if len(context) != 5 {
		t.Fatalf("Expected 5 messages, got %d", len(context))
	}

	// Verify structure: system prompt, history (3), trailing context
	expected := []struct {
		role    string
		content string
	}{
		{RoleSystem, "You help users create images."},
		{RoleUser, "I want a cat"},
		{RoleAssistant, "Here's a cat prompt"},
		{RoleUser, "Make it fluffy"},
		{RoleSystem, `[current prompt: "a cute cat"]`},
	}

	for i, exp := range expected {
		if context[i].Role != exp.role {
			t.Errorf("Message %d role = %q, want %q", i, context[i].Role, exp.role)
		}
		if context[i].Content != exp.content {
			t.Errorf("Message %d content = %q, want %q", i, context[i].Content, exp.content)
		}
	}
}

func TestBuildLLMContextWithEditNotification(t *testing.T) {
	m := NewManager()

	m.AddUserMessage("I want a cat")
	m.AddAssistantMessage("Here's a cat prompt", "a cute cat")
	m.UpdatePrompt("a fluffy cat")
	m.NotifyPromptEdited()
	m.AddUserMessage("Now make it orange")

	context := m.BuildLLMContext("You help users.")

	// Structure: system prompt, user, assistant, edit notification, user, trailing
	if len(context) != 6 {
		t.Fatalf("Expected 6 messages, got %d", len(context))
	}

	// Verify edit notification is in history (uses RoleUser, not RoleSystem)
	editNotification := context[3]
	if editNotification.Role != RoleUser {
		t.Errorf("Edit notification role = %q, want %q", editNotification.Role, RoleUser)
	}

	expectedEdit := `[user edited prompt to: "a fluffy cat"]`
	if editNotification.Content != expectedEdit {
		t.Errorf("Edit notification = %q, want %q", editNotification.Content, expectedEdit)
	}

	// Verify trailing context has current prompt
	trailing := context[5]
	expectedTrailing := `[current prompt: "a fluffy cat"]`
	if trailing.Content != expectedTrailing {
		t.Errorf("Trailing context = %q, want %q", trailing.Content, expectedTrailing)
	}
}

func TestBuildLLMContextDoesNotModifyHistory(t *testing.T) {
	m := NewManager()

	m.AddUserMessage("hello")
	m.AddAssistantMessage("hi", "test prompt")

	// Build context
	_ = m.BuildLLMContext("system prompt")

	// History should still have only 2 messages (not system prompt or trailing)
	history := m.GetHistory()
	if len(history) != 2 {
		t.Errorf("History should have 2 messages, got %d", len(history))
	}
}

func TestBuildLLMContextNoTrailingWhenNoPrompt(t *testing.T) {
	m := NewManager()

	m.AddUserMessage("hello")
	m.AddAssistantMessage("hi there", "") // No prompt provided

	context := m.BuildLLMContext("You are helpful.")

	// Should be: system prompt + 2 history messages (no trailing)
	if len(context) != 3 {
		t.Fatalf("Expected 3 messages (no trailing), got %d", len(context))
	}

	// Last message should be the assistant message, not trailing context
	last := context[len(context)-1]
	if last.Role != RoleAssistant {
		t.Errorf("Last message should be assistant, got %q", last.Role)
	}
}

func TestTrimHistory_BelowLimit(t *testing.T) {
	m := NewManager()

	// Add messages below the limit
	for i := 0; i < 50; i++ {
		m.AddUserMessage("message")
	}

	history := m.GetHistory()
	if len(history) != 50 {
		t.Errorf("Expected 50 messages, got %d", len(history))
	}
}

func TestTrimHistory_AtLimit(t *testing.T) {
	m := NewManager()

	// Add exactly MaxHistorySize messages
	for i := 0; i < MaxHistorySize; i++ {
		m.AddUserMessage("message")
	}

	history := m.GetHistory()
	if len(history) != MaxHistorySize {
		t.Errorf("Expected %d messages, got %d", MaxHistorySize, len(history))
	}
}

func TestTrimHistory_ExceedsLimit(t *testing.T) {
	m := NewManager()

	// Add more than MaxHistorySize messages
	for i := 0; i < MaxHistorySize+50; i++ {
		m.AddUserMessage("message")
	}

	history := m.GetHistory()
	if len(history) != MaxHistorySize {
		t.Errorf("Expected %d messages (trimmed), got %d", MaxHistorySize, len(history))
	}
}

func TestTrimHistory_KeepsNewestMessages(t *testing.T) {
	m := NewManager()

	// Add messages with unique content to verify which are kept
	for i := 0; i < MaxHistorySize+10; i++ {
		m.AddUserMessage("message-" + string(rune('0'+i%10)))
	}

	history := m.GetHistory()

	// Should have exactly MaxHistorySize messages
	if len(history) != MaxHistorySize {
		t.Errorf("Expected %d messages, got %d", MaxHistorySize, len(history))
	}

	// First message should be from index 10 (oldest 10 were removed)
	// Note: This test assumes MaxHistorySize > 10
	if history[0].Content != "message-0" {
		t.Errorf("First message = %q, expected oldest retained message", history[0].Content)
	}

	// Last message should be the newest
	expectedLast := MaxHistorySize + 9
	lastDigit := expectedLast % 10
	expectedContent := "message-" + string(rune('0'+lastDigit))
	if history[len(history)-1].Content != expectedContent {
		t.Errorf("Last message = %q, expected %q", history[len(history)-1].Content, expectedContent)
	}
}

func TestTrimHistory_MixedMessageTypes(t *testing.T) {
	m := NewManager()

	// Fill history to just below limit with alternating user/assistant
	for i := 0; i < MaxHistorySize-2; i++ {
		if i%2 == 0 {
			m.AddUserMessage("user message")
		} else {
			m.AddAssistantMessage("assistant message", "")
		}
	}

	// Verify we're at MaxHistorySize-2
	if len(m.GetHistory()) != MaxHistorySize-2 {
		t.Fatalf("Setup failed: expected %d messages, got %d", MaxHistorySize-2, len(m.GetHistory()))
	}

	// Add 3 more messages (should trigger trim)
	m.AddUserMessage("final user 1")
	m.AddAssistantMessage("final assistant", "")
	m.AddUserMessage("final user 2")

	history := m.GetHistory()

	// Should be trimmed to MaxHistorySize
	if len(history) != MaxHistorySize {
		t.Errorf("Expected %d messages after trim, got %d", MaxHistorySize, len(history))
	}

	// Last message should be the newest
	if history[len(history)-1].Content != "final user 2" {
		t.Errorf("Last message = %q, expected 'final user 2'", history[len(history)-1].Content)
	}
}

func TestTrimHistory_NotifyPromptEditedTriggersTrim(t *testing.T) {
	m := NewManager()

	// Fill to capacity
	for i := 0; i < MaxHistorySize; i++ {
		m.AddUserMessage("message")
	}

	// Update prompt and notify (adds user message with notification)
	m.UpdatePrompt("new prompt")
	m.NotifyPromptEdited()

	history := m.GetHistory()

	// Should still be at MaxHistorySize (oldest message removed)
	if len(history) != MaxHistorySize {
		t.Errorf("Expected %d messages after notify, got %d", MaxHistorySize, len(history))
	}

	// Last message should be the notification (RoleUser, not RoleSystem)
	last := history[len(history)-1]
	if last.Role != RoleUser {
		t.Errorf("Last message role = %q, expected user", last.Role)
	}
	if last.Content != `[user edited prompt to: "new prompt"]` {
		t.Errorf("Last message content = %q, expected prompt notification", last.Content)
	}
}
