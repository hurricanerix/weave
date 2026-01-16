package conversation

import (
	"context"
	"log"
	"sync"
	"time"
)

const (
	// SessionInactivityTimeout is how long a session can be inactive before cleanup.
	SessionInactivityTimeout = 24 * time.Hour

	// SessionCleanupInterval is how often to run cleanup.
	SessionCleanupInterval = 1 * time.Hour

	// MaxSessions is the maximum number of sessions before LRU eviction.
	MaxSessions = 1000
)

// Session tracks a session, its conversation manager, generation settings,
// and last activity time.
//
// Session is thread-safe. All access to the manager and settings is protected
// by a mutex. This allows multiple HTTP requests for the same session to be
// handled concurrently without data races.
type Session struct {
	mu           sync.Mutex // protects all fields below
	manager      *Manager
	lastActivity time.Time
	// settings stores the current generation settings for this session.
	// nil means settings have not been set yet (use server defaults).
	settings *GenerationSettings
}

// SessionManager provides thread-safe management of conversation sessions.
// Each session is identified by a unique session ID and contains its own
// conversation state.
//
// SessionManager is safe for concurrent access from multiple goroutines.
// It uses a read-write mutex to allow concurrent reads while serializing
// writes.
//
// Sessions are automatically cleaned up after 24 hours of inactivity.
// A background goroutine runs every hour to remove stale sessions.
// If the session count exceeds MaxSessions, the least recently used session
// is evicted.
type SessionManager struct {
	mu            sync.RWMutex
	sessions      map[string]*Session
	cancelCleanup context.CancelFunc
	cleanupDone   chan struct{}
}

// NewSessionManager creates a new session manager with an empty session map.
// It starts a background goroutine that periodically cleans up inactive sessions.
func NewSessionManager() *SessionManager {
	ctx, cancel := context.WithCancel(context.Background())

	sm := &SessionManager{
		sessions:      make(map[string]*Session),
		cancelCleanup: cancel,
		cleanupDone:   make(chan struct{}),
	}

	// Start background cleanup goroutine
	go sm.cleanupLoop(ctx)

	return sm
}

// GetSession returns the Session for the given session ID.
// If the session does not exist, a new Session is created and stored.
// Updates the last activity time for the session.
//
// This method is thread-safe and can be called concurrently from multiple
// goroutines.
func (sm *SessionManager) GetSession(sessionID string) *Session {
	now := time.Now()

	// Try read lock first for existing sessions (fast path)
	sm.mu.RLock()
	if session, ok := sm.sessions[sessionID]; ok {
		sm.mu.RUnlock()
		// Update last activity time
		sm.mu.Lock()
		session.lastActivity = now
		sm.mu.Unlock()
		return session
	}
	sm.mu.RUnlock()

	// Session doesn't exist, need write lock to create
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Double-check after acquiring write lock (another goroutine may have
	// created the session while we were waiting for the lock)
	if session, ok := sm.sessions[sessionID]; ok {
		session.lastActivity = now
		return session
	}

	// Check if we need to evict LRU session
	if len(sm.sessions) >= MaxSessions {
		sm.evictLRU()
	}

	// Create new session
	manager := NewManager()
	session := &Session{
		manager:      manager,
		lastActivity: now,
	}
	sm.sessions[sessionID] = session
	return session
}

// GetOrCreate returns the Manager for the given session ID.
// If the session does not exist, a new Manager is created and stored.
// Updates the last activity time for the session.
//
// This method is thread-safe and can be called concurrently from multiple
// goroutines.
//
// Deprecated: Use GetSession() instead to access both Manager and settings.
func (sm *SessionManager) GetOrCreate(sessionID string) *Manager {
	return sm.GetSession(sessionID).manager
}

// Get returns the Manager for the given session ID, or nil if it doesn't exist.
// This method does not create a new session.
//
// This method is thread-safe.
func (sm *SessionManager) Get(sessionID string) *Manager {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	if info, ok := sm.sessions[sessionID]; ok {
		return info.manager
	}
	return nil
}

// Delete removes the session with the given ID.
// If the session doesn't exist, this is a no-op.
//
// This method is thread-safe.
func (sm *SessionManager) Delete(sessionID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	delete(sm.sessions, sessionID)
}

// Count returns the number of active sessions.
//
// This method is thread-safe.
func (sm *SessionManager) Count() int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return len(sm.sessions)
}

// Shutdown stops the cleanup goroutine and waits for it to finish.
func (sm *SessionManager) Shutdown() {
	if sm.cancelCleanup != nil {
		sm.cancelCleanup()
		<-sm.cleanupDone
	}
}

// cleanupLoop runs periodically to remove inactive sessions.
func (sm *SessionManager) cleanupLoop(ctx context.Context) {
	defer close(sm.cleanupDone)

	ticker := time.NewTicker(SessionCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sm.cleanupInactiveSessions()
		}
	}
}

// cleanupInactiveSessions removes sessions that have been inactive for too long.
func (sm *SessionManager) cleanupInactiveSessions() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	now := time.Now()
	removed := 0

	for sessionID, info := range sm.sessions {
		if now.Sub(info.lastActivity) > SessionInactivityTimeout {
			delete(sm.sessions, sessionID)
			removed++
		}
	}

	if removed > 0 {
		log.Printf("Cleaned up %d inactive sessions (total: %d)", removed, len(sm.sessions))
	}
}

// Manager returns the conversation manager for this session.
// The returned Manager is thread-safe and can be used concurrently.
func (s *Session) Manager() *Manager {
	// No lock needed - manager itself is thread-safe and the pointer is immutable.
	return s.manager
}

// SetGenerationSettings updates the generation settings for this session.
// This stores the current values so they can be retrieved later.
func (s *Session) SetGenerationSettings(steps int, cfg float64, seed int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.settings = &GenerationSettings{
		Steps: steps,
		CFG:   cfg,
		Seed:  seed,
	}
}

// GetGenerationSettings retrieves the current generation settings for this session.
// If settings have not been set yet, hasSettings will be false and the caller
// should use server defaults. The returned values are only valid when hasSettings is true.
func (s *Session) GetGenerationSettings() (steps int, cfg float64, seed int64, hasSettings bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.settings == nil {
		return 0, 0, 0, false
	}
	return s.settings.Steps, s.settings.CFG, s.settings.Seed, true
}

// evictLRU removes the least recently used session.
// Must be called with sm.mu held for writing.
func (sm *SessionManager) evictLRU() {
	var oldestID string
	var oldestTime time.Time

	// Find the least recently used session
	for sessionID, info := range sm.sessions {
		if oldestID == "" || info.lastActivity.Before(oldestTime) {
			oldestID = sessionID
			oldestTime = info.lastActivity
		}
	}

	if oldestID != "" {
		delete(sm.sessions, oldestID)
		log.Printf("Evicted LRU session %s (was inactive for %v)", oldestID, time.Since(oldestTime))
	}
}
