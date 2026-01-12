package web

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hurricanerix/weave/internal/conversation"
	"github.com/hurricanerix/weave/internal/ollama"
)

// TestUserFlow_ChatValidation tests the chat endpoint validation.
// Full integration with ollama is tested separately when ollama is available.
func TestUserFlow_ChatValidation(t *testing.T) {
	s, err := NewServer("")
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	tests := []struct {
		name           string
		body           string
		wantStatusCode int
		wantBody       string
	}{
		{
			name:           "empty message rejected",
			body:           "message=",
			wantStatusCode: http.StatusBadRequest,
			wantBody:       "message required",
		},
		{
			name:           "whitespace-only message rejected",
			body:           "message=   ",
			wantStatusCode: http.StatusBadRequest,
			wantBody:       "message required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/chat", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

			// Add session ID to context
			ctx := setSessionID(req.Context(), "test-session")
			req = req.WithContext(ctx)

			w := httptest.NewRecorder()
			s.handleChat(w, req)

			if w.Code != tt.wantStatusCode {
				t.Errorf("status code = %d, want %d", w.Code, tt.wantStatusCode)
			}

			body := w.Body.String()
			if !strings.Contains(body, tt.wantBody) {
				t.Errorf("body = %q, want to contain %q", body, tt.wantBody)
			}
		})
	}
}

// TestUserFlow_PromptUpdate tests the prompt update flow.
func TestUserFlow_PromptUpdate(t *testing.T) {
	s, err := NewServer("")
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	sessionID := "test-session-123"

	// First, update the prompt
	req := httptest.NewRequest("POST", "/prompt", strings.NewReader("prompt=a+fluffy+orange+cat"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	ctx := setSessionID(req.Context(), sessionID)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	s.handlePrompt(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusOK)
	}

	// Verify the prompt was stored
	manager := s.sessionManager.Get(sessionID)
	if manager == nil {
		t.Fatal("session manager not found")
	}

	prompt := manager.GetCurrentPrompt()
	if prompt != "a fluffy orange cat" {
		t.Errorf("prompt = %q, want %q", prompt, "a fluffy orange cat")
	}
}

// TestUserFlow_GenerateWithoutPrompt tests generate fails gracefully without a prompt.
func TestUserFlow_GenerateWithoutPrompt(t *testing.T) {
	s, err := NewServer("")
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	sessionID := "test-session-no-prompt"

	req := httptest.NewRequest("POST", "/generate", nil)
	ctx := setSessionID(req.Context(), sessionID)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	s.handleGenerate(w, req)

	// Should return OK (SSE will send error)
	if w.Code != http.StatusOK {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusOK)
	}
}

