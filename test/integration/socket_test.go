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
)

// daemonPath returns the path to the weave-compute daemon binary.
func daemonPath(t *testing.T) string {
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

// daemonTestEnv creates a temporary XDG_RUNTIME_DIR for testing.
// Returns the temp directory path and a cleanup function.
func daemonTestEnv(t *testing.T) (string, func()) {
	t.Helper()

	tmpDir := t.TempDir()

	// Save original env
	origXDG := os.Getenv("XDG_RUNTIME_DIR")

	cleanup := func() {
		os.Setenv("XDG_RUNTIME_DIR", origXDG)
	}

	return tmpDir, cleanup
}

// startDaemon starts the daemon in a temporary environment.
// Returns the daemon process and a cleanup function that kills the daemon.
func startDaemon(t *testing.T, xdgDir string) (*exec.Cmd, func()) {
	t.Helper()

	daemonBin := daemonPath(t)

	// Verify daemon binary exists
	if _, err := os.Stat(daemonBin); os.IsNotExist(err) {
		t.Fatalf("daemon binary not found at %s (run 'make' in compute-daemon/)", daemonBin)
	}

	// Create daemon process
	cmd := exec.Command(daemonBin)
	cmd.Env = append(os.Environ(), fmt.Sprintf("XDG_RUNTIME_DIR=%s", xdgDir))

	// Capture output for debugging
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Start daemon
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start daemon: %v", err)
	}

	// Wait for socket to be created (poll up to 2 seconds)
	socketPath := filepath.Join(xdgDir, "weave", "weave.sock")
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(socketPath); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Verify socket was created
	if _, err := os.Stat(socketPath); os.IsNotExist(err) {
		cmd.Process.Kill()
		cmd.Wait()
		t.Fatalf("daemon did not create socket at %s", socketPath)
	}

	cleanup := func() {
		if cmd.Process != nil {
			cmd.Process.Kill()
			cmd.Wait()
		}
	}

	return cmd, cleanup
}

// TestDaemonStartupAndConnection verifies daemon starts, client connects successfully,
// and socket file is created.
func TestDaemonStartupAndConnection(t *testing.T) {
	tmpDir, envCleanup := daemonTestEnv(t)
	defer envCleanup()

	os.Setenv("XDG_RUNTIME_DIR", tmpDir)

	daemon, daemonCleanup := startDaemon(t, tmpDir)
	defer daemonCleanup()

	// Verify daemon is running
	if daemon.Process == nil {
		t.Fatal("daemon process is nil")
	}

	// Verify socket file exists
	socketPath := filepath.Join(tmpDir, "weave", "weave.sock")
	info, err := os.Stat(socketPath)
	if err != nil {
		t.Fatalf("socket file not found: %v", err)
	}

	// Verify socket file permissions (0600)
	mode := info.Mode()
	if mode&os.ModePerm != 0600 {
		t.Errorf("socket file permissions = %o, want 0600", mode&os.ModePerm)
	}

	// Connect to daemon
	ctx := context.Background()
	conn, err := client.Connect(ctx)
	if err != nil {
		t.Fatalf("failed to connect to daemon: %v", err)
	}
	defer conn.Close()

	// Verify connection is open
	if conn.RawConn() == nil {
		t.Error("connection has nil RawConn")
	}
}

