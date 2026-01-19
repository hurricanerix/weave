package persistence

import (
	"fmt"
	"regexp"
	"strings"
)

// validSessionIDPattern matches valid session IDs:
// - Lowercase hexadecimal characters only
// - Fixed length of 32 characters (matching session ID generation)
var validSessionIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

// validateSessionID validates that a session ID is safe to use in file paths.
// It checks for:
// - Empty string
// - Path traversal attempts (../, ..\, etc.)
// - Invalid characters (only hex digits allowed)
// - Incorrect length (must be 32 hex characters)
//
// This provides defense in depth at the persistence layer, even though
// session IDs should be validated upstream.
func validateSessionID(sessionID string) error {
	if sessionID == "" {
		return fmt.Errorf("session ID cannot be empty")
	}

	// Check for path traversal attempts
	if strings.Contains(sessionID, "..") {
		return fmt.Errorf("session ID contains path traversal sequence")
	}
	if strings.Contains(sessionID, "/") || strings.Contains(sessionID, "\\") {
		return fmt.Errorf("session ID contains path separator")
	}

	// Check format (32 lowercase hex characters)
	if !validSessionIDPattern.MatchString(sessionID) {
		return fmt.Errorf("session ID must be 32 lowercase hexadecimal characters")
	}

	return nil
}
