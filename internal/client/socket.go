// Package client provides connectivity to the weave-compute process.
//
// There are two connection modes:
//
//  1. Per-request connections (legacy): Use Connect() to dial the socket for each request.
//     This is the old pattern where compute owned the socket.
//
//  2. Persistent connection (new): Use AcceptConnection() to accept compute's connection.
//     This is the new pattern where weave owns the socket and compute connects once.
//     All requests are multiplexed over a single persistent connection.
//
// Typical usage (new pattern):
//
//	listener, err := net.Listen("unix", socketPath)
//	if err != nil {
//	    return err
//	}
//	defer listener.Close()
//
//	conn, err := client.AcceptConnection(ctx, listener)
//	if err != nil {
//	    return err
//	}
//	defer conn.Close()
//
//	// Multiple goroutines can call Send() concurrently
//	response, err := conn.Send(ctx, request)
package client

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

const (
	// socketDir is the subdirectory under XDG_RUNTIME_DIR where the socket is created
	socketDir = "weave"
	// socketName is the name of the Unix domain socket file
	socketName = "weave.sock"
	// connectTimeout is the maximum time to wait for a connection to establish
	connectTimeout = 5 * time.Second
	// readTimeout is the maximum time to wait for reading from the socket
	// Set to 65s to be slightly longer than compute's 60s timeout to avoid race
	readTimeout = 65 * time.Second
	// maxPayloadSize is the maximum size of a response payload (10 MB)
	maxPayloadSize = 10 * 1024 * 1024
)

var (
	// ErrXDGNotSet is returned when XDG_RUNTIME_DIR environment variable is not set
	ErrXDGNotSet = errors.New("XDG_RUNTIME_DIR not set")
	// ErrComputeNotRunning is returned when the socket file doesn't exist (compute process not running)
	ErrComputeNotRunning = errors.New("weave-compute process not running (socket not found)")
	// ErrComputeNotAccepting is returned when connection is refused (compute process not accepting)
	ErrComputeNotAccepting = errors.New("weave-compute process not accepting connections")
	// ErrConnectionTimeout is returned when connection attempt times out
	ErrConnectionTimeout = errors.New("weave-compute process connection timeout")
	// ErrReadTimeout is returned when reading from socket times out
	ErrReadTimeout = errors.New("weave-compute process read timeout")
	// ErrConnectionClosed is returned when the compute process closes the connection unexpectedly
	ErrConnectionClosed = errors.New("weave-compute process closed connection")
	// ErrAcceptTimeout is returned when accepting a connection times out
	ErrAcceptTimeout = errors.New("timeout waiting for compute process connection")
	// ErrReaderDead is returned when the response reader goroutine has stopped
	ErrReaderDead = errors.New("response reader goroutine has stopped")
)

// Conn represents a connection to the weave-compute process.
//
// For persistent connections (created via AcceptConnection), the connection
// is multiplexed: multiple concurrent requests are sent over the same socket,
// and responses are routed back to the correct caller based on request ID.
//
// For per-request connections (created via Connect), the connection is not
// multiplexed and behaves like the legacy pattern.
type Conn struct {
	conn net.Conn

	// Multiplexing fields (nil for per-request connections)
	mu              sync.Mutex
	pendingRequests map[uint64]chan []byte // Maps request ID to response channel
	readerDone      chan struct{}          // Closed when response reader exits
	readerErr       error                  // Error from response reader (if any)
}

// Connect establishes a connection to the weave-compute process.
// It reads XDG_RUNTIME_DIR from the environment, constructs the socket path,
// and connects with appropriate timeouts.
//
// Returns ErrXDGNotSet if XDG_RUNTIME_DIR is not set.
// Returns ErrComputeNotRunning if the socket file doesn't exist.
// Returns ErrComputeNotAccepting if the compute process refuses the connection.
// Returns ErrConnectionTimeout if the connection attempt times out.
func Connect(ctx context.Context) (*Conn, error) {
	socketPath, err := getSocketPath()
	if err != nil {
		return nil, err
	}

	// Create a dialer with connection timeout
	dialer := &net.Dialer{
		Timeout: connectTimeout,
	}

	// Attempt to connect with context
	conn, err := dialer.DialContext(ctx, "unix", socketPath)
	if err != nil {
		return nil, classifyDialError(err)
	}

	// Set read timeout on the connection.
	// Note: This deadline is connection-scoped (fixed at 65s from connection time),
	// not per-operation. Task 005 (protocol implementation) should reset the deadline
	// before each read operation to ensure proper timeout behavior per-request.
	if err := conn.SetReadDeadline(time.Now().Add(readTimeout)); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to set read timeout: %w", err)
	}

	return &Conn{conn: conn}, nil
}

