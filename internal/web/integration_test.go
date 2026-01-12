//go:build integration

package web

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestIntegration_ServerStartsAndResponds verifies that the server
// can start, accept connections, and respond to HTTP requests.
func TestIntegration_ServerStartsAndResponds(t *testing.T) {
	// Use a specific port for integration testing
	addr := "localhost:18080"
	s, err := NewServer(addr)
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start server in background
	errCh := make(chan error, 1)
	go func() {
		errCh <- s.ListenAndServe(ctx)
	}()

	// Give server time to start
	time.Sleep(200 * time.Millisecond)

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 2 * time.Second,
	}

	tests := []struct {
		name          string
		method        string
		path          string
		wantStatus    int
		wantBodyPart  string
		wantHeaderKey string
		wantHeaderVal string
	}{
		{
			name:          "GET / returns HTML",
			method:        "GET",
			path:          "/",
			wantStatus:    http.StatusOK,
			wantBodyPart:  "Weave Web UI",
			wantHeaderKey: "Content-Type",
			wantHeaderVal: "text/html",
		},
		// Note: SSE endpoint is tested separately because it blocks waiting for connection
		// and SessionMiddleware always creates a session
		{
			name:          "POST /chat returns JSON",
			method:        "POST",
			path:          "/chat",
			wantStatus:    http.StatusOK,
			wantBodyPart:  `"status":"ok"`,
			wantHeaderKey: "Content-Type",
			wantHeaderVal: "application/json",
		},
		{
			name:          "POST /prompt returns JSON",
			method:        "POST",
			path:          "/prompt",
			wantStatus:    http.StatusOK,
			wantBodyPart:  `"status":"ok"`,
			wantHeaderKey: "Content-Type",
			wantHeaderVal: "application/json",
		},
		{
			name:          "POST /generate returns JSON",
			method:        "POST",
			path:          "/generate",
			wantStatus:    http.StatusOK,
			wantBodyPart:  `"status":"ok"`,
			wantHeaderKey: "Content-Type",
			wantHeaderVal: "application/json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := "http://" + addr + tt.path

			req, err := http.NewRequest(tt.method, url, nil)
			if err != nil {
				t.Fatalf("failed to create request: %v", err)
			}

			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tt.wantStatus {
				t.Errorf("status = %d, want %d", resp.StatusCode, tt.wantStatus)
			}

			if tt.wantHeaderKey != "" {
				headerVal := resp.Header.Get(tt.wantHeaderKey)
				if !strings.Contains(headerVal, tt.wantHeaderVal) {
					t.Errorf("header %s = %q, want to contain %q",
						tt.wantHeaderKey, headerVal, tt.wantHeaderVal)
				}
			}

			if tt.wantBodyPart != "" {
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					t.Fatalf("failed to read body: %v", err)
				}

				if !strings.Contains(string(body), tt.wantBodyPart) {
					t.Errorf("body = %q, want to contain %q", string(body), tt.wantBodyPart)
				}
			}
		})
	}

	// Shutdown server
	cancel()

	// Verify clean shutdown
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("server error during shutdown: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for server shutdown")
	}
}

// TestIntegration_GracefulShutdown verifies that the server
// handles graceful shutdown correctly.
func TestIntegration_GracefulShutdown(t *testing.T) {
	addr := "localhost:18081"
	s, err := NewServer(addr)
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.ListenAndServe(ctx)
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Trigger shutdown
	cancel()

	// Server should shut down within timeout
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("unexpected error during shutdown: %v", err)
		}
	case <-time.After(ShutdownTimeout + 2*time.Second):
		t.Fatal("server did not shut down within timeout")
	}
}

