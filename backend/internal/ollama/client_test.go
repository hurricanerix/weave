package ollama

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"syscall"
	"testing"
	"time"
)

// timeoutError implements net.Error with Timeout() returning true
type timeoutError struct{}

func (e *timeoutError) Error() string   { return "timeout" }
func (e *timeoutError) Timeout() bool   { return true }
func (e *timeoutError) Temporary() bool { return true }

func TestNewClient(t *testing.T) {
	client := NewClient()

	if client.endpoint != DefaultEndpoint {
		t.Errorf("endpoint = %q, want %q", client.endpoint, DefaultEndpoint)
	}

	if client.model != DefaultModel {
		t.Errorf("model = %q, want %q", client.model, DefaultModel)
	}

	if client.httpClient == nil {
		t.Error("httpClient is nil")
	}

	expectedTimeout := time.Duration(DefaultTimeout) * time.Second
	if client.httpClient.Timeout != expectedTimeout {
		t.Errorf("timeout = %v, want %v", client.httpClient.Timeout, expectedTimeout)
	}
}

func TestClientModel(t *testing.T) {
	client := NewClient()
	if client.Model() != DefaultModel {
		t.Errorf("Model() = %q, want %q", client.Model(), DefaultModel)
	}
}

func TestClientEndpoint(t *testing.T) {
	client := NewClient()
	if client.Endpoint() != DefaultEndpoint {
		t.Errorf("Endpoint() = %q, want %q", client.Endpoint(), DefaultEndpoint)
	}
}

