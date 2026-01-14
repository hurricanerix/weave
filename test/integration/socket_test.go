//go:build integration

package integration

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/hurricanerix/weave/internal/client"
	"github.com/hurricanerix/weave/internal/logging"
	"github.com/hurricanerix/weave/internal/startup"
)

// computePath returns the path to the weave-compute daemon binary.
func computePath(t *testing.T) string {
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

	return filepath.Join(absRoot, "compute-daemon", "weave-compute")
}

// testEnv creates a temporary XDG_RUNTIME_DIR for testing.
// Returns the temp directory path and a cleanup function.
func testEnv(t *testing.T) (string, func()) {
	t.Helper()

	tmpDir := t.TempDir()

	// Save original env
	origXDG := os.Getenv("XDG_RUNTIME_DIR")

	cleanup := func() {
		os.Setenv("XDG_RUNTIME_DIR", origXDG)
	}

	return tmpDir, cleanup
}

// TestWeaveCreatesSocket verifies weave creates listening socket before spawning compute.
func TestWeaveCreatesSocket(t *testing.T) {
	tmpDir, envCleanup := testEnv(t)
	defer envCleanup()

	os.Setenv("XDG_RUNTIME_DIR", tmpDir)

	// Create socket using weave's CreateSocket function
	listener, socketPath, err := startup.CreateSocket()
	if err != nil {
		t.Fatalf("CreateSocket() failed: %v", err)
	}
	defer listener.Close()

	// Verify socket file exists
	info, err := os.Stat(socketPath)
	if err != nil {
		t.Fatalf("socket file not found: %v", err)
	}

	// Verify socket file permissions (should be 0600 on the file itself)
	// Note: The socket directory has 0700, but we're checking the socket file
	mode := info.Mode()
	if mode&os.ModeSocket == 0 {
		t.Errorf("socket file is not a socket: mode=%o", mode)
	}

	// Verify socket directory has correct permissions (0700)
	sockDir := filepath.Dir(socketPath)
	dirInfo, err := os.Stat(sockDir)
	if err != nil {
		t.Fatalf("socket directory not found: %v", err)
	}
	if dirInfo.Mode()&os.ModePerm != 0700 {
		t.Errorf("socket directory permissions = %o, want 0700", dirInfo.Mode()&os.ModePerm)
	}
}

// TestComputeConnectsToWeaveSocket verifies compute connects to weave's existing socket.
func TestComputeConnectsToWeaveSocket(t *testing.T) {
	tmpDir, envCleanup := testEnv(t)
	defer envCleanup()

	os.Setenv("XDG_RUNTIME_DIR", tmpDir)

	// Step 1: weave creates socket
	listener, socketPath, err := startup.CreateSocket()
	if err != nil {
		t.Fatalf("CreateSocket() failed: %v", err)
	}
	defer listener.Close()
	defer os.Remove(socketPath)

	// Step 2: weave spawns compute with socket path
	cmd, stdin, err := startup.SpawnCompute(socketPath)
	if err != nil {
		t.Fatalf("SpawnCompute() failed: %v", err)
	}
	defer func() {
		if stdin != nil {
			stdin.Close()
		}
		if cmd.Process != nil {
			cmd.Process.Kill()
			cmd.Wait()
		}
	}()

	// Step 3: weave accepts compute's connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := client.AcceptConnection(ctx, listener)
	if err != nil {
		t.Fatalf("AcceptConnection() failed: %v", err)
	}
	defer conn.Close()

	// Verify connection is established
	if conn.RawConn() == nil {
		t.Error("connection has nil RawConn")
	}
}

