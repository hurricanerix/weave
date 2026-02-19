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

// monitorForOrphanProcesses using the provided reader by monitoring for EOF and cancels the context when detected.
// This is used to detect parent process death when running as a child process.
// When stdin reaches EOF (parent died), the context is cancelled to trigger graceful shutdown.
//
// This function blocks until EOF is reached or an error occurs.
func monitorForOrphanProcesses(cancel context.CancelFunc, stdin io.Reader, logger *logging.Logger) {
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
	cfg, err := config.Parse(os.Args[1:], os.Stderr)
	if errors.Is(err, config.ErrShowHelp) || errors.Is(err, config.ErrShowVersion) {
		return 0
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	logger := startup.CreateLogger(cfg)

	logger.Info("Starting weave...")
	logger.Debug("Configuration: port=%d, steps=%d, cfg=%.1f, width=%d, height=%d, seed=%d, llm-seed=%d",
		cfg.Port, cfg.Steps, cfg.CFG, cfg.Width, cfg.Height, cfg.Seed, cfg.LLMSeed)
	logger.Debug("Ollama: url=%s, model=%s", cfg.OllamaURL, cfg.OllamaModel)
	logger.Debug("Log level: %s", cfg.LogLevel)

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

	logger.Debug("Creating socket for weave-compute...")
	listener, socketPath, err := startup.CreateSocket()
	if err != nil {
		logger.Error("Failed to create socket: %v", err)
		fmt.Fprintf(os.Stderr, "Error: failed to create socket: %v\n", err)
		return 1
	}
	defer listener.Close()
	logger.Info("Created socket at %s", socketPath)

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

	logger.Debug("Waiting for compute process to connect...")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go monitorForOrphanProcesses(cancel, os.Stdin, logger)

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
	components.ComputeClient = computeConn

	defer startup.CleanupCompute(components, logger)

	// Wire compute restart function so the stop-generation endpoint can respawn
	// the compute process after killing it mid-inference.
	components.WebServer.SetComputeRestartFunc(func(restartCtx context.Context) (*client.Conn, error) {
		logger.Info("Restarting compute process...")

		// Kill old compute process.
		if components.ComputeProcess != nil && components.ComputeProcess.Process != nil {
			logger.Debug("Killing old compute process (PID: %d)", components.ComputeProcess.Process.Pid)
			_ = components.ComputeProcess.Process.Kill()
			_ = components.ComputeProcess.Wait() // Reap zombie to avoid resource leak.
		}

		// Close old connection.
		if components.ComputeClient != nil {
			_ = components.ComputeClient.Close()
		}

		// Close old listener and remove socket file.
		if components.ComputeListener != nil {
			_ = components.ComputeListener.Close()
		}
		if components.ComputeSocketPath != "" {
			_ = os.Remove(components.ComputeSocketPath)
		}

		// Create new socket.
		newListener, newSocketPath, err := startup.CreateSocket()
		if err != nil {
			return nil, fmt.Errorf("failed to create socket: %w", err)
		}

		// Spawn new compute process.
		newProcess, newStdin, err := startup.SpawnCompute(newSocketPath)
		if err != nil {
			_ = newListener.Close()
			return nil, fmt.Errorf("failed to spawn compute: %w", err)
		}

		// Accept connection from the new compute process.
		newConn, err := client.AcceptConnection(restartCtx, newListener)
		if err != nil {
			_ = newStdin.Close()
			_ = newProcess.Process.Kill()
			_ = newProcess.Wait()
			_ = newListener.Close()
			return nil, fmt.Errorf("failed to accept connection: %w", err)
		}

		// Update components so the next restart uses the new state.
		components.ComputeListener = newListener
		components.ComputeSocketPath = newSocketPath
		components.ComputeProcess = newProcess
		components.ComputeStdin = newStdin
		components.ComputeClient = newConn

		logger.Info("Compute process restarted (new PID: %d)", newProcess.Process.Pid)
		return newConn, nil
	})

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