// AcceptConnection accepts a single connection from the compute process.
// It blocks until a connection is accepted or the context is cancelled.
//
// The returned Conn supports request multiplexing: multiple goroutines can
// call Send() concurrently, and responses are routed back to the correct
// caller based on request ID.
//
// A background goroutine reads responses from the socket and routes them to
// pending requests. This goroutine runs until the connection is closed or
// an unrecoverable read error occurs.
//
// CALLER MUST call Close() on the returned Conn to stop the background
// goroutine and release resources.
//
// Returns ErrAcceptTimeout if the accept times out.
// Returns ErrConnectionClosed if the connection closes during setup.
func AcceptConnection(ctx context.Context, listener net.Listener) (*Conn, error) {
	// Accept with timeout
	acceptCh := make(chan net.Conn, 1)
	errCh := make(chan error, 1)

	// Start accept goroutine
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			errCh <- err
			return
		}
		acceptCh <- conn
	}()

	var conn net.Conn
	select {
	case <-ctx.Done():
		// Close listener to unblock Accept() and stop goroutine
		listener.Close()
		return nil, ErrAcceptTimeout
	case err := <-errCh:
		return nil, classifyDialError(err)
	case conn = <-acceptCh:
		// Connection accepted
	}

	// Do not set a fixed read deadline on the connection.
	// The responseReader() will block indefinitely on reads, which is correct
	// behavior for a persistent connection. Individual request timeouts are
	// handled by context deadlines in Send().

	// Create multiplexed connection
	c := &Conn{
		conn:            conn,
		pendingRequests: make(map[uint64]chan []byte),
		readerDone:      make(chan struct{}),
	}

	// Start response reader goroutine
	go c.responseReader()

	return c, nil
}

// Close closes the connection to the compute process.
// For multiplexed connections, this also stops the response reader goroutine.
func (c *Conn) Close() error {
	if c.conn == nil {
		return nil
	}

	// Close the underlying connection
	// This will cause responseReader to exit with read error
	err := c.conn.Close()

	// Wait for response reader to exit (if it exists)
	if c.readerDone != nil {
		<-c.readerDone
	}

	return err
}

// RawConn returns the underlying net.Conn for protocol layer access.
// Use this for reading/writing binary protocol messages.
func (c *Conn) RawConn() net.Conn {
	return c.conn
}

// responseReader reads responses from the socket and routes them to pending requests.
// This runs in a background goroutine for multiplexed connections (created via AcceptConnection).
// It continues until the connection is closed or an unrecoverable error occurs.
//
// The reader extracts the request ID from each response header (bytes 16-23)
// and delivers the response to the corresponding channel in pendingRequests.
func (c *Conn) responseReader() {
	defer close(c.readerDone)

	for {
		// Read response header (16 bytes)
		header := make([]byte, 16)
		if _, err := io.ReadFull(c.conn, header); err != nil {
			c.mu.Lock()
			c.readerErr = classifyReadError(err)
			// Notify all pending requests of the error
			for _, ch := range c.pendingRequests {
				close(ch)
			}
			c.pendingRequests = make(map[uint64]chan []byte)
			c.mu.Unlock()
			return
		}

		// Extract payload length from header (bytes 8-11, big-endian)
		payloadLen := binary.BigEndian.Uint32(header[8:12])

		// Validate payload length
		if payloadLen > maxPayloadSize {
			c.mu.Lock()
			c.readerErr = fmt.Errorf("payload too large: %d bytes (max %d)", payloadLen, maxPayloadSize)
			// Notify all pending requests of the error
			for _, ch := range c.pendingRequests {
				close(ch)
			}
			c.pendingRequests = make(map[uint64]chan []byte)
			c.mu.Unlock()
			return
		}

		// Allocate buffer for full message (header + payload)
		totalLen := 16 + payloadLen
		response := make([]byte, totalLen)
		copy(response, header)

		// Read remaining payload
		if payloadLen > 0 {
			if _, err := io.ReadFull(c.conn, response[16:]); err != nil {
				c.mu.Lock()
				c.readerErr = classifyReadError(err)
				// Notify all pending requests of the error
				for _, ch := range c.pendingRequests {
					close(ch)
				}
				c.pendingRequests = make(map[uint64]chan []byte)
				c.mu.Unlock()
				return
			}
		}

		// Extract request ID from response payload (bytes 16-23, little-endian)
		// The protocol has: Header (16 bytes) + RequestID (8 bytes) + ...
		if len(response) < 24 {
			// Response too short to contain request ID - skip it
			continue
		}
		requestID := binary.LittleEndian.Uint64(response[16:24])

		// Route response to the correct pending request
		c.mu.Lock()
		ch, ok := c.pendingRequests[requestID]
		if ok {
			delete(c.pendingRequests, requestID)
			c.mu.Unlock()

			// Send response to waiting goroutine
			// Use non-blocking send in case the receiver has timed out
			select {
			case ch <- response:
			default:
				// Receiver already timed out - discard response
			}
		} else {
			c.mu.Unlock()
			// No pending request for this ID - likely a late response after timeout
			// This is benign, just discard it
		}
	}
}

