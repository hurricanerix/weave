package web

import (
	"context"
	"sync"
	"time"
)

const (
	// MaxChatRequestsPerMinute limits chat messages per session.
	MaxChatRequestsPerMinute = 10

	// MaxGenerateRequestsPerMinute limits generate requests per session.
	MaxGenerateRequestsPerMinute = 5

	// cleanupInterval is how often to check for stale sessions
	cleanupInterval = 5 * time.Minute

	// maxSessionAge is the maximum idle time before a session is cleaned up
	maxSessionAge = 30 * time.Minute
)

// tokenBucket implements a simple token bucket rate limiter.
type tokenBucket struct {
	capacity   int
	tokens     int
	lastRefill time.Time
	lastAccess time.Time
	mu         sync.Mutex
}

// newTokenBucket creates a new token bucket with the specified capacity.
// Tokens refill at a rate of capacity per minute.
func newTokenBucket(capacity int) *tokenBucket {
	now := time.Now()
	return &tokenBucket{
		capacity:   capacity,
		tokens:     capacity,
		lastRefill: now,
		lastAccess: now,
	}
}

// allow checks if a request can proceed and consumes a token if so.
// Returns true if the request is allowed, false if rate limited.
func (tb *tokenBucket) allow() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	// Refill tokens based on time elapsed
	now := time.Now()
	elapsed := now.Sub(tb.lastRefill)

	// Refill at rate of capacity per minute
	tokensToAdd := int(elapsed.Minutes() * float64(tb.capacity))
	if tokensToAdd > 0 {
		tb.tokens += tokensToAdd
		if tb.tokens > tb.capacity {
			tb.tokens = tb.capacity
		}
		tb.lastRefill = now
	}

	// Update last access time
	tb.lastAccess = now

	// Check if we have tokens available
	if tb.tokens > 0 {
		tb.tokens--
		return true
	}

	return false
}

// rateLimiter tracks rate limits per session.
type rateLimiter struct {
	mu       sync.RWMutex
	chat     map[string]*tokenBucket
	generate map[string]*tokenBucket
}

// newRateLimiter creates a new rate limiter.
func newRateLimiter() *rateLimiter {
	return &rateLimiter{
		chat:     make(map[string]*tokenBucket),
		generate: make(map[string]*tokenBucket),
	}
}

// allowChat checks if a chat request is allowed for the given session.
func (rl *rateLimiter) allowChat(sessionID string) bool {
	rl.mu.Lock()
	bucket, ok := rl.chat[sessionID]
	if !ok {
		bucket = newTokenBucket(MaxChatRequestsPerMinute)
		rl.chat[sessionID] = bucket
	}
	rl.mu.Unlock()

	return bucket.allow()
}

// allowGenerate checks if a generate request is allowed for the given session.
func (rl *rateLimiter) allowGenerate(sessionID string) bool {
	rl.mu.Lock()
	bucket, ok := rl.generate[sessionID]
	if !ok {
		bucket = newTokenBucket(MaxGenerateRequestsPerMinute)
		rl.generate[sessionID] = bucket
	}
	rl.mu.Unlock()

	return bucket.allow()
}

// cleanup removes rate limit state for a deleted session.
func (rl *rateLimiter) cleanup(sessionID string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.chat, sessionID)
	delete(rl.generate, sessionID)
}

// cleanupStale removes rate limit state for sessions that have been idle for too long.
// SECURITY: Prevents unbounded growth of rate limiter maps (DoS prevention).
func (rl *rateLimiter) cleanupStale(maxAge time.Duration) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	for sessionID, bucket := range rl.chat {
		bucket.mu.Lock()
		stale := now.Sub(bucket.lastAccess) > maxAge
		bucket.mu.Unlock()
		if stale {
			delete(rl.chat, sessionID)
			delete(rl.generate, sessionID)
		}
	}
}

// startCleanup starts a background goroutine that periodically cleans up stale sessions.
// The goroutine will stop when the context is cancelled.
// SECURITY: This is required to prevent unbounded growth of the rate limiter maps.
func (rl *rateLimiter) startCleanup(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(cleanupInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				rl.cleanupStale(maxSessionAge)
			case <-ctx.Done():
				return
			}
		}
	}()
}
