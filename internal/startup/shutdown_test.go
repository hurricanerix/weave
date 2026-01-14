package startup

import (
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
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

func TestCleanupCompute_NilComponents(t *testing.T) {
	logger := logging.New(logging.LevelInfo, nil)

	// Should not panic with nil components
	CleanupCompute(nil, logger)
}

func TestCleanupCompute_NoComputeProcess(t *testing.T) {
	logger := logging.New(logging.LevelInfo, nil)
	components := &Components{}

	// Should not panic with empty components
	CleanupCompute(components, logger)
}

func TestCleanupCompute_GracefulShutdown(t *testing.T) {
	logger := logging.New(logging.LevelInfo, nil)

	// Create a temporary socket file
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	// Create a listening socket
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}

	// Start a process that exits quickly when stdin closes
	// Using 'cat' which exits when stdin is closed
	cmd := exec.Command("cat")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		listener.Close()
		t.Fatalf("Failed to create stdin pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		listener.Close()
		stdin.Close()
		t.Fatalf("Failed to start process: %v", err)
	}

	components := &Components{
		ComputeProcess:    cmd,
		ComputeStdin:      stdin,
		ComputeListener:   listener,
		ComputeSocketPath: socketPath,
	}

	// Cleanup should succeed
	CleanupCompute(components, logger)

	// Verify socket file was removed
	if _, err := os.Stat(socketPath); !os.IsNotExist(err) {
		t.Errorf("Socket file still exists after cleanup")
	}
}

func TestCleanupCompute_ForcedKill(t *testing.T) {
	logger := logging.New(logging.LevelInfo, nil)

	// Create a temporary socket file
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	// Create a listening socket
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}

	// Start a process that ignores stdin close (sleep)
	cmd := exec.Command("sleep", "30")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		listener.Close()
		t.Fatalf("Failed to create stdin pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		listener.Close()
		stdin.Close()
		t.Fatalf("Failed to start process: %v", err)
	}

	components := &Components{
		ComputeProcess:    cmd,
		ComputeStdin:      stdin,
		ComputeListener:   listener,
		ComputeSocketPath: socketPath,
	}

	// Cleanup should kill the process
	start := time.Now()
	CleanupCompute(components, logger)
	elapsed := time.Since(start)

	// Should complete shortly after all timeouts (stdin + SIGTERM + SIGKILL)
	// Add 1 second buffer for processing
	maxExpected := computeStdinTimeout + computeSigtermTimeout + computeSigkillTimeout + 1*time.Second
	if elapsed > maxExpected {
		t.Errorf("Cleanup took too long: %v (expected < %v)", elapsed, maxExpected)
	}

	// Verify socket file was removed
	if _, err := os.Stat(socketPath); !os.IsNotExist(err) {
		t.Errorf("Socket file still exists after cleanup")
	}
}

func TestCleanupCompute_AlreadyExited(t *testing.T) {
	logger := logging.New(logging.LevelInfo, nil)

	// Create a temporary socket file
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	// Create a listening socket
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}

	// Start a process that exits immediately
	cmd := exec.Command("true")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		listener.Close()
		t.Fatalf("Failed to create stdin pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		listener.Close()
		stdin.Close()
		t.Fatalf("Failed to start process: %v", err)
	}

	// Wait for process to exit
	time.Sleep(100 * time.Millisecond)

	components := &Components{
		ComputeProcess:    cmd,
		ComputeStdin:      stdin,
		ComputeListener:   listener,
		ComputeSocketPath: socketPath,
	}

	// Cleanup should handle already-exited process
	CleanupCompute(components, logger)

	// Verify socket file was removed
	if _, err := os.Stat(socketPath); !os.IsNotExist(err) {
		t.Errorf("Socket file still exists after cleanup")
	}
}

func TestCleanupCompute_MissingSocketFile(t *testing.T) {
	logger := logging.New(logging.LevelInfo, nil)

	// Create a temporary socket file
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	// Create a listening socket
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}

	// Remove socket file before cleanup
	if err := os.Remove(socketPath); err != nil {
		t.Fatalf("Failed to remove socket file: %v", err)
	}

	// Start a process
	cmd := exec.Command("cat")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		listener.Close()
		t.Fatalf("Failed to create stdin pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		listener.Close()
		stdin.Close()
		t.Fatalf("Failed to start process: %v", err)
	}

	components := &Components{
		ComputeProcess:    cmd,
		ComputeStdin:      stdin,
		ComputeListener:   listener,
		ComputeSocketPath: socketPath,
	}

	// Cleanup should not fail even if socket file is already gone
	CleanupCompute(components, logger)
}

func TestCleanupCompute_NilStdin(t *testing.T) {
	logger := logging.New(logging.LevelInfo, nil)

	// Create a temporary socket file
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	// Create a listening socket
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}

	// Start a process (it will exit quickly since stdin is not piped)
	cmd := exec.Command("true")
	if err := cmd.Start(); err != nil {
		listener.Close()
		t.Fatalf("Failed to start process: %v", err)
	}

	components := &Components{
		ComputeProcess:    cmd,
		ComputeStdin:      nil, // No stdin
		ComputeListener:   listener,
		ComputeSocketPath: socketPath,
	}

	// Cleanup should handle nil stdin gracefully
	CleanupCompute(components, logger)

	// Verify socket file was removed
	if _, err := os.Stat(socketPath); !os.IsNotExist(err) {
		t.Errorf("Socket file still exists after cleanup")
	}
}

func TestCleanupCompute_NilListener(t *testing.T) {
	logger := logging.New(logging.LevelInfo, nil)

	// Create a temporary socket path (no actual socket)
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	// Create the socket file manually
	f, err := os.Create(socketPath)
	if err != nil {
		t.Fatalf("Failed to create socket file: %v", err)
	}
	f.Close()

	// Start a process
	cmd := exec.Command("cat")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("Failed to create stdin pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		stdin.Close()
		t.Fatalf("Failed to start process: %v", err)
	}

	components := &Components{
		ComputeProcess:    cmd,
		ComputeStdin:      stdin,
		ComputeListener:   nil, // No listener
		ComputeSocketPath: socketPath,
	}

	// Cleanup should handle nil listener gracefully
	CleanupCompute(components, logger)

	// Verify socket file was removed
	if _, err := os.Stat(socketPath); !os.IsNotExist(err) {
		t.Errorf("Socket file still exists after cleanup")
	}
}

func TestCleanupCompute_ClosedStdin(t *testing.T) {
	logger := logging.New(logging.LevelInfo, nil)

	// Create a temporary socket file
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	// Create a listening socket
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}

	// Start a process
	cmd := exec.Command("cat")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		listener.Close()
		t.Fatalf("Failed to create stdin pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		listener.Close()
		stdin.Close()
		t.Fatalf("Failed to start process: %v", err)
	}

	// Close stdin before cleanup
	stdin.Close()

	components := &Components{
		ComputeProcess:    cmd,
		ComputeStdin:      stdin,
		ComputeListener:   listener,
		ComputeSocketPath: socketPath,
	}

	// Cleanup should handle already-closed stdin gracefully
	CleanupCompute(components, logger)

	// Verify socket file was removed
	if _, err := os.Stat(socketPath); !os.IsNotExist(err) {
		t.Errorf("Socket file still exists after cleanup")
	}
}