// TestUserFlow_GenerateWithPrompt tests generate with a valid prompt.
// When the compute daemon is not running, we expect 503 Service Unavailable.
func TestUserFlow_GenerateWithPrompt(t *testing.T) {
	s, err := NewServer("")
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	sessionID := "test-session-with-prompt"

	// First, set a prompt via the manager
	manager := s.sessionManager.GetOrCreate(sessionID)
	manager.UpdatePrompt("a test prompt for generation")

	req := httptest.NewRequest("POST", "/generate", nil)
	ctx := setSessionID(req.Context(), sessionID)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	s.handleGenerate(w, req)

	// Without a running daemon, we expect 503 Service Unavailable
	// This is correct behavior - the daemon is required for image generation
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

// TestUserFlow_SessionPersistence tests that conversation state persists across requests.
func TestUserFlow_SessionPersistence(t *testing.T) {
	s, err := NewServer("")
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	sessionID := "test-persistence"

	// Add a message to conversation
	manager := s.sessionManager.GetOrCreate(sessionID)
	manager.AddUserMessage("Hello agent")

	// Verify message persists
	history := manager.GetHistory()
	if len(history) != 1 {
		t.Errorf("history length = %d, want 1", len(history))
	}

	if history[0].Content != "Hello agent" {
		t.Errorf("message content = %q, want %q", history[0].Content, "Hello agent")
	}

	// Get manager again (simulating new request)
	manager2 := s.sessionManager.GetOrCreate(sessionID)

	// Should be same manager with same history
	history2 := manager2.GetHistory()
	if len(history2) != 1 {
		t.Errorf("history length after re-get = %d, want 1", len(history2))
	}
}

// TestUserFlow_PromptEditNotification tests that editing the prompt triggers notification.
func TestUserFlow_PromptEditNotification(t *testing.T) {
	s, err := NewServer("")
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	sessionID := "test-edit-notification"

	// Set initial prompt
	manager := s.sessionManager.GetOrCreate(sessionID)
	manager.UpdatePrompt("initial prompt")

	// Verify edited flag is set
	if !manager.IsPromptEdited() {
		t.Error("IsPromptEdited() = false, want true after UpdatePrompt")
	}

	// Call NotifyPromptEdited (simulating what handlePrompt does)
	manager.NotifyPromptEdited()

	// Flag should be cleared
	if manager.IsPromptEdited() {
		t.Error("IsPromptEdited() = true, want false after NotifyPromptEdited")
	}

	// History should contain the notification
	history := manager.GetHistory()
	if len(history) != 1 {
		t.Errorf("history length = %d, want 1", len(history))
	}

	if !strings.Contains(history[0].Content, "user edited prompt to") {
		t.Errorf("notification message = %q, want to contain 'user edited prompt to'", history[0].Content)
	}
}

// TestUserFlow_ChatStreaming tests chat streaming with SSE events.
// This test verifies that the streaming callback properly handles errors
// when the client disconnects (no SSE connection exists).
func TestUserFlow_ChatStreaming(t *testing.T) {
	s, err := NewServer("")
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	sessionID := "test-chat-streaming"

	// Create a mock ollama client that streams tokens
	mockClient := &mockOllamaClient{
		response: "Here's a prompt for your cat",
		metadata: ollama.LLMMetadata{
			Prompt: "a fluffy orange cat",
		},
		tokens: []string{
			"Here's ", "a ", "prompt ", "for ", "your ", "cat",
		},
	}
	// Replace the ollama client with mock for testing
	s.setOllamaClientForTesting(mockClient)

	// Send chat request without SSE connection
	// This simulates a client disconnect scenario
	chatReq := httptest.NewRequest("POST", "/chat", strings.NewReader("message=I+want+a+cat"))
	chatReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	ctx := setSessionID(chatReq.Context(), sessionID)
	chatReq = chatReq.WithContext(ctx)
	chatRec := httptest.NewRecorder()

	s.handleChat(chatRec, chatReq)

	// Verify HTTP response is still OK (error sent via SSE, not HTTP error)
	if chatRec.Code != http.StatusOK {
		t.Errorf("chat status code = %d, want %d", chatRec.Code, http.StatusOK)
	}

	// Verify conversation state - user message should be added
	manager := s.sessionManager.Get(sessionID)
	if manager == nil {
		t.Fatal("session manager not found")
	}

	history := manager.GetHistory()
	// Without SSE connection, streaming aborts early so only user message is added
	if len(history) < 1 {
		t.Fatalf("history length = %d, want at least 1 (user message)", len(history))
	}

	// Verify user message was added
	if history[0].Role != conversation.RoleUser || history[0].Content != "I want a cat" {
		t.Errorf("first message = {%s, %s}, want {user, I want a cat}", history[0].Role, history[0].Content)
	}

	// Note: Assistant message is NOT added because streaming aborted due to no SSE connection
	// This is correct behavior - if client disconnects, we stop processing
}

// mockOllamaClient is a mock implementation of ollama.Client for testing.
// It supports both single-response mode (for simple tests) and multi-response mode (for retry tests).
type mockOllamaClient struct {
	// Single-response mode fields (legacy support for existing tests)
	response string
	metadata ollama.LLMMetadata
	tokens   []string
	err      error

	// Multi-response mode fields (for retry testing)
	// If responses is non-nil, it takes precedence over single-response fields
	responses []mockResponse
	callCount int
}

type mockResponse struct {
	result ollama.ChatResult
	tokens []string
	err    error
}

// Chat simulates streaming tokens to the callback.
func (m *mockOllamaClient) Chat(ctx context.Context, messages []ollama.Message, seed *int64, callback ollama.StreamCallback) (ollama.ChatResult, error) {
	// Multi-response mode (for retry testing)
	if m.responses != nil {
		if m.callCount >= len(m.responses) {
			return ollama.ChatResult{}, errors.New("no more mock responses configured")
		}

		resp := m.responses[m.callCount]
		m.callCount++

		if resp.err != nil {
			return ollama.ChatResult{}, resp.err
		}

		// Stream tokens if callback provided
		if callback != nil {
			tokens := resp.tokens
			if len(tokens) == 0 && resp.result.Response != "" {
				// Auto-generate tokens from response if not explicitly provided
				tokens = strings.Split(resp.result.Response, " ")
			}

			for _, token := range tokens {
				if err := callback(ollama.StreamToken{Content: token + " ", Done: false}); err != nil {
					return ollama.ChatResult{}, err
				}
			}

			// Send final token
			if err := callback(ollama.StreamToken{Content: "", Done: true}); err != nil {
				return ollama.ChatResult{}, err
			}
		}

		return resp.result, nil
	}

	// Single-response mode (legacy support)
	if m.err != nil {
		return ollama.ChatResult{}, m.err
	}

	// Stream tokens
	for _, token := range m.tokens {
		if err := callback(ollama.StreamToken{Content: token, Done: false}); err != nil {
			return ollama.ChatResult{}, err
		}
	}

	// Send final token
	if err := callback(ollama.StreamToken{Content: "", Done: true}); err != nil {
		return ollama.ChatResult{}, err
	}

	return ollama.ChatResult{
		Response: m.response,
		Metadata: m.metadata,
	}, nil
}
