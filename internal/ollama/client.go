package ollama

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"syscall"
	"time"
)

// Sentinel errors for ollama client operations
var (
	// ErrNotRunning is returned when ollama is not running at the configured endpoint
	ErrNotRunning = errors.New("ollama not running")
	// ErrModelNotFound is returned when the requested model is not available
	ErrModelNotFound = errors.New("model not available in ollama")
	// ErrConnectionTimeout is returned when the connection times out
	ErrConnectionTimeout = errors.New("ollama connection timeout")
	// ErrRequestFailed is returned when an API request fails
	ErrRequestFailed = errors.New("ollama request failed")
	// ErrConnectionFailed is returned when connection fails for unknown reasons
	ErrConnectionFailed = errors.New("ollama connection failed")
)

// Client provides methods to communicate with the ollama API.
type Client struct {
	endpoint   string
	model      string
	httpClient *http.Client
}

// NewClient creates a new ollama client with default settings.
// The client connects to http://localhost:11434 with a 60-second timeout.
func NewClient() *Client {
	return &Client{
		endpoint: DefaultEndpoint,
		model:    DefaultModel,
		httpClient: &http.Client{
			Timeout: time.Duration(DefaultTimeout) * time.Second,
		},
	}
}

// NewClientWithConfig creates a new ollama client with custom configuration.
// Parameters:
//   - endpoint: Ollama API endpoint URL (e.g., "http://localhost:11434")
//   - model: Model name (e.g., "llama3.2:1b")
//   - timeout: HTTP timeout for non-streaming requests
func NewClientWithConfig(endpoint, model string, timeout time.Duration) *Client {
	return &Client{
		endpoint: endpoint,
		model:    model,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// Connect verifies that ollama is reachable and the required model is available.
// It makes a GET request to /api/tags to check connectivity and model availability.
//
// Returns ErrNotRunning if ollama is not reachable.
// Returns ErrModelNotFound if the configured model is not available.
// Returns ErrConnectionTimeout if the connection times out.
func (c *Client) Connect(ctx context.Context) error {
	url := c.endpoint + EndpointTags

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		classified := c.classifyError(err)
		// Add endpoint context and actionable suggestion for connection errors
		if errors.Is(classified, ErrNotRunning) {
			return fmt.Errorf("%w at %s (start with: ollama serve)", ErrNotRunning, c.endpoint)
		}
		return classified
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: unexpected status %d", ErrRequestFailed, resp.StatusCode)
	}

	var tagsResp TagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&tagsResp); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	// Check if the required model is available
	modelFound := false
	for _, model := range tagsResp.Models {
		if model.Name == c.model {
			modelFound = true
			break
		}
	}

	if !modelFound {
		return fmt.Errorf("%w: %s (pull with: ollama pull %s)", ErrModelNotFound, c.model, c.model)
	}

	return nil
}

// Model returns the configured model name.
func (c *Client) Model() string {
	return c.model
}

// Endpoint returns the configured endpoint URL.
func (c *Client) Endpoint() string {
	return c.endpoint
}

// StreamCallback is called for each token received during streaming.
// The callback receives the token text and a done flag indicating completion.
// If the callback returns an error, streaming is aborted.
type StreamCallback func(token StreamToken) error

