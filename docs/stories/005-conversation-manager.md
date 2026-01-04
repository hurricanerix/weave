# Story 005: Conversation Manager

## Problem

The Go application needs to track conversation state: chat history for LLM context, current prompt state, and user edits. When the user manually edits the prompt, the system must inject a notification into the chat history so the agent knows to incorporate the edit. This is the state machine that connects the LLM client to the UI.

## User/Actor

- End user (wants multi-turn conversation with edit awareness)
- Weave developer (implementing state management)

## Desired Outcome

A working conversation manager where:
- Chat history is maintained across multiple turns
- Current prompt state is tracked separately from chat history
- User edits to the prompt are detected and recorded
- Agent receives notification when user has edited the prompt
- Agent can see the current prompt state to avoid clobbering user edits

## Acceptance Criteria

### State Management

- [ ] Manager maintains ordered list of messages (role + content)
- [ ] Supported roles: `user`, `assistant`, `system`
- [ ] Manager tracks current prompt as separate state (not derived from messages)
- [ ] Manager tracks whether prompt was edited since last agent update
- [ ] State is in-memory only (no persistence for MVP)
- [ ] Each browser session gets its own conversation state (no shared state)

### Message Operations

- [ ] `AddUserMessage(content)` - Adds user message to history
- [ ] `AddAssistantMessage(content, prompt)` - Adds agent message and updates current prompt
- [ ] `GetHistory()` - Returns full message history (for LLM context)
- [ ] `GetCurrentPrompt()` - Returns current prompt state
- [ ] `Clear()` - Resets conversation to empty state

### User Edit Flow

- [ ] `UpdatePrompt(newPrompt)` - Called when user edits prompt in UI
- [ ] If prompt changed from previous value, sets "edited" flag
- [ ] `NotifyPromptEdited()` - Injects system message into history
- [ ] Injected message content: `[user edited prompt to: "<current prompt>"]`
- [ ] Injected message includes the current prompt text so agent can see what user wrote
- [ ] After injection, "edited" flag is cleared
- [ ] If prompt hasn't changed, `NotifyPromptEdited()` does nothing

### LLM Context Construction

- [ ] `BuildLLMContext()` - Returns messages formatted for ollama
- [ ] System prompt is prepended (not stored in message history)
- [ ] Current prompt is appended as context: `[current prompt: "<prompt>"]`
- [ ] Agent sees both the conversation history and current prompt state

Example LLM context:
```
[system] You help users create images... (system prompt)
[user] I want a cat in a hat
[assistant] A cat in a hat! Let me ask...
[user] Make it a tabby cat with a wizard hat
[assistant] Got it! Here's what I have:

Prompt: a tabby cat wearing a wizard hat, fantasy style
[system] [user edited prompt to: "a tabby cat wearing a sparkly wizard hat, fantasy style"]
[user] Now make the background purple
[system] [current prompt: "a tabby cat wearing a sparkly wizard hat, fantasy style"]
```

### Session Management

- [ ] New session created on first request from a browser
- [ ] Session identified by cookie or session ID
- [ ] Session state isolated from other sessions
- [ ] Session timeout/cleanup deferred (acceptable to leak for MVP)

### Testing

- [ ] Unit test: adding messages maintains order
- [ ] Unit test: `GetHistory()` returns all messages
- [ ] Unit test: `AddAssistantMessage` updates current prompt
- [ ] Unit test: `UpdatePrompt` sets edited flag when prompt changes
- [ ] Unit test: `UpdatePrompt` does not set flag when prompt unchanged
- [ ] Unit test: `NotifyPromptEdited` injects system message with prompt text
- [ ] Unit test: `NotifyPromptEdited` clears edited flag
- [ ] Unit test: `NotifyPromptEdited` no-ops if not edited
- [ ] Unit test: `BuildLLMContext` includes system prompt, history, and current prompt
- [ ] Unit test: `Clear` resets all state

### Documentation

- [ ] Code comments explain the edit notification flow
- [ ] Code comments explain why current prompt is passed to LLM separately

## Out of Scope

