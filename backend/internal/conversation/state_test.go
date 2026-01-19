package conversation

import (
	"testing"

	"github.com/hurricanerix/weave/internal/ollama"
)

// TestAddUserMessageAssignsID tests that user messages get sequential IDs.
func TestAddUserMessageAssignsID(t *testing.T) {
	m := NewManager()

	id1 := m.AddUserMessage("first message")
	id2 := m.AddUserMessage("second message")
	id3 := m.AddUserMessage("third message")

	if id1 != 1 {
		t.Errorf("First message ID = %d, want 1", id1)
	}
	if id2 != 2 {
		t.Errorf("Second message ID = %d, want 2", id2)
	}
	if id3 != 3 {
		t.Errorf("Third message ID = %d, want 3", id3)
	}
}

// TestAddAssistantMessageAssignsID tests that assistant messages get sequential IDs.
func TestAddAssistantMessageAssignsID(t *testing.T) {
	m := NewManager()

	id1 := m.AddAssistantMessage("first response", "", nil)
	id2 := m.AddAssistantMessage("second response", "", nil)
	id3 := m.AddAssistantMessage("third response", "", nil)

	if id1 != 1 {
		t.Errorf("First message ID = %d, want 1", id1)
	}
	if id2 != 2 {
		t.Errorf("Second message ID = %d, want 2", id2)
	}
	if id3 != 3 {
		t.Errorf("Third message ID = %d, want 3", id3)
	}
}

// TestMixedMessagesSequentialIDs tests that IDs increment across user and assistant messages.
func TestMixedMessagesSequentialIDs(t *testing.T) {
	m := NewManager()

	id1 := m.AddUserMessage("user 1")
	id2 := m.AddAssistantMessage("assistant 1", "", nil)
	id3 := m.AddUserMessage("user 2")
	id4 := m.AddAssistantMessage("assistant 2", "", nil)

	expected := []int{1, 2, 3, 4}
	actual := []int{id1, id2, id3, id4}

	for i, exp := range expected {
		if actual[i] != exp {
			t.Errorf("Message %d: ID = %d, want %d", i+1, actual[i], exp)
		}
	}
}

// TestSnapshotCreatedWhenPromptChanges tests that snapshots are created when metadata contains a new prompt.
func TestSnapshotCreatedWhenPromptChanges(t *testing.T) {
	m := NewManager()

	// First message with prompt - should create snapshot (first prompt)
	metadata1 := &ollama.LLMMetadata{
		Prompt: "a cat",
	}
	id1 := m.AddAssistantMessage("Here's a cat", "a cat", metadata1)

	msg1 := m.GetMessage(id1)
	if msg1 == nil {
		t.Fatal("Message 1 not found")
	}
	if msg1.Snapshot == nil {
		t.Error("Expected snapshot for first prompt, got nil")
	} else {
		if msg1.Snapshot.Prompt != "a cat" {
			t.Errorf("Snapshot prompt = %q, want %q", msg1.Snapshot.Prompt, "a cat")
		}
		if msg1.Snapshot.PreviewStatus != PreviewStatusNone {
			t.Errorf("Snapshot preview status = %q, want %q", msg1.Snapshot.PreviewStatus, PreviewStatusNone)
		}
	}

	// Second message with different prompt - should create snapshot
	metadata2 := &ollama.LLMMetadata{
		Prompt: "a dog",
	}
	id2 := m.AddAssistantMessage("Here's a dog", "a dog", metadata2)

	msg2 := m.GetMessage(id2)
	if msg2 == nil {
		t.Fatal("Message 2 not found")
	}
	if msg2.Snapshot == nil {
		t.Error("Expected snapshot for changed prompt, got nil")
	} else {
		if msg2.Snapshot.Prompt != "a dog" {
			t.Errorf("Snapshot prompt = %q, want %q", msg2.Snapshot.Prompt, "a dog")
		}
	}
}

