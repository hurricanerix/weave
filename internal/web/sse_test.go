package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewBroker(t *testing.T) {
	broker := NewBroker()

	if broker == nil {
		t.Fatal("NewBroker returned nil")
	}

	if broker.connections == nil {
		t.Error("broker.connections is nil")
	}

	if broker.ConnectionCount() != 0 {
		t.Errorf("new broker should have 0 connections, got %d", broker.ConnectionCount())
	}
}

func TestBroker_ServeHTTP_NoSession(t *testing.T) {
	broker := NewBroker()

	req := httptest.NewRequest("GET", "/events", nil)
	w := httptest.NewRecorder()

	broker.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}

	body := w.Body.String()
	if !strings.Contains(body, "session required") {
		t.Errorf("body = %q, want error about session required", body)
	}
}

func TestBroker_ServeHTTP_Headers(t *testing.T) {
	broker := NewBroker()

	req := httptest.NewRequest("GET", "/events", nil)
	ctx := setSessionID(req.Context(), "test-session")
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	// Start serving in a goroutine
	done := make(chan struct{})
	go func() {
		defer close(done)
		broker.ServeHTTP(w, req)
	}()

	// Give it time to set headers and send initial event
	time.Sleep(50 * time.Millisecond)

	// Verify SSE headers are set
	tests := []struct {
		header string
		want   string
	}{
		{"Content-Type", "text/event-stream"},
		{"Cache-Control", "no-cache"},
		{"Connection", "keep-alive"},
		{"X-Accel-Buffering", "no"},
	}

	for _, tt := range tests {
		got := w.Header().Get(tt.header)
		if got != tt.want {
			t.Errorf("%s = %q, want %q", tt.header, got, tt.want)
		}
	}

	// Close the connection
	broker.CloseSession("test-session")

	// Wait for handler to complete
	select {
	case <-done:
		// Success
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for ServeHTTP to complete")
	}
}

func TestBroker_ServeHTTP_InitialEvent(t *testing.T) {
	broker := NewBroker()

	req := httptest.NewRequest("GET", "/events", nil)
	ctx := setSessionID(req.Context(), "test-session")
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	// Start serving in a goroutine
	done := make(chan struct{})
	go func() {
		defer close(done)
		broker.ServeHTTP(w, req)
	}()

	// Give it time to send initial event
	time.Sleep(50 * time.Millisecond)

	// Verify connection event was sent
	body := w.Body.String()
	if !strings.Contains(body, "event: connected") {
		t.Errorf("body should contain connected event, got %q", body)
	}
	if !strings.Contains(body, `"session":"test-session"`) {
		t.Errorf("body should contain session ID, got %q", body)
	}

	// Verify connection is registered
	if broker.ConnectionCount() != 1 {
		t.Errorf("connection count = %d, want 1", broker.ConnectionCount())
	}

	// Close the connection
	broker.CloseSession("test-session")

	// Wait for handler to complete
	select {
	case <-done:
		// Success
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for ServeHTTP to complete")
	}

	// Verify connection was unregistered
	if broker.ConnectionCount() != 0 {
		t.Errorf("connection count after close = %d, want 0", broker.ConnectionCount())
	}
}

func TestBroker_SendEvent(t *testing.T) {
	broker := NewBroker()

	req := httptest.NewRequest("GET", "/events", nil)
	ctx := setSessionID(req.Context(), "test-session")
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	// Start serving in a goroutine
	done := make(chan struct{})
	go func() {
		defer close(done)
		broker.ServeHTTP(w, req)
	}()

	// Give it time to connect
	time.Sleep(50 * time.Millisecond)

	// Send a test event
	err := broker.SendEvent("test-session", EventAgentToken, map[string]string{"token": "Hello"})
	if err != nil {
		t.Fatalf("SendEvent failed: %v", err)
	}

	// Give it time to write
	time.Sleep(50 * time.Millisecond)

	// Verify event was sent
	body := w.Body.String()
	if !strings.Contains(body, "event: agent-token") {
		t.Errorf("body should contain agent-token event, got %q", body)
	}
	if !strings.Contains(body, `"token":"Hello"`) {
		t.Errorf("body should contain token data, got %q", body)
	}

	// Close the connection
	broker.CloseSession("test-session")

	// Wait for handler to complete
	select {
	case <-done:
		// Success
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for ServeHTTP to complete")
	}
}

