package main

import (
	"context"
	"errors"
	"fmt"
	"os"

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

	// Validate weave-compute is running
	logger.Debug("Validating weave-compute connection...")
	if err := startup.ValidateCompute(); err != nil {
		logger.Error("Compute validation failed: %v", err)
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		fmt.Fprintf(os.Stderr, "\nPlease ensure weave-compute daemon is running.\n")
		fmt.Fprintf(os.Stderr, "See docs/DEVELOPMENT.md for setup instructions.\n")
		return 1
	}

	// Get socket path for logging
	socketPath, err := startup.GetSocketPath()
	if err != nil {
		logger.Error("Failed to get socket path: %v", err)
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	logger.Info("Connected to weave-compute at %s", socketPath)

	// Initialize all components
	logger.Debug("Initializing components...")
	ctx := context.Background()
	components, err := startup.InitializeAll(ctx, cfg, logger)
	if err != nil {
		logger.Error("Initialization failed: %v", err)
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

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
