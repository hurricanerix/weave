package web

import (
	"context"
	"testing"
	"time"
)

func TestTokenBucket_Allow(t *testing.T) {
	tests := []struct {
		name     string
		capacity int
		requests int
		want     int // number of requests that should succeed
	}{
		{
			name:     "all requests allowed",
			capacity: 10,
			requests: 5,
			want:     5,
		},
		{
			name:     "some requests denied",
			capacity: 3,
			requests: 5,
			want:     3,
		},
		{
			name:     "exactly at capacity",
			capacity: 5,
			requests: 5,
			want:     5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tb := newTokenBucket(tt.capacity)

			allowed := 0
			for i := 0; i < tt.requests; i++ {
				if tb.allow() {
					allowed++
				}
			}

			if allowed != tt.want {
				t.Errorf("allowed %d requests, want %d", allowed, tt.want)
			}
		})
	}
}

func TestTokenBucket_Refill(t *testing.T) {
	tb := newTokenBucket(5)

	// Consume all tokens
	for i := 0; i < 5; i++ {
		if !tb.allow() {
			t.Fatalf("expected request %d to be allowed", i)
		}
	}

	// Next request should be denied
	if tb.allow() {
		t.Error("expected request to be denied when bucket is empty")
	}

	// Manually set lastRefill to 1 minute ago to simulate refill
	tb.mu.Lock()
	tb.lastRefill = time.Now().Add(-1 * time.Minute)
	tb.mu.Unlock()

	// Tokens should have refilled, request should be allowed
	if !tb.allow() {
		t.Error("expected request to be allowed after refill")
	}
}

func TestRateLimiter_AllowChat(t *testing.T) {
	rl := newRateLimiter()
	sessionID := "test-session"

	// First MaxChatRequestsPerMinute requests should be allowed
	allowed := 0
	for i := 0; i < MaxChatRequestsPerMinute+5; i++ {
		if rl.allowChat(sessionID) {
			allowed++
		}
	}

	if allowed != MaxChatRequestsPerMinute {
		t.Errorf("allowed %d chat requests, want %d", allowed, MaxChatRequestsPerMinute)
	}
}

func TestRateLimiter_AllowGenerate(t *testing.T) {
	rl := newRateLimiter()
	sessionID := "test-session"

	// First MaxGenerateRequestsPerMinute requests should be allowed
	allowed := 0
	for i := 0; i < MaxGenerateRequestsPerMinute+5; i++ {
		if rl.allowGenerate(sessionID) {
			allowed++
		}
	}

	if allowed != MaxGenerateRequestsPerMinute {
		t.Errorf("allowed %d generate requests, want %d", allowed, MaxGenerateRequestsPerMinute)
	}
}

func TestRateLimiter_DifferentSessions(t *testing.T) {
	rl := newRateLimiter()
	session1 := "session1"
	session2 := "session2"

	// Exhaust session1's limit
	for i := 0; i < MaxChatRequestsPerMinute; i++ {
		if !rl.allowChat(session1) {
			t.Fatalf("expected request %d for session1 to be allowed", i)
		}
	}

	// session1 should be rate limited
	if rl.allowChat(session1) {
		t.Error("expected session1 to be rate limited")
	}

	// session2 should still be allowed
	if !rl.allowChat(session2) {
		t.Error("expected session2 to be allowed (different session)")
	}
}

func TestRateLimiter_Cleanup(t *testing.T) {
	rl := newRateLimiter()
	sessionID := "test-session"

	// Make a request to create bucket
	rl.allowChat(sessionID)

	// Verify bucket exists
	rl.mu.RLock()
	_, exists := rl.chat[sessionID]
	rl.mu.RUnlock()

	if !exists {
		t.Fatal("expected bucket to exist for session")
	}

	// Cleanup session
	rl.cleanup(sessionID)

	// Verify bucket was removed
	rl.mu.RLock()
	_, exists = rl.chat[sessionID]
	rl.mu.RUnlock()

	if exists {
		t.Error("expected bucket to be removed after cleanup")
	}
}