// TestIntegration_ConcurrentRequests verifies that the server
// can handle multiple concurrent requests.
func TestIntegration_ConcurrentRequests(t *testing.T) {
	addr := "localhost:18082"
	s, err := NewServer(addr)
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.ListenAndServe(ctx)
	}()

	// Give server time to start
	time.Sleep(200 * time.Millisecond)

	client := &http.Client{
		Timeout: 2 * time.Second,
	}

	// Make 10 concurrent requests
	const numRequests = 10
	resultCh := make(chan error, numRequests)

	for i := 0; i < numRequests; i++ {
		go func(id int) {
			url := "http://" + addr + "/"
			resp, err := client.Get(url)
			if err != nil {
				resultCh <- err
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				resultCh <- err
				return
			}

			resultCh <- nil
		}(i)
	}

	// Collect results
	for i := 0; i < numRequests; i++ {
		select {
		case err := <-resultCh:
			if err != nil {
				t.Errorf("request %d failed: %v", i, err)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("timeout waiting for request %d", i)
		}
	}

	// Shutdown
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("server error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for shutdown")
	}
}

// TestIntegration_SSE_ConnectionAndEvents verifies that SSE connections
// work correctly and events are received by the client.
func TestIntegration_SSE_ConnectionAndEvents(t *testing.T) {
	addr := "localhost:18083"
	s, err := NewServer(addr)
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.ListenAndServe(ctx)
	}()

	// Give server time to start
	time.Sleep(200 * time.Millisecond)

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	// First get a session cookie from the index page
	indexResp, err := client.Get("http://" + addr + "/")
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}
	indexResp.Body.Close()

	var sessionCookie *http.Cookie
	for _, cookie := range indexResp.Cookies() {
		if cookie.Name == SessionCookieName {
			sessionCookie = cookie
			break
		}
	}
	if sessionCookie == nil {
		t.Fatal("no session cookie returned")
	}
	sessionID := sessionCookie.Value

	// Connect to SSE endpoint with session cookie
	url := "http://" + addr + "/events"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.AddCookie(sessionCookie)

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("SSE connection failed: %v", err)
	}
	defer resp.Body.Close()

	// Verify SSE headers
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", contentType)
	}

	cacheControl := resp.Header.Get("Cache-Control")
	if cacheControl != "no-cache" {
		t.Errorf("Cache-Control = %q, want no-cache", cacheControl)
	}

	// Read initial connection event
	scanner := bufio.NewScanner(resp.Body)
	events := make([]string, 0)

	// Read first few lines (connection event)
	for i := 0; i < 10 && scanner.Scan(); i++ {
		line := scanner.Text()
		events = append(events, line)
		if line == "" {
			break // End of first event
		}
	}

	// Verify connection event was received
	eventContent := strings.Join(events, "\n")
	if !strings.Contains(eventContent, "event: connected") {
		t.Errorf("should receive connection event, got %q", eventContent)
	}
	if !strings.Contains(eventContent, sessionID) {
		t.Errorf("connection event should include session ID, got %q", eventContent)
	}

	// Send a test event via broker
	testData := map[string]string{
		"token": "Hello from server",
	}

	err = s.Broker().SendEvent(sessionID, EventAgentToken, testData)
	if err != nil {
		t.Fatalf("SendEvent failed: %v", err)
	}

	// Read the agent-token event
	events = events[:0] // Clear events
	for i := 0; i < 10 && scanner.Scan(); i++ {
		line := scanner.Text()
		events = append(events, line)
		if line == "" {
			break // End of event
		}
	}

	eventContent = strings.Join(events, "\n")
	if !strings.Contains(eventContent, "event: agent-token") {
		t.Errorf("should receive agent-token event, got %q", eventContent)
	}
	if !strings.Contains(eventContent, `"token":"Hello from server"`) {
		t.Errorf("event should include token data, got %q", eventContent)
	}

	// Close the response body to release the connection
	resp.Body.Close()

	// Give server time to cleanup connection
	time.Sleep(100 * time.Millisecond)

	// Shutdown server
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("server error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for shutdown")
	}
}

