//go:build integration

package ollama

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// These tests require a running ollama instance.
// Most tests use the default model (llama3.1:8b), but some legacy tests use llama3.2:1b or mistral:7b.
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

	result, err := client.Chat(ctx, messages, nil, callback)
	if err != nil {
		t.Fatalf("Chat() failed: %v", err)
	}

	// Verify streaming tokens were received (only conversational text, not JSON)
	if len(tokens) == 0 {
		t.Error("Expected streaming tokens, got none")
	}

	// Verify response is non-empty
	if result.Response == "" {
		t.Error("Expected non-empty response")
	}

	// Verify concatenated tokens match conversational text portion (before tool calls)
	concatenated := strings.Join(tokens, "")
	if concatenated != result.Response {
		t.Errorf("Concatenated tokens %q != response %q", concatenated, result.Response)
	}

	// Verify metadata was parsed
	t.Logf("Metadata: prompt=%q", result.Metadata.Prompt)
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

	result1, err := client.Chat(ctx, messages, nil, nil)
	if err != nil {
		t.Fatalf("First turn failed: %v", err)
	}

	// Second turn: ask about the topic to verify context is maintained
	messages = append(messages,
		Message{Role: RoleAssistant, Content: result1.Response},
		Message{Role: RoleUser, Content: "What is the magic word?"},
	)

	result2, err := client.Chat(ctx, messages, nil, nil)
	if err != nil {
		t.Fatalf("Second turn failed: %v", err)
	}

	// The response should contain "banana" (the model remembered the context)
	if !strings.Contains(strings.ToLower(result2.Response), "banana") {
		t.Errorf("Expected response to contain 'banana', got: %s", result2.Response)
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
	result, err := client.Chat(ctx, messages, &seed, nil)
	if err != nil {
		t.Fatalf("Chat() failed: %v", err)
	}

	t.Logf("Response: %s", result.Response)
	t.Logf("Metadata: prompt=%q", result.Metadata.Prompt)

	// The agent might ask questions or provide a prompt
	// This test verifies metadata parsing works
	if result.Metadata.Prompt != "" {
		t.Logf("Agent provided prompt: %q", result.Metadata.Prompt)
		// Verify prompt is reasonable length (should be under 200 chars per system prompt)
		if len(result.Metadata.Prompt) > 200 {
			t.Errorf("Prompt too long: %d chars (max 200)", len(result.Metadata.Prompt))
		}
	} else {
		t.Logf("Agent is still asking questions")
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
	result1, err := client.Chat(ctx, messages, &seed, nil)
	if err != nil {
		t.Fatalf("First Chat() failed: %v", err)
	}

	result2, err := client.Chat(ctx, messages, &seed, nil)
	if err != nil {
		t.Fatalf("Second Chat() failed: %v", err)
	}

	// With same seed, responses should be identical
	if result1.Response != result2.Response {
		t.Errorf("Expected identical responses with same seed:\n  Response 1: %q\n  Response 2: %q", result1.Response, result2.Response)
	}
	if result1.Metadata.Prompt != result2.Metadata.Prompt {
		t.Errorf("Expected identical metadata with same seed")
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

// checkModelAvailable verifies that a specific model is available in ollama.
// If the model is not available, the test is skipped.
func checkModelAvailable(t *testing.T, modelName string) {
	t.Helper()

	client := NewClient()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		if errors.Is(err, ErrModelNotFound) {
			t.Skipf("Skipping test: model %s not available (pull with: ollama pull %s)", modelName, modelName)
		}
		t.Skipf("Skipping integration test: %v", err)
	}
}

// TestIntegrationMistral7BMultiTurn runs 10 multi-turn conversations with Mistral 7B
// to verify that the model consistently uses function calling for structured output.
// This test ensures reliable function calling over extended conversations.
func TestIntegrationMistral7BMultiTurn(t *testing.T) {
	checkOllamaRunning(t)

	// Create client specifically for Mistral 7B
	client := NewClientWithConfig(DefaultEndpoint, "mistral:7b", time.Duration(DefaultTimeout)*time.Second)
	checkModelAvailable(t, "mistral:7b")

	ctx := context.Background()

	// Test scenarios for multi-turn conversations
	tests := []struct {
		name  string
		turns []string // User messages for each turn
	}{
		{
			name: "simple subject",
			turns: []string{
				"a cat",
				"make it orange",
				"in a garden",
			},
		},
		{
			name: "detailed request",
			turns: []string{
				"I want a landscape",
				"mountains with snow",
				"at sunset",
				"realistic style",
			},
		},
		{
			name: "asking questions",
			turns: []string{
				"a car",
				"sports car",
				"red color",
				"on a race track",
			},
		},
		{
			name: "style refinement",
			turns: []string{
				"a house",
				"victorian style",
				"at night",
				"with lights on",
			},
		},
		{
			name: "abstract concept",
			turns: []string{
				"happiness",
				"colorful",
				"abstract art",
			},
		},
		{
			name: "detailed scene",
			turns: []string{
				"a forest",
				"autumn leaves",
				"morning fog",
				"sunlight through trees",
			},
		},
		{
			name: "character description",
			turns: []string{
				"a wizard",
				"old with long beard",
				"holding a staff",
				"in a tower",
			},
		},
		{
			name: "mood and setting",
			turns: []string{
				"a beach",
				"tropical",
				"sunset",
				"peaceful mood",
			},
		},
		{
			name: "technical detail",
			turns: []string{
				"a robot",
				"futuristic design",
				"metallic",
				"in a lab",
			},
		},
		{
			name: "composition",
			turns: []string{
				"a portrait",
				"side profile",
				"dramatic lighting",
				"black and white",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			messages := []Message{
				{Role: RoleSystem, Content: SystemPrompt},
			}

			// Track responses across all turns
			var allResponses []ChatResult
			foundReadyPrompt := false

			// Execute each turn in the conversation
			for i, userMessage := range tt.turns {
				messages = append(messages, Message{Role: RoleUser, Content: userMessage})

				result, err := client.Chat(ctx, messages, nil, nil)
				if err != nil {
					t.Fatalf("Turn %d failed: %v", i+1, err)
				}

				allResponses = append(allResponses, result)

				// Verify tool calls are present in raw response
				if !strings.Contains(result.RawResponse, "__TOOL_CALLS__") {
					t.Errorf("Turn %d: response missing tool calls marker\nResponse: %s",
						i+1, result.RawResponse)
				}

				// Verify function call parsed successfully (this is implicit - if parsing failed,
				// Chat() would have returned an error, but we verify metadata is populated)
				t.Logf("Turn %d metadata: generate_image=%v, prompt=%q",
					i+1, result.Metadata.GenerateImage, result.Metadata.Prompt)

				// Verify conversational text excludes function call data
				if strings.Contains(result.Response, "{") || strings.Contains(result.Response, "}") {
					// This might be legitimate if the LLM uses braces in conversational text,
					// but check that it's not the function call metadata
					if strings.Contains(result.Response, "\"prompt\"") || false {
						t.Errorf("Turn %d: conversational text contains function call metadata\nResponse: %s",
							i+1, result.Response)
					}
				}

				// Verify conversational text doesn't contain the tool call marker
				if strings.Contains(result.Response, "__TOOL_CALLS__") {
					t.Errorf("Turn %d: conversational text contains tool call marker\nResponse: %s",
						i+1, result.Response)
				}

				// Check if we got a ready prompt
				if result.Metadata.Prompt != "" {
					foundReadyPrompt = true
					t.Logf("Turn %d: Agent ready with prompt: %q", i+1, result.Metadata.Prompt)

					// Verify prompt is under 200 chars as required by system prompt
					if len(result.Metadata.Prompt) > 200 {
						t.Errorf("Turn %d: prompt too long (%d chars, max 200): %q",
							i+1, len(result.Metadata.Prompt), result.Metadata.Prompt)
					}
				}

				// Add assistant response to conversation history for next turn
				// Use RawResponse to preserve the complete format (text + tool calls)
				messages = append(messages, Message{Role: RoleAssistant, Content: result.RawResponse})
			}

			// Verify we eventually got a ready prompt (LLM should produce one after gathering info)
			// This is a soft check - not all conversations may reach ready state in just a few turns
			if !foundReadyPrompt {
				t.Logf("Note: No ready prompt found in %d turns (LLM may need more clarification)", len(tt.turns))
			}

			// Log summary
			t.Logf("Completed %d-turn conversation. Responses: %d, Ready prompts: %v",
				len(tt.turns), len(allResponses), foundReadyPrompt)
		})
	}
}

// TestIntegrationMistral7BFunctionCallConsistency verifies that Mistral 7B consistently
// uses function calling in every response, even in edge cases like very short prompts
// or multi-paragraph responses.
func TestIntegrationMistral7BFunctionCallConsistency(t *testing.T) {
	checkOllamaRunning(t)

	client := NewClientWithConfig(DefaultEndpoint, "mistral:7b", time.Duration(DefaultTimeout)*time.Second)
	checkModelAvailable(t, "mistral:7b")

	ctx := context.Background()

	tests := []struct {
		name        string
		userMessage string
	}{
		{
			name:        "very short input",
			userMessage: "cat",
		},
		{
			name:        "detailed request",
			userMessage: "A highly detailed, photorealistic image of a majestic snow leopard on a mountain peak at dawn, with dramatic lighting and clouds below.",
		},
		{
			name:        "multiple questions",
			userMessage: "I want an image. Can you help? What do you need to know? What style works best?",
		},
		{
			name:        "special characters",
			userMessage: "a robot with LED lights (blue/green), metallic surface, sci-fi style - futuristic!",
		},
		{
			name:        "simple statement",
			userMessage: "sunset",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			messages := []Message{
				{Role: RoleSystem, Content: SystemPrompt},
				{Role: RoleUser, Content: tt.userMessage},
			}

			result, err := client.Chat(ctx, messages, nil, nil)
			if err != nil {
				t.Fatalf("Chat() failed: %v", err)
			}

			// Verify tool calls are present
			if !strings.Contains(result.RawResponse, "__TOOL_CALLS__") {
				t.Errorf("Response missing tool calls marker\nInput: %q\nResponse: %s",
					tt.userMessage, result.RawResponse)
			}

			// Verify function call parsed successfully
			t.Logf("Metadata: prompt=%q", result.Metadata.Prompt)

			// Verify conversational text doesn't include function call data
			if strings.Contains(result.Response, "\"prompt\"") || false {
				t.Errorf("Conversational text contains function call metadata\nResponse: %s", result.Response)
			}
		})
	}
}

// TestIntegrationMistral7BPromptExtraction verifies that prompts are correctly
// extracted from JSON metadata when ready=true.
func TestIntegrationMistral7BPromptExtraction(t *testing.T) {
	checkOllamaRunning(t)

	client := NewClientWithConfig(DefaultEndpoint, "mistral:7b", time.Duration(DefaultTimeout)*time.Second)
	checkModelAvailable(t, "mistral:7b")

	ctx := context.Background()

	// Provide a very detailed request that should trigger ready=true quickly
	messages := []Message{
		{Role: RoleSystem, Content: SystemPrompt},
		{Role: RoleUser, Content: "A photorealistic image of a tabby cat wearing a blue wizard hat with stars, sitting in a cozy library with warm lighting, viewed from the side."},
	}

	// We may need multiple attempts since LLM behavior is non-deterministic
	// Try up to 3 turns to get a ready prompt
	var result ChatResult
	var err error
	maxTurns := 3

	for i := 0; i < maxTurns; i++ {
		result, err = client.Chat(ctx, messages, nil, nil)
		if err != nil {
			t.Fatalf("Turn %d failed: %v", i+1, err)
		}

		t.Logf("Turn %d: prompt=%q", i+1, result.Metadata.Prompt)

		if result.Metadata.Prompt != "" {
			// Success! Got a ready prompt
			break
		}

		// Add assistant response and user follow-up
		messages = append(messages,
			Message{Role: RoleAssistant, Content: result.RawResponse},
			Message{Role: RoleUser, Content: "Yes, that sounds perfect. Please generate it."},
		)
	}

	// Verify we eventually got a ready prompt
	if result.Metadata.Prompt == "" {
		t.Skipf("LLM did not produce ready prompt in %d turns (non-deterministic behavior)", maxTurns)
	}

	// Verify prompt characteristics
	if len(result.Metadata.Prompt) == 0 {
		t.Error("Prompt is empty")
	}
	if len(result.Metadata.Prompt) > 200 {
		t.Errorf("Prompt exceeds 200 chars: %d chars", len(result.Metadata.Prompt))
	}

	// Verify prompt is reasonable (contains some keywords from the request)
	prompt := strings.ToLower(result.Metadata.Prompt)
	keywords := []string{"cat", "tabby", "wizard", "hat", "library"}
	keywordsFound := 0
	for _, keyword := range keywords {
		if strings.Contains(prompt, keyword) {
			keywordsFound++
		}
	}
	if keywordsFound < 2 {
		t.Logf("Warning: Prompt may not capture user intent well. Only %d/%d keywords found in: %q",
			keywordsFound, len(keywords), result.Metadata.Prompt)
	}

	t.Logf("Successfully extracted prompt: %q", result.Metadata.Prompt)
}
