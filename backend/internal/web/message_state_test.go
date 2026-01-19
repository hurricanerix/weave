package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hurricanerix/weave/internal/conversation"
	"github.com/hurricanerix/weave/internal/ollama"
)

func TestHandleMessageState(t *testing.T) {
	tests := []struct {
		name           string
		messageID      string
		setupConv      func(*conversation.Manager)
		wantStatus     int
		wantResponse   *messageStateResponse
		wantErrMessage string
	}{
		{
			name:      "valid message with snapshot",
			messageID: "1",
			setupConv: func(m *conversation.Manager) {
				// Add a message with a snapshot
				metadata := &ollama.LLMMetadata{
					Prompt:        "a fluffy cat in space",
					Steps:         20,
					CFG:           3.5,
					Seed:          42,
					GenerateImage: true,
				}
				m.AddAssistantMessage("Here's your cat!", "a fluffy cat in space", metadata)
			},
			wantStatus: http.StatusOK,
			wantResponse: &messageStateResponse{
				MessageID:     1,
				Prompt:        "a fluffy cat in space",
				Steps:         0, // Settings not tracked in snapshot yet (Task 002 limitation)
				CFG:           0,
				Seed:          0,
				PreviewStatus: "none",
				PreviewURL:    "",
			},
		},
		{
			name:      "message without snapshot",
			messageID: "1",
			setupConv: func(m *conversation.Manager) {
				// Add a pure conversational message (no metadata)
				m.AddAssistantMessage("Hello!", "", nil)
			},
			wantStatus:     http.StatusNotFound,
			wantErrMessage: "Message has no snapshot",
		},
		{
			name:           "message not found",
			messageID:      "999",
			setupConv:      func(m *conversation.Manager) {},
			wantStatus:     http.StatusNotFound,
			wantErrMessage: "Message not found",
		},
		{
			name:           "invalid message ID format",
			messageID:      "abc",
			setupConv:      func(m *conversation.Manager) {},
			wantStatus:     http.StatusBadRequest,
			wantErrMessage: "Invalid message ID",
		},
		{
			name:           "missing message ID",
			messageID:      "",
			setupConv:      func(m *conversation.Manager) {},
			wantStatus:     http.StatusBadRequest,
			wantErrMessage: "Missing message ID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create server with session manager
			s, err := NewServer("")
			if err != nil {
				t.Fatalf("NewServer() error = %v", err)
			}

			// Setup conversation state
			session := s.sessionManager.GetSession("test-session")
			manager := session.Manager()
			tt.setupConv(manager)

			// Create request
			req := httptest.NewRequest("GET", "/message/"+tt.messageID+"/state", nil)
			req.SetPathValue("id", tt.messageID)

			// Set session ID in context
			ctx := setSessionID(req.Context(), "test-session")
			req = req.WithContext(ctx)

			// Create response recorder
			w := httptest.NewRecorder()

			// Call handler
			s.handleMessageState(w, req)

			// Check status code
			if w.Code != tt.wantStatus {
				t.Errorf("status code = %d, want %d", w.Code, tt.wantStatus)
			}

			// Check response body
			if tt.wantResponse != nil {
				// Parse JSON response
				var response messageStateResponse
				if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}

				// Compare fields
				if response.MessageID != tt.wantResponse.MessageID {
					t.Errorf("MessageID = %d, want %d", response.MessageID, tt.wantResponse.MessageID)
				}
				if response.Prompt != tt.wantResponse.Prompt {
					t.Errorf("Prompt = %q, want %q", response.Prompt, tt.wantResponse.Prompt)
				}
				if response.Steps != tt.wantResponse.Steps {
					t.Errorf("Steps = %d, want %d", response.Steps, tt.wantResponse.Steps)
				}
				if response.CFG != tt.wantResponse.CFG {
					t.Errorf("CFG = %.2f, want %.2f", response.CFG, tt.wantResponse.CFG)
				}
				if response.Seed != tt.wantResponse.Seed {
					t.Errorf("Seed = %d, want %d", response.Seed, tt.wantResponse.Seed)
				}
				if response.PreviewStatus != tt.wantResponse.PreviewStatus {
					t.Errorf("PreviewStatus = %q, want %q", response.PreviewStatus, tt.wantResponse.PreviewStatus)
				}
				if response.PreviewURL != tt.wantResponse.PreviewURL {
					t.Errorf("PreviewURL = %q, want %q", response.PreviewURL, tt.wantResponse.PreviewURL)
				}
			}

			// Check error message if expected
			if tt.wantErrMessage != "" {
				bodyStr := w.Body.String()
				if !contains(bodyStr, tt.wantErrMessage) {
					t.Errorf("body does not contain %q, got: %s", tt.wantErrMessage, bodyStr)
				}
			}
		})
	}
}

// contains checks if s contains substr (case-sensitive).
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && (s[:len(substr)] == substr || contains(s[1:], substr))))
}
