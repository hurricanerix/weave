//go:build integration

package ollama

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// These tests require a running ollama instance with llama3.2:1b model.
// Run with: go test -tags=integration ./internal/ollama/...

// checkOllamaRunning verifies ollama is accessible before running tests.
func checkOllamaRunning(t *testing.T) {
	t.Helper()

	client := NewClient()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Skipf("Skipping integration test: %v", err)
	}
}

func TestIntegrationConnect(t *testing.T) {
	checkOllamaRunning(t)

	client := NewClient()
	ctx := context.Background()

	err := client.Connect(ctx)
	if err != nil {
		t.Fatalf("Connect() failed: %v", err)
	}
}

func TestIntegrationChatStreaming(t *testing.T) {
	checkOllamaRunning(t)

	client := NewClient()
	ctx := context.Background()

	messages := []Message{
		{Role: RoleSystem, Content: "You are a helpful assistant. Respond briefly."},
		{Role: RoleUser, Content: "Say hello in exactly three words."},
	}

	var tokens []string
	callback := func(token StreamToken) error {
		tokens = append(tokens, token.Content)
		return nil
	}

	response, err := client.Chat(ctx, messages, nil, callback)
	if err != nil {
		t.Fatalf("Chat() failed: %v", err)
	}

	// Verify streaming tokens were received
	if len(tokens) == 0 {
		t.Error("Expected streaming tokens, got none")
	}

	// Verify response is non-empty
	if response == "" {
		t.Error("Expected non-empty response")
	}

	// Verify concatenated tokens match response
	concatenated := strings.Join(tokens, "")
	if concatenated != response {
		t.Errorf("Concatenated tokens %q != response %q", concatenated, response)
	}
}

func TestIntegrationMultiTurnConversation(t *testing.T) {
	checkOllamaRunning(t)

	client := NewClient()
	ctx := context.Background()

	// First turn: introduce a topic
	messages := []Message{
		{Role: RoleSystem, Content: "You are a helpful assistant. Be brief."},
		{Role: RoleUser, Content: "The magic word is 'banana'. Remember it."},
	}

	response1, err := client.Chat(ctx, messages, nil, nil)
	if err != nil {
		t.Fatalf("First turn failed: %v", err)
	}

	// Second turn: ask about the topic to verify context is maintained
	messages = append(messages,
		Message{Role: RoleAssistant, Content: response1},
		Message{Role: RoleUser, Content: "What is the magic word?"},
	)

	response2, err := client.Chat(ctx, messages, nil, nil)
	if err != nil {
		t.Fatalf("Second turn failed: %v", err)
	}

	// The response should contain "banana" (the model remembered the context)
	if !strings.Contains(strings.ToLower(response2), "banana") {
		t.Errorf("Expected response to contain 'banana', got: %s", response2)
	}
}

func TestIntegrationPromptExtraction(t *testing.T) {
	checkOllamaRunning(t)

	client := NewClient()
	ctx := context.Background()

	// Use the actual system prompt to get a response with Prompt: line
	messages := []Message{
		{Role: RoleSystem, Content: SystemPrompt},
		{Role: RoleUser, Content: "A realistic photo of a tabby cat wearing a blue wizard hat in a cozy library."},
	}

	// Use a seed for more predictable output
	seed := int64(42)
	response, err := client.Chat(ctx, messages, &seed, nil)
	if err != nil {
		t.Fatalf("Chat() failed: %v", err)
	}

	t.Logf("Response: %s", response)

	// The agent might ask questions or provide a prompt
	// This test verifies the extraction works, not that the agent always provides a prompt
	prompt := ExtractPrompt(response)
	t.Logf("Extracted prompt: %q", prompt)

	// If a prompt was extracted, verify it's non-empty and doesn't have prefix
	if prompt != "" {
		if strings.HasPrefix(prompt, PromptPrefix) {
			t.Errorf("Extracted prompt still has prefix: %q", prompt)
		}
	}
}

func TestIntegrationSeedDeterminism(t *testing.T) {
	checkOllamaRunning(t)

	client := NewClient()
	ctx := context.Background()

	messages := []Message{
		{Role: RoleSystem, Content: "You are a helpful assistant."},
		{Role: RoleUser, Content: "Generate a random three-digit number."},
	}

	seed := int64(12345)

	// Generate response with same seed twice
	response1, err := client.Chat(ctx, messages, &seed, nil)
	if err != nil {
		t.Fatalf("First Chat() failed: %v", err)
	}

	response2, err := client.Chat(ctx, messages, &seed, nil)
	if err != nil {
		t.Fatalf("Second Chat() failed: %v", err)
	}

	// With same seed, responses should be identical
	if response1 != response2 {
		t.Errorf("Expected identical responses with same seed:\n  Response 1: %q\n  Response 2: %q", response1, response2)
	}
}

func TestIntegrationContextCancellation(t *testing.T) {
	checkOllamaRunning(t)

	client := NewClient()

	// Create a context that's already cancelled
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	messages := []Message{
		{Role: RoleUser, Content: "Hello"},
	}

	_, err := client.Chat(ctx, messages, nil, nil)
	if err == nil {
		t.Fatal("Expected error with cancelled context")
	}

	// Error should indicate context was cancelled
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Expected context.Canceled error, got: %v", err)
	}
}
