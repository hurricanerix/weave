package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/hurricanerix/weave/internal/client"
	"github.com/hurricanerix/weave/internal/config"
	"github.com/hurricanerix/weave/internal/conversation"
	"github.com/hurricanerix/weave/internal/image"
	"github.com/hurricanerix/weave/internal/ollama"
	"github.com/hurricanerix/weave/internal/persistence"
	"github.com/hurricanerix/weave/internal/protocol"
	"github.com/hurricanerix/weave/internal/resources"
)

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
	Chat(ctx context.Context, messages []ollama.Message, seed *int64, tools []ollama.Tool, callback ollama.StreamCallback) (ollama.ChatResult, error)
}

// Server provides HTTP serving for the web UI.
// It handles routes for the index page, SSE events, and API endpoints.
type Server struct {
	addr      string
	server    *http.Server
	broker    *Broker
	templates *template.Template
	webFS     fs.FS

	// Dependencies for chat functionality
	ollamaClient   ollamaClient
	sessionManager *conversation.SessionManager
	rateLimiter    *rateLimiter

	// Image storage for generated images (in-memory)
	imageStorage *image.Storage

	// Image store for session-specific persistent images
	imageStore *persistence.ImageStore

	// Compute client for image generation (persistent connection)
	computeClient *client.Conn

	// Default generation settings from CLI flags
	defaultSteps  int
	defaultCFG    float64
	defaultSeed   int64
	defaultWidth  int
	defaultHeight int

	// Agent prompts loaded from file
	agentPrompt      string
	agentToolsPrompt string

	// Request ID counter for compute process requests
	requestID uint64

	// Active generation tracking for stop functionality
	activeGenMu     sync.Mutex
	activeGenCancel map[string]context.CancelFunc // sessionID -> cancel func

	// Compute process restart function (set by main after server creation).
	// Called when stop-generation kills the compute process and needs to respawn.
	// Returns the new client connection or error.
	computeRestartFunc func(ctx context.Context) (*client.Conn, error)
}

// indexTemplateData holds data passed to the index.html template.
// It contains default values for generation settings that populate the UI controls.
type indexTemplateData struct {
	Steps  int
	CFG    float64
	Seed   int64
	Width  int
	Height int
}

// NewServer creates a new Server listening on the given address.
// If addr is empty, DefaultAddr is used.
// Returns an error if templates cannot be parsed.
//
// Deprecated: Use NewServerWithDeps to inject dependencies.
// This function creates default clients for backward compatibility with tests.
func NewServer(addr string) (*Server, error) {
	return NewServerWithDeps(addr, nil, nil, nil, nil, nil, nil)
}

