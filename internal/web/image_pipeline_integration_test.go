//go:build integration

package web

import (
	"bytes"
	"context"
	"image/png"
	"net/http/httptest"
	"testing"

	"github.com/hurricanerix/weave/internal/conversation"
	"github.com/hurricanerix/weave/internal/image"
	"github.com/hurricanerix/weave/internal/logging"
	"github.com/hurricanerix/weave/internal/ollama"
)

// TestImagePipeline_EndToEnd tests the complete image pipeline:
// generate -> encode -> store -> retrieve via HTTP -> verify PNG valid
func TestImagePipeline_EndToEnd(t *testing.T) {
	storage := image.NewStorage()
	sessionMgr := conversation.NewSessionManager()

	// Create server with mocked dependencies
	mockOllama := &mockOllamaClient{
		response: "test response",
		metadata: ollama.LLMMetadata{
			Prompt: "test prompt",
		},
	}
	server, err := NewServerWithDeps("", mockOllama, sessionMgr, storage, nil)
	if err != nil {
		t.Fatalf("NewServerWithDeps failed: %v", err)
	}

	// 1. Generate test pixel data and encode
	width, height := 64, 64 // Small for fast test
	pixels := make([]byte, width*height*3)
	for i := range pixels {
		pixels[i] = byte(i % 256)
	}

	pngData, err := image.EncodePNG(width, height, pixels, image.FormatRGB)
	if err != nil {
		t.Fatalf("EncodePNG failed: %v", err)
	}

	// 2. Store image
	imageID, err := storage.Store(pngData, width, height)
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	// 3. Retrieve via HTTP
	req := httptest.NewRequest("GET", "/images/"+imageID+".png", nil)
	w := httptest.NewRecorder()

	server.server.Handler.ServeHTTP(w, req)

	// 4. Verify response
	if w.Code != 200 {
		t.Errorf("got status %d, want 200", w.Code)
	}

	if ct := w.Header().Get("Content-Type"); ct != "image/png" {
		t.Errorf("got Content-Type %q, want %q", ct, "image/png")
	}

	// 5. Verify PNG is valid by decoding
	img, err := png.Decode(bytes.NewReader(w.Body.Bytes()))
	if err != nil {
		t.Fatalf("Failed to decode PNG: %v", err)
	}

	bounds := img.Bounds()
	if bounds.Dx() != width || bounds.Dy() != height {
		t.Errorf("decoded image dimensions %dx%d, want %dx%d", bounds.Dx(), bounds.Dy(), width, height)
	}
}

// TestImagePipeline_FullWorkflow tests the complete workflow from generation to serving
func TestImagePipeline_FullWorkflow(t *testing.T) {
	storage := image.NewStorage()
	sessionMgr := conversation.NewSessionManager()
	logger := logging.New(logging.LevelDebug, &bytes.Buffer{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start cleanup goroutine
	storage.StartCleanup(ctx, logger)

	// Create server
	mockOllama := &mockOllamaClient{
		response: "test response",
		metadata: ollama.LLMMetadata{
			Prompt: "A beautiful sunset",
		},
	}
	server, err := NewServerWithDeps("", mockOllama, sessionMgr, storage, nil)
	if err != nil {
		t.Fatalf("NewServerWithDeps failed: %v", err)
	}

	// Verify storage is initially empty
	if storage.Count() != 0 {
		t.Errorf("storage should be empty initially, got %d images", storage.Count())
	}

	// Generate some test images to verify storage works
	for i := 0; i < 5; i++ {
		pixels := make([]byte, 32*32*3)
		pngData, err := image.EncodePNG(32, 32, pixels, image.FormatRGB)
		if err != nil {
			t.Fatalf("EncodePNG failed: %v", err)
		}

		imageID, err := storage.Store(pngData, 32, 32)
		if err != nil {
			t.Fatalf("Store failed: %v", err)
		}

		// Verify we can retrieve it
		req := httptest.NewRequest("GET", "/images/"+imageID, nil)
		w := httptest.NewRecorder()
		server.server.Handler.ServeHTTP(w, req)

		if w.Code != 200 {
			t.Errorf("failed to retrieve image %d: got status %d", i, w.Code)
		}
	}

	// Verify all images are stored
	if storage.Count() != 5 {
		t.Errorf("expected 5 images, got %d", storage.Count())
	}
}

// Note: Cleanup and LRU tests are in the image package unit tests
// where we have access to internal methods. Integration tests focus
// on the full HTTP workflow.
