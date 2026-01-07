package web

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/hurricanerix/weave/internal/client"
	"github.com/hurricanerix/weave/internal/conversation"
	"github.com/hurricanerix/weave/internal/image"
	"github.com/hurricanerix/weave/internal/ollama"
	"github.com/hurricanerix/weave/internal/protocol"
)

//go:embed templates/* static/*
var embeddedFS embed.FS

const (
	// DefaultAddr is the default address the server listens on.
	DefaultAddr = "localhost:8080"

	// ReadTimeout is the maximum duration for reading the entire request.
	ReadTimeout = 15 * time.Second

	// WriteTimeout is the maximum duration before timing out writes.
	WriteTimeout = 15 * time.Second

	// IdleTimeout is the maximum amount of time to wait for the next request.
	IdleTimeout = 60 * time.Second

	// ShutdownTimeout is the maximum time to wait for graceful shutdown.
	ShutdownTimeout = 30 * time.Second

	// MaxRequestBodySize is the maximum size of POST request bodies (1MB).
	MaxRequestBodySize = 1 * 1024 * 1024

	// MaxMessageLength is the maximum length of a chat message (10KB).
	MaxMessageLength = 10 * 1024

	// MaxPromptLength is the maximum length of an image prompt (50KB).
	MaxPromptLength = 50 * 1024
)

// ollamaClient is an interface for ollama client operations.
// This allows for mocking in tests.
type ollamaClient interface {
	Chat(ctx context.Context, messages []ollama.Message, seed *int64, callback ollama.StreamCallback) (string, error)
}

// Server provides HTTP serving for the web UI.
// It handles routes for the index page, SSE events, and API endpoints.
type Server struct {
	addr      string
	server    *http.Server
	broker    *Broker
	templates *template.Template

	// Dependencies for chat functionality
	ollamaClient   ollamaClient
	sessionManager *conversation.SessionManager
	rateLimiter    *rateLimiter

	// Image storage for generated images
	imageStorage *image.Storage

	// Request ID counter for compute daemon requests
	requestID uint64
}

// NewServer creates a new Server listening on the given address.
// If addr is empty, DefaultAddr is used.
// Returns an error if templates cannot be parsed.
//
// Deprecated: Use NewServerWithDeps to inject dependencies.
// This function creates default clients for backward compatibility with tests.
func NewServer(addr string) (*Server, error) {
	return NewServerWithDeps(addr, nil, nil, nil)
}

// NewServerWithDeps creates a new Server with injected dependencies.
// If addr is empty, DefaultAddr is used.
// If ollamaClient is nil, a default client is created.
// If sessionManager is nil, a default session manager is created.
// If imageStorage is nil, a default image storage is created.
// Returns an error if templates cannot be parsed.
func NewServerWithDeps(addr string, ollamaClient ollamaClient, sessionManager *conversation.SessionManager, imageStorage *image.Storage) (*Server, error) {
	if addr == "" {
		addr = DefaultAddr
	}

	// Use provided dependencies or create defaults
	if ollamaClient == nil {
		ollamaClient = ollama.NewClient()
	}
	if sessionManager == nil {
		sessionManager = conversation.NewSessionManager()
	}
	if imageStorage == nil {
		imageStorage = image.NewStorage()
	}

	// Parse templates from embedded filesystem
	tmpl, err := template.ParseFS(embeddedFS, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("failed to parse templates: %w", err)
	}

	s := &Server{
		addr:           addr,
		broker:         NewBroker(),
		templates:      tmpl,
		ollamaClient:   ollamaClient,
		sessionManager: sessionManager,
		rateLimiter:    newRateLimiter(),
		imageStorage:   imageStorage,
	}

	mux := http.NewServeMux()
	s.registerRoutes(mux)

	// Wrap handler with session middleware to ensure all requests have a session ID
	handler := SessionMiddleware(mux)

	s.server = &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  ReadTimeout,
		WriteTimeout: WriteTimeout,
		IdleTimeout:  IdleTimeout,
	}

	return s, nil
}

// Broker returns the SSE broker for sending events to connected clients.
func (s *Server) Broker() *Broker {
	return s.broker
}