// Chat sends a chat request to ollama and streams the response.
// It posts to /api/chat with the conversation history and streams tokens
// as they arrive via the callback function.
//
// Parameters:
//   - ctx: Context for cancellation and timeout. IMPORTANT: Use context.WithTimeout
//     to prevent indefinite hangs if ollama stops responding. There is no default
//     timeout on streaming requests.
//   - messages: Conversation history (system prompt should be first). Must not be empty.
//   - seed: Optional seed for deterministic responses. nil = random (ollama default),
//     any non-nil value (including 0) produces deterministic output with that seed.
//   - callback: Function called for each streamed token (may be nil to collect silently)
//
// Returns the parsed ChatResult containing conversational text and metadata.
// The callback is called for each token as it arrives, with Done=true on the final token.
//
// Returns ErrNotRunning if ollama is not reachable.
// Returns an error if messages is empty.
// Returns ErrMissingDelimiter, ErrInvalidJSON, or ErrMissingFields if response parsing fails.
func (c *Client) Chat(ctx context.Context, messages []Message, seed *int64, callback StreamCallback) (ChatResult, error) {
	// Validate messages
	if len(messages) == 0 {
		return ChatResult{}, errors.New("messages cannot be empty")
	}

	// Validate message roles to prevent system prompt injection.
	// Only the first message may have role="system". This prevents
	// callers from injecting additional system prompts mid-conversation
	// to manipulate LLM behavior.
	for i, msg := range messages {
		switch msg.Role {
		case RoleSystem:
			if i != 0 {
				return ChatResult{}, errors.New("system message must be first in conversation")
			}
		case RoleUser, RoleAssistant:
			// Valid roles
		default:
			return ChatResult{}, fmt.Errorf("invalid message role: %q", msg.Role)
		}
	}

	url := c.endpoint + EndpointChat

	// Build request body
	chatReq := ChatRequest{
		Model:    c.model,
		Messages: messages,
		Stream:   true,
	}

	// Add seed if provided
	if seed != nil {
		chatReq.Options = &ChatOptions{Seed: seed}
	}

	body, err := json.Marshal(chatReq)
	if err != nil {
		return ChatResult{}, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return ChatResult{}, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Use a separate client without timeout for streaming.
	// We cannot use c.httpClient because its 60-second timeout would
	// terminate long-running LLM streams. Context cancellation handles
	// request lifetime instead.
	streamClient := &http.Client{}
	resp, err := streamClient.Do(req)
	if err != nil {
		classified := c.classifyError(err)
		if errors.Is(classified, ErrNotRunning) {
			return ChatResult{}, fmt.Errorf("%w at %s (start with: ollama serve)", ErrNotRunning, c.endpoint)
		}
		return ChatResult{}, classified
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Read error body for diagnostic information
		errBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 1024))
		if readErr != nil {
			return ChatResult{}, fmt.Errorf("%w: status %d (failed to read error: %v)", ErrRequestFailed, resp.StatusCode, readErr)
		}
		return ChatResult{}, fmt.Errorf("%w: status %d: %s", ErrRequestFailed, resp.StatusCode, string(errBody))
	}

	// Parse streaming response (newline-delimited JSON)
	fullResponse, err := c.parseStreamingResponse(resp.Body, callback)
	if err != nil {
		return ChatResult{}, err
	}

	// Parse the response to extract conversational text and metadata
	conversationalText, metadata, err := parseResponse(fullResponse)
	if err != nil {
		return ChatResult{}, err
	}

	return ChatResult{
		Response:    conversationalText,
		Metadata:    metadata,
		RawResponse: fullResponse,
	}, nil
}

// Maximum response size to prevent unbounded memory usage (1 MB)
const maxResponseSize = 1024 * 1024

