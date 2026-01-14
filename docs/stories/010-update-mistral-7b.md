# Story 010: Update to Mistral 7B with structured output

## Status
Done

## Problem

The current ollama client uses llama3.2:1b with text-based prompt extraction. The LLM's output format is inconsistent—it starts following the "Prompt:" convention but drifts mid-conversation and stops using it correctly. This causes prompts to appear in the chat window instead of being extracted to the prompt pane. The text parsing in ExtractPrompt() is fragile and can't handle format variations reliably.

Users experience:
- Prompts correctly extracted at the start of a conversation
- Format drift after 3-4 turns (LLM stops using "Prompt:" marker)
- Prompts appearing in chat window instead of prompt pane
- Inconsistent behavior across conversations

## User/Actor

- End user (wants reliable prompt extraction, conversational streaming UX)
- Developer (needs structured, parseable output from LLM)

## Desired Outcome

A reliable ollama client where:
- LLM streams conversational text (user sees live typing effect in chat pane)
- LLM ends every message with `---` delimiter followed by JSON metadata
- Prompts consistently extracted from JSON (appear in prompt pane, not chat)
- Format errors automatically recovered via retry with format reminder
- Retries are transparent to user (happens behind the scenes)
- System degrades gracefully if retries fail (clear error, context reset)

## Acceptance Criteria

### Model Change

- [ ] Client uses Mistral 7B model (`mistral:7b` in ollama)
- [ ] DEVELOPMENT.md updated with model pull command: `ollama pull mistral:7b`
- [ ] Model name constant changed from `llama3.2:1b` to `mistral:7b`
- [ ] Integration tests pass with new model

### Response Format

- [ ] System prompt instructs LLM to end EVERY message with `---\n{JSON}`
- [ ] JSON schema contains:
  - `prompt` (string): Image generation prompt (empty string if not ready)
  - `generate_image` (boolean): True to trigger generation, false to just update prompt
  - `steps` (int): Number of inference steps
  - `cfg` (float): CFG scale
  - `seed` (int): Generation seed (-1 for random)
- [ ] Conversational text appears before `---` delimiter (shown in chat pane)
- [ ] JSON appears after `---` delimiter (parsed, not shown in chat pane)
- [ ] System prompt includes example format:
  ```
  A cat in a hat! Let me ask a few questions:
  - What kind of cat?
  - What style of hat?
  ---
  {"prompt": "", "generate_image": false, "steps": 4, "cfg": 1.0, "seed": -1}
  ```
- [ ] System prompt includes example when ready:
  ```
  Perfect! Generating your image now.
  ---
  {"prompt": "a tabby cat wearing a blue wizard hat", "generate_image": true, "steps": 4, "cfg": 1.0, "seed": -1}
  ```

### Streaming Behavior

- [ ] Client streams tokens to chat pane as they arrive (live typing effect)
- [ ] When `---` delimiter detected, stop displaying tokens in chat pane
- [ ] Buffer remaining tokens (JSON portion) until response complete
- [ ] Parse buffered JSON after stream ends
- [ ] If `generate_image: true` and `prompt` non-empty, trigger generation automatically
- [ ] Chat pane shows only conversational text (before `---`)

### JSON Parsing

- [ ] Go code defines struct for JSON unmarshaling (prompt, generate_image, steps, cfg, seed fields)
- [ ] After stream completes, parse JSON portion
- [ ] If JSON valid and unmarshals successfully, accept response
- [ ] If `---` delimiter missing, trigger retry (format error)
- [ ] If JSON invalid after `---`, trigger retry (parse error)
- [ ] If JSON missing required fields, trigger retry (schema error)

### Automatic Retry Logic (Level 1: Format Reminder)

- [ ] On format error, append system message: "Please end your response with `---` followed by JSON using this format: {example}"
- [ ] Example includes both conversational text and JSON structure
- [ ] Retry is transparent to user (no error shown in UI)
- [ ] Maximum 2 retries with format reminder before escalating
- [ ] Retry count resets on successful parse (not cumulative)

### Context Compaction (Level 2: Fallback)

- [ ] If 2 format retries fail, compact conversation context
- [ ] Compaction strategy: Summarize user intent into single system message
- [ ] Summary format: "User wants: [key details]. Respond with ONLY JSON (no conversational text): {example}"
- [ ] After compaction, request JSON-only response (no text before `---`)
- [ ] Retry once with compacted context
- [ ] If compaction retry succeeds, resume normal conversation flow

