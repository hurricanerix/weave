# Story: Agent reliability overhaul

## Status
Done

## Problem
The conversational agent (Ara) is unreliable, creating friction when testing the prototype and iterating on the UI/UX. Specific issues:

1. **Doesn't generate often enough** - Ara asks too many clarifying questions before generating, slowing down the iteration loop
2. **Context fills fast** - Conversations become incoherent after a few exchanges
3. **Hallucinations** - Ara loses track of the conversation, inventing participants or referencing things that didn't happen
4. **Format failures** - The delimiter+JSON output format is fragile; Ara frequently outputs malformed responses requiring retries

## User/Actor
Developer testing the prototype and iterating on UI/UX. Needs Ara to be reliable enough to demo the app and test other features without the agent being a constant source of friction.

## Desired Outcome
Ara generates images early in the conversation (when visual concepts are mentioned) and maintains coherent context across multiple exchanges. The developer can run short demos and iterate on UI/UX without fighting the agent.

## Acceptance Criteria
- [ ] When user describes any visual concept (even minimal like "a cat"), Ara generates an image immediately using a best-effort prompt AND responds with suggestions/questions for refinement
- [ ] When user is just chatting with no visual concept, Ara responds conversationally without generating
- [ ] Conversations can go 10+ exchanges while Ara maintains context - knows what was discussed, doesn't invent participants or reference things that didn't happen
- [ ] Backend sends a "thinking" SSE event before the LLM call starts, giving the UI feedback while waiting for a response
- [ ] Ara's behavioral instructions (when to generate, personality, interaction style) are externalized to a config file that can be edited without rebuilding
- [ ] Function calling replaces delimiter+JSON parsing for structured output

## Out of Scope
- Multi-image generation (showing 2+ options per response)
- Preview vs quality model switching
- Adaptive behavior (reading user tone, adjusting verbosity)
- Context compaction for indefinite conversations
- Deep SD expertise in the system prompt

## Dependencies
None

## Open Questions
- What model should be the new default? Llama 3.1 8B is the current recommendation, but may need testing to confirm it performs well for this use case.

## Notes
This story changes the LLM integration architecture:

1. **Model upgrade** - Switch default from Mistral 7B (8k context) to Llama 3.1 8B (128k context) for better context handling and native function calling support

2. **Function calling** - Replace the fragile delimiter+JSON parsing with native function calling. The model calls an `update_generation` function with structured parameters instead of outputting formatted text:
   ```
   update_generation(
     prompt: string,
     steps: integer,
     cfg: number,
     seed: integer,
     generate: boolean
   )
   ```

3. **System prompt restructure** - Move behavioral instructions to `config/agents/ara.md`. Function schema and technical wiring stay in code. Prompt should emphasize "generate early, iterate through conversation" behavior.

4. **Thinking indicator** - New SSE event type so UI can show feedback while waiting for slower model responses.

Prior discussion captured the architecture rationale: function calling is more reliable than delimiter parsing, and the ecosystem is moving toward tool use as the standard pattern for structured LLM output.

## Tasks

### 001: Add thinking SSE event
**Domain:** backend
**Status:** done
**Depends on:** none

Add `EventAgentThinking` constant to `backend/internal/web/sse.go`. Send this event in `handleChat` immediately before calling `chatWithRetry()` (around line 380). Data schema: `{"started": true}`. This gives the UI a signal that Ara is processing before tokens start streaming.

---

### 002: Display thinking indicator in UI
**Domain:** electron
**Status:** done
**Depends on:** 001

Handle the `agent-thinking` SSE event in the web UI. When received, display a typing indicator or "Ara is thinking..." message in the chat pane. Hide the indicator when the first `agent-token` event arrives or on `agent-done`. Keep it simple - a CSS animation or text indicator is sufficient.

---

### 003: Add agent prompt file loading
**Domain:** backend
**Status:** done
**Depends on:** none

Add support for loading the agent prompt from an external file. Add `--agent-prompt` CLI flag (default: `config/agents/ara.md`). Load file contents during server initialization. Store in `Server` struct for use in chat handler. Return clear error if file doesn't exist or is unreadable.

---

### 004: Create ara.md behavioral prompt
**Domain:** backend
**Status:** done
**Depends on:** 003

Create `config/agents/ara.md` with Ara's behavioral instructions extracted from the current system prompt in `types.go`. Include: personality/tone, when to generate images (generate early on any visual concept), interaction style, SD parameter guidance. Exclude: function calling format instructions (those stay in code). Update `handleChat` to assemble the full system prompt from file content + code-generated function instructions.

---

### 005: Add function calling types to ollama client
**Domain:** backend
**Status:** done
**Depends on:** none

Extend ollama client types to support function calling. Add `Tools` field to `ChatRequest` (slice of tool definitions). Add `ToolCalls` field to `ChatResponse` and `Message` structs. Define `Tool`, `ToolFunction`, and `ToolCall` types matching ollama's function calling API. The `update_generation` function takes: prompt (string), steps (int), cfg (float), seed (int), generate (bool).

---

### 006: Implement function call parsing
**Domain:** backend
**Status:** done
**Depends on:** 005

Update `parseStreamingResponse()` to handle responses with function calls. Tool calls appear in the final response chunk when `Done: true`. Extract tool calls from the response and populate `ChatResult`. Update `StreamCallback` behavior - conversational text still streams to callback, function call data is extracted at the end. Remove delimiter detection logic from streaming parser.

---

### 007: Integrate function calling in chat handler
**Domain:** backend
**Status:** done
**Depends on:** 004, 006

Update `handleChat` to use function calls instead of delimiter-parsed `LLMMetadata`. Build the system prompt by combining ara.md content with function schema instructions. Send tools array with chat request. Extract generation parameters from tool call response instead of JSON metadata. Update retry logic - format errors (ErrMissingDelimiter, etc.) are replaced by tool call validation errors. Send same SSE events (prompt-update, settings-update, generation-started) based on function call data.

---

### 008: Update default model to llama3.1:8b
**Domain:** backend
**Status:** done
**Depends on:** none

Change `DefaultModel` constant in `backend/internal/ollama/types.go` from `mistral:7b` to `llama3.1:8b`. Update any documentation or comments that reference the old default. This model has 128k context window and native function calling support.

---

### 009: Remove deprecated delimiter parsing
**Domain:** backend
**Status:** done
**Depends on:** 007

Clean up code that's no longer needed after function calling migration. Remove: `ResponseDelimiter` constant, `ErrMissingDelimiter`/`ErrInvalidJSON`/`ErrMissingFields` error types, delimiter-based `parseResponse()` function, format reminder retry logic in `chatWithRetry()`. Keep the compaction retry logic (Level 2) as it's still useful for context management. Update or remove tests that verify delimiter parsing behavior.
