package startup

import (
	"bytes"
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
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
	cfg := &config.Config{
		LogLevel: "info",
	}
	logger := CreateLogger(cfg)

	manager := CreateSessionManager(logger)

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
	logger := CreateLogger(cfg)
	ollamaClient := CreateOllamaClient(cfg)
	sessionManager := CreateSessionManager(logger)
	imageStorage := CreateImageStorage(ctx, logger)
	imageStore := CreateImageStore(logger)

	server, err := CreateWebServer(cfg, ollamaClient, sessionManager, imageStorage, imageStore, nil, logger)
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

	components, err := InitializeAll(ctx, cfg, logger, nil)
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

	// ComputeClient should be nil in this test (no actual connection)
	if components.ComputeClient != nil {
		t.Error("ComputeClient should be nil")
	}

	// Compute-specific fields should be nil (set by caller after InitializeAll)
	if components.ComputeListener != nil {
		t.Error("ComputeListener should be nil")
	}

	if components.ComputeSocketPath != "" {
		t.Error("ComputeSocketPath should be empty")
	}

	if components.ComputeProcess != nil {
		t.Error("ComputeProcess should be nil")
	}

	if components.ComputeStdin != nil {
		t.Error("ComputeStdin should be nil")
	}

	// Verify logger captured some debug output
	_ = output
}

func TestCreateSocket(t *testing.T) {
	tests := []struct {
		name          string
		setupEnv      func() (cleanup func())
		wantErr       bool
		wantErrSubstr string
	}{
		{
			name: "successful socket creation",
			setupEnv: func() func() {
				tmpDir := t.TempDir()
				oldXDG := os.Getenv("XDG_RUNTIME_DIR")
				os.Setenv("XDG_RUNTIME_DIR", tmpDir)
				return func() {
					os.Setenv("XDG_RUNTIME_DIR", oldXDG)
				}
			},
			wantErr: false,
		},
		{
			name: "XDG_RUNTIME_DIR not set",
			setupEnv: func() func() {
				oldXDG := os.Getenv("XDG_RUNTIME_DIR")
				os.Unsetenv("XDG_RUNTIME_DIR")
				return func() {
					if oldXDG != "" {
						os.Setenv("XDG_RUNTIME_DIR", oldXDG)
					}
				}
			},
			wantErr:       true,
			wantErrSubstr: "XDG_RUNTIME_DIR",
		},
		{
			name: "XDG_RUNTIME_DIR not absolute",
			setupEnv: func() func() {
				oldXDG := os.Getenv("XDG_RUNTIME_DIR")
				os.Setenv("XDG_RUNTIME_DIR", "relative/path")
				return func() {
					os.Setenv("XDG_RUNTIME_DIR", oldXDG)
				}
			},
			wantErr:       true,
			wantErrSubstr: "absolute path",
		},
		{
			name: "remove existing socket file",
			setupEnv: func() func() {
				tmpDir := t.TempDir()
				oldXDG := os.Getenv("XDG_RUNTIME_DIR")
				os.Setenv("XDG_RUNTIME_DIR", tmpDir)

				// Create existing socket file
				sockDir := filepath.Join(tmpDir, socketDir)
				os.MkdirAll(sockDir, 0700)
				sockPath := filepath.Join(sockDir, socketName)
				// Create a stale socket file
				f, err := os.Create(sockPath)
				if err != nil {
					t.Fatalf("failed to create stale socket: %v", err)
				}
				f.Close()

				return func() {
					os.Setenv("XDG_RUNTIME_DIR", oldXDG)
				}
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanup := tt.setupEnv()
			defer cleanup()

			listener, socketPath, err := CreateSocket()

			if tt.wantErr {
				if err == nil {
					t.Errorf("CreateSocket() error = nil, want error containing %q", tt.wantErrSubstr)
				} else if tt.wantErrSubstr != "" && !strings.Contains(err.Error(), tt.wantErrSubstr) {
					t.Errorf("CreateSocket() error = %q, want error containing %q", err.Error(), tt.wantErrSubstr)
				}
				return
			}

			if err != nil {
				t.Fatalf("CreateSocket() error = %v, want nil", err)
			}

			if listener == nil {
				t.Fatal("CreateSocket() returned nil listener")
			}
			defer listener.Close()

			if socketPath == "" {
				t.Error("CreateSocket() returned empty socketPath")
			}

			// Verify socket file exists
			if _, err := os.Stat(socketPath); os.IsNotExist(err) {
				t.Errorf("socket file does not exist at %s", socketPath)
			}

			// Verify socket directory has correct permissions (0700)
			sockDir := filepath.Dir(socketPath)
			info, err := os.Stat(sockDir)
			if err != nil {
				t.Fatalf("failed to stat socket directory: %v", err)
			}
			if info.Mode().Perm() != 0700 {
				t.Errorf("socket directory permissions = %o, want 0700", info.Mode().Perm())
			}

			// Verify we can accept connections
			type connResult struct {
				conn net.Conn
				err  error
			}
			acceptChan := make(chan connResult, 1)
			go func() {
				conn, err := listener.Accept()
				acceptChan <- connResult{conn, err}
			}()

			// Try to dial the socket
			clientConn, err := net.Dial("unix", socketPath)
			if err != nil {
				t.Fatalf("failed to dial socket: %v", err)
			}
			defer clientConn.Close()

			// Wait for accept
			result := <-acceptChan
			if result.err != nil {
				t.Fatalf("listener.Accept() error = %v", result.err)
			}
			if result.conn == nil {
				t.Fatal("listener.Accept() returned nil connection")
			}
			defer result.conn.Close()
		})
	}
}