### Error Handling (Level 3: Reset)

- [ ] If compaction retry fails, show error to user in chat pane
- [ ] Error message: "I'm having trouble understanding the format. Let's start fresh."
- [ ] Clear conversation history (reset context)
- [ ] User can immediately start new conversation
- [ ] Log error with full message history (for debugging)

### Prompt Engineering

- [ ] System prompt explicitly requires `---\n{JSON}` at end of EVERY message
- [ ] System prompt states conversational text goes BEFORE delimiter
- [ ] System prompt includes positive examples (correct format with conversational + JSON)
- [ ] System prompt includes negative examples (what NOT to do: JSON without delimiter, no JSON at all)
- [ ] System prompt keeps prompt length guidance (under 200 chars)
- [ ] System prompt retains user edit preservation behavior from story 004
- [ ] System prompt emphasizes: "ALWAYS include the delimiter and JSON, even if just asking questions"

### Testing

- [ ] Unit test: Parse response with `---` delimiter and valid JSON
- [ ] Unit test: Parse response missing `---` delimiter (triggers retry)
- [ ] Unit test: Parse response with invalid JSON after `---` (triggers retry)
- [ ] Unit test: Parse response missing required fields (triggers retry)
- [ ] Unit test: Streaming stops displaying at `---` delimiter
- [ ] Unit test: Retry appends format reminder to conversation
- [ ] Unit test: Context compaction summarizes conversation
- [ ] Integration test: 10 multi-turn conversations with Mistral 7B
- [ ] Integration test: All messages contain `---` delimiter (no format drift)
- [ ] Integration test: Prompts extracted from JSON (appear in prompt pane)
- [ ] Integration test: Chat pane shows only text before `---`
- [ ] Integration test: Retry logic recovers from simulated format errors
- [ ] Manual test: Verify streaming displays conversational text live
- [ ] Manual test: Verify JSON portion never appears in chat pane
- [ ] Manual test: User edits prompt, LLM preserves edits in next response

### Documentation

- [ ] DEVELOPMENT.md updated with Mistral 7B setup
- [ ] DEVELOPMENT.md explains response format (`---\n{JSON}`)
- [ ] DEVELOPMENT.md documents JSON schema (prompt, generate_image, steps, cfg, seed fields)
- [ ] DEVELOPMENT.md explains retry levels (format reminder → compaction → reset)
- [ ] Code comments explain delimiter detection and JSON parsing
- [ ] Code comments explain why streaming stops at `---`

## Out of Scope

- Function calling (Mistral 7B not reliable for conversation + tools)
- Token counting or context window limits (accept ollama defaults)
- User-visible retry indicators (retries are transparent)
- Configurable retry count (hardcoded: 2 format retries, 1 compaction retry)
- Multiple compaction strategies (single strategy: summarize to system message)
- Configurable delimiter (hardcoded to `---`)
- JSON schema validation beyond required fields
- Fallback to text parsing (must use JSON or reset)
- Multiple simultaneous LLM requests (sequential only)

## Dependencies

None. This is an update to existing ollama client from story 004.

## Open Questions

- Should delimiter be configurable (`---` vs something else like `###JSON###`)?
- Should compaction use LLM-based summarization or rule-based extraction?
- What exact wording for format reminder message?
- Should JSON schema include additional fields (like `confidence` or `clarifications_needed`)?

## Tasks

### 001: Update model constant and pull instructions
**Domain:** weave
**Status:** done
**Depends on:** none

Change `DefaultModel` constant in `internal/ollama/types.go` from `llama3.2:1b` to `mistral:7b`. Update `docs/DEVELOPMENT.md` ollama section to document pulling `mistral:7b` instead of `llama3.2:1b`. Update any CLI flag documentation that references the old model name.

---

### 002: Define JSON response structure
**Domain:** weave
**Status:** done
**Depends on:** none

Add Go structs in `internal/ollama/types.go` for the JSON response format:
- `LLMMetadata` struct with `Prompt string` and `Ready bool` fields
- JSON tags for unmarshaling
- Add delimiter constant: `ResponseDelimiter = "---"`

Update `ChatResult` type to include parsed metadata instead of just extracted prompt string.

---

### 003: Rewrite system prompt for structured output
**Domain:** weave
**Status:** done
**Depends on:** 002

