package ollama

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Parsing errors returned by parseResponse.
var (
	// ErrMissingFields indicates the JSON is valid but missing required fields.
	// All required fields (prompt, steps, cfg, seed) must be present in the metadata.
	ErrMissingFields = errors.New("JSON missing required fields")
)

// parseResponse parses a complete LLM response into conversational text and metadata.
//
// This function uses function calling to extract structured generation parameters.
// The LLM calls the update_generation function with these parameters:
//   - prompt (string): Image generation prompt
//   - steps (integer): Inference steps (1-100)
//   - cfg (number): Classifier-free guidance scale (0-20)
//   - seed (integer): Random seed (-1 for random, 0+ for deterministic)
//   - generate_image (boolean): Whether to trigger automatic generation
//
// The response contains conversational text and optionally a __TOOL_CALLS__ marker
// with JSON tool call data. This function extracts both the conversational text
// (for display in chat UI) and the structured metadata (for generation).
//
// Returns:
//   - Conversational text (displayed to user)
//   - LLMMetadata with generation parameters (zero values if no tool call)
//   - hasToolCall indicates if the LLM called a function
//   - Error only if tool calls are malformed (not if missing)
func parseResponse(response string) (string, LLMMetadata, bool, error) {
	// Extract tool calls from response
	conversationalText, toolCalls, hasToolCalls := extractToolCallsFromResponse(response)
	if !hasToolCalls {
		// No tool calls - this is valid for pure conversational responses
		// Return the full response as conversational text with empty metadata
		return response, LLMMetadata{}, false, nil
	}

	// Parse tool calls to extract metadata
	metadata, err := parseToolCalls(toolCalls)
	if err != nil {
		return "", LLMMetadata{}, false, fmt.Errorf("failed to parse tool calls: %w", err)
	}

	return conversationalText, metadata, true, nil
}

// parseToolCalls extracts LLMMetadata from a tool call response.
// This function parses the tool call arguments to extract generation parameters
// using the model's native function calling capability.
//
// WHY SEPARATE FUNCTION:
// This isolates tool call parsing logic, making it easier to test and maintain.
//
// TOOL CALL FORMAT:
// The LLM calls the "update_generation" function with these arguments:
//
//	{
//	  "prompt": "...",
//	  "steps": N,
//	  "cfg": X.X,
//	  "seed": N,
//	  "generate_image": true/false
//	}
//
// This matches the LLMMetadata struct, so we can unmarshal directly.
func parseToolCalls(toolCalls []ToolCall) (LLMMetadata, error) {
	if len(toolCalls) == 0 {
		return LLMMetadata{}, errors.New("no tool calls found")
	}

	// Look for the update_generation function call.
	// WHY SEARCH: The LLM might call multiple functions in the future.
	// We specifically need the update_generation call for image generation.
	var updateGenCall *ToolCall
	for i := range toolCalls {
		if toolCalls[i].Function.Name == "update_generation" {
			updateGenCall = &toolCalls[i]
			break
		}
	}

	if updateGenCall == nil {
		return LLMMetadata{}, errors.New("update_generation tool call not found")
	}

	// Parse the function arguments into LLMMetadata.
	// WHY TWO-STEP UNMARSHAL: Ollama sends Arguments as a JSON-encoded string.
	// For example: Arguments = `"{\"prompt\":\"a cat\",\"steps\":28,...}"` (a string).
	// We must first decode it as a string, then parse that string as JSON.
	//
	// WHY LENIENT PARSING: LLMs don't always respect JSON schema types.
	// They may return "4.3" (string) instead of 4.3 (number), or "true" (string)
	// instead of true (bool). We use rawLLMMetadata to accept any type, then
	// convert to proper types using parseRawMetadata().
	//
	// Step 1: Decode the JSON string to get the actual JSON content
	var argsJSON string
	if err := json.Unmarshal(updateGenCall.Function.Arguments, &argsJSON); err != nil {
		// If unmarshaling as string fails, try unmarshaling directly as object
		// (in case ollama's API changes or for testing with direct JSON objects)
		var rawMeta rawLLMMetadata
		if err2 := json.Unmarshal(updateGenCall.Function.Arguments, &rawMeta); err2 != nil {
			return LLMMetadata{}, fmt.Errorf("failed to parse tool call arguments: %w", err2)
		}
		// Validate required fields are present
		if rawMeta.Prompt == nil || rawMeta.Steps == nil || rawMeta.CFG == nil || rawMeta.Seed == nil {
			return LLMMetadata{}, ErrMissingFields
		}
		// Convert raw metadata to proper types
		return parseRawMetadata(rawMeta)
	}

	// Step 2: Parse the JSON string using lenient rawLLMMetadata
	var rawMeta rawLLMMetadata
	if err := json.Unmarshal([]byte(argsJSON), &rawMeta); err != nil {
		return LLMMetadata{}, fmt.Errorf("failed to parse tool call arguments JSON: %w", err)
	}

	// Validate that required fields are present.
	// WHY VALIDATE: Even with function calling, the LLM might omit required fields.
	// We must check to ensure we have complete metadata.
	if rawMeta.Prompt == nil || rawMeta.Steps == nil || rawMeta.CFG == nil || rawMeta.Seed == nil {
		return LLMMetadata{}, ErrMissingFields
	}

	// Convert raw metadata to proper types
	return parseRawMetadata(rawMeta)
}