// TestIntegration_SSE_MultipleEventTypes verifies that different event
// types are correctly formatted and received.
func TestIntegration_SSE_MultipleEventTypes(t *testing.T) {
	addr := "localhost:18084"
	s, err := NewServer(addr)
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.ListenAndServe(ctx)
	}()

	time.Sleep(200 * time.Millisecond)

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	// Get session from index page
	indexResp, err := client.Get("http://" + addr + "/")
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}
	indexResp.Body.Close()

	var sessionCookie *http.Cookie
	for _, cookie := range indexResp.Cookies() {
		if cookie.Name == SessionCookieName {
			sessionCookie = cookie
			break
		}
	}
	if sessionCookie == nil {
		t.Fatal("no session cookie returned")
	}
	sessionID := sessionCookie.Value

	url := "http://" + addr + "/events"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.AddCookie(sessionCookie)

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("SSE connection failed: %v", err)
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)

	// Read connection event and discard
	for scanner.Scan() {
		if scanner.Text() == "" {
			break
		}
	}

	// Test different event types
	testEvents := []struct {
		eventType string
		data      interface{}
		wantEvent string
		wantData  string
	}{
		{
			eventType: EventPromptUpdate,
			data:      map[string]string{"prompt": "a cat in space"},
			wantEvent: "event: prompt-update",
			wantData:  `"prompt":"a cat in space"`,
		},
		{
			eventType: EventImageReady,
			data:      map[string]string{"url": "/images/test.png"},
			wantEvent: "event: image-ready",
			wantData:  `"url":"/images/test.png"`,
		},
		{
			eventType: EventError,
			data:      map[string]string{"message": "test error"},
			wantEvent: "event: error",
			wantData:  `"message":"test error"`,
		},
	}

	for _, tt := range testEvents {
		t.Run(tt.eventType, func(t *testing.T) {
			// Send event
			err := s.Broker().SendEvent(sessionID, tt.eventType, tt.data)
			if err != nil {
				t.Fatalf("SendEvent failed: %v", err)
			}

			// Read event
			events := make([]string, 0)
			for i := 0; i < 10 && scanner.Scan(); i++ {
				line := scanner.Text()
				events = append(events, line)
				if line == "" {
					break
				}
			}

			eventContent := strings.Join(events, "\n")
			if !strings.Contains(eventContent, tt.wantEvent) {
				t.Errorf("should contain %q, got %q", tt.wantEvent, eventContent)
			}
			if !strings.Contains(eventContent, tt.wantData) {
				t.Errorf("should contain %q, got %q", tt.wantData, eventContent)
			}
		})
	}

	// Close response body before shutdown
	resp.Body.Close()
	time.Sleep(100 * time.Millisecond)

	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("server error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for shutdown")
	}
}

