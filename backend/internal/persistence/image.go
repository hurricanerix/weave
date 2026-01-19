package persistence

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	// MaxImageSizeBytes is the maximum size allowed for an image file.
	// This prevents disk exhaustion from malicious or corrupted data.
	// 50MB is enough for very large high-resolution images.
	MaxImageSizeBytes = 50 * 1024 * 1024 // 50MB
)

// ImageStore manages persisting session images to disk.
// Images are stored per-session, keyed by message ID.
//
// Storage structure:
//
//	config/sessions/{session_id}/images/{message_id}.png
type ImageStore struct {
	basePath string // Base directory for all sessions (e.g., "config/sessions")
}

// NewImageStore creates a new image store rooted at the specified base path.
// The base path is typically "config/sessions".
//
// The directory structure will be created as needed when Save is called.
func NewImageStore(basePath string) *ImageStore {
	return &ImageStore{
		basePath: basePath,
	}
}

// Save persists an image to disk.
// The image is written to:
// {basePath}/{sessionID}/images/{messageID}.png
//
// The session and images directories are created if they don't exist.
// If the image file exists, it is overwritten atomically.
func (s *ImageStore) Save(sessionID string, messageID int, pngData []byte) error {
	if err := validateSessionID(sessionID); err != nil {
		return fmt.Errorf("invalid session ID: %w", err)
	}
	if messageID <= 0 {
		return fmt.Errorf("message ID must be positive")
	}
	if len(pngData) == 0 {
		return fmt.Errorf("PNG data cannot be empty")
	}

	// Check size limit to prevent disk exhaustion
	if len(pngData) > MaxImageSizeBytes {
		return fmt.Errorf("image size %d bytes exceeds maximum %d bytes", len(pngData), MaxImageSizeBytes)
	}

	// Create images directory structure (0700: owner-only access)
	imagesDir := filepath.Join(s.basePath, sessionID, "images")
	if err := os.MkdirAll(imagesDir, 0700); err != nil {
		return fmt.Errorf("failed to create images directory: %w", err)
	}

	// Write to temp file first, then rename (atomic write)
	// 0600: owner read/write only
	imagePath := filepath.Join(imagesDir, fmt.Sprintf("%d.png", messageID))
	tempPath := imagePath + ".tmp"

	if err := os.WriteFile(tempPath, pngData, 0600); err != nil {
		return fmt.Errorf("failed to write image file: %w", err)
	}

	if err := os.Rename(tempPath, imagePath); err != nil {
		// Clean up temp file if rename fails
		_ = os.Remove(tempPath)
		return fmt.Errorf("failed to commit image file: %w", err)
	}

	return nil
}

// Load reads an image from disk and returns the PNG data.
// Returns os.ErrNotExist if the image doesn't exist.
func (s *ImageStore) Load(sessionID string, messageID int) ([]byte, error) {
	if err := validateSessionID(sessionID); err != nil {
		return nil, fmt.Errorf("invalid session ID: %w", err)
	}
	if messageID <= 0 {
		return nil, fmt.Errorf("message ID must be positive")
	}

	imagePath := filepath.Join(s.basePath, sessionID, "images", fmt.Sprintf("%d.png", messageID))

	// Read the file
	data, err := os.ReadFile(imagePath)
	if err != nil {
		// Return the original error (which will be os.ErrNotExist if file doesn't exist)
		return nil, err
	}

	return data, nil
}

// Exists checks if an image exists on disk.
// Returns true if the image file exists.
func (s *ImageStore) Exists(sessionID string, messageID int) bool {
	if err := validateSessionID(sessionID); err != nil {
		return false
	}
	if messageID <= 0 {
		return false
	}

	imagePath := filepath.Join(s.basePath, sessionID, "images", fmt.Sprintf("%d.png", messageID))
	_, err := os.Stat(imagePath)
	return err == nil
}

// Delete removes an image from disk.
// Returns nil if the image was deleted or didn't exist.
// Returns an error only if deletion fails for a file that exists.
func (s *ImageStore) Delete(sessionID string, messageID int) error {
	if err := validateSessionID(sessionID); err != nil {
		return fmt.Errorf("invalid session ID: %w", err)
	}
	if messageID <= 0 {
		return fmt.Errorf("message ID must be positive")
	}

	imagePath := filepath.Join(s.basePath, sessionID, "images", fmt.Sprintf("%d.png", messageID))

	// Remove the file
	err := os.Remove(imagePath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete image: %w", err)
	}

	return nil
}

// GetURL returns the URL path for an image.
// The path is relative and suitable for HTTP serving:
// /sessions/{sessionID}/images/{messageID}.png
func (s *ImageStore) GetURL(sessionID string, messageID int) string {
	return fmt.Sprintf("/sessions/%s/images/%d.png", sessionID, messageID)
}
