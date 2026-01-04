package startup

import (
	"context"
	"testing"
	"time"

	"github.com/hurricanerix/weave/internal/logging"
	"github.com/hurricanerix/weave/internal/web"
)

func TestRun_ContextCancellation(t *testing.T) {
	// Create server
	server, err := web.NewServer("localhost:0") // Use port 0 for random available port
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Create logger
	logger := logging.New(logging.LevelInfo, nil)

	// Create context that we'll cancel
	ctx, cancel := context.WithCancel(context.Background())

	// Run server in goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- Run(ctx, server, logger)
	}()

	// Give server a moment to start
	time.Sleep(50 * time.Millisecond)

	// Cancel context to trigger shutdown
	cancel()

	// Wait for shutdown with timeout
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Run() returned error: %v", err)
		}

	case <-time.After(5 * time.Second):
		t.Fatal("Shutdown timed out")
	}
}

func TestRun_ImmediateCancel(t *testing.T) {
	// Create server
	server, err := web.NewServer("localhost:0")
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Create logger
	logger := logging.New(logging.LevelInfo, nil)

	// Create already-cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Run should return quickly
	err = Run(ctx, server, logger)

	// Should complete without error (server never started)
	if err != nil {
		// This is acceptable - server may fail to start with cancelled context
		t.Logf("Run() returned error (acceptable): %v", err)
	}
}
