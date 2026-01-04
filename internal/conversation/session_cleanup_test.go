package conversation

import (
	"testing"
	"time"
)

func TestSessionManager_InactivityCleanup(t *testing.T) {
	sm := NewSessionManager()
	defer sm.Shutdown()

	// Create a session
	sessionID := "test-session"
	manager := sm.GetOrCreate(sessionID)
	if manager == nil {
		t.Fatal("expected manager to be created")
	}

	// Manually set last activity to past timeout
	sm.mu.Lock()
	if info, ok := sm.sessions[sessionID]; ok {
		info.lastActivity = time.Now().Add(-SessionInactivityTimeout - 1*time.Hour)
	}
	sm.mu.Unlock()

	// Run cleanup
	sm.cleanupInactiveSessions()

	// Session should be removed
	if sm.Get(sessionID) != nil {
		t.Error("expected inactive session to be cleaned up")
	}
}

func TestSessionManager_ActiveSessionNotCleanedUp(t *testing.T) {
	sm := NewSessionManager()
	defer sm.Shutdown()

	// Create a session
	sessionID := "test-session"
	manager := sm.GetOrCreate(sessionID)
	if manager == nil {
		t.Fatal("expected manager to be created")
	}

	// Run cleanup
	sm.cleanupInactiveSessions()

	// Active session should still exist
	if sm.Get(sessionID) == nil {
		t.Error("expected active session to not be cleaned up")
	}
}

func TestSessionManager_LRUEviction(t *testing.T) {
	sm := NewSessionManager()
	defer sm.Shutdown()

	// Create MaxSessions sessions
	for i := 0; i < MaxSessions; i++ {
		sessionID := string(rune('a' + i))
		sm.GetOrCreate(sessionID)
	}

	if sm.Count() != MaxSessions {
		t.Fatalf("expected %d sessions, got %d", MaxSessions, sm.Count())
	}

	// Manually set first session to be oldest
	firstSession := "a"
	sm.mu.Lock()
	if info, ok := sm.sessions[firstSession]; ok {
		info.lastActivity = time.Now().Add(-2 * time.Hour)
	}
	sm.mu.Unlock()

	// Create one more session (should trigger LRU eviction)
	sm.GetOrCreate("new-session")

	// Should still have MaxSessions (one was evicted)
	if sm.Count() != MaxSessions {
		t.Errorf("expected %d sessions after eviction, got %d", MaxSessions, sm.Count())
	}

	// Oldest session should be evicted
	if sm.Get(firstSession) != nil {
		t.Error("expected oldest session to be evicted")
	}

	// New session should exist
	if sm.Get("new-session") == nil {
		t.Error("expected new session to exist")
	}
}

func TestSessionManager_UpdateLastActivity(t *testing.T) {
	sm := NewSessionManager()
	defer sm.Shutdown()

	sessionID := "test-session"

	// Create session
	sm.GetOrCreate(sessionID)

	// Get initial last activity time
	sm.mu.RLock()
	initialTime := sm.sessions[sessionID].lastActivity
	sm.mu.RUnlock()

	// Wait a bit
	time.Sleep(10 * time.Millisecond)

	// Access session again
	sm.GetOrCreate(sessionID)

	// Last activity should be updated
	sm.mu.RLock()
	updatedTime := sm.sessions[sessionID].lastActivity
	sm.mu.RUnlock()

	if !updatedTime.After(initialTime) {
		t.Error("expected last activity time to be updated")
	}
}

func TestSessionManager_Shutdown(t *testing.T) {
	sm := NewSessionManager()

	// Create a session
	sm.GetOrCreate("test-session")

	// Shutdown should not hang
	done := make(chan struct{})
	go func() {
		sm.Shutdown()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(5 * time.Second):
		t.Fatal("Shutdown timed out")
	}
}