// Send sends a protocol message to the compute process and reads the response.
//
// For multiplexed connections (created via AcceptConnection), this method:
// - Extracts the request ID from the request
// - Registers a response channel for this request ID
// - Writes the request to the socket
// - Waits for the response reader to deliver the response
// - Returns the response or error
//
// For non-multiplexed connections (created via Connect), this method:
// - Writes the request to the socket
// - Reads the response directly (legacy behavior)
// - Returns the response or error
//
// Multiple goroutines can call Send() concurrently on multiplexed connections.
// Responses are routed back to the correct caller based on request ID.
//
// Returns the response bytes or an error if the send/receive fails.
func (c *Conn) Send(ctx context.Context, request []byte) ([]byte, error) {
	if c.conn == nil {
		return nil, errors.New("connection is nil")
	}

	// Check if context is already cancelled
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Check if this is a multiplexed connection
	if c.pendingRequests != nil {
		return c.sendMultiplexed(ctx, request)
	}

	// Non-multiplexed connection (legacy behavior)
	return c.sendDirect(ctx, request)
}

// sendMultiplexed sends a request over a multiplexed connection.
// It extracts the request ID, registers a response channel, and waits for
// the response reader to deliver the response.
func (c *Conn) sendMultiplexed(ctx context.Context, request []byte) ([]byte, error) {
	// Extract request ID from request (bytes 16-23, little-endian)
	// Protocol: Header (16 bytes) + RequestID (8 bytes) + ...
	if len(request) < 24 {
		return nil, fmt.Errorf("request too short: %d bytes (minimum 24)", len(request))
	}
	requestID := binary.LittleEndian.Uint64(request[16:24])

	// Create response channel for this request
	responseCh := make(chan []byte, 1)

	// Register pending request
	c.mu.Lock()
	// Check if response reader is still alive
	select {
	case <-c.readerDone:
		c.mu.Unlock()
		if c.readerErr != nil {
			return nil, c.readerErr
		}
		return nil, ErrReaderDead
	default:
	}
	c.pendingRequests[requestID] = responseCh
	c.mu.Unlock()

	// Write request to socket
	// The write is protected by the net.Conn's internal locking
	if _, err := c.conn.Write(request); err != nil {
		// Remove pending request on write failure
		c.mu.Lock()
		delete(c.pendingRequests, requestID)
		c.mu.Unlock()
		return nil, fmt.Errorf("failed to write request: %w", err)
	}

	// Wait for response or timeout
	// Calculate timeout duration, checking for already-cancelled context
	timeout := readTimeout
	if ctxDeadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(ctxDeadline)
		if remaining <= 0 {
			// Context already expired
			c.mu.Lock()
			delete(c.pendingRequests, requestID)
			c.mu.Unlock()
			return nil, ctx.Err()
		}
		timeout = remaining
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		// Context cancelled - remove pending request
		c.mu.Lock()
		delete(c.pendingRequests, requestID)
		c.mu.Unlock()
		return nil, ctx.Err()

	case <-timer.C:
		// Timeout - remove pending request
		c.mu.Lock()
		delete(c.pendingRequests, requestID)
		c.mu.Unlock()
		return nil, ErrReadTimeout

	case <-c.readerDone:
		// Response reader died - return error
		c.mu.Lock()
		delete(c.pendingRequests, requestID)
		err := c.readerErr
		c.mu.Unlock()
		if err != nil {
			return nil, err
		}
		return nil, ErrReaderDead

	case response, ok := <-responseCh:
		if !ok {
			// Channel closed by response reader due to error
			c.mu.Lock()
			err := c.readerErr
			c.mu.Unlock()
			if err != nil {
				return nil, err
			}
			return nil, ErrConnectionClosed
		}
		return response, nil
	}
}