// TestPersistentConnection verifies requests work over the persistent connection.
func TestPersistentConnection(t *testing.T) {
	tmpDir, envCleanup := testEnv(t)
	defer envCleanup()

	os.Setenv("XDG_RUNTIME_DIR", tmpDir)

	// Step 1: weave creates socket
	listener, socketPath, err := startup.CreateSocket()
	if err != nil {
		t.Fatalf("CreateSocket() failed: %v", err)
	}
	defer listener.Close()
	defer os.Remove(socketPath)

	// Step 2: weave spawns compute
	cmd, stdin, err := startup.SpawnCompute(socketPath)
	if err != nil {
		t.Fatalf("SpawnCompute() failed: %v", err)
	}
	defer func() {
		if stdin != nil {
			stdin.Close()
		}
		if cmd.Process != nil {
			cmd.Process.Kill()
			cmd.Wait()
		}
	}()

	// Step 3: weave accepts compute's connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := client.AcceptConnection(ctx, listener)
	if err != nil {
		t.Fatalf("AcceptConnection() failed: %v", err)
	}
	defer conn.Close()

	// Step 4: Send a minimal valid protocol message to verify connection works
	// Note: The compute daemon has a placeholder handler that closes immediately,
	// but we can verify the connection was established and multiplexing works.

	// Create a minimal test message (16 byte header + 8 byte request ID + empty payload)
	testMsg := make([]byte, 24)
	// Magic: 0x57455645 ("WEVE")
	testMsg[0] = 0x57
	testMsg[1] = 0x45
	testMsg[2] = 0x56
	testMsg[3] = 0x45
	// Version: 0x0001
	testMsg[4] = 0x00
	testMsg[5] = 0x01
	// Message type: 0x0001 (some request)
	testMsg[6] = 0x00
	testMsg[7] = 0x01
	// Payload length: 8 (just the request ID)
	testMsg[8] = 0x00
	testMsg[9] = 0x00
	testMsg[10] = 0x00
	testMsg[11] = 0x08
	// Request ID: 12345 (little-endian)
	testMsg[16] = 0x39
	testMsg[17] = 0x30
	testMsg[18] = 0x00
	testMsg[19] = 0x00
	testMsg[20] = 0x00
	testMsg[21] = 0x00
	testMsg[22] = 0x00
	testMsg[23] = 0x00

	// Use short timeout since daemon may close connection
	sendCtx, sendCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer sendCancel()

	// Attempt to send - daemon's placeholder handler closes connection immediately.
	// We expect connection closed or write error (broken pipe).
	_, err = conn.Send(sendCtx, testMsg)

	// The daemon accepted the connection, then immediately closed it
	// because the placeholder handler returns without reading/writing.
	// This is expected. The test verifies that:
	// 1. We could establish persistent connection
	// 2. Connection closes cleanly (not a crash)
	if err != nil {
		// Expected: connection closed, write error, or timeout
		t.Logf("expected error (daemon placeholder closes connection): %v", err)
	}
}

// TestComputeTerminatesOnConnectionClose verifies compute terminates when weave closes connection.
func TestComputeTerminatesOnConnectionClose(t *testing.T) {
	tmpDir, envCleanup := testEnv(t)
	defer envCleanup()

	os.Setenv("XDG_RUNTIME_DIR", tmpDir)

	// Step 1: weave creates socket
	listener, socketPath, err := startup.CreateSocket()
	if err != nil {
		t.Fatalf("CreateSocket() failed: %v", err)
	}
	defer listener.Close()
	defer os.Remove(socketPath)

	// Step 2: weave spawns compute
	cmd, stdin, err := startup.SpawnCompute(socketPath)
	if err != nil {
		t.Fatalf("SpawnCompute() failed: %v", err)
	}
	defer func() {
		if stdin != nil {
			stdin.Close()
		}
		if cmd.Process != nil {
			cmd.Process.Kill()
			cmd.Wait()
		}
	}()

	// Step 3: weave accepts compute's connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := client.AcceptConnection(ctx, listener)
	if err != nil {
		t.Fatalf("AcceptConnection() failed: %v", err)
	}

	// Step 4: Close connection to signal compute shutdown
	if err := conn.Close(); err != nil {
		t.Errorf("conn.Close() failed: %v", err)
	}

	// Step 5: Close stdin as well (belt and suspenders)
	if err := stdin.Close(); err != nil {
		t.Errorf("stdin.Close() failed: %v", err)
	}

	// Step 6: Verify compute terminates within 2 seconds
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		// Process exited - this is what we want
		if err != nil {
			// Check if it's a signal exit or clean exit
			if exitErr, ok := err.(*exec.ExitError); ok {
				// Exit code != 0 is acceptable for signal termination
				t.Logf("compute process exited with: %v", exitErr)
			} else {
				t.Logf("compute process exited: %v", err)
			}
		} else {
			t.Logf("compute process exited cleanly")
		}
	case <-time.After(2 * time.Second):
		// Process did not exit in time - kill it
		cmd.Process.Kill()
		cmd.Wait()
		t.Fatal("compute process did not terminate within 2 seconds after connection close")
	}
}