func TestConnectSuccess(t *testing.T) {
	// Create test server that returns the expected model
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != EndpointTags {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		if r.Method != http.MethodGet {
			t.Errorf("unexpected method: %s", r.Method)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		resp := TagsResponse{
			Models: []ModelInfo{
				{Name: "other-model:latest", Size: 1000000},
				{Name: DefaultModel, Size: 2000000},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := &Client{
		endpoint:   server.URL,
		model:      DefaultModel,
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}

	err := client.Connect(context.Background())
	if err != nil {
		t.Errorf("Connect() error = %v, want nil", err)
	}
}

func TestConnectModelNotFound(t *testing.T) {
	// Create test server that returns different models
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := TagsResponse{
			Models: []ModelInfo{
				{Name: "other-model:latest", Size: 1000000},
				{Name: "another-model:7b", Size: 2000000},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := &Client{
		endpoint:   server.URL,
		model:      DefaultModel,
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}

	err := client.Connect(context.Background())
	if !errors.Is(err, ErrModelNotFound) {
		t.Errorf("Connect() error = %v, want ErrModelNotFound", err)
	}
}

func TestConnectEmptyModelList(t *testing.T) {
	// Create test server that returns empty model list
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := TagsResponse{
			Models: []ModelInfo{},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := &Client{
		endpoint:   server.URL,
		model:      DefaultModel,
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}

	err := client.Connect(context.Background())
	if !errors.Is(err, ErrModelNotFound) {
		t.Errorf("Connect() error = %v, want ErrModelNotFound", err)
	}
}

func TestConnectServerError(t *testing.T) {
	// Create test server that returns 500
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer server.Close()

	client := &Client{
		endpoint:   server.URL,
		model:      DefaultModel,
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}

	err := client.Connect(context.Background())
	if err == nil {
		t.Error("Connect() error = nil, want error")
	}
	if !errors.Is(err, ErrRequestFailed) {
		t.Errorf("Connect() error = %v, want ErrRequestFailed", err)
	}
}

func TestConnectInvalidJSON(t *testing.T) {
	// Create test server that returns invalid JSON
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not valid json"))
	}))
	defer server.Close()

	client := &Client{
		endpoint:   server.URL,
		model:      DefaultModel,
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}

	err := client.Connect(context.Background())
	if err == nil {
		t.Error("Connect() error = nil, want error for invalid JSON")
	}
}

func TestConnectContextCanceled(t *testing.T) {
	// Create a slow server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(1 * time.Second)
		resp := TagsResponse{Models: []ModelInfo{{Name: DefaultModel}}}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := &Client{
		endpoint:   server.URL,
		model:      DefaultModel,
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := client.Connect(ctx)
	if err == nil {
		t.Error("Connect() error = nil, want error for canceled context")
	}
}

func TestClassifyError(t *testing.T) {
	client := NewClient()

	tests := []struct {
		name    string
		err     error
		wantErr error
	}{
		{
			name:    "nil error",
			err:     nil,
			wantErr: nil,
		},
		{
			name:    "context deadline exceeded",
			err:     context.DeadlineExceeded,
			wantErr: ErrConnectionTimeout,
		},
		{
			name:    "context canceled",
			err:     context.Canceled,
			wantErr: context.Canceled,
		},
		{
			name:    "connection refused via OpError",
			err:     &net.OpError{Err: syscall.ECONNREFUSED},
			wantErr: ErrNotRunning,
		},
		{
			name:    "connection refused via syscall",
			err:     syscall.ECONNREFUSED,
			wantErr: ErrNotRunning,
		},
		{
			name:    "network timeout",
			err:     &timeoutError{},
			wantErr: ErrConnectionTimeout,
		},
		{
			name:    "unknown error",
			err:     errors.New("some unknown error"),
			wantErr: ErrConnectionFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotErr := client.classifyError(tt.err)

			if tt.wantErr == nil {
				if gotErr != nil {
					t.Errorf("classifyError() = %v, want nil", gotErr)
				}
				return
			}

			if !errors.Is(gotErr, tt.wantErr) {
				t.Errorf("classifyError() = %v, want %v", gotErr, tt.wantErr)
			}
		})
	}
}

func TestErrorMessages(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantMsg string
	}{
		{
			name:    "not running",
			err:     ErrNotRunning,
			wantMsg: "ollama not running",
		},
		{
			name:    "model not found",
			err:     ErrModelNotFound,
			wantMsg: "model not available in ollama",
		},
		{
			name:    "connection timeout",
			err:     ErrConnectionTimeout,
			wantMsg: "ollama connection timeout",
		},
		{
			name:    "request failed",
			err:     ErrRequestFailed,
			wantMsg: "ollama request failed",
		},
		{
			name:    "connection failed",
			err:     ErrConnectionFailed,
			wantMsg: "ollama connection failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err.Error() != tt.wantMsg {
				t.Errorf("Error message = %q, want %q", tt.err.Error(), tt.wantMsg)
			}
		})
	}
}

func TestConnectConnectionRefused(t *testing.T) {
	// Use a port that's definitely not running anything
	endpoint := "http://127.0.0.1:59999"
	client := &Client{
		endpoint:   endpoint,
		model:      DefaultModel,
		httpClient: &http.Client{Timeout: 1 * time.Second},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := client.Connect(ctx)
	if err == nil {
		t.Error("Connect() error = nil, want error for connection refused")
	}

	// Should be ErrNotRunning or a wrapped error containing it
	if !errors.Is(err, ErrNotRunning) {
		t.Errorf("Connect() error = %v, want ErrNotRunning", err)
	}

	// Error message should include the endpoint
	if !strings.Contains(err.Error(), endpoint) {
		t.Errorf("error message should contain endpoint %q, got %q", endpoint, err.Error())
	}
}

func TestConnectModelNotFoundIncludesModelName(t *testing.T) {
	// Create test server that returns different models
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := TagsResponse{
			Models: []ModelInfo{
				{Name: "other-model:latest", Size: 1000000},
			},
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Errorf("failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	modelName := "my-custom-model:7b"
	client := &Client{
		endpoint:   server.URL,
		model:      modelName,
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}

	err := client.Connect(context.Background())
	if !errors.Is(err, ErrModelNotFound) {
		t.Errorf("Connect() error = %v, want ErrModelNotFound", err)
	}

	// Error message should include the model name
	if !strings.Contains(err.Error(), modelName) {
		t.Errorf("error message should contain model name %q, got %q", modelName, err.Error())
	}
}

func TestChatSuccess(t *testing.T) {
	// Create test server that returns streaming response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != EndpointChat {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		if r.Method != http.MethodPost {
			t.Errorf("unexpected method: %s", r.Method)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Verify request body
		var chatReq ChatRequest
		if err := json.NewDecoder(r.Body).Decode(&chatReq); err != nil {
			t.Errorf("failed to decode request: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		if chatReq.Model != DefaultModel {
			t.Errorf("unexpected model: %s", chatReq.Model)
		}

		if !chatReq.Stream {
			t.Error("stream should be true")
		}

		// Send streaming response
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Error("expected http.Flusher")
			return
		}

		responses := []ChatResponse{
			{Model: DefaultModel, Message: Message{Role: RoleAssistant, Content: "Hello"}, Done: false},
			{Model: DefaultModel, Message: Message{Role: RoleAssistant, Content: " there"}, Done: false},
			{Model: DefaultModel, Message: Message{Role: RoleAssistant, Content: "!\n"}, Done: false},
			{Model: DefaultModel, Message: Message{Role: RoleAssistant, Content: "---\n"}, Done: false},
			{Model: DefaultModel, Message: Message{Role: RoleAssistant, Content: `{"prompt": "a test prompt", "generate_image": true, "steps": 28, "cfg": 7.5, "seed": 42}`}, Done: true},
		}

		for _, resp := range responses {
			data, _ := json.Marshal(resp)
			w.Write(data)
			w.Write([]byte("\n"))
			flusher.Flush()
		}
	}))
	defer server.Close()

	client := &Client{
		endpoint:   server.URL,
		model:      DefaultModel,
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}

	messages := []Message{
		{Role: RoleUser, Content: "Hi"},
	}

	var tokens []string
	callback := func(token StreamToken) error {
		tokens = append(tokens, token.Content)
		return nil
	}

	result, err := client.Chat(context.Background(), messages, nil, callback)
	if err != nil {
		t.Errorf("Chat() error = %v, want nil", err)
	}

	// Check conversational text (before delimiter)
	expectedResponse := "Hello there!"
	if result.Response != expectedResponse {
		t.Errorf("Chat() response = %q, want %q", result.Response, expectedResponse)
	}

	// Check metadata
	expectedPrompt := "a test prompt"
	if result.Metadata.Prompt != expectedPrompt {
		t.Errorf("Metadata.Prompt = %q, want %q", result.Metadata.Prompt, expectedPrompt)
	}

	// Check RawResponse includes full response (text + delimiter + JSON)
	expectedRawResponse := "Hello there!\n---\n{\"prompt\": \"a test prompt\", \"generate_image\": true, \"steps\": 28, \"cfg\": 7.5, \"seed\": 42}"
	if result.RawResponse != expectedRawResponse {
		t.Errorf("RawResponse = %q, want %q", result.RawResponse, expectedRawResponse)
	}

	// Check that tokens sent to callback only include text before delimiter
	expectedTokens := []string{"Hello", " there", "!\n"}
	if len(tokens) != len(expectedTokens) {
		t.Errorf("got %d tokens, want %d", len(tokens), len(expectedTokens))
	}
	for i, token := range tokens {
		if i < len(expectedTokens) && token != expectedTokens[i] {
			t.Errorf("token[%d] = %q, want %q", i, token, expectedTokens[i])
		}
	}
}

func TestChatWithSeed(t *testing.T) {
	var receivedSeed *int64

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var chatReq ChatRequest
		if err := json.NewDecoder(r.Body).Decode(&chatReq); err != nil {
			t.Errorf("failed to decode request: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		if chatReq.Options != nil {
			receivedSeed = chatReq.Options.Seed
		}

		// Send minimal response with delimiter and JSON
		responses := []ChatResponse{
			{Model: DefaultModel, Message: Message{Role: RoleAssistant, Content: "ok\n---\n"}, Done: false},
			{Model: DefaultModel, Message: Message{Role: RoleAssistant, Content: `{"prompt": "", "generate_image": false, "steps": 20, "cfg": 7.0, "seed": 0}`}, Done: true},
		}
		for _, resp := range responses {
			data, _ := json.Marshal(resp)
			w.Write(data)
			w.Write([]byte("\n"))
		}
	}))
	defer server.Close()

	client := &Client{
		endpoint:   server.URL,
		model:      DefaultModel,
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}

	messages := []Message{{Role: RoleUser, Content: "test"}}

	// Test with seed
	seed := int64(42)
	_, err := client.Chat(context.Background(), messages, &seed, nil)
	if err != nil {
		t.Errorf("Chat() error = %v", err)
	}

	if receivedSeed == nil {
		t.Error("seed should have been sent")
	} else if *receivedSeed != 42 {
		t.Errorf("seed = %d, want 42", *receivedSeed)
	}
}

