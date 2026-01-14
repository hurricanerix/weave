package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/hurricanerix/weave/internal/client"
	"github.com/hurricanerix/weave/internal/config"
	"github.com/hurricanerix/weave/internal/startup"
)

func main() {
	os.Exit(run())
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

	// Create socket for compute daemon communication
	logger.Debug("Creating socket for weave-compute...")
	listener, socketPath, err := startup.CreateSocket()
	if err != nil {
		logger.Error("Failed to create socket: %v", err)
		fmt.Fprintf(os.Stderr, "Error: failed to create socket: %v\n", err)
		return 1
	}
	defer listener.Close()
	logger.Info("Created socket at %s", socketPath)

	// Spawn compute daemon
	logger.Debug("Spawning weave-compute daemon...")
	computeProcess, computeStdin, err := startup.SpawnCompute(socketPath)
	if err != nil {
		logger.Error("Failed to spawn compute daemon: %v", err)
		fmt.Fprintf(os.Stderr, "Error: failed to spawn compute daemon: %v\n", err)
		fmt.Fprintf(os.Stderr, "\nEnsure the compute daemon binary is available.\n")
		fmt.Fprintf(os.Stderr, "See docs/DEVELOPMENT.md for build instructions.\n")
		return 1
	}
	logger.Info("Spawned weave-compute daemon (PID: %d)", computeProcess.Process.Pid)

	// Accept connection from compute daemon
	logger.Debug("Waiting for compute daemon to connect...")
	ctx := context.Background()
	acceptCtx, acceptCancel := context.WithTimeout(ctx, 10*time.Second)
	defer acceptCancel()

	computeConn, err := client.AcceptConnection(acceptCtx, listener)
	if err != nil {
		logger.Error("Failed to accept compute connection: %v", err)
		fmt.Fprintf(os.Stderr, "Error: failed to accept compute connection: %v\n", err)
		return 1
	}
	logger.Info("Accepted connection from weave-compute daemon")

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
