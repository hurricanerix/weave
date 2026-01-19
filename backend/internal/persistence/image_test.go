package persistence

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// createTestSessionID creates a valid test session ID.
// All session IDs must be 32 lowercase hex characters.
func createTestSessionID(suffix int) string {
	// Create a valid 32-character hex session ID
	return fmt.Sprintf("00000000000000000000000000000%03d", suffix)
}

// createTestPNGData creates fake PNG data for testing
func createTestPNGData(size int) []byte {
	data := make([]byte, size)
	// Add PNG magic bytes for realism
	copy(data, []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A})
	return data
}

func TestImageStore_Save(t *testing.T) {
	tests := []struct {
		name      string
		sessionID string
		messageID int
		pngData   []byte
		wantErr   bool
	}{
		{
			name:      "valid image",
			sessionID: createTestSessionID(1),
			messageID: 1,
			pngData:   createTestPNGData(100),
			wantErr:   false,
		},
		{
			name:      "large image",
			sessionID: createTestSessionID(2),
			messageID: 2,
			pngData:   createTestPNGData(1024 * 1024), // 1MB
			wantErr:   false,
		},
		{
			name:      "empty session ID",
			sessionID: "",
			messageID: 1,
			pngData:   createTestPNGData(100),
			wantErr:   true,
		},
		{
			name:      "zero message ID",
			sessionID: createTestSessionID(3),
			messageID: 0,
			pngData:   createTestPNGData(100),
			wantErr:   true,
		},
		{
			name:      "negative message ID",
			sessionID: createTestSessionID(4),
			messageID: -1,
			pngData:   createTestPNGData(100),
			wantErr:   true,
		},
		{
			name:      "empty PNG data",
			sessionID: createTestSessionID(5),
			messageID: 1,
			pngData:   []byte{},
			wantErr:   true,
		},
		{
			name:      "nil PNG data",
			sessionID: createTestSessionID(6),
			messageID: 1,
			pngData:   nil,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory for test
			tmpDir := t.TempDir()
			store := NewImageStore(tmpDir)

			// Save
			err := store.Save(tt.sessionID, tt.messageID, tt.pngData)

			// Check error
			if (err != nil) != tt.wantErr {
				t.Errorf("Save() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// If save succeeded, verify file structure
			if err == nil && tt.sessionID != "" && tt.messageID > 0 {
				imagesDir := filepath.Join(tmpDir, tt.sessionID, "images")
				imagePath := filepath.Join(imagesDir, fmt.Sprintf("%d.png", tt.messageID))

				// Check images directory exists
				if _, err := os.Stat(imagesDir); os.IsNotExist(err) {
					t.Errorf("images directory not created")
				}

				// Check image file exists
				if _, err := os.Stat(imagePath); os.IsNotExist(err) {
					t.Errorf("image file not created")
				}

				// Verify file permissions (0600: owner read/write only)
				info, err := os.Stat(imagePath)
				if err != nil {
					t.Fatalf("failed to stat image file: %v", err)
				}
				if info.Mode().Perm() != 0600 {
					t.Errorf("image file permissions = %o, want 0600", info.Mode().Perm())
				}

				// Verify directory permissions (0700: owner-only access)
				dirInfo, err := os.Stat(imagesDir)
				if err != nil {
					t.Fatalf("failed to stat images directory: %v", err)
				}
				if dirInfo.Mode().Perm() != 0700 {
					t.Errorf("images directory permissions = %o, want 0700", dirInfo.Mode().Perm())
				}

				// Verify file size
				if info.Size() != int64(len(tt.pngData)) {
					t.Errorf("image file size = %d, want %d", info.Size(), len(tt.pngData))
				}
			}
		})
	}
}

func TestImageStore_Load(t *testing.T) {
	tests := []struct {
		name       string
		sessionID  string
		messageID  int
		setupStore func(*ImageStore)
		wantErr    bool
		checkFunc  func(*testing.T, []byte)
	}{
		{
			name:      "existing image",
			sessionID: createTestSessionID(1),
			messageID: 1,
			setupStore: func(s *ImageStore) {
				data := createTestPNGData(100)
				if err := s.Save(createTestSessionID(1), 1, data); err != nil {
					t.Fatalf("failed to save test image: %v", err)
				}
			},
			wantErr: false,
			checkFunc: func(t *testing.T, data []byte) {
				if len(data) != 100 {
					t.Errorf("expected 100 bytes, got %d", len(data))
				}
			},
		},
		{
			name:      "nonexistent image",
			sessionID: createTestSessionID(2),
			messageID: 1,
			setupStore: func(s *ImageStore) {
				// Don't save anything
			},
			wantErr: true,
			checkFunc: func(t *testing.T, data []byte) {
				// Should not be called
			},
		},
		{
			name:      "empty session ID",
			sessionID: "",
			messageID: 1,
			setupStore: func(s *ImageStore) {
				// Don't save anything
			},
			wantErr: true,
			checkFunc: func(t *testing.T, data []byte) {
				// Should not be called
			},
		},
		{
			name:      "zero message ID",
			sessionID: createTestSessionID(3),
			messageID: 0,
			setupStore: func(s *ImageStore) {
				// Don't save anything
			},
			wantErr: true,
			checkFunc: func(t *testing.T, data []byte) {
				// Should not be called
			},
		},
		{
			name:      "negative message ID",
			sessionID: createTestSessionID(4),
			messageID: -1,
			setupStore: func(s *ImageStore) {
				// Don't save anything
			},
			wantErr: true,
			checkFunc: func(t *testing.T, data []byte) {
				// Should not be called
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			store := NewImageStore(tmpDir)

			// Setup
			if tt.setupStore != nil {
				tt.setupStore(store)
			}

			// Load
			data, err := store.Load(tt.sessionID, tt.messageID)

			// Check error
			if (err != nil) != tt.wantErr {
				t.Errorf("Load() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Run check function
			if !tt.wantErr && tt.checkFunc != nil {
				tt.checkFunc(t, data)
			}
		})
	}
}

func TestImageStore_Load_ReturnsNotExist(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewImageStore(tmpDir)

	// Load nonexistent image
	_, err := store.Load(createTestSessionID(0), 1)
	if err == nil {
		t.Fatal("Load() should return error for nonexistent image")
	}

	// Verify error is os.ErrNotExist
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Load() error should be os.ErrNotExist, got: %v", err)
	}
}

func TestImageStore_Exists(t *testing.T) {
	tests := []struct {
		name       string
		sessionID  string
		messageID  int
		setupStore func(*ImageStore)
		want       bool
	}{
		{
			name:      "existing image",
			sessionID: createTestSessionID(1),
			messageID: 1,
			setupStore: func(s *ImageStore) {
				data := createTestPNGData(100)
				if err := s.Save(createTestSessionID(1), 1, data); err != nil {
					t.Fatalf("failed to save test image: %v", err)
				}
			},
			want: true,
		},
		{
			name:      "nonexistent image",
			sessionID: createTestSessionID(2),
			messageID: 1,
			setupStore: func(s *ImageStore) {
				// Don't save anything
			},
			want: false,
		},
		{
			name:      "empty session ID",
			sessionID: "",
			messageID: 1,
			setupStore: func(s *ImageStore) {
				// Don't save anything
			},
			want: false,
		},
		{
			name:      "zero message ID",
			sessionID: createTestSessionID(3),
			messageID: 0,
			setupStore: func(s *ImageStore) {
				// Don't save anything
			},
			want: false,
		},
		{
			name:      "negative message ID",
			sessionID: createTestSessionID(4),
			messageID: -1,
			setupStore: func(s *ImageStore) {
				// Don't save anything
			},
			want: false,
		},
		{
			name:      "different message ID",
			sessionID: createTestSessionID(5),
			messageID: 2,
			setupStore: func(s *ImageStore) {
				// Save with message ID 1
				data := createTestPNGData(100)
				if err := s.Save(createTestSessionID(5), 1, data); err != nil {
					t.Fatalf("failed to save test image: %v", err)
				}
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			store := NewImageStore(tmpDir)

			// Setup
			if tt.setupStore != nil {
				tt.setupStore(store)
			}

			// Check existence
			got := store.Exists(tt.sessionID, tt.messageID)
			if got != tt.want {
				t.Errorf("Exists() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestImageStore_Delete(t *testing.T) {
	tests := []struct {
		name       string
		sessionID  string
		messageID  int
		setupStore func(*ImageStore)
		wantErr    bool
	}{
		{
			name:      "delete existing image",
			sessionID: createTestSessionID(1),
			messageID: 1,
			setupStore: func(s *ImageStore) {
				data := createTestPNGData(100)
				if err := s.Save(createTestSessionID(1), 1, data); err != nil {
					t.Fatalf("failed to save test image: %v", err)
				}
			},
			wantErr: false,
		},
		{
			name:      "delete nonexistent image",
			sessionID: createTestSessionID(2),
			messageID: 1,
			setupStore: func(s *ImageStore) {
				// Don't save anything
			},
			wantErr: false, // Deleting nonexistent file is not an error
		},
		{
			name:      "empty session ID",
			sessionID: "",
			messageID: 1,
			setupStore: func(s *ImageStore) {
				// Don't save anything
			},
			wantErr: true,
		},
		{
			name:      "zero message ID",
			sessionID: createTestSessionID(3),
			messageID: 0,
			setupStore: func(s *ImageStore) {
				// Don't save anything
			},
			wantErr: true,
		},
		{
			name:      "negative message ID",
			sessionID: createTestSessionID(4),
			messageID: -1,
			setupStore: func(s *ImageStore) {
				// Don't save anything
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			store := NewImageStore(tmpDir)

			// Setup
			if tt.setupStore != nil {
				tt.setupStore(store)
			}

			// Delete
			err := store.Delete(tt.sessionID, tt.messageID)

			// Check error
			if (err != nil) != tt.wantErr {
				t.Errorf("Delete() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// If delete succeeded, verify file is gone
			if err == nil && tt.sessionID != "" && tt.messageID > 0 {
				if store.Exists(tt.sessionID, tt.messageID) {
					t.Errorf("image still exists after deletion")
				}
			}
		})
	}
}

func TestImageStore_GetURL(t *testing.T) {
	tests := []struct {
		name      string
		sessionID string
		messageID int
		want      string
	}{
		{
			name:      "basic URL",
			sessionID: createTestSessionID(10),
			messageID: 1,
			want:      "/sessions/00000000000000000000000000000010/images/1.png",
		},
		{
			name:      "different session",
			sessionID: createTestSessionID(11),
			messageID: 2,
			want:      "/sessions/00000000000000000000000000000011/images/2.png",
		},
		{
			name:      "large message ID",
			sessionID: createTestSessionID(12),
			messageID: 999,
			want:      "/sessions/00000000000000000000000000000012/images/999.png",
		},
		{
			name:      "valid 32-char hex session ID",
			sessionID: "550e8400e29b41d4a716446655440000",
			messageID: 1,
			want:      "/sessions/550e8400e29b41d4a716446655440000/images/1.png",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := NewImageStore("/any/path") // basePath doesn't affect URL

			got := store.GetURL(tt.sessionID, tt.messageID)
			if got != tt.want {
				t.Errorf("GetURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestImageStore_SaveLoad_RoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewImageStore(tmpDir)

	// Create test data
	original := createTestPNGData(1024)

	// Save
	if err := store.Save(createTestSessionID(30), 1, original); err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	// Load
	loaded, err := store.Load(createTestSessionID(30), 1)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Compare data
	if len(loaded) != len(original) {
		t.Fatalf("loaded data length = %d, want %d", len(loaded), len(original))
	}

	for i := range original {
		if loaded[i] != original[i] {
			t.Errorf("data mismatch at byte %d: got %d, want %d", i, loaded[i], original[i])
			break
		}
	}
}

func TestImageStore_Save_Overwrite(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewImageStore(tmpDir)

	// Save first image
	first := createTestPNGData(100)
	if err := store.Save(createTestSessionID(31), 1, first); err != nil {
		t.Fatalf("first Save() failed: %v", err)
	}

	// Verify first image
	loaded1, err := store.Load(createTestSessionID(31), 1)
	if err != nil {
		t.Fatalf("first Load() failed: %v", err)
	}
	if len(loaded1) != 100 {
		t.Errorf("first image size = %d, want 100", len(loaded1))
	}

	// Save second image with same ID (overwrite)
	second := createTestPNGData(200)
	if err := store.Save(createTestSessionID(31), 1, second); err != nil {
		t.Fatalf("second Save() failed: %v", err)
	}

	// Verify second image replaced first
	loaded2, err := store.Load(createTestSessionID(31), 1)
	if err != nil {
		t.Fatalf("second Load() failed: %v", err)
	}
	if len(loaded2) != 200 {
		t.Errorf("second image size = %d, want 200", len(loaded2))
	}

	// Verify no .tmp file remains
	imagesDir := filepath.Join(tmpDir, createTestSessionID(31), "images")
	tmpFiles, err := filepath.Glob(filepath.Join(imagesDir, "*.tmp"))
	if err != nil {
		t.Fatalf("failed to glob tmp files: %v", err)
	}
	if len(tmpFiles) > 0 {
		t.Errorf("found temp files after save: %v", tmpFiles)
	}
}

func TestImageStore_Save_AtomicWrite(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewImageStore(tmpDir)

	// Save image
	data := createTestPNGData(100)
	if err := store.Save(createTestSessionID(32), 1, data); err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	// Verify no .tmp file remains
	imagesDir := filepath.Join(tmpDir, createTestSessionID(32), "images")
	tmpFiles, err := filepath.Glob(filepath.Join(imagesDir, "*.tmp"))
	if err != nil {
		t.Fatalf("failed to glob tmp files: %v", err)
	}
	if len(tmpFiles) > 0 {
		t.Errorf("found temp files after save: %v", tmpFiles)
	}
}

func TestImageStore_MultipleImages(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewImageStore(tmpDir)

	// Save multiple images in same session
	sessionID := createTestSessionID(50)
	for i := 1; i <= 5; i++ {
		data := createTestPNGData(100 * i)
		if err := store.Save(sessionID, i, data); err != nil {
			t.Fatalf("Save(%d) failed: %v", i, err)
		}
	}

	// Verify all images exist
	for i := 1; i <= 5; i++ {
		if !store.Exists(sessionID, i) {
			t.Errorf("image %d should exist", i)
		}

		// Verify correct size
		data, err := store.Load(sessionID, i)
		if err != nil {
			t.Fatalf("Load(%d) failed: %v", i, err)
		}
		expectedSize := 100 * i
		if len(data) != expectedSize {
			t.Errorf("image %d size = %d, want %d", i, len(data), expectedSize)
		}
	}

	// Delete one image
	if err := store.Delete(sessionID, 3); err != nil {
		t.Fatalf("Delete(3) failed: %v", err)
	}

	// Verify deleted image is gone
	if store.Exists(sessionID, 3) {
		t.Errorf("image 3 should not exist after deletion")
	}

	// Verify other images still exist
	for _, i := range []int{1, 2, 4, 5} {
		if !store.Exists(sessionID, i) {
			t.Errorf("image %d should still exist", i)
		}
	}
}
