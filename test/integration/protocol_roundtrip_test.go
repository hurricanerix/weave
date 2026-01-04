//go:build integration

package integration

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/hurricanerix/weave/internal/protocol"
)

// stubGeneratorPath returns the path to the C stub generator executable.
// It locates the project root by searching upward from the test file location.
func stubGeneratorPath(t *testing.T) string {
	t.Helper()

	// Get the directory containing this test file
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to get current file path")
	}
	testDir := filepath.Dir(filename)

	// Navigate up to project root (test/integration -> test -> root)
	projectRoot := filepath.Join(testDir, "..", "..")

	// Resolve to absolute path
	absRoot, err := filepath.Abs(projectRoot)
	if err != nil {
		t.Fatalf("failed to resolve project root: %v", err)
	}

	return filepath.Join(absRoot, "compute-daemon", "test", "test_stub_generator")
}

// runStubGenerator executes the C stub generator with the given request bytes.
// It returns the response bytes or an error.
func runStubGenerator(t *testing.T, requestBytes []byte) ([]byte, error) {
	t.Helper()

	stubPath := stubGeneratorPath(t)

	// Verify stub generator exists
	if _, err := os.Stat(stubPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("stub generator not found at %s (run 'make test-stub' in compute-daemon/)", stubPath)
	}

	cmd := exec.Command(stubPath)

	// Set up pipes
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Start the process
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start stub generator: %w", err)
	}

	// Write request to stdin
	if _, err := stdin.Write(requestBytes); err != nil {
		stdin.Close()
		cmd.Wait()
		return nil, fmt.Errorf("failed to write request: %w", err)
	}

	// Close stdin to signal EOF
	if err := stdin.Close(); err != nil {
		return nil, fmt.Errorf("failed to close stdin: %w", err)
	}

	// Wait for process to complete
	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("stub generator failed: %w\nstderr: %s", err, stderr.String())
	}

	return stdout.Bytes(), nil
}

// verifyCheckerboard verifies that the image data contains a valid checkerboard pattern.
// Checkerboard has 8x8 pixel blocks alternating between 0x00 (black) and 0xFF (white).
func verifyCheckerboard(t *testing.T, width, height, channels uint32, data []byte) {
	t.Helper()

	const blockSize = 8

	expectedLen := width * height * channels
	if uint32(len(data)) != expectedLen {
		t.Fatalf("image data length mismatch: got %d, expected %d", len(data), expectedLen)
	}

	// Check a sampling of pixels (not every pixel, too slow for large images)
	// Check corners of each 8x8 block
	for y := uint32(0); y < height; y += blockSize {
		for x := uint32(0); x < width; x += blockSize {
			blockX := x / blockSize
			blockY := y / blockSize
			expectedValue := uint8(0x00)
			if (blockX+blockY)%2 == 1 {
				expectedValue = 0xFF
			}

			// Check top-left pixel of this block
			pixelOffset := (y*width + x) * channels
			for c := uint32(0); c < channels; c++ {
				actualValue := data[pixelOffset+c]
				if actualValue != expectedValue {
					t.Errorf("checkerboard mismatch at pixel (%d,%d) channel %d: got 0x%02X, expected 0x%02X",
						x, y, c, actualValue, expectedValue)
					return // Fail fast on first mismatch
				}
			}
		}
	}
}

// TestProtocolRoundTrip_64x64_RGB tests round-trip with minimum dimensions.
func TestProtocolRoundTrip_64x64_RGB(t *testing.T) {
	const (
		width  = 64
		height = 64
	)

	// Create request
	req, err := protocol.NewSD35GenerateRequest(
		12345, // request_id
		"test prompt",
		width,
		height,
		28,   // steps
		7.0,  // cfg_scale
		9999, // seed
	)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	// Encode request
	requestBytes, err := protocol.EncodeSD35GenerateRequest(req)
	if err != nil {
		t.Fatalf("failed to encode request: %v", err)
	}

	// Run stub generator
	responseBytes, err := runStubGenerator(t, requestBytes)
	if err != nil {
		t.Fatalf("stub generator failed: %v", err)
	}

	// Decode response
	resp, err := protocol.DecodeResponse(responseBytes)
	if err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Type assertion to SD35GenerateResponse
	sd35Resp, ok := resp.(*protocol.SD35GenerateResponse)
	if !ok {
		t.Fatalf("expected *SD35GenerateResponse, got %T", resp)
	}

	// Verify response metadata
	if sd35Resp.RequestID != req.RequestID {
		t.Errorf("request_id mismatch: got %d, expected %d", sd35Resp.RequestID, req.RequestID)
	}
	if sd35Resp.Status != protocol.StatusOK {
		t.Errorf("status mismatch: got %d, expected %d", sd35Resp.Status, protocol.StatusOK)
	}
	if sd35Resp.ImageWidth != width {
		t.Errorf("image width mismatch: got %d, expected %d", sd35Resp.ImageWidth, width)
	}
	if sd35Resp.ImageHeight != height {
		t.Errorf("image height mismatch: got %d, expected %d", sd35Resp.ImageHeight, height)
	}
	if sd35Resp.Channels != protocol.SD35ChannelsRGB {
		t.Errorf("channels mismatch: got %d, expected %d (RGB)", sd35Resp.Channels, protocol.SD35ChannelsRGB)
	}

	// Verify image data
	verifyCheckerboard(t, sd35Resp.ImageWidth, sd35Resp.ImageHeight, sd35Resp.Channels, sd35Resp.ImageData)
}

