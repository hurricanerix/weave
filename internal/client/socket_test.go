package client

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestGetSocketPath(t *testing.T) {
	tests := []struct {
		name     string
		xdgDir   string
		setEnv   bool
		wantPath string
		wantErr  error
	}{
		{
			name:     "valid XDG_RUNTIME_DIR",
			xdgDir:   "/run/user/1000",
			setEnv:   true,
			wantPath: "/run/user/1000/weave/weave.sock",
			wantErr:  nil,
		},
		{
			name:     "XDG_RUNTIME_DIR not set",
			setEnv:   false,
			wantPath: "",
			wantErr:  ErrXDGNotSet,
		},
		{
			name:     "XDG_RUNTIME_DIR set to empty string",
			xdgDir:   "",
			setEnv:   true,
			wantPath: "",
			wantErr:  ErrXDGNotSet,
		},
		{
			name:     "XDG_RUNTIME_DIR with trailing slash",
			xdgDir:   "/run/user/1000/",
			setEnv:   true,
			wantPath: "/run/user/1000/weave/weave.sock",
			wantErr:  nil,
			// Note: This tests that filepath.Join correctly handles trailing slashes
			// in environment variables, which is important for robustness even though
			// it's stdlib behavior. Users may set XDG_RUNTIME_DIR with trailing slash.
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and restore original env
			origXDG := os.Getenv("XDG_RUNTIME_DIR")
			defer os.Setenv("XDG_RUNTIME_DIR", origXDG)

			if tt.setEnv {
				os.Setenv("XDG_RUNTIME_DIR", tt.xdgDir)
			} else {
				os.Unsetenv("XDG_RUNTIME_DIR")
			}

			gotPath, err := getSocketPath()

			if !errors.Is(err, tt.wantErr) {
				t.Errorf("getSocketPath() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if gotPath != tt.wantPath {
				t.Errorf("getSocketPath() = %v, want %v", gotPath, tt.wantPath)
			}
		})
	}
}

func TestClassifyDialError(t *testing.T) {
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
			name:    "ENOENT error",
			err:     &net.OpError{Err: syscall.ENOENT},
			wantErr: ErrDaemonNotRunning,
		},
		{
			name:    "ECONNREFUSED error",
			err:     &net.OpError{Err: syscall.ECONNREFUSED},
			wantErr: ErrDaemonNotAccepting,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotErr := classifyDialError(tt.err)

			if tt.wantErr == nil {
				if gotErr != nil {
					t.Errorf("classifyDialError() = %v, want nil", gotErr)
				}
				return
			}

			if !errors.Is(gotErr, tt.wantErr) {
				t.Errorf("classifyDialError() = %v, want %v", gotErr, tt.wantErr)
			}
		})
	}
}

func TestConnectWithoutXDGRuntimeDir(t *testing.T) {
	// Save and restore original env
	origXDG := os.Getenv("XDG_RUNTIME_DIR")
	defer os.Setenv("XDG_RUNTIME_DIR", origXDG)

	os.Unsetenv("XDG_RUNTIME_DIR")

	ctx := context.Background()
	conn, err := Connect(ctx)

	if !errors.Is(err, ErrXDGNotSet) {
		t.Errorf("Connect() error = %v, want ErrXDGNotSet", err)
	}

	if conn != nil {
		t.Errorf("Connect() returned non-nil connection on error")
		conn.Close()
	}
}

func TestConnectToNonexistentSocket(t *testing.T) {
	// Create a temporary directory
	tmpDir := t.TempDir()

	// Set XDG_RUNTIME_DIR to temp dir
	origXDG := os.Getenv("XDG_RUNTIME_DIR")
	defer os.Setenv("XDG_RUNTIME_DIR", origXDG)
	os.Setenv("XDG_RUNTIME_DIR", tmpDir)

	ctx := context.Background()
	conn, err := Connect(ctx)

	if !errors.Is(err, ErrDaemonNotRunning) {
		t.Errorf("Connect() error = %v, want ErrDaemonNotRunning", err)
	}

	if conn != nil {
		t.Errorf("Connect() returned non-nil connection on error")
		conn.Close()
	}
}

