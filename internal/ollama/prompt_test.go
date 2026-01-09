package ollama

import "testing"

func TestParseResponse(t *testing.T) {
	tests := []struct {
		name               string
		response           string
		wantText           string
		wantPrompt         string
		wantReady          bool
		wantSteps          int
		wantCFG            float64
		wantSeed           int64
		wantErr            error
		wantErrDescription string
	}{
		{
			name:       "valid response with prompt ready",
			response:   "Perfect! Generating your image now.\n---\n{\"prompt\": \"a tabby cat wearing a blue wizard hat\", \"ready\": true, \"steps\": 28, \"cfg\": 7.5, \"seed\": 42}",
			wantText:   "Perfect! Generating your image now.",
			wantPrompt: "a tabby cat wearing a blue wizard hat",
			wantReady:  true,
			wantSteps:  28,
			wantCFG:    7.5,
			wantSeed:   42,
			wantErr:    nil,
		},
		{
			name:       "valid response still asking questions",
			response:   "What kind of cat? Hat style?\n---\n{\"prompt\": \"\", \"ready\": false, \"steps\": 20, \"cfg\": 7.0, \"seed\": 0}",
			wantText:   "What kind of cat? Hat style?",
			wantPrompt: "",
			wantReady:  false,
			wantSteps:  20,
			wantCFG:    7.0,
			wantSeed:   0,
			wantErr:    nil,
		},
		{
			name:       "valid response with empty prompt not ready",
			response:   "I need more details about the setting.\n---\n{\"prompt\": \"\", \"ready\": false, \"steps\": 25, \"cfg\": 8.0, \"seed\": 123}",
			wantText:   "I need more details about the setting.",
			wantPrompt: "",
			wantReady:  false,
			wantSteps:  25,
			wantCFG:    8.0,
			wantSeed:   123,
			wantErr:    nil,
		},
		{
			name:       "valid response with multiline conversational text",
			response:   "Great! I have all the details I need.\n\nLet me create that image for you.\n---\n{\"prompt\": \"realistic photo of a golden retriever\", \"ready\": true, \"steps\": 30, \"cfg\": 7.5, \"seed\": 999}",
			wantText:   "Great! I have all the details I need.\n\nLet me create that image for you.",
			wantPrompt: "realistic photo of a golden retriever",
			wantReady:  true,
			wantSteps:  30,
			wantCFG:    7.5,
			wantSeed:   999,
			wantErr:    nil,
		},
		{
			name:       "valid response with whitespace before delimiter",
			response:   "Here you go!   \n   ---\n{\"prompt\": \"a cat in space\", \"ready\": true, \"steps\": 15, \"cfg\": 6.0, \"seed\": 555}",
			wantText:   "Here you go!",
			wantPrompt: "a cat in space",
			wantReady:  true,
			wantSteps:  15,
			wantCFG:    6.0,
			wantSeed:   555,
			wantErr:    nil,
		},
		{
			name:       "valid response with whitespace after delimiter",
			response:   "Ready!\n---   \n   {\"prompt\": \"a dog on the moon\", \"ready\": true, \"steps\": 40, \"cfg\": 9.0, \"seed\": 1234}",
			wantText:   "Ready!",
			wantPrompt: "a dog on the moon",
			wantReady:  true,
			wantSteps:  40,
			wantCFG:    9.0,
			wantSeed:   1234,
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
			name:               "valid JSON missing steps field",
			response:           "Some text\n---\n{\"prompt\": \"a cat\", \"ready\": true, \"cfg\": 7.5, \"seed\": 42}",
			wantText:           "",
			wantPrompt:         "",
			wantReady:          false,
			wantErr:            ErrMissingFields,
			wantErrDescription: "JSON missing required fields",
		},
		{
			name:               "valid JSON missing cfg field",
			response:           "Some text\n---\n{\"prompt\": \"a cat\", \"ready\": true, \"steps\": 28, \"seed\": 42}",
			wantText:           "",
			wantPrompt:         "",
			wantReady:          false,
			wantErr:            ErrMissingFields,
			wantErrDescription: "JSON missing required fields",
		},
		{
			name:               "valid JSON missing seed field",
			response:           "Some text\n---\n{\"prompt\": \"a cat\", \"ready\": true, \"steps\": 28, \"cfg\": 7.5}",
			wantText:           "",
			wantPrompt:         "",
			wantReady:          false,
			wantErr:            ErrMissingFields,
			wantErrDescription: "JSON missing required fields",
		},
		{
			name:               "valid JSON with only new fields",
			response:           "Some text\n---\n{\"steps\": 28, \"cfg\": 7.5, \"seed\": 42}",
			wantText:           "",
			wantPrompt:         "",
			wantReady:          false,
			wantErr:            ErrMissingFields,
			wantErrDescription: "JSON missing required fields",
		},
		{
			name:       "delimiter in conversational text",
			response:   "I'll use --- as a separator in the prompt.\n---\n{\"prompt\": \"image with --- separator\", \"ready\": true, \"steps\": 20, \"cfg\": 7.5, \"seed\": 42}",
			wantText:   "I'll use --- as a separator in the prompt.",
			wantPrompt: "image with --- separator",
			wantReady:  true,
			wantSteps:  20,
			wantCFG:    7.5,
			wantSeed:   42,
			wantErr:    nil,
		},
		{
			name:       "response with extra fields in JSON",
			response:   "Great!\n---\n{\"prompt\": \"a cat\", \"ready\": true, \"steps\": 28, \"cfg\": 7.0, \"seed\": 100, \"confidence\": 0.95, \"other\": \"field\"}",
			wantText:   "Great!",
			wantPrompt: "a cat",
			wantReady:  true,
			wantSteps:  28,
			wantCFG:    7.0,
			wantSeed:   100,
			wantErr:    nil,
		},
		{
			name:       "response with special characters in prompt",
			response:   "Done!\n---\n{\"prompt\": \"a cat with \\\"quotes\\\" and \\n newlines\", \"ready\": true, \"steps\": 25, \"cfg\": 8.5, \"seed\": 777}",
			wantText:   "Done!",
			wantPrompt: "a cat with \"quotes\" and \n newlines",
			wantReady:  true,
			wantSteps:  25,
			wantCFG:    8.5,
			wantSeed:   777,
			wantErr:    nil,
		},
		{
			name:       "response with unicode in conversational text",
			response:   "Perfect! 🎨\n---\n{\"prompt\": \"a cat in Tokyo\", \"ready\": true, \"steps\": 30, \"cfg\": 7.5, \"seed\": 888}",
			wantText:   "Perfect! 🎨",
			wantPrompt: "a cat in Tokyo",
			wantReady:  true,
			wantSteps:  30,
			wantCFG:    7.5,
			wantSeed:   888,
			wantErr:    nil,
		},
		{
			name:       "compact format no whitespace",
			response:   "Done\n---\n{\"prompt\":\"a cat\",\"ready\":true,\"steps\":20,\"cfg\":7.5,\"seed\":42}",
			wantText:   "Done",
			wantPrompt: "a cat",
			wantReady:  true,
			wantSteps:  20,
			wantCFG:    7.5,
			wantSeed:   42,
			wantErr:    nil,
		},
		{
			name:       "formatted JSON with indentation",
			response:   "Done\n---\n{\n  \"prompt\": \"a cat\",\n  \"ready\": true,\n  \"steps\": 28,\n  \"cfg\": 7.0,\n  \"seed\": 12345\n}",
			wantText:   "Done",
			wantPrompt: "a cat",
			wantReady:  true,
			wantSteps:  28,
			wantCFG:    7.0,
			wantSeed:   12345,
			wantErr:    nil,
		},
		{
			name:       "multiple delimiters on own lines uses last",
			response:   "Here's a separator:\n---\nLet me continue.\n---\n{\"prompt\": \"a cat\", \"ready\": true, \"steps\": 22, \"cfg\": 6.5, \"seed\": 9999}",
			wantText:   "Here's a separator:\n---\nLet me continue.",
			wantPrompt: "a cat",
			wantReady:  true,
			wantSteps:  22,
			wantCFG:    6.5,
			wantSeed:   9999,
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
			if gotMetadata.Steps != tt.wantSteps {
				t.Errorf("parseResponse() metadata.Steps = %d, want %d", gotMetadata.Steps, tt.wantSteps)
			}
			if gotMetadata.CFG != tt.wantCFG {
				t.Errorf("parseResponse() metadata.CFG = %f, want %f", gotMetadata.CFG, tt.wantCFG)
			}
			if gotMetadata.Seed != tt.wantSeed {
				t.Errorf("parseResponse() metadata.Seed = %d, want %d", gotMetadata.Seed, tt.wantSeed)
			}
		})
	}
}
