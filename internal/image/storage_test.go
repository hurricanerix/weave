package image

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hurricanerix/weave/internal/logging"
)

func TestStorage_StoreAndRetrieve(t *testing.T) {
	storage := NewStorage()

	// Store an image
	pngData := []byte{1, 2, 3, 4}
	width, height := 100, 200

	id, err := storage.Store(pngData, width, height)
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	// Verify ID is valid UUID
	if _, err := uuid.Parse(id); err != nil {
		t.Errorf("Store returned invalid UUID: %v", err)
	}

	// Retrieve the image
	data, w, h, err := storage.Get(id)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	// Verify data matches
	if len(data) != len(pngData) {
		t.Errorf("got data length %d, want %d", len(data), len(pngData))
	}
	for i := range pngData {
		if data[i] != pngData[i] {
			t.Errorf("data[%d] = %d, want %d", i, data[i], pngData[i])
		}
	}

	// Verify dimensions
	if w != width {
		t.Errorf("got width %d, want %d", w, width)
	}
	if h != height {
		t.Errorf("got height %d, want %d", h, height)
	}
}

func TestStorage_GetNonExistent(t *testing.T) {
	storage := NewStorage()

	// Try to get non-existent image
	validID := uuid.New().String()
	_, _, _, err := storage.Get(validID)
	if err != ErrNotFound {
		t.Errorf("got error %v, want %v", err, ErrNotFound)
	}
}

func TestStorage_GetInvalidID(t *testing.T) {
	storage := NewStorage()

	tests := []struct {
		name string
		id   string
	}{
		{"empty string", ""},
		{"invalid format", "not-a-uuid"},
		{"partial uuid", "12345678"},
		{"malformed", "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, _, err := storage.Get(tt.id)
			if err != ErrInvalidID {
				t.Errorf("got error %v, want %v", err, ErrInvalidID)
			}
		})
	}
}

func TestStorage_ConcurrentAccess(t *testing.T) {
	storage := NewStorage()

	// Store initial image
	pngData := []byte{1, 2, 3, 4}
	id, err := storage.Store(pngData, 100, 100)
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	// Concurrent reads and writes
	const numGoroutines = 10
	var wg sync.WaitGroup
	wg.Add(numGoroutines * 2)

	// Start readers
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			_, _, _, err := storage.Get(id)
			if err != nil {
				t.Errorf("concurrent Get failed: %v", err)
			}
		}()
	}

	// Start writers
	for i := 0; i < numGoroutines; i++ {
		go func(n int) {
			defer wg.Done()
			data := []byte{byte(n), byte(n + 1), byte(n + 2)}
			_, err := storage.Store(data, 50, 50)
			if err != nil {
				t.Errorf("concurrent Store failed: %v", err)
			}
		}(i)
	}

	wg.Wait()

	// Verify original image still accessible
	data, _, _, err := storage.Get(id)
	if err != nil {
		t.Errorf("Get after concurrent access failed: %v", err)
	}
	if len(data) != len(pngData) {
		t.Errorf("data corrupted after concurrent access")
	}
}

func TestStorage_Count(t *testing.T) {
	storage := NewStorage()

	if count := storage.Count(); count != 0 {
		t.Errorf("new storage count = %d, want 0", count)
	}

	// Store some images
	for i := 0; i < 5; i++ {
		_, err := storage.Store([]byte{byte(i)}, 10, 10)
		if err != nil {
			t.Fatalf("Store failed: %v", err)
		}
	}

	if count := storage.Count(); count != 5 {
		t.Errorf("count = %d, want 5", count)
	}
}

func TestStorage_IDUniqueness(t *testing.T) {
	storage := NewStorage()

	const numImages = 100
	ids := make(map[string]bool)

	for i := 0; i < numImages; i++ {
		id, err := storage.Store([]byte{byte(i)}, 10, 10)
		if err != nil {
			t.Fatalf("Store failed: %v", err)
		}

		if ids[id] {
			t.Errorf("duplicate ID generated: %s", id)
		}
		ids[id] = true
	}

	if len(ids) != numImages {
		t.Errorf("got %d unique IDs, want %d", len(ids), numImages)
	}
}

func TestStorage_StoreEmptyData(t *testing.T) {
	storage := NewStorage()

	_, err := storage.Store([]byte{}, 100, 100)
	if err == nil {
		t.Error("Store with empty data should fail")
	}
}

func TestStorage_StoreInvalidDimensions(t *testing.T) {
	storage := NewStorage()

	tests := []struct {
		name   string
		width  int
		height int
	}{
		{"zero width", 0, 100},
		{"zero height", 100, 0},
		{"negative width", -10, 100},
		{"negative height", 100, -10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := storage.Store([]byte{1, 2, 3}, tt.width, tt.height)
			if err != ErrInvalidDimensions {
				t.Errorf("got error %v, want %v", err, ErrInvalidDimensions)
			}
		})
	}
}

func TestStorage_Delete(t *testing.T) {
	storage := NewStorage()

	// Store an image
	id, err := storage.Store([]byte{1, 2, 3}, 10, 10)
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	// Delete existing image
	deleted := storage.Delete(id)
	if !deleted {
		t.Error("Delete should return true for existing image")
	}

	// Verify it's gone
	_, _, _, err = storage.Get(id)
	if err != ErrNotFound {
		t.Errorf("got error %v, want %v", err, ErrNotFound)
	}

	// Delete non-existent image
	deleted = storage.Delete(id)
	if deleted {
		t.Error("Delete should return false for non-existent image")
	}
}