// registerRoutes sets up all HTTP routes.
func (s *Server) registerRoutes(mux *http.ServeMux) {
	// Index page
	mux.HandleFunc("GET /", s.handleIndex)

	// Static files (if we add CSS/JS later)
	mux.Handle("GET /static/", http.FileServer(http.FS(embeddedFS)))

	// SSE endpoint for real-time updates
	mux.HandleFunc("GET /events", s.handleEvents)

	// API endpoints (placeholders)
	mux.HandleFunc("POST /chat", s.handleChat)
	mux.HandleFunc("POST /prompt", s.handlePrompt)
	mux.HandleFunc("POST /generate", s.handleGenerate)

	// Image serving endpoint
	mux.HandleFunc("GET /images/{id}", s.handleImage)
}

// ListenAndServe starts the HTTP server and blocks until the context is cancelled.
// Returns an error if the server fails to start or encounters a non-graceful shutdown error.
func (s *Server) ListenAndServe(ctx context.Context) error {
	// Start rate limiter cleanup goroutine
	s.rateLimiter.startCleanup(ctx)

	// Channel to capture server errors
	errCh := make(chan error, 1)

	// Start server in goroutine
	go func() {
		log.Printf("Starting web server on http://%s", s.addr)
		if err := s.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	// Wait for context cancellation or server error
	select {
	case <-ctx.Done():
		// Context cancelled, initiate graceful shutdown
		log.Println("Shutting down web server...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), ShutdownTimeout)
		defer cancel()

		if err := s.server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("server shutdown failed: %w", err)
		}

		// Shutdown SSE broker
		if err := s.broker.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("broker shutdown failed: %w", err)
		}

		log.Println("Web server stopped")
		return nil

	case err := <-errCh:
		// Server failed to start or encountered error
		return fmt.Errorf("server error: %w", err)
	}
}

// handleIndex serves the index page.
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if err := s.templates.ExecuteTemplate(w, "index.html", nil); err != nil {
		log.Printf("Failed to execute template: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
}

// handleEvents serves the SSE endpoint for real-time updates.
// It delegates to the SSE broker which manages the connection lifecycle.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	s.broker.ServeHTTP(w, r)
}

