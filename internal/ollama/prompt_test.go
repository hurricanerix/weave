package ollama

import "testing"

func TestExtractPrompt(t *testing.T) {
	tests := []struct {
		name     string
		response string
		want     string
	}{
		{
			name:     "single prompt line",
			response: "Here's your prompt:\n\nPrompt: a cat wearing a wizard hat",
			want:     "a cat wearing a wizard hat",
		},
		{
			name:     "multiple prompt lines uses last",
			response: "First attempt:\nPrompt: a simple cat\n\nActually, let me revise:\nPrompt: a tabby cat wearing a blue wizard hat",
			want:     "a tabby cat wearing a blue wizard hat",
		},
		{
			name:     "no prompt line returns empty",
			response: "I need more information. What style would you like?",
			want:     "",
		},
		{
			name:     "empty response returns empty",
			response: "",
			want:     "",
		},
		{
			name:     "whitespace only response returns empty",
			response: "   \n\n   \t\n",
			want:     "",
		},
		{
			name:     "prompt with leading whitespace on line",
			response: "  Prompt: a cat in space",
			want:     "a cat in space",
		},
		{
			name:     "prompt with trailing whitespace",
			response: "Prompt: a cat in space   ",
			want:     "a cat in space",
		},
		{
			name:     "prompt with whitespace around content",
			response: "Prompt:   a cat in space   ",
			want:     "a cat in space",
		},
		{
			name:     "prompt line in middle of response",
			response: "Great! Based on your preferences:\n\nPrompt: a realistic photo of a tabby cat\n\nLet me know if you'd like changes.",
			want:     "a realistic photo of a tabby cat",
		},
		{
			name:     "prompt prefix as part of word ignored",
			response: "The Prompted response was helpful.\nPrompt: actual prompt here",
			want:     "actual prompt here",
		},
		{
			name:     "prompt with special characters",
			response: "Prompt: a cat with a $100 hat & fancy monocle",
			want:     "a cat with a $100 hat & fancy monocle",
		},
		{
			name:     "prompt with unicode characters",
			response: "Prompt: a cat wearing a hat in Tokyo",
			want:     "a cat wearing a hat in Tokyo",
		},
		{
			name:     "prompt case sensitivity",
			response: "PROMPT: should not match\nprompt: should not match\nPrompt: correct match",
			want:     "correct match",
		},
		{
			name:     "prompt followed by colon in content",
			response: "Prompt: Subject: a cat, Style: realistic",
			want:     "Subject: a cat, Style: realistic",
		},
		{
			name:     "windows line endings",
			response: "First line\r\nPrompt: a cat\r\nLast line",
			want:     "a cat",
		},
		{
			name:     "multi-line prompt with content on next line",
			response: "Here's your prompt:\n\nPrompt:\na cat wearing a hat",
			want:     "a cat wearing a hat",
		},
		{
			name:     "multi-line prompt with multiple lines",
			response: "Prompt:\na realistic photograph\nof a happy cat\nwearing a bowtie\n\nLet me know if you want changes.",
			want:     "a realistic photograph of a happy cat wearing a bowtie",
		},
		{
			name:     "multi-line prompt with list markers",
			response: "Prompt:\n- A realistic photograph featuring a happy cat\n- wearing a colorful bowtie\n\nDone!",
			want:     "A realistic photograph featuring a happy cat wearing a colorful bowtie",
		},
		{
			name:     "prompt with only colon no space",
			response: "Here's the prompt:\nPrompt:a cat in space",
			want:     "a cat in space",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractPrompt(tt.response)
			if got != tt.want {
				t.Errorf("ExtractPrompt() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPromptPrefix(t *testing.T) {
	// Verify the constant is correct
	if PromptPrefix != "Prompt:" {
		t.Errorf("PromptPrefix = %q, want %q", PromptPrefix, "Prompt:")
	}
}