func TestChatWithoutSeed(t *testing.T) {
	var receivedOptions *ChatOptions

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var chatReq ChatRequest
		if err := json.NewDecoder(r.Body).Decode(&chatReq); err != nil {
			t.Errorf("failed to decode request: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		receivedOptions = chatReq.Options

		// Send minimal response with delimiter and JSON
		responses := []ChatResponse{
			{Model: DefaultModel, Message: Message{Role: RoleAssistant, Content: "ok\n---\n"}, Done: false},
			{Model: DefaultModel, Message: Message{Role: RoleAssistant, Content: `{"prompt": "", "generate_image": false, "steps": 20, "cfg": 7.0, "seed": 0}`}, Done: true},
		}
		for _, resp := range responses {
			data, _ := json.Marshal(resp)
			w.Write(data)
			w.Write([]byte("\n"))
		}
	}))
	defer server.Close()

	client := &Client{
		endpoint:   server.URL,
		model:      DefaultModel,
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}

	messages := []Message{{Role: RoleUser, Content: "test"}}

	// Test without seed
	_, err := client.Chat(context.Background(), messages, nil, nil)
	if err != nil {
		t.Errorf("Chat() error = %v", err)
	}

	if receivedOptions != nil {
		t.Error("options should be nil when no seed provided")
	}
}