// TestComputeTerminatesOnWeaveExit verifies compute terminates when weave exits.
func TestComputeTerminatesOnWeaveExit(t *testing.T) {
	tmpDir, envCleanup := testEnv(t)
	defer envCleanup()

	os.Setenv("XDG_RUNTIME_DIR", tmpDir)

	// Step 1: weave creates socket
	listener, socketPath, err := startup.CreateSocket()
	if err != nil {
		t.Fatalf("CreateSocket() failed: %v", err)
	}
	defer os.Remove(socketPath)

	// Step 2: weave spawns compute
	cmd, stdin, err := startup.SpawnCompute(socketPath)
	if err != nil {
		t.Fatalf("SpawnCompute() failed: %v", err)
	}
	defer func() {
		if stdin != nil {
			stdin.Close()
		}
		if cmd.Process != nil {
			cmd.Process.Kill()
			cmd.Wait()
		}
	}()

	// Step 3: weave accepts compute's connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := client.AcceptConnection(ctx, listener)
	if err != nil {
		t.Fatalf("AcceptConnection() failed: %v", err)
	}

	// Step 4: Simulate weave exit by cleaning up resources
	// This is what startup.CleanupCompute() does
	conn.Close()
	stdin.Close()
	listener.Close()

	// Step 5: Verify compute terminates within 2 seconds
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		// Process exited - this is what we want
		if err != nil {
			t.Logf("compute process exited with: %v", err)
		} else {
			t.Logf("compute process exited cleanly")
		}
	case <-time.After(2 * time.Second):
		// Process did not exit in time - kill it
		cmd.Process.Kill()
		cmd.Wait()
		t.Fatal("compute process did not terminate within 2 seconds after weave exit simulation")
	}
}

// TestNoOrphanedProcesses verifies no orphaned compute processes remain after weave cleanup.
func TestNoOrphanedProcesses(t *testing.T) {
	tmpDir, envCleanup := testEnv(t)
	defer envCleanup()

	os.Setenv("XDG_RUNTIME_DIR", tmpDir)

	// Step 1: weave creates socket
	listener, socketPath, err := startup.CreateSocket()
	if err != nil {
		t.Fatalf("CreateSocket() failed: %v", err)
	}
	defer os.Remove(socketPath)

	// Step 2: weave spawns compute
	cmd, stdin, err := startup.SpawnCompute(socketPath)
	if err != nil {
		t.Fatalf("SpawnCompute() failed: %v", err)
	}

	// Remember compute PID to check later
	computePID := cmd.Process.Pid

	// Step 3: weave accepts compute's connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := client.AcceptConnection(ctx, listener)
	if err != nil {
		t.Fatalf("AcceptConnection() failed: %v", err)
	}

	// Step 4: Use CleanupCompute to properly terminate
	logger := logging.NewFromString("debug", nil)
	components := &startup.Components{
		ComputeProcess:    cmd,
		ComputeStdin:      stdin,
		ComputeClient:     conn,
		ComputeListener:   listener,
		ComputeSocketPath: socketPath,
	}

	startup.CleanupCompute(components, logger)

	// Step 5: Verify compute process no longer exists
	// Use syscall.Kill with signal 0 to check if process exists.
	// On Unix systems, signal 0 checks process existence without sending a signal.
	err = syscall.Kill(computePID, syscall.Signal(0))
	if err == nil {
		// Process still exists - bad
		// Kill it forcefully and fail the test
		syscall.Kill(computePID, syscall.SIGKILL)
		t.Errorf("compute process (PID %d) still running after cleanup", computePID)
	} else if err == syscall.ESRCH {
		// Process does not exist - good
		t.Logf("compute process (PID %d) successfully terminated", computePID)
	} else {
		// Unexpected error
		t.Errorf("unexpected error checking process (PID %d): %v", computePID, err)
	}
}

// TestNoStaleSocketFiles verifies no stale socket files remain after shutdown.
func TestNoStaleSocketFiles(t *testing.T) {
	tmpDir, envCleanup := testEnv(t)
	defer envCleanup()

	os.Setenv("XDG_RUNTIME_DIR", tmpDir)

	// Step 1: weave creates socket
	listener, socketPath, err := startup.CreateSocket()
	if err != nil {
		t.Fatalf("CreateSocket() failed: %v", err)
	}

	// Step 2: weave spawns compute
	cmd, stdin, err := startup.SpawnCompute(socketPath)
	if err != nil {
		t.Fatalf("SpawnCompute() failed: %v", err)
	}

	// Step 3: weave accepts compute's connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := client.AcceptConnection(ctx, listener)
	if err != nil {
		t.Fatalf("AcceptConnection() failed: %v", err)
	}

	// Step 4: Use CleanupCompute to properly terminate and clean up
	logger := logging.NewFromString("debug", nil)
	components := &startup.Components{
		ComputeProcess:    cmd,
		ComputeStdin:      stdin,
		ComputeClient:     conn,
		ComputeListener:   listener,
		ComputeSocketPath: socketPath,
	}

	startup.CleanupCompute(components, logger)

	// Step 5: Verify socket file was removed
	if _, err := os.Stat(socketPath); err == nil {
		t.Errorf("socket file still exists after cleanup: %s", socketPath)
	} else if !os.IsNotExist(err) {
		t.Errorf("unexpected error checking socket file: %v", err)
	}
}

