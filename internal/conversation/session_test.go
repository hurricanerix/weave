package conversation

import (
	"sync"
	"testing"
)

func TestNewSessionManager(t *testing.T) {
	sm := NewSessionManager()

	if sm == nil {
		t.Fatal("NewSessionManager() returned nil")
	}

	if sm.Count() != 0 {
		t.Errorf("New session manager should have 0 sessions, got %d", sm.Count())
	}
}

func TestGetOrCreateNewSession(t *testing.T) {
	sm := NewSessionManager()

	manager := sm.GetOrCreate("session-1")

	if manager == nil {
		t.Fatal("GetOrCreate returned nil")
	}

	if sm.Count() != 1 {
		t.Errorf("Expected 1 session, got %d", sm.Count())
	}
}

func TestGetOrCreateExistingSession(t *testing.T) {
	sm := NewSessionManager()

	first := sm.GetOrCreate("session-1")
	first.AddUserMessage("hello")

	second := sm.GetOrCreate("session-1")

	// Should be the same manager
	if first != second {
		t.Error("GetOrCreate should return same manager for same session ID")
	}

	// Message should persist
	history := second.GetHistory()
	if len(history) != 1 {
		t.Errorf("Expected 1 message, got %d", len(history))
	}
}

func TestGetNonExistentSession(t *testing.T) {
	sm := NewSessionManager()

	manager := sm.Get("unknown")

	if manager != nil {
		t.Error("Get should return nil for unknown session")
	}
}

func TestGetExistingSession(t *testing.T) {
	sm := NewSessionManager()

	created := sm.GetOrCreate("session-1")
	retrieved := sm.Get("session-1")

	if created != retrieved {
		t.Error("Get should return same manager as GetOrCreate")
	}
}

func TestDeleteSession(t *testing.T) {
	sm := NewSessionManager()

	sm.GetOrCreate("session-1")
	sm.GetOrCreate("session-2")

	if sm.Count() != 2 {
		t.Fatalf("Expected 2 sessions, got %d", sm.Count())
	}

	sm.Delete("session-1")

	if sm.Count() != 1 {
		t.Errorf("Expected 1 session after delete, got %d", sm.Count())
	}

	if sm.Get("session-1") != nil {
		t.Error("Deleted session should return nil on Get")
	}

	if sm.Get("session-2") == nil {
		t.Error("Other session should still exist")
	}
}

func TestDeleteNonExistentSession(t *testing.T) {
	sm := NewSessionManager()

	// Should not panic
	sm.Delete("unknown")

	if sm.Count() != 0 {
		t.Errorf("Count should be 0, got %d", sm.Count())
	}
}

func TestSessionIsolation(t *testing.T) {
	sm := NewSessionManager()

	session1 := sm.GetOrCreate("session-1")
	session2 := sm.GetOrCreate("session-2")

	// Modify session 1
	session1.AddUserMessage("hello from session 1")
	session1.AddAssistantMessage("response 1", "prompt 1")

	// Modify session 2 differently
	session2.AddUserMessage("hello from session 2")

	// Verify isolation
	history1 := session1.GetHistory()
	history2 := session2.GetHistory()

	if len(history1) != 2 {
		t.Errorf("Session 1 should have 2 messages, got %d", len(history1))
	}

	if len(history2) != 1 {
		t.Errorf("Session 2 should have 1 message, got %d", len(history2))
	}

	if session1.GetCurrentPrompt() != "prompt 1" {
		t.Errorf("Session 1 prompt = %q, want %q", session1.GetCurrentPrompt(), "prompt 1")
	}

	if session2.GetCurrentPrompt() != "" {
		t.Errorf("Session 2 prompt should be empty, got %q", session2.GetCurrentPrompt())
	}
}

func TestConcurrentGetOrCreate(t *testing.T) {
	sm := NewSessionManager()
	sessionID := "concurrent-session"

	const goroutines = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)

	managers := make(chan *Manager, goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			managers <- sm.GetOrCreate(sessionID)
		}()
	}

	wg.Wait()
	close(managers)

	// All goroutines should get the same manager
	var first *Manager
	for manager := range managers {
		if first == nil {
			first = manager
		} else if manager != first {
			t.Error("Concurrent GetOrCreate returned different managers for same ID")
		}
	}

	// Should only have one session
	if sm.Count() != 1 {
		t.Errorf("Expected 1 session, got %d", sm.Count())
	}
}

func TestConcurrentDifferentSessions(t *testing.T) {
	sm := NewSessionManager()

	const sessions = 100
	var wg sync.WaitGroup
	wg.Add(sessions)

	for i := 0; i < sessions; i++ {
		go func(id int) {
			defer wg.Done()
			sessionID := string(rune('a'+id%26)) + string(rune('0'+id/26))
			manager := sm.GetOrCreate(sessionID)
			manager.AddUserMessage("message")
		}(i)
	}

	wg.Wait()

	if sm.Count() != sessions {
		t.Errorf("Expected %d sessions, got %d", sessions, sm.Count())
	}
}

func TestConcurrentReadWrite(t *testing.T) {
	sm := NewSessionManager()

	// Pre-create some sessions
	for i := 0; i < 10; i++ {
		sm.GetOrCreate(string(rune('a' + i)))
	}

	const operations = 100
	var wg sync.WaitGroup
	wg.Add(operations * 3) // readers, writers, deleters

	// Concurrent reads
	for i := 0; i < operations; i++ {
		go func(id int) {
			defer wg.Done()
			_ = sm.Get(string(rune('a' + id%10)))
		}(i)
	}

	// Concurrent writes
	for i := 0; i < operations; i++ {
		go func(id int) {
			defer wg.Done()
			sm.GetOrCreate(string(rune('A' + id%26)))
		}(i)
	}

	// Concurrent deletes
	for i := 0; i < operations; i++ {
		go func(id int) {
			defer wg.Done()
			sm.Delete(string(rune('a' + id%10)))
		}(i)
	}

	wg.Wait()

	// Just verify no panics occurred and count is reasonable
	count := sm.Count()
	if count < 0 {
		t.Errorf("Invalid session count: %d", count)
	}
}
