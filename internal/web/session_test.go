package web

import (
	"context"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGenerateSessionID(t *testing.T) {
	tests := []struct {
		name string
	}{
		{"generates valid session ID"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := GenerateSessionID()
			if err != nil {
				t.Fatalf("GenerateSessionID() error = %v", err)
			}

			// Verify length (16 bytes = 32 hex characters)
			if len(id) != SessionIDLength*2 {
				t.Errorf("GenerateSessionID() length = %d, want %d", len(id), SessionIDLength*2)
			}

			// Verify it's valid hex
			_, err = hex.DecodeString(id)
			if err != nil {
				t.Errorf("GenerateSessionID() returned invalid hex: %v", err)
			}

			// Verify it's not empty
			if id == "" {
				t.Error("GenerateSessionID() returned empty string")
			}
		})
	}
}

func TestGenerateSessionID_Uniqueness(t *testing.T) {
	// Generate multiple IDs and verify they're unique
	const numIDs = 100
	ids := make(map[string]bool, numIDs)

	for i := 0; i < numIDs; i++ {
		id, err := GenerateSessionID()
		if err != nil {
			t.Fatalf("GenerateSessionID() error = %v", err)
		}

		if ids[id] {
			t.Errorf("GenerateSessionID() generated duplicate ID: %s", id)
		}
		ids[id] = true
	}

	if len(ids) != numIDs {
		t.Errorf("Generated %d unique IDs, want %d", len(ids), numIDs)
	}
}

func TestGetSessionID(t *testing.T) {
	tests := []struct {
		name      string
		ctx       context.Context
		want      string
		wantEmpty bool
	}{
		{
			name:      "no session ID in context",
			ctx:       context.Background(),
			wantEmpty: true,
		},
		{
			name: "session ID in context",
			ctx:  setSessionID(context.Background(), "test-session-id"),
			want: "test-session-id",
		},
		{
			name:      "wrong type in context",
			ctx:       context.WithValue(context.Background(), sessionIDKey, 12345),
			wantEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetSessionID(tt.ctx)
			if tt.wantEmpty {
				if got != "" {
					t.Errorf("GetSessionID() = %q, want empty string", got)
				}
			} else {
				if got != tt.want {
					t.Errorf("GetSessionID() = %q, want %q", got, tt.want)
				}
			}
		})
	}
}

func TestSessionMiddleware_NewSession(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sessionID := GetSessionID(r.Context())
		if sessionID == "" {
			t.Error("Session ID not found in context")
		}
		w.WriteHeader(http.StatusOK)
	})

	wrappedHandler := SessionMiddleware(handler)

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(rec, req)

	// Verify response
	if rec.Code != http.StatusOK {
		t.Errorf("Response code = %d, want %d", rec.Code, http.StatusOK)
	}

	// Verify cookie was set
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("Got %d cookies, want 1", len(cookies))
	}

	cookie := cookies[0]

	// Verify cookie properties
	if cookie.Name != SessionCookieName {
		t.Errorf("Cookie name = %q, want %q", cookie.Name, SessionCookieName)
	}

	if cookie.Value == "" {
		t.Error("Cookie value is empty")
	}

	if cookie.Path != "/" {
		t.Errorf("Cookie path = %q, want %q", cookie.Path, "/")
	}

	if !cookie.HttpOnly {
		t.Error("Cookie HttpOnly = false, want true")
	}

	if cookie.SameSite != http.SameSiteStrictMode {
		t.Errorf("Cookie SameSite = %v, want %v", cookie.SameSite, http.SameSiteStrictMode)
	}

	if cookie.MaxAge != int(SessionExpiry.Seconds()) {
		t.Errorf("Cookie MaxAge = %d, want %d", cookie.MaxAge, int(SessionExpiry.Seconds()))
	}

	// Verify the cookie value is valid hex
	_, err := hex.DecodeString(cookie.Value)
	if err != nil {
		t.Errorf("Cookie value is not valid hex: %v", err)
	}
}

func TestSessionMiddleware_ExistingSession(t *testing.T) {
	existingSessionID := "0123456789abcdef0123456789abcdef"

	var capturedSessionID string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedSessionID = GetSessionID(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	wrappedHandler := SessionMiddleware(handler)

	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{
		Name:  SessionCookieName,
		Value: existingSessionID,
	})
	rec := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(rec, req)

	// Verify response
	if rec.Code != http.StatusOK {
		t.Errorf("Response code = %d, want %d", rec.Code, http.StatusOK)
	}

	// Verify existing session ID was used
	if capturedSessionID != existingSessionID {
		t.Errorf("Session ID = %q, want %q", capturedSessionID, existingSessionID)
	}

	// Verify no new cookie was set (existing session should not create new cookie)
	// Note: This is a design decision - we could also refresh the cookie
	setCookieHeaders := rec.Header().Values("Set-Cookie")
	if len(setCookieHeaders) > 0 {
		// If a cookie was set, it should be the same ID
		for _, header := range setCookieHeaders {
			if strings.Contains(header, SessionCookieName) {
				if !strings.Contains(header, existingSessionID) {
					t.Errorf("Cookie was set with different ID than existing session")
				}
			}
		}
	}
}