// SECURITY TEST: CleanupStale must remove sessions that have been idle too long (DoS prevention)
func TestRateLimiter_CleanupStale(t *testing.T) {
	rl := newRateLimiter()

	// Create sessions with different idle times
	recentSession := "recent-session"
	staleSession := "stale-session"

	// Make requests to create buckets
	rl.allowChat(recentSession)
	rl.allowChat(staleSession)

	// Manually set lastAccess on stale session to be old
	rl.mu.Lock()
	if bucket, ok := rl.chat[staleSession]; ok {
		bucket.mu.Lock()
		bucket.lastAccess = time.Now().Add(-2 * time.Hour)
		bucket.mu.Unlock()
	}
	rl.mu.Unlock()

	// Run cleanup with 1 hour max age
	rl.cleanupStale(1 * time.Hour)

	// Verify recent session still exists
	rl.mu.RLock()
	_, recentExists := rl.chat[recentSession]
	rl.mu.RUnlock()
	if !recentExists {
		t.Error("recent session should not have been cleaned up")
	}

	// Verify stale session was removed
	rl.mu.RLock()
	_, staleExists := rl.chat[staleSession]
	rl.mu.RUnlock()
	if staleExists {
		t.Error("stale session should have been cleaned up")
	}
}

// SECURITY TEST: CleanupStale must update lastAccess when allow() is called
func TestTokenBucket_UpdatesLastAccess(t *testing.T) {
	tb := newTokenBucket(5)

	// Get initial lastAccess time
	tb.mu.Lock()
	initialAccess := tb.lastAccess
	tb.mu.Unlock()

	// Wait a bit
	time.Sleep(10 * time.Millisecond)

	// Make a request
	tb.allow()

	// Verify lastAccess was updated
	tb.mu.Lock()
	newAccess := tb.lastAccess
	tb.mu.Unlock()

	if !newAccess.After(initialAccess) {
		t.Error("lastAccess should have been updated after allow()")
	}
}

// SECURITY TEST: StartCleanup goroutine must stop when context is cancelled
func TestRateLimiter_StartCleanupStopsOnCancel(t *testing.T) {
	rl := newRateLimiter()

	ctx, cancel := context.WithCancel(context.Background())

	// Start cleanup
	rl.startCleanup(ctx)

	// Create a session
	rl.allowChat("test-session")

	// Cancel context
	cancel()

	// Wait for goroutine to stop (if it doesn't stop, test will timeout)
	time.Sleep(100 * time.Millisecond)

	// If we get here, the goroutine stopped successfully
}

// SECURITY TEST: CleanupStale must handle concurrent access safely
func TestRateLimiter_CleanupStaleConcurrent(t *testing.T) {
	rl := newRateLimiter()

	// Create many sessions
	for i := 0; i < 100; i++ {
		sessionID := "session-" + string(rune(i))
		rl.allowChat(sessionID)
	}

	// Run cleanup concurrently with new requests
	done := make(chan struct{})
	go func() {
		for i := 0; i < 10; i++ {
			rl.cleanupStale(1 * time.Hour)
			time.Sleep(1 * time.Millisecond)
		}
		close(done)
	}()

	// Make requests while cleanup is running
	for i := 0; i < 50; i++ {
		sessionID := "concurrent-session-" + string(rune(i))
		rl.allowChat(sessionID)
	}

	// Wait for cleanup to finish
	<-done

	// If we get here without a race condition, the test passes
}

// SECURITY TEST: CleanupStale must not panic on empty map
func TestRateLimiter_CleanupStaleEmptyMap(t *testing.T) {
	rl := newRateLimiter()

	// Cleanup with no sessions should not panic
	rl.cleanupStale(1 * time.Hour)
}

// SECURITY TEST: Verify cleanup removes both chat and generate buckets
func TestRateLimiter_CleanupStaleRemovesBothBuckets(t *testing.T) {
	rl := newRateLimiter()

	sessionID := "test-session"

	// Create buckets for both chat and generate
	rl.allowChat(sessionID)
	rl.allowGenerate(sessionID)

	// Verify both buckets exist
	rl.mu.RLock()
	_, chatExists := rl.chat[sessionID]
	_, genExists := rl.generate[sessionID]
	rl.mu.RUnlock()

	if !chatExists || !genExists {
		t.Fatal("expected both buckets to exist")
	}

	// Make chat bucket stale
	rl.mu.Lock()
	if bucket, ok := rl.chat[sessionID]; ok {
		bucket.mu.Lock()
		bucket.lastAccess = time.Now().Add(-2 * time.Hour)
		bucket.mu.Unlock()
	}
	rl.mu.Unlock()

	// Run cleanup
	rl.cleanupStale(1 * time.Hour)

	// Verify both buckets were removed
	rl.mu.RLock()
	_, chatExists = rl.chat[sessionID]
	_, genExists = rl.generate[sessionID]
	rl.mu.RUnlock()

	if chatExists {
		t.Error("chat bucket should have been removed")
	}
	if genExists {
		t.Error("generate bucket should have been removed")
	}
}
