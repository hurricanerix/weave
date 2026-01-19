package persistence

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hurricanerix/weave/internal/conversation"
)

func TestSessionStore_Save(t *testing.T) {
	tests := []struct {
		name      string
		sessionID string
		setup     func(*conversation.Conversation)
		wantErr   bool
	}{
		{
			name:      "empty conversation",
			sessionID: createTestSessionID(1),
			setup:     func(c *conversation.Conversation) {},
			wantErr:   false,
		},
		{
			name:      "conversation with messages",
			sessionID: createTestSessionID(2),
			setup: func(c *conversation.Conversation) {
				msgs := []conversation.ConversationMessage{
					{
						ID:      1,
						Role:    conversation.RoleUser,
						Content: "I want a cat picture",
					},
					{
						ID:      2,
						Role:    conversation.RoleAssistant,
						Content: "Here's a prompt for you",
						Snapshot: &conversation.StateSnapshot{
							Prompt:        "a fluffy cat",
							Steps:         20,
							CFG:           3.5,
							Seed:          -1,
							PreviewStatus: conversation.PreviewStatusComplete,
							PreviewURL:    "/sessions/1/images/2.png",
						},
					},
				}
				c.SetMessages(msgs)
				c.SetNextMessageID(3)
				c.SetCurrentPrompt("a fluffy cat")
			},
			wantErr: false,
		},
		{
			name:      "empty session ID",
			sessionID: "",
			setup:     func(c *conversation.Conversation) {},
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory for test
			tmpDir := t.TempDir()
			store := NewSessionStore(tmpDir)

			// Setup conversation
			conv := conversation.NewConversation()
			if tt.setup != nil {
				tt.setup(conv)
			}

			// Save
			err := store.Save(tt.sessionID, conv)

			// Check error
			if (err != nil) != tt.wantErr {
				t.Errorf("Save() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// If save succeeded, verify directory structure
			if err == nil && tt.sessionID != "" {
				sessionDir := filepath.Join(tmpDir, tt.sessionID)
				convPath := filepath.Join(sessionDir, "conversation.json")
				imagesDir := filepath.Join(sessionDir, "images")

				// Check session directory exists
				if _, err := os.Stat(sessionDir); os.IsNotExist(err) {
					t.Errorf("session directory not created")
				}

				// Check conversation.json exists
				if _, err := os.Stat(convPath); os.IsNotExist(err) {
					t.Errorf("conversation.json not created")
				}

				// Check images directory exists
				if _, err := os.Stat(imagesDir); os.IsNotExist(err) {
					t.Errorf("images directory not created")
				}

				// Verify file permissions
				info, err := os.Stat(convPath)
				if err != nil {
					t.Fatalf("failed to stat conversation.json: %v", err)
				}
				if info.Mode().Perm() != 0600 {
					t.Errorf("conversation.json permissions = %o, want 0600", info.Mode().Perm())
				}

				// Verify directory permissions (0700: owner-only access)
				dirInfo, err := os.Stat(sessionDir)
				if err != nil {
					t.Fatalf("failed to stat session directory: %v", err)
				}
				if dirInfo.Mode().Perm() != 0700 {
					t.Errorf("session directory permissions = %o, want 0700", dirInfo.Mode().Perm())
				}
			}
		})
	}
}

func TestSessionStore_Save_NilConversation(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewSessionStore(tmpDir)

	err := store.Save(createTestSessionID(0), nil)
	if err == nil {
		t.Error("Save() with nil conversation should return error")
	}
}

func TestSessionStore_Load(t *testing.T) {
	tests := []struct {
		name       string
		sessionID  string
		setupStore func(*SessionStore, *conversation.Conversation)
		wantErr    bool
		checkFunc  func(*testing.T, *conversation.Conversation)
	}{
		{
			name:      "nonexistent session",
			sessionID: createTestSessionID(20),
			setupStore: func(s *SessionStore, c *conversation.Conversation) {
				// Don't save anything
			},
			wantErr: false,
			checkFunc: func(t *testing.T, conv *conversation.Conversation) {
				// Should return empty conversation
				if conv == nil {
					t.Error("Load() returned nil conversation")
					return
				}
				if len(conv.GetMessages()) != 0 {
					t.Errorf("expected empty conversation, got %d messages", len(conv.GetMessages()))
				}
				if conv.GetNextMessageID() != 1 {
					t.Errorf("expected next message ID 1, got %d", conv.GetNextMessageID())
				}
			},
		},
		{
			name:      "existing session",
			sessionID: createTestSessionID(21),
			setupStore: func(s *SessionStore, c *conversation.Conversation) {
				msgs := []conversation.ConversationMessage{
					{
						ID:      1,
						Role:    conversation.RoleUser,
						Content: "Test message",
					},
				}
				c.SetMessages(msgs)
				c.SetNextMessageID(2)
				c.SetCurrentPrompt("test prompt")
				if err := s.Save(createTestSessionID(21), c); err != nil {
					t.Fatalf("failed to save test conversation: %v", err)
				}
			},
			wantErr: false,
			checkFunc: func(t *testing.T, conv *conversation.Conversation) {
				if conv == nil {
					t.Error("Load() returned nil conversation")
					return
				}
				msgs := conv.GetMessages()
				if len(msgs) != 1 {
					t.Errorf("expected 1 message, got %d", len(msgs))
					return
				}
				if msgs[0].Content != "Test message" {
					t.Errorf("expected 'Test message', got %q", msgs[0].Content)
				}
				if conv.GetNextMessageID() != 2 {
					t.Errorf("expected next message ID 2, got %d", conv.GetNextMessageID())
				}
				if conv.GetCurrentPrompt() != "test prompt" {
					t.Errorf("expected 'test prompt', got %q", conv.GetCurrentPrompt())
				}
			},
		},
		{
			name:      "empty session ID",
			sessionID: "",
			setupStore: func(s *SessionStore, c *conversation.Conversation) {
				// Don't save anything
			},
			wantErr: true,
			checkFunc: func(t *testing.T, conv *conversation.Conversation) {
				// Should return error before returning conversation
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			store := NewSessionStore(tmpDir)

			// Setup
			if tt.setupStore != nil {
				setupConv := conversation.NewConversation()
				tt.setupStore(store, setupConv)
			}

			// Load
			conv, err := store.Load(tt.sessionID)

			// Check error
			if (err != nil) != tt.wantErr {
				t.Errorf("Load() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Run check function
			if tt.checkFunc != nil {
				tt.checkFunc(t, conv)
			}
		})
	}
}

func TestSessionStore_Load_CorruptJSON(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewSessionStore(tmpDir)

	// Create session directory with corrupt JSON
	sessionDir := filepath.Join(tmpDir, createTestSessionID(40))
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatalf("failed to create session directory: %v", err)
	}

	convPath := filepath.Join(sessionDir, "conversation.json")
	if err := os.WriteFile(convPath, []byte("not valid json"), 0644); err != nil {
		t.Fatalf("failed to write corrupt JSON: %v", err)
	}

	// Load should return empty conversation (not error)
	conv, err := store.Load(createTestSessionID(40))
	if err != nil {
		t.Errorf("Load() with corrupt JSON should not return error, got: %v", err)
	}
	if conv == nil {
		t.Error("Load() returned nil conversation")
		return
	}
	if len(conv.GetMessages()) != 0 {
		t.Errorf("expected empty conversation, got %d messages", len(conv.GetMessages()))
	}
}

func TestSessionStore_Exists(t *testing.T) {
	tests := []struct {
		name       string
		sessionID  string
		setupStore func(*SessionStore)
		want       bool
	}{
		{
			name:      "existing session",
			sessionID: createTestSessionID(21),
			setupStore: func(s *SessionStore) {
				conv := conversation.NewConversation()
				if err := s.Save(createTestSessionID(21), conv); err != nil {
					t.Fatalf("failed to save test conversation: %v", err)
				}
			},
			want: true,
		},
		{
			name:      "nonexistent session",
			sessionID: createTestSessionID(20),
			setupStore: func(s *SessionStore) {
				// Don't save anything
			},
			want: false,
		},
		{
			name:      "empty session ID",
			sessionID: "",
			setupStore: func(s *SessionStore) {
				// Don't save anything
			},
			want: false,
		},
		{
			name:      "session directory without conversation.json",
			sessionID: createTestSessionID(22),
			setupStore: func(s *SessionStore) {
				// Create directory but no conversation.json
				sessionDir := filepath.Join(s.basePath, "incomplete")
				if err := os.MkdirAll(sessionDir, 0755); err != nil {
					t.Fatalf("failed to create session directory: %v", err)
				}
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			store := NewSessionStore(tmpDir)

			// Setup
			if tt.setupStore != nil {
				tt.setupStore(store)
			}

			// Check existence
			got := store.Exists(tt.sessionID)
			if got != tt.want {
				t.Errorf("Exists() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSessionStore_ListSessions(t *testing.T) {
	tests := []struct {
		name       string
		setupStore func(*SessionStore)
		want       []string
		wantErr    bool
	}{
		{
			name: "no sessions",
			setupStore: func(s *SessionStore) {
				// Don't save anything
			},
			want:    []string{},
			wantErr: false,
		},
		{
			name: "single session",
			setupStore: func(s *SessionStore) {
				conv := conversation.NewConversation()
				if err := s.Save(createTestSessionID(41), conv); err != nil {
					t.Fatalf("failed to save test conversation: %v", err)
				}
			},
			want:    []string{createTestSessionID(41)},
			wantErr: false,
		},
		{
			name: "multiple sessions",
			setupStore: func(s *SessionStore) {
				conv := conversation.NewConversation()
				for _, id := range []string{createTestSessionID(41), createTestSessionID(45), createTestSessionID(46)} {
					if err := s.Save(id, conv); err != nil {
						t.Fatalf("failed to save test conversation %s: %v", id, err)
					}
				}
			},
			want:    []string{createTestSessionID(41), createTestSessionID(45), createTestSessionID(46)},
			wantErr: false,
		},
		{
			name: "ignore directories without conversation.json",
			setupStore: func(s *SessionStore) {
				conv := conversation.NewConversation()
				if err := s.Save(createTestSessionID(42), conv); err != nil {
					t.Fatalf("failed to save test conversation: %v", err)
				}

				// Create directory without conversation.json
				invalidDir := filepath.Join(s.basePath, "invalid")
				if err := os.MkdirAll(invalidDir, 0755); err != nil {
					t.Fatalf("failed to create invalid directory: %v", err)
				}
			},
			want:    []string{createTestSessionID(42)},
			wantErr: false,
		},
		{
			name: "base path doesn't exist",
			setupStore: func(s *SessionStore) {
				// Don't create base path
			},
			want:    []string{},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			store := NewSessionStore(filepath.Join(tmpDir, "sessions"))

			// Setup
			if tt.setupStore != nil {
				tt.setupStore(store)
			}

			// List sessions
			got, err := store.ListSessions()

			// Check error
			if (err != nil) != tt.wantErr {
				t.Errorf("ListSessions() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Check result length
			if len(got) != len(tt.want) {
				t.Errorf("ListSessions() returned %d sessions, want %d", len(got), len(tt.want))
				return
			}

			// Check all expected sessions are present (order doesn't matter)
			gotMap := make(map[string]bool)
			for _, id := range got {
				gotMap[id] = true
			}

			for _, wantID := range tt.want {
				if !gotMap[wantID] {
					t.Errorf("ListSessions() missing session %q", wantID)
				}
			}
		})
	}
}

func TestSessionStore_SaveLoad_RoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewSessionStore(tmpDir)

	// Create conversation with full state
	original := conversation.NewConversation()
	msgs := []conversation.ConversationMessage{
		{
			ID:      1,
			Role:    conversation.RoleUser,
			Content: "I want a cat picture",
		},
		{
			ID:      2,
			Role:    conversation.RoleAssistant,
			Content: "Here's a prompt for you",
			Snapshot: &conversation.StateSnapshot{
				Prompt:        "a fluffy cat",
				Steps:         20,
				CFG:           3.5,
				Seed:          -1,
				PreviewStatus: conversation.PreviewStatusComplete,
				PreviewURL:    "/sessions/1/images/2.png",
			},
		},
		{
			ID:      3,
			Role:    conversation.RoleUser,
			Content: "Make it orange",
		},
	}
	original.SetMessages(msgs)
	original.SetNextMessageID(4)
	original.SetCurrentPrompt("an orange fluffy cat")
	original.SetPreviousPrompt("a fluffy cat")
	original.SetPromptEdited(true)

	// Save
	if err := store.Save(createTestSessionID(43), original); err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	// Load
	loaded, err := store.Load(createTestSessionID(43))
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Compare all fields
	if loaded.GetNextMessageID() != original.GetNextMessageID() {
		t.Errorf("NextMessageID = %d, want %d", loaded.GetNextMessageID(), original.GetNextMessageID())
	}

	if loaded.GetCurrentPrompt() != original.GetCurrentPrompt() {
		t.Errorf("CurrentPrompt = %q, want %q", loaded.GetCurrentPrompt(), original.GetCurrentPrompt())
	}

	if loaded.GetPreviousPrompt() != original.GetPreviousPrompt() {
		t.Errorf("PreviousPrompt = %q, want %q", loaded.GetPreviousPrompt(), original.GetPreviousPrompt())
	}

	if loaded.IsPromptEdited() != original.IsPromptEdited() {
		t.Errorf("PromptEdited = %v, want %v", loaded.IsPromptEdited(), original.IsPromptEdited())
	}

	// Compare messages
	loadedMsgs := loaded.GetMessages()
	if len(loadedMsgs) != len(msgs) {
		t.Fatalf("got %d messages, want %d", len(loadedMsgs), len(msgs))
	}

	for i, want := range msgs {
		got := loadedMsgs[i]

		if got.ID != want.ID {
			t.Errorf("message[%d].ID = %d, want %d", i, got.ID, want.ID)
		}
		if got.Role != want.Role {
			t.Errorf("message[%d].Role = %q, want %q", i, got.Role, want.Role)
		}
		if got.Content != want.Content {
			t.Errorf("message[%d].Content = %q, want %q", i, got.Content, want.Content)
		}

		// Compare snapshots
		if (got.Snapshot == nil) != (want.Snapshot == nil) {
			t.Errorf("message[%d].Snapshot nil mismatch: got %v, want %v", i, got.Snapshot == nil, want.Snapshot == nil)
			continue
		}

		if want.Snapshot != nil {
			if got.Snapshot.Prompt != want.Snapshot.Prompt {
				t.Errorf("message[%d].Snapshot.Prompt = %q, want %q", i, got.Snapshot.Prompt, want.Snapshot.Prompt)
			}
			if got.Snapshot.Steps != want.Snapshot.Steps {
				t.Errorf("message[%d].Snapshot.Steps = %d, want %d", i, got.Snapshot.Steps, want.Snapshot.Steps)
			}
			if got.Snapshot.CFG != want.Snapshot.CFG {
				t.Errorf("message[%d].Snapshot.CFG = %f, want %f", i, got.Snapshot.CFG, want.Snapshot.CFG)
			}
			if got.Snapshot.Seed != want.Snapshot.Seed {
				t.Errorf("message[%d].Snapshot.Seed = %d, want %d", i, got.Snapshot.Seed, want.Snapshot.Seed)
			}
			if got.Snapshot.PreviewStatus != want.Snapshot.PreviewStatus {
				t.Errorf("message[%d].Snapshot.PreviewStatus = %q, want %q", i, got.Snapshot.PreviewStatus, want.Snapshot.PreviewStatus)
			}
			if got.Snapshot.PreviewURL != want.Snapshot.PreviewURL {
				t.Errorf("message[%d].Snapshot.PreviewURL = %q, want %q", i, got.Snapshot.PreviewURL, want.Snapshot.PreviewURL)
			}
		}
	}
}

func TestSessionStore_Save_AtomicWrite(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewSessionStore(tmpDir)

	// Save initial conversation
	conv1 := conversation.NewConversation()
	conv1.SetCurrentPrompt("first prompt")
	if err := store.Save(createTestSessionID(44), conv1); err != nil {
		t.Fatalf("first Save() failed: %v", err)
	}

	// Save again with different content
	conv2 := conversation.NewConversation()
	conv2.SetCurrentPrompt("second prompt")
	if err := store.Save(createTestSessionID(44), conv2); err != nil {
		t.Fatalf("second Save() failed: %v", err)
	}

	// Verify no .tmp file remains
	sessionDir := filepath.Join(tmpDir, createTestSessionID(44))
	tmpFiles, err := filepath.Glob(filepath.Join(sessionDir, "*.tmp"))
	if err != nil {
		t.Fatalf("failed to glob tmp files: %v", err)
	}
	if len(tmpFiles) > 0 {
		t.Errorf("found temp files after save: %v", tmpFiles)
	}

	// Verify final content is from second save
	loaded, err := store.Load(createTestSessionID(44))
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	if loaded.GetCurrentPrompt() != "second prompt" {
		t.Errorf("CurrentPrompt = %q, want 'second prompt'", loaded.GetCurrentPrompt())
	}
}
