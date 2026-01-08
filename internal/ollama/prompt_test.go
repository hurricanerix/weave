package ollama

import "testing"

func TestParseResponse(t *testing.T) {
	tests := []struct {
		name               string
		response           string
		wantText           string
		wantPrompt         string
		wantReady          bool
		wantErr            error
		wantErrDescription string
	}{
		{
			name:       "valid response with prompt ready",
			response:   "Perfect! Generating your image now.\n---\n{\"prompt\": \"a tabby cat wearing a blue wizard hat\", \"ready\": true}",
			wantText:   "Perfect! Generating your image now.",
			wantPrompt: "a tabby cat wearing a blue wizard hat",
			wantReady:  true,
			wantErr:    nil,
		},
		{
			name:       "valid response still asking questions",
			response:   "What kind of cat? Hat style?\n---\n{\"prompt\": \"\", \"ready\": false}",
			wantText:   "What kind of cat? Hat style?",
			wantPrompt: "",
			wantReady:  false,
			wantErr:    nil,
		},
		{
			name:       "valid response with empty prompt not ready",
			response:   "I need more details about the setting.\n---\n{\"prompt\": \"\", \"ready\": false}",
			wantText:   "I need more details about the setting.",
			wantPrompt: "",
			wantReady:  false,
			wantErr:    nil,
		},
		{
			name:       "valid response with multiline conversational text",
			response:   "Great! I have all the details I need.\n\nLet me create that image for you.\n---\n{\"prompt\": \"realistic photo of a golden retriever\", \"ready\": true}",
			wantText:   "Great! I have all the details I need.\n\nLet me create that image for you.",
			wantPrompt: "realistic photo of a golden retriever",
			wantReady:  true,
			wantErr:    nil,
		},
		{
			name:       "valid response with whitespace before delimiter",
			response:   "Here you go!   \n   ---\n{\"prompt\": \"a cat in space\", \"ready\": true}",
			wantText:   "Here you go!",
			wantPrompt: "a cat in space",
			wantReady:  true,
			wantErr:    nil,
		},
		{
			name:       "valid response with whitespace after delimiter",
			response:   "Ready!\n---   \n   {\"prompt\": \"a dog on the moon\", \"ready\": true}",
			wantText:   "Ready!",
			wantPrompt: "a dog on the moon",
			wantReady:  true,
			wantErr:    nil,
		},
		{
			name:               "missing delimiter",
			response:           "This response has no delimiter or JSON",
			wantText:           "",
			wantPrompt:         "",
			wantReady:          false,
			wantErr:            ErrMissingDelimiter,
			wantErrDescription: "response missing delimiter",
		},
		{
			name:               "delimiter but no JSON",
			response:           "Some text\n---\n",
			wantText:           "",
			wantPrompt:         "",
			wantReady:          false,
			wantErr:            ErrInvalidJSON,
			wantErrDescription: "invalid JSON after delimiter",
		},
		{
			name:               "delimiter with invalid JSON",
			response:           "Some text\n---\n{this is not valid json}",
			wantText:           "",
			wantPrompt:         "",
			wantReady:          false,
			wantErr:            ErrInvalidJSON,
			wantErrDescription: "invalid JSON after delimiter",
		},
		{
			name:               "delimiter with malformed JSON",
			response:           "Some text\n---\n{\"prompt\": \"missing closing brace\", \"ready\": true",
			wantText:           "",
			wantPrompt:         "",
			wantReady:          false,
			wantErr:            ErrInvalidJSON,
			wantErrDescription: "invalid JSON after delimiter",
		},
		{
			name:               "valid JSON missing prompt field",
			response:           "Some text\n---\n{\"ready\": true}",
			wantText:           "",
			wantPrompt:         "",
			wantReady:          false,
			wantErr:            ErrMissingFields,
			wantErrDescription: "JSON missing required fields",
		},
		{
			name:               "valid JSON missing ready field",
			response:           "Some text\n---\n{\"prompt\": \"a cat\"}",
			wantText:           "",
			wantPrompt:         "",
			wantReady:          false,
			wantErr:            ErrMissingFields,
			wantErrDescription: "JSON missing required fields",
		},
		{
			name:               "valid JSON missing both fields",
			response:           "Some text\n---\n{\"other\": \"field\"}",
			wantText:           "",
			wantPrompt:         "",
			wantReady:          false,
			wantErr:            ErrMissingFields,
			wantErrDescription: "JSON missing required fields",
		},
		{
			name:               "empty JSON object",
			response:           "Some text\n---\n{}",
			wantText:           "",
			wantPrompt:         "",
			wantReady:          false,
			wantErr:            ErrMissingFields,
			wantErrDescription: "JSON missing required fields",
		},
		{
			name:       "delimiter in conversational text",
			response:   "I'll use --- as a separator in the prompt.\n---\n{\"prompt\": \"image with --- separator\", \"ready\": true}",
			wantText:   "I'll use --- as a separator in the prompt.",
			wantPrompt: "image with --- separator",
			wantReady:  true,
			wantErr:    nil,
		},
		{
			name:       "response with extra fields in JSON",
			response:   "Great!\n---\n{\"prompt\": \"a cat\", \"ready\": true, \"confidence\": 0.95, \"other\": \"field\"}",
			wantText:   "Great!",
			wantPrompt: "a cat",
			wantReady:  true,
			wantErr:    nil,
		},
		{
			name:       "response with special characters in prompt",
			response:   "Done!\n---\n{\"prompt\": \"a cat with \\\"quotes\\\" and \\n newlines\", \"ready\": true}",
			wantText:   "Done!",
			wantPrompt: "a cat with \"quotes\" and \n newlines",
			wantReady:  true,
			wantErr:    nil,
		},
		{
			name:       "response with unicode in conversational text",
			response:   "Perfect! 🎨\n---\n{\"prompt\": \"a cat in Tokyo\", \"ready\": true}",
			wantText:   "Perfect! 🎨",
			wantPrompt: "a cat in Tokyo",
			wantReady:  true,
			wantErr:    nil,
		},
		{
			name:       "compact format no whitespace",
			response:   "Done\n---\n{\"prompt\":\"a cat\",\"ready\":true}",
			wantText:   "Done",
			wantPrompt: "a cat",
			wantReady:  true,
			wantErr:    nil,
		},
		{
			name:       "formatted JSON with indentation",
			response:   "Done\n---\n{\n  \"prompt\": \"a cat\",\n  \"ready\": true\n}",
			wantText:   "Done",
			wantPrompt: "a cat",
			wantReady:  true,
			wantErr:    nil,
		},
		{
			name:       "multiple delimiters on own lines uses last",
			response:   "Here's a separator:\n---\nLet me continue.\n---\n{\"prompt\": \"a cat\", \"ready\": true}",
			wantText:   "Here's a separator:\n---\nLet me continue.",
			wantPrompt: "a cat",
			wantReady:  true,
			wantErr:    nil,
		},
		{
			name:               "JSON with prompt as number",
			response:           "Done\n---\n{\"prompt\": 123, \"ready\": true}",
			wantText:           "",
			wantPrompt:         "",
			wantReady:          false,
			wantErr:            ErrInvalidJSON,
			wantErrDescription: "invalid JSON after delimiter",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotText, gotMetadata, gotErr := parseResponse(tt.response)

			// Check error
			if gotErr != tt.wantErr {
				t.Errorf("parseResponse() error = %v, wantErr %v", gotErr, tt.wantErr)
				if tt.wantErrDescription != "" && gotErr != nil {
					t.Errorf("  error description = %q, want %q", gotErr.Error(), tt.wantErrDescription)
				}
				return
			}

			// If we expected an error, don't check the other return values
			if tt.wantErr != nil {
				return
			}

			// Check conversational text
			if gotText != tt.wantText {
				t.Errorf("parseResponse() text = %q, want %q", gotText, tt.wantText)
			}

			// Check metadata fields
			if gotMetadata.Prompt != tt.wantPrompt {
				t.Errorf("parseResponse() metadata.Prompt = %q, want %q", gotMetadata.Prompt, tt.wantPrompt)
			}
			if gotMetadata.Ready != tt.wantReady {
				t.Errorf("parseResponse() metadata.Ready = %v, want %v", gotMetadata.Ready, tt.wantReady)
			}
		})
	}
}