func TestBroker_SendEvent_NoConnection(t *testing.T) {
	broker := NewBroker()

	err := broker.SendEvent("nonexistent", EventAgentToken, map[string]string{"token": "test"})
	if err == nil {
		t.Error("SendEvent should return error for nonexistent session")
	}

	if !strings.Contains(err.Error(), "not connected") {
		t.Errorf("error = %v, want 'not connected' message", err)
	}
}

func TestBroker_SendEvent_AllEventTypes(t *testing.T) {
	tests := []struct {
		name      string
		eventType string
		data      interface{}
		wantEvent string
		wantData  string
	}{
		{
			name:      "agent-token",
			eventType: EventAgentToken,
			data:      map[string]string{"token": "test"},
			wantEvent: "event: agent-token",
			wantData:  `"token":"test"`,
		},
		{
			name:      "agent-done",
			eventType: EventAgentDone,
			data:      map[string]bool{"complete": true},
			wantEvent: "event: agent-done",
			wantData:  `"complete":true`,
		},
		{
			name:      "prompt-update",
			eventType: EventPromptUpdate,
			data:      map[string]string{"prompt": "a cat wearing a hat"},
			wantEvent: "event: prompt-update",
			wantData:  `"prompt":"a cat wearing a hat"`,
		},
		{
			name:      "image-ready",
			eventType: EventImageReady,
			data:      map[string]string{"url": "/images/abc123.png"},
			wantEvent: "event: image-ready",
			wantData:  `"url":"/images/abc123.png"`,
		},
		{
			name:      "error",
			eventType: EventError,
			data:      map[string]string{"message": "generation failed"},
			wantEvent: "event: error",
			wantData:  `"message":"generation failed"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			broker := NewBroker()

			req := httptest.NewRequest("GET", "/events", nil)
			ctx := setSessionID(req.Context(), "test-session")
			req = req.WithContext(ctx)
			w := httptest.NewRecorder()

			// Start serving
			done := make(chan struct{})
			go func() {
				defer close(done)
				broker.ServeHTTP(w, req)
			}()

			// Wait for connection
			time.Sleep(50 * time.Millisecond)

			// Send event
			err := broker.SendEvent("test-session", tt.eventType, tt.data)
			if err != nil {
				t.Fatalf("SendEvent failed: %v", err)
			}

			// Wait for write
			time.Sleep(50 * time.Millisecond)

			// Verify event format
			body := w.Body.String()
			if !strings.Contains(body, tt.wantEvent) {
				t.Errorf("body should contain %q, got %q", tt.wantEvent, body)
			}
			if !strings.Contains(body, tt.wantData) {
				t.Errorf("body should contain %q, got %q", tt.wantData, body)
			}

			// Close connection
			broker.CloseSession("test-session")
			<-done
		})
	}
}

func TestBroker_SendEventToAll(t *testing.T) {
	broker := NewBroker()

	// Connect multiple sessions
	sessions := []string{"session-1", "session-2", "session-3"}
	writers := make([]*httptest.ResponseRecorder, len(sessions))
	dones := make([]chan struct{}, len(sessions))

	for i, sessionID := range sessions {
		req := httptest.NewRequest("GET", "/events", nil)
		ctx := setSessionID(req.Context(), sessionID)
		req = req.WithContext(ctx)
		w := httptest.NewRecorder()
		writers[i] = w
		dones[i] = make(chan struct{})

		go func(w http.ResponseWriter, req *http.Request, done chan struct{}) {
			defer close(done)
			broker.ServeHTTP(w, req)
		}(w, req, dones[i])
	}

	// Wait for all connections
	time.Sleep(100 * time.Millisecond)

	// Verify all connected
	if broker.ConnectionCount() != len(sessions) {
		t.Errorf("connection count = %d, want %d", broker.ConnectionCount(), len(sessions))
	}

	// Send broadcast event
	broker.SendEventToAll(EventPromptUpdate, map[string]string{"prompt": "broadcast test"})

	// Wait for writes
	time.Sleep(100 * time.Millisecond)

	// Verify all sessions received the event
	for i, w := range writers {
		body := w.Body.String()
		if !strings.Contains(body, "event: prompt-update") {
			t.Errorf("session %d should contain prompt-update event, got %q", i, body)
		}
		if !strings.Contains(body, `"prompt":"broadcast test"`) {
			t.Errorf("session %d should contain broadcast data, got %q", i, body)
		}
	}

	// Close all connections
	for _, sessionID := range sessions {
		broker.CloseSession(sessionID)
	}

	// Wait for all handlers to complete
	for i, done := range dones {
		select {
		case <-done:
			// Success
		case <-time.After(1 * time.Second):
			t.Fatalf("timeout waiting for session %d to complete", i)
		}
	}
}

func TestBroker_MultipleConnectionsSameSession(t *testing.T) {
	broker := NewBroker()

	// Connect first session using a cancellable context
	ctx1, cancel1 := context.WithCancel(context.Background())
	defer cancel1()
	req1 := httptest.NewRequest("GET", "/events", nil)
	req1 = req1.WithContext(setSessionID(ctx1, "same-session"))
	w1 := httptest.NewRecorder()

	done1 := make(chan struct{})
	go func() {
		defer close(done1)
		broker.ServeHTTP(w1, req1)
	}()

	time.Sleep(50 * time.Millisecond)

	// Verify first connection is registered
	if broker.ConnectionCount() != 1 {
		t.Errorf("connection count after first connect = %d, want 1", broker.ConnectionCount())
	}

	// Connect second session with same ID (should be ignored - first connection stays active)
	// Use a cancellable context so we can clean up
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	req2 := httptest.NewRequest("GET", "/events", nil)
	req2 = req2.WithContext(setSessionID(ctx2, "same-session"))
	w2 := httptest.NewRecorder()

	done2 := make(chan struct{})
	go func() {
		defer close(done2)
		broker.ServeHTTP(w2, req2)
	}()

	time.Sleep(100 * time.Millisecond)

	// First connection should still be active (not closed)
	select {
	case <-done1:
		t.Error("first connection should NOT have been closed when second connected")
	default:
		// First connection still open (expected)
	}

	// Second connection is ignored and just blocks - verify only one registered
	if broker.ConnectionCount() != 1 {
		t.Errorf("connection count = %d, want 1 (duplicate should be ignored)", broker.ConnectionCount())
	}

	// Send event (should go to first connection, the registered one)
	err := broker.SendEvent("same-session", EventAgentToken, map[string]string{"token": "test"})
	if err != nil {
		t.Fatalf("SendEvent failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	// Verify first connection received the event
	body1 := w1.Body.String()
	if !strings.Contains(body1, "event: agent-token") {
		t.Error("first connection should have received event")
	}

	// Second connection should NOT have received the event (it's ignored)
	body2 := w2.Body.String()
	// Second connection only receives "connected" initial event (if even that)
	if strings.Contains(body2, "event: agent-token") {
		t.Error("second connection should NOT have received event (it was ignored)")
	}

	// Close session - this closes the first connection
	broker.CloseSession("same-session")

	select {
	case <-done1:
		// First connection closed (expected)
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for first connection to close")
	}

	// Cancel second connection's context to clean up (simulates browser closing tab)
	cancel2()

	select {
	case <-done2:
		// Second connection closed (expected)
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for second connection to close")
	}

	// Verify no connections remain
	if broker.ConnectionCount() != 0 {
		t.Errorf("connection count after close = %d, want 0", broker.ConnectionCount())
	}
}

func TestBroker_Shutdown(t *testing.T) {
	broker := NewBroker()

	// Connect multiple sessions
	sessions := []string{"session-1", "session-2"}
	dones := make([]chan struct{}, len(sessions))

	for i, sessionID := range sessions {
		req := httptest.NewRequest("GET", "/events", nil)
		ctx := setSessionID(req.Context(), sessionID)
		req = req.WithContext(ctx)
		w := httptest.NewRecorder()
		dones[i] = make(chan struct{})

		go func(w http.ResponseWriter, req *http.Request, done chan struct{}) {
			defer close(done)
			broker.ServeHTTP(w, req)
		}(w, req, dones[i])
	}

	time.Sleep(50 * time.Millisecond)

	// Verify connections established
	if broker.ConnectionCount() != len(sessions) {
		t.Errorf("connection count = %d, want %d", broker.ConnectionCount(), len(sessions))
	}

	// Shutdown broker
	ctx := context.Background()
	err := broker.Shutdown(ctx)
	if err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}

	// Verify all connections closed
	for i, done := range dones {
		select {
		case <-done:
			// Success
		case <-time.After(1 * time.Second):
			t.Fatalf("timeout waiting for session %d to close after shutdown", i)
		}
	}

	// Verify connection count is 0
	if broker.ConnectionCount() != 0 {
		t.Errorf("connection count after shutdown = %d, want 0", broker.ConnectionCount())
	}
}

func TestBroker_ContextCancellation(t *testing.T) {
	broker := NewBroker()

	ctx, cancel := context.WithCancel(context.Background())
	ctx = setSessionID(ctx, "test-session")
	req := httptest.NewRequest("GET", "/events", nil)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		defer close(done)
		broker.ServeHTTP(w, req)
	}()

	time.Sleep(50 * time.Millisecond)

	// Verify connection is registered
	if broker.ConnectionCount() != 1 {
		t.Errorf("connection count = %d, want 1", broker.ConnectionCount())
	}

	// Cancel context (simulates client disconnect)
	cancel()

	// Wait for handler to complete
	select {
	case <-done:
		// Success
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for handler to complete after context cancellation")
	}

	// Verify connection was cleaned up
	time.Sleep(50 * time.Millisecond)
	if broker.ConnectionCount() != 0 {
		t.Errorf("connection count after cancellation = %d, want 0", broker.ConnectionCount())
	}
}

func TestBroker_EventFormatting(t *testing.T) {
	broker := NewBroker()

	req := httptest.NewRequest("GET", "/events", nil)
	ctx := setSessionID(req.Context(), "test-session")
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		defer close(done)
		broker.ServeHTTP(w, req)
	}()

	time.Sleep(50 * time.Millisecond)

	// Send event with specific data
	testData := map[string]interface{}{
		"message": "hello world",
		"count":   42,
		"active":  true,
	}

	err := broker.SendEvent("test-session", "test-event", testData)
	if err != nil {
		t.Fatalf("SendEvent failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	// Verify SSE format:
	// event: <type>\n
	// data: <json>\n
	// \n
	body := w.Body.String()

	// Check event line
	if !strings.Contains(body, "event: test-event\n") {
		t.Error("body should contain 'event: test-event\\n'")
	}

	// Check data line starts with "data: "
	if !strings.Contains(body, "data: {") {
		t.Error("body should contain 'data: {' for JSON payload")
	}

	// Check JSON content
	if !strings.Contains(body, `"message":"hello world"`) {
		t.Error("body should contain message field")
	}
	if !strings.Contains(body, `"count":42`) {
		t.Error("body should contain count field")
	}
	if !strings.Contains(body, `"active":true`) {
		t.Error("body should contain active field")
	}

	// Check blank line after data (double newline)
	lines := strings.Split(body, "\n")
	foundEvent := false
	for i, line := range lines {
		if strings.HasPrefix(line, "event: test-event") {
			foundEvent = true
			// Next line should be data
			if i+1 < len(lines) && !strings.HasPrefix(lines[i+1], "data: ") {
				t.Error("data line should follow event line")
			}
			// Line after data should be blank
			if i+2 < len(lines) && lines[i+2] != "" {
				t.Error("blank line should follow data line")
			}
			break
		}
	}

	if !foundEvent {
		t.Error("test-event not found in output")
	}

	broker.CloseSession("test-session")
	<-done
}