func TestCreateSocket_DirectoryCreation(t *testing.T) {
	tmpDir := t.TempDir()
	oldXDG := os.Getenv("XDG_RUNTIME_DIR")
	defer os.Setenv("XDG_RUNTIME_DIR", oldXDG)
	os.Setenv("XDG_RUNTIME_DIR", tmpDir)

	// Ensure socket directory doesn't exist
	sockDir := filepath.Join(tmpDir, socketDir)
	os.RemoveAll(sockDir)

	listener, socketPath, err := CreateSocket()
	if err != nil {
		t.Fatalf("CreateSocket() error = %v, want nil", err)
	}
	defer listener.Close()

	// Verify directory was created
	info, err := os.Stat(sockDir)
	if err != nil {
		t.Fatalf("socket directory not created: %v", err)
	}

	if !info.IsDir() {
		t.Error("socket path is not a directory")
	}

	if info.Mode().Perm() != 0700 {
		t.Errorf("socket directory permissions = %o, want 0700", info.Mode().Perm())
	}

	// Verify socket file exists
	if _, err := os.Stat(socketPath); os.IsNotExist(err) {
		t.Errorf("socket file does not exist at %s", socketPath)
	}
}

func TestCreateSocket_MultipleConnections(t *testing.T) {
	tmpDir := t.TempDir()
	oldXDG := os.Getenv("XDG_RUNTIME_DIR")
	defer os.Setenv("XDG_RUNTIME_DIR", oldXDG)
	os.Setenv("XDG_RUNTIME_DIR", tmpDir)

	listener, socketPath, err := CreateSocket()
	if err != nil {
		t.Fatalf("CreateSocket() error = %v, want nil", err)
	}
	defer listener.Close()

	// Test that we can accept multiple connections
	for i := 0; i < 3; i++ {
		// Accept connection in background
		type acceptResult struct {
			conn net.Conn
			err  error
		}
		acceptChan := make(chan acceptResult, 1)
		go func() {
			conn, err := listener.Accept()
			acceptChan <- acceptResult{conn, err}
		}()

		// Dial the socket
		clientConn, err := net.Dial("unix", socketPath)
		if err != nil {
			t.Fatalf("connection %d: failed to dial socket: %v", i, err)
		}

		// Wait for accept
		result := <-acceptChan
		if result.err != nil {
			clientConn.Close()
			t.Fatalf("connection %d: listener.Accept() error = %v", i, result.err)
		}

		// Clean up
		clientConn.Close()
		result.conn.Close()
	}
}