// TestNoSnapshotWhenPromptUnchanged tests that no snapshot is created when prompt doesn't change.
func TestNoSnapshotWhenPromptUnchanged(t *testing.T) {
	m := NewManager()

	// First message with prompt
	metadata1 := &ollama.LLMMetadata{
		Prompt: "a cat",
	}
	id1 := m.AddAssistantMessage("Here's a cat", "a cat", metadata1)

	msg1 := m.GetMessage(id1)
	if msg1 == nil {
		t.Fatal("Message 1 not found")
	}
	if msg1.Snapshot == nil {
		t.Fatal("Expected snapshot for first prompt")
	}

	// Second message with same prompt - should NOT create snapshot
	metadata2 := &ollama.LLMMetadata{
		Prompt: "a cat",
	}
	id2 := m.AddAssistantMessage("Same cat, different words", "a cat", metadata2)

	msg2 := m.GetMessage(id2)
	if msg2 == nil {
		t.Fatal("Message 2 not found")
	}
	if msg2.Snapshot != nil {
		t.Error("Expected no snapshot for unchanged prompt, got snapshot")
	}
}

// TestNoSnapshotWhenNoMetadata tests that no snapshot is created when metadata is nil.
func TestNoSnapshotWhenNoMetadata(t *testing.T) {
	m := NewManager()

	id := m.AddAssistantMessage("Just conversation", "", nil)

	msg := m.GetMessage(id)
	if msg == nil {
		t.Fatal("Message not found")
	}
	if msg.Snapshot != nil {
		t.Error("Expected no snapshot for nil metadata, got snapshot")
	}
}

// TestNoSnapshotWhenEmptyPrompt tests that no snapshot is created when prompt is empty.
func TestNoSnapshotWhenEmptyPrompt(t *testing.T) {
	m := NewManager()

	metadata := &ollama.LLMMetadata{
		Prompt: "",
	}
	id := m.AddAssistantMessage("Asking questions", "", metadata)

	msg := m.GetMessage(id)
	if msg == nil {
		t.Fatal("Message not found")
	}
	if msg.Snapshot != nil {
		t.Error("Expected no snapshot for empty prompt, got snapshot")
	}
}

// TestGetMessageByID tests retrieving messages by their ID.
func TestGetMessageByID(t *testing.T) {
	m := NewManager()

	id1 := m.AddUserMessage("user message")
	id2 := m.AddAssistantMessage("assistant message", "", nil)

	msg1 := m.GetMessage(id1)
	if msg1 == nil {
		t.Fatal("Message 1 not found")
	}
	if msg1.ID != id1 {
		t.Errorf("Message 1 ID = %d, want %d", msg1.ID, id1)
	}
	if msg1.Role != RoleUser {
		t.Errorf("Message 1 role = %q, want %q", msg1.Role, RoleUser)
	}
	if msg1.Content != "user message" {
		t.Errorf("Message 1 content = %q, want %q", msg1.Content, "user message")
	}

	msg2 := m.GetMessage(id2)
	if msg2 == nil {
		t.Fatal("Message 2 not found")
	}
	if msg2.ID != id2 {
		t.Errorf("Message 2 ID = %d, want %d", msg2.ID, id2)
	}
	if msg2.Role != RoleAssistant {
		t.Errorf("Message 2 role = %q, want %q", msg2.Role, RoleAssistant)
	}
}

// TestGetMessageByIDNotFound tests that GetMessage returns nil for unknown IDs.
func TestGetMessageByIDNotFound(t *testing.T) {
	m := NewManager()

	m.AddUserMessage("message")

	msg := m.GetMessage(999)
	if msg != nil {
		t.Error("Expected nil for unknown message ID, got message")
	}
}

