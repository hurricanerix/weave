package startup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/hurricanerix/weave/internal/client"
	"github.com/hurricanerix/weave/internal/config"
	"github.com/hurricanerix/weave/internal/conversation"
	"github.com/hurricanerix/weave/internal/image"
	"github.com/hurricanerix/weave/internal/logging"
	"github.com/hurricanerix/weave/internal/ollama"
	"github.com/hurricanerix/weave/internal/web"
)

const (
	// socketDir is the subdirectory under XDG_RUNTIME_DIR where the socket is created
	socketDir = "weave"
	// socketName is the name of the Unix domain socket file
	socketName = "weave.sock"
)

var (
	// ErrXDGNotAbsolute is returned when XDG_RUNTIME_DIR is not an absolute path
	ErrXDGNotAbsolute = errors.New("XDG_RUNTIME_DIR must be an absolute path")
	// ErrXDGResolvesToRelative is returned when XDG_RUNTIME_DIR resolves to a relative path after cleaning
	ErrXDGResolvesToRelative = errors.New("XDG_RUNTIME_DIR resolves to relative path")
	// ErrComputeBinaryNotFound is returned when the compute binary is not found
	ErrComputeBinaryNotFound = errors.New("compute binary not found")
	// ErrComputeSpawnFailed is returned when spawning the compute process fails
	ErrComputeSpawnFailed = errors.New("failed to spawn compute process")
)

// Components holds all initialized application components
type Components struct {
	OllamaClient      *ollama.Client
	SessionManager    *conversation.SessionManager
	ComputeClient     *client.Conn
	ComputeListener   net.Listener
	ComputeSocketPath string
	ComputeProcess    *exec.Cmd
	ComputeStdin      io.WriteCloser
	WebServer         *web.Server
	Logger            *logging.Logger
	ImageStorage      *image.Storage
}

// CreateSocket creates the Unix socket for weave-compute communication.
// It constructs the socket path from XDG_RUNTIME_DIR, creates the socket
// directory with mode 0700 if it doesn't exist, removes any existing socket
// file, and creates a listening Unix socket.
//
// CALLER MUST CLOSE THE LISTENER when done to avoid resource leaks.
//
// Returns the listener, socket path, and error if XDG_RUNTIME_DIR is not set,
// not absolute, or if socket creation fails.
func CreateSocket() (net.Listener, string, error) {
	// Get XDG_RUNTIME_DIR
	xdgRuntimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if xdgRuntimeDir == "" {
		return nil, "", client.ErrXDGNotSet
	}

	// Validate XDG_RUNTIME_DIR is an absolute path
	if !filepath.IsAbs(xdgRuntimeDir) {
		return nil, "", ErrXDGNotAbsolute
	}

	// Clean path to remove .. and .
	xdgRuntimeDir = filepath.Clean(xdgRuntimeDir)

	// Verify path is still absolute after cleaning
	if !filepath.IsAbs(xdgRuntimeDir) {
		return nil, "", ErrXDGResolvesToRelative
	}

	// SECURITY: Validate cleaned path doesn't contain path traversal attempts
	// After cleaning, the path should not contain .. components
	if filepath.Clean(xdgRuntimeDir) != xdgRuntimeDir {
		return nil, "", fmt.Errorf("XDG_RUNTIME_DIR contains path traversal elements")
	}

	// Construct socket directory path
	sockDir := filepath.Join(xdgRuntimeDir, socketDir)

	// Create socket directory with mode 0700 if it doesn't exist
	if err := os.MkdirAll(sockDir, 0700); err != nil {
		return nil, "", fmt.Errorf("failed to create socket directory: %w", err)
	}

	// Construct full socket path
	socketPath := filepath.Join(sockDir, socketName)

	// Remove any existing socket file (left over from previous crash)
	// Ignore error if file doesn't exist
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		return nil, "", fmt.Errorf("failed to remove existing socket file: %w", err)
	}

	// Create and bind the Unix socket
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create socket: %w", err)
	}

	return listener, socketPath, nil
}

