# Story: Two-Stage LLM Flow for Contextual Responses

## Status
Ready

## Problem
When users chat with Ara, they receive the same generic response every time: "Generating image. Try adjusting the style or adding more details to refine the result." This happens because llama3.1:8b returns only a function call without conversational text, and the backend falls back to a hardcoded message.

Users expect Ara to engage with their specific prompt - acknowledging what they asked for and offering relevant suggestions for refinement.

## User/Actor
End user interacting with Ara through the chat interface to generate images.

## Desired Outcome
Ara responds with contextual, helpful text that references the user's actual prompt and guides them toward refining their vision. The response feels like a conversation with a creative partner, not a generic system message.

## Acceptance Criteria
- [ ] When user describes an image (e.g., "a cat dancing"), Ara's response references the prompt content (e.g., "Here's a dancing cat! Want to try a specific dance style?")
- [ ] Ara does not respond with the generic fallback message under normal operation
- [ ] When user asks a question (e.g., "how does seed work?"), Ara answers the question without generating an image
- [ ] When user delegates (e.g., "you decide"), Ara makes creative choices and generates without asking more questions
- [ ] If the conversation LLM call fails after one retry, user sees "I'm sorry, I'm having trouble responding right now." instead of a blank or broken response
- [ ] Image generation still works correctly (prompt extraction, settings, generation trigger)

## Out of Scope
- UI/visual feedback changes (separate story)
- Cancellation of in-flight requests
- Changing the LLM model
- Changes to compute or protocol layers

## Dependencies
None - this builds on existing infrastructure.

## Open Questions
None - design was resolved in /architect discussion.

## Notes
Technical approach: Split the single LLM call into two stages:
1. **Extraction** (`ara_tools.md` with function calling) - extracts prompt, settings, generate_image flag
2. **Conversation** (`ara.md` without tools) - generates contextual response text

When `generate_image: true`, Stage 2 and image generation run in parallel to hide latency.

Prompt files `ara.md` and `ara_tools.md` have been drafted in `config/agents/`.

## Tasks

### Task 001: Add second prompt path to config
**Domain:** backend
**Status:** pending

Add `AgentToolsPromptPath` field to `config.Config` and corresponding CLI flag `--agent-tools-prompt`. Default to `config/agents/ara_tools.md`. Update `LoadAgentPrompt` or add similar function to load both prompts.

**Files:**
- `backend/internal/config/config.go`

**Test:** Unit test that both prompts can be loaded with valid paths.

---

### Task 002: Load both prompts in server initialization
**Domain:** backend
**Status:** pending

Update `Server` struct to hold both `agentPrompt` (for conversation) and `agentToolsPrompt` (for extraction). Load both during `NewServer()` initialization.

**Files:**
- `backend/internal/web/server.go`

**Test:** Server starts successfully with both prompts loaded.

---

### Task 003: Create buildExtractionPrompt function
**Domain:** backend
**Status:** pending

Create `buildExtractionPrompt()` that combines `agentToolsPrompt` with function calling instructions. This replaces `buildSystemPrompt()` for Stage 1. The function calling schema stays the same.

**Files:**
- `backend/internal/web/server.go`

**Test:** Unit test verifying prompt includes tools content and function schema.

---

### Task 004: Create buildConversationPrompt function
**Domain:** backend
**Status:** pending

Create `buildConversationPrompt(prompt string, generating bool)` that combines `agentPrompt` with context about what's being generated. No function calling instructions (Stage 2 doesn't use tools).

**Files:**
- `backend/internal/web/server.go`

**Test:** Unit test verifying prompt includes conversation content and context variables.

---

### Task 005: Refactor handleChat for two-stage flow
**Domain:** backend
**Status:** pending

Refactor `handleChat` to implement two-stage LLM flow:

1. **Stage 1 (Extraction):** Call LLM with `buildExtractionPrompt()` and tools. No streaming callback. Extract `LLMMetadata` from result.
2. **Stage 2 (Conversation):** Call LLM with `buildConversationPrompt()` and no tools. Stream tokens to SSE.
3. When `generate_image: true`, run Stage 2 and `generateImage` in parallel using goroutines.
4. When `generate_image: false`, just run Stage 2.

Remove `generateFallbackResponse()` usage - Stage 2 always provides text.

**Files:**
- `backend/internal/web/server.go`

**Test:** Integration test with mock ollama verifying both stages are called.

---

### Task 006: Add retry logic for Stage 2
**Domain:** backend
**Status:** pending

Add retry logic specifically for Stage 2 conversation call:
- On failure, retry once
- If second attempt fails, send canned error: "I'm sorry, I'm having trouble responding right now."
- Log full error server-side

Stage 1 failures should use existing retry/error handling.

**Files:**
- `backend/internal/web/server.go`

**Test:** Unit test verifying retry behavior and error message.

---

### Task 007: Remove fallback response function
**Domain:** backend
**Status:** pending

Delete `generateFallbackResponse()` function and all references. The two-stage flow eliminates the need for fallback responses.

**Files:**
- `backend/internal/web/server.go`

**Test:** Grep confirms no references to fallback response remain.

---

### Task 008: Update SSE event for expanded state
**Domain:** backend
**Status:** pending

Modify `EventAgentThinking` to accept `expanded` boolean:
- `agent-thinking { expanded: false }` - Stage 1 starting (small bubble)
- `agent-thinking { expanded: true }` - Stage 1 complete, Stage 2 starting (expand bubble)

Send first event before Stage 1, second event after Stage 1 completes.

**Files:**
- `backend/internal/web/server.go`
- `backend/internal/web/sse.go` (update comment/schema docs)

**Test:** Integration test verifying both events are sent in sequence.
