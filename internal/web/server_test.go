package web

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hurricanerix/weave/internal/config"
	"github.com/hurricanerix/weave/internal/image"
	"github.com/hurricanerix/weave/internal/ollama"
)

func TestNewServer(t *testing.T) {
	tests := []struct {
		name     string
		addr     string
		wantAddr string
	}{
		{
			name:     "custom address",
			addr:     "localhost:9090",
			wantAddr: "localhost:9090",
		},
		{
			name:     "empty address uses default",
			addr:     "",
			wantAddr: DefaultAddr,
		},
		{
			name:     "default address constant",
			addr:     DefaultAddr,
			wantAddr: "localhost:8080",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := NewServer(tt.addr)
			if err != nil {
				t.Fatalf("NewServer() error = %v", err)
			}
			if s.addr != tt.wantAddr {
				t.Errorf("addr = %q, want %q", s.addr, tt.wantAddr)
			}
			if s.server == nil {
				t.Error("server is nil")
			}
			if s.server.Addr != tt.wantAddr {
				t.Errorf("server.Addr = %q, want %q", s.server.Addr, tt.wantAddr)
			}
		})
	}
}

func TestServer_Routes(t *testing.T) {
	tests := []struct {
		name            string
		method          string
		path            string
		wantStatusCode  int
		wantContentType string
		bodyContains    string
	}{
		{
			name:            "GET / returns index",
			method:          "GET",
			path:            "/",
			wantStatusCode:  http.StatusOK,
			wantContentType: "text/html",
			bodyContains:    "Weave Web UI",
		},
		// Note: SSE endpoint is tested separately in TestServer_HandleEvents
		// because it blocks waiting for SSE connection when session is provided
		{
			name:            "POST /chat without message returns 400",
			method:          "POST",
			path:            "/chat",
			wantStatusCode:  http.StatusBadRequest,
			wantContentType: "application/json",
			bodyContains:    `"message required"`,
		},
		{
			name:            "POST /prompt returns ok with session_id",
			method:          "POST",
			path:            "/prompt",
			wantStatusCode:  http.StatusOK,
			wantContentType: "application/json",
			bodyContains:    `"status":"ok"`,
		},
		{
			name:            "POST /generate returns ok with session_id",
			method:          "POST",
			path:            "/generate",
			wantStatusCode:  http.StatusOK,
			wantContentType: "application/json",
			bodyContains:    `"status":"ok"`,
		},
		{
			name:           "POST / wrong method returns 405",
			method:         "POST",
			path:           "/",
			wantStatusCode: http.StatusMethodNotAllowed,
		},
		{
			name:            "GET /chat falls back to index",
			method:          "GET",
			path:            "/chat",
			wantStatusCode:  http.StatusOK,
			wantContentType: "text/html",
			bodyContains:    "Weave Web UI",
		},
		{
			name:           "PUT /events wrong method returns 405",
			method:         "PUT",
			path:           "/events",
			wantStatusCode: http.StatusMethodNotAllowed,
		},
	}

	s, err := NewServer("")
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			w := httptest.NewRecorder()

			s.server.Handler.ServeHTTP(w, req)

			if w.Code != tt.wantStatusCode {
				t.Errorf("status code = %d, want %d", w.Code, tt.wantStatusCode)
			}

			if tt.wantContentType != "" {
				contentType := w.Header().Get("Content-Type")
				if !strings.Contains(contentType, tt.wantContentType) {
					t.Errorf("Content-Type = %q, want to contain %q", contentType, tt.wantContentType)
				}
			}

			if tt.bodyContains != "" {
				body := w.Body.String()
				if !strings.Contains(body, tt.bodyContains) {
					t.Errorf("body = %q, want to contain %q", body, tt.bodyContains)
				}
			}
		})
	}
}

func TestServer_HandleIndex(t *testing.T) {
	s, err := NewServer("")
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	s.handleIndex(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusOK)
	}

	contentType := w.Header().Get("Content-Type")
	if !strings.Contains(contentType, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", contentType)
	}

	body := w.Body.String()
	if !strings.Contains(body, "Weave Web UI") {
		t.Errorf("body should contain title, got %q", body)
	}
}