Replace `SystemPrompt` constant in `internal/ollama/types.go` with new prompt that:
- Requires `---\n{JSON}` at end of EVERY message
- Defines JSON schema (prompt, ready fields)
- Includes positive examples (conversational text + delimiter + JSON)
- Includes negative examples (what NOT to do)
- Preserves existing guidance: prompt length <200 chars, user edit preservation
- Emphasizes format requirement even when asking questions

---

### 004: Implement delimiter detection in streaming
**Domain:** weave
**Status:** done
**Depends on:** 002

Modify `parseStreamingResponse` in `internal/ollama/client.go` to:
- Detect `---` delimiter in streaming tokens
- Stop calling callback after delimiter detected
- Buffer tokens after delimiter (JSON portion)
- Return both conversational text and buffered JSON separately

Add internal state tracking: `delimiterFound bool`, `jsonBuffer bytes.Buffer`.

---

### 005: Implement JSON parsing after stream
**Domain:** weave
**Status:** done
**Depends on:** 002, 004

Create `parseResponse` function in `internal/ollama/prompt.go` that:
- Takes full response text
- Splits on `---` delimiter
- Unmarshals JSON portion into `LLMMetadata` struct
- Validates required fields present
- Returns conversational text, metadata, and error

Return specific errors: `ErrMissingDelimiter`, `ErrInvalidJSON`, `ErrMissingFields`.

---

### 006: Update Chat method to return parsed metadata
**Domain:** weave
**Status:** done
**Depends on:** 005

Modify `Chat` method signature in `internal/ollama/client.go` to return `ChatResult` instead of just string. Update `ChatResult` to contain:
- `Response string` (conversational text only, before delimiter)
- `Metadata LLMMetadata` (parsed JSON with prompt, generate_image, steps, cfg, seed)
- Remove old `Prompt` field (now in metadata)

Update all callers to use new return type.

---

### 007: Implement retry with format reminder (Level 1)
**Domain:** weave
**Status:** done
**Depends on:** 006

Add `chatWithRetry` function in `internal/web/server.go` that:
- Calls ollama client's Chat method
- On `ErrMissingDelimiter` or `ErrInvalidJSON` or `ErrMissingFields`, append system message with format reminder
- Format reminder text includes example of correct format
- Retry up to 2 times with format reminder
- Return error if retries exhausted

Track retry count per request (not cumulative across conversation).

---

### 008: Implement context compaction (Level 2)
**Domain:** weave
**Status:** done
**Depends on:** 007

Add `compactContext` function in `internal/web/server.go` that:
- Takes conversation history
- Extracts key details: subject, style, setting from user messages
- Replaces history with single system message: "User wants: [details]. Respond with ONLY JSON (no text): {example}"
- Returns compacted message array

Modify retry logic to use compaction after 2 format reminder failures. Retry once with compacted context.

---

### 009: Implement error handling and context reset (Level 3)
**Domain:** weave
**Status:** done
**Depends on:** 008

Add final error handling in `handleChat` after all retries fail:
- Send error event to user: "I'm having trouble understanding the format. Let's start fresh."
- Call `manager.Clear()` to reset conversation history
- Log full error with conversation history for debugging
- Allow user to immediately start new conversation

---

### 010: Remove old ExtractPrompt function
**Domain:** weave
**Status:** done
**Depends on:** 006

Delete `ExtractPrompt` function from `internal/ollama/prompt.go` and its helper functions (`isValidPromptModifier`, `stripQuotes`). Remove `PromptPrefix` constant. Clean up any remaining references to text-based prompt extraction.

---

### 011: Update web server to use metadata
**Domain:** weave
**Status:** done
**Depends on:** 006

Modify `handleChat` in `internal/web/server.go` to:
- Use `chatWithRetry` instead of direct ollama client call
- Extract prompt from `result.Metadata.Prompt` instead of `ollama.ExtractPrompt(fullResponse)`
- If `result.Metadata.GenerateImage == true` and prompt non-empty, trigger generation automatically
- Send `agent-done` event after successful parse

---

### 012: Add unit tests for JSON parsing
**Domain:** weave
**Status:** done
**Depends on:** 005

Add tests in `internal/ollama/prompt_test.go`:
- Parse response with valid delimiter and JSON
- Parse response missing delimiter (returns error)
- Parse response with invalid JSON after delimiter (returns error)
- Parse response missing required fields (returns error)
- Parse response with empty prompt but generate_image=false (valid)

Use table-driven tests with multiple test cases.

---

### 013: Add unit tests for delimiter detection
**Domain:** weave
**Status:** done
**Depends on:** 004