func TestChatCallbackError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		responses := []ChatResponse{
			{Model: DefaultModel, Message: Message{Role: RoleAssistant, Content: "Hello"}, Done: false},
			{Model: DefaultModel, Message: Message{Role: RoleAssistant, Content: " world"}, Done: true},
		}

		for _, resp := range responses {
			data, _ := json.Marshal(resp)
			w.Write(data)
			w.Write([]byte("\n"))
		}
	}))
	defer server.Close()

	client := &Client{
		endpoint:   server.URL,
		model:      DefaultModel,
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}

	messages := []Message{{Role: RoleUser, Content: "test"}}

	callbackErr := errors.New("callback failed")
	callback := func(token StreamToken) error {
		return callbackErr
	}

	_, err := client.Chat(context.Background(), messages, nil, callback)
	if err == nil {
		t.Error("Chat() should return error when callback fails")
	}

	if !strings.Contains(err.Error(), "callback error") {
		t.Errorf("error should mention callback, got: %v", err)
	}
}

func TestChatServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "model not loaded", http.StatusInternalServerError)
	}))
	defer server.Close()

	client := &Client{
		endpoint:   server.URL,
		model:      DefaultModel,
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}

	messages := []Message{{Role: RoleUser, Content: "test"}}

	_, err := client.Chat(context.Background(), messages, nil, nil)
	if err == nil {
		t.Error("Chat() should return error on server error")
	}

	if !errors.Is(err, ErrRequestFailed) {
		t.Errorf("Chat() error = %v, want ErrRequestFailed", err)
	}
}

func TestChatInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not valid json\n"))
	}))
	defer server.Close()

	client := &Client{
		endpoint:   server.URL,
		model:      DefaultModel,
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}

	messages := []Message{{Role: RoleUser, Content: "test"}}

	_, err := client.Chat(context.Background(), messages, nil, nil)
	if err == nil {
		t.Error("Chat() should return error on invalid JSON")
	}

	if !strings.Contains(err.Error(), "failed to parse") {
		t.Errorf("error should mention parse failure, got: %v", err)
	}
}

func TestChatConnectionRefused(t *testing.T) {
	client := &Client{
		endpoint:   "http://127.0.0.1:59998",
		model:      DefaultModel,
		httpClient: &http.Client{Timeout: 1 * time.Second},
	}

	messages := []Message{{Role: RoleUser, Content: "test"}}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := client.Chat(ctx, messages, nil, nil)
	if err == nil {
		t.Error("Chat() should return error when connection refused")
	}

	if !errors.Is(err, ErrNotRunning) {
		t.Errorf("Chat() error = %v, want ErrNotRunning", err)
	}
}

func TestParseStreamingResponse(t *testing.T) {
	client := NewClient()

	tests := []struct {
		name         string
		input        string
		wantResponse string
		wantTokens   []string
		wantErr      bool
	}{
		{
			name: "single token",
			input: `{"model":"test","message":{"role":"assistant","content":"Hello"},"done":true}
`,
			wantResponse: "Hello",
			wantTokens:   []string{"Hello"},
			wantErr:      false,
		},
		{
			name: "multiple tokens",
			input: `{"model":"test","message":{"role":"assistant","content":"Hello"},"done":false}
{"model":"test","message":{"role":"assistant","content":" world"},"done":true}
`,
			wantResponse: "Hello world",
			wantTokens:   []string{"Hello", " world"},
			wantErr:      false,
		},
		{
			name: "empty lines ignored",
			input: `{"model":"test","message":{"role":"assistant","content":"A"},"done":false}

{"model":"test","message":{"role":"assistant","content":"B"},"done":true}
`,
			wantResponse: "AB",
			wantTokens:   []string{"A", "B"},
			wantErr:      false,
		},
		{
			name:         "invalid json",
			input:        "not json\n",
			wantResponse: "",
			wantTokens:   nil,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var tokens []string
			callback := func(token StreamToken) error {
				tokens = append(tokens, token.Content)
				return nil
			}

			response, err := client.parseStreamingResponse(strings.NewReader(tt.input), callback)

			if (err != nil) != tt.wantErr {
				t.Errorf("parseStreamingResponse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if response != tt.wantResponse {
				t.Errorf("parseStreamingResponse() = %q, want %q", response, tt.wantResponse)
			}

			if !tt.wantErr && len(tokens) != len(tt.wantTokens) {
				t.Errorf("got %d tokens, want %d", len(tokens), len(tt.wantTokens))
			}
		})
	}
}

func TestChatNilCallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		responses := []ChatResponse{
			{Model: DefaultModel, Message: Message{Role: RoleAssistant, Content: "ok\n---\n"}, Done: false},
			{Model: DefaultModel, Message: Message{Role: RoleAssistant, Content: `{"prompt": "test", "generate_image": true, "steps": 28, "cfg": 7.5, "seed": 42}`}, Done: true},
		}
		for _, resp := range responses {
			data, _ := json.Marshal(resp)
			w.Write(data)
			w.Write([]byte("\n"))
		}
	}))
	defer server.Close()

	client := &Client{
		endpoint:   server.URL,
		model:      DefaultModel,
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}

	messages := []Message{{Role: RoleUser, Content: "test"}}

	// Test with nil callback - should work without panicking
	result, err := client.Chat(context.Background(), messages, nil, nil)
	if err != nil {
		t.Errorf("Chat() error = %v", err)
	}

	if result.Response != "ok" {
		t.Errorf("Chat() response = %q, want %q", result.Response, "ok")
	}
	if result.Metadata.Prompt != "test" {
		t.Errorf("Metadata.Prompt = %q, want %q", result.Metadata.Prompt, "test")
	}

	// Verify RawResponse contains full response
	expectedRaw := "ok\n---\n{\"prompt\": \"test\", \"generate_image\": true, \"steps\": 28, \"cfg\": 7.5, \"seed\": 42}"
	if result.RawResponse != expectedRaw {
		t.Errorf("RawResponse = %q, want %q", result.RawResponse, expectedRaw)
	}
}