// sendDirect sends a request over a non-multiplexed connection (legacy behavior).
// This is the original implementation used by Connect().
func (c *Conn) sendDirect(ctx context.Context, request []byte) ([]byte, error) {
	// Write request to socket
	if _, err := c.conn.Write(request); err != nil {
		return nil, fmt.Errorf("failed to write request: %w", err)
	}

	// Set read deadline based on context deadline or default timeout
	deadline := time.Now().Add(readTimeout)
	if ctxDeadline, ok := ctx.Deadline(); ok {
		deadline = ctxDeadline
	}
	if err := c.conn.SetReadDeadline(deadline); err != nil {
		return nil, fmt.Errorf("failed to reset read deadline: %w", err)
	}

	// Read response header first (16 bytes) to determine payload length
	header := make([]byte, 16)
	if _, err := io.ReadFull(c.conn, header); err != nil {
		return nil, classifyReadError(err)
	}

	// Extract payload length from header (bytes 8-11, big-endian)
	payloadLen := binary.BigEndian.Uint32(header[8:12])

	// Validate payload length (protect against malicious compute process)
	if payloadLen > maxPayloadSize {
		return nil, fmt.Errorf("payload too large: %d bytes (max %d)", payloadLen, maxPayloadSize)
	}

	// Allocate buffer for full message (header + payload)
	totalLen := 16 + payloadLen
	response := make([]byte, totalLen)
	copy(response, header)

	// Read remaining payload
	if payloadLen > 0 {
		if _, err := io.ReadFull(c.conn, response[16:]); err != nil {
			return nil, classifyReadError(err)
		}
	}

	return response, nil
}

// getSocketPath constructs the socket path from XDG_RUNTIME_DIR
func getSocketPath() (string, error) {
	xdgRuntimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if xdgRuntimeDir == "" {
		return "", ErrXDGNotSet
	}

	return filepath.Join(xdgRuntimeDir, socketDir, socketName), nil
}

// classifyDialError converts low-level dial errors into user-friendly errors
func classifyDialError(err error) error {
	if err == nil {
		return nil
	}

	// Check for context deadline exceeded (timeout)
	if errors.Is(err, context.DeadlineExceeded) {
		return ErrConnectionTimeout
	}

	// Check for timeout from net package
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return ErrConnectionTimeout
	}

	// Check for syscall errors
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		if opErr.Err != nil {
			// ENOENT - socket file doesn't exist
			if errors.Is(opErr.Err, syscall.ENOENT) {
				return ErrComputeNotRunning
			}
			// ECONNREFUSED - compute process not accepting connections
			if errors.Is(opErr.Err, syscall.ECONNREFUSED) {
				return ErrComputeNotAccepting
			}
		}
	}

	// Return the original error if we can't classify it
	return fmt.Errorf("failed to connect to compute process: %w", err)
}

// classifyReadError converts low-level read errors into user-friendly errors
func classifyReadError(err error) error {
	if err == nil {
		return nil
	}

	// Check for EOF (connection closed)
	if errors.Is(err, io.EOF) {
		return ErrConnectionClosed
	}

	// Check for timeout
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return ErrReadTimeout
	}

	// Check for connection reset
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		if opErr.Err != nil {
			if errors.Is(opErr.Err, syscall.ECONNRESET) {
				return ErrConnectionClosed
			}
		}
	}

	// Return wrapped error
	return fmt.Errorf("failed to read from compute process: %w", err)
}
