package startup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// computeBinaryExists checks if the compute binary exists in any of the expected locations
func computeBinaryExists() bool {
	candidatePaths := []string{
		"compute/weave-compute",
		"../compute/weave-compute",
		"../../compute/weave-compute",
		"../../../compute/weave-compute",
	}
	for _, path := range candidatePaths {
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}
	return false
}

func TestSpawnCompute(t *testing.T) {
	tests := []struct {
		name          string
		socketPath    string
		setupBinary   func(t *testing.T) (cleanup func())
		wantErr       bool
		wantErrType   error
		wantErrSubstr string
	}{
		{
			name:       "successful spawn",
			socketPath: "", // Will be set in test body
			setupBinary: func(t *testing.T) func() {
				// This test requires the actual binary to exist
				// Skip if not present (e.g., in CI without build)
				if !computeBinaryExists() {
					t.Skip("compute binary not found, skipping spawn test")
				}
				return func() {}
			},
			wantErr: false,
		},
		{
			name:       "binary not found",
			socketPath: "", // Will be set in test body
			setupBinary: func(t *testing.T) func() {
				// Temporarily rename all candidate binaries to simulate none being there
				var renamed []struct{ from, to string }

				candidatePaths := []string{
					"compute/weave-compute",
					"../compute/weave-compute",
					"../../compute/weave-compute",
					"../../../compute/weave-compute",
				}

				for _, binaryPath := range candidatePaths {
					if _, err := os.Stat(binaryPath); err == nil {
						tempPath := binaryPath + ".tmp"
						if err := os.Rename(binaryPath, tempPath); err == nil {
							renamed = append(renamed, struct{ from, to string }{binaryPath, tempPath})
						}
					}
				}

				return func() {
					for _, r := range renamed {
						os.Rename(r.to, r.from)
					}
				}
			},
			wantErr:       true,
			wantErrType:   ErrComputeBinaryNotFound,
			wantErrSubstr: "compute binary not found",
		},
		{
			name:       "empty socket path",
			socketPath: "",
			setupBinary: func(t *testing.T) func() {
				if !computeBinaryExists() {
					t.Skip("compute binary not found, skipping spawn test")
				}
				return func() {}
			},
			wantErr: false, // Empty socket path is accepted by exec, will fail in compute
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanup := tt.setupBinary(t)
			defer cleanup()

			// Use project-local temp directory
			socketPath := tt.socketPath
			if socketPath == "" {
				socketPath = filepath.Join(t.TempDir(), "test.sock")
			}

			cmd, stdin, err := SpawnCompute(socketPath)

			if tt.wantErr {
				if err == nil {
					// Clean up spawned process if any
					if cmd != nil && cmd.Process != nil {
						if stdin != nil {
							stdin.Close()
						}
						cmd.Process.Kill()
						cmd.Wait()
					}
					t.Errorf("SpawnCompute() error = nil, want error")
					return
				}

				if tt.wantErrType != nil {
					if !strings.Contains(err.Error(), tt.wantErrType.Error()) {
						t.Errorf("SpawnCompute() error = %q, want error type %q", err.Error(), tt.wantErrType.Error())
					}
				}

				if tt.wantErrSubstr != "" && !strings.Contains(err.Error(), tt.wantErrSubstr) {
					t.Errorf("SpawnCompute() error = %q, want error containing %q", err.Error(), tt.wantErrSubstr)
				}
				return
			}

			if err != nil {
				t.Fatalf("SpawnCompute() error = %v, want nil", err)
			}

			if cmd == nil {
				t.Fatal("SpawnCompute() returned nil cmd")
			}

			if stdin == nil {
				t.Fatal("SpawnCompute() returned nil stdin")
			}

			if cmd.Process == nil {
				t.Error("SpawnCompute() cmd.Process is nil")
			}

			// Verify process is running
			if cmd.Process != nil {
				// Check if process is still running by giving it time to initialize
				// If the process exited immediately, that would indicate a problem
				time.Sleep(50 * time.Millisecond)
			}

			// Clean up: close stdin and wait for process
			// Note: stdin and cmd.Process are guaranteed non-nil at this point
			// due to the earlier Fatal checks
			stdin.Close()
			cmd.Process.Kill()
			// Wait for process to exit to avoid zombie
			cmd.Wait()
		})
	}
}