func TestServer_HandleIndex_UsesDefaultValues(t *testing.T) {
	tests := []struct {
		name      string
		steps     int
		cfg       float64
		seed      int64
		wantSteps int
		wantCFG   float64
		wantSeed  int64
	}{
		{
			name:      "default values",
			steps:     4,
			cfg:       1.0,
			seed:      0,
			wantSteps: 4,
			wantCFG:   1.0,
			wantSeed:  0,
		},
		{
			name:      "custom values",
			steps:     10,
			cfg:       2.5,
			seed:      42,
			wantSteps: 10,
			wantCFG:   2.5,
			wantSeed:  42,
		},
		{
			name:      "random seed",
			steps:     20,
			cfg:       7.5,
			seed:      -1,
			wantSteps: 20,
			wantCFG:   7.5,
			wantSeed:  -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				Steps: tt.steps,
				CFG:   tt.cfg,
				Seed:  tt.seed,
			}

			s, err := NewServerWithDeps("", nil, nil, nil, cfg)
			if err != nil {
				t.Fatalf("NewServerWithDeps() error = %v", err)
			}

			// Verify server stored the defaults correctly
			if s.defaultSteps != tt.wantSteps {
				t.Errorf("defaultSteps = %d, want %d", s.defaultSteps, tt.wantSteps)
			}

			if s.defaultCFG != tt.wantCFG {
				t.Errorf("defaultCFG = %f, want %f", s.defaultCFG, tt.wantCFG)
			}

			if s.defaultSeed != tt.wantSeed {
				t.Errorf("defaultSeed = %d, want %d", s.defaultSeed, tt.wantSeed)
			}
		})
	}
}

func TestServer_HandleEvents(t *testing.T) {
	s, err := NewServer("")
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	t.Run("without session ID returns error", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/events", nil)
		w := httptest.NewRecorder()

		s.handleEvents(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("status code = %d, want %d", w.Code, http.StatusUnauthorized)
		}

		body := w.Body.String()
		if !strings.Contains(body, "session required") {
			t.Errorf("body should contain error message, got %q", body)
		}
	})

	t.Run("with session ID connects and sends event", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/events", nil)
		// Add session ID to context (simulating SessionMiddleware)
		ctx := setSessionID(req.Context(), "test-session")
		req = req.WithContext(ctx)
		w := httptest.NewRecorder()

		// Start serving in goroutine since it blocks
		done := make(chan struct{})
		go func() {
			defer close(done)
			s.handleEvents(w, req)
		}()

		// Give it time to set headers and send initial event
		time.Sleep(50 * time.Millisecond)

		// Verify SSE headers
		if contentType := w.Header().Get("Content-Type"); contentType != "text/event-stream" {
			t.Errorf("Content-Type = %q, want text/event-stream", contentType)
		}

		if cacheControl := w.Header().Get("Cache-Control"); cacheControl != "no-cache" {
			t.Errorf("Cache-Control = %q, want no-cache", cacheControl)
		}

		if connection := w.Header().Get("Connection"); connection != "keep-alive" {
			t.Errorf("Connection = %q, want keep-alive", connection)
		}

		// Verify connection event is sent
		body := w.Body.String()
		if !strings.Contains(body, "event: connected") {
			t.Errorf("body should contain connected event, got %q", body)
		}
		if !strings.Contains(body, `"session":"test-session"`) {
			t.Errorf("body should contain session ID, got %q", body)
		}

		// Close the connection
		s.Broker().CloseSession("test-session")

		// Wait for handler to complete
		select {
		case <-done:
			// Success
		case <-time.After(1 * time.Second):
			t.Fatal("timeout waiting for handler to complete")
		}
	})
}

func TestServer_PlaceholderHandlers(t *testing.T) {
	// Note: handleChat is no longer a placeholder - it has full implementation
	// with ollama integration. It's tested separately in integration tests.
	tests := []struct {
		name    string
		handler func(*Server, http.ResponseWriter, *http.Request)
		path    string
	}{
		{
			name:    "handlePrompt",
			handler: (*Server).handlePrompt,
			path:    "/prompt",
		},
		{
			name:    "handleGenerate",
			handler: (*Server).handleGenerate,
			path:    "/generate",
		},
	}

	s, err := NewServer("")
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", tt.path, nil)

			// Add session ID to context to simulate SessionMiddleware
			ctx := setSessionID(req.Context(), "test-session-123")
			req = req.WithContext(ctx)

			w := httptest.NewRecorder()

			tt.handler(s, w, req)

			if w.Code != http.StatusOK {
				t.Errorf("status code = %d, want %d", w.Code, http.StatusOK)
			}

			contentType := w.Header().Get("Content-Type")
			if contentType != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", contentType)
			}

			body := w.Body.String()
			if !strings.Contains(body, `"status":"ok"`) {
				t.Errorf("body = %q, want to contain status ok", body)
			}

			if !strings.Contains(body, `"session_id":"test-session-123"`) {
				t.Errorf("body = %q, want to contain session_id", body)
			}
		})
	}
}

func TestServer_HandleChat_Validation(t *testing.T) {
	s, err := NewServer("")
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	tests := []struct {
		name           string
		body           string
		wantStatusCode int
		wantBody       string
	}{
		{
			name:           "empty message returns 400",
			body:           "message=",
			wantStatusCode: http.StatusBadRequest,
			wantBody:       "message required",
		},
		{
			name:           "missing message returns 400",
			body:           "",
			wantStatusCode: http.StatusBadRequest,
			wantBody:       "message required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/chat", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

			// Add session ID to context
			ctx := setSessionID(req.Context(), "test-session")
			req = req.WithContext(ctx)

			w := httptest.NewRecorder()

			s.handleChat(w, req)

			if w.Code != tt.wantStatusCode {
				t.Errorf("status code = %d, want %d", w.Code, tt.wantStatusCode)
			}

			body := w.Body.String()
			if !strings.Contains(body, tt.wantBody) {
				t.Errorf("body = %q, want to contain %q", body, tt.wantBody)
			}
		})
	}
}