// TestProtocolRoundTrip_512x512_RGB tests round-trip with typical dimensions.
func TestProtocolRoundTrip_512x512_RGB(t *testing.T) {
	const (
		width  = 512
		height = 512
	)

	req, err := protocol.NewSD35GenerateRequest(
		67890,
		"another test prompt",
		width,
		height,
		28,
		7.5,
		1234,
	)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	requestBytes, err := protocol.EncodeSD35GenerateRequest(req)
	if err != nil {
		t.Fatalf("failed to encode request: %v", err)
	}

	responseBytes, err := runStubGenerator(t, requestBytes)
	if err != nil {
		t.Fatalf("stub generator failed: %v", err)
	}

	resp, err := protocol.DecodeResponse(responseBytes)
	if err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	sd35Resp, ok := resp.(*protocol.SD35GenerateResponse)
	if !ok {
		t.Fatalf("expected *SD35GenerateResponse, got %T", resp)
	}

	if sd35Resp.RequestID != req.RequestID {
		t.Errorf("request_id mismatch: got %d, expected %d", sd35Resp.RequestID, req.RequestID)
	}
	if sd35Resp.ImageWidth != width {
		t.Errorf("image width mismatch: got %d, expected %d", sd35Resp.ImageWidth, width)
	}
	if sd35Resp.ImageHeight != height {
		t.Errorf("image height mismatch: got %d, expected %d", sd35Resp.ImageHeight, height)
	}

	verifyCheckerboard(t, sd35Resp.ImageWidth, sd35Resp.ImageHeight, sd35Resp.Channels, sd35Resp.ImageData)
}

// TestProtocolRoundTrip_1024x1024_RGB tests round-trip with larger dimensions.
func TestProtocolRoundTrip_1024x1024_RGB(t *testing.T) {
	const (
		width  = 1024
		height = 1024
	)

	req, err := protocol.NewSD35GenerateRequest(
		11111,
		"large image test",
		width,
		height,
		50,
		8.0,
		5555,
	)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	requestBytes, err := protocol.EncodeSD35GenerateRequest(req)
	if err != nil {
		t.Fatalf("failed to encode request: %v", err)
	}

	responseBytes, err := runStubGenerator(t, requestBytes)
	if err != nil {
		t.Fatalf("stub generator failed: %v", err)
	}

	resp, err := protocol.DecodeResponse(responseBytes)
	if err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	sd35Resp, ok := resp.(*protocol.SD35GenerateResponse)
	if !ok {
		t.Fatalf("expected *SD35GenerateResponse, got %T", resp)
	}

	if sd35Resp.RequestID != req.RequestID {
		t.Errorf("request_id mismatch: got %d, expected %d", sd35Resp.RequestID, req.RequestID)
	}
	if sd35Resp.ImageWidth != width {
		t.Errorf("image width mismatch: got %d, expected %d", sd35Resp.ImageWidth, width)
	}
	if sd35Resp.ImageHeight != height {
		t.Errorf("image height mismatch: got %d, expected %d", sd35Resp.ImageHeight, height)
	}

	verifyCheckerboard(t, sd35Resp.ImageWidth, sd35Resp.ImageHeight, sd35Resp.Channels, sd35Resp.ImageData)
}

// TestProtocolRoundTrip_MultipleRequests verifies the stub generator handles multiple sequential requests.
func TestProtocolRoundTrip_MultipleRequests(t *testing.T) {
	testCases := []struct {
		name      string
		requestID uint64
		prompt    string
		width     uint32
		height    uint32
	}{
		{"64x64", 1, "prompt 1", 64, 64},
		{"128x128", 2, "prompt 2", 128, 128},
		{"256x256", 3, "prompt 3", 256, 256},
		{"512x512", 4, "prompt 4", 512, 512},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := protocol.NewSD35GenerateRequest(
				tc.requestID,
				tc.prompt,
				tc.width,
				tc.height,
				28,
				7.0,
				0,
			)
			if err != nil {
				t.Fatalf("failed to create request: %v", err)
			}

			requestBytes, err := protocol.EncodeSD35GenerateRequest(req)
			if err != nil {
				t.Fatalf("failed to encode request: %v", err)
			}

			responseBytes, err := runStubGenerator(t, requestBytes)
			if err != nil {
				t.Fatalf("stub generator failed: %v", err)
			}

			resp, err := protocol.DecodeResponse(responseBytes)
			if err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}

			sd35Resp, ok := resp.(*protocol.SD35GenerateResponse)
			if !ok {
				t.Fatalf("expected *SD35GenerateResponse, got %T", resp)
			}

			if sd35Resp.RequestID != tc.requestID {
				t.Errorf("request_id mismatch: got %d, expected %d", sd35Resp.RequestID, tc.requestID)
			}
			if sd35Resp.ImageWidth != tc.width || sd35Resp.ImageHeight != tc.height {
				t.Errorf("dimensions mismatch: got %dx%d, expected %dx%d",
					sd35Resp.ImageWidth, sd35Resp.ImageHeight, tc.width, tc.height)
			}

			verifyCheckerboard(t, sd35Resp.ImageWidth, sd35Resp.ImageHeight, sd35Resp.Channels, sd35Resp.ImageData)
		})
	}
}