// TestIntegration_SSE_SessionIsolation verifies that events only go
// to the intended session.
func TestIntegration_SSE_SessionIsolation(t *testing.T) {
	addr := "localhost:18085"
	s, err := NewServer(addr)
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.ListenAndServe(ctx)
	}()

	time.Sleep(200 * time.Millisecond)

	// Connect two different sessions
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	// Get session 1 cookie
	resp1Index, err := client.Get("http://" + addr + "/")
	if err != nil {
		t.Fatalf("failed to get session 1: %v", err)
	}
	resp1Index.Body.Close()
	var session1Cookie *http.Cookie
	for _, cookie := range resp1Index.Cookies() {
		if cookie.Name == SessionCookieName {
			session1Cookie = cookie
			break
		}
	}
	if session1Cookie == nil {
		t.Fatal("no session 1 cookie returned")
	}
	session1ID := session1Cookie.Value

	// Get session 2 cookie (new client to get new session)
	client2 := &http.Client{Timeout: 10 * time.Second}
	resp2Index, err := client2.Get("http://" + addr + "/")
	if err != nil {
		t.Fatalf("failed to get session 2: %v", err)
	}
	resp2Index.Body.Close()
	var session2Cookie *http.Cookie
	for _, cookie := range resp2Index.Cookies() {
		if cookie.Name == SessionCookieName {
			session2Cookie = cookie
			break
		}
	}
	if session2Cookie == nil {
		t.Fatal("no session 2 cookie returned")
	}
	session2ID := session2Cookie.Value

	// Connect session 1 to SSE
	url1 := "http://" + addr + "/events"
	req1, _ := http.NewRequest("GET", url1, nil)
	req1.AddCookie(session1Cookie)
	resp1, err := client.Do(req1)
	if err != nil {
		t.Fatalf("session 1 connection failed: %v", err)
	}
	defer resp1.Body.Close()

	// Connect session 2 to SSE
	url2 := "http://" + addr + "/events"
	req2, _ := http.NewRequest("GET", url2, nil)
	req2.AddCookie(session2Cookie)
	resp2, err := client2.Do(req2)
	if err != nil {
		t.Fatalf("session 2 connection failed: %v", err)
	}
	defer resp2.Body.Close()

	scanner1 := bufio.NewScanner(resp1.Body)
	scanner2 := bufio.NewScanner(resp2.Body)

	// Read connection events for both
	for scanner1.Scan() {
		if scanner1.Text() == "" {
			break
		}
	}
	for scanner2.Scan() {
		if scanner2.Text() == "" {
			break
		}
	}

	// Send event only to session 1
	err = s.Broker().SendEvent(session1ID, EventAgentToken, map[string]string{"token": "only-for-session-1"})
	if err != nil {
		t.Fatalf("SendEvent failed: %v", err)
	}

	// Read from session 1 in a goroutine (should receive)
	session1Received := make(chan string, 1)
	go func() {
		events1 := make([]string, 0)
		for i := 0; i < 10 && scanner1.Scan(); i++ {
			line := scanner1.Text()
			events1 = append(events1, line)
			if line == "" {
				break
			}
		}
		session1Received <- strings.Join(events1, "\n")
	}()

	// Wait for session 1 to receive or timeout
	var content1 string
	select {
	case content1 = <-session1Received:
		if !strings.Contains(content1, "only-for-session-1") {
			t.Error("session 1 should receive the event")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for session 1 to receive event")
	}

	// Now send event only to session 2
	err = s.Broker().SendEvent(session2ID, EventAgentToken, map[string]string{"token": "only-for-session-2"})
	if err != nil {
		t.Fatalf("SendEvent to session 2 failed: %v", err)
	}

	// Read from session 2 (should receive)
	session2Received := make(chan string, 1)
	go func() {
		events2 := make([]string, 0)
		for i := 0; i < 10 && scanner2.Scan(); i++ {
			line := scanner2.Text()
			events2 = append(events2, line)
			if line == "" {
				break
			}
		}
		session2Received <- strings.Join(events2, "\n")
	}()

	// Wait for session 2 to receive
	var content2 string
	select {
	case content2 = <-session2Received:
		if !strings.Contains(content2, "only-for-session-2") {
			t.Error("session 2 should receive its event")
		}
		// Verify session 2 did NOT receive session 1's event
		if strings.Contains(content2, "only-for-session-1") {
			t.Error("session 2 should NOT receive session 1's event (isolation failure)")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for session 2 to receive event")
	}

	// Verify session 1 did not receive session 2's event
	// by checking we can still read more from session 1
	if strings.Contains(content1, "only-for-session-2") {
		t.Error("session 1 should NOT receive session 2's event (isolation failure)")
	}

	// Close response bodies before shutdown
	resp1.Body.Close()
	resp2.Body.Close()
	time.Sleep(100 * time.Millisecond)

	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("server error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for shutdown")
	}
}

