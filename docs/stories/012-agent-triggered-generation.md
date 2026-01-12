# Story: Agent-triggered image generation

## Status
Complete

## Problem
The agent can update prompt and generation settings through conversation, but cannot trigger generation automatically. Users must manually click the generate button every time they want to see results, even when the agent decides it would be helpful to show a preview. This breaks conversational flow - the agent might refine a prompt through several iterations, but the user has to remember to click generate to see the results.

Additionally, the existing `ready` field blocks prompt updates from reaching the UI when `ready=false`. This prevents users from seeing and manually tweaking the prompt as the agent refines it during the conversation.

## User/Actor
End-user conversing with the AI agent through the web UI to create images.

## Desired Outcome
The agent can trigger image generation automatically when it determines a preview would be helpful. The user can tell the agent their preference for how often to auto-generate (every time settings change, never, every N tweaks, or agent's discretion). The UI always reflects the current prompt and settings regardless of whether the agent is ready to generate, so users can manually tweak values at any time.

## Acceptance Criteria
- [x] Agent JSON includes `generate_image` boolean field alongside prompt and generation settings
- [x] The `ready` field is removed from agent JSON entirely
- [x] Prompt updates always reach the UI, regardless of any other field values
- [x] When `generate_image=true`, the backend directly invokes the existing generation code (same path as POST /generate)
- [x] When `generate_image=false`, no automatic generation occurs (user can still click manually)
- [x] Generation triggered by `generate_image=true` behaves identically to manual button click (same generation logic, same EventImageReady response)
- [x] Prompt and settings fields are marked read-only (disabled) during agent response
- [x] Prompt and settings fields become editable again when agent finishes responding
- [x] Agent system prompt explains when and how to use the `generate_image` field
- [x] Agent system prompt explains that users can specify auto-generation preferences conversationally
- [x] User can tell agent "generate every time you change something" and agent respects it
- [x] User can tell agent "never auto-generate" and agent respects it
- [x] User can tell agent "generate every 3 tweaks" and agent respects it
- [x] User can tell agent "use your judgment" and agent decides when previews are most helpful
- [x] All existing tests pass with `ready` field removed and `generate_image` added
- [x] Agent-triggered generation respects rate limiting (added per security review)

## Out of Scope
- UI controls for auto-generation preferences (handled conversationally only)
- Persisting user preferences across sessions
- Model switching for final vs preview images (future feature)
- Canceling an in-progress generation triggered by the agent
- Confirming with user before auto-generating (agent decides based on conversation)

Note: Rate limiting for agent-triggered generations was added per security review (originally listed as out of scope).

## Dependencies
Story 011 (generation settings UI) must be complete.

## Open Questions
None.

## Notes
This story removes the `ready` field that was creating confusion and blocking prompt updates. The original intent of `ready` (indicating "I have enough info to generate") is replaced by the clearer, action-oriented `generate_image` field that explicitly controls whether generation happens.

The conversational preference system means no new UI is required. Users naturally express their preferences ("show me previews as you go" or "only when you think it's ready"), and the agent tracks and respects those preferences when deciding whether to set `generate_image=true`.

Current agent JSON format (story 011):
```json
{
  "prompt": "detailed prompt text",
  "ready": true,
  "steps": 20,
  "cfg": 5.0,
  "seed": -1
}
```

New agent JSON format (this story):
```json
{
  "prompt": "detailed prompt text",
  "generate_image": true,
  "steps": 20,
  "cfg": 5.0,
  "seed": -1
}
```

When `generate_image=true`, the backend directly calls the existing generation logic (same code path as POST /generate). The UI receives the standard `EventImageReady` when generation completes. No new SSE event type is needed.

## Tasks

### 001: Remove `ready` field from agent JSON parsing
**Domain:** weave
**Status:** pending
**Depends on:** none

Remove the `Ready bool` field from `Metadata` struct in `internal/ollama/client.go`. Remove all references to parsing and validating the `ready` field in JSON extraction logic. The field should no longer be expected in agent responses. Update any tests that reference the `ready` field to remove those assertions.

---

### 002: Add `generate_image` field to agent JSON parsing
**Domain:** weave
**Status:** pending
**Depends on:** 001

Add `GenerateImage bool` field to `Metadata` struct in `internal/ollama/client.go`. Add JSON parsing for the `generate_image` field in the metadata extraction logic. Default to `false` if the field is missing. Add unit tests in `internal/ollama/client_test.go` to verify `generate_image` parsing works correctly (true, false, missing).

---

### 003: Update prompt update logic to remove `ready` check
**Domain:** weave
**Status:** pending
**Depends on:** 001

In `handleChat` function in `internal/web/server.go`, find the code that extracts and sends prompt updates (around line 391-406). Remove the condition `if result.Metadata.Ready` that blocks prompt updates when ready is false. Prompt updates should always be sent if the prompt is non-empty, regardless of any other field values. This ensures the UI always reflects the current prompt.

---

### 004: Trigger generation directly when `generate_image=true`
**Domain:** weave
**Status:** pending
**Depends on:** 002, 003

In `handleChat` function in `internal/web/server.go`, after sending prompt and settings updates, check if `result.Metadata.GenerateImage` is true. If so, call the existing generation logic directly (same code path that handles POST /generate). Use the session's current prompt, steps, cfg, and seed values. This happens server-side, before sending `EventAgentDone`. The existing `EventImageReady` will notify the UI when generation completes.

---

### 005: Disable prompt/settings during agent response
**Domain:** weave
**Status:** done
**Depends on:** none

In `internal/web/templates/index.html`, modify `handleChatSubmit` function to disable prompt field and all settings inputs when user sends a message (set `isAgentResponding=true`). Add helper function `setPromptAndSettingsEnabled(enabled)` that sets the `disabled` property on `#prompt-field`, `#steps-input`, `#cfg-input`, and `#seed-input`. Call with `false` in `handleChatSubmit` and with `true` in `handleAgentDone`.

---

### 006: Remove focus-based update blocking in UI
**Domain:** weave
**Status:** done
**Depends on:** 005

In `internal/web/templates/index.html`, modify `handlePromptUpdate` to remove the `if (!isPromptFocused)` check. Modify `handleSettingsUpdate` to remove focus checks for `isStepsFocused`, `isCfgFocused`, and `isSeedFocused`. Since fields are disabled during agent response, these checks are redundant and can cause race conditions where updates are blocked incorrectly.

---

### 007: Update agent system prompt with `generate_image` field
**Domain:** weave
**Status:** done
**Depends on:** 002

Update `SystemPrompt` constant in `internal/ollama/systemprompt.go` to replace mentions of the `ready` field with `generate_image`. Document that `generate_image: true` triggers automatic generation, `generate_image: false` does not. Explain that users can specify auto-generation preferences conversationally ("generate every time", "never auto-generate", "every 3 tweaks", "use your judgment"). The agent should track user preferences and set `generate_image` accordingly.

---

### 008: Update unit tests for `generate_image` field
**Domain:** weave
**Status:** pending
**Depends on:** 002, 003

Update tests in `internal/ollama/client_test.go` and `internal/web/server_test.go` that reference the `ready` field to use `generate_image` instead. Verify that prompt updates are sent regardless of `generate_image` value. Add test cases for parsing `generate_image` from agent JSON responses.

---

### 009: Integration test for agent-triggered generation
**Domain:** weave
**Status:** pending
**Depends on:** 004

Add integration test in `internal/web/integration_test.go` that simulates an agent response with `generate_image=true`. Mock the Ollama client to return metadata with `generate_image: true`. Verify that:
1. `EventPromptUpdate` is sent with the prompt
2. `EventSettingsUpdate` is sent with settings
3. Generation is triggered server-side
4. `EventImageReady` is eventually sent with the generated image
Test the complete flow from agent response to image delivery.

---

### 010: Integration test for agent without auto-generation
**Domain:** weave
**Status:** pending
**Depends on:** 004

Add integration test in `internal/web/integration_test.go` that simulates an agent response with `generate_image=false`. Mock the Ollama client to return metadata with `generate_image: false`. Verify that:
1. `EventPromptUpdate` is sent with the prompt
2. `EventSettingsUpdate` is sent with settings
3. `EventAgentDone` is sent
4. No `EventImageReady` follows (generation not triggered)
Confirm the agent can update settings without triggering generation.

---
