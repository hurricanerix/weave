package ollama

import (
	"strings"
)

// PromptPrefix is the marker that precedes image generation prompts in agent responses.
const PromptPrefix = "Prompt:"

// ExtractPrompt parses an assistant response to find the image generation prompt.
// The agent outputs prompts with "Prompt:" marker followed by the prompt content.
//
// Supported formats:
//   - "Prompt: a cat wearing a hat" (content on same line)
//   - "Prompt:\na cat wearing a hat" (content on next line(s))
//   - "Prompt Updated: ..." (LLM variation)
//   - "Prompt Revised: ..." (LLM variation)
//
// For multi-line prompts, content continues until a blank line or end of response.
// If multiple prompts exist, returns the last one (agent may revise).
// If no prompt is found, returns empty string.
func ExtractPrompt(response string) string {
	lines := strings.Split(response, "\n")
	var lastPrompt string

	for i := 0; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		lower := strings.ToLower(trimmed)

		// Check if line starts with "prompt" followed by optional words then ":"
		// This handles variations like "Prompt:", "Prompt Updated:", "Prompt Revised:"
		if !strings.HasPrefix(lower, "prompt") {
			continue
		}

		// Find the colon after "prompt"
		colonIdx := strings.Index(trimmed, ":")
		if colonIdx == -1 {
			continue
		}

		// Verify the text between "prompt" and ":" is reasonable (only letters/spaces)
		// This prevents matching things like "Prompt123:" or "Prompt-foo:"
		between := strings.TrimSpace(trimmed[len("prompt"):colonIdx])
		if !isValidPromptModifier(between) {
			continue
		}

		// Extract content after the colon
		content := strings.TrimSpace(trimmed[colonIdx+1:])

		// Strip surrounding quotes if present (LLM sometimes wraps in quotes)
		content = stripQuotes(content)

		if content != "" {
			// Content is on the same line
			lastPrompt = content
		} else {
			// Content is on subsequent lines - collect until blank line
			var promptLines []string
			for j := i + 1; j < len(lines); j++ {
				line := strings.TrimSpace(lines[j])
				if line == "" {
					break // Stop at blank line
				}
				// Clean up list markers like "- " at start
				if strings.HasPrefix(line, "- ") {
					line = strings.TrimPrefix(line, "- ")
				}
				promptLines = append(promptLines, line)
			}
			if len(promptLines) > 0 {
				lastPrompt = strings.Join(promptLines, " ")
			}
		}
	}

	return lastPrompt
}

// isValidPromptModifier checks if the text between "Prompt" and ":" is valid.
// Valid modifiers are empty or contain only letters and spaces (e.g., "Updated", "Revised").
func isValidPromptModifier(s string) bool {
	for _, r := range s {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == ' ') {
			return false
		}
	}
	return true
}

// stripQuotes removes surrounding double quotes from a string if present.
func stripQuotes(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}
