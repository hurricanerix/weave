package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/hurricanerix/l5/internal/relay"
)

func TestHTTPPoster_Post(t *testing.T) {
	const runID = "run-abc-123"

	tests := []struct {
		name       string
		statusCode int
		wantErr    bool
	}{
		{
			name:       "200 OK succeeds",
			statusCode: http.StatusOK,
		},
		{
			name:       "201 Created succeeds",
			statusCode: http.StatusCreated,
		},
		{
			name:       "204 No Content succeeds",
			statusCode: http.StatusNoContent,
		},
		{
			name:       "400 Bad Request returns error",
			statusCode: http.StatusBadRequest,
			wantErr:    true,
		},
		{
			name:       "500 Internal Server Error returns error",
			statusCode: http.StatusInternalServerError,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotBody []byte
			var gotContentType string
			var gotRunID string

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotContentType = r.Header.Get("Content-Type")
				gotRunID = r.Header.Get("X-Run-ID")
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Errorf("reading request body: %v", err)
				}
				gotBody = body
				w.WriteHeader(tt.statusCode)
			}))
			defer srv.Close()

			poster := &httpPoster{
				endpoint: srv.URL,
				runID:    runID,
				client:   http.DefaultClient,
			}

			payload := []byte(`{"entry":"test"}`)
			err := poster.Post(context.Background(), payload)

			if tt.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if gotContentType != "application/json" {
				t.Errorf("Content-Type = %q, want %q", gotContentType, "application/json")
			}
			if gotRunID != runID {
				t.Errorf("X-Run-ID = %q, want %q", gotRunID, runID)
			}
			if !bytes.Equal(gotBody, payload) {
				t.Errorf("body = %q, want %q", gotBody, payload)
			}
		})
	}
}

func TestHTTPPoster_RunIDIncludedInEveryRequest(t *testing.T) {
	const runID = "run-xyz-789"
	var mu sync.Mutex
	var receivedRunIDs []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		receivedRunIDs = append(receivedRunIDs, r.Header.Get("X-Run-ID"))
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	poster := &httpPoster{
		endpoint: srv.URL,
		runID:    runID,
		client:   http.DefaultClient,
	}

	bodies := [][]byte{
		[]byte(`{"a":1}`),
		[]byte(`{"b":2}`),
		[]byte(`{"c":3}`),
	}

	for i, body := range bodies {
		if err := poster.Post(context.Background(), body); err != nil {
			t.Fatalf("Post %d: unexpected error: %v", i, err)
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if len(receivedRunIDs) != len(bodies) {
		t.Fatalf("got %d requests, want %d", len(receivedRunIDs), len(bodies))
	}
	for i, id := range receivedRunIDs {
		if id != runID {
			t.Errorf("request %d: X-Run-ID = %q, want %q", i, id, runID)
		}
	}
}

func TestHTTPPoster_UsesCustomClient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	customClient := &http.Client{}
	poster := &httpPoster{
		endpoint: srv.URL,
		runID:    "test",
		client:   customClient,
	}

	if err := poster.Post(context.Background(), []byte(`{}`)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHTTPPoster_NetworkError(t *testing.T) {
	poster := &httpPoster{
		endpoint: "http://127.0.0.1:0/nonexistent",
		runID:    "test",
		client:   http.DefaultClient,
	}

	err := poster.Post(context.Background(), []byte(`{}`))
	if err == nil {
		t.Fatal("expected error for unreachable endpoint, got nil")
	}
}

// TestRun_WithHTTPPoster is an integration-style test of Run using a real
// httpPoster and httptest server. It verifies the full pipeline: decode JSON
// from reader, post via httpPoster, receive at HTTP server with correct
// headers and body.
func TestRun_WithHTTPPoster(t *testing.T) {
	const runID = "e2e-run-456"

	type request struct {
		body  string
		runID string
	}

	var mu sync.Mutex
	var received []request

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("reading body: %v", err)
		}
		mu.Lock()
		received = append(received, request{
			body:  string(body),
			runID: r.Header.Get("X-Run-ID"),
		})
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	poster := &httpPoster{
		endpoint: srv.URL + "/api/agent/journal",
		runID:    runID,
		client:   http.DefaultClient,
	}
	logger := log.New(io.Discard, "", 0)
	input := `{"event":"start"}` + "\n" + `{"event":"end"}` + "\n"

	err := relay.Run(context.Background(), strings.NewReader(input), poster, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(received) != 2 {
		t.Fatalf("got %d requests, want 2", len(received))
	}

	for i, r := range received {
		if r.runID != runID {
			t.Errorf("request %d: X-Run-ID = %q, want %q", i, r.runID, runID)
		}
	}

	wantBodies := []string{`{"event":"start"}`, `{"event":"end"}`}
	for i, want := range wantBodies {
		var gotVal, wantVal interface{}
		if err := json.Unmarshal([]byte(received[i].body), &gotVal); err != nil {
			t.Fatalf("request %d: unmarshal got: %v", i, err)
		}
		if err := json.Unmarshal([]byte(want), &wantVal); err != nil {
			t.Fatalf("request %d: unmarshal want: %v", i, err)
		}
		gotJSON, _ := json.Marshal(gotVal)
		wantJSON, _ := json.Marshal(wantVal)
		if string(gotJSON) != string(wantJSON) {
			t.Errorf("request %d: body = %s, want %s", i, gotJSON, wantJSON)
		}
	}
}

// TestRun_PostFailureWithHTTPPoster tests that when the HTTP server returns
// an error status, Run logs the failure and continues to process remaining
// objects.
func TestRun_PostFailureWithHTTPPoster(t *testing.T) {
	const runID = "fail-run-789"
	var mu sync.Mutex
	var requestCount int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestCount++
		n := requestCount
		mu.Unlock()

		// Verify run ID on every request, even the ones that will fail.
		if got := r.Header.Get("X-Run-ID"); got != runID {
			t.Errorf("request %d: X-Run-ID = %q, want %q", n, got, runID)
		}

		// Fail the second request.
		if n == 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	poster := &httpPoster{
		endpoint: srv.URL,
		runID:    runID,
		client:   http.DefaultClient,
	}

	var logBuf bytes.Buffer
	logger := log.New(&logBuf, "", 0)
	input := `{"a":1}` + "\n" + `{"b":2}` + "\n" + `{"c":3}` + "\n"

	err := relay.Run(context.Background(), strings.NewReader(input), poster, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	// All three objects should have been posted despite the second one failing.
	if requestCount != 3 {
		t.Errorf("got %d requests, want 3", requestCount)
	}

	// The failure should have been logged.
	if !strings.Contains(logBuf.String(), "post failed") {
		t.Errorf("log output %q does not contain 'post failed'", logBuf.String())
	}
}