// SpawnCompute spawns the compute process as a child process.
// It passes the socket path via the --socket-path CLI argument and sets up
// stdio pipes for lifecycle monitoring and logging.
//
// The compute process will automatically terminate when:
// - weave closes stdin (indicating parent process death)
// - The socket connection is closed
//
// CALLER MUST:
// 1. Store the returned stdin pipe in Components.ComputeStdin and close it during shutdown
// 2. Call cmd.Wait() to reap the child process and prevent zombies
//
// Closing stdin triggers graceful compute shutdown. After closing stdin, call
// cmd.Wait() to collect the exit status and prevent zombie processes.
//
// Returns the *exec.Cmd and stdin WriteCloser, or error if spawning fails.
func SpawnCompute(socketPath string) (*exec.Cmd, io.WriteCloser, error) {
	// Find the compute binary
	// Try multiple locations to handle both runtime and test contexts
	candidatePaths := []string{
		"compute/weave-compute",           // From project root
		"../compute/weave-compute",        // From backend/
		"../../compute/weave-compute",     // From backend/cmd/weave
		"../../../compute/weave-compute",  // From backend/test/integration/
		"/usr/local/bin/weave-compute",    // System install
		"/usr/bin/weave-compute",          // System install
	}

	// Also check for compute binary next to weave executable (packaged Electron app)
	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		siblingPath := filepath.Join(exeDir, "weave-compute")
		candidatePaths = append([]string{siblingPath}, candidatePaths...)
	}

	var binaryPath string
	for _, path := range candidatePaths {
		if _, err := os.Stat(path); err == nil {
			binaryPath = path
			break
		}
	}

	if binaryPath == "" {
		return nil, nil, fmt.Errorf("%w: tried paths: %v", ErrComputeBinaryNotFound, candidatePaths)
	}

	// Create command with --socket-path argument
	cmd := exec.Command(binaryPath, "--socket-path", socketPath)

	// Set up stdin pipe for lifecycle monitoring
	// When weave dies, stdin will be closed, triggering compute shutdown
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("%w: failed to create stdin pipe: %v", ErrComputeSpawnFailed, err)
	}

	// Connect stdout and stderr to parent's streams for logging
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Start the process
	if err := cmd.Start(); err != nil {
		stdin.Close() // Clean up stdin pipe on failure
		return nil, nil, fmt.Errorf("%w: %v", ErrComputeSpawnFailed, err)
	}

	return cmd, stdin, nil
}

// CreateLogger creates a logger with the configured log level
func CreateLogger(cfg *config.Config) *logging.Logger {
	return logging.NewFromString(cfg.LogLevel, nil)
}

// CreateOllamaClient creates an ollama client with the configured URL and model.
// It does NOT validate connection - use ValidateOllama() separately.
func CreateOllamaClient(cfg *config.Config) *ollama.Client {
	return ollama.NewClientWithConfig(cfg.OllamaURL, cfg.OllamaModel, 60*time.Second)
}

// CreateSessionManager creates a session manager for conversation state
func CreateSessionManager() *conversation.SessionManager {
	return conversation.NewSessionManager()
}

// CreateImageStorage creates image storage and starts cleanup goroutine
func CreateImageStorage(ctx context.Context, logger *logging.Logger) *image.Storage {
	storage := image.NewStorage()
	storage.StartCleanup(ctx, logger)
	return storage
}

// CreateWebServer creates the HTTP server with all dependencies wired
func CreateWebServer(cfg *config.Config, ollamaClient *ollama.Client, sessionManager *conversation.SessionManager, imageStorage *image.Storage, computeClient *client.Conn, logger *logging.Logger) (*web.Server, error) {
	addr := fmt.Sprintf("localhost:%d", cfg.Port)

	// Create server with dependencies including config for default generation settings
	server, err := web.NewServerWithDeps(addr, ollamaClient, sessionManager, imageStorage, computeClient, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create web server: %w", err)
	}

	return server, nil
}

// InitializeAll creates and initializes all application components.
// It does NOT validate dependencies - validation should be done separately.
//
// Parameters:
//   - ctx: Context for component initialization
//   - cfg: Configuration
//   - logger: Logger instance
//   - computeClient: Connection to compute process (from AcceptConnection)
func InitializeAll(ctx context.Context, cfg *config.Config, logger *logging.Logger, computeClient *client.Conn) (*Components, error) {
	logger.Debug("Initializing components")

	// Create ollama client
	ollamaClient := CreateOllamaClient(cfg)
	logger.Debug("Created ollama client: endpoint=%s, model=%s", cfg.OllamaURL, cfg.OllamaModel)

	// Create session manager
	sessionManager := CreateSessionManager()
	logger.Debug("Created session manager")

	// Create image storage with cleanup goroutine
	imageStorage := CreateImageStorage(ctx, logger)
	logger.Debug("Created image storage with cleanup enabled")

	// Create web server with compute client
	webServer, err := CreateWebServer(cfg, ollamaClient, sessionManager, imageStorage, computeClient, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create web server: %w", err)
	}
	logger.Debug("Created web server on port %d", cfg.Port)

	return &Components{
		OllamaClient:      ollamaClient,
		SessionManager:    sessionManager,
		ComputeClient:     computeClient,
		ComputeListener:   nil, // Set by caller
		ComputeSocketPath: "",  // Set by caller
		ComputeProcess:    nil, // Set by caller
		ComputeStdin:      nil, // Set by caller
		WebServer:         webServer,
		Logger:            logger,
		ImageStorage:      imageStorage,
	}, nil
}