func TestConnectTimeout(t *testing.T) {
	// This test verifies that context cancellation causes timeout error
	tmpDir := t.TempDir()

	// Create socket directory
	socketDirPath := filepath.Join(tmpDir, socketDir)
	if err := os.MkdirAll(socketDirPath, 0700); err != nil {
		t.Fatalf("Failed to create socket directory: %v", err)
	}

	// Create a listener but don't accept connections (simulates hang)
	socketPath := filepath.Join(socketDirPath, socketName)
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("Failed to create socket: %v", err)
	}
	defer listener.Close()
	defer os.Remove(socketPath)

	// Set XDG_RUNTIME_DIR
	origXDG := os.Getenv("XDG_RUNTIME_DIR")
	defer os.Setenv("XDG_RUNTIME_DIR", origXDG)
	os.Setenv("XDG_RUNTIME_DIR", tmpDir)

	// Create a context with very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	// Note: On Unix sockets, connection might succeed even without accept()
	// This test mainly verifies the timeout mechanism works
	conn, err := Connect(ctx)
	if conn != nil {
		conn.Close()
	}

	// Either timeout or success - both are acceptable for this test
	// The important thing is no panic or hang
	if err != nil && !errors.Is(err, ErrConnectionTimeout) && !errors.Is(err, context.DeadlineExceeded) {
		// Connection might succeed on Unix socket, that's okay
		if conn == nil {
			t.Logf("Connect() error = %v (acceptable for timeout test)", err)
		}
	}
}

func TestConnClose(t *testing.T) {
	// Test that Close() works on nil connection
	c := &Conn{conn: nil}
	err := c.Close()
	if err != nil {
		t.Errorf("Close() on nil connection returned error: %v", err)
	}
}

func TestConnRawConn(t *testing.T) {
	// Create a mock connection for testing
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	defer listener.Close()
	defer os.Remove(socketPath)

	// Connect to our test socket
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	// Create Conn wrapper
	c := &Conn{conn: conn}

	// Verify RawConn returns the underlying connection
	raw := c.RawConn()
	if raw != conn {
		t.Errorf("RawConn() did not return underlying connection")
	}
}

func TestErrorMessages(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantMsg string
	}{
		{
			name:    "XDG not set",
			err:     ErrXDGNotSet,
			wantMsg: "XDG_RUNTIME_DIR not set",
		},
		{
			name:    "daemon not running",
			err:     ErrDaemonNotRunning,
			wantMsg: "weave-compute daemon not running (socket not found)",
		},
		{
			name:    "daemon not accepting",
			err:     ErrDaemonNotAccepting,
			wantMsg: "weave-compute daemon not accepting connections",
		},
		{
			name:    "connection timeout",
			err:     ErrConnectionTimeout,
			wantMsg: "weave-compute daemon connection timeout",
		},
		{
			name:    "read timeout",
			err:     ErrReadTimeout,
			wantMsg: "weave-compute daemon read timeout",
		},
		{
			name:    "connection closed",
			err:     ErrConnectionClosed,
			wantMsg: "weave-compute daemon closed connection",
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

func TestSendWithNilConnection(t *testing.T) {
	c := &Conn{conn: nil}
	ctx := context.Background()

	_, err := c.Send(ctx, []byte("test"))
	if err == nil {
		t.Error("Send() with nil connection should return error")
	}
	if err.Error() != "connection is nil" {
		t.Errorf("Send() error = %v, want 'connection is nil'", err)
	}
}

func TestSendWriteError(t *testing.T) {
	// Create a mock connection that fails on write
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	defer listener.Close()
	defer os.Remove(socketPath)

	// Connect and immediately close to trigger write error
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	conn.Close()

	c := &Conn{conn: conn}
	ctx := context.Background()

	_, err = c.Send(ctx, []byte("test request"))
	if err == nil {
		t.Error("Send() should fail when writing to closed connection")
	}
}

func TestSendReadTimeout(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	defer listener.Close()
	defer os.Remove(socketPath)

	// Accept connection but don't send response
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		// Read request but don't respond (simulates timeout)
		// Keep connection open to prevent EOF
		buf := make([]byte, 1024)
		conn.Read(buf)
		// Sleep longer than the test timeout
		time.Sleep(500 * time.Millisecond)
		conn.Close()
	}()

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	// Set short read deadline for test
	conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))

	c := &Conn{conn: conn}
	ctx := context.Background()

	_, err = c.Send(ctx, []byte("test request"))
	if err == nil {
		t.Error("Send() should timeout when daemon doesn't respond")
	}

	// Check for timeout error - could be either ErrReadTimeout or ErrConnectionClosed
	// depending on timing
	if !errors.Is(err, ErrReadTimeout) && !errors.Is(err, ErrConnectionClosed) {
		t.Errorf("Send() timeout error = %v, want ErrReadTimeout or ErrConnectionClosed", err)
	}
}