func TestSessionMiddleware_Integration(t *testing.T) {
	// Simulates multiple requests from the same "client"
	var firstSessionID, secondSessionID string

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sessionID := GetSessionID(r.Context())
		if sessionID == "" {
			t.Error("Session ID not found in context")
		}
		w.WriteHeader(http.StatusOK)
	})

	wrappedHandler := SessionMiddleware(handler)

	// First request - no cookie
	req1 := httptest.NewRequest("GET", "/", nil)
	rec1 := httptest.NewRecorder()
	wrappedHandler.ServeHTTP(rec1, req1)

	// Extract session ID from first request
	cookies1 := rec1.Result().Cookies()
	if len(cookies1) != 1 {
		t.Fatalf("First request: got %d cookies, want 1", len(cookies1))
	}
	firstSessionID = cookies1[0].Value

	// Second request - with cookie from first request
	req2 := httptest.NewRequest("GET", "/", nil)
	req2.AddCookie(cookies1[0])
	rec2 := httptest.NewRecorder()

	// Capture session ID in handler
	handlerWithCapture := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secondSessionID = GetSessionID(r.Context())
		w.WriteHeader(http.StatusOK)
	})
	wrappedHandler2 := SessionMiddleware(handlerWithCapture)
	wrappedHandler2.ServeHTTP(rec2, req2)

	// Verify session ID persisted
	if firstSessionID == "" {
		t.Error("First session ID is empty")
	}

	if secondSessionID == "" {
		t.Error("Second session ID is empty")
	}

	if firstSessionID != secondSessionID {
		t.Errorf("Session ID changed: first=%q, second=%q", firstSessionID, secondSessionID)
	}
}

func TestSessionMiddleware_EmptyCookie(t *testing.T) {
	// Test that empty cookie value triggers new session generation
	var capturedSessionID string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedSessionID = GetSessionID(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	wrappedHandler := SessionMiddleware(handler)

	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{
		Name:  SessionCookieName,
		Value: "", // Empty value
	})
	rec := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(rec, req)

	// Verify new session ID was generated
	if capturedSessionID == "" {
		t.Error("No session ID was generated for empty cookie")
	}

	// Verify new cookie was set
	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Error("No cookie was set for empty cookie value")
	}
}

func TestValidateSessionID(t *testing.T) {
	tests := []struct {
		name      string
		sessionID string
		want      bool
	}{
		{
			name:      "valid session ID",
			sessionID: "0123456789abcdef0123456789abcdef",
			want:      true,
		},
		{
			name:      "valid session ID with uppercase",
			sessionID: "0123456789ABCDEF0123456789ABCDEF",
			want:      true,
		},
		{
			name:      "empty string",
			sessionID: "",
			want:      false,
		},
		{
			name:      "too short",
			sessionID: "0123456789abcdef",
			want:      false,
		},
		{
			name:      "too long",
			sessionID: "0123456789abcdef0123456789abcdef00",
			want:      false,
		},
		{
			name:      "invalid hex characters",
			sessionID: "0123456789abcdefghij456789abcdef",
			want:      false,
		},
		{
			name:      "special characters",
			sessionID: "0123456789abcdef!@#$456789abcdef",
			want:      false,
		},
		{
			name:      "sql injection attempt",
			sessionID: "' OR '1'='1",
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidateSessionID(tt.sessionID)
			if got != tt.want {
				t.Errorf("ValidateSessionID(%q) = %v, want %v", tt.sessionID, got, tt.want)
			}
		})
	}
}

func TestSessionMiddleware_InvalidSessionID(t *testing.T) {
	tests := []struct {
		name              string
		cookieValue       string
		wantNewSessionGen bool
	}{
		{
			name:              "too short session ID",
			cookieValue:       "short",
			wantNewSessionGen: true,
		},
		{
			name:              "invalid hex characters",
			cookieValue:       "0123456789abcdefghij456789abcdef",
			wantNewSessionGen: true,
		},
		{
			name:              "malicious input",
			cookieValue:       "' OR '1'='1",
			wantNewSessionGen: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedSessionID string
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedSessionID = GetSessionID(r.Context())
				w.WriteHeader(http.StatusOK)
			})

			wrappedHandler := SessionMiddleware(handler)

			req := httptest.NewRequest("GET", "/", nil)
			req.AddCookie(&http.Cookie{
				Name:  SessionCookieName,
				Value: tt.cookieValue,
			})
			rec := httptest.NewRecorder()

			wrappedHandler.ServeHTTP(rec, req)

			// Verify new session ID was generated (different from invalid cookie)
			if capturedSessionID == tt.cookieValue {
				t.Errorf("Invalid session ID was accepted: %q", tt.cookieValue)
			}

			// Verify the generated session ID is valid
			if !ValidateSessionID(capturedSessionID) {
				t.Errorf("Generated session ID is invalid: %q", capturedSessionID)
			}

			// Verify new cookie was set
			cookies := rec.Result().Cookies()
			if len(cookies) == 0 {
				t.Error("No cookie was set for invalid session ID")
			}
		})
	}
}
