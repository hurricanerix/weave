package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// Event types that can be sent via SSE.
const (
	// EventAgentToken is sent for each token during streaming agent response.
	// Data schema: {"token": string}
	// Example: {"token": "Hello"}
	EventAgentToken = "agent-token"

	// EventAgentDone indicates agent response streaming is complete.
	// Data schema: {"done": bool}
	// Example: {"done": true}
	EventAgentDone = "agent-done"

	// EventPromptUpdate indicates the image generation prompt has changed.
	// Sent when the agent extracts a new prompt from its response.
	// Data schema: {"prompt": string}
	// Example: {"prompt": "a fluffy orange cat"}
	EventPromptUpdate = "prompt-update"

	// EventImageReady indicates a generated image is available for download.
	// Data schema: {"url": string, "width": int, "height": int}
	// Example: {"url": "/images/abc123.png", "width": 512, "height": 512}
	EventImageReady = "image-ready"

	// EventError indicates an error occurred during processing.
	// Data schema: {"message": string}
	// Example: {"message": "Generation failed: timeout"}
	EventError = "error"

	// MaxConnections is the maximum number of concurrent SSE connections.
	MaxConnections = 1000
)

// Event represents a Server-Sent Event with a named type and JSON data.
type Event struct {
	Type string
	Data interface{}
}

// connection represents a single SSE connection for a session.
type connection struct {
	sessionID string
	writer    http.ResponseWriter
	flusher   http.Flusher
	done      chan struct{}
}

// Broker manages SSE connections and routes events to the correct sessions.
// It maintains a map of session IDs to connections and handles connection lifecycle.
type Broker struct {
	mu          sync.RWMutex
	connections map[string]*connection
}

// NewBroker creates a new SSE broker.
func NewBroker() *Broker {
	return &Broker{
		connections: make(map[string]*connection),
	}
}

// ServeHTTP handles SSE connection requests.
// It sets up the connection, registers it with the session ID, and keeps it open.
func (b *Broker) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// SECURITY: Check connection limit
	b.mu.RLock()
	currentConnections := len(b.connections)
	b.mu.RUnlock()

	if currentConnections >= MaxConnections {
		http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
		return
	}

	// SECURITY: Get session ID from context (set by SessionMiddleware via cookie).
	// This avoids exposing session ID in URL which could leak via logs/history.
	sessionID := GetSessionID(r.Context())
	if sessionID == "" {
		http.Error(w, "session required", http.StatusUnauthorized)
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // Disable nginx buffering

	// Disable write deadline for SSE connections.
	// The server's WriteTimeout would otherwise kill long-lived SSE connections.
	// This may fail in tests using httptest.ResponseRecorder which doesn't support
	// ResponseController - that's fine, we just ignore the error in that case.
	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Time{})

	// Verify we can flush
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	// Create connection
	conn := &connection{
		sessionID: sessionID,
		writer:    w,
		flusher:   flusher,
		done:      make(chan struct{}),
	}

	// Register connection
	b.addConnection(conn)
	defer b.removeConnection(sessionID, conn)

	// Send initial connection event
	b.sendToConnection(conn, Event{
		Type: "connected",
		Data: map[string]string{"session": sessionID},
	})

	// Keep connection open until client disconnects or context is cancelled
	select {
	case <-r.Context().Done():
		// Client disconnected
	case <-conn.done:
		// Connection closed by broker
	}
}

// SendEvent sends an event to a specific session.
// Returns an error if the session is not connected.
func (b *Broker) SendEvent(sessionID string, eventType string, data interface{}) error {
	b.mu.RLock()
	conn, ok := b.connections[sessionID]
	b.mu.RUnlock()

	if !ok {
		return fmt.Errorf("session %s not connected", sessionID)
	}

	return b.sendToConnection(conn, Event{Type: eventType, Data: data})
}

// SendEventToAll sends an event to all connected sessions.
// This is useful for broadcast notifications.
func (b *Broker) SendEventToAll(eventType string, data interface{}) {
	b.mu.RLock()
	connections := make([]*connection, 0, len(b.connections))
	for _, conn := range b.connections {
		connections = append(connections, conn)
	}
	b.mu.RUnlock()

	event := Event{Type: eventType, Data: data}
	for _, conn := range connections {
		// Ignore errors for broadcast (connection may have closed)
		_ = b.sendToConnection(conn, event)
	}
}

// CloseSession closes the SSE connection for a specific session.
func (b *Broker) CloseSession(sessionID string) {
	b.mu.Lock()
	conn, ok := b.connections[sessionID]
	if ok {
		close(conn.done)
		delete(b.connections, sessionID)
	}
	b.mu.Unlock()
}

// ConnectionCount returns the number of active connections.
func (b *Broker) ConnectionCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.connections)
}

// addConnection registers a new connection.
// If a connection already exists for this session, it is closed.
// This handles the case where a user opens multiple tabs or reconnects.
func (b *Broker) addConnection(conn *connection) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Close existing connection for this session if any.
	// This may close a connection that removeConnection is also trying to clean up,
	// but removeConnection uses identity check (current == conn) to prevent removing
	// the wrong connection. Since we're about to replace the connection in the map,
	// removeConnection will see that the old connection is no longer current and
	// will not delete the map entry. This prevents a race where:
	// 1. Thread A: addConnection closes existing.done
	// 2. Thread B: existing's ServeHTTP returns, calls removeConnection
	// 3. Thread A: Replaces map entry with new connection
	// 4. Thread B: Deletes map entry (would be wrong!)
	// The identity check in removeConnection prevents step 4.
	if existing, ok := b.connections[conn.sessionID]; ok {
		close(existing.done)
	}

	b.connections[conn.sessionID] = conn
}

// removeConnection unregisters a connection.
// Only removes if the connection is still the current one for this session.
func (b *Broker) removeConnection(sessionID string, conn *connection) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Only delete if this connection is still the registered one
	if current, ok := b.connections[sessionID]; ok && current == conn {
		delete(b.connections, sessionID)
	}
}

// sendToConnection sends an event to a specific connection.
// Formats the event according to SSE specification:
//
//	event: <type>
//	data: <json>
//	<blank line>
func (b *Broker) sendToConnection(conn *connection, event Event) error {
	if conn == nil || conn.writer == nil || conn.flusher == nil {
		return fmt.Errorf("connection not available")
	}

	// Marshal data to JSON
	jsonData, err := json.Marshal(event.Data)
	if err != nil {
		return fmt.Errorf("failed to marshal event data: %w", err)
	}

	// Format SSE event
	// event: <type>\n
	// data: <json>\n
	// \n
	_, err = fmt.Fprintf(conn.writer, "event: %s\ndata: %s\n\n", event.Type, jsonData)
	if err != nil {
		return fmt.Errorf("failed to write event: %w", err)
	}

	// Flush immediately
	conn.flusher.Flush()

	return nil
}

// Shutdown gracefully closes all connections.
func (b *Broker) Shutdown(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Close all connections
	for sessionID, conn := range b.connections {
		close(conn.done)
		delete(b.connections, sessionID)
	}

	return nil
}