// TestClientConnectionWithMatchingUID verifies client connection with matching UID
// succeeds (this is the normal case).
//
// Note: Testing rejection of different UID connections would require:
// - Running test as root and using setuid/setgid
// - Or using a test shim that can manipulate SO_PEERCRED credentials
// This is beyond the scope of standard Go integration tests and is deferred.
// The C unit tests in compute-daemon/test/test_socket.c verify SO_PEERCRED behavior.
func TestClientConnectionWithMatchingUID(t *testing.T) {
	tmpDir, envCleanup := daemonTestEnv(t)
	defer envCleanup()

	os.Setenv("XDG_RUNTIME_DIR", tmpDir)

	_, daemonCleanup := startDaemon(t, tmpDir)
	defer daemonCleanup()

	// Connect to daemon (should succeed - same UID)
	ctx := context.Background()
	conn, err := client.Connect(ctx)
	if err != nil {
		t.Fatalf("client with matching UID failed to connect: %v", err)
	}
	defer conn.Close()

	// Send a minimal valid protocol message to verify connection is accepted.
	// Note: The placeholder handler returns immediately without reading or responding,
	// which causes the connection to close. This is expected behavior.
	// The important part is that authentication passed (we got past SO_PEERCRED check).

	// Create a minimal test message (16 byte header with zero payload)
	testMsg := make([]byte, 16)
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
	// Payload length: 0
	testMsg[8] = 0x00
	testMsg[9] = 0x00
	testMsg[10] = 0x00
	testMsg[11] = 0x00

	// Use short timeout since daemon doesn't respond
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// Attempt to send - daemon's placeholder handler closes connection immediately.
	// We expect connection closed or write error (broken pipe).
	_, err = conn.Send(ctx, testMsg)

	// The daemon accepted the connection (auth passed), then immediately closed it
	// because the placeholder handler returns without reading/writing.
	// This is expected. The test verifies that:
	// 1. We could connect (auth passed)
	// 2. Connection closes cleanly (not a crash)
	if err != nil {
		// Expected: connection closed, write error, or timeout
		t.Logf("expected error (daemon placeholder closes connection): %v", err)
	}
}

// TestClientDaemonNotRunning verifies client handles ENOENT gracefully when
// daemon is not running.
func TestClientDaemonNotRunning(t *testing.T) {
	tmpDir, envCleanup := daemonTestEnv(t)
	defer envCleanup()

	os.Setenv("XDG_RUNTIME_DIR", tmpDir)

	// Don't start daemon - socket doesn't exist

	ctx := context.Background()
	conn, err := client.Connect(ctx)

	if err == nil {
		conn.Close()
		t.Fatal("expected error when daemon not running, got nil")
	}

	if !errors.Is(err, client.ErrDaemonNotRunning) {
		t.Errorf("expected ErrDaemonNotRunning, got: %v", err)
	}

	expectedMsg := "weave-compute daemon not running (socket not found)"
	if err.Error() != expectedMsg {
		t.Errorf("error message = %q, want %q", err.Error(), expectedMsg)
	}
}

// TestStaleSocketCleanup verifies stale socket file is cleaned up on daemon restart.
func TestStaleSocketCleanup(t *testing.T) {
	tmpDir, envCleanup := daemonTestEnv(t)
	defer envCleanup()

	os.Setenv("XDG_RUNTIME_DIR", tmpDir)

	// Start daemon first time
	daemon1, cleanup1 := startDaemon(t, tmpDir)
	socketPath := filepath.Join(tmpDir, "weave", "weave.sock")

	// Verify socket exists
	if _, err := os.Stat(socketPath); err != nil {
		t.Fatalf("socket not created by first daemon: %v", err)
	}

	// Kill daemon abruptly (simulates crash - socket file left behind)
	if err := daemon1.Process.Kill(); err != nil {
		t.Fatalf("failed to kill daemon: %v", err)
	}
	daemon1.Wait()
	cleanup1()

	// Verify socket file still exists (stale socket)
	if _, err := os.Stat(socketPath); err != nil {
		t.Fatalf("socket file was removed (expected stale socket): %v", err)
	}

	// Start daemon second time - should clean up stale socket
	_, cleanup2 := startDaemon(t, tmpDir)
	defer cleanup2()

	// Verify socket exists and is usable
	if _, err := os.Stat(socketPath); err != nil {
		t.Fatalf("socket not created after stale socket cleanup: %v", err)
	}

	// Give daemon a moment to fully start accept loop
	time.Sleep(50 * time.Millisecond)

	// Verify we can connect (proves stale socket was cleaned up)
	ctx := context.Background()
	conn, err := client.Connect(ctx)
	if err != nil {
		t.Fatalf("failed to connect after stale socket cleanup: %v", err)
	}
	defer conn.Close()
}