// TestUpdateMessagePreview tests updating the preview status and URL.
func TestUpdateMessagePreview(t *testing.T) {
	m := NewManager()

	metadata := &ollama.LLMMetadata{
		Prompt: "a cat",
	}
	id := m.AddAssistantMessage("Here's a cat", "a cat", metadata)

	// Initially should be "none" with empty URL
	msg := m.GetMessage(id)
	if msg == nil {
		t.Fatal("Message not found")
	}
	if msg.Snapshot == nil {
		t.Fatal("Expected snapshot")
	}
	if msg.Snapshot.PreviewStatus != PreviewStatusNone {
		t.Errorf("Initial preview status = %q, want %q", msg.Snapshot.PreviewStatus, PreviewStatusNone)
	}
	if msg.Snapshot.PreviewURL != "" {
		t.Errorf("Initial preview URL = %q, want empty", msg.Snapshot.PreviewURL)
	}

	// Update to generating
	m.UpdateMessagePreview(id, PreviewStatusGenerating, "")

	msg = m.GetMessage(id)
	if msg.Snapshot.PreviewStatus != PreviewStatusGenerating {
		t.Errorf("Preview status = %q, want %q", msg.Snapshot.PreviewStatus, PreviewStatusGenerating)
	}

	// Update to complete with URL
	m.UpdateMessagePreview(id, PreviewStatusComplete, "/images/123.png")

	msg = m.GetMessage(id)
	if msg.Snapshot.PreviewStatus != PreviewStatusComplete {
		t.Errorf("Preview status = %q, want %q", msg.Snapshot.PreviewStatus, PreviewStatusComplete)
	}
	if msg.Snapshot.PreviewURL != "/images/123.png" {
		t.Errorf("Preview URL = %q, want %q", msg.Snapshot.PreviewURL, "/images/123.png")
	}
}

// TestUpdateMessagePreviewNoSnapshot tests that UpdateMessagePreview does nothing for messages without snapshots.
func TestUpdateMessagePreviewNoSnapshot(t *testing.T) {
	m := NewManager()

	id := m.AddAssistantMessage("No snapshot", "", nil)

	// Should not panic
	m.UpdateMessagePreview(id, PreviewStatusComplete, "/image.png")

	msg := m.GetMessage(id)
	if msg == nil {
		t.Fatal("Message not found")
	}
	if msg.Snapshot != nil {
		t.Error("Expected no snapshot, got snapshot")
	}
}

// TestUpdateMessagePreviewNotFound tests that UpdateMessagePreview does nothing for unknown IDs.
func TestUpdateMessagePreviewNotFound(t *testing.T) {
	m := NewManager()

	// Should not panic
	m.UpdateMessagePreview(999, PreviewStatusComplete, "/image.png")
}

// TestClearResetsMessageID tests that Clear resets the message ID counter.
func TestClearResetsMessageID(t *testing.T) {
	m := NewManager()

	id1 := m.AddUserMessage("first")
	if id1 != 1 {
		t.Errorf("First ID = %d, want 1", id1)
	}

	id2 := m.AddUserMessage("second")
	if id2 != 2 {
		t.Errorf("Second ID = %d, want 2", id2)
	}

	m.Clear()

	// After clear, IDs should restart at 1
	id3 := m.AddUserMessage("after clear")
	if id3 != 1 {
		t.Errorf("ID after clear = %d, want 1", id3)
	}
}

// TestNotifyPromptEditedAssignsID tests that edit notifications get sequential IDs.
func TestNotifyPromptEditedAssignsID(t *testing.T) {
	m := NewManager()

	id1 := m.AddUserMessage("user message")
	m.UpdatePrompt("new prompt")

	// NotifyPromptEdited should create a message with ID 2
	m.NotifyPromptEdited()

	// Add another message - should be ID 3
	id2 := m.AddUserMessage("another message")

	if id2 != 3 {
		t.Errorf("Message after notification ID = %d, want 3", id2)
	}

	// Verify the notification message has ID 2
	msg := m.GetMessage(2)
	if msg == nil {
		t.Fatal("Notification message not found")
	}
	if msg.Role != RoleUser {
		t.Errorf("Notification role = %q, want %q", msg.Role, RoleUser)
	}
	if msg.ID != 2 {
		t.Errorf("Notification ID = %d, want 2", msg.ID)
	}

	// Ensure the first user message has ID 1
	msg1 := m.GetMessage(id1)
	if msg1 == nil || msg1.ID != 1 {
		t.Error("First user message should have ID 1")
	}
}
