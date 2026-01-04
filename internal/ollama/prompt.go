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
//
// For multi-line prompts, content continues until a blank line or end of response.
// If multiple prompts exist, returns the last one (agent may revise).
// If no prompt is found, returns empty string.
func ExtractPrompt(response string) string {
	lines := strings.Split(response, "\n")
	var lastPrompt string

	for i := 0; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])

		// Check for "Prompt:" prefix (case-insensitive for robustness)
		if strings.HasPrefix(strings.ToLower(trimmed), "prompt:") {
			// Extract content after "Prompt:" on same line
			content := strings.TrimSpace(trimmed[len("Prompt:"):])

			if content != "" {
				// Content is on the same line as "Prompt:"
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
	}

	return lastPrompt
}