// parseStreamingResponse reads newline-delimited JSON from the response body
// and calls the callback for each token.
//
// This function implements delimiter detection for structured output. The LLM is
// required to format responses as:
//
//	[conversational text]
//	---
//	{"prompt": "...", "steps": N, "cfg": X.X, "seed": N}
//
// WHY DELIMITER DETECTION:
// We want to preserve the streaming UX (live typing effect) for conversational
// text while hiding the JSON metadata from the user. The delimiter signals when
// to stop displaying tokens and start buffering JSON.
//
// WHY STREAMING STOPS AT DELIMITER:
// The JSON portion contains structured metadata (prompt, generation settings) that should
// not be displayed in the chat UI. By stopping the callback when we detect "---",
// we ensure the user sees only conversational text. The JSON is buffered silently
// and parsed after the stream completes.
//
// DELIMITER SPLIT HANDLING:
// The delimiter may appear in the middle of a token (ollama's tokenization doesn't
// respect our delimiter boundary). For example, a token might be "text---{". We
// must split this token:
// - "text" goes to callback (conversational text)
// - "---{" goes to JSON buffer (metadata)
//
// Without this split logic, we'd either:
// - Show part of the delimiter/JSON to the user (bad UX)
// - Skip the pre-delimiter text in that token (lost content)
//
// RESPONSE SIZE LIMIT:
// We enforce a 1MB limit to prevent unbounded memory usage if the LLM generates
// an extremely long response (malicious or malfunctioning model).
func (c *Client) parseStreamingResponse(body io.Reader, callback StreamCallback) (string, error) {
	scanner := bufio.NewScanner(body)
	var fullResponse bytes.Buffer
	var jsonBuffer bytes.Buffer
	delimiterFound := false

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			// Skip empty lines in newline-delimited JSON stream.
			// WHY SKIP: Ollama's streaming format may include blank lines between
			// JSON objects. These are not part of the protocol and should be ignored.
			continue
		}

		// Parse the JSON object for this chunk.
		// WHY UNMARSHAL PER LINE: Ollama's streaming format sends one JSON object
		// per line. Each object contains a token in the Message.Content field.
		var chatResp ChatResponse
		if err := json.Unmarshal(line, &chatResp); err != nil {
			// Parsing failed - malformed JSON from ollama.
			// WHY FAIL IMMEDIATELY: If ollama sends malformed JSON, we can't trust
			// the rest of the stream. Better to fail fast than continue with corrupted data.
			return "", fmt.Errorf("failed to parse response: %w", err)
		}

		token := chatResp.Message.Content

		// Append token to full response (always include everything).
		// WHY ALWAYS APPEND: We need the complete response for parsing and storage,
		// regardless of what gets displayed. The fullResponse includes both
		// conversational text and JSON metadata. Even if we stop displaying tokens
		// at the delimiter, we must preserve everything for parseResponse().
		fullResponse.WriteString(token)

		// Check response size limit to prevent unbounded memory usage
		// WHY: Without a limit, a malicious or malfunctioning LLM could exhaust
		// server memory by generating an infinite response.
		if fullResponse.Len() > maxResponseSize {
			return fullResponse.String(), fmt.Errorf("response too large (>%d bytes)", maxResponseSize)
		}

		// If delimiter already found, buffer everything as JSON
		// WHY: Once we've seen "---", everything after it is JSON metadata that
		// should not be displayed to the user. We buffer it for parsing after
		// the stream completes.
		if delimiterFound {
			jsonBuffer.WriteString(token)
		} else {
			// Check if this token contains the delimiter
			// WHY: We check each token because the delimiter can appear at any time.
			// Ollama's tokenization doesn't align with our delimiter, so we must
			// search within each token.
			delimiterIndex := bytes.Index([]byte(token), []byte(ResponseDelimiter))
			if delimiterIndex != -1 {
				// Delimiter found in this token - split it
				delimiterFound = true

				// Part before delimiter goes to callback (conversational text)
				// WHY: This ensures we display all conversational text up to the
				// delimiter, even if the delimiter appears mid-token. Without this,
				// we'd lose the text that appears before "---" in this token.
				beforeDelimiter := token[:delimiterIndex]
				if callback != nil && beforeDelimiter != "" {
					streamToken := StreamToken{
						Content: beforeDelimiter,
						Done:    false,
					}
					if err := callback(streamToken); err != nil {
						return fullResponse.String(), fmt.Errorf("callback error after %d bytes: %w", fullResponse.Len(), err)
					}
				}

				// Part after delimiter (including delimiter) goes to JSON buffer
				// WHY: We include the delimiter in the JSON buffer so that parseResponse()
				// can find it when splitting the full response later. The delimiter is
				// part of the structured format, not conversational text.
				jsonBuffer.WriteString(token[delimiterIndex:])
			} else {
				// No delimiter yet - send entire token to callback
				// WHY: Before we've seen the delimiter, all tokens are conversational
				// text that should be displayed to the user for the live typing effect.
				if callback != nil {
					streamToken := StreamToken{
						Content: token,
						Done:    false,
					}
					if err := callback(streamToken); err != nil {
						return fullResponse.String(), fmt.Errorf("callback error after %d bytes: %w", fullResponse.Len(), err)
					}
				}
			}
		}

		if chatResp.Done {
			// Stream is complete - stop reading.
			// WHY CHECK DONE: Ollama signals stream completion by setting Done=true
			// in the final JSON object. This is more reliable than waiting for EOF
			// because it's an explicit protocol signal that all tokens have been sent.
			break
		}
	}

	// Check for errors during stream reading.
	// WHY CHECK SCANNER ERROR: Scanner.Scan() returns false on EOF or error.
	// We must distinguish between clean completion (Done=true) and I/O errors
	// (network failure, connection closed). Scanner.Err() tells us which it was.
	if err := scanner.Err(); err != nil {
		return fullResponse.String(), fmt.Errorf("stream read error: %w", err)
	}

	return fullResponse.String(), nil
}

// classifyError converts low-level HTTP errors into user-friendly errors.
func (c *Client) classifyError(err error) error {
	if err == nil {
		return nil
	}

	// Check for context deadline exceeded (timeout)
	if errors.Is(err, context.DeadlineExceeded) {
		return ErrConnectionTimeout
	}

	// Check for context canceled
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}

	// Check for timeout from net package
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return ErrConnectionTimeout
	}

	// Check for connection refused (ollama not running)
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		if opErr.Err != nil {
			if errors.Is(opErr.Err, syscall.ECONNREFUSED) {
				return ErrNotRunning
			}
		}
	}

	// Check for syscall errors wrapped differently
	var syscallErr syscall.Errno
	if errors.As(err, &syscallErr) {
		if syscallErr == syscall.ECONNREFUSED {
			return ErrNotRunning
		}
	}

	// Return wrapped error for unknown cases (DNS errors, TLS errors, etc.)
	return fmt.Errorf("%w: %v", ErrConnectionFailed, err)
}
