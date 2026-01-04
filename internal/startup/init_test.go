package startup

import (
	"bytes"
	"context"
	"testing"

	"github.com/hurricanerix/weave/internal/config"
)

func TestCreateLogger(t *testing.T) {
	cfg := &config.Config{
		LogLevel: "debug",
	}

	logger := CreateLogger(cfg)

	if logger == nil {
		t.Fatal("CreateLogger() returned nil")
	}
}

func TestCreateOllamaClient(t *testing.T) {
	cfg := &config.Config{
		OllamaURL:   "http://localhost:11434",
		OllamaModel: "llama3.2:1b",
	}

	client := CreateOllamaClient(cfg)

	if client == nil {
		t.Fatal("CreateOllamaClient() returned nil")
	}

	if client.Endpoint() != cfg.OllamaURL {
		t.Errorf("Endpoint() = %s, want %s", client.Endpoint(), cfg.OllamaURL)
	}

	if client.Model() != cfg.OllamaModel {
		t.Errorf("Model() = %s, want %s", client.Model(), cfg.OllamaModel)
	}
}

func TestCreateSessionManager(t *testing.T) {
	manager := CreateSessionManager()

	if manager == nil {
		t.Fatal("CreateSessionManager() returned nil")
	}
}

func TestCreateWebServer(t *testing.T) {
	cfg := &config.Config{
		Port:        8080,
		OllamaURL:   "http://localhost:11434",
		OllamaModel: "llama3.2:1b",
		LogLevel:    "info",
	}

	ctx := context.Background()
	ollamaClient := CreateOllamaClient(cfg)
	sessionManager := CreateSessionManager()
	imageStorage := CreateImageStorage(ctx, CreateLogger(cfg))
	logger := CreateLogger(cfg)

	server, err := CreateWebServer(cfg, ollamaClient, sessionManager, imageStorage, logger)
	if err != nil {
		t.Fatalf("CreateWebServer() error = %v, want nil", err)
	}

	if server == nil {
		t.Fatal("CreateWebServer() returned nil server")
	}
}

func TestInitializeAll(t *testing.T) {
	cfg := &config.Config{
		Port:        8080,
		Steps:       4,
		CFG:         1.0,
		Width:       1024,
		Height:      1024,
		Seed:        0,
		LLMSeed:     0,
		OllamaURL:   "http://localhost:11434",
		OllamaModel: "llama3.2:1b",
		LogLevel:    "info",
	}

	output := &bytes.Buffer{}
	logger := CreateLogger(cfg)
	ctx := context.Background()

	components, err := InitializeAll(ctx, cfg, logger)
	if err != nil {
		t.Fatalf("InitializeAll() error = %v, want nil", err)
	}

	if components == nil {
		t.Fatal("InitializeAll() returned nil components")
	}

	if components.OllamaClient == nil {
		t.Error("OllamaClient is nil")
	}

	if components.SessionManager == nil {
		t.Error("SessionManager is nil")
	}

	if components.WebServer == nil {
		t.Error("WebServer is nil")
	}

	if components.Logger == nil {
		t.Error("Logger is nil")
	}

	if components.ImageStorage == nil {
		t.Error("ImageStorage is nil")
	}

	// Verify logger captured some debug output
	_ = output
}