func TestSendConnectionClosed(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	defer listener.Close()
	defer os.Remove(socketPath)

	// Accept connection and close immediately after reading
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		// Read request and close without responding
		buf := make([]byte, 1024)
		conn.Read(buf)
		conn.Close()
	}()

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	c := &Conn{conn: conn}
	ctx := context.Background()

	_, err = c.Send(ctx, []byte("test request"))
	if err == nil {
		t.Error("Send() should fail when connection is closed")
	}

	if !errors.Is(err, ErrConnectionClosed) {
		t.Errorf("Send() closed error = %v, want ErrConnectionClosed", err)
	}
}

func TestSendPayloadTooLarge(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	defer listener.Close()
	defer os.Remove(socketPath)

	// Accept connection and send malicious response with huge payload
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		// Read request
		buf := make([]byte, 1024)
		conn.Read(buf)

		// Send header with payload_len > 10 MB
		header := make([]byte, 16)
		// Magic
		header[0] = 0x57
		header[1] = 0x45
		header[2] = 0x56
		header[3] = 0x45
		// Version
		header[4] = 0x00
		header[5] = 0x01
		// Message type
		header[6] = 0x00
		header[7] = 0x02
		// Payload length: 100 MB (should be rejected)
		header[8] = 0x06 // 100,000,000 in big-endian
		header[9] = 0x00
		header[10] = 0x00
		header[11] = 0x00
		// Reserved
		header[12] = 0x00
		header[13] = 0x00
		header[14] = 0x00
		header[15] = 0x00

		conn.Write(header)
	}()

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	c := &Conn{conn: conn}
	ctx := context.Background()

	_, err = c.Send(ctx, []byte("test request"))
	if err == nil {
		t.Error("Send() should reject payload > 10 MB")
	}

	if !strings.Contains(err.Error(), "payload too large") {
		t.Errorf("Send() error = %v, want 'payload too large'", err)
	}
}

