package startup

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestValidateOllama_Success(t *testing.T) {
	// Create test server that simulates ollama
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
		t.Errorf("ValidateOllama() error = %v, want nil", err)
	}
}

func TestValidateOllama_NotRunning(t *testing.T) {
	// Use a URL that won't accept connections
	err := ValidateOllama("http://localhost:99999")

	if err == nil {
		t.Fatal("ValidateOllama() error = nil, want error")
	}

	if !errors.Is(err, ErrOllamaNotRunning) {
		t.Errorf("error does not wrap ErrOllamaNotRunning: %v", err)
	}
}

func TestValidateOllama_UnexpectedStatusCode(t *testing.T) {
	// Create test server that returns non-2xx status
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	err := ValidateOllama(server.URL)

	if err == nil {
		t.Fatal("ValidateOllama() error = nil, want error")
	}

	if !errors.Is(err, ErrOllamaNotRunning) {
		t.Errorf("error does not wrap ErrOllamaNotRunning: %v", err)
	}
}

// SECURITY TEST: ValidateOllama must reject invalid URL schemes (SSRF prevention)
func TestValidateOllama_InvalidScheme(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{"file scheme", "file:///etc/passwd"},
		{"ftp scheme", "ftp://example.com"},
		{"data scheme", "data:text/plain,hello"},
		{"javascript scheme", "javascript:alert(1)"},
		{"no scheme", "localhost:11434"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateOllama(tt.url)
			if err == nil {
				t.Errorf("ValidateOllama(%q) error = nil, want error", tt.url)
			}
			if !errors.Is(err, ErrOllamaNotRunning) {
				t.Errorf("error does not wrap ErrOllamaNotRunning: %v", err)
			}
		})
	}
}

// SECURITY TEST: ValidateOllama must reject malformed URLs
func TestValidateOllama_InvalidURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{"invalid URL", "ht!tp://invalid"},
		{"empty URL", ""},
		{"malformed URL", "http://[::1]::11434"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateOllama(tt.url)
			if err == nil {
				t.Errorf("ValidateOllama(%q) error = nil, want error", tt.url)
			}
		})
	}
}

// SECURITY TEST: ValidateOllama must reject URLs without a host
func TestValidateOllama_NoHost(t *testing.T) {
	err := ValidateOllama("http://")
	if err == nil {
		t.Error("ValidateOllama with no host should return error")
	}
	if !errors.Is(err, ErrOllamaNotRunning) {
		t.Errorf("error does not wrap ErrOllamaNotRunning: %v", err)
	}
}

// SECURITY TEST: ValidateOllama must sanitize path/query/fragment from user input
func TestValidateOllama_PathInjection(t *testing.T) {
	// Create test server that tracks which paths are requested
	var requestedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Try to inject a different path
	err := ValidateOllama(server.URL + "/evil/path?query=param#fragment")
	if err != nil {
		t.Errorf("ValidateOllama() error = %v, want nil", err)
	}

	// Verify that the path was sanitized to /api/tags
	if requestedPath != "/api/tags" {
		t.Errorf("requested path = %q, want %q (path injection not prevented)", requestedPath, "/api/tags")
	}
}

func TestValidateCompute_SocketNotFound(t *testing.T) {
	// Set XDG_RUNTIME_DIR to a non-existent directory
	tempDir := t.TempDir()

	oldXDG := os.Getenv("XDG_RUNTIME_DIR")
	os.Setenv("XDG_RUNTIME_DIR", tempDir)
	defer os.Setenv("XDG_RUNTIME_DIR", oldXDG)

	err := ValidateCompute()

	if err == nil {
		t.Fatal("ValidateCompute() error = nil, want error")
	}

	if !errors.Is(err, ErrComputeNotRunning) {
		t.Errorf("error does not wrap ErrComputeNotRunning: %v", err)
	}

	// Just verify error is not nil and has the right type
	// Integration tests will verify actual connection behavior
}

func TestValidateCompute_XDGNotSet(t *testing.T) {
	// Save and unset XDG_RUNTIME_DIR
	oldXDG := os.Getenv("XDG_RUNTIME_DIR")
	os.Unsetenv("XDG_RUNTIME_DIR")
	defer func() {
		if oldXDG != "" {
			os.Setenv("XDG_RUNTIME_DIR", oldXDG)
		}
	}()

	err := ValidateCompute()

	if err == nil {
		t.Fatal("ValidateCompute() error = nil, want error")
	}
}

func TestGetSocketPath(t *testing.T) {
	tests := []struct {
		name    string
		xdgDir  string
		wantErr bool
	}{
		{
			name:    "valid XDG_RUNTIME_DIR",
			xdgDir:  "/tmp/test",
			wantErr: false,
		},
		{
			name:    "empty XDG_RUNTIME_DIR",
			xdgDir:  "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldXDG := os.Getenv("XDG_RUNTIME_DIR")
			defer func() {
				if oldXDG != "" {
					os.Setenv("XDG_RUNTIME_DIR", oldXDG)
				} else {
					os.Unsetenv("XDG_RUNTIME_DIR")
				}
			}()

			if tt.xdgDir != "" {
				os.Setenv("XDG_RUNTIME_DIR", tt.xdgDir)
			} else {
				os.Unsetenv("XDG_RUNTIME_DIR")
			}

			path, err := GetSocketPath()
			if (err != nil) != tt.wantErr {
				t.Errorf("GetSocketPath() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				expectedPath := filepath.Join(tt.xdgDir, socketDir, socketName)
				if path != expectedPath {
					t.Errorf("GetSocketPath() = %q, want %q", path, expectedPath)
				}
			}
		})
	}
}

// SECURITY TEST: GetSocketPath must reject relative paths (path traversal prevention)
func TestGetSocketPath_RelativePath(t *testing.T) {
	tests := []struct {
		name   string
		xdgDir string
	}{
		{"relative path", "relative/path"},
		{"dot path", "."},
		{"dot dot path", ".."},
		{"relative with dots", "some/../path"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldXDG := os.Getenv("XDG_RUNTIME_DIR")
			defer func() {
				if oldXDG != "" {
					os.Setenv("XDG_RUNTIME_DIR", oldXDG)
				} else {
					os.Unsetenv("XDG_RUNTIME_DIR")
				}
			}()

			os.Setenv("XDG_RUNTIME_DIR", tt.xdgDir)

			_, err := GetSocketPath()
			if err == nil {
				t.Errorf("GetSocketPath() with relative path %q should return error", tt.xdgDir)
			}
		})
	}
}

// SECURITY TEST: GetSocketPath must clean paths with .. and .
func TestGetSocketPath_PathCleaning(t *testing.T) {
	oldXDG := os.Getenv("XDG_RUNTIME_DIR")
	defer func() {
		if oldXDG != "" {
			os.Setenv("XDG_RUNTIME_DIR", oldXDG)
		} else {
			os.Unsetenv("XDG_RUNTIME_DIR")
		}
	}()

	// Set path with dots (but still absolute)
	os.Setenv("XDG_RUNTIME_DIR", "/tmp/test/./subdir/../")

	path, err := GetSocketPath()
	if err != nil {
		t.Fatalf("GetSocketPath() error = %v", err)
	}

	// Path should be cleaned (no . or ..)
	if filepath.Clean(path) != path {
		t.Errorf("GetSocketPath() returned unclean path: %q", path)
	}

	// Should not contain . or ..
	if filepath.Dir(path) != filepath.Clean(filepath.Dir(path)) {
		t.Errorf("GetSocketPath() path contains unclean components: %q", path)
	}
}