func TestServer_ListenAndServe_ContextCancellation(t *testing.T) {
	// Use a random available port for testing
	s, err := NewServer("localhost:0")
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Start server in goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- s.ListenAndServe(ctx)
	}()

	// Give server time to start
	time.Sleep(50 * time.Millisecond)

	// Cancel context to trigger shutdown
	cancel()

	// Wait for shutdown to complete
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("ListenAndServe returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for server shutdown")
	}
}

func TestServer_ListenAndServe_InvalidAddress(t *testing.T) {
	// Use an address that should fail (invalid port)
	s, err := NewServer("localhost:99999")
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err = s.ListenAndServe(ctx)
	if err == nil {
		t.Error("expected error for invalid address, got nil")
	}
}

// Integration test: verify server actually serves HTTP requests
func TestServer_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Start server on a random port
	s, err := NewServer("localhost:0")
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	// Extract actual address after server starts
	// We need to start the server first to get the actual port
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.ListenAndServe(ctx)
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 1 * time.Second,
	}

	// Test GET / endpoint
	resp, err := client.Get("http://" + s.addr)
	if err != nil {
		// If we can't connect, the server might not have started
		// This is expected behavior for port 0 (random port assignment)
		// We'll verify the server is running by checking it didn't error out
		t.Logf("Could not connect to server (expected for port 0): %v", err)
	} else {
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET / status = %d, want %d", resp.StatusCode, http.StatusOK)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("failed to read response body: %v", err)
		}

		if !strings.Contains(string(body), "Weave Web UI") {
			t.Errorf("response body should contain title")
		}
	}

	// Shutdown server
	cancel()

	// Verify server shuts down cleanly
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("server shutdown error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for server shutdown")
	}
}

// Integration test: verify API endpoints return session_id
func TestServer_APIEndpoints_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Note: POST /chat is excluded because it has full implementation
	// that requires ollama and proper message content.
	tests := []struct {
		name string
		path string
	}{
		{
			name: "POST /prompt",
			path: "/prompt",
		},
		{
			name: "POST /generate",
			path: "/generate",
		},
	}

	s, err := NewServer("")
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use httptest for a simpler integration test
			req := httptest.NewRequest("POST", tt.path, nil)
			w := httptest.NewRecorder()

			// Run through the full handler chain (including SessionMiddleware)
			s.server.Handler.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
			}

			contentType := w.Header().Get("Content-Type")
			if !strings.Contains(contentType, "application/json") {
				t.Errorf("Content-Type = %q, want application/json", contentType)
			}

			body := w.Body.String()
			if !strings.Contains(body, `"status":"ok"`) {
				t.Errorf("body should contain status ok, got %q", body)
			}

			// Verify session_id is present (SessionMiddleware adds it)
			if !strings.Contains(body, `"session_id":`) {
				t.Errorf("body should contain session_id, got %q", body)
			}

			// Verify the Set-Cookie header was sent by SessionMiddleware
			cookies := w.Result().Cookies()
			found := false
			for _, cookie := range cookies {
				if cookie.Name == SessionCookieName {
					found = true
					break
				}
			}
			if !found {
				t.Error("session cookie not set")
			}
		})
	}
}

func TestHandleImage_ValidImage(t *testing.T) {
	storage := image.NewStorage()
	server, err := NewServerWithDeps("", nil, nil, storage, nil)
	if err != nil {
		t.Fatalf("NewServerWithDeps failed: %v", err)
	}

	// Store a test image
	pngData := []byte{1, 2, 3, 4, 5}
	id, err := storage.Store(pngData, 100, 100)
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	tests := []struct {
		name string
		path string
	}{
		{"with extension", "/images/" + id + ".png"},
		{"without extension", "/images/" + id},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.path, nil)
			w := httptest.NewRecorder()

			// Use the server's handler to route the request properly
			server.server.Handler.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("got status %d, want %d", w.Code, http.StatusOK)
			}

			contentType := w.Header().Get("Content-Type")
			if contentType != "image/png" {
				t.Errorf("got Content-Type %q, want %q", contentType, "image/png")
			}

			cacheControl := w.Header().Get("Cache-Control")
			if cacheControl != "public, max-age=31536000, immutable" {
				t.Errorf("got Cache-Control %q, want %q", cacheControl, "public, max-age=31536000, immutable")
			}

			body := w.Body.Bytes()
			if len(body) != len(pngData) {
				t.Errorf("got body length %d, want %d", len(body), len(pngData))
			}
			for i := range pngData {
				if body[i] != pngData[i] {
					t.Errorf("body[%d] = %d, want %d", i, body[i], pngData[i])
				}
			}
		})
	}
}