// handleChat handles user chat messages.
// It adds the user message to the conversation, calls ollama for a response,
// streams tokens via SSE, and updates the prompt if the agent provides one.
func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	sessionID := GetSessionID(r.Context())

	// SECURITY: Limit request body size
	r.Body = http.MaxBytesReader(w, r.Body, MaxRequestBodySize)

	// SECURITY: Check rate limit
	if !s.rateLimiter.allowChat(sessionID) {
		log.Printf("Rate limit exceeded for session %s (chat)", sessionID)
		s.sendErrorEvent(sessionID, "Too many requests. Please wait a moment.")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprintf(w, `{"status":"error","message":"rate limit exceeded"}`)
		return
	}

	// Parse form data
	if err := r.ParseForm(); err != nil {
		log.Printf("Failed to parse form: %v", err)
		s.sendErrorEvent(sessionID, "Failed to parse message")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, `{"status":"error","message":"failed to parse form"}`)
		return
	}

	message := strings.TrimSpace(r.FormValue("message"))
	if message == "" {
		s.sendErrorEvent(sessionID, "Message cannot be empty")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, `{"status":"error","message":"message required"}`)
		return
	}

	// SECURITY: Validate message length
	if len(message) > MaxMessageLength {
		log.Printf("Message too long for session %s: %d bytes", sessionID, len(message))
		s.sendErrorEvent(sessionID, "Message is too long. Please shorten your message.")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		fmt.Fprintf(w, `{"status":"error","message":"message too long"}`)
		return
	}

	// Get conversation manager for this session
	manager := s.sessionManager.GetOrCreate(sessionID)

	// Add user message to conversation
	manager.AddUserMessage(message)

	// Build LLM context with system prompt
	context := manager.BuildLLMContext(ollama.SystemPrompt)

	// Convert conversation messages to ollama messages
	ollamaMessages := make([]ollama.Message, len(context))
	for i, msg := range context {
		ollamaMessages[i] = ollama.Message{
			Role:    msg.Role,
			Content: msg.Content,
		}
	}

	// Stream response from ollama
	fullResponse, err := s.ollamaClient.Chat(r.Context(), ollamaMessages, nil, func(token ollama.StreamToken) error {
		// Send each token via SSE
		if token.Content != "" {
			// Check for send errors to detect client disconnection
			if err := s.broker.SendEvent(sessionID, EventAgentToken, map[string]string{
				"token": token.Content,
			}); err != nil {
				// Client disconnected - abort streaming to avoid wasting resources
				return fmt.Errorf("client disconnected: %w", err)
			}
		}
		return nil
	})

	if err != nil {
		// SECURITY: Log full error server-side but send generic message to client
		log.Printf("Ollama chat error for session %s: %v", sessionID, err)
		s.sendErrorEvent(sessionID, "An error occurred while processing your message. Please try again.")
		// Send agent-done to finalize any partial message
		_ = s.broker.SendEvent(sessionID, EventAgentDone, map[string]bool{"done": true})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK) // Return OK so HTMX doesn't show error
		fmt.Fprintf(w, `{"status":"ok","session_id":"%s"}`, sessionID)
		return
	}

	// Extract prompt from response
	prompt := ollama.ExtractPrompt(fullResponse)

	// Add assistant message to conversation (with prompt if found)
	manager.AddAssistantMessage(fullResponse, prompt)

	// Send prompt-update event if prompt was extracted
	if prompt != "" {
		_ = s.broker.SendEvent(sessionID, EventPromptUpdate, map[string]string{
			"prompt": prompt,
		})
	}

	// Send agent-done event
	_ = s.broker.SendEvent(sessionID, EventAgentDone, map[string]bool{"done": true})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"ok","session_id":"%s"}`, sessionID)
}

// sendErrorEvent sends an error event to the client via SSE.
func (s *Server) sendErrorEvent(sessionID string, message string) {
	_ = s.broker.SendEvent(sessionID, EventError, map[string]string{
		"message": message,
	})
}

// handlePrompt handles prompt updates from the user.
// When the user edits the prompt in the UI and blurs the field,
// this handler saves the new prompt and notifies the conversation manager.
func (s *Server) handlePrompt(w http.ResponseWriter, r *http.Request) {
	sessionID := GetSessionID(r.Context())

	// SECURITY: Limit request body size
	r.Body = http.MaxBytesReader(w, r.Body, MaxRequestBodySize)

	// Parse form data
	if err := r.ParseForm(); err != nil {
		log.Printf("Failed to parse form: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, `{"status":"error","message":"failed to parse form"}`)
		return
	}

	prompt := r.FormValue("prompt")
	// Empty prompt is allowed (user may clear the prompt)

	// SECURITY: Validate prompt length
	if len(prompt) > MaxPromptLength {
		log.Printf("Prompt too long for session %s: %d bytes", sessionID, len(prompt))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		fmt.Fprintf(w, `{"status":"error","message":"prompt too long"}`)
		return
	}

	// Get conversation manager for this session
	manager := s.sessionManager.GetOrCreate(sessionID)

	// Update the prompt (this sets the edited flag if changed)
	manager.UpdatePrompt(prompt)

	// Notify that the prompt was edited (injects system message if changed)
	manager.NotifyPromptEdited()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"ok","session_id":"%s"}`, sessionID)
}