func TestParseStreamingResponseTooLarge(t *testing.T) {
	client := NewClient()

	// Create a response larger than maxResponseSize (1 MB)
	// Use 10KB tokens to stay under bufio.Scanner's 64KB limit
	tokenContent := strings.Repeat("x", 10000) // 10KB per token
	var input strings.Builder
	for i := 0; i < 110; i++ { // 110 * 10KB = 1.1MB > 1MB limit
		resp := ChatResponse{
			Model:   "test",
			Message: Message{Role: RoleAssistant, Content: tokenContent},
			Done:    i == 109,
		}
		data, _ := json.Marshal(resp)
		input.Write(data)
		input.WriteString("\n")
	}

	_, err := client.parseStreamingResponse(strings.NewReader(input.String()), nil)
	if err == nil {
		t.Error("parseStreamingResponse() should return error for response > 1MB")
	}

	if !strings.Contains(err.Error(), "response too large") {
		t.Errorf("error should mention response too large, got: %v", err)
	}
}

func TestChatCallbackErrorIncludesBytes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		responses := []ChatResponse{
			{Model: DefaultModel, Message: Message{Role: RoleAssistant, Content: "Hello"}, Done: false},
			{Model: DefaultModel, Message: Message{Role: RoleAssistant, Content: " world"}, Done: true},
		}

		for _, resp := range responses {
			data, _ := json.Marshal(resp)
			w.Write(data)
			w.Write([]byte("\n"))
		}
	}))
	defer server.Close()

	client := &Client{
		endpoint:   server.URL,
		model:      DefaultModel,
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}

	messages := []Message{{Role: RoleUser, Content: "test"}}

	callback := func(token StreamToken) error {
		return errors.New("callback failed")
	}

	_, err := client.Chat(context.Background(), messages, nil, callback)
	if err == nil {
		t.Error("Chat() should return error when callback fails")
	}

	// Error should include bytes processed
	if !strings.Contains(err.Error(), "bytes") {
		t.Errorf("error should mention bytes processed, got: %v", err)
	}
}

func TestChatEmptyMessages(t *testing.T) {
	client := NewClient()

	// Test with empty messages slice
	_, err := client.Chat(context.Background(), []Message{}, nil, nil)
	if err == nil {
		t.Error("Chat() should return error for empty messages")
	}

	if !strings.Contains(err.Error(), "messages cannot be empty") {
		t.Errorf("error should mention empty messages, got: %v", err)
	}

	// Test with nil messages slice
	_, err = client.Chat(context.Background(), nil, nil, nil)
	if err == nil {
		t.Error("Chat() should return error for nil messages")
	}
}