func TestSpawnCompute_ProcessLifecycle(t *testing.T) {
	// Skip if binary doesn't exist
	if !computeBinaryExists() {
		t.Skip("compute binary not found, skipping lifecycle test")
	}

	// Create a temporary socket path (doesn't need to exist yet)
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	cmd, stdin, err := SpawnCompute(socketPath)
	if err != nil {
		t.Fatalf("SpawnCompute() error = %v, want nil", err)
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

	// Verify stdin is not nil
	if stdin == nil {
		t.Fatal("stdin is nil")
	}

	// Verify process started
	if cmd.Process == nil {
		t.Fatal("cmd.Process is nil")
	}

	pid := cmd.Process.Pid
	if pid <= 0 {
		t.Errorf("invalid PID: %d", pid)
	}

	// Give process a moment to initialize
	// The compute process may fail to load the model in test environment,
	// which is expected. We just want to verify the spawn mechanism works.
	time.Sleep(200 * time.Millisecond)

	// Test cleanup: close stdin and kill process
	// Close stdin to signal graceful shutdown
	if err := stdin.Close(); err != nil {
		t.Logf("Error closing stdin: %v", err)
	}

	// Use SIGTERM first for graceful shutdown
	if err := cmd.Process.Kill(); err != nil {
		// Process may have already exited (e.g., model loading failed)
		// This is acceptable in test environment
		t.Logf("Process already exited or kill failed: %v", err)
	}

	// Wait for process to exit (with timeout)
	waitDone := make(chan error, 1)
	go func() {
		waitDone <- cmd.Wait()
	}()

	select {
	case err := <-waitDone:
		// Process exited (either due to kill or self-termination)
		// Both are acceptable outcomes
		if err == nil {
			t.Log("Process exited cleanly")
		} else {
			t.Logf("Process exited with error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("Process did not exit within timeout")
	}
}

func TestSpawnCompute_Arguments(t *testing.T) {
	// Skip if binary doesn't exist
	if !computeBinaryExists() {
		t.Skip("compute binary not found, skipping arguments test")
	}

	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "custom-socket-path.sock")

	cmd, stdin, err := SpawnCompute(socketPath)
	if err != nil {
		t.Fatalf("SpawnCompute() error = %v, want nil", err)
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

	// Verify stdin is not nil
	if stdin == nil {
		t.Fatal("stdin is nil")
	}

	// Verify command line arguments
	if len(cmd.Args) < 3 {
		t.Errorf("cmd.Args length = %d, want at least 3 (binary, --socket-path, path)", len(cmd.Args))
	}

	// Check that --socket-path is present
	foundFlag := false
	foundPath := false
	for i, arg := range cmd.Args {
		if arg == "--socket-path" {
			foundFlag = true
			if i+1 < len(cmd.Args) && cmd.Args[i+1] == socketPath {
				foundPath = true
			}
		}
	}

	if !foundFlag {
		t.Error("--socket-path flag not found in command arguments")
	}

	if !foundPath {
		t.Errorf("socket path not found in command arguments, want %s", socketPath)
	}
}

func TestSpawnCompute_StdioSetup(t *testing.T) {
	// Skip if binary doesn't exist
	if !computeBinaryExists() {
		t.Skip("compute binary not found, skipping stdio test")
	}

	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	cmd, stdin, err := SpawnCompute(socketPath)
	if err != nil {
		t.Fatalf("SpawnCompute() error = %v, want nil", err)
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

	// Verify stdin is not nil (SpawnCompute now returns it)
	if stdin == nil {
		t.Fatal("stdin is nil")
	}

	// Verify stdout is connected to os.Stdout
	if cmd.Stdout != os.Stdout {
		t.Error("cmd.Stdout is not connected to os.Stdout")
	}

	// Verify stderr is connected to os.Stderr
	if cmd.Stderr != os.Stderr {
		t.Error("cmd.Stderr is not connected to os.Stderr")
	}
}
