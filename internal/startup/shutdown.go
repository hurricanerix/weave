package startup

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hurricanerix/weave/internal/logging"
	"github.com/hurricanerix/weave/internal/web"
)

const (
	// shutdownTimeout is the maximum time to wait for graceful shutdown
	shutdownTimeout = 30 * time.Second
	// computeStdinTimeout is time to wait after closing stdin before sending SIGTERM
	computeStdinTimeout = 500 * time.Millisecond
	// computeSigtermTimeout is time to wait after SIGTERM before sending SIGKILL
	computeSigtermTimeout = 1 * time.Second
	// computeSigkillTimeout is time to wait after SIGKILL for process to die
	computeSigkillTimeout = 500 * time.Millisecond
)

// CleanupCompute terminates the compute daemon process and cleans up resources.
// It performs the following steps:
//  1. Close stdin pipe to signal compute to shutdown
//  2. Wait up to 1 second for graceful exit
//  3. Send SIGTERM if still running, wait 1 second
//  4. Send SIGKILL if still running, wait 1 second
//  5. Close the listening socket
//  6. Remove the socket file from filesystem
//
// Errors during cleanup are logged but do not cause the function to fail.
// This ensures cleanup proceeds even if individual steps fail.
//
// If components is nil or has no compute process, this is a no-op.
func CleanupCompute(components *Components, logger *logging.Logger) {
	if components == nil {
		return
	}

	// Check if compute was started
	if components.ComputeProcess == nil {
		return
	}

	logger.Debug("Starting compute cleanup")

	// Step 1: Close stdin to signal graceful shutdown
	if components.ComputeStdin != nil {
		logger.Debug("Closing compute stdin to signal shutdown")
		if err := components.ComputeStdin.Close(); err != nil {
			logger.Error("Failed to close compute stdin: %v", err)
		}
	}

	// Step 2: Wait for graceful shutdown with timeout
	// Use buffered channel to prevent goroutine leak if we don't read from it
	done := make(chan error, 1)
	go func() {
		// Wait() may block forever if already called elsewhere, but the buffered
		// channel ensures this goroutine won't leak - it will send and exit.
		done <- components.ComputeProcess.Wait()
	}()

	var processExited bool

	// Try stdin close first
	select {
	case err := <-done:
		processExited = true
		if err != nil {
			logger.Debug("Compute process exited with error: %v", err)
		} else {
			logger.Debug("Compute process exited cleanly after stdin close")
		}
	case <-time.After(computeStdinTimeout):
		logger.Debug("Compute process did not exit within %v after stdin close", computeStdinTimeout)
	}

	// Step 3: Send SIGTERM if still running
	if !processExited && components.ComputeProcess.Process != nil {
		logger.Debug("Sending SIGTERM to compute process")
		if err := components.ComputeProcess.Process.Signal(syscall.SIGTERM); err != nil {
			logger.Error("Failed to send SIGTERM: %v", err)
		} else {
			select {
			case err := <-done:
				processExited = true
				if err != nil {
					logger.Debug("Compute process exited with error after SIGTERM: %v", err)
				} else {
					logger.Debug("Compute process exited cleanly after SIGTERM")
				}
			case <-time.After(computeSigtermTimeout):
				logger.Debug("Compute process did not exit within %v after SIGTERM", computeSigtermTimeout)
			}
		}
	}

	// Step 4: Send SIGKILL if still running
	if !processExited && components.ComputeProcess.Process != nil {
		logger.Debug("Sending SIGKILL to compute process")
		if err := components.ComputeProcess.Process.Kill(); err != nil {
			logger.Error("Failed to send SIGKILL: %v", err)
		} else {
			// Use non-blocking select with timeout to avoid hanging if goroutine is stuck
			select {
			case err := <-done:
				if err != nil {
					logger.Debug("Compute process exited with error after SIGKILL: %v", err)
				} else {
					logger.Debug("Compute process killed successfully")
				}
			case <-time.After(computeSigkillTimeout):
				// Process didn't exit even after SIGKILL - this is very unusual
				// Log and continue cleanup anyway
				logger.Error("Compute process did not exit within %v after SIGKILL", computeSigkillTimeout)
			}
		}
	}

	// Step 5: Close the listening socket
	if components.ComputeListener != nil {
		logger.Debug("Closing compute listener socket")
		if err := components.ComputeListener.Close(); err != nil {
			logger.Error("Failed to close compute listener: %v", err)
		}
	}

	// Step 6: Remove socket file from filesystem
	if components.ComputeSocketPath != "" {
		logger.Debug("Removing socket file: %s", components.ComputeSocketPath)
		if err := os.Remove(components.ComputeSocketPath); err != nil {
			if !os.IsNotExist(err) {
				logger.Error("Failed to remove socket file: %v", err)
			}
		}
	}

	logger.Debug("Compute cleanup complete")
}

// Run starts the web server and blocks until a shutdown signal is received.
// It handles SIGTERM and SIGINT signals for graceful shutdown.
//
// Parameters:
//   - ctx: Context for server lifecycle (cancellation triggers shutdown)
//   - server: Web server to run
//   - logger: Logger for shutdown messages
//
// Returns nil on clean shutdown, error otherwise.
func Run(ctx context.Context, server *web.Server, logger *logging.Logger) error {
	// Create context that will be cancelled on signal
	shutdownCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Start server and wait for it to finish
	// ListenAndServe blocks until context is cancelled or error occurs
	// The web.Server itself logs "Shutting down..." and "Web server stopped"
	err := server.ListenAndServe(shutdownCtx)
	if err != nil {
		return fmt.Errorf("server error: %w", err)
	}

	return nil
}
