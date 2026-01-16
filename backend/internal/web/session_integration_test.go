package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestSessionIntegration_ServerWithMiddleware verifies that the server
// has session middleware enabled and sessions persist across requests.
func TestSessionIntegration_ServerWithMiddleware(t *testing.T) {
	server, err := NewServer("")
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	// Make first request through the server's handler (which includes middleware)
	req1 := httptest.NewRequest("GET", "/", nil)
	rec1 := httptest.NewRecorder()

	server.server.Handler.ServeHTTP(rec1, req1)

	if rec1.Code != http.StatusOK {
		t.Errorf("First request status = %d, want %d", rec1.Code, http.StatusOK)
	}

	// Extract session cookie
	cookies1 := rec1.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, cookie := range cookies1 {
		if cookie.Name == SessionCookieName {
			sessionCookie = cookie
			break
		}
	}

	if sessionCookie == nil {
		t.Fatal("No session cookie set on first request")
	}

	firstSessionID := sessionCookie.Value
	if firstSessionID == "" {
		t.Fatal("Session ID is empty")
	}

	// Verify cookie properties
	if sessionCookie.Path != "/" {
		t.Errorf("Cookie path = %q, want %q", sessionCookie.Path, "/")
	}

	if !sessionCookie.HttpOnly {
		t.Error("Cookie HttpOnly = false, want true")
	}

	if sessionCookie.SameSite != http.SameSiteStrictMode {
		t.Errorf("Cookie SameSite = %v, want %v", sessionCookie.SameSite, http.SameSiteStrictMode)
	}

	// Make second request with the same cookie
	// Using POST /prompt since POST /chat now requires message content
	req2 := httptest.NewRequest("POST", "/prompt", nil)
	req2.AddCookie(sessionCookie)
	rec2 := httptest.NewRecorder()

	server.server.Handler.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Errorf("Second request status = %d, want %d", rec2.Code, http.StatusOK)
	}

	// Make third request without cookie to verify new session is created
	req3 := httptest.NewRequest("GET", "/", nil)
	rec3 := httptest.NewRecorder()

	server.server.Handler.ServeHTTP(rec3, req3)

	cookies3 := rec3.Result().Cookies()
	var newSessionCookie *http.Cookie
	for _, cookie := range cookies3 {
		if cookie.Name == SessionCookieName {
			newSessionCookie = cookie
			break
		}
	}

	if newSessionCookie == nil {
		t.Fatal("No session cookie set on third request")
	}

	if newSessionCookie.Value == firstSessionID {
		t.Error("Expected new session ID for request without cookie, got same ID")
	}
}

// TestSessionIntegration_AllEndpoints verifies that all server endpoints
// have session middleware applied.
func TestSessionIntegration_AllEndpoints(t *testing.T) {
	server, err := NewServer("")
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	endpoints := []struct {
		method string
		path   string
	}{
		{"GET", "/"},
		// Note: SSE endpoint is skipped because it blocks waiting for connection
		{"POST", "/chat"},
		{"POST", "/prompt"},
		{"POST", "/generate"},
	}

	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			req := httptest.NewRequest(ep.method, ep.path, nil)
			rec := httptest.NewRecorder()

			server.server.Handler.ServeHTTP(rec, req)

			// All endpoints should set a session cookie
			cookies := rec.Result().Cookies()
			var hasSessionCookie bool
			for _, cookie := range cookies {
				if cookie.Name == SessionCookieName {
					hasSessionCookie = true
					if cookie.Value == "" {
						t.Error("Session cookie value is empty")
					}
					break
				}
			}

			if !hasSessionCookie {
				t.Error("No session cookie set")
			}
		})
	}
}
