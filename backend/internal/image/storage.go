package image

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/hurricanerix/weave/internal/logging"
)

const (
	// MaxImages is the maximum number of images to keep in storage
	MaxImages = 100
	// MaxAge is the maximum age of an image before cleanup
	MaxAge = 1 * time.Hour
	// CleanupInterval is how often cleanup runs
	CleanupInterval = 10 * time.Minute
	// MaxImageSize is the maximum size of a single image (10MB)
	MaxImageSize = 10 * 1024 * 1024
)

var (
	// ErrNotFound indicates the requested image does not exist
	ErrNotFound = errors.New("image not found")
	// ErrInvalidID indicates the provided image ID is invalid
	ErrInvalidID = errors.New("invalid image ID")
	// ErrImageTooLarge indicates the image exceeds the maximum allowed size
	ErrImageTooLarge = errors.New("image exceeds maximum size")
)

// storedImage holds image data with metadata
type storedImage struct {
	Data       []byte
	Width      int
	Height     int
	CreatedAt  time.Time
	AccessedAt time.Time
}

// Storage provides thread-safe in-memory image storage
type Storage struct {
	mu     sync.RWMutex
	images map[string]*storedImage
}

// NewStorage creates a new image storage
func NewStorage() *Storage {
	return &Storage{
		images: make(map[string]*storedImage),
	}
}

// Store saves PNG bytes and returns a unique ID
func (s *Storage) Store(pngData []byte, width, height int) (string, error) {
	if len(pngData) == 0 {
		return "", errors.New("empty PNG data")
	}

	if len(pngData) > MaxImageSize {
		return "", ErrImageTooLarge
	}

	if width <= 0 || height <= 0 {
		return "", ErrInvalidDimensions
	}

	// Generate unique ID
	id := uuid.New().String()

	now := time.Now()
	img := &storedImage{
		Data:       pngData,
		Width:      width,
		Height:     height,
		CreatedAt:  now,
		AccessedAt: now,
	}

	s.mu.Lock()
	s.images[id] = img
	s.mu.Unlock()

	return id, nil
}

// Get retrieves PNG bytes by ID, returns ErrNotFound if not exists
func (s *Storage) Get(id string) ([]byte, int, int, error) {
	// Validate ID format (no lock needed)
	if _, err := uuid.Parse(id); err != nil {
		return nil, 0, 0, ErrInvalidID
	}

	// Use read lock to check existence
	s.mu.RLock()
	img, exists := s.images[id]
	s.mu.RUnlock()

	if !exists {
		return nil, 0, 0, ErrNotFound
	}

	// Update access time with write lock (separate from read)
	s.mu.Lock()
	img.AccessedAt = time.Now()
	s.mu.Unlock()

	// Return copy of data to prevent external modification
	data := make([]byte, len(img.Data))
	copy(data, img.Data)
	return data, img.Width, img.Height, nil
}

// Count returns number of stored images
func (s *Storage) Count() int {
	s.mu.RLock()
	count := len(s.images)
	s.mu.RUnlock()
	return count
}

// Delete removes an image by ID. Returns true if image was deleted.
func (s *Storage) Delete(id string) bool {
	s.mu.Lock()
	_, exists := s.images[id]
	if exists {
		delete(s.images, id)
	}
	s.mu.Unlock()
	return exists
}

// StartCleanup starts a background goroutine that periodically removes
// old images (older than MaxAge) and enforces the MaxImages limit via LRU.
// The goroutine runs until ctx is cancelled. Caller MUST cancel ctx
// to stop cleanup and prevent goroutine leak.
func (s *Storage) StartCleanup(ctx context.Context, logger *logging.Logger) {
	ticker := time.NewTicker(CleanupInterval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				logger.Debug("Image cleanup goroutine stopping")
				return
			case <-ticker.C:
				s.cleanup(logger)
			}
		}
	}()
}

// cleanup removes images older than MaxAge and enforces MaxImages limit
func (s *Storage) cleanup(logger *logging.Logger) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	initialCount := len(s.images)

	// Remove images older than MaxAge
	ageDeleted := 0
	for id, img := range s.images {
		if now.Sub(img.CreatedAt) > MaxAge {
			delete(s.images, id)
			ageDeleted++
		}
	}

	if ageDeleted > 0 {
		logger.Debug("Removed %d images older than %v", ageDeleted, MaxAge)
	}

	// Enforce MaxImages limit using LRU eviction
	if len(s.images) > MaxImages {
		// Build sorted list by AccessedAt (oldest first)
		type imageEntry struct {
			id         string
			accessedAt time.Time
		}

		entries := make([]imageEntry, 0, len(s.images))
		for id, img := range s.images {
			entries = append(entries, imageEntry{
				id:         id,
				accessedAt: img.AccessedAt,
			})
		}

		sort.Slice(entries, func(i, j int) bool {
			return entries[i].accessedAt.Before(entries[j].accessedAt)
		})

		// Remove oldest images until we're at MaxImages
		lruDeleted := 0
		toDelete := len(entries) - MaxImages
		if toDelete > 0 {
			for i := 0; i < toDelete; i++ {
				delete(s.images, entries[i].id)
				lruDeleted++
			}
		}

		if lruDeleted > 0 {
			logger.Debug("LRU eviction removed %d images (limit: %d)", lruDeleted, MaxImages)
		}
	}

	finalCount := len(s.images)
	if initialCount != finalCount {
		logger.Debug("Cleanup complete: %d -> %d images", initialCount, finalCount)
	}
}
