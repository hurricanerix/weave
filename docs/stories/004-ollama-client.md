# Story 004: ollama LLM Client

## Problem

The Go application needs to communicate with ollama to get LLM responses for the conversational interface. The agent must understand user intent, ask clarifying questions to gather details, and output an image generation prompt in a parseable format. Responses must stream so the UI can update in real-time as the agent types.

## User/Actor

- End user (wants conversational image generation)
- Weave developer (implementing Go client)

## Desired Outcome

A working ollama client where:
- Go can send chat messages to ollama and receive streaming responses
- Multi-turn conversation context is maintained (agent remembers previous messages)
- Agent asks clarifying questions rather than making artistic assumptions
- Agent outputs image prompts in a consistent, parseable format (`Prompt: <text>`)
- Go extracts the prompt from the agent's response for display in the prompt pane
- Connection errors produce clear, actionable messages

## Acceptance Criteria

### HTTP Client

- [x] Client connects to ollama at `http://localhost:11434` (hardcoded for MVP)
- [x] Client uses `/api/chat` endpoint for chat completions
- [x] Client sends model name `llama3.2:1b` (hardcoded for MVP)
- [x] Client streams responses (processes newline-delimited JSON as it arrives)
- [x] Client accepts optional seed parameter for deterministic responses
- [x] Seed passed to ollama via `options.seed` in request body
- [x] Seed=nil means random (ollama default), any non-nil value (including 0) is deterministic
- [x] Client handles connection refused with error: "ollama not running at localhost:11434"
- [x] Client handles model not found with error: "model llama3.2:1b not available in ollama"
- [x] Client implements request timeout of 60 seconds (LLM responses can be slow)

### System Prompt

- [x] System prompt defines agent role: helping users describe images by asking clarifying questions
- [x] System prompt instructs agent to ask about details (style, subject specifics, setting, mood, etc.) rather than assume
- [x] System prompt instructs agent to output prompt on its own line starting with `Prompt: `
- [x] System prompt tells agent to only include `Prompt: ` line when it has enough information
- [x] System prompt instructs agent to preserve user edits when `[user edited prompt]` appears in history
- [x] System prompt is minimal (no prompt engineering tips, no style suggestions)
- [x] System prompt is defined as a constant in code (not configurable for MVP)

Example system prompt:
```
You help users create images. Your job is to ask clarifying questions to understand exactly what they want, then provide a prompt for the image generator.

When the user describes something, ask about details you need:
- Style (realistic, cartoon, painting, etc.)
- Subject details (what kind of cat? what kind of hat?)
- Setting/background
- Mood/tone
- Any other relevant details

Do not assume or inject your own artistic interpretation. Ask the user.

When you have enough information to generate, include exactly one line starting with "Prompt: " followed by the prompt. Only include this line when you're ready to generate.

When you see "[user edited prompt to: ...]" in the conversation, the user has manually edited the prompt. Preserve their changes in your next prompt—do not remove or override their edits unless they explicitly ask you to. Build on what they wrote.

Example:
User: I want a cat wearing a hat
Assistant: A cat in a hat! Let me ask a few questions:
- What kind of cat? (tabby, black cat, Persian, cartoon cat, etc.)
- What style of hat? (wizard hat, top hat, beanie, etc.)
- What's the setting? (indoors, outdoors, plain background, etc.)
- What style should the image be? (photo-realistic, illustration, painting, etc.)
```

### Multi-turn Conversation

- [x] Client accepts a list of previous messages (role + content pairs)
- [x] Client sends full conversation history to ollama on each request
- [x] Agent can reference previous context ("make it more blue" works if previous prompt mentioned a subject)
- [x] System prompt is always first message in conversation

### Prompt Extraction

- [x] Go parses agent response to extract line starting with `Prompt: `
- [x] If multiple `Prompt: ` lines exist, use the last one (agent may revise)
- [x] If no `Prompt: ` line exists, prompt field remains unchanged (agent gave conversational response only)
- [x] Extracted prompt has `Prompt: ` prefix stripped
- [x] Extracted prompt has leading/trailing whitespace trimmed

### Streaming Interface

- [x] Client provides callback/channel for streaming tokens as they arrive
- [x] Each streamed chunk contains the token text
- [x] Final message signals completion (with full response text)
- [x] Prompt extraction happens on complete response (not mid-stream)

### Testing

- [x] Unit test: prompt extraction finds `Prompt: ` line correctly
- [x] Unit test: prompt extraction handles multiple `Prompt: ` lines (uses last)
- [x] Unit test: prompt extraction handles missing `Prompt: ` line (returns empty)
- [x] Unit test: prompt extraction strips prefix and trims whitespace
- [x] Integration test: client connects to ollama and gets streaming response
- [x] Integration test: multi-turn conversation maintains context
- [x] Integration test: client handles ollama not running gracefully
- [x] Integration test: client handles model not found gracefully
- [x] Integration test: same seed produces identical responses (determinism test)
- [ ] Manual test: agent preserves user edits (edit prompt, send message, verify edit is retained in new prompt)