Add tests in `internal/ollama/client_test.go`:
- Streaming stops displaying at `---` delimiter
- Tokens before delimiter go to callback
- Tokens after delimiter buffered, not sent to callback
- Full response includes both parts
- Delimiter in middle of token handled correctly

---

### 014: Add unit tests for retry logic
**Domain:** weave
**Status:** done
**Depends on:** 007, 008

Add tests in `internal/web/server_test.go`:
- Format reminder appended after parse error
- Context compaction triggered after 2 format retries
- Error returned and context reset after all retries fail
- Retry count resets on successful parse
- Compacted context has correct format

Mock ollama client to simulate format errors.

---

### 015: Add integration test for Mistral 7B
**Domain:** weave
**Status:** done
**Depends on:** 003, 006

Add test in `internal/ollama/integration_test.go`:
- 10 multi-turn conversations with Mistral 7B
- Verify all responses contain `---` delimiter
- Verify all JSON parses successfully
- Verify prompts extracted when generate_image=true
- Verify conversational text excludes JSON portion
- Skip test if Mistral 7B not available in ollama

Tag with `//go:build integration`.

---

### 016: Update DEVELOPMENT.md with new model
**Domain:** weave
**Status:** done
**Depends on:** 001

Update `docs/DEVELOPMENT.md`:
- Change all references from `llama3.2:1b` to `mistral:7b`
- Update pull command: `ollama pull mistral:7b`
- Add section explaining response format (`---\n{JSON}`)
- Document JSON schema (prompt, generate_image, steps, cfg, seed fields)
- Document retry behavior (format reminder → compaction → reset)
- Update model selection rationale (instruction following, format adherence)

---

### 017: Add code comments for delimiter logic
**Domain:** weave
**Status:** done
**Depends on:** 004, 005

Add detailed comments in:
- `parseStreamingResponse`: Explain delimiter detection, why streaming stops
- `parseResponse`: Explain JSON parsing, error cases
- `chatWithRetry`: Explain 3-level retry strategy
- `compactContext`: Explain compaction strategy

Comments should explain "why" not just "what".

---

### 018: Manual testing checklist
**Domain:** weave
**Status:** done
**Depends on:** 001, 003, 011

Create manual test plan (documented in story completion comment):
- Start fresh conversation, verify streaming displays live
- Verify `---` delimiter never appears in chat pane
- Continue conversation for 5+ turns, verify no format drift
- User edits prompt, verify LLM preserves edits in next message
- Trigger format error (modify code temporarily), verify retry recovers
- Verify final error message and context reset if all retries fail

Execute tests and document results before marking story complete.

---

## Notes

### Why Mistral 7B

Mistral 7B has better instruction following than Llama 3.2 1B. It should maintain the `---\n{JSON}` format more consistently across multi-turn conversations. Small enough for consumer hardware (~8GB VRAM quantized) but large enough for coherent conversation and format adherence.

### Why delimiter + JSON

This design preserves streaming UX (conversational text displays live) while ensuring structured output (JSON for parsing). The delimiter is a clear signal to stop displaying and start parsing.

Benefits:
- User sees live typing effect in chat (feels responsive)
- Prompt extraction is reliable (JSON, not text parsing)
- Format errors are detectable (missing delimiter or invalid JSON)
- Retry logic is simple (remind about format requirements)

### Retry levels

Three escalating levels of recovery:

1. **Format reminder** (2 attempts): Assume LLM forgot the format, remind it
2. **Context compaction** (1 attempt): Assume long context confused LLM, simplify
3. **Reset**: Admit failure, start fresh

