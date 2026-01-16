package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/hurricanerix/weave/internal/client"
	"github.com/hurricanerix/weave/internal/config"
	"github.com/hurricanerix/weave/internal/logging"
	"github.com/hurricanerix/weave/internal/startup"
)

func main() {
	os.Exit(run())
}

// monitorStdin monitors the provided reader for EOF and cancels the context when detected.
// This is used to detect parent process death when running as a child process.
// When stdin reaches EOF (parent died), the context is cancelled to trigger graceful shutdown.
//
// This function blocks until EOF is reached or an error occurs.
func monitorStdin(cancel context.CancelFunc, stdin io.Reader, logger *logging.Logger) {
	buf := make([]byte, 32)
	for {
		_, err := stdin.Read(buf)
		if err == io.EOF {
			logger.Info("Parent process died, initiating shutdown")
			cancel()
			return
		}
		if err != nil {
			// Non-EOF error means stdin is broken but parent may still be alive.
			// Don't trigger shutdown - let normal shutdown mechanisms handle it.
			logger.Debug("Error reading stdin: %v", err)
			return
		}
		// Data received on stdin is unexpected when running under Electron,
		// but not an error. Continue monitoring for EOF.
	}
}

func run() int {
	// Parse configuration from CLI flags
	cfg, err := config.Parse(os.Args[1:], os.Stderr)
	if errors.Is(err, config.ErrShowHelp) || errors.Is(err, config.ErrShowVersion) {
		// Help or version was shown, exit successfully
		return 0
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	// Create logger early
	logger := startup.CreateLogger(cfg)

	// Log startup
	logger.Info("Starting weave...")
	logger.Debug("Configuration: port=%d, steps=%d, cfg=%.1f, width=%d, height=%d, seed=%d, llm-seed=%d",
		cfg.Port, cfg.Steps, cfg.CFG, cfg.Width, cfg.Height, cfg.Seed, cfg.LLMSeed)
	logger.Debug("Ollama: url=%s, model=%s", cfg.OllamaURL, cfg.OllamaModel)
	logger.Debug("Log level: %s", cfg.LogLevel)

	// Validate ollama is running
	logger.Debug("Validating ollama connection...")
	if err := startup.ValidateOllama(cfg.OllamaURL); err != nil {
		logger.Error("Ollama validation failed: %v", err)
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		fmt.Fprintf(os.Stderr, "\nPlease ensure ollama is running:\n")
		fmt.Fprintf(os.Stderr, "  ollama serve\n")
		fmt.Fprintf(os.Stderr, "\nAnd that the model is available:\n")
		fmt.Fprintf(os.Stderr, "  ollama pull %s\n", cfg.OllamaModel)
		return 1
	}
	logger.Info("Connected to ollama at %s (model: %s)", cfg.OllamaURL, cfg.OllamaModel)

	// Create socket for weave-compute communication
	logger.Debug("Creating socket for weave-compute...")
	listener, socketPath, err := startup.CreateSocket()
	if err != nil {
		logger.Error("Failed to create socket: %v", err)
		fmt.Fprintf(os.Stderr, "Error: failed to create socket: %v\n", err)
		return 1
	}
	defer listener.Close()
	logger.Info("Created socket at %s", socketPath)

	// Spawn compute process
	logger.Debug("Spawning weave-compute process...")
	computeProcess, computeStdin, err := startup.SpawnCompute(socketPath)
	if err != nil {
		logger.Error("Failed to spawn compute process: %v", err)
		fmt.Fprintf(os.Stderr, "Error: failed to spawn compute process: %v\n", err)
		fmt.Fprintf(os.Stderr, "\nEnsure the compute binary is available.\n")
		fmt.Fprintf(os.Stderr, "See docs/DEVELOPMENT.md for build instructions.\n")
		return 1
	}
	logger.Info("Spawned weave-compute process (PID: %d)", computeProcess.Process.Pid)

	// Accept connection from compute process
	logger.Debug("Waiting for compute process to connect...")

	// Create cancellable context for server lifecycle
	// This context will be cancelled by signal handlers or stdin EOF detection
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start stdin monitoring for orphan process detection
	// When parent process dies, stdin EOF triggers graceful shutdown
	go monitorStdin(cancel, os.Stdin, logger)

	acceptCtx, acceptCancel := context.WithTimeout(ctx, 10*time.Second)
	defer acceptCancel()

	computeConn, err := client.AcceptConnection(acceptCtx, listener)
	if err != nil {
		logger.Error("Failed to accept compute connection: %v", err)
		fmt.Fprintf(os.Stderr, "Error: failed to accept compute connection: %v\n", err)
		return 1
	}
	logger.Info("Accepted connection from weave-compute process")

	// Initialize all components
	logger.Debug("Initializing components...")
	components, err := startup.InitializeAll(ctx, cfg, logger, computeConn)
	if err != nil {
		logger.Error("Initialization failed: %v", err)
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	// Set compute-specific fields on components
	components.ComputeListener = listener
	components.ComputeSocketPath = socketPath
	components.ComputeProcess = computeProcess
	components.ComputeStdin = computeStdin

	defer startup.CleanupCompute(components, logger)

	// Log server startup
	logger.Info("Listening on http://localhost:%d", cfg.Port)

	// Run server and wait for shutdown signal
	if err := startup.Run(ctx, components.WebServer, logger); err != nil {
		logger.Error("Server error: %v", err)
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	return 0
}