// rawLLMMetadata is a lenient intermediate struct for parsing LLM responses.
// LLMs don't always respect JSON schema types - they may return "true" instead
// of true, or "4.3" instead of 4.3. This struct accepts any JSON value type
// and then converts to proper types using parseRawMetadata().
type rawLLMMetadata struct {
	Prompt        interface{} `json:"prompt"`
	GenerateImage interface{} `json:"generate_image"`
	Steps         interface{} `json:"steps"`
	CFG           interface{} `json:"cfg"`
	Seed          interface{} `json:"seed"`
}

// parseRawMetadata converts a rawLLMMetadata with potentially stringified values
// into a proper LLMMetadata with correct types.
func parseRawMetadata(raw rawLLMMetadata) (LLMMetadata, error) {
	var metadata LLMMetadata

	// Prompt is always a string
	if raw.Prompt != nil {
		metadata.Prompt = fmt.Sprintf("%v", raw.Prompt)
	}

	// GenerateImage: accept bool or string "true"/"false"
	if raw.GenerateImage != nil {
		switch v := raw.GenerateImage.(type) {
		case bool:
			metadata.GenerateImage = v
		case string:
			b, err := strconv.ParseBool(v)
			if err != nil {
				return LLMMetadata{}, fmt.Errorf("invalid generate_image value: %q", v)
			}
			metadata.GenerateImage = b
		default:
			return LLMMetadata{}, fmt.Errorf("invalid generate_image type: %T", v)
		}
	}

	// Steps: accept int or string
	if raw.Steps != nil {
		switch v := raw.Steps.(type) {
		case float64:
			metadata.Steps = int(v)
		case string:
			i, err := strconv.Atoi(v)
			if err != nil {
				return LLMMetadata{}, fmt.Errorf("invalid steps value: %q", v)
			}
			metadata.Steps = i
		default:
			return LLMMetadata{}, fmt.Errorf("invalid steps type: %T", v)
		}
	}

	// CFG: accept float64 or string
	if raw.CFG != nil {
		switch v := raw.CFG.(type) {
		case float64:
			metadata.CFG = v
		case string:
			f, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return LLMMetadata{}, fmt.Errorf("invalid cfg value: %q", v)
			}
			metadata.CFG = f
		default:
			return LLMMetadata{}, fmt.Errorf("invalid cfg type: %T", v)
		}
	}

	// Seed: accept int64 or string
	if raw.Seed != nil {
		switch v := raw.Seed.(type) {
		case float64:
			metadata.Seed = int64(v)
		case string:
			i, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				return LLMMetadata{}, fmt.Errorf("invalid seed value: %q", v)
			}
			metadata.Seed = i
		default:
			return LLMMetadata{}, fmt.Errorf("invalid seed type: %T", v)
		}
	}

	return metadata, nil
}

// extractToolCallsFromResponse checks if the response contains the __TOOL_CALLS__ marker
// and extracts tool calls if present. This is used to separate conversational text
// from structured function call data.
//
// WHY THIS FUNCTION:
// parseStreamingResponse() appends tool calls with a marker to the full response.
// This function detects that marker and extracts the tool calls, separating
// the conversational text (for display) from the function call data (for metadata).
//
// Returns the conversational text, tool calls (if any), and whether tool calls were found.
func extractToolCallsFromResponse(response string) (conversationalText string, toolCalls []ToolCall, hasToolCalls bool) {
	// Check for tool call marker
	toolCallMarker := "\n__TOOL_CALLS__\n"
	markerIndex := strings.Index(response, toolCallMarker)
	if markerIndex == -1 {
		// No tool calls - return full response as conversational text
		return response, nil, false
	}

	// Split response: everything before marker is conversational text,
	// everything after marker is tool call JSON
	conversationalText = response[:markerIndex]
	toolCallJSON := response[markerIndex+len(toolCallMarker):]

	// Parse tool calls from JSON
	var calls []ToolCall
	if err := json.Unmarshal([]byte(toolCallJSON), &calls); err != nil {
		// Failed to parse tool calls - treat entire response as conversational text
		return response, nil, false
	}

	return conversationalText, calls, true
}
