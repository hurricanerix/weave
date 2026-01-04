// Package client provides connectivity to the weave-compute daemon.
//
// Connections are established per-request via Connect(). The daemon performs
// SO_PEERCRED authentication at the socket level to verify the connecting
// process's identity. No tokens or credentials are needed in the protocol.
//
// Typical usage:
//
//	conn, err := client.Connect(ctx)
//	if err != nil {
//	    return err
//	}
//	defer conn.Close()
//
//	// Use conn.RawConn() for protocol-level communication
//	err = protocol.SendRequest(conn.RawConn(), req)
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
	// Set to 65s to be slightly longer than daemon's 60s timeout to avoid race
	readTimeout = 65 * time.Second
	// maxPayloadSize is the maximum size of a response payload (10 MB)
	maxPayloadSize = 10 * 1024 * 1024
)

var (
	// ErrXDGNotSet is returned when XDG_RUNTIME_DIR environment variable is not set
	ErrXDGNotSet = errors.New("XDG_RUNTIME_DIR not set")
	// ErrDaemonNotRunning is returned when the socket file doesn't exist
	ErrDaemonNotRunning = errors.New("weave-compute daemon not running (socket not found)")
	// ErrDaemonNotAccepting is returned when connection is refused
	ErrDaemonNotAccepting = errors.New("weave-compute daemon not accepting connections")
	// ErrConnectionTimeout is returned when connection attempt times out
	ErrConnectionTimeout = errors.New("weave-compute daemon connection timeout")
	// ErrReadTimeout is returned when reading from socket times out
	ErrReadTimeout = errors.New("weave-compute daemon read timeout")
	// ErrConnectionClosed is returned when the daemon closes the connection unexpectedly
	ErrConnectionClosed = errors.New("weave-compute daemon closed connection")
)

// Conn represents a connection to the weave-compute daemon
type Conn struct {
	conn net.Conn
}

// Connect establishes a connection to the weave-compute daemon.
// It reads XDG_RUNTIME_DIR from the environment, constructs the socket path,
// and connects with appropriate timeouts.
//
// Returns ErrXDGNotSet if XDG_RUNTIME_DIR is not set.
// Returns ErrDaemonNotRunning if the socket file doesn't exist.
// Returns ErrDaemonNotAccepting if the daemon refuses the connection.
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

// Close closes the connection to the daemon
func (c *Conn) Close() error {
	if c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

// RawConn returns the underlying net.Conn for protocol layer access.
// Use this for reading/writing binary protocol messages.
func (c *Conn) RawConn() net.Conn {
	return c.conn
}

// Send sends a protocol message to the daemon and reads the response.
// It writes the request bytes to the socket, resets the read deadline,
// reads the full response, and returns the response bytes.
//
// The read deadline is reset before reading to ensure proper timeout
// behavior per request (the deadline set in Connect() is connection-scoped).
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

	// Validate payload length (protect against malicious daemon)
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
				return ErrDaemonNotRunning
			}
			// ECONNREFUSED - daemon not accepting connections
			if errors.Is(opErr.Err, syscall.ECONNREFUSED) {
				return ErrDaemonNotAccepting
			}
		}
	}

	// Return the original error if we can't classify it
	return fmt.Errorf("failed to connect to daemon: %w", err)
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
	return fmt.Errorf("failed to read from daemon: %w", err)
}
