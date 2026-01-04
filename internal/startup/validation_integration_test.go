//go:build integration

package startup

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestValidateOllama_Integration_NotRunning(t *testing.T) {
	// Use a URL that definitely won't have ollama running
	err := ValidateOllama("http://localhost:99999")

	if err == nil {
		t.Fatal("ValidateOllama() should fail when ollama is not running")
	}

	t.Logf("Error message (as expected): %v", err)
}

func TestValidateOllama_Integration_Running(t *testing.T) {
	// This test requires ollama to actually be running
	// Create a test server that simulates ollama
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"models":[]}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	err := ValidateOllama(server.URL)
	if err != nil {
		t.Errorf("ValidateOllama() failed with running server: %v", err)
	}
}

func TestValidateCompute_Integration_NotRunning(t *testing.T) {
	// Set XDG_RUNTIME_DIR to a temporary directory with no socket
	tempDir := t.TempDir()

	oldXDG := os.Getenv("XDG_RUNTIME_DIR")
	os.Setenv("XDG_RUNTIME_DIR", tempDir)
	defer os.Setenv("XDG_RUNTIME_DIR", oldXDG)

	err := ValidateCompute()

	if err == nil {
		t.Fatal("ValidateCompute() should fail when socket doesn't exist")
	}

	t.Logf("Error message (as expected): %v", err)
}

func TestValidateCompute_Integration_SocketExists(t *testing.T) {
	// Create a temporary directory with a socket file
	tempDir := t.TempDir()
	socketDir := filepath.Join(tempDir, "weave")
	if err := os.MkdirAll(socketDir, 0700); err != nil {
		t.Fatalf("Failed to create socket directory: %v", err)
	}

	socketPath := filepath.Join(socketDir, "weave.sock")

	// Create an empty file to simulate socket existing (but not accepting connections)
	f, err := os.Create(socketPath)
	if err != nil {
		t.Fatalf("Failed to create socket file: %v", err)
	}
	f.Close()

	oldXDG := os.Getenv("XDG_RUNTIME_DIR")
	os.Setenv("XDG_RUNTIME_DIR", tempDir)
	defer os.Setenv("XDG_RUNTIME_DIR", oldXDG)

	err = ValidateCompute()

	// Should fail because the socket exists but nothing is listening
	if err == nil {
		t.Fatal("ValidateCompute() should fail when socket exists but daemon not accepting connections")
	}

	t.Logf("Error message (as expected): %v", err)
}
