package relay

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"
	"testing"
)

// mockPoster records every call to Post and optionally returns an error.
type mockPoster struct {
	mu      sync.Mutex
	calls   [][]byte
	errFunc func(index int) error
}

func (m *mockPoster) Post(_ context.Context, body []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	idx := len(m.calls)
	cp := make([]byte, len(body))
	copy(cp, body)
	m.calls = append(m.calls, cp)
	if m.errFunc != nil {
		return m.errFunc(idx)
	}
	return nil
}

func (m *mockPoster) getCalls() [][]byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([][]byte, len(m.calls))
	copy(out, m.calls)
	return out
}

func TestRun(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		errFunc   func(int) error
		wantCalls int
		wantErr   bool
		wantLog   string
	}{
		{
			name:      "single valid JSON object",
			input:     `{"key":"value"}` + "\n",
			wantCalls: 1,
		},
		{
			name:      "multiple valid JSON objects are each forwarded",
			input:     `{"a":1}` + "\n" + `{"b":2}` + "\n" + `{"c":3}` + "\n",
			wantCalls: 3,
		},
		{
			name:      "empty input returns nil (stdin close)",
			input:     "",
			wantCalls: 0,
		},
		{
			name:  "post failure logs error and continues processing",
			input: `{"a":1}` + "\n" + `{"b":2}` + "\n" + `{"c":3}` + "\n",
			errFunc: func(idx int) error {
				if idx == 1 {
					return fmt.Errorf("server unavailable")
				}
				return nil
			},
			wantCalls: 3,
			wantLog:   "post failed",
		},
		{
			name:  "all posts fail but relay still processes every object",
			input: `{"x":1}` + "\n" + `{"y":2}` + "\n",
			errFunc: func(_ int) error {
				return fmt.Errorf("always fails")
			},
			wantCalls: 2,
			wantLog:   "post failed",
		},
		{
			name:      "nested JSON object forwarded intact",
			input:     `{"outer":{"inner":"value"}}` + "\n",
			wantCalls: 1,
		},
		{
			name:      "JSON array forwarded as single value",
			input:     `[1,2,3]` + "\n",
			wantCalls: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var logBuf bytes.Buffer
			logger := log.New(&logBuf, "", 0)
			poster := &mockPoster{errFunc: tt.errFunc}
			r := strings.NewReader(tt.input)

			err := Run(context.Background(), r, poster, logger)

			if tt.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			calls := poster.getCalls()
			if len(calls) != tt.wantCalls {
				t.Errorf("got %d Post calls, want %d", len(calls), tt.wantCalls)
			}

			if tt.wantLog != "" {
				if !strings.Contains(logBuf.String(), tt.wantLog) {
					t.Errorf("log output %q does not contain %q", logBuf.String(), tt.wantLog)
				}
			}
		})
	}
}

func TestRun_MalformedJSON(t *testing.T) {
	var logBuf bytes.Buffer
	logger := log.New(&logBuf, "", 0)
	poster := &mockPoster{}

	// First object is valid, second is malformed, third is valid.
	// The relay should skip the bad line and forward both valid objects.
	input := `{"a":1}` + "\n" + `{bad json` + "\n" + `{"b":2}` + "\n"

	err := Run(context.Background(), strings.NewReader(input), poster, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	calls := poster.getCalls()
	if len(calls) != 2 {
		t.Errorf("got %d Post calls, want 2 (both valid objects)", len(calls))
	}

	if !strings.Contains(logBuf.String(), "decode error") {
		t.Errorf("log output %q does not contain 'decode error'", logBuf.String())
	}
}

func TestRun_ForwardsExactJSON(t *testing.T) {
	input := `{"msg":"hello"}` + "\n" + `{"msg":"world"}` + "\n"
	poster := &mockPoster{}
	logger := log.New(io.Discard, "", 0)

	err := Run(context.Background(), strings.NewReader(input), poster, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	calls := poster.getCalls()
	if len(calls) != 2 {
		t.Fatalf("got %d calls, want 2", len(calls))
	}

	want := []string{`{"msg":"hello"}`, `{"msg":"world"}`}
	for i, w := range want {
		var gotVal, wantVal interface{}
		if err := json.Unmarshal(calls[i], &gotVal); err != nil {
			t.Fatalf("call %d: unmarshal got: %v", i, err)
		}
		if err := json.Unmarshal([]byte(w), &wantVal); err != nil {
			t.Fatalf("call %d: unmarshal want: %v", i, err)
		}
		gotJSON, _ := json.Marshal(gotVal)
		wantJSON, _ := json.Marshal(wantVal)
		if string(gotJSON) != string(wantJSON) {
			t.Errorf("call %d: got %s, want %s", i, gotJSON, wantJSON)
		}
	}
}

func TestRun_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	poster := &mockPoster{}
	logger := log.New(io.Discard, "", 0)

	err := Run(ctx, strings.NewReader(`{"a":1}`+"\n"), poster, logger)
	if err == nil {
		t.Fatal("expected error from cancelled context, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("got error %v, want context.Canceled", err)
	}
}