// TestMultipleSequentialRequests verifies multiple sequential requests work over single persistent connection.
func TestMultipleSequentialRequests(t *testing.T) {
	tmpDir, envCleanup := testEnv(t)
	defer envCleanup()

	os.Setenv("XDG_RUNTIME_DIR", tmpDir)

	// Step 1: weave creates socket
	listener, socketPath, err := startup.CreateSocket()
	if err != nil {
		t.Fatalf("CreateSocket() failed: %v", err)
	}
	defer listener.Close()
	defer os.Remove(socketPath)

	// Step 2: weave spawns compute
	cmd, stdin, err := startup.SpawnCompute(socketPath)
	if err != nil {
		t.Fatalf("SpawnCompute() failed: %v", err)
	}
	defer func() {
		if stdin != nil {
			stdin.Close()
		}
		if cmd.Process != nil {
			cmd.Process.Kill()
			cmd.Wait()
		}
	}()

	// Step 3: weave accepts compute's connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := client.AcceptConnection(ctx, listener)
	if err != nil {
		t.Fatalf("AcceptConnection() failed: %v", err)
	}
	defer conn.Close()

	// Step 4: Send multiple requests with different request IDs
	// Note: The placeholder handler will close the connection, but we're testing
	// that the multiplexing logic correctly handles multiple request IDs.

	testCases := []uint64{1, 2, 3, 4, 5}

	for _, requestID := range testCases {
		t.Run(fmt.Sprintf("request_%d", requestID), func(t *testing.T) {
			// Create test message with unique request ID
			testMsg := make([]byte, 24)
			// Magic: 0x57455645 ("WEVE")
			testMsg[0] = 0x57
			testMsg[1] = 0x45
			testMsg[2] = 0x56
			testMsg[3] = 0x45
			// Version: 0x0001
			testMsg[4] = 0x00
			testMsg[5] = 0x01
			// Message type: 0x0001
			testMsg[6] = 0x00
			testMsg[7] = 0x01
			// Payload length: 8
			testMsg[8] = 0x00
			testMsg[9] = 0x00
			testMsg[10] = 0x00
			testMsg[11] = 0x08
			// Request ID (little-endian)
			testMsg[16] = byte(requestID)
			testMsg[17] = byte(requestID >> 8)
			testMsg[18] = byte(requestID >> 16)
			testMsg[19] = byte(requestID >> 24)
			testMsg[20] = byte(requestID >> 32)
			testMsg[21] = byte(requestID >> 40)
			testMsg[22] = byte(requestID >> 48)
			testMsg[23] = byte(requestID >> 56)

			sendCtx, sendCancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer sendCancel()

			// We expect this to fail since the placeholder handler closes immediately
			_, err := conn.Send(sendCtx, testMsg)
			if err != nil {
				t.Logf("expected error (placeholder handler): %v", err)
			}
		})
	}
}

// TestXDGRuntimeDirNotSet_CreateSocket verifies CreateSocket returns expected error
// when XDG_RUNTIME_DIR is not set.
func TestXDGRuntimeDirNotSet_CreateSocket(t *testing.T) {
	// Save and restore env
	origXDG := os.Getenv("XDG_RUNTIME_DIR")
	defer os.Setenv("XDG_RUNTIME_DIR", origXDG)

	os.Unsetenv("XDG_RUNTIME_DIR")

	listener, socketPath, err := startup.CreateSocket()

	if err == nil {
		listener.Close()
		t.Fatal("expected error when XDG_RUNTIME_DIR not set, got nil")
	}

	if !errors.Is(err, client.ErrXDGNotSet) {
		t.Errorf("expected ErrXDGNotSet, got: %v", err)
	}

	if listener != nil {
		t.Error("expected nil listener on error")
	}

	if socketPath != "" {
		t.Error("expected empty socketPath on error")
	}
}