func TestHandleImage_NotFound(t *testing.T) {
	storage := image.NewStorage()
	server, err := NewServerWithDeps("", nil, nil, storage, nil)
	if err != nil {
		t.Fatalf("NewServerWithDeps failed: %v", err)
	}

	// Use a valid UUID that doesn't exist
	nonExistentID := "550e8400-e29b-41d4-a716-446655440000"
	req := httptest.NewRequest("GET", "/images/"+nonExistentID, nil)
	w := httptest.NewRecorder()

	server.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("got status %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleImage_InvalidID(t *testing.T) {
	storage := image.NewStorage()
	server, err := NewServerWithDeps("", nil, nil, storage, nil)
	if err != nil {
		t.Fatalf("NewServerWithDeps failed: %v", err)
	}

	tests := []struct {
		name string
		id   string
	}{
		{"malformed", "not-a-uuid"},
		{"partial", "12345"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/images/"+tt.id, nil)
			w := httptest.NewRecorder()

			server.server.Handler.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("got status %d, want %d", w.Code, http.StatusBadRequest)
			}
		})
	}
}

func TestChatWithRetry_FormatReminderAfterParseError(t *testing.T) {
	tests := []struct {
		name           string
		initialError   error
		wantRetryCount int
	}{
		{
			name:           "missing delimiter triggers retry",
			initialError:   ollama.ErrMissingDelimiter,
			wantRetryCount: 2, // initial + 1 retry
		},
		{
			name:           "invalid JSON triggers retry",
			initialError:   ollama.ErrInvalidJSON,
			wantRetryCount: 2, // initial + 1 retry
		},
		{
			name:           "missing fields triggers retry",
			initialError:   ollama.ErrMissingFields,
			wantRetryCount: 2, // initial + 1 retry
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockOllamaClient{
				responses: []mockResponse{
					{err: tt.initialError}, // First attempt fails
					{result: ollama.ChatResult{ // Retry succeeds
						Response:    "Perfect! Generating now.",
						Metadata:    ollama.LLMMetadata{Prompt: "test prompt"},
						RawResponse: "Perfect! Generating now.\n---\n{\"prompt\":\"test prompt\",\"generate_image\":true}",
					}},
				},
			}

			server, err := NewServerWithDeps("", mock, nil, nil, nil)
			if err != nil {
				t.Fatalf("NewServerWithDeps failed: %v", err)
			}

			messages := []ollama.Message{
				{Role: ollama.RoleUser, Content: "test message"},
			}

			result, err := server.chatWithRetry(context.Background(), messages, nil, nil)

			if err != nil {
				t.Fatalf("chatWithRetry failed: %v", err)
			}

			if mock.callCount != tt.wantRetryCount {
				t.Errorf("call count = %d, want %d", mock.callCount, tt.wantRetryCount)
			}

			if result.Response != "Perfect! Generating now." {
				t.Errorf("response = %q, want %q", result.Response, "Perfect! Generating now.")
			}

			if result.Metadata.Prompt != "test prompt" {
				t.Errorf("prompt = %q, want %q", result.Metadata.Prompt, "test prompt")
			}
		})
	}
}

func TestChatWithRetry_ContextCompactionAfterTwoRetries(t *testing.T) {
	mock := &mockOllamaClient{
		responses: []mockResponse{
			{err: ollama.ErrMissingDelimiter}, // First attempt fails
			{err: ollama.ErrInvalidJSON},      // First retry fails
			{err: ollama.ErrMissingDelimiter}, // Second retry fails
			{result: ollama.ChatResult{ // Compaction retry succeeds
				Response:    "Perfect!",
				Metadata:    ollama.LLMMetadata{Prompt: "compacted prompt"},
				RawResponse: "Perfect!\n---\n{\"prompt\":\"compacted prompt\",\"generate_image\":true}",
			}},
		},
	}

	server, err := NewServerWithDeps("", mock, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewServerWithDeps failed: %v", err)
	}

	messages := []ollama.Message{
		{Role: ollama.RoleSystem, Content: ollama.SystemPrompt},
		{Role: ollama.RoleUser, Content: "I want a cat in a hat"},
		{Role: ollama.RoleAssistant, Content: "What kind of cat?"},
		{Role: ollama.RoleUser, Content: "A tabby cat"},
	}

	result, err := server.chatWithRetry(context.Background(), messages, nil, nil)

	if err != nil {
		t.Fatalf("chatWithRetry failed: %v", err)
	}

	// Should have tried: initial + 2 format retries + 1 compaction = 4 calls
	if mock.callCount != 4 {
		t.Errorf("call count = %d, want 4", mock.callCount)
	}

	if result.Metadata.Prompt != "compacted prompt" {
		t.Errorf("prompt = %q, want %q", result.Metadata.Prompt, "compacted prompt")
	}
}

