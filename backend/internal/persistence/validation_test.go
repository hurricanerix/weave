package persistence

import (
	"testing"
)

func TestValidateSessionID(t *testing.T) {
	tests := []struct {
		name      string
		sessionID string
		wantErr   bool
	}{
		{
			name:      "valid session ID",
			sessionID: "abcdef0123456789abcdef0123456789",
			wantErr:   false,
		},
		{
			name:      "all zeros valid",
			sessionID: "00000000000000000000000000000000",
			wantErr:   false,
		},
		{
			name:      "all f valid",
			sessionID: "ffffffffffffffffffffffffffffffff",
			wantErr:   false,
		},
		{
			name:      "empty string",
			sessionID: "",
			wantErr:   true,
		},
		{
			name:      "too short",
			sessionID: "abcdef0123456789",
			wantErr:   true,
		},
		{
			name:      "too long",
			sessionID: "abcdef0123456789abcdef0123456789ff",
			wantErr:   true,
		},
		{
			name:      "uppercase hex",
			sessionID: "ABCDEF0123456789ABCDEF0123456789",
			wantErr:   true,
		},
		{
			name:      "mixed case hex",
			sessionID: "AbCdEf0123456789AbCdEf0123456789",
			wantErr:   true,
		},
		{
			name:      "path traversal with ..",
			sessionID: "../abcdef0123456789abcdef01234567",
			wantErr:   true,
		},
		{
			name:      "path traversal with .. in middle",
			sessionID: "abcdef..89abcdef0123456789abcdef",
			wantErr:   true,
		},
		{
			name:      "forward slash",
			sessionID: "abcdef/123456789abcdef0123456789",
			wantErr:   true,
		},
		{
			name:      "backslash",
			sessionID: "abcdef\\123456789abcdef0123456789",
			wantErr:   true,
		},
		{
			name:      "contains spaces",
			sessionID: "abcdef 123456789abcdef0123456789",
			wantErr:   true,
		},
		{
			name:      "contains non-hex",
			sessionID: "ghijkl0123456789abcdef0123456789",
			wantErr:   true,
		},
		{
			name:      "null byte",
			sessionID: "abcdef0123456789\x00bcdef0123456789",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSessionID(tt.sessionID)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateSessionID() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
