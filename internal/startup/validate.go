// Package startup provides startup validation and initialization for weave.
//
// It validates that required dependencies (ollama, weave-compute) are available
// before the application starts accepting requests.
package startup

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/hurricanerix/weave/internal/client"
)

var (
	// ErrOllamaNotRunning is returned when ollama is not reachable
	ErrOllamaNotRunning = errors.New("ollama not running")
	// ErrComputeNotRunning is returned when weave-compute socket doesn't exist
	ErrComputeNotRunning = errors.New("weave-compute not running (socket not found)")
	// ErrComputeNotAccepting is returned when weave-compute refuses connections
	ErrComputeNotAccepting = errors.New("weave-compute not accepting connections")
)

const (
	// ollamaTimeout is the timeout for ollama validation request
	ollamaTimeout = 5 * time.Second
	// computeTimeout is the timeout for compute daemon connection test
	computeTimeout = 5 * time.Second
)

// ValidateOllama checks if ollama is running and reachable at the given URL.
// It sends an HTTP GET request to the /api/tags endpoint.
// Returns nil if ollama is reachable, ErrOllamaNotRunning otherwise.
func ValidateOllama(baseURL string) error {
	ctx, cancel := context.WithTimeout(context.Background(), ollamaTimeout)
	defer cancel()

	// SECURITY: Parse and validate base URL to prevent SSRF
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return fmt.Errorf("%w: invalid URL: %v", ErrOllamaNotRunning, err)
	}

	// SECURITY: Validate scheme is http or https only
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return fmt.Errorf("%w: URL must use http or https scheme, got: %s", ErrOllamaNotRunning, parsedURL.Scheme)
	}

	// SECURITY: Validate host is not empty
	if parsedURL.Host == "" {
		return fmt.Errorf("%w: URL must have a host", ErrOllamaNotRunning)
	}

	// SECURITY: Construct tags endpoint properly (clear path, query, fragment to prevent injection)
	parsedURL.Path = "/api/tags"
	parsedURL.RawQuery = ""
	parsedURL.Fragment = ""
	tagsURL := parsedURL.String()

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, tagsURL, nil)
	if err != nil {
		return fmt.Errorf("%w at %s: failed to create request: %v", ErrOllamaNotRunning, baseURL, err)
	}

	// Send request
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		// Check if connection was refused
		var netErr *net.OpError
		if errors.As(err, &netErr) {
			if errors.Is(netErr.Err, syscall.ECONNREFUSED) {
				return fmt.Errorf("%w at %s", ErrOllamaNotRunning, baseURL)
			}
		}
		// Check for timeout
		if errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("%w at %s: connection timeout", ErrOllamaNotRunning, baseURL)
		}
		return fmt.Errorf("%w at %s: %v", ErrOllamaNotRunning, baseURL, err)
	}
	defer resp.Body.Close()

	// Any 2xx response is considered success
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	return fmt.Errorf("%w at %s: unexpected status code %d", ErrOllamaNotRunning, baseURL, resp.StatusCode)
}

// ValidateCompute checks if weave-compute daemon is running and accepting connections.
// It checks that the socket file exists and can be connected to.
// Returns nil if daemon is available, error otherwise.
func ValidateCompute() error {
	socketPath, err := GetSocketPath()
	if err != nil {
		return err
	}

	// Check if socket file exists
	if _, err := os.Stat(socketPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%w at %s", ErrComputeNotRunning, socketPath)
		}
		return fmt.Errorf("failed to check socket: %w", err)
	}

	// Try to connect to verify daemon is accepting connections
	ctx, cancel := context.WithTimeout(context.Background(), computeTimeout)
	defer cancel()

	conn, err := client.Connect(ctx)
	if err != nil {
		// Check for specific error types
		if errors.Is(err, client.ErrDaemonNotRunning) {
			return fmt.Errorf("%w at %s", ErrComputeNotRunning, socketPath)
		}
		if errors.Is(err, client.ErrDaemonNotAccepting) {
			return fmt.Errorf("%w", ErrComputeNotAccepting)
		}
		return fmt.Errorf("failed to connect to compute daemon: %w", err)
	}
	defer conn.Close()

	return nil
}

// GetSocketPath returns the path to the weave-compute socket.
// It uses $XDG_RUNTIME_DIR/weave/weave.sock.
func GetSocketPath() (string, error) {
	xdgRuntimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if xdgRuntimeDir == "" {
		return "", client.ErrXDGNotSet
	}

	// SECURITY: Validate XDG_RUNTIME_DIR is an absolute path
	if !filepath.IsAbs(xdgRuntimeDir) {
		return "", errors.New("XDG_RUNTIME_DIR must be an absolute path")
	}

	// SECURITY: Clean path to remove .. and .
	xdgRuntimeDir = filepath.Clean(xdgRuntimeDir)

	return filepath.Join(xdgRuntimeDir, socketDir, socketName), nil
}