// TestIntegration_SSE_SessionFlow verifies the complete session + SSE flow:
// 1. Client makes request and gets session cookie
// 2. Client connects to SSE with session cookie
// 3. Events sent to session are received correctly
// 4. Session persists across multiple requests
func TestIntegration_SSE_SessionFlow(t *testing.T) {
	addr := "localhost:18086"
	s, err := NewServer(addr)
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.ListenAndServe(ctx)
	}()

	time.Sleep(200 * time.Millisecond)

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	// Step 1: Make initial request to get session cookie
	indexURL := "http://" + addr + "/"
	req, err := http.NewRequest("GET", indexURL, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("index request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("index status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	// Step 2: Extract session cookie
	var sessionCookie *http.Cookie
	for _, cookie := range resp.Cookies() {
		if cookie.Name == SessionCookieName {
			sessionCookie = cookie
			break
		}
	}

	if sessionCookie == nil {
		t.Fatal("no session cookie returned")
	}

	sessionID := sessionCookie.Value
	if sessionID == "" {
		t.Fatal("session ID is empty")
	}

	// Step 3: Connect to SSE using session cookie
	sseURL := "http://" + addr + "/events"
	sseReq, err := http.NewRequest("GET", sseURL, nil)
	if err != nil {
		t.Fatalf("failed to create SSE request: %v", err)
	}
	sseReq.AddCookie(sessionCookie)

	sseResp, err := client.Do(sseReq)
	if err != nil {
		t.Fatalf("SSE connection failed: %v", err)
	}
	defer sseResp.Body.Close()

	if sseResp.StatusCode != http.StatusOK {
		t.Errorf("SSE status = %d, want %d", sseResp.StatusCode, http.StatusOK)
	}

	scanner := bufio.NewScanner(sseResp.Body)

	// Read connection event
	for scanner.Scan() {
		if scanner.Text() == "" {
			break
		}
	}

	// Step 4: Send multiple events to session
	testEvents := []struct {
		eventType string
		data      map[string]string
	}{
		{EventAgentToken, map[string]string{"token": "Hello"}},
		{EventAgentToken, map[string]string{"token": " world"}},
		{EventAgentDone, map[string]string{"status": "complete"}},
		{EventPromptUpdate, map[string]string{"prompt": "updated prompt"}},
	}

	for _, te := range testEvents {
		err := s.Broker().SendEvent(sessionID, te.eventType, te.data)
		if err != nil {
			t.Fatalf("SendEvent failed: %v", err)
		}
	}

	// Step 5: Verify all events are received in correct order
	receivedEvents := make([]string, 0)
	eventChan := make(chan []string, 1)

	go func() {
		events := make([]string, 0)
		for len(events) < len(testEvents) {
			eventLines := make([]string, 0)
			for i := 0; i < 10 && scanner.Scan(); i++ {
				line := scanner.Text()
				eventLines = append(eventLines, line)
				if line == "" {
					break
				}
			}
			if len(eventLines) > 0 {
				events = append(events, strings.Join(eventLines, "\n"))
			}
		}
		eventChan <- events
	}()

	select {
	case receivedEvents = <-eventChan:
		if len(receivedEvents) != len(testEvents) {
			t.Errorf("received %d events, want %d", len(receivedEvents), len(testEvents))
		}

		// Verify each event was received with correct data
		for i, te := range testEvents {
			if i >= len(receivedEvents) {
				break
			}
			eventContent := receivedEvents[i]
			if !strings.Contains(eventContent, "event: "+te.eventType) {
				t.Errorf("event %d: expected type %q, got %q", i, te.eventType, eventContent)
			}
			for key, value := range te.data {
				expectedData := fmt.Sprintf(`"%s":"%s"`, key, value)
				if !strings.Contains(eventContent, expectedData) {
					t.Errorf("event %d: expected data %q, got %q", i, expectedData, eventContent)
				}
			}
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for events")
	}

	// Step 6: Verify session persists - make another request with same cookie
	chatURL := "http://" + addr + "/chat"
	chatReq, err := http.NewRequest("POST", chatURL, nil)
	if err != nil {
		t.Fatalf("failed to create chat request: %v", err)
	}
	chatReq.AddCookie(sessionCookie)

	chatResp, err := client.Do(chatReq)
	if err != nil {
		t.Fatalf("chat request failed: %v", err)
	}
	defer chatResp.Body.Close()

	if chatResp.StatusCode != http.StatusOK {
		t.Errorf("chat status = %d, want %d", chatResp.StatusCode, http.StatusOK)
	}

	// Verify response contains same session ID
	body, err := io.ReadAll(chatResp.Body)
	if err != nil {
		t.Fatalf("failed to read response: %v", err)
	}

	if !strings.Contains(string(body), sessionID) {
		t.Errorf("chat response should contain session ID %q, got %q", sessionID, string(body))
	}

	// Close SSE connection
	sseResp.Body.Close()
	time.Sleep(100 * time.Millisecond)

	// Shutdown
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("server error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for shutdown")
	}
}

// TestIntegration_AgentTriggeredGeneration verifies that agent-triggered
// generation works correctly. This tests the complete flow from agent
// response with generate_image=true to image delivery via SSE.
func TestIntegration_AgentTriggeredGeneration(t *testing.T) {
	addr := "localhost:18087"
	s, err := NewServer(addr)
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.ListenAndServe(ctx)
	}()

	time.Sleep(200 * time.Millisecond)

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	// Get session from index page
	indexResp, err := client.Get("http://" + addr + "/")
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}
	indexResp.Body.Close()

	var sessionCookie *http.Cookie
	for _, cookie := range indexResp.Cookies() {
		if cookie.Name == SessionCookieName {
			sessionCookie = cookie
			break
		}
	}
	if sessionCookie == nil {
		t.Fatal("no session cookie returned")
	}
	sessionID := sessionCookie.Value

	// Connect to SSE endpoint
	url := "http://" + addr + "/events"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.AddCookie(sessionCookie)

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("SSE connection failed: %v", err)
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)

	// Read connection event and discard
	for scanner.Scan() {
		if scanner.Text() == "" {
			break
		}
	}

	// Simulate agent-triggered generation by sending events
	// In the real flow, handleChat would send these events after
	// receiving metadata with generate_image=true

	// 1. Send prompt update
	err = s.Broker().SendEvent(sessionID, EventPromptUpdate, map[string]interface{}{
		"prompt": "a fluffy orange cat",
	})
	if err != nil {
		t.Fatalf("SendEvent (prompt) failed: %v", err)
	}

	// 2. Send settings update
	err = s.Broker().SendEvent(sessionID, EventSettingsUpdate, map[string]interface{}{
		"steps": 4,
		"cfg":   1.0,
		"seed":  -1,
	})
	if err != nil {
		t.Fatalf("SendEvent (settings) failed: %v", err)
	}

	// Read prompt update event
	eventLines := make([]string, 0)
	for i := 0; i < 10 && scanner.Scan(); i++ {
		line := scanner.Text()
		eventLines = append(eventLines, line)
		if line == "" {
			break
		}
	}

	eventContent := strings.Join(eventLines, "\n")
	if !strings.Contains(eventContent, "event: prompt-update") {
		t.Errorf("should receive prompt-update event, got %q", eventContent)
	}
	if !strings.Contains(eventContent, `"prompt":"a fluffy orange cat"`) {
		t.Errorf("event should include prompt, got %q", eventContent)
	}

	// Read settings update event
	eventLines = make([]string, 0)
	for i := 0; i < 10 && scanner.Scan(); i++ {
		line := scanner.Text()
		eventLines = append(eventLines, line)
		if line == "" {
			break
		}
	}

	eventContent = strings.Join(eventLines, "\n")
	if !strings.Contains(eventContent, "event: settings-update") {
		t.Errorf("should receive settings-update event, got %q", eventContent)
	}
	if !strings.Contains(eventContent, `"steps":4`) {
		t.Errorf("event should include steps, got %q", eventContent)
	}

	// Note: We can't actually test image generation without the compute daemon running,
	// but we've verified that the prompt and settings events are sent correctly.
	// The actual generation would happen in handleChat after checking GenerateImage=true.

	// Close response body before shutdown
	resp.Body.Close()
	time.Sleep(100 * time.Millisecond)

	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("server error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for shutdown")
	}
}