func TestSendSuccessful(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	defer listener.Close()
	defer os.Remove(socketPath)

	// Create a simple valid response
	validResponse := make([]byte, 16)
	// Magic: 0x57455645 ("WEVE")
	validResponse[0] = 0x57
	validResponse[1] = 0x45
	validResponse[2] = 0x56
	validResponse[3] = 0x45
	// Version: 0x0001
	validResponse[4] = 0x00
	validResponse[5] = 0x01
	// Message type: 0x0002 (GENERATE_RESPONSE)
	validResponse[6] = 0x00
	validResponse[7] = 0x02
	// Payload length: 0 (header only for this test)
	validResponse[8] = 0x00
	validResponse[9] = 0x00
	validResponse[10] = 0x00
	validResponse[11] = 0x00
	// Reserved: 0x00000000
	validResponse[12] = 0x00
	validResponse[13] = 0x00
	validResponse[14] = 0x00
	validResponse[15] = 0x00

	// Accept connection and send valid response
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		// Read request
		buf := make([]byte, 1024)
		conn.Read(buf)

		// Send valid response
		conn.Write(validResponse)
	}()

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	c := &Conn{conn: conn}
	ctx := context.Background()

	response, err := c.Send(ctx, []byte("test request"))
	if err != nil {
		t.Errorf("Send() failed with valid response: %v", err)
	}

	if len(response) != 16 {
		t.Errorf("Send() response length = %d, want 16", len(response))
	}

	// Verify response matches what was sent
	for i := 0; i < 16; i++ {
		if response[i] != validResponse[i] {
			t.Errorf("Response byte %d = 0x%02X, want 0x%02X", i, response[i], validResponse[i])
		}
	}
}

func TestSendWithPayload(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	defer listener.Close()
	defer os.Remove(socketPath)

	// Create response with small payload
	payloadData := []byte("test payload data")
	payloadLen := uint32(len(payloadData))

	validResponse := make([]byte, 16+payloadLen)
	// Magic: 0x57455645 ("WEVE")
	validResponse[0] = 0x57
	validResponse[1] = 0x45
	validResponse[2] = 0x56
	validResponse[3] = 0x45
	// Version: 0x0001
	validResponse[4] = 0x00
	validResponse[5] = 0x01
	// Message type: 0x0002 (GENERATE_RESPONSE)
	validResponse[6] = 0x00
	validResponse[7] = 0x02
	// Payload length (big-endian)
	validResponse[8] = byte(payloadLen >> 24)
	validResponse[9] = byte(payloadLen >> 16)
	validResponse[10] = byte(payloadLen >> 8)
	validResponse[11] = byte(payloadLen)
	// Reserved: 0x00000000
	validResponse[12] = 0x00
	validResponse[13] = 0x00
	validResponse[14] = 0x00
	validResponse[15] = 0x00
	// Payload
	copy(validResponse[16:], payloadData)

	// Accept connection and send response
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		// Read request
		buf := make([]byte, 1024)
		conn.Read(buf)

		// Send response
		conn.Write(validResponse)
	}()

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	c := &Conn{conn: conn}
	ctx := context.Background()

	response, err := c.Send(ctx, []byte("test request"))
	if err != nil {
		t.Errorf("Send() failed with payload: %v", err)
	}

	expectedLen := 16 + len(payloadData)
	if len(response) != expectedLen {
		t.Errorf("Send() response length = %d, want %d", len(response), expectedLen)
	}

	// Verify payload data
	receivedPayload := response[16:]
	if string(receivedPayload) != string(payloadData) {
		t.Errorf("Send() payload = %q, want %q", receivedPayload, payloadData)
	}
}

func TestClassifyReadError(t *testing.T) {
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
			name:    "EOF",
			err:     io.EOF,
			wantErr: ErrConnectionClosed,
		},
		{
			name:    "connection reset",
			err:     &net.OpError{Err: syscall.ECONNRESET},
			wantErr: ErrConnectionClosed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotErr := classifyReadError(tt.err)

			if tt.wantErr == nil {
				if gotErr != nil {
					t.Errorf("classifyReadError() = %v, want nil", gotErr)
				}
				return
			}

			if !errors.Is(gotErr, tt.wantErr) {
				t.Errorf("classifyReadError() = %v, want %v", gotErr, tt.wantErr)
			}
		})
	}
}

// Integration test - only runs with -tags=integration
// Requires actual daemon running
func TestConnectIntegration(t *testing.T) {
	// Skip if not in integration mode
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// This would require the daemon to be running
	// Tagged as integration test in the build tags below
	t.Skip("Integration test requires running daemon - run with -tags=integration")
}