// NewServerWithDeps creates a new Server with injected dependencies.
// If addr is empty, DefaultAddr is used.
// If ollamaClient is nil, a default client is created.
// If sessionManager is nil, a default session manager is created.
// If imageStorage is nil, a default image storage is created.
// If imageStore is nil, a default image store is created.
// If computeClient is nil, generation requests will fail (for testing only).
// If cfg is nil, default generation settings are used (steps=20, cfg=3.5, seed=0).
// Returns an error if templates cannot be parsed or agent prompt file cannot be loaded.
func NewServerWithDeps(addr string, ollamaClient ollamaClient, sessionManager *conversation.SessionManager, imageStorage *image.Storage, imageStore *persistence.ImageStore, computeClient *client.Conn, cfg *config.Config) (*Server, error) {
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
	if imageStore == nil {
		imageStore = persistence.NewImageStore("config/sessions")
	}

	// Extract default generation settings from config
	defaultSteps := 20
	defaultCFG := 3.5
	defaultSeed := int64(0)
	defaultWidth := 1024
	defaultHeight := 1024
	var agentPromptPath string
	var agentToolsPromptPath string
	if cfg != nil {
		defaultSteps = cfg.Steps
		defaultCFG = cfg.CFG
		defaultSeed = cfg.Seed
		defaultWidth = cfg.Width
		defaultHeight = cfg.Height
		agentPromptPath = cfg.AgentPromptPath
		agentToolsPromptPath = cfg.AgentToolsPromptPath
	}

	// Load agent prompts from files (only if config provided)
	// When cfg is nil (deprecated NewServer for testing), use empty prompts
	var agentPrompt string
	if agentPromptPath != "" {
		var err error
		agentPrompt, err = config.LoadAgentPrompt(agentPromptPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load agent prompt: %w", err)
		}
	}
	var agentToolsPrompt string
	if agentToolsPromptPath != "" {
		var err error
		agentToolsPrompt, err = config.LoadAgentPrompt(agentToolsPromptPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load agent tools prompt: %w", err)
		}
	}

	// Get embedded resources and extract the web sub-filesystem
	embedded, err := resources.Embedded()
	if err != nil {
		return nil, fmt.Errorf("failed to load embedded resources: %w", err)
	}
	webFS, err := fs.Sub(embedded, "web")
	if err != nil {
		return nil, fmt.Errorf("failed to access web resources: %w", err)
	}

	// Parse templates from embedded filesystem
	tmpl, err := template.ParseFS(webFS, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("failed to parse templates: %w", err)
	}

	s := &Server{
		addr:             addr,
		broker:           NewBroker(),
		templates:        tmpl,
		webFS:            webFS,
		ollamaClient:     ollamaClient,
		sessionManager:   sessionManager,
		rateLimiter:      newRateLimiter(),
		imageStorage:     imageStorage,
		imageStore:       imageStore,
		computeClient:    computeClient,
		defaultSteps:     defaultSteps,
		defaultCFG:       defaultCFG,
		defaultSeed:      defaultSeed,
		defaultWidth:     defaultWidth,
		defaultHeight:    defaultHeight,
		agentPrompt:      agentPrompt,
		agentToolsPrompt: agentToolsPrompt,
		activeGenCancel:  make(map[string]context.CancelFunc),
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

// SetComputeClient replaces the compute client connection.
// Used by the compute restart function after spawning a new compute process.
func (s *Server) SetComputeClient(c *client.Conn) {
	s.computeClient = c
}

// SetComputeRestartFunc sets the function used to restart the compute process.
// This is called by main after server creation to wire in the restart logic.
func (s *Server) SetComputeRestartFunc(f func(ctx context.Context) (*client.Conn, error)) {
	s.computeRestartFunc = f
}

// registerRoutes sets up all HTTP routes.
func (s *Server) registerRoutes(mux *http.ServeMux) {
	// Index page
	mux.HandleFunc("GET /", s.handleIndex)

	// Static files (if we add CSS/JS later)
	mux.Handle("GET /static/", http.FileServer(http.FS(s.webFS)))

	// SSE endpoint for real-time updates
	mux.HandleFunc("GET /events", s.handleEvents)

	// API endpoints (placeholders)
	mux.HandleFunc("POST /chat", s.handleChat)
	mux.HandleFunc("POST /prompt", s.handlePrompt)
	mux.HandleFunc("POST /generate", s.handleGenerate)
	mux.HandleFunc("POST /stop-generation", s.handleStopGeneration)
	mux.HandleFunc("POST /new-chat", s.handleNewChat)

	// Image serving endpoints
	mux.HandleFunc("GET /images/{id}", s.handleImage)
	mux.HandleFunc("GET /sessions/{sessionID}/images/{filename}", s.handleSessionImage)

	// Message state endpoint for loading historical snapshots
	mux.HandleFunc("GET /message/{id}/state", s.handleMessageState)

	// Health check endpoint for Electron
	mux.HandleFunc("GET /ready", s.handleReady)
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

	// Populate template data with default generation settings from CLI flags
	data := indexTemplateData{
		Steps:  s.defaultSteps,
		CFG:    s.defaultCFG,
		Seed:   s.defaultSeed,
		Width:  s.defaultWidth,
		Height: s.defaultHeight,
	}

	if err := s.templates.ExecuteTemplate(w, "index.html", data); err != nil {
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

	// Disable write deadline for chat requests since LLM streaming can take a long time.
	// Without this, the server's WriteTimeout (15s) would kill long-running requests.
	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Time{})

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

	// Parse generation settings from form data
	steps := s.parseSteps(r.FormValue("steps"))
	cfg := s.parseCFG(r.FormValue("cfg"))
	seed := s.parseSeed(r.FormValue("seed"))

	// Get session and update generation settings
	session := s.sessionManager.GetSession(sessionID)
	session.SetGenerationSettings(int(steps), cfg, seed)

	// Get conversation manager for this session
	manager := session.Manager()

	// ========================================================================
	// TWO-STAGE LLM FLOW
	// Stage 1 (Extraction): Call LLM with tools prompt to extract parameters
	// Stage 2 (Conversation): Call LLM with conversation prompt to generate text
	// When generate_image is true, Stage 2 and image generation run in parallel
	// ========================================================================

	// Build extraction context with tools prompt for Stage 1.
	// We do NOT call AddUserMessage yet - only add to history after successful response.
	extractionPrompt := s.buildExtractionPrompt()
	extractionContext := manager.BuildLLMContext(extractionPrompt, int(steps), cfg, seed)

	// Append the new user message to the context (but not to history yet)
	extractionContext = append(extractionContext, conversation.Message{
		Role:    conversation.RoleUser,
		Content: message,
	})

	// Convert to ollama messages
	ollamaMessages := make([]ollama.Message, len(extractionContext))
	for i, msg := range extractionContext {
		ollamaMessages[i] = ollama.Message{
			Role:    msg.Role,
			Content: msg.Content,
		}
	}

	// Send thinking event (small bubble) before Stage 1
	_ = s.broker.SendEvent(sessionID, EventAgentThinking, map[string]interface{}{
		"started": true,
	})

	// Stage 1: Extraction - call LLM with tools, no streaming.
	// Nil callback means no token streaming; the real ollama client and mock both handle this.
	tools := []ollama.Tool{ollama.UpdateGenerationTool()}
	extractionResult, err := s.chatWithRetry(r.Context(), sessionID, ollamaMessages, nil, tools, nil)

	if err != nil {
		// Stage 1 failed - use existing error handling
		if errors.Is(err, ollama.ErrMissingFields) {
			log.Printf("Stage 1 missing fields after retry for session %s, clearing context: %v", sessionID, err)
			manager.Clear()
			s.sendErrorEvent(sessionID, "I'm having trouble responding. Let's start fresh.")
		} else {
			log.Printf("Stage 1 error for session %s: %v", sessionID, err)
			s.sendErrorEvent(sessionID, "An error occurred while processing your message. Please try again.")
		}

		_ = s.broker.SendEvent(sessionID, EventAgentDone, AgentDoneData{
			Done:        true,
			MessageID:   0,
			HasSnapshot: false,
		})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status":"ok","session_id":"%s"}`, sessionID)
		return
	}

	log.Printf("Stage 1 result for session %s: HasToolCall=%v", sessionID, extractionResult.HasToolCall)

	// Extract metadata from Stage 1
	prompt := ""
	if extractionResult.HasToolCall && extractionResult.Metadata.Prompt != "" {
		prompt = extractionResult.Metadata.Prompt
		// SECURITY: Clamp LLM-extracted prompt to prevent DoS via oversized prompts
		// in SSE events, conversation history, and Stage 2 system prompt.
		if len(prompt) > MaxPromptLength {
			log.Printf("LLM-extracted prompt too long for session %s: %d bytes (clamped to %d)",
				sessionID, len(prompt), MaxPromptLength)
			prompt = prompt[:MaxPromptLength]
		}
	}
	generateImage := extractionResult.HasToolCall && extractionResult.Metadata.GenerateImage

	log.Printf("Stage 1 metadata: Prompt=%q, GenerateImage=%v, Steps=%d, CFG=%.2f, Seed=%d",
		extractionResult.Metadata.Prompt, extractionResult.Metadata.GenerateImage,
		extractionResult.Metadata.Steps, extractionResult.Metadata.CFG, extractionResult.Metadata.Seed)

	// Send expanded thinking event - Stage 1 complete, Stage 2 starting
	_ = s.broker.SendEvent(sessionID, EventAgentThinking, map[string]interface{}{
		"started":  true,
		"expanded": true,
	})

	// Process extraction results: update settings and prompt
	var clampedSteps int
	var clampedCFG float64
	var clampedSeed int64
	var clampedList []clampedSetting

	if extractionResult.HasToolCall {
		clampedSteps, clampedCFG, clampedSeed, clampedList = clampGenerationSettings(
			extractionResult.Metadata.Steps,
			extractionResult.Metadata.CFG,
			extractionResult.Metadata.Seed,
		)
		session.SetGenerationSettings(clampedSteps, clampedCFG, clampedSeed)

		_ = s.broker.SendEvent(sessionID, EventSettingsUpdate, map[string]interface{}{
			"steps": clampedSteps,
			"cfg":   clampedCFG,
			"seed":  clampedSeed,
		})
	}

	if prompt != "" {
		_ = s.broker.SendEvent(sessionID, EventPromptUpdate, map[string]string{
			"prompt": prompt,
		})
	}

	// Add user message to history now that Stage 1 succeeded
	manager.AddUserMessage(message)

	// Pre-create the assistant message so generation-started can reference its ID.
	// Content is empty now; it gets updated after Stage 2 streams the response.
	hasSnapshot := prompt != ""
	var metadata *ollama.LLMMetadata
	if extractionResult.HasToolCall {
		metadata = &extractionResult.Metadata
	}
	messageID := manager.AddAssistantMessage("", prompt, metadata)

	// If generating, start image generation in parallel with Stage 2
	if generateImage {
		currentPrompt := session.Manager().GetCurrentPrompt()
		if currentPrompt != "" && s.rateLimiter.allowGenerate(sessionID) {
			_ = s.broker.SendEvent(sessionID, EventGenerationStarted, map[string]interface{}{
				"source":     "agent",
				"message_id": messageID,
			})
			// Run image generation in a goroutine so Stage 2 can proceed in parallel.
			// Use context.Background() because r.Context() is cancelled when handleChat returns.
			// SECURITY: Orphaned goroutine risk is mitigated by generateImage's internal
			// 120s timeout (see generateImage). Rate limiting also bounds goroutine count.
			go func() {
				_ = s.generateImage(context.Background(), sessionID, currentPrompt, clampedSteps, clampedCFG, clampedSeed, messageID)
			}()
		} else if currentPrompt == "" {
			log.Printf("Skipping auto-generation for session %s: empty prompt", sessionID)
			s.sendErrorEvent(sessionID, "Cannot generate: no prompt available")
		} else {
			log.Printf("Rate limit exceeded for session %s (agent-triggered generation)", sessionID)
			s.sendErrorEvent(sessionID, "Too many generation requests. Please wait a moment.")
		}
	}

	// Stage 2: Conversation - call LLM with conversation prompt, streaming tokens.
	// BuildLLMContext includes the user message added on line 499.
	conversationPrompt := s.buildConversationPrompt(prompt, generateImage)
	convContext := manager.BuildLLMContext(conversationPrompt, int(steps), cfg, seed)

	convOllamaMessages := make([]ollama.Message, len(convContext))
	for i, msg := range convContext {
		convOllamaMessages[i] = ollama.Message{
			Role:    msg.Role,
			Content: msg.Content,
		}
	}

	// Stream Stage 2 tokens to SSE
	var responseText strings.Builder
	_, convErr := s.ollamaClient.Chat(r.Context(), convOllamaMessages, nil, nil, func(token ollama.StreamToken) error {
		if token.Content != "" {
			responseText.WriteString(token.Content)
			if err := s.broker.SendEvent(sessionID, EventAgentToken, map[string]string{
				"token":      token.Content,
				"message_id": fmt.Sprintf("%d", messageID),
			}); err != nil {
				return fmt.Errorf("client disconnected: %w", err)
			}
		}
		return nil
	})

	if convErr != nil {
		// Stage 2 failed - retry once
		log.Printf("Stage 2 failed for session %s, retrying: %v", sessionID, convErr)
		responseText.Reset()

		// Send retry event so UI clears partial message
		_ = s.broker.SendEvent(sessionID, EventAgentRetry, map[string]int{
			"attempt": 2,
		})

		_, convErr = s.ollamaClient.Chat(r.Context(), convOllamaMessages, nil, nil, func(token ollama.StreamToken) error {
			if token.Content != "" {
				responseText.WriteString(token.Content)
				if err := s.broker.SendEvent(sessionID, EventAgentToken, map[string]string{
					"token":      token.Content,
					"message_id": fmt.Sprintf("%d", messageID),
				}); err != nil {
					return fmt.Errorf("client disconnected: %w", err)
				}
			}
			return nil
		})

		if convErr != nil {
			// Both attempts failed - send canned error response
			log.Printf("Stage 2 retry failed for session %s: %v", sessionID, convErr)
			cannedResponse := "I'm sorry, I'm having trouble responding right now."
			_ = s.broker.SendEvent(sessionID, EventAgentToken, map[string]string{
				"token":      cannedResponse,
				"message_id": fmt.Sprintf("%d", messageID),
			})
			responseText.WriteString(cannedResponse)
		}
	}

	// Update the assistant message with the actual response text from Stage 2
	manager.UpdateAssistantMessage(messageID, responseText.String())

	// Send clamped feedback if applicable
	if feedback := formatClampedFeedback(clampedList); feedback != "" {
		log.Printf("Settings clamped for session %s: %s", sessionID, feedback)
		_ = s.broker.SendEvent(sessionID, EventAgentToken, map[string]string{
			"token": "\n\n[" + feedback + "]",
		})
	}

	// Finalize the message
	_ = s.broker.SendEvent(sessionID, EventAgentDone, AgentDoneData{
		Done:        true,
		MessageID:   messageID,
		HasSnapshot: hasSnapshot,
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"ok","session_id":"%s"}`, sessionID)
}

// buildExtractionPrompt builds the system prompt for Stage 1 (extraction).
// Uses the tools prompt (ara_tools.md) combined with function calling schema.
// Stage 1 extracts parameters via function calling without generating conversational text.
func (s *Server) buildExtractionPrompt() string {
	var prompt strings.Builder
	if s.agentToolsPrompt != "" {
		prompt.WriteString(s.agentToolsPrompt)
		prompt.WriteString("\n\n")
	} else {
		prompt.WriteString("Extract image generation parameters from user messages.\n\n")
	}

	// Add function calling schema
	prompt.WriteString("## Function Calling\n\n")
	prompt.WriteString("You have access to the `update_generation` function for updating image generation settings.\n\n")
	prompt.WriteString("Call the function with the appropriate parameters based on the user's message.\n\n")
	prompt.WriteString("Function parameters:\n")
	prompt.WriteString("- `prompt` (string): Image generation prompt (under 200 chars)\n")
	prompt.WriteString("- `steps` (integer, 1-100): Inference steps, default 4\n")
	prompt.WriteString("- `cfg` (number, 0-20): Guidance scale, default 1.0\n")
	prompt.WriteString("- `seed` (integer): Random seed, -1 for random\n")
	prompt.WriteString("- `generate_image` (boolean): true to generate, false to just update settings\n")

	return prompt.String()
}

// buildConversationPrompt builds the system prompt for Stage 2 (conversation).
// Uses the conversation prompt (ara.md) with context about what's being generated.
// Stage 2 generates contextual response text without function calling.
func (s *Server) buildConversationPrompt(extractedPrompt string, generating bool) string {
	var prompt strings.Builder
	if s.agentPrompt != "" {
		prompt.WriteString(s.agentPrompt)
		prompt.WriteString("\n\n")
	} else {
		prompt.WriteString("You are a helpful creative partner for image generation.\n\n")
	}

	// Add context about current state
	prompt.WriteString("## Current Context\n\n")
	if extractedPrompt != "" {
		fmt.Fprintf(&prompt, "Current prompt: \"%s\"\n", extractedPrompt)
	} else {
		prompt.WriteString("Current prompt: (none)\n")
	}
	if generating {
		prompt.WriteString("Generating: yes (image is being generated right now)\n")
	} else {
		prompt.WriteString("Generating: no\n")
	}

	return prompt.String()
}

// sendErrorEvent sends an error event to the client via SSE.
func (s *Server) sendErrorEvent(sessionID string, message string) {
	_ = s.broker.SendEvent(sessionID, EventError, map[string]string{
		"message": message,
	})
}

// clampedSetting tracks a single setting that was clamped.
type clampedSetting struct {
	name     string
	original string
	clamped  string
	reason   string
}

// clampGenerationSettings clamps agent-provided values to valid ranges.
// Returns the clamped values and a list of settings that were adjusted.
// Valid ranges: steps 1-100, cfg 0-20, seed >= -1
func clampGenerationSettings(steps int, cfg float64, seed int64) (int, float64, int64, []clampedSetting) {
	var clamped []clampedSetting

	// Clamp steps (1-100)
	clampedSteps := steps
	if steps < 1 {
		clampedSteps = 1
		clamped = append(clamped, clampedSetting{
			name:     "steps",
			original: fmt.Sprintf("%d", steps),
			clamped:  "1",
			reason:   "minimum is 1",
		})
	} else if steps > 100 {
		clampedSteps = 100
		clamped = append(clamped, clampedSetting{
			name:     "steps",
			original: fmt.Sprintf("%d", steps),
			clamped:  "100",
			reason:   "maximum is 100",
		})
	}

	// Clamp cfg (0-20)
	clampedCFG := cfg
	if cfg < 0 {
		clampedCFG = 0
		clamped = append(clamped, clampedSetting{
			name:     "cfg",
			original: fmt.Sprintf("%.1f", cfg),
			clamped:  "0.0",
			reason:   "minimum is 0",
		})
	} else if cfg > 20 {
		clampedCFG = 20
		clamped = append(clamped, clampedSetting{
			name:     "cfg",
			original: fmt.Sprintf("%.1f", cfg),
			clamped:  "20.0",
			reason:   "maximum is 20",
		})
	}

	// Clamp seed (>= -1)
	clampedSeed := seed
	if seed < -1 {
		clampedSeed = -1
		clamped = append(clamped, clampedSetting{
			name:     "seed",
			original: fmt.Sprintf("%d", seed),
			clamped:  "-1",
			reason:   "minimum is -1",
		})
	}

	return clampedSteps, clampedCFG, clampedSeed, clamped
}

// formatClampedFeedback builds a feedback message for clamped settings.
// Returns empty string if nothing was clamped.
func formatClampedFeedback(clamped []clampedSetting) string {
	if len(clamped) == 0 {
		return ""
	}

	parts := make([]string, len(clamped))
	for i, c := range clamped {
		parts[i] = fmt.Sprintf("%s %s→%s (%s)", c.name, c.original, c.clamped, c.reason)
	}

	return "Settings adjusted: " + strings.Join(parts, ", ")
}

// compactContext compacts conversation history into a single system message
// summarizing user intent. This is used as a recovery strategy when the LLM
// struggles with large context or fails to provide complete function calls.
//
// The compaction strategy is rule-based (not LLM-based): it extracts key words
// from user messages to summarize what the user wants. This reduces cognitive
// load on the LLM.
//
// Parameters:
//   - messages: Full conversation history
//
// Returns:
//   - Compacted message array with simplified system prompt
//
// Compaction logic:
//   - Extracts user messages (skips system and assistant messages)
//   - Identifies key words related to: subject, style, setting, mood
//   - Builds summary: "User wants: [extracted details]"
//   - Returns single system message with summary
//
// Example:
//
//	Input messages:
//	  [system] System prompt...
//	  [user] I want a cat in a hat
//	  [assistant] What kind of cat?
//	  [user] A tabby cat
//	  [user] Make it realistic
//
//	Output:
//	  [system] User wants: cat, hat, tabby, realistic.
//	           Use the update_generation function to set the generation parameters.
func (s *Server) compactContext(messages []ollama.Message) []ollama.Message {
	// Extract user messages and concatenate their content
	// WHY ONLY USER MESSAGES: User messages contain the requirements (what they want).
	// Assistant messages are the LLM's responses (which may be incomplete).
	// System messages are instructions (already in the system prompt). We only need
	// to preserve what the user wants, not the failed conversation.
	var userContent strings.Builder
	for _, msg := range messages {
		if msg.Role == ollama.RoleUser {
			// Skip system-injected messages (edit notifications, current prompt)
			// WHY SKIP SYSTEM-INJECTED: These are metadata messages like
			// "[User edited prompt to: ...]" that aren't user requirements.
			if strings.HasPrefix(msg.Content, "[") {
				continue
			}
			if userContent.Len() > 0 {
				userContent.WriteString(" ")
			}
			userContent.WriteString(msg.Content)
		}
	}

	// Extract key details from user messages
	// WHY RULE-BASED: We use simple string processing instead of asking an LLM
	// to summarize. This avoids:
	// - Additional LLM call (slow, may fail)
	// - Compounding errors (if summarization LLM also fails)
	// - Unpredictable summarization (deterministic is better for debugging)
	content := userContent.String()
	content = strings.ToLower(content)

	// Remove common filler words to focus on key details
	// WHY REMOVE FILLERS: Filler words add noise without meaning. For example:
	// "I want a cat in a hat" → "cat hat"
	// This reduces token count and makes the summary clearer to the LLM.
	fillers := []string{
		"i want", "i'd like", "can you", "please", "make", "create",
		"generate", "a ", "an ", "the ", "some ", "of ", "in ", "on ",
		"with ", "and ", "or ", "but ", "that ", "this ", "these ", "those",
	}
	for _, filler := range fillers {
		content = strings.ReplaceAll(content, filler, " ")
	}

	// Clean up extra whitespace
	content = strings.Join(strings.Fields(content), " ")

	// Truncate to reasonable length (avoid sending huge compacted messages)
	// WHY TRUNCATE: If the user wrote a novel, we don't want to send it all.
	// 200 chars is enough to capture key details while keeping the context small.
	const maxSummaryLen = 200
	if len(content) > maxSummaryLen {
		content = content[:maxSummaryLen] + "..."
	}

	// Build compacted system message
	// WHY INCLUDE USER SUMMARY: The LLM still needs to know what the user wants
	// to generate a valid prompt. The summary preserves this context while
	// discarding the failed conversation history.
	summary := fmt.Sprintf(`User wants: %s

Call the update_generation function with appropriate parameters based on the user's request.
Set generate_image to true if you have enough information to create a prompt, false otherwise.`, content)

	// Return single system message with compacted context
	// WHY SINGLE MESSAGE: We replace the entire conversation (potentially dozens
	// of messages) with a single system message. This:
	// - Reduces token count (faster, cheaper)
	// - Eliminates confusing context (simpler for LLM)
	// - Focuses on the task: call update_generation function
	return []ollama.Message{
		{
			Role:    ollama.RoleSystem,
			Content: summary,
		},
	}
}

// chatWithRetry calls the ollama client's Chat method with automatic retry
// on context overflow errors.
//
// This implements context compaction recovery: when the conversation context
// becomes too large or the LLM fails to respond properly, we compact the
// conversation history into a summary and retry once.
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - sessionID: Session ID for SSE event routing
//   - messages: Conversation history
//   - seed: Optional seed for deterministic responses
//   - tools: Function calling tools to send with the request
//   - callback: Function called for each streamed token
//
// Returns:
//   - ChatResult: Parsed result with conversational text and metadata
//   - error: Returns error if both initial attempt and compaction retry fail
//
// Retry behavior:
//   - Missing fields errors trigger compaction retry (likely context issue)
//   - Other errors (connection, timeout, etc.) are returned immediately
//   - Maximum 2 total attempts (initial + 1 compaction retry)
//   - Retry count is per-request, not cumulative across conversation
func (s *Server) chatWithRetry(ctx context.Context, sessionID string, messages []ollama.Message, seed *int64, tools []ollama.Tool, callback ollama.StreamCallback) (ollama.ChatResult, error) {
	// Try initial request
	result, err := s.ollamaClient.Chat(ctx, messages, seed, tools, callback)
	if err == nil {
		return result, nil
	}

	// Check if this is a missing fields error (might be fixed by compaction)
	// WHY RETRY ON MISSING FIELDS: Missing fields in function calls often indicates
	// the LLM is struggling with context. Compaction can help by simplifying the
	// conversation history.
	isMissingFieldsError := errors.Is(err, ollama.ErrMissingFields)

	if !isMissingFieldsError {
		// Non-retryable error (connection, timeout, etc.) - return immediately
		return ollama.ChatResult{}, err
	}

	// Missing fields error - try context compaction
	log.Printf("Missing fields error, trying context compaction: %v", err)

	// Send retry event to UI so it can clear the partial streaming message
	_ = s.broker.SendEvent(sessionID, EventAgentRetry, map[string]int{
		"attempt": 2, // Compaction retry attempt
	})

	// Compact conversation context to reduce cognitive load
	compactedMessages := s.compactContext(messages)
	result, compactErr := s.ollamaClient.Chat(ctx, compactedMessages, seed, tools, callback)

	if compactErr == nil {
		// Compaction retry succeeded
		log.Printf("Context compaction retry succeeded")
		return result, nil
	}

	// Compaction retry failed - return the error
	log.Printf("Context compaction retry failed: %v", compactErr)
	return ollama.ChatResult{}, compactErr
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
	session := s.sessionManager.GetSession(sessionID)
	manager := session.Manager()

	// Update the prompt (this sets the edited flag if changed)
	manager.UpdatePrompt(prompt)

	// Notify that the prompt was edited (injects system message if changed)
	manager.NotifyPromptEdited()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"ok","session_id":"%s"}`, sessionID)
}

// handleNewChat clears the conversation history for the current session.
// This allows users to start a fresh conversation without stale context.
func (s *Server) handleNewChat(w http.ResponseWriter, r *http.Request) {
	sessionID := GetSessionID(r.Context())

	// Get session and clear its conversation
	session := s.sessionManager.GetSession(sessionID)
	manager := session.Manager()
	manager.Clear()

	log.Printf("Cleared conversation for session %s", sessionID)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"ok","session_id":"%s"}`, sessionID)
}

// generateImage performs image generation using the compute process.
// It handles the entire generation flow: protocol request creation, compute communication,
// response handling, and SSE event sending. This method is called from both handleGenerate
// (manual button click) and handleChat (agent-triggered generation).
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - sessionID: Session ID for SSE event routing
//   - prompt: Image generation prompt (already validated and truncated)
//   - steps: Number of inference steps (1-100)
//   - cfg: CFG scale (0-20)
//   - seed: Random seed (-1 for random, >= 0 for deterministic)
//   - messageID: Optional message ID to associate the image with (0 means no association)
//
// Returns:
//   - error: Connection or generation error (for HTTP status code handling in handleGenerate)
func (s *Server) generateImage(ctx context.Context, sessionID string, prompt string, steps int, cfg float64, seed int64, messageID int) error {
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
	}

	log.Printf("Generation settings for session %s: steps=%d, cfg=%.2f, seed=%d",
		sessionID, steps, cfg, seed)

	// Generate unique request ID
	reqID := atomic.AddUint64(&s.requestID, 1)

	// Create protocol request
	// 768x768 balances quality and VRAM usage; 1024x1024 causes OOM during VAE decode
	width, height := uint32(768), uint32(768)
	cfgScale := float32(cfg)

	// Convert seed to uint64 for protocol
	// seed=-1 means random (use 0 in protocol)
	seedValue := uint64(0)
	if seed != -1 {
		seedValue = uint64(seed)
	}

	protoReq, err := protocol.NewSD35GenerateRequest(reqID, prompt, width, height, uint32(steps), cfgScale, seedValue)
	if err != nil {
		log.Printf("Failed to create protocol request for session %s: %v", sessionID, err)
		s.sendErrorEvent(sessionID, "Failed to create generation request: invalid prompt")
		return fmt.Errorf("failed to create protocol request: %w", err)
	}

	// Set preview interval to 5 (preview every 5 steps)
	protoReq.PreviewInterval = 5

	// Encode request
	requestData, err := protocol.EncodeSD35GenerateRequest(protoReq)
	if err != nil {
		log.Printf("Failed to encode protocol request for session %s: %v", sessionID, err)
		s.sendErrorEvent(sessionID, "Failed to encode generation request")
		return fmt.Errorf("failed to encode request: %w", err)
	}

	// Use persistent compute connection
	if s.computeClient == nil {
		log.Printf("Compute client not available for session %s", sessionID)
		s.sendErrorEvent(sessionID, "Image generation is not available (compute process not connected)")
		return client.ErrComputeNotRunning
	}

	// Send request and receive streaming response over persistent connection
	genCtx, cancel := context.WithTimeout(ctx, 120*time.Second) // 2 min timeout for generation
	defer cancel()

	// Create a separate cancellable context for stop functionality.
	// This wraps genCtx so the stop handler can cancel generation independently
	// of the timeout, without cancelling the parent context.
	stopCtx, stopCancel := context.WithCancel(genCtx)
	defer stopCancel()

	// Register stop cancel for this session so handleStopGeneration can cancel us.
	s.activeGenMu.Lock()
	s.activeGenCancel[sessionID] = stopCancel
	s.activeGenMu.Unlock()

	// Deregister when generateImage returns so stale cancel funcs don't accumulate.
	defer func() {
		s.activeGenMu.Lock()
		delete(s.activeGenCancel, sessionID)
		s.activeGenMu.Unlock()
	}()

	responseCh, err := s.computeClient.SendStream(stopCtx, requestData)
	if err != nil {
		log.Printf("Failed to send request to compute process for session %s: %v", sessionID, err)
		if errors.Is(err, client.ErrConnectionClosed) || errors.Is(err, client.ErrReaderDead) {
			s.sendErrorEvent(sessionID, "Connection to image generation service was closed")
		} else if errors.Is(err, client.ErrReadTimeout) {
			s.sendErrorEvent(sessionID, "Image generation timed out. Try a simpler prompt.")
		} else {
			s.sendErrorEvent(sessionID, "Failed to generate image")
		}
		return fmt.Errorf("failed to send request: %w", err)
	}

	// Process streaming responses (progress, preview, final)
	for responseData := range responseCh {
		if responseData == nil {
			// Channel closed without data - check for error
			log.Printf("Response channel closed without data for session %s", sessionID)
			s.sendErrorEvent(sessionID, "Connection to image generation service was closed")
			return client.ErrConnectionClosed
		}

		// Decode response
		response, err := protocol.DecodeResponse(responseData)
		if err != nil {
			log.Printf("Failed to decode response for session %s: %v", sessionID, err)
			s.sendErrorEvent(sessionID, "Failed to decode image generation response")
			return fmt.Errorf("failed to decode response: %w", err)
		}

		// Handle response type
		switch resp := response.(type) {
		case *protocol.SD35ProgressEvent:
			log.Printf("Progress for session %s: step %d/%d (%dms)",
				sessionID, resp.Step, resp.TotalSteps, resp.StepTimeMs)
			// Send progress event via SSE
			_ = s.broker.SendEvent(sessionID, EventGenerationProgress, GenerationProgressData{
				Step:       int(resp.Step),
				TotalSteps: int(resp.TotalSteps),
				MessageID:  messageID,
			})

		case *protocol.SD35PreviewEvent:
			// Skip preview storage if no message association (messageID=0)
			if messageID <= 0 {
				continue
			}

			// Encode preview image as PNG
			var format image.PixelFormat
			if resp.Channels == 3 {
				format = image.FormatRGB
			} else {
				format = image.FormatRGBA
			}

			pngData, err := image.EncodePNG(int(resp.ImageWidth), int(resp.ImageHeight), resp.ImageData, format)
			if err != nil {
				log.Printf("Failed to encode preview PNG for session %s: %v", sessionID, err)
				// Don't fail the whole generation for a preview encoding error
				continue
			}

			// Store preview image in persistent session storage
			if err := s.imageStore.SavePreview(sessionID, messageID, pngData); err != nil {
				log.Printf("Failed to save preview image for session %s: %v", sessionID, err)
				continue
			}

			// Send preview event via SSE with URL pointing to persistent storage
			// Include step number as cache-buster so the browser re-fetches each update
			previewURL := s.imageStore.GetPreviewURL(sessionID, messageID) + fmt.Sprintf("?step=%d", resp.Step)
			_ = s.broker.SendEvent(sessionID, EventGenerationPreview, GenerationPreviewData{
				URL:       previewURL,
				Width:     int(resp.ImageWidth),
				Height:    int(resp.ImageHeight),
				MessageID: messageID,
			})

		case *protocol.SD35GenerateResponse:
			// Success - convert raw pixels to PNG
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
				return fmt.Errorf("failed to encode PNG: %w", err)
			}

			// Determine storage strategy based on message ID
			var imageURL string
			if messageID > 0 {
				// Save to persistent session-specific storage
				if err := s.imageStore.Save(sessionID, messageID, pngData); err != nil {
					log.Printf("Failed to save session image for session %s, message %d: %v", sessionID, messageID, err)
					s.sendErrorEvent(sessionID, "Failed to save image. Please try again.")
					return fmt.Errorf("failed to save session image: %w", err)
				}

				// Update message preview status to complete
				session := s.sessionManager.GetSession(sessionID)
				manager := session.Manager()
				manager.UpdateMessagePreview(messageID, conversation.PreviewStatusComplete, s.imageStore.GetURL(sessionID, messageID))

				// Clean up preview file - final image supersedes it
				if err := s.imageStore.DeletePreview(sessionID, messageID); err != nil {
					log.Printf("Failed to delete preview for session %s, message %d: %v", sessionID, messageID, err)
					// Non-fatal: preview cleanup failure does not affect the final image
				}

				imageURL = s.imageStore.GetURL(sessionID, messageID)
				log.Printf("Saved image to session storage: %s", imageURL)
			} else {
				// Use in-memory storage (fallback for legacy/non-message generation)
				imageID, err := s.imageStorage.Store(pngData, int(resp.ImageWidth), int(resp.ImageHeight))
				if err != nil {
					log.Printf("Failed to store image for session %s: %v", sessionID, err)
					if errors.Is(err, image.ErrImageTooLarge) {
						s.sendErrorEvent(sessionID, "Image is too large to store")
					} else {
						s.sendErrorEvent(sessionID, "Failed to store image. Please try again.")
					}
					return fmt.Errorf("failed to store image: %w", err)
				}
				imageURL = fmt.Sprintf("/images/%s.png", imageID)
			}

			log.Printf("Generated image for session %s: %dx%d in %dms",
				sessionID, resp.ImageWidth, resp.ImageHeight, resp.GenerationTime)

			// Send image-ready event with message ID
			_ = s.broker.SendEvent(sessionID, EventImageReady, ImageReadyData{
				URL:       imageURL,
				Width:     int(resp.ImageWidth),
				Height:    int(resp.ImageHeight),
				MessageID: messageID,
			})

		case *protocol.ErrorResponse:
			log.Printf("Compute process error for session %s: code=%d, msg=%s",
				sessionID, resp.ErrorCode, resp.ErrorMessage)
			s.sendErrorEvent(sessionID, fmt.Sprintf("Image generation failed: %s", resp.ErrorMessage))
			return fmt.Errorf("compute error: %s", resp.ErrorMessage)

		default:
			log.Printf("Unexpected response type for session %s: %T", sessionID, response)
			s.sendErrorEvent(sessionID, "Unexpected response from image generation service")
			return fmt.Errorf("unexpected response type: %T", response)
		}
	}

	// Check if generation was stopped by user (context cancelled by handleStopGeneration).
	if stopCtx.Err() != nil && messageID > 0 {
		log.Printf("Generation stopped by user for session %s, message %d", sessionID, messageID)

		// Update message preview status to stopped, keeping the last preview URL if one exists.
		session := s.sessionManager.GetSession(sessionID)
		manager := session.Manager()
		previewURL := ""
		if s.imageStore.PreviewExists(sessionID, messageID) {
			previewURL = s.imageStore.GetPreviewURL(sessionID, messageID)
		}
		manager.UpdateMessagePreview(messageID, conversation.PreviewStatusStopped, previewURL)

		// Send generation-stopped SSE event so the UI can update the bubble.
		_ = s.broker.SendEvent(sessionID, EventGenerationStopped, GenerationStoppedData{
			MessageID:  messageID,
			PreviewURL: previewURL,
		})

		return nil // Not an error - user intentionally stopped.
	}

	return nil
}

// handleGenerate handles image generation requests.
// It reads the current prompt from the conversation manager and triggers generation
// using the shared generateImage helper. The response (image or error) is sent via SSE.
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

	// Get session for this request
	session := s.sessionManager.GetSession(sessionID)
	manager := session.Manager()

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

	// Parse generation settings from form data
	steps := s.parseSteps(r.FormValue("steps"))
	cfg := s.parseCFG(r.FormValue("cfg"))
	seed := s.parseSeed(r.FormValue("seed"))

	// Parse optional message_id parameter
	// If provided, the generated image will be associated with that message
	messageID := 0
	if messageIDStr := r.FormValue("message_id"); messageIDStr != "" {
		var err error
		messageID, err = strconv.Atoi(messageIDStr)
		if err != nil || messageID <= 0 {
			log.Printf("Invalid message_id for session %s: %s", sessionID, messageIDStr)
			messageID = 0 // Reset to 0 if invalid
		}
	}

	// Store settings in session for consistency
	session.SetGenerationSettings(int(steps), cfg, seed)

	// Send generation-started event with message ID if provided
	eventData := map[string]interface{}{
		"source": "manual",
	}
	if messageID > 0 {
		eventData["message_id"] = messageID
	}
	_ = s.broker.SendEvent(sessionID, EventGenerationStarted, eventData)

	// Call shared generation logic
	err := s.generateImage(r.Context(), sessionID, prompt, int(steps), cfg, seed, messageID)
	if err != nil {
		// Error already sent via SSE and logged
		// Determine appropriate HTTP status code based on error type
		var statusCode int
		if errors.Is(err, client.ErrComputeNotRunning) || errors.Is(err, client.ErrXDGNotSet) {
			statusCode = http.StatusServiceUnavailable
		} else {
			statusCode = http.StatusInternalServerError
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		fmt.Fprintf(w, `{"status":"error","message":"generation failed"}`)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"ok","session_id":"%s"}`, sessionID)
}

