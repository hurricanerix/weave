package startup

import (
	"context"
	"fmt"
	"time"

	"github.com/hurricanerix/weave/internal/client"
	"github.com/hurricanerix/weave/internal/config"
	"github.com/hurricanerix/weave/internal/conversation"
	"github.com/hurricanerix/weave/internal/image"
	"github.com/hurricanerix/weave/internal/logging"
	"github.com/hurricanerix/weave/internal/ollama"
	"github.com/hurricanerix/weave/internal/web"
)

// Components holds all initialized application components
type Components struct {
	OllamaClient   *ollama.Client
	SessionManager *conversation.SessionManager
	ComputeClient  *client.Conn
	WebServer      *web.Server
	Logger         *logging.Logger
	ImageStorage   *image.Storage
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
func CreateWebServer(cfg *config.Config, ollamaClient *ollama.Client, sessionManager *conversation.SessionManager, imageStorage *image.Storage, logger *logging.Logger) (*web.Server, error) {
	addr := fmt.Sprintf("localhost:%d", cfg.Port)

	// Create server with dependencies
	server, err := web.NewServerWithDeps(addr, ollamaClient, sessionManager, imageStorage)
	if err != nil {
		return nil, fmt.Errorf("failed to create web server: %w", err)
	}

	return server, nil
}

// InitializeAll creates and initializes all application components.
// It does NOT validate dependencies - validation should be done separately.
func InitializeAll(ctx context.Context, cfg *config.Config, logger *logging.Logger) (*Components, error) {
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

	// Create web server
	webServer, err := CreateWebServer(cfg, ollamaClient, sessionManager, imageStorage, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create web server: %w", err)
	}
	logger.Debug("Created web server on port %d", cfg.Port)

	return &Components{
		OllamaClient:   ollamaClient,
		SessionManager: sessionManager,
		ComputeClient:  nil, // ComputeClient is nil - compute connections are made per-request by web handlers
		WebServer:      webServer,
		Logger:         logger,
		ImageStorage:   imageStorage,
	}, nil
}