- Conversation persistence (database, file storage)
- Conversation branching or undo
- Multiple conversations per session
- Session timeout/cleanup
- Token counting or history truncation
- Message editing or deletion

## Dependencies

- Story 004: ollama LLM Client (provides system prompt constant, consumes context)

## Notes

The key insight is that the agent needs TWO pieces of information about user edits:
1. **That** the user edited (the `[user edited prompt]` notification)
2. **What** the current prompt is (included in the notification and as trailing context)

This lets the agent incorporate user changes without the user having to explain what they changed.

The current prompt is passed as trailing context on every LLM request so the agent always knows the current state, even if several turns have passed since the last edit.

Session management can be simple for MVP—a map of session ID to conversation state. Memory leaks from abandoned sessions are acceptable for MVP scope.

## Tasks

### 001: Define conversation state types
**Domain:** weave
**Status:** done
**Depends on:** none

Create `internal/conversation/types.go` with Message struct (role, content), Conversation struct (messages, current_prompt, edited_flag). Define message roles (system, user, assistant). Keep types simple, in-memory only.

**Files to create:**
- `internal/conversation/types.go`

**Testing:** None (type definitions only).

---

### 002: Implement conversation manager core operations
**Domain:** weave
**Status:** done
**Depends on:** 001

Create `internal/conversation/manager.go` with Manager type. Implement AddUserMessage(), AddAssistantMessage(content, prompt), GetHistory(), GetCurrentPrompt(), Clear(). Messages stored in order. Current prompt tracked separately from history.

**Files to create:**
- `internal/conversation/manager.go`
- `internal/conversation/manager_test.go`

**Testing:** Unit tests verify message ordering, prompt tracking, history retrieval, clear operation.

---

### 003: Implement user edit detection and notification
**Domain:** weave
**Status:** done
**Depends on:** 002

In manager.go, add UpdatePrompt(newPrompt) that sets edited flag if prompt changed. Add NotifyPromptEdited() that injects system message "[user edited prompt to: <prompt>]" if edited flag is set. Clear flag after injection. No-op if not edited.

**Files to modify:**
- `internal/conversation/manager.go`
- `internal/conversation/manager_test.go`

**Testing:** Unit tests verify edit detection, notification injection, flag clearing, no-op when unchanged.

---

### 004: Implement LLM context construction
**Domain:** weave
**Status:** done
**Depends on:** 003

In manager.go, add BuildLLMContext(systemPrompt) that prepends system prompt, includes all messages, appends "[current prompt: <prompt>]" as trailing context. Return as slice of messages for ollama client (Story 004).

**Files to modify:**
- `internal/conversation/manager.go`
- `internal/conversation/manager_test.go`

**Testing:** Unit tests verify system prompt prepending, trailing context, message ordering, integration with edit notifications.

---

### 005: Implement session manager for multi-session support
**Domain:** weave
**Status:** done
**Depends on:** 002

Create `internal/conversation/session.go` with SessionManager type. Map of session ID (string) to Manager. Implement GetOrCreate(sessionID) that returns existing or creates new Manager. Thread-safe with mutex. No session cleanup for MVP.

**Files to create:**
- `internal/conversation/session.go`
- `internal/conversation/session_test.go`

**Testing:** Unit tests verify session creation, retrieval, isolation between sessions, thread safety.

---

### 006: Integration test for conversation flow
**Domain:** weave
**Status:** done
**Depends on:** 004, 005

Create integration test that simulates multi-turn conversation with user edits. Verify messages maintain order, edits trigger notifications, LLM context includes all required parts. Test multiple sessions remain isolated.

**Files to create:**
- `internal/conversation/integration_test.go`

**Testing:** Integration test passes. Verify edit notification flow, context construction, session isolation.

---

### 007: Document conversation manager design
**Domain:** documentation
**Status:** done
**Depends on:** 006

Add code comments explaining edit notification flow, why current prompt is separate from history, how LLM context is constructed. Explain session management approach and MVP limitations (no cleanup).

**Files to modify:**
- `internal/conversation/manager.go`
- `internal/conversation/session.go`

**Verification:** Comments are clear and explain the design rationale.
