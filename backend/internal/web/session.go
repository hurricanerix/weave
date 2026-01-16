package web

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"
)

const (
	// SessionCookieName is the name of the session cookie.
	SessionCookieName = "weave_session"

	// SessionIDLength is the length of the session ID in bytes.
	// 16 bytes = 128 bits of entropy.
	SessionIDLength = 16

	// SessionExpiry is how long a session cookie lasts.
	SessionExpiry = 24 * time.Hour
)

// contextKey is an unexported type for context keys to avoid collisions.
type contextKey int

const (
	sessionIDKey contextKey = iota
)

// GenerateSessionID creates a new cryptographically secure session ID.
// Returns a hex-encoded string of random bytes.
func GenerateSessionID() (string, error) {
	bytes := make([]byte, SessionIDLength)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate session ID: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}

// GetSessionID retrieves the session ID from the request context.
// Returns an empty string if no session ID exists in the context.
func GetSessionID(ctx context.Context) string {
	if sessionID, ok := ctx.Value(sessionIDKey).(string); ok {
		return sessionID
	}
	return ""
}

// setSessionID stores the session ID in the context.
func setSessionID(ctx context.Context, sessionID string) context.Context {
	return context.WithValue(ctx, sessionIDKey, sessionID)
}

// ValidateSessionID validates that a session ID is properly formatted.
// Session IDs must be hex-encoded strings of SessionIDLength*2 characters.
// Returns true if valid, false otherwise.
func ValidateSessionID(sessionID string) bool {
	// Check length (hex encoding doubles the byte length)
	expectedLen := SessionIDLength * 2
	if len(sessionID) != expectedLen {
		return false
	}

	// Check if it's valid hex encoding
	_, err := hex.DecodeString(sessionID)
	return err == nil
}

// SessionMiddleware ensures every request has a session ID.
// If the request has a valid session cookie, it uses that ID.
// Otherwise, it generates a new ID and sets a cookie.
// The session ID is stored in the request context for handlers to access.
func SessionMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var sessionID string

		// Try to get session ID from cookie
		cookie, err := r.Cookie(SessionCookieName)
		if err == nil && cookie.Value != "" {
			// Validate session ID format
			if ValidateSessionID(cookie.Value) {
				sessionID = cookie.Value
			}
			// If invalid, sessionID remains empty and we'll generate a new one
		}

		// Generate new session ID if none exists or validation failed
		if sessionID == "" {
			var err error
			sessionID, err = GenerateSessionID()
			if err != nil {
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}

			// Set session cookie
			// SECURITY: Secure flag requires HTTPS in production
			http.SetCookie(w, &http.Cookie{
				Name:     SessionCookieName,
				Value:    sessionID,
				Path:     "/",
				MaxAge:   int(SessionExpiry.Seconds()),
				HttpOnly: true,
				SameSite: http.SameSiteStrictMode,
				Secure:   true,
			})
		}

		// Store session ID in context
		ctx := setSessionID(r.Context(), sessionID)

		// Call next handler with updated context
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