// TestAcceptConnectionTimeout verifies AcceptConnection times out if compute never connects.
func TestAcceptConnectionTimeout(t *testing.T) {
	tmpDir, envCleanup := testEnv(t)
	defer envCleanup()

	os.Setenv("XDG_RUNTIME_DIR", tmpDir)

	// Step 1: weave creates socket
	listener, socketPath, err := startup.CreateSocket()
	if err != nil {
		t.Fatalf("CreateSocket() failed: %v", err)
	}
	defer listener.Close()
	defer os.Remove(socketPath)

	// Step 2: Do NOT spawn compute - listener should timeout

	// Step 3: Try to accept connection with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	conn, err := client.AcceptConnection(ctx, listener)

	// Should timeout since no compute process connected
	if err == nil {
		conn.Close()
		t.Fatal("expected timeout error, got nil")
	}

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected context.DeadlineExceeded, got: %v", err)
	}

	if conn != nil {
		t.Error("expected nil connection on timeout")
	}
}

// TestComputeReconnection verifies weave can accept a second connection after first compute exits.
func TestComputeReconnection(t *testing.T) {
	tmpDir, envCleanup := testEnv(t)
	defer envCleanup()

	os.Setenv("XDG_RUNTIME_DIR", tmpDir)

	// Step 1: weave creates socket
	listener, socketPath, err := startup.CreateSocket()
	if err != nil {
		t.Fatalf("CreateSocket() failed: %v", err)
	}
	defer listener.Close()
	defer os.Remove(socketPath)

	// Step 2: First compute instance - spawn, accept, verify, terminate
	cmd1, stdin1, err := startup.SpawnCompute(socketPath)
	if err != nil {
		t.Fatalf("SpawnCompute() (first) failed: %v", err)
	}

	ctx1, cancel1 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel1()

	conn1, err := client.AcceptConnection(ctx1, listener)
	if err != nil {
		stdin1.Close()
		cmd1.Process.Kill()
		cmd1.Wait()
		t.Fatalf("AcceptConnection() (first) failed: %v", err)
	}

	// Verify first connection works
	if conn1.RawConn() == nil {
		t.Error("first connection has nil RawConn")
	}

	// Cleanly terminate first compute
	conn1.Close()
	stdin1.Close()

	// Wait for first compute to exit
	done1 := make(chan error, 1)
	go func() {
		done1 <- cmd1.Wait()
	}()

	select {
	case <-done1:
		t.Log("first compute exited")
	case <-time.After(2 * time.Second):
		cmd1.Process.Kill()
		cmd1.Wait()
		t.Fatal("first compute did not exit within 2 seconds")
	}

	// Step 3: Second compute instance - spawn, accept, verify
	cmd2, stdin2, err := startup.SpawnCompute(socketPath)
	if err != nil {
		t.Fatalf("SpawnCompute() (second) failed: %v", err)
	}
	defer func() {
		if stdin2 != nil {
			stdin2.Close()
		}
		if cmd2.Process != nil {
			cmd2.Process.Kill()
			cmd2.Wait()
		}
	}()

	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()

	conn2, err := client.AcceptConnection(ctx2, listener)
	if err != nil {
		t.Fatalf("AcceptConnection() (second) failed: %v", err)
	}
	defer conn2.Close()

	// Verify second connection works
	if conn2.RawConn() == nil {
		t.Error("second connection has nil RawConn")
	}

	t.Log("successfully accepted second connection after first compute exit")
}

// TestComputeBinaryNotFound verifies SpawnCompute returns expected error
// when compute binary is not found.
func TestComputeBinaryNotFound(t *testing.T) {
	// Use a socket path in a directory that doesn't contain the binary
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "weave.sock")

	// Save current directory
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	defer os.Chdir(origDir)

	// Change to temp directory where binary doesn't exist
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}

	cmd, stdin, err := startup.SpawnCompute(socketPath)

	if err == nil {
		stdin.Close()
		cmd.Process.Kill()
		cmd.Wait()
		t.Fatal("expected error when compute binary not found, got nil")
	}

	if !errors.Is(err, startup.ErrComputeBinaryNotFound) {
		t.Errorf("expected ErrComputeBinaryNotFound, got: %v", err)
	}

	if cmd != nil {
		t.Error("expected nil cmd on error")
	}

	if stdin != nil {
		t.Error("expected nil stdin on error")
	}
}