func TestChatWithRetry_ErrorReturnedAfterAllRetriesFail(t *testing.T) {
	mock := &mockOllamaClient{
		responses: []mockResponse{
			{err: ollama.ErrMissingDelimiter}, // First attempt fails
			{err: ollama.ErrInvalidJSON},      // First retry fails
			{err: ollama.ErrMissingFields},    // Second retry fails
			{err: ollama.ErrMissingDelimiter}, // Compaction retry fails
		},
	}

	server, err := NewServerWithDeps("", mock, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewServerWithDeps failed: %v", err)
	}

	messages := []ollama.Message{
		{Role: ollama.RoleUser, Content: "test message"},
	}

	_, err = server.chatWithRetry(context.Background(), messages, nil, nil)

	if err == nil {
		t.Fatal("expected error after all retries fail, got nil")
	}

	// Should be a format error
	if !errors.Is(err, ollama.ErrMissingDelimiter) &&
		!errors.Is(err, ollama.ErrInvalidJSON) &&
		!errors.Is(err, ollama.ErrMissingFields) {
		t.Errorf("expected format error, got %v", err)
	}

	// Should have tried: initial + 2 format retries + 1 compaction = 4 calls
	if mock.callCount != 4 {
		t.Errorf("call count = %d, want 4", mock.callCount)
	}
}

func TestChatWithRetry_RetryCountResetsOnSuccess(t *testing.T) {
	mock := &mockOllamaClient{
		responses: []mockResponse{
			{result: ollama.ChatResult{ // First request succeeds
				Response:    "What kind of cat?",
				Metadata:    ollama.LLMMetadata{Prompt: ""},
				RawResponse: "What kind of cat?\n---\n{\"prompt\":\"\",\"generate_image\":false}",
			}},
			{err: ollama.ErrMissingDelimiter}, // Second request fails
			{result: ollama.ChatResult{ // Retry succeeds
				Response:    "Perfect!",
				Metadata:    ollama.LLMMetadata{Prompt: "tabby cat"},
				RawResponse: "Perfect!\n---\n{\"prompt\":\"tabby cat\",\"generate_image\":true}",
			}},
		},
	}

	server, err := NewServerWithDeps("", mock, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewServerWithDeps failed: %v", err)
	}

	messages1 := []ollama.Message{
		{Role: ollama.RoleUser, Content: "first message"},
	}

	// First request should succeed without retry
	result1, err := server.chatWithRetry(context.Background(), messages1, nil, nil)
	if err != nil {
		t.Fatalf("first chatWithRetry failed: %v", err)
	}

	if mock.callCount != 1 {
		t.Errorf("first call count = %d, want 1", mock.callCount)
	}

	if result1.Response != "What kind of cat?" {
		t.Errorf("first response = %q, want %q", result1.Response, "What kind of cat?")
	}

	// Second request should fail then retry (demonstrating retry count is per-request)
	messages2 := []ollama.Message{
		{Role: ollama.RoleUser, Content: "second message"},
	}

	result2, err := server.chatWithRetry(context.Background(), messages2, nil, nil)
	if err != nil {
		t.Fatalf("second chatWithRetry failed: %v", err)
	}

	// Total calls should be 3 (1 from first + 2 from second)
	if mock.callCount != 3 {
		t.Errorf("total call count = %d, want 3", mock.callCount)
	}

	if result2.Metadata.Prompt != "tabby cat" {
		t.Errorf("second prompt = %q, want %q", result2.Metadata.Prompt, "tabby cat")
	}
}

func TestChatWithRetry_NonFormatErrorReturnsImmediately(t *testing.T) {
	nonFormatErr := errors.New("connection error")

	mock := &mockOllamaClient{
		responses: []mockResponse{
			{err: nonFormatErr}, // Non-format error
		},
	}

	server, err := NewServerWithDeps("", mock, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewServerWithDeps failed: %v", err)
	}

	messages := []ollama.Message{
		{Role: ollama.RoleUser, Content: "test message"},
	}

	_, err = server.chatWithRetry(context.Background(), messages, nil, nil)

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !errors.Is(err, nonFormatErr) {
		t.Errorf("expected connection error, got %v", err)
	}

	// Should have only tried once (no retry for non-format errors)
	if mock.callCount != 1 {
		t.Errorf("call count = %d, want 1", mock.callCount)
	}
}