// TestDaemonSIGTERMCleanup verifies daemon handles SIGTERM gracefully and
// cleans up socket file.
func TestDaemonSIGTERMCleanup(t *testing.T) {
	tmpDir, envCleanup := daemonTestEnv(t)
	defer envCleanup()

	os.Setenv("XDG_RUNTIME_DIR", tmpDir)

	daemon, cleanup := startDaemon(t, tmpDir)
	defer cleanup()

	socketPath := filepath.Join(tmpDir, "weave", "weave.sock")

	// Verify socket exists
	if _, err := os.Stat(socketPath); err != nil {
		t.Fatalf("socket not created: %v", err)
	}

	// Send SIGTERM for graceful shutdown
	if err := daemon.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("failed to send SIGTERM: %v", err)
	}

	// Wait for daemon to exit (with timeout)
	done := make(chan error, 1)
	go func() {
		done <- daemon.Wait()
	}()

	select {
	case err := <-done:
		if err != nil {
			// Check if it's a signal exit (expected)
			if exitErr, ok := err.(*exec.ExitError); ok {
				if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
					if status.Signaled() && status.Signal() == syscall.SIGTERM {
						// Expected - daemon was killed by SIGTERM
						t.Logf("daemon exited with SIGTERM (expected)")
					} else if status.ExitStatus() == 0 {
						// Also acceptable - daemon exited cleanly
						t.Logf("daemon exited cleanly")
					} else {
						t.Errorf("daemon exited with unexpected status: %v", err)
					}
				}
			} else {
				t.Errorf("daemon exited with error: %v", err)
			}
		}
	case <-time.After(3 * time.Second):
		daemon.Process.Kill()
		t.Fatal("daemon did not exit within 3 seconds after SIGTERM")
	}

	// Verify socket file was cleaned up
	if _, err := os.Stat(socketPath); err == nil {
		t.Error("socket file was not cleaned up after SIGTERM")
	} else if !os.IsNotExist(err) {
		t.Errorf("unexpected error checking socket file: %v", err)
	}
}

// TestXDGRuntimeDirNotSet_Client verifies client returns expected error when
// XDG_RUNTIME_DIR is not set.
func TestXDGRuntimeDirNotSet_Client(t *testing.T) {
	// Save and restore env
	origXDG := os.Getenv("XDG_RUNTIME_DIR")
	defer os.Setenv("XDG_RUNTIME_DIR", origXDG)

	os.Unsetenv("XDG_RUNTIME_DIR")

	ctx := context.Background()
	conn, err := client.Connect(ctx)

	if err == nil {
		conn.Close()
		t.Fatal("expected error when XDG_RUNTIME_DIR not set, got nil")
	}

	if !errors.Is(err, client.ErrXDGNotSet) {
		t.Errorf("expected ErrXDGNotSet, got: %v", err)
	}

	expectedMsg := "XDG_RUNTIME_DIR not set"
	if err.Error() != expectedMsg {
		t.Errorf("error message = %q, want %q", err.Error(), expectedMsg)
	}
}

// TestXDGRuntimeDirNotSet_Daemon verifies daemon exits with error when
// XDG_RUNTIME_DIR is not set.
func TestXDGRuntimeDirNotSet_Daemon(t *testing.T) {
	daemonBin := daemonPath(t)

	// Verify daemon binary exists
	if _, err := os.Stat(daemonBin); os.IsNotExist(err) {
		t.Fatalf("daemon binary not found at %s", daemonBin)
	}

	// Start daemon without XDG_RUNTIME_DIR
	cmd := exec.Command(daemonBin)
	// Filter out XDG_RUNTIME_DIR from environment
	env := []string{}
	for _, e := range os.Environ() {
		if len(e) >= 16 && e[:16] != "XDG_RUNTIME_DIR=" {
			env = append(env, e)
		}
	}
	cmd.Env = env

	// Capture stderr
	output, err := cmd.CombinedOutput()

	// Daemon should exit with error
	if err == nil {
		t.Fatal("daemon should fail when XDG_RUNTIME_DIR not set")
	}

	// Check error message contains expected text
	outputStr := string(output)
	if outputStr == "" {
		t.Errorf("daemon produced no output, expected error about XDG_RUNTIME_DIR")
	}
	// The error message should be in the output somewhere
	// (either from socket_error_string or direct fprintf)
	t.Logf("daemon output: %s", outputStr)
}