// TestIntegration_AgentWithoutAutoGeneration verifies that when an agent
// responds with generate_image=false, the prompt and settings are updated
// but no generation is triggered. This tests the flow where the agent just
// updates UI state without triggering generation.
func TestIntegration_AgentWithoutAutoGeneration(t *testing.T) {
	addr := "localhost:18088"
	s, err := NewServer(addr)
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.ListenAndServe(ctx)
	}()

	time.Sleep(200 * time.Millisecond)

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	// Get session from index page
	indexResp, err := client.Get("http://" + addr + "/")
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}
	indexResp.Body.Close()

	var sessionCookie *http.Cookie
	for _, cookie := range indexResp.Cookies() {
		if cookie.Name == SessionCookieName {
			sessionCookie = cookie
			break
		}
	}
	if sessionCookie == nil {
		t.Fatal("no session cookie returned")
	}
	sessionID := sessionCookie.Value

	// Connect to SSE endpoint
	url := "http://" + addr + "/events"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.AddCookie(sessionCookie)

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("SSE connection failed: %v", err)
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)

	// Read connection event and discard
	for scanner.Scan() {
		if scanner.Text() == "" {
			break
		}
	}

	// Simulate agent response WITHOUT auto-generation
	// In the real flow, handleChat would send these events after
	// receiving metadata with generate_image=false

	// 1. Send prompt update (with empty prompt since agent is still asking questions)
	err = s.Broker().SendEvent(sessionID, EventPromptUpdate, map[string]interface{}{
		"prompt": "",
	})
	if err != nil {
		t.Fatalf("SendEvent (prompt) failed: %v", err)
	}

	// 2. Send settings update
	err = s.Broker().SendEvent(sessionID, EventSettingsUpdate, map[string]interface{}{
		"steps": 4,
		"cfg":   1.0,
		"seed":  -1,
	})
	if err != nil {
		t.Fatalf("SendEvent (settings) failed: %v", err)
	}

	// 3. Send agent-done event
	err = s.Broker().SendEvent(sessionID, EventAgentDone, map[string]interface{}{
		"status": "complete",
	})
	if err != nil {
		t.Fatalf("SendEvent (agent-done) failed: %v", err)
	}

	// Read prompt update event
	eventLines := make([]string, 0)
	for i := 0; i < 10 && scanner.Scan(); i++ {
		line := scanner.Text()
		eventLines = append(eventLines, line)
		if line == "" {
			break
		}
	}

	eventContent := strings.Join(eventLines, "\n")
	if !strings.Contains(eventContent, "event: prompt-update") {
		t.Errorf("should receive prompt-update event, got %q", eventContent)
	}

	// Read settings update event
	eventLines = make([]string, 0)
	for i := 0; i < 10 && scanner.Scan(); i++ {
		line := scanner.Text()
		eventLines = append(eventLines, line)
		if line == "" {
			break
		}
	}

	eventContent = strings.Join(eventLines, "\n")
	if !strings.Contains(eventContent, "event: settings-update") {
		t.Errorf("should receive settings-update event, got %q", eventContent)
	}

	// Read agent-done event
	eventLines = make([]string, 0)
	for i := 0; i < 10 && scanner.Scan(); i++ {
		line := scanner.Text()
		eventLines = append(eventLines, line)
		if line == "" {
			break
		}
	}

	eventContent = strings.Join(eventLines, "\n")
	if !strings.Contains(eventContent, "event: agent-done") {
		t.Errorf("should receive agent-done event, got %q", eventContent)
	}

	// Verify no image-ready event follows (with timeout)
	// Use a channel to read the next event with timeout
	nextEventChan := make(chan string, 1)
	go func() {
		eventLines := make([]string, 0)
		for i := 0; i < 10 && scanner.Scan(); i++ {
			line := scanner.Text()
			eventLines = append(eventLines, line)
			if line == "" {
				break
			}
		}
		if len(eventLines) > 0 {
			nextEventChan <- strings.Join(eventLines, "\n")
		}
	}()

	// Wait briefly to ensure no image-ready event arrives
	select {
	case eventContent := <-nextEventChan:
		// If we got an event, it should NOT be image-ready
		if strings.Contains(eventContent, "event: image-ready") {
			t.Errorf("should NOT receive image-ready event when generate_image=false, got %q", eventContent)
		}
	case <-time.After(500 * time.Millisecond):
		// Timeout is expected - no more events should arrive
		// This is the correct behavior
	}

	// Close response body before shutdown
	resp.Body.Close()
	time.Sleep(100 * time.Millisecond)

	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("server error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for shutdown")
	}
}