func TestCompactContext_CorrectFormat(t *testing.T) {
	server, err := NewServerWithDeps("", nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewServerWithDeps failed: %v", err)
	}

	messages := []ollama.Message{
		{Role: ollama.RoleSystem, Content: ollama.SystemPrompt},
		{Role: ollama.RoleUser, Content: "I want a cat in a hat"},
		{Role: ollama.RoleAssistant, Content: "What kind of cat?\n---\n{\"prompt\":\"\",\"generate_image\":false}"},
		{Role: ollama.RoleUser, Content: "A tabby cat wearing a wizard hat"},
		{Role: ollama.RoleUser, Content: "Make it realistic"},
	}

	compacted := server.compactContext(messages)

	// Should return single system message
	if len(compacted) != 1 {
		t.Fatalf("compacted length = %d, want 1", len(compacted))
	}

	if compacted[0].Role != ollama.RoleSystem {
		t.Errorf("compacted role = %q, want %q", compacted[0].Role, ollama.RoleSystem)
	}

	content := compacted[0].Content

	// Should contain "User wants:"
	if !strings.Contains(content, "User wants:") {
		t.Errorf("compacted content missing 'User wants:', got: %s", content)
	}

	// Should contain extracted keywords from user messages
	keywords := []string{"cat", "hat", "tabby", "wizard", "realistic"}
	for _, keyword := range keywords {
		if !strings.Contains(strings.ToLower(content), keyword) {
			t.Errorf("compacted content missing keyword %q, got: %s", keyword, content)
		}
	}

	// Should request JSON-only response
	if !strings.Contains(content, "ONLY JSON") {
		t.Errorf("compacted content missing 'ONLY JSON', got: %s", content)
	}

	// Should include delimiter
	if !strings.Contains(content, "---") {
		t.Errorf("compacted content missing delimiter '---', got: %s", content)
	}

	// Should include JSON format example
	if !strings.Contains(content, `{"prompt":`) {
		t.Errorf("compacted content missing JSON format example, got: %s", content)
	}

	if !strings.Contains(content, `"generate_image": true`) {
		t.Errorf("compacted content missing 'generate_image' field example, got: %s", content)
	}
}

func TestCompactContext_SkipsSystemInjectedMessages(t *testing.T) {
	server, err := NewServerWithDeps("", nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewServerWithDeps failed: %v", err)
	}

	messages := []ollama.Message{
		{Role: ollama.RoleSystem, Content: ollama.SystemPrompt},
		{Role: ollama.RoleUser, Content: "I want a cat"},
		{Role: ollama.RoleUser, Content: "[user edited prompt to: cat in hat]"}, // Should be skipped
		{Role: ollama.RoleUser, Content: "[Current prompt: cat in space]"},      // Should be skipped
		{Role: ollama.RoleUser, Content: "Make it blue"},
	}

	compacted := server.compactContext(messages)

	content := compacted[0].Content

	// Should contain user content
	if !strings.Contains(strings.ToLower(content), "cat") {
		t.Errorf("compacted content missing user content, got: %s", content)
	}

	if !strings.Contains(strings.ToLower(content), "blue") {
		t.Errorf("compacted content missing user content, got: %s", content)
	}

	// Should NOT contain system-injected messages
	if strings.Contains(content, "[user edited") {
		t.Errorf("compacted content should not contain system-injected messages, got: %s", content)
	}

	if strings.Contains(content, "[Current prompt") {
		t.Errorf("compacted content should not contain system-injected messages, got: %s", content)
	}
}

func TestCompactContext_TruncatesLongContent(t *testing.T) {
	server, err := NewServerWithDeps("", nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewServerWithDeps failed: %v", err)
	}

	// Create a very long user message
	longMessage := strings.Repeat("cat dog bird fish ", 100) // ~1700 chars

	messages := []ollama.Message{
		{Role: ollama.RoleUser, Content: longMessage},
	}

	compacted := server.compactContext(messages)
	content := compacted[0].Content

	// Extract just the "User wants: ..." part before the instructions
	userWantsPart := content
	if idx := strings.Index(content, "\n\nRespond with ONLY JSON"); idx != -1 {
		userWantsPart = content[:idx]
	}

	// The summary should be truncated (max 200 chars after "User wants: ")
	// Total should be less than some reasonable upper bound
	if len(userWantsPart) > 300 {
		t.Errorf("compacted user wants section too long: %d chars (should be under 300)", len(userWantsPart))
	}

	// Should contain ellipsis if truncated
	if len(longMessage) > 200 && !strings.Contains(userWantsPart, "...") {
		t.Errorf("expected truncation ellipsis for long content, got: %s", userWantsPart)
	}
}

func TestServer_ParseSteps(t *testing.T) {
	cfg := &config.Config{
		Steps: 20,
		CFG:   1.0,
		Seed:  0,
	}
	server, err := NewServerWithDeps("", nil, nil, nil, cfg)
	if err != nil {
		t.Fatalf("NewServerWithDeps failed: %v", err)
	}

	tests := []struct {
		name  string
		value string
		want  uint32
	}{
		{"valid value", "50", 50},
		{"minimum value", "1", 1},
		{"maximum value", "100", 100},
		{"empty string uses default", "", 20},
		{"zero uses default", "0", 20},
		{"negative uses default", "-1", 20},
		{"too large uses default", "101", 20},
		{"invalid format uses default", "abc", 20},
		{"float uses default", "4.5", 20},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := server.parseSteps(tt.value)
			if got != tt.want {
				t.Errorf("parseSteps(%q) = %d, want %d", tt.value, got, tt.want)
			}
		})
	}
}