// handleStopGeneration cancels any active image generation for the session.
// It returns immediately after cancelling; the compute process restart happens
// in a background goroutine to avoid blocking the HTTP response.
//
// POST /stop-generation
func (s *Server) handleStopGeneration(w http.ResponseWriter, r *http.Request) {
	sessionID := GetSessionID(r.Context())

	// SECURITY: Check rate limit (reuse generate limiter).
	if !s.rateLimiter.allowGenerate(sessionID) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprintf(w, `{"status":"error","message":"rate limit exceeded"}`)
		return
	}

	// Look up and remove the active generation cancel function for this session.
	s.activeGenMu.Lock()
	cancel, ok := s.activeGenCancel[sessionID]
	s.activeGenMu.Unlock()

	if !ok {
		// No active generation - nothing to stop.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status":"ok","message":"no active generation"}`)
		return
	}

	// Cancel the generation context. generateImage will detect this after
	// the response loop and send the generation-stopped SSE event.
	cancel()

	// Restart compute process in the background.
	// The old compute process may be mid-inference and unresponsive; killing and
	// respawning is the only reliable way to get it back to a clean state.
	if s.computeRestartFunc != nil {
		go func() {
			restartCtx, restartCancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer restartCancel()

			newConn, err := s.computeRestartFunc(restartCtx)
			if err != nil {
				log.Printf("Failed to restart compute process: %v", err)
				return
			}
			s.SetComputeClient(newConn)
			log.Printf("Compute process restarted successfully")
		}()
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"ok"}`)
}

// generatePlaceholderPixels creates a colored gradient for testing.
// This will be replaced with actual compute process output.
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

// handleSessionImage serves a session-specific image by message ID.
// GET /sessions/{sessionID}/images/{messageID}.png
func (s *Server) handleSessionImage(w http.ResponseWriter, r *http.Request) {
	// SECURITY: Get authenticated session ID from context
	authenticatedSessionID := GetSessionID(r.Context())
	if authenticatedSessionID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Extract session ID and filename from path parameters
	requestedSessionID := r.PathValue("sessionID")
	filename := r.PathValue("filename")

	if requestedSessionID == "" || filename == "" {
		http.Error(w, "Missing session ID or filename", http.StatusBadRequest)
		return
	}

	// Determine whether the request is for a preview image or a final image.
	// Preview filename format: {messageID}_preview.png
	// Final image filename format: {messageID}.png
	isPreview := strings.HasSuffix(filename, "_preview.png")

	var messageIDStr string
	if isPreview {
		messageIDStr = strings.TrimSuffix(filename, "_preview.png")
	} else {
		messageIDStr = strings.TrimSuffix(filename, ".png")
		if messageIDStr == filename {
			// No .png extension found
			http.Error(w, "Invalid image filename (must be .png)", http.StatusBadRequest)
			return
		}
	}

	// SECURITY: Verify that the requesting session matches the sessionID in the path
	// This prevents users from accessing images from other sessions
	if authenticatedSessionID != requestedSessionID {
		log.Printf("SECURITY: Session %s attempted to access images from session %s", authenticatedSessionID, requestedSessionID)
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	// Parse message ID as integer
	messageID, err := strconv.Atoi(messageIDStr)
	if err != nil || messageID <= 0 {
		http.Error(w, "Invalid message ID", http.StatusBadRequest)
		return
	}

	// Load image from persistent storage.
	// Preview images use a separate file ({messageID}_preview.png).
	var pngData []byte
	if isPreview {
		pngData, err = s.imageStore.LoadPreview(requestedSessionID, messageID)
	} else {
		pngData, err = s.imageStore.Load(requestedSessionID, messageID)
	}
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.Error(w, "Image not found", http.StatusNotFound)
			return
		}
		log.Printf("Failed to load session image %s/%d (preview=%v): %v", requestedSessionID, messageID, isPreview, err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Set headers for image serving.
	// Preview images must not be cached as immutable because they are overwritten
	// each time a new preview frame arrives during generation. Final images are
	// content-addressed and never change, so they can be cached indefinitely.
	w.Header().Set("Content-Type", "image/png")
	if isPreview {
		w.Header().Set("Cache-Control", "private, max-age=300")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}

	// Write PNG data
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(pngData); err != nil {
		log.Printf("Failed to write image data for session %s, message %d: %v", requestedSessionID, messageID, err)
	}
}

// setOllamaClientForTesting replaces the ollama client with a test mock.
// This is only used in tests to inject mock implementations.
func (s *Server) setOllamaClientForTesting(client ollamaClient) {
	s.ollamaClient = client
}

// parseSteps parses the steps value from form data.
// Returns the parsed value if valid (1-100), otherwise returns default.
func (s *Server) parseSteps(value string) uint32 {
	if value == "" {
		return uint32(s.defaultSteps)
	}

	// Parse as int64 first to handle negative values
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed < 1 || parsed > 100 {
		return uint32(s.defaultSteps)
	}

	return uint32(parsed)
}

// parseCFG parses the CFG scale value from form data.
// Returns the parsed value if valid (0-20), otherwise returns default.
func (s *Server) parseCFG(value string) float64 {
	if value == "" {
		return s.defaultCFG
	}

	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil || parsed < 0 || parsed > 20 {
		return s.defaultCFG
	}

	return parsed
}

// parseSeed parses the seed value from form data.
// Returns the parsed value if valid (>= -1), otherwise returns default.
// -1 means random, 0+ are deterministic seeds.
func (s *Server) parseSeed(value string) int64 {
	if value == "" {
		return s.defaultSeed
	}

	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed < -1 {
		return s.defaultSeed
	}

	return parsed
}

// messageStateResponse is the JSON response for the message state endpoint.
type messageStateResponse struct {
	MessageID     int     `json:"message_id"`
	Prompt        string  `json:"prompt"`
	Steps         int     `json:"steps"`
	CFG           float64 `json:"cfg"`
	Seed          int64   `json:"seed"`
	PreviewStatus string  `json:"preview_status"`
	PreviewURL    string  `json:"preview_url"`
}

// handleMessageState handles requests to load historical message state.
// GET /message/{id}/state
// Returns the snapshot data for a given message ID.
func (s *Server) handleMessageState(w http.ResponseWriter, r *http.Request) {
	sessionID := GetSessionID(r.Context())

	// Extract message ID from path parameter
	idStr := r.PathValue("id")
	if idStr == "" {
		http.Error(w, "Missing message ID", http.StatusBadRequest)
		return
	}

	// Parse message ID as integer
	messageID, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid message ID", http.StatusBadRequest)
		return
	}

	// Get session and manager
	session := s.sessionManager.GetSession(sessionID)
	manager := session.Manager()

	// Look up message by ID
	msg := manager.GetMessage(messageID)
	if msg == nil {
		http.Error(w, "Message not found", http.StatusNotFound)
		return
	}

	// Check if message has a snapshot
	if msg.Snapshot == nil {
		http.Error(w, "Message has no snapshot", http.StatusNotFound)
		return
	}

	// Build response from snapshot
	response := messageStateResponse{
		MessageID:     msg.ID,
		Prompt:        msg.Snapshot.Prompt,
		Steps:         msg.Snapshot.Steps,
		CFG:           msg.Snapshot.CFG,
		Seed:          msg.Snapshot.Seed,
		PreviewStatus: msg.Snapshot.PreviewStatus,
		PreviewURL:    msg.Snapshot.PreviewURL,
	}

	// Return JSON response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Failed to encode message state response: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
}

// handleReady is a health check endpoint for Electron.
// Returns HTTP 200 with JSON {"status":"ready"} when the server is ready.
func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"ready"}`)
}