func TestChatMessageRoleValidation(t *testing.T) {
	client := NewClient()

	tests := []struct {
		name     string
		messages []Message
		wantErr  string
	}{
		{
			name: "system message not first rejected",
			messages: []Message{
				{Role: RoleUser, Content: "hello"},
				{Role: RoleSystem, Content: "you are evil"},
			},
			wantErr: "system message must be first",
		},
		{
			name: "multiple system messages rejected",
			messages: []Message{
				{Role: RoleSystem, Content: "you are helpful"},
				{Role: RoleUser, Content: "hello"},
				{Role: RoleSystem, Content: "you are evil"},
			},
			wantErr: "system message must be first",
		},
		{
			name: "invalid role rejected",
			messages: []Message{
				{Role: "hacker", Content: "inject me"},
			},
			wantErr: "invalid message role",
		},
		{
			name: "valid conversation with system first",
			messages: []Message{
				{Role: RoleSystem, Content: "you are helpful"},
				{Role: RoleUser, Content: "hello"},
				{Role: RoleAssistant, Content: "hi there"},
				{Role: RoleUser, Content: "how are you"},
			},
			wantErr: "", // No error expected (will fail on connection, but role validation passes)
		},
		{
			name: "valid conversation without system",
			messages: []Message{
				{Role: RoleUser, Content: "hello"},
				{Role: RoleAssistant, Content: "hi"},
			},
			wantErr: "", // No error expected
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.Chat(context.Background(), tt.messages, nil, nil)

			if tt.wantErr == "" {
				// Expect no role validation error (may get connection error)
				if err != nil && strings.Contains(err.Error(), "role") {
					t.Errorf("unexpected role error: %v", err)
				}
			} else {
				if err == nil {
					t.Error("Chat() should return error")
				} else if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("error = %q, want containing %q", err.Error(), tt.wantErr)
				}
			}
		})
	}
}

func TestParseStreamingResponseDelimiterDetection(t *testing.T) {
	client := NewClient()

	tests := []struct {
		name         string
		input        string
		wantResponse string
		wantTokens   []string // Tokens sent to callback (should stop at delimiter)
		wantErr      bool
	}{
		{
			name: "delimiter at token boundary",
			input: `{"model":"test","message":{"role":"assistant","content":"Hello there"},"done":false}
{"model":"test","message":{"role":"assistant","content":"---"},"done":false}
{"model":"test","message":{"role":"assistant","content":"{\"prompt\":\"\",\"generate_image\":false}"},"done":true}
`,
			wantResponse: "Hello there---{\"prompt\":\"\",\"generate_image\":false}",
			wantTokens:   []string{"Hello there"}, // Callback stops before delimiter
			wantErr:      false,
		},
		{
			name: "delimiter in middle of token",
			input: `{"model":"test","message":{"role":"assistant","content":"Hello"},"done":false}
{"model":"test","message":{"role":"assistant","content":" there\n---\n{\"prompt\":\"\",\"generate_image\":false}"},"done":true}
`,
			wantResponse: "Hello there\n---\n{\"prompt\":\"\",\"generate_image\":false}",
			wantTokens:   []string{"Hello", " there\n"}, // Callback stops at delimiter boundary
			wantErr:      false,
		},
		{
			name: "no delimiter",
			input: `{"model":"test","message":{"role":"assistant","content":"Hello"},"done":false}
{"model":"test","message":{"role":"assistant","content":" world"},"done":true}
`,
			wantResponse: "Hello world",
			wantTokens:   []string{"Hello", " world"}, // All tokens go to callback
			wantErr:      false,
		},
		{
			name: "delimiter followed by json",
			input: `{"model":"test","message":{"role":"assistant","content":"Ready to generate!\n"},"done":false}
{"model":"test","message":{"role":"assistant","content":"---"},"done":false}
{"model":"test","message":{"role":"assistant","content":"\n"},"done":false}
{"model":"test","message":{"role":"assistant","content":"{\"prompt\":\"a cat\",\"generate_image\":true}"},"done":true}
`,
			wantResponse: "Ready to generate!\n---\n{\"prompt\":\"a cat\",\"generate_image\":true}",
			wantTokens:   []string{"Ready to generate!\n"}, // Stops before delimiter
			wantErr:      false,
		},
		{
			name: "delimiter at start of token",
			input: `{"model":"test","message":{"role":"assistant","content":"Text before"},"done":false}
{"model":"test","message":{"role":"assistant","content":"---JSON after"},"done":true}
`,
			wantResponse: "Text before---JSON after",
			wantTokens:   []string{"Text before"}, // Stops before delimiter
			wantErr:      false,
		},
		{
			name: "multiple delimiters (only first matters)",
			input: `{"model":"test","message":{"role":"assistant","content":"Text"},"done":false}
{"model":"test","message":{"role":"assistant","content":"---"},"done":false}
{"model":"test","message":{"role":"assistant","content":"JSON with --- inside"},"done":true}
`,
			wantResponse: "Text---JSON with --- inside",
			wantTokens:   []string{"Text"}, // Stops at first delimiter
			wantErr:      false,
		},
		{
			name: "empty token before delimiter",
			input: `{"model":"test","message":{"role":"assistant","content":""},"done":false}
{"model":"test","message":{"role":"assistant","content":"---"},"done":false}
{"model":"test","message":{"role":"assistant","content":"{}"},"done":true}
`,
			wantResponse: "---{}",
			wantTokens:   []string{""}, // Empty token still sent
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var tokens []string
			callback := func(token StreamToken) error {
				tokens = append(tokens, token.Content)
				return nil
			}

			response, err := client.parseStreamingResponse(strings.NewReader(tt.input), callback)

			if (err != nil) != tt.wantErr {
				t.Errorf("parseStreamingResponse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if response != tt.wantResponse {
				t.Errorf("parseStreamingResponse() response = %q, want %q", response, tt.wantResponse)
			}

			if !tt.wantErr {
				if len(tokens) != len(tt.wantTokens) {
					t.Errorf("got %d tokens, want %d", len(tokens), len(tt.wantTokens))
					t.Errorf("tokens: %v", tokens)
					t.Errorf("want: %v", tt.wantTokens)
				} else {
					for i, token := range tokens {
						if token != tt.wantTokens[i] {
							t.Errorf("token[%d] = %q, want %q", i, token, tt.wantTokens[i])
						}
					}
				}
			}
		})
	}
}