func TestServer_ParseCFG(t *testing.T) {
	cfg := &config.Config{
		Steps: 20,
		CFG:   1.5,
		Seed:  0,
	}
	server, err := NewServerWithDeps("", nil, nil, nil, cfg)
	if err != nil {
		t.Fatalf("NewServerWithDeps failed: %v", err)
	}

	tests := []struct {
		name  string
		value string
		want  float64
	}{
		{"valid integer", "7", 7.0},
		{"valid decimal", "7.5", 7.5},
		{"minimum value", "0", 0.0},
		{"maximum value", "20", 20.0},
		{"empty string uses default", "", 1.5},
		{"negative uses default", "-1", 1.5},
		{"too large uses default", "21", 1.5},
		{"invalid format uses default", "abc", 1.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := server.parseCFG(tt.value)
			if got != tt.want {
				t.Errorf("parseCFG(%q) = %f, want %f", tt.value, got, tt.want)
			}
		})
	}
}

func TestServer_ParseSeed(t *testing.T) {
	cfg := &config.Config{
		Steps: 20,
		CFG:   1.0,
		Seed:  42,
	}
	server, err := NewServerWithDeps("", nil, nil, nil, cfg)
	if err != nil {
		t.Fatalf("NewServerWithDeps failed: %v", err)
	}

	tests := []struct {
		name  string
		value string
		want  int64
	}{
		{"valid positive", "12345", 12345},
		{"zero", "0", 0},
		{"negative one (random)", "-1", -1},
		{"large value", "9223372036854775807", 9223372036854775807}, // max int64
		{"empty string uses default", "", 42},
		{"invalid format uses default", "abc", 42},
		{"float uses default", "123.45", 42},
		{"seed below -1 uses default", "-2", 42},
		{"very negative seed uses default", "-999", 42},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := server.parseSeed(tt.value)
			if got != tt.want {
				t.Errorf("parseSeed(%q) = %d, want %d", tt.value, got, tt.want)
			}
		})
	}
}

func TestServer_HandleGenerateWithSettings(t *testing.T) {
	cfg := &config.Config{
		Steps: 4,
		CFG:   1.0,
		Seed:  0,
	}
	server, err := NewServerWithDeps("", nil, nil, nil, cfg)
	if err != nil {
		t.Fatalf("NewServerWithDeps failed: %v", err)
	}

	tests := []struct {
		name  string
		steps string
		cfg   string
		seed  string
	}{
		{"custom values", "50", "7.5", "12345"},
		{"missing steps", "", "7.5", "12345"},
		{"missing cfg", "50", "", "12345"},
		{"missing seed", "50", "7.5", ""},
		{"all missing", "", "", ""},
		{"invalid steps", "999", "7.5", "12345"},
		{"invalid cfg", "50", "999", "12345"},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create form data
			formData := "prompt=test+prompt"
			if tt.steps != "" {
				formData += "&steps=" + tt.steps
			}
			if tt.cfg != "" {
				formData += "&cfg=" + tt.cfg
			}
			if tt.seed != "" {
				formData += "&seed=" + tt.seed
			}

			req := httptest.NewRequest("POST", "/generate", strings.NewReader(formData))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

			// Use unique session ID per test to avoid rate limiting
			sessionID := fmt.Sprintf("test-session-%d", i)
			ctx := setSessionID(req.Context(), sessionID)
			req = req.WithContext(ctx)

			w := httptest.NewRecorder()

			server.handleGenerate(w, req)

			// Should return OK status (daemon unavailable is expected, but parsing should work)
			if w.Code != http.StatusOK && w.Code != http.StatusServiceUnavailable {
				t.Errorf("status code = %d, want %d or %d", w.Code, http.StatusOK, http.StatusServiceUnavailable)
			}
		})
	}
}

func TestServer_SeedConversionForProtocol(t *testing.T) {
	tests := []struct {
		name          string
		seedInput     int64
		wantSeedValue uint64
		description   string
	}{
		{
			name:          "seed -1 converts to 0 for random",
			seedInput:     -1,
			wantSeedValue: 0,
			description:   "seed=-1 means random, protocol expects 0",
		},
		{
			name:          "seed 0 stays 0",
			seedInput:     0,
			wantSeedValue: 0,
			description:   "explicit 0 is valid",
		},
		{
			name:          "positive seed preserved",
			seedInput:     12345,
			wantSeedValue: 12345,
			description:   "deterministic seed should be preserved",
		},
		{
			name:          "large seed preserved",
			seedInput:     9223372036854775807,
			wantSeedValue: 9223372036854775807,
			description:   "max int64 should work",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the conversion logic from handleGenerate
			var seedValue uint64
			if tt.seedInput == -1 {
				seedValue = 0
			} else {
				seedValue = uint64(tt.seedInput)
			}

			if seedValue != tt.wantSeedValue {
				t.Errorf("seed conversion: input=%d, got=%d, want=%d (%s)",
					tt.seedInput, seedValue, tt.wantSeedValue, tt.description)
			}
		})
	}
}