func TestStorage_CleanupAge(t *testing.T) {
	storage := NewStorage()
	logger := logging.New(logging.LevelDebug, &bytes.Buffer{})

	// Store some images with manipulated timestamps
	now := time.Now()
	oldTime := now.Add(-2 * time.Hour)    // Older than MaxAge (1 hour)
	newTime := now.Add(-30 * time.Minute) // Newer than MaxAge

	// Store old image
	id1, _ := storage.Store([]byte{1}, 10, 10)
	storage.mu.Lock()
	storage.images[id1].CreatedAt = oldTime
	storage.mu.Unlock()

	// Store new image
	id2, _ := storage.Store([]byte{2}, 10, 10)
	storage.mu.Lock()
	storage.images[id2].CreatedAt = newTime
	storage.mu.Unlock()

	if storage.Count() != 2 {
		t.Fatalf("expected 2 images before cleanup, got %d", storage.Count())
	}

	// Run cleanup
	storage.cleanup(logger)

	// Verify old image removed, new image remains
	if storage.Count() != 1 {
		t.Errorf("expected 1 image after cleanup, got %d", storage.Count())
	}

	_, _, _, err := storage.Get(id1)
	if err != ErrNotFound {
		t.Error("old image should be removed")
	}

	_, _, _, err = storage.Get(id2)
	if err != nil {
		t.Error("new image should remain")
	}
}

func TestStorage_CleanupLRU(t *testing.T) {
	storage := NewStorage()
	logger := logging.New(logging.LevelDebug, &bytes.Buffer{})

	// Store more than MaxImages
	ids := make([]string, MaxImages+10)
	for i := 0; i < MaxImages+10; i++ {
		id, err := storage.Store([]byte{byte(i)}, 10, 10)
		if err != nil {
			t.Fatalf("Store failed: %v", err)
		}
		ids[i] = id

		// Add small delay to ensure different access times
		time.Sleep(1 * time.Millisecond)
	}

	if storage.Count() != MaxImages+10 {
		t.Fatalf("expected %d images before cleanup, got %d", MaxImages+10, storage.Count())
	}

	// Access the last 10 images (make them most recently accessed)
	for i := MaxImages; i < MaxImages+10; i++ {
		_, _, _, _ = storage.Get(ids[i])
	}

	// Run cleanup
	storage.cleanup(logger)

	// Should have MaxImages remaining
	if storage.Count() != MaxImages {
		t.Errorf("expected %d images after cleanup, got %d", MaxImages, storage.Count())
	}

	// First 10 images (oldest accessed) should be removed
	for i := 0; i < 10; i++ {
		_, _, _, err := storage.Get(ids[i])
		if err != ErrNotFound {
			t.Errorf("old image %d should be removed", i)
		}
	}

	// Last 10 images (recently accessed) should remain
	for i := MaxImages; i < MaxImages+10; i++ {
		_, _, _, err := storage.Get(ids[i])
		if err != nil {
			t.Errorf("recent image %d should remain", i)
		}
	}
}

func TestStorage_StartCleanup(t *testing.T) {
	storage := NewStorage()
	logger := logging.New(logging.LevelDebug, &bytes.Buffer{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start cleanup goroutine
	storage.StartCleanup(ctx, logger)

	// Store an old image
	id, _ := storage.Store([]byte{1}, 10, 10)
	storage.mu.Lock()
	storage.images[id].CreatedAt = time.Now().Add(-2 * time.Hour)
	storage.mu.Unlock()

	if storage.Count() != 1 {
		t.Fatalf("expected 1 image before cleanup cycle")
	}

	// Wait slightly longer than CleanupInterval for cleanup to run
	// For testing, we'll manually trigger cleanup instead of waiting
	storage.cleanup(logger)

	if storage.Count() != 0 {
		t.Errorf("expected 0 images after cleanup, got %d", storage.Count())
	}

	// Cancel context to stop cleanup goroutine
	cancel()
	time.Sleep(10 * time.Millisecond) // Give goroutine time to stop
}

func TestStorage_CleanupContextCancellation(t *testing.T) {
	storage := NewStorage()
	logger := logging.New(logging.LevelDebug, &bytes.Buffer{})

	ctx, cancel := context.WithCancel(context.Background())

	// Start cleanup
	storage.StartCleanup(ctx, logger)

	// Cancel immediately
	cancel()

	// Give goroutine time to stop
	time.Sleep(10 * time.Millisecond)

	// Test passes if no panic or deadlock occurs
}

func TestStorage_MaxImageSize(t *testing.T) {
	storage := NewStorage()

	// Test oversized image rejected
	largeData := make([]byte, MaxImageSize+1)
	_, err := storage.Store(largeData, 100, 100)
	if !errors.Is(err, ErrImageTooLarge) {
		t.Errorf("expected ErrImageTooLarge, got %v", err)
	}

	// Test max size accepted
	maxData := make([]byte, MaxImageSize)
	id, err := storage.Store(maxData, 100, 100)
	if err != nil {
		t.Errorf("max size should be accepted: %v", err)
	}
	if id == "" {
		t.Error("expected valid ID")
	}
}