### Documentation

- [x] `docs/DEVELOPMENT.md` includes section on installing ollama
- [x] `docs/DEVELOPMENT.md` documents required model: `ollama pull llama3.2:1b`
- [x] `docs/DEVELOPMENT.md` explains how to verify ollama is running (`ollama list`)
- [x] `docs/DEVELOPMENT.md` includes troubleshooting for common ollama issues

## Out of Scope

- Configurable ollama endpoint
- Configurable model name
- Custom system prompts per session
- Token counting or context window management
- Retry logic on transient failures
- Response caching
- Prompt quality improvements or engineering tips in system prompt

## Dependencies

None. This is independent of the compute daemon stories.

## Notes

The agent's role is to clarify, not create. It asks questions to understand what the user wants rather than making artistic assumptions. This keeps the user in control of the creative direction.

The system prompt is intentionally minimal. We're proving the conversational interface works, not optimizing prompt quality. The `Prompt: ` marker format is simple and works well with streaming—Go can detect when the prompt line appears and update the UI immediately.

Conversation history is passed in full on each request. For MVP, we don't worry about context window limits. If conversations get too long, ollama will truncate or error—acceptable for MVP.

The model name `llama3.2:1b` may need adjustment based on exact ollama model naming. Developer should verify with `ollama list` after pulling.

## Tasks

### 001: Define ollama API types and constants
**Domain:** weave
**Status:** done
**Depends on:** none

Create `internal/ollama/types.go` with types for ollama API requests/responses. Define ChatRequest struct (model, messages, stream, options with seed), ChatResponse struct for streaming JSON responses, Message struct (role, content). Define system prompt constant per acceptance criteria.

**Files to create:**
- `internal/ollama/types.go`

**Testing:** None (type definitions only).

---

### 002: Implement ollama HTTP client
**Domain:** weave
**Status:** done
**Depends on:** 001

Create `internal/ollama/client.go` with Client type. Implement Connect() that verifies ollama is reachable at http://localhost:11434 (GET /api/tags). Handle connection refused with clear error. Set request timeout to 60 seconds.

**Files to create:**
- `internal/ollama/client.go`
- `internal/ollama/client_test.go`

**Testing:** Unit tests for error handling. Integration test connects to real ollama (tagged integration).

---

### 003: Implement streaming chat completion
**Domain:** weave
**Status:** done
**Depends on:** 002

In client.go, implement Chat() function that posts to /api/chat with model name, message history, stream=true, and optional seed in options. Parse newline-delimited JSON responses as they arrive. Provide callback/channel for streaming tokens. Signal completion with final response.

**Files to modify:**
- `internal/ollama/client.go`
- `internal/ollama/client_test.go`

**Testing:** Unit test parses streaming JSON. Integration test receives streamed response from ollama.

---

### 004: Implement prompt extraction logic
**Domain:** weave
**Status:** done
**Depends on:** 001

Create `internal/ollama/prompt.go` with ExtractPrompt() function. Parse assistant response to find lines starting with "Prompt: ". If multiple found, use last one. Strip "Prompt: " prefix and trim whitespace. Return empty string if no prompt line found.

**Files to create:**
- `internal/ollama/prompt.go`
- `internal/ollama/prompt_test.go`

**Testing:** Table-driven tests for single prompt, multiple prompts (uses last), no prompt, whitespace handling.

---

### 005: Implement seed support for deterministic responses
**Domain:** weave
**Status:** done
**Depends on:** 002

Seed support was implemented as part of Task 003. Chat() accepts `*int64` seed parameter. If non-nil, included in request options. If nil, omitted (ollama uses random). Behavior documented in ChatOptions struct.

**Files to modify:**
- `internal/ollama/client.go`
- `internal/ollama/client_test.go`

**Testing:** Integration test verifies same seed produces identical responses (requires real ollama).

---

### 006: Integration test for full chat flow
**Domain:** weave
**Status:** done
**Depends on:** 003, 004

Create integration test that sends multi-turn conversation to ollama, receives streaming responses, extracts prompt from agent response. Verify conversation context is maintained across turns.

**Files to create:**
- `internal/ollama/integration_test.go` (tagged integration)

**Testing:** Integration test passes with real ollama. Verify streaming, prompt extraction, multi-turn context.

---

### 007: Update DEVELOPMENT.md with ollama setup
**Domain:** documentation
**Status:** done
**Depends on:** 006

Add section explaining how to install ollama, pull llama3.2:1b model, verify it's running. Include troubleshooting for common ollama issues (not running, model not found, port conflict).

**Files to modify:**
- `docs/DEVELOPMENT.md`

**Verification:** Documentation is clear. Commands work as documented.