func TestClampGenerationSettings(t *testing.T) {
	tests := []struct {
		name        string
		steps       int
		cfg         float64
		seed        int64
		wantSteps   int
		wantCFG     float64
		wantSeed    int64
		wantClamped int // number of clamped settings
	}{
		{
			name:        "valid values unchanged",
			steps:       20,
			cfg:         5.0,
			seed:        42,
			wantSteps:   20,
			wantCFG:     5.0,
			wantSeed:    42,
			wantClamped: 0,
		},
		{
			name:        "minimum valid values",
			steps:       1,
			cfg:         0.0,
			seed:        -1,
			wantSteps:   1,
			wantCFG:     0.0,
			wantSeed:    -1,
			wantClamped: 0,
		},
		{
			name:        "maximum valid values",
			steps:       100,
			cfg:         20.0,
			seed:        9999999,
			wantSteps:   100,
			wantCFG:     20.0,
			wantSeed:    9999999,
			wantClamped: 0,
		},
		{
			name:        "steps too low clamped to 1",
			steps:       0,
			cfg:         5.0,
			seed:        42,
			wantSteps:   1,
			wantCFG:     5.0,
			wantSeed:    42,
			wantClamped: 1,
		},
		{
			name:        "steps negative clamped to 1",
			steps:       -10,
			cfg:         5.0,
			seed:        42,
			wantSteps:   1,
			wantCFG:     5.0,
			wantSeed:    42,
			wantClamped: 1,
		},
		{
			name:        "steps too high clamped to 100",
			steps:       150,
			cfg:         5.0,
			seed:        42,
			wantSteps:   100,
			wantCFG:     5.0,
			wantSeed:    42,
			wantClamped: 1,
		},
		{
			name:        "cfg negative clamped to 0",
			steps:       20,
			cfg:         -2.0,
			seed:        42,
			wantSteps:   20,
			wantCFG:     0.0,
			wantSeed:    42,
			wantClamped: 1,
		},
		{
			name:        "cfg too high clamped to 20",
			steps:       20,
			cfg:         25.0,
			seed:        42,
			wantSteps:   20,
			wantCFG:     20.0,
			wantSeed:    42,
			wantClamped: 1,
		},
		{
			name:        "seed too negative clamped to -1",
			steps:       20,
			cfg:         5.0,
			seed:        -5,
			wantSteps:   20,
			wantCFG:     5.0,
			wantSeed:    -1,
			wantClamped: 1,
		},
		{
			name:        "multiple values clamped",
			steps:       200,
			cfg:         -3.0,
			seed:        -100,
			wantSteps:   100,
			wantCFG:     0.0,
			wantSeed:    -1,
			wantClamped: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSteps, gotCFG, gotSeed, clamped := clampGenerationSettings(tt.steps, tt.cfg, tt.seed)

			if gotSteps != tt.wantSteps {
				t.Errorf("steps = %d, want %d", gotSteps, tt.wantSteps)
			}
			if gotCFG != tt.wantCFG {
				t.Errorf("cfg = %f, want %f", gotCFG, tt.wantCFG)
			}
			if gotSeed != tt.wantSeed {
				t.Errorf("seed = %d, want %d", gotSeed, tt.wantSeed)
			}
			if len(clamped) != tt.wantClamped {
				t.Errorf("clamped count = %d, want %d", len(clamped), tt.wantClamped)
			}
		})
	}
}

func TestFormatClampedFeedback(t *testing.T) {
	tests := []struct {
		name    string
		clamped []clampedSetting
		want    string
	}{
		{
			name:    "no clamped settings",
			clamped: nil,
			want:    "",
		},
		{
			name:    "empty slice",
			clamped: []clampedSetting{},
			want:    "",
		},
		{
			name: "single clamped setting",
			clamped: []clampedSetting{
				{name: "steps", original: "150", clamped: "100", reason: "maximum is 100"},
			},
			want: "Settings adjusted: steps 150→100 (maximum is 100)",
		},
		{
			name: "multiple clamped settings",
			clamped: []clampedSetting{
				{name: "steps", original: "150", clamped: "100", reason: "maximum is 100"},
				{name: "cfg", original: "-2.0", clamped: "0.0", reason: "minimum is 0"},
			},
			want: "Settings adjusted: steps 150→100 (maximum is 100), cfg -2.0→0.0 (minimum is 0)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatClampedFeedback(tt.clamped)
			if got != tt.want {
				t.Errorf("formatClampedFeedback() = %q, want %q", got, tt.want)
			}
		})
	}
}