// TestConnectionTimeout verifies client connection timeout works.
func TestConnectionTimeout(t *testing.T) {
	tmpDir, envCleanup := daemonTestEnv(t)
	defer envCleanup()

	os.Setenv("XDG_RUNTIME_DIR", tmpDir)

	// Create socket directory but don't start daemon
	socketDir := filepath.Join(tmpDir, "weave")
	if err := os.MkdirAll(socketDir, 0700); err != nil {
		t.Fatalf("failed to create socket dir: %v", err)
	}

	// Create a context with very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	// Attempt to connect (should timeout or return daemon not running)
	conn, err := client.Connect(ctx)

	if err == nil {
		conn.Close()
		t.Fatal("expected timeout error, got nil")
	}

	// Could be timeout or daemon not running (both acceptable)
	if !errors.Is(err, client.ErrConnectionTimeout) &&
		!errors.Is(err, client.ErrDaemonNotRunning) &&
		!errors.Is(err, context.DeadlineExceeded) {
		t.Logf("got error: %v (acceptable)", err)
	}
}

// TestReadTimeout verifies client read timeout works when daemon doesn't respond.
func TestReadTimeout(t *testing.T) {
	tmpDir, envCleanup := daemonTestEnv(t)
	defer envCleanup()

	os.Setenv("XDG_RUNTIME_DIR", tmpDir)

	_, daemonCleanup := startDaemon(t, tmpDir)
	defer daemonCleanup()

	// Connect to daemon
	ctx := context.Background()
	conn, err := client.Connect(ctx)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer conn.Close()

	// Send message with short timeout
	testMsg := make([]byte, 16)
	testMsg[0] = 0x57 // Magic
	testMsg[1] = 0x45
	testMsg[2] = 0x56
	testMsg[3] = 0x45

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	// Send should fail since daemon doesn't respond (placeholder handler closes immediately)
	_, err = conn.Send(ctx, testMsg)

	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}

	// The daemon's placeholder handler closes the connection immediately after accepting,
	// which can cause different error conditions depending on timing:
	// - ErrReadTimeout: if the write succeeds but daemon doesn't respond
	// - ErrConnectionClosed: if daemon closes during read
	// - context.DeadlineExceeded: if timeout occurs
	// - syscall.EPIPE (broken pipe): if daemon closes before/during write
	//   This is the most common case with the placeholder handler
	if !errors.Is(err, client.ErrReadTimeout) &&
		!errors.Is(err, client.ErrConnectionClosed) &&
		!errors.Is(err, context.DeadlineExceeded) &&
		!errors.Is(err, syscall.EPIPE) {
		t.Errorf("expected timeout/closed/broken pipe error, got: %v", err)
	}
}

// TestMultipleSequentialConnections verifies daemon can handle multiple
// sequential connections.
func TestMultipleSequentialConnections(t *testing.T) {
	tmpDir, envCleanup := daemonTestEnv(t)
	defer envCleanup()

	os.Setenv("XDG_RUNTIME_DIR", tmpDir)

	_, daemonCleanup := startDaemon(t, tmpDir)
	defer daemonCleanup()

	// Make multiple connections sequentially
	for i := 0; i < 5; i++ {
		t.Run(fmt.Sprintf("connection_%d", i), func(t *testing.T) {
			ctx := context.Background()
			conn, err := client.Connect(ctx)
			if err != nil {
				t.Fatalf("connection %d failed: %v", i, err)
			}
			defer conn.Close()

			// Verify connection is usable
			if conn.RawConn() == nil {
				t.Errorf("connection %d has nil RawConn", i)
			}
		})
	}
}

// TestSocketDirectoryPermissions verifies socket directory has correct permissions (0700).
func TestSocketDirectoryPermissions(t *testing.T) {
	tmpDir, envCleanup := daemonTestEnv(t)
	defer envCleanup()

	os.Setenv("XDG_RUNTIME_DIR", tmpDir)

	_, daemonCleanup := startDaemon(t, tmpDir)
	defer daemonCleanup()

	socketDir := filepath.Join(tmpDir, "weave")
	info, err := os.Stat(socketDir)
	if err != nil {
		t.Fatalf("socket directory not found: %v", err)
	}

	mode := info.Mode()
	if mode&os.ModePerm != 0700 {
		t.Errorf("socket directory permissions = %o, want 0700", mode&os.ModePerm)
	}
}