// handleGenerate handles image generation requests.
// It reads the current prompt from the conversation manager and sends a request
// to the compute daemon. The response (image or error) is sent via SSE.
func (s *Server) handleGenerate(w http.ResponseWriter, r *http.Request) {
	sessionID := GetSessionID(r.Context())

	// SECURITY: Limit request body size
	r.Body = http.MaxBytesReader(w, r.Body, MaxRequestBodySize)

	// SECURITY: Check rate limit
	if !s.rateLimiter.allowGenerate(sessionID) {
		log.Printf("Rate limit exceeded for session %s (generate)", sessionID)
		s.sendErrorEvent(sessionID, "Too many generation requests. Please wait a moment.")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprintf(w, `{"status":"error","message":"rate limit exceeded"}`)
		return
	}

	// Parse form data to get prompt from request
	if err := r.ParseForm(); err != nil {
		log.Printf("Failed to parse form: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, `{"status":"error","message":"failed to parse form"}`)
		return
	}

	// Get conversation manager for this session
	manager := s.sessionManager.GetOrCreate(sessionID)

	// Get prompt from request, fall back to stored prompt
	// The request includes the prompt to avoid race conditions when the
	// user edits and clicks generate quickly (blur/save may not complete)
	prompt := strings.TrimSpace(r.FormValue("prompt"))
	if prompt == "" {
		// Fall back to stored prompt
		prompt = manager.GetCurrentPrompt()
	} else {
		// Update stored prompt with the one from request
		manager.UpdatePrompt(prompt)
	}

	if prompt == "" {
		s.sendErrorEvent(sessionID, "No prompt available. Chat with the agent first to create a prompt.")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status":"ok","session_id":"%s"}`, sessionID)
		return
	}

	// Truncate prompt if it exceeds maximum length
	// This works around the CLIP/T5 token mismatch bug in stable-diffusion.cpp
	// where T5 producing more tokens than CLIP causes GGML assertion failures.
	// See docs/bugs/003-long-prompt-crash.md for details.
	promptBytes := []byte(prompt)
	if len(promptBytes) > int(protocol.SD35MaxPromptLen) {
		originalLen := len(promptBytes)

		// Truncate at UTF-8 character boundary to avoid corrupting multi-byte characters
		maxLen := int(protocol.SD35MaxPromptLen)
		for maxLen > 0 && !utf8.RuneStart(promptBytes[maxLen]) {
			maxLen--
		}

		prompt = string(promptBytes[:maxLen])
		log.Printf("Truncated prompt from %d to %d bytes for session %s",
			originalLen, len(prompt), sessionID)
		manager.UpdatePrompt(prompt)
	}

	// Generate unique request ID
	reqID := atomic.AddUint64(&s.requestID, 1)

	// Create protocol request
	// Use reasonable defaults for now (TODO: make these configurable)
	// 768x768 balances quality and VRAM usage; 1024x1024 causes OOM during VAE decode
	width, height := uint32(768), uint32(768)
	steps := uint32(4)
	cfgScale := float32(1.0)
	seed := uint64(0) // 0 = random

	protoReq, err := protocol.NewSD35GenerateRequest(reqID, prompt, width, height, steps, cfgScale, seed)
	if err != nil {
		log.Printf("Failed to create protocol request for session %s: %v", sessionID, err)
		s.sendErrorEvent(sessionID, "Failed to create generation request: invalid prompt")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, `{"status":"error","message":"invalid prompt"}`)
		return
	}

	// Encode request
	requestData, err := protocol.EncodeSD35GenerateRequest(protoReq)
	if err != nil {
		log.Printf("Failed to encode protocol request for session %s: %v", sessionID, err)
		s.sendErrorEvent(sessionID, "Failed to encode generation request")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, `{"status":"error","message":"encoding failed"}`)
		return
	}

	// Connect to compute daemon
	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second) // 2 min timeout for generation
	defer cancel()

	conn, err := client.Connect(ctx)
	if err != nil {
		log.Printf("Failed to connect to compute daemon for session %s: %v", sessionID, err)
		if errors.Is(err, client.ErrDaemonNotRunning) {
			s.sendErrorEvent(sessionID, "Image generation is not available (weave-compute not running)")
		} else if errors.Is(err, client.ErrXDGNotSet) {
			s.sendErrorEvent(sessionID, "Image generation is not available (XDG_RUNTIME_DIR not set)")
		} else {
			s.sendErrorEvent(sessionID, "Failed to connect to image generation service")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintf(w, `{"status":"error","message":"daemon not available"}`)
		return
	}
	defer conn.Close()

	// Send request and receive response
	responseData, err := conn.Send(ctx, requestData)
	if err != nil {
		log.Printf("Failed to send request to compute daemon for session %s: %v", sessionID, err)
		if errors.Is(err, client.ErrConnectionClosed) {
			s.sendErrorEvent(sessionID, "Connection to image generation service was closed")
		} else if errors.Is(err, client.ErrReadTimeout) {
			s.sendErrorEvent(sessionID, "Image generation timed out. Try a simpler prompt.")
		} else {
			s.sendErrorEvent(sessionID, "Failed to generate image")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, `{"status":"error","message":"generation failed"}`)
		return
	}

	// Decode response
	response, err := protocol.DecodeResponse(responseData)
	if err != nil {
		log.Printf("Failed to decode response for session %s: %v", sessionID, err)
		s.sendErrorEvent(sessionID, "Failed to decode image generation response")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, `{"status":"error","message":"decode failed"}`)
		return
	}

	// Handle response type
	switch resp := response.(type) {
	case *protocol.SD35GenerateResponse:
		// Success! Convert raw pixels to PNG
		var format image.PixelFormat
		if resp.Channels == 3 {
			format = image.FormatRGB
		} else {
			format = image.FormatRGBA
		}

		pngData, err := image.EncodePNG(int(resp.ImageWidth), int(resp.ImageHeight), resp.ImageData, format)
		if err != nil {
			log.Printf("Failed to encode PNG for session %s: %v", sessionID, err)
			s.sendErrorEvent(sessionID, "Failed to encode generated image")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, `{"status":"error","message":"encoding failed"}`)
			return
		}

		// Store image
		imageID, err := s.imageStorage.Store(pngData, int(resp.ImageWidth), int(resp.ImageHeight))
		if err != nil {
			log.Printf("Failed to store image for session %s: %v", sessionID, err)
			if errors.Is(err, image.ErrImageTooLarge) {
				s.sendErrorEvent(sessionID, "Image is too large to store")
			} else {
				s.sendErrorEvent(sessionID, "Failed to store image. Please try again.")
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, `{"status":"error","message":"storage failed"}`)
			return
		}

		log.Printf("Generated image for session %s: %dx%d in %dms",
			sessionID, resp.ImageWidth, resp.ImageHeight, resp.GenerationTime)

		// Send image-ready event
		imageURL := fmt.Sprintf("/images/%s.png", imageID)
		s.broker.SendEvent(sessionID, EventImageReady, map[string]interface{}{
			"url":    imageURL,
			"width":  resp.ImageWidth,
			"height": resp.ImageHeight,
		})

	case *protocol.ErrorResponse:
		log.Printf("Compute daemon error for session %s: code=%d, msg=%s",
			sessionID, resp.ErrorCode, resp.ErrorMessage)
		s.sendErrorEvent(sessionID, fmt.Sprintf("Image generation failed: %s", resp.ErrorMessage))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, `{"status":"error","message":"generation failed"}`)
		return

	default:
		log.Printf("Unexpected response type for session %s: %T", sessionID, response)
		s.sendErrorEvent(sessionID, "Unexpected response from image generation service")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, `{"status":"error","message":"unexpected response"}`)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"ok","session_id":"%s"}`, sessionID)
}

// generatePlaceholderPixels creates a colored gradient for testing.
// This will be replaced with actual compute daemon output.
func generatePlaceholderPixels(width, height int) []byte {
	pixels := make([]byte, width*height*3) // RGB format
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			idx := (y*width + x) * 3
			// Create a gradient: red increases with x, green with y, blue is constant
			pixels[idx] = byte((x * 255) / width)    // R
			pixels[idx+1] = byte((y * 255) / height) // G
			pixels[idx+2] = 128                      // B (constant)
		}
	}
	return pixels
}

// handleImage serves a generated image by ID.
// GET /images/{id}
func (s *Server) handleImage(w http.ResponseWriter, r *http.Request) {
	// Extract ID from path parameter
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "Missing image ID", http.StatusBadRequest)
		return
	}

	// Remove .png extension if present
	id = strings.TrimSuffix(id, ".png")

	// Get image from storage
	pngData, _, _, err := s.imageStorage.Get(id)
	if err != nil {
		if errors.Is(err, image.ErrNotFound) {
			http.Error(w, "Image not found", http.StatusNotFound)
			return
		}
		if errors.Is(err, image.ErrInvalidID) {
			http.Error(w, "Invalid image ID", http.StatusBadRequest)
			return
		}
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Set headers for image serving
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")

	// Write PNG data
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(pngData); err != nil {
		log.Printf("Failed to write image data for %s: %v", id, err)
	}
}

// setOllamaClientForTesting replaces the ollama client with a test mock.
// This is only used in tests to inject mock implementations.
func (s *Server) setOllamaClientForTesting(client ollamaClient) {
	s.ollamaClient = client
}