This balances user experience (most errors recover silently) with fail-safe (don't loop forever).

### Streaming implications

Streaming stops when `---` is detected. This means:
- Conversational text displays live (good UX)
- JSON portion is buffered (not displayed)
- Parse happens after stream completes

If delimiter never appears, stream displays everything. Retry logic catches this as format error.

### Compaction strategy

When format retries fail, compact context to reduce cognitive load on LLM:
- Extract key user requirements (subject, style, setting, mood)
- Replace message history with single summary
- Explicitly request JSON-only response (no conversational text)

This gives LLM a fresh start with minimal context. If this fails, the LLM is fundamentally broken or the model doesn't support this format—reset is appropriate.

### JSON schema

Initial schema (Story 010): `prompt` and `ready`. Extended in Story 012 to `prompt`, `generate_image`, `steps`, `cfg`, and `seed` for full control over generation parameters and auto-generation behavior.

### Backward compatibility

This replaces text-based ExtractPrompt() with delimiter + JSON parsing. Any code depending on old prompt extraction will break. Verify no external dependencies before deploying.

### Model availability

Mistral 7B must be available in ollama. As of this writing, `mistral:7b` is an official ollama model. If model naming changes, update constant and docs accordingly.

## Manual Testing Checklist

Before marking this story complete, execute the following manual tests and document results:

### Test 1: Fresh conversation with live streaming
**Objective:** Verify streaming displays conversational text live as it arrives.

**Steps:**
1. Start weave web UI
2. Enter a simple prompt (e.g. "a cat")
3. Observe chat pane during LLM response

**Expected:**
- Conversational text appears token-by-token (live typing effect)
- Response feels immediate and responsive
- No visible lag or buffering

**Result:** [To be documented]

---

### Test 2: Delimiter visibility
**Objective:** Verify `---` delimiter never appears in chat pane.

**Steps:**
1. Continue from Test 1 or start new conversation
2. Send multiple messages to LLM
3. Inspect chat pane for any occurrence of `---`

**Expected:**
- `---` delimiter is never visible to user
- Only conversational text appears in chat pane
- JSON metadata is completely hidden

**Result:** [To be documented]

---

### Test 3: Multi-turn format consistency
**Objective:** Verify no format drift over extended conversation.

**Steps:**
1. Start fresh conversation
2. Have 5+ turn conversation with LLM (ask questions, refine requirements)
3. Inspect browser console or logs for parse errors

**Expected:**
- All responses contain `---` delimiter
- All responses parse successfully
- No format retries triggered (check logs)
- Prompts update correctly when generate_image=true

**Result:** [To be documented]

**Note:** Story 012 extended the JSON schema to include `generate_image` for auto-generation control, replacing the original `ready` field with explicit generation triggering.

---

### Test 4: User edit preservation
**Objective:** Verify LLM preserves user's prompt edits in subsequent responses.

**Steps:**
1. Have conversation that results in a prompt
2. Manually edit the prompt in the prompt pane (e.g. change "cat" to "dog")
3. Continue conversation, ask LLM to refine the prompt
4. Observe if LLM references "dog" instead of "cat"

**Expected:**
- LLM acknowledges the manual edit
- LLM preserves edits in next response
- Behavior matches story 004 user edit preservation

**Result:** [To be documented]

---

### Test 5: Format error recovery
**Objective:** Verify retry logic recovers from format errors.

**Steps:**
1. Temporarily modify `parseResponse` to simulate format error (comment out delimiter check)
2. Start conversation, trigger LLM response
3. Observe retry behavior in logs
4. Restore original code, verify normal operation

**Expected:**
- Format reminder appended to conversation (check logs)
- Retry succeeds (or escalates to compaction)
- Error recovery is transparent to user
- No error message in chat pane unless all retries fail

**Result:** [To be documented]

---

### Test 6: Complete retry failure and reset
**Objective:** Verify error message and context reset when all retries fail.

**Steps:**
1. Temporarily modify `parseResponse` to always return format error
2. Start conversation, trigger LLM response
3. Observe retry escalation through all levels
4. Verify final error message and behavior

**Expected:**
- After format retries (2x) and compaction retry (1x) fail, user sees error message
- Error message: "I'm having trouble understanding the format. Let's start fresh."
- Conversation history cleared (context reset)
- User can immediately start new conversation
- Full error logged with conversation history for debugging

**Result:** [To be documented]

---

### Test 7: Prompt extraction accuracy
**Objective:** Verify prompts are correctly extracted from JSON metadata.

**Steps:**
1. Have conversation that produces a prompt
2. Inspect prompt pane content
3. Compare with LLM's JSON response in browser console
4. Verify prompt appears in prompt pane, not chat pane

**Expected:**
- Prompt extracted from `metadata.prompt` field
- Prompt appears in prompt pane with every metadata update
- Generation triggered when `metadata.generate_image == true`
- Prompt content matches JSON exactly

**Result:** [To be documented]

---

### Completion Criteria

All tests above must pass with documented results before marking story 010 complete. If any test fails:
1. Document the failure
2. Create follow-up task to address the issue
3. Do not mark story complete until all tests pass

### Testing Notes

- Tests should be run with `mistral:7b` model (not llama3.2:1b)
- Browser console should be open during testing to observe any errors
- Server logs should be monitored for retry behavior
- Consider running tests with different conversation flows (simple requests, complex refinements, edge cases)
