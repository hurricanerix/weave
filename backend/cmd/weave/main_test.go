package main

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"github.com/hurricanerix/weave/internal/logging"
)

// Note: Tests use context for checking cancellation results, but monitorStdin
// no longer takes ctx as a parameter (only cancel func) to avoid race conditions.

func TestMonitorStdin(t *testing.T) {
	tests := []struct {
		name           string
		input          io.Reader
		expectCancel   bool
		cancelAfter    time.Duration
		timeoutSeconds int
	}{
		{
			name:           "EOF triggers cancellation",
			input:          bytes.NewReader([]byte{}), // Empty reader returns EOF immediately
			expectCancel:   true,
			timeoutSeconds: 1,
		},
		{
			name:           "non-EOF error stops monitoring without cancellation",
			input:          &contextAwareReader{}, // Returns non-EOF error
			expectCancel:   false,
			timeoutSeconds: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Create a minimal logger (discard output for tests)
			logger := logging.New(logging.LevelError, io.Discard)

			// Channel to signal when monitorStdin returns
			done := make(chan struct{})

			// Start monitoring in background
			go func() {
				monitorStdin(cancel, tt.input, logger)
				close(done)
			}()

			// If test requires external cancellation, do it after delay
			if tt.cancelAfter > 0 {
				time.Sleep(tt.cancelAfter)
				cancel()
			}

			// Wait for monitoring to complete or timeout
			select {
			case <-done:
				// Monitor returned
			case <-time.After(time.Duration(tt.timeoutSeconds) * time.Second):
				t.Fatalf("monitorStdin did not return within %d seconds", tt.timeoutSeconds)
			}

			// Check if context was cancelled
			select {
			case <-ctx.Done():
				if !tt.expectCancel {
					t.Error("context was cancelled but should not have been")
				}
			default:
				if tt.expectCancel {
					t.Error("context was not cancelled but should have been")
				}
			}
		})
	}
}

func TestMonitorStdin_MultipleReads(t *testing.T) {
	// Test that stdin monitor handles partial reads correctly
	// Create a reader that provides data slowly then EOF
	slowReader := &slowReader{
		data:  []byte("test data"),
		delay: 10 * time.Millisecond,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	logger := logging.New(logging.LevelError, io.Discard)

	done := make(chan struct{})
	go func() {
		monitorStdin(cancel, slowReader, logger)
		close(done)
	}()

	// Should complete within reasonable time after all data read
	select {
	case <-done:
		// Success - monitor returned after EOF
	case <-time.After(2 * time.Second):
		t.Fatal("monitorStdin did not return after slow reader EOF")
	}

	// Context should be cancelled due to EOF
	select {
	case <-ctx.Done():
		// Expected
	default:
		t.Error("context not cancelled after EOF")
	}
}

// contextAwareReader simulates a reader that returns a non-EOF error
// This allows testing that monitorStdin exits cleanly on error
type contextAwareReader struct{}

func (r *contextAwareReader) Read(p []byte) (n int, err error) {
	// Simulate a short delay then return a non-EOF error
	time.Sleep(50 * time.Millisecond)
	return 0, io.ErrUnexpectedEOF
}

// slowReader simulates a reader that provides data slowly then EOF
type slowReader struct {
	data  []byte
	index int
	delay time.Duration
}

func (r *slowReader) Read(p []byte) (n int, err error) {
	if r.index >= len(r.data) {
		return 0, io.EOF
	}

	time.Sleep(r.delay)

	// Read one byte at a time to simulate slow input
	p[0] = r.data[r.index]
	r.index++
	return 1, nil
}
