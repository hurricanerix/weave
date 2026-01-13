package ollama

import (
	"encoding/json"
	"errors"
	"strings"
)

// Parsing errors returned by parseResponse.
var (
	// ErrMissingDelimiter indicates the response does not contain the ResponseDelimiter.
	// This means the LLM did not follow the required format of ending with "---\n{JSON}".
	ErrMissingDelimiter = errors.New("response missing delimiter")

	// ErrInvalidJSON indicates the JSON portion after the delimiter could not be parsed.
	// This means the LLM included the delimiter but the JSON is malformed.
	ErrInvalidJSON = errors.New("invalid JSON after delimiter")

	// ErrMissingFields indicates the JSON is valid but missing required fields.
	// All required fields (prompt, steps, cfg, seed) must be present in the metadata.
	ErrMissingFields = errors.New("JSON missing required fields")
)

// parseResponse parses a complete LLM response into conversational text and metadata.
//
// The LLM is required to format responses as:
//
//	[conversational text]
//	---
//	{"prompt": "...", "steps": N, "cfg": X.X, "seed": N}
//
// WHY THIS FORMAT:
// This structured format allows us to:
// 1. Display conversational text in the chat UI (user sees friendly dialog)
// 2. Extract structured metadata (prompt, ready flag, generation settings) for automation
// 3. Detect format errors reliably (missing delimiter or invalid JSON)
//
// WHY THREE ERROR TYPES:
//
// ErrMissingDelimiter - The LLM didn't include "---" in its response.
// This usually means the LLM forgot the format or didn't understand the
// system prompt. Recoverable via Level 1 retry (format reminder).
//
// ErrInvalidJSON - The LLM included "---" but the JSON is malformed.
// This means the LLM attempted to follow the format but made a syntax error
// (missing quote, invalid escape, etc.). Recoverable via Level 1 retry.
//
// ErrMissingFields - The JSON is valid but missing required fields.
// This means the LLM generated syntactically correct JSON but didn't include
// all required fields (prompt, steps, cfg, seed). This violates the schema.
// Recoverable via retry.
//
// WHY SEARCH FROM START:
// We search for the delimiter from the start of the response because some
// LLMs generate multiple conversation turns with multiple "---" delimiters.
// The FIRST occurrence gives us the agent's actual response, not hallucinated
// continuations. If conversational text contains "---" before the delimiter,
// the LLM should escape it or use a different format.
//
// WHY VALIDATE FIELD PRESENCE:
// Go's json.Unmarshal sets missing fields to zero values (empty string, 0).
// We need to distinguish between:
// - {"prompt": "", "steps": 0, "cfg": 0.0, "seed": 0} - Valid (explicit zero values)
// - {} - Invalid (missing required fields)
//
// Without the map check, both would unmarshal successfully but have different
// semantics. The map check ensures the LLM explicitly provided all four fields.
func parseResponse(response string) (string, LLMMetadata, error) {
	// Find the delimiter that separates conversational text from JSON.
	// WHY SPLIT BY NEWLINES: The delimiter must be on its own line to avoid
	// false matches. For example, "What do you think of this---no really?" should
	// not be detected as a delimiter. Only "---" as a standalone line counts.
	lines := strings.Split(response, "\n")
	delimiterLineIndex := -1

	// Find the FIRST line that is exactly the delimiter (with whitespace trimmed).
	// WHY SEARCH FROM START: The LLM sometimes generates multiple conversation turns
	// in a single response, with multiple "---" delimiters. For example:
	//
	//   "What style of cat?
	//    ---
	//    {"prompt": "", ...}
	//
	//    Given "tabby"
	//    Perfect!
	//    ---
	//    {"prompt": "tabby cat", ...}"
	//
	// The system prompt explicitly forbids this ("Generate ONLY your response"),
	// but some models do it anyway. Using the FIRST delimiter ensures we get
	// the agent's actual response, not the hallucinated continuation.
	for i := 0; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == ResponseDelimiter {
			delimiterLineIndex = i
			break
		}
	}

	if delimiterLineIndex == -1 {
		// No delimiter found - LLM didn't follow format
		// WHY RETURN ERROR: Without a delimiter, we can't reliably separate
		// conversational text from JSON. The entire response might be text,
		// or it might be malformed JSON, or both mixed together.
		return "", LLMMetadata{}, ErrMissingDelimiter
	}

	// Split into conversational text (before delimiter) and JSON (after delimiter)
	conversationalText := strings.TrimSpace(strings.Join(lines[:delimiterLineIndex], "\n"))
	jsonPortion := strings.TrimSpace(strings.Join(lines[delimiterLineIndex+1:], "\n"))

	// Unmarshal the JSON into metadata struct
	// WHY CHECK UNMARSHAL ERROR: The LLM might have included the delimiter but
	// written invalid JSON syntax: missing quotes, trailing commas, etc.
	var metadata LLMMetadata
	if err := json.Unmarshal([]byte(jsonPortion), &metadata); err != nil {
		return "", LLMMetadata{}, ErrInvalidJSON
	}

	// Validate that the JSON contains the required fields.
	// WHY DOUBLE UNMARSHAL: Go's json.Unmarshal sets missing fields to zero values.
	// We need to distinguish between:
	// - {"prompt": "", "ready": false, "steps": 0, "cfg": 0.0, "seed": 0} - Valid (LLM explicitly provided fields)
	// - {"other": "data"} - Invalid (missing required fields, but unmarshals to zero values)
	//
	// The map check tells us which fields were actually present in the JSON,
	// not just which fields ended up with zero values after unmarshaling.
	var rawMap map[string]interface{}
	if err := json.Unmarshal([]byte(jsonPortion), &rawMap); err != nil {
		// This should not happen since we already unmarshaled successfully above,
		// but handle it defensively
		return "", LLMMetadata{}, ErrInvalidJSON
	}

	// Check that all required fields are present in the JSON
	// WHY REQUIRE ALL FIELDS: The "prompt" field contains the generation prompt
	// (empty if still asking questions). The "steps", "cfg", and "seed" fields specify
	// generation settings. All four fields are required for the system to function.
	// Missing any field means the LLM didn't follow the schema.
	_, hasPrompt := rawMap["prompt"]
	_, hasSteps := rawMap["steps"]
	_, hasCFG := rawMap["cfg"]
	_, hasSeed := rawMap["seed"]
	if !hasPrompt || !hasSteps || !hasCFG || !hasSeed {
		return "", LLMMetadata{}, ErrMissingFields
	}

	return conversationalText, metadata, nil
}