func TestParseStreamingResponseDelimiterWithCallback(t *testing.T) {
	client := NewClient()

	// This test verifies that when delimiter is detected:
	// 1. Callback stops being called
	// 2. Full response still includes everything
	// 3. Tokens after delimiter are buffered (not sent to callback)

	input := `{"model":"test","message":{"role":"assistant","content":"Sure! "},"done":false}
{"model":"test","message":{"role":"assistant","content":"Let me help.\n"},"done":false}
{"model":"test","message":{"role":"assistant","content":"---"},"done":false}
{"model":"test","message":{"role":"assistant","content":"\n{\"prompt\":\"a test\","},"done":false}
{"model":"test","message":{"role":"assistant","content":"\"generate_image\":true}"},"done":true}
`

	var tokens []string
	callback := func(token StreamToken) error {
		// Verify delimiter and JSON are never sent to callback
		if strings.Contains(token.Content, "---") {
			t.Errorf("callback received delimiter: %q", token.Content)
		}
		if strings.Contains(token.Content, "{\"prompt\"") {
			t.Errorf("callback received JSON: %q", token.Content)
		}
		tokens = append(tokens, token.Content)
		return nil
	}

	response, err := client.parseStreamingResponse(strings.NewReader(input), callback)
	if err != nil {
		t.Errorf("parseStreamingResponse() error = %v", err)
	}

	// Full response should include everything
	expectedResponse := "Sure! Let me help.\n---\n{\"prompt\":\"a test\",\"generate_image\":true}"
	if response != expectedResponse {
		t.Errorf("response = %q, want %q", response, expectedResponse)
	}

	// Callback should only receive tokens before delimiter
	expectedTokens := []string{"Sure! ", "Let me help.\n"}
	if len(tokens) != len(expectedTokens) {
		t.Errorf("got %d tokens in callback, want %d", len(tokens), len(expectedTokens))
		t.Errorf("tokens: %v", tokens)
	} else {
		for i, token := range tokens {
			if token != expectedTokens[i] {
				t.Errorf("token[%d] = %q, want %q", i, token, expectedTokens[i])
			}
		}
	}
}
