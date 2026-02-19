package persistence

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
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

// imagePath builds the full file path for an image.
// suffix is "" for final images and "_preview" for preview images.
func (s *ImageStore) imagePath(sessionID string, suffix string, messageID int) string {
	return filepath.Join(s.basePath, sessionID, "images", fmt.Sprintf("%d%s.png", messageID, suffix))
}

// saveImage writes pngData to path using an atomic write (temp file + rename).
// Creates the parent directory if needed.
func (s *ImageStore) saveImage(path string, pngData []byte) error {
	// Create images directory structure (0700: owner-only access)
	imagesDir := filepath.Dir(path)
	if err := os.MkdirAll(imagesDir, 0700); err != nil {
		return fmt.Errorf("failed to create images directory: %w", err)
	}

	// Write to temp file first, then rename (atomic write)
	// 0600: owner read/write only
	tempPath := path + ".tmp"

	if err := os.WriteFile(tempPath, pngData, 0600); err != nil {
		return fmt.Errorf("failed to write image file: %w", err)
	}

	if err := os.Rename(tempPath, path); err != nil {
		// Clean up temp file if rename fails
		_ = os.Remove(tempPath)
		return fmt.Errorf("failed to commit image file: %w", err)
	}

	return nil
}

// loadImage reads and returns the contents of the file at path.
// Returns the raw os error so callers can check os.ErrNotExist.
func (s *ImageStore) loadImage(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// imageExists reports whether a file exists at path.
func (s *ImageStore) imageExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// deleteImage removes the file at path.
// Returns nil if the file did not exist; returns an error only if
// deletion fails for a file that exists.
func (s *ImageStore) deleteImage(path string) error {
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete image: %w", err)
	}
	return nil
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

	return s.saveImage(s.imagePath(sessionID, "", messageID), pngData)
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

	return s.loadImage(s.imagePath(sessionID, "", messageID))
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

	return s.imageExists(s.imagePath(sessionID, "", messageID))
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

	return s.deleteImage(s.imagePath(sessionID, "", messageID))
}

// GetURL returns the URL path for an image with a cache-busting query parameter.
// The path is relative and suitable for HTTP serving:
// /sessions/{sessionID}/images/{messageID}.png?v={unixNano}
//
// The ?v= parameter ensures the browser treats each generation as a unique
// resource, bypassing cached responses when an image is regenerated.
func (s *ImageStore) GetURL(sessionID string, messageID int) string {
	return fmt.Sprintf("/sessions/%s/images/%d.png?v=%d", sessionID, messageID, time.Now().UnixNano())
}

// SavePreview persists a preview image to disk.
// The image is written to:
// {basePath}/{sessionID}/images/{messageID}_preview.png
//
// The session and images directories are created if they don't exist.
// If the preview file exists, it is overwritten atomically.
func (s *ImageStore) SavePreview(sessionID string, messageID int, pngData []byte) error {
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

	return s.saveImage(s.imagePath(sessionID, "_preview", messageID), pngData)
}

// LoadPreview reads a preview image from disk and returns the PNG data.
// Returns os.ErrNotExist if the preview doesn't exist.
func (s *ImageStore) LoadPreview(sessionID string, messageID int) ([]byte, error) {
	if err := validateSessionID(sessionID); err != nil {
		return nil, fmt.Errorf("invalid session ID: %w", err)
	}
	if messageID <= 0 {
		return nil, fmt.Errorf("message ID must be positive")
	}

	return s.loadImage(s.imagePath(sessionID, "_preview", messageID))
}

// PreviewExists checks if a preview image exists on disk.
// Returns true if the preview file exists.
func (s *ImageStore) PreviewExists(sessionID string, messageID int) bool {
	if err := validateSessionID(sessionID); err != nil {
		return false
	}
	if messageID <= 0 {
		return false
	}

	return s.imageExists(s.imagePath(sessionID, "_preview", messageID))
}

// DeletePreview removes a preview image from disk.
// Returns nil if the preview was deleted or didn't exist.
// Returns an error only if deletion fails for a file that exists.
func (s *ImageStore) DeletePreview(sessionID string, messageID int) error {
	if err := validateSessionID(sessionID); err != nil {
		return fmt.Errorf("invalid session ID: %w", err)
	}
	if messageID <= 0 {
		return fmt.Errorf("message ID must be positive")
	}

	return s.deleteImage(s.imagePath(sessionID, "_preview", messageID))
}

// GetPreviewURL returns the URL path for a preview image.
// The path is relative and suitable for HTTP serving:
// /sessions/{sessionID}/images/{messageID}_preview.png
func (s *ImageStore) GetPreviewURL(sessionID string, messageID int) string {
	return fmt.Sprintf("/sessions/%s/images/%d_preview.png", sessionID, messageID)
}
