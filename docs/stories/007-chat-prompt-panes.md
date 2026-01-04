# Story 007: Chat and Prompt Panes

## Problem

The user needs to interact with the conversational interface: send messages, see agent responses stream in, view and edit the prompt, and generate images. This story implements the actual UI features on top of the foundation from Story 006.

## User/Actor

- End user (wants to chat, edit prompts, generate images)
- Weave developer (implementing UI features)

## Desired Outcome

A working UI where:
- User can send chat messages and see them appear
- Agent responses stream in token by token
- Prompt pane updates live as agent responds
- User can edit the prompt directly
- Blur on prompt field notifies the backend
- Generate button triggers image generation
- Generated images appear inline in chat

## Acceptance Criteria

### Chat Pane (Left Side)

#### Message Display
- [ ] User messages appear with visual distinction (alignment, color, or label)
- [ ] Agent messages appear with visual distinction from user messages
- [ ] Messages appear in chronological order (newest at bottom)
- [ ] Chat auto-scrolls to bottom when new messages appear
- [ ] Agent messages stream in token by token (via SSE `agent-token` events)
- [ ] Streaming agent message shows typing indicator or partial text

#### Message Input
- [ ] Text input field at bottom of chat pane
- [ ] Send button (or Enter key) submits message
- [ ] Input clears after sending
- [ ] Input disabled while agent is responding (prevents double-send)
- [ ] Submit posts to `/chat` endpoint with message content

#### Inline Images
- [ ] When image is generated, it appears in chat at correct position
- [ ] Image appears after the agent message that preceded generation
- [ ] Image displays at reasonable size (not full resolution, clickable to expand—or just fixed size for MVP)
- [ ] Image loaded via `image-ready` SSE event with image URL/data

### Prompt Pane (Right Side)

#### Prompt Display
- [ ] Textarea shows current prompt
- [ ] Prompt updates live as agent responds (via SSE `prompt-update` events)
- [ ] Live updates only happen while agent is streaming (not mid-user-edit)

#### Prompt Editing
- [ ] User can click into textarea and edit text
- [ ] While user is focused on textarea, SSE updates do not clobber their typing
- [ ] On blur (click outside textarea), `POST /prompt` is sent with current content
- [ ] Backend handles prompt update and injects edit notification (Story 005)
- [ ] Visual feedback that edit was saved (subtle, e.g., brief flash or checkmark)

#### Edit Conflict Prevention
- [ ] If textarea has focus, incoming `prompt-update` events are queued or ignored
- [ ] On blur, queued updates are discarded (user edit takes precedence)
- [ ] This prevents the jarring experience of text changing while typing

### Generate Flow

- [ ] "Generate" button visible in UI (location flexible—below prompt or in chat)
- [ ] Clicking Generate posts to `/generate` endpoint
- [ ] Button disabled while generation is in progress
- [ ] Progress indicator shown during generation (spinner, text, or similar)
- [ ] On completion, `image-ready` SSE event triggers image display in chat
- [ ] On error, error message displayed in chat

### Error Handling

- [ ] Network errors show message in chat: "Connection error, please try again"
- [ ] LLM errors show message in chat: "Agent error: <message>"
- [ ] Generation errors show message in chat: "Generation failed: <message>"
- [ ] Errors are dismissible or auto-clear on next action

### Testing

- [ ] Integration test: send message, receive streamed response
- [ ] Integration test: prompt pane updates as agent streams
- [ ] Integration test: edit prompt, blur, verify backend receives update
- [ ] Integration test: generate image, verify it appears in chat
- [ ] Manual test: rapid typing in prompt field is not interrupted by SSE updates
- [ ] Manual test: conversation flows naturally over multiple turns

### Documentation

- [ ] `docs/DEVELOPMENT.md` includes walkthrough of user flow
- [ ] `docs/DEVELOPMENT.md` explains how to test each interaction

## Out of Scope

- Conversation history persistence
- Image download/save functionality
- Image zoom/lightbox
- Markdown rendering in messages
- Message timestamps
- Agent "thinking" indicator beyond token streaming
- Keyboard shortcuts
- Accessibility (a11y) polish

## Dependencies

- Story 004: ollama LLM Client (provides agent responses)
- Story 005: Conversation Manager (tracks state, handles edit notifications)
- Story 006: Web UI Foundation (provides server, SSE, layout)

## Notes

The edit conflict prevention is important UX. If the user is typing and the prompt suddenly changes, they'll lose their work and trust. Simple solution: track focus state in JS, ignore SSE updates while focused.

For image display, the simplest approach is to have the server save the image to a temp file and send the URL. Browser fetches image via normal `<img src="...">`. Alternatively, send base64 data URL—works but bloats the SSE payload.

The "Generate" button could live in the prompt pane (next to the prompt) or in the chat pane (as a chat action). Either works—pick during implementation based on what feels natural.

Agent streaming: each `agent-token` event appends to the current agent message div. When `agent-done` fires, the message is finalized. The `prompt-update` event fires whenever the agent outputs a `Prompt: ` line (detected server-side during streaming or on completion).

## Tasks

### 001: Implement chat message display logic
**Domain:** weave
**Status:** pending
**Depends on:** none

Update index.html to render user and agent messages with visual distinction (classes, alignment). Add JavaScript or HTMX attributes to auto-scroll chat to bottom when new messages arrive. Style messages for readability.

**Files to modify:**
- `internal/web/templates/index.html`
- `internal/web/static/style.css`

**Testing:** Manual test. Send messages, verify they appear correctly and auto-scroll works.

---

### 002: Implement chat input form with HTMX
**Domain:** weave
**Status:** pending
**Depends on:** 001

Add form in chat pane with text input and send button. Use hx-post="/chat" to submit. Clear input after send. Disable input while agent is responding (track with JavaScript state or CSS class). Include session ID in request.

**Files to modify:**
- `internal/web/templates/index.html`

**Testing:** Manual test. Type message, click send, verify input clears and disables during response.

---

### 003: Wire POST /chat handler to ollama and conversation manager
**Domain:** weave
**Status:** pending
**Depends on:** 002

In server.go, implement POST /chat handler. Extract session ID, get conversation manager (Story 005), add user message, build LLM context, call ollama client (Story 004) with streaming. For each token, send agent-token SSE event. Extract prompt from complete response (Story 004), send prompt-update SSE event. Add assistant message to conversation. Return 200.

**Files to modify:**
- `internal/web/server.go`
- `internal/web/server_test.go`

**Testing:** Integration test sends chat message, verifies streaming tokens arrive via SSE, prompt extracted and sent as prompt-update event.

---

### 004: Implement prompt pane with live updates
**Domain:** weave
**Status:** pending
**Depends on:** none

In index.html, add textarea for prompt with id="prompt-field". Connect hx-sse to prompt-update events that replace textarea content. Ensure updates only apply when textarea is not focused (prevent clobbering user typing).

**Files to modify:**
- `internal/web/templates/index.html`

**Testing:** Manual test. Agent sends prompt-update, verify textarea updates. Verify no update while typing (focus prevention).

---

### 005: Implement prompt blur handler with edit notification
**Domain:** weave
**Status:** pending
**Depends on:** 004

Add onblur event to prompt textarea that posts to /prompt with current content. In server.go, implement POST /prompt handler that gets conversation manager, calls UpdatePrompt(), calls NotifyPromptEdited() (Story 005). Send visual feedback (brief flash or checkmark) to user.

**Files to modify:**
- `internal/web/templates/index.html`
- `internal/web/server.go`
- `internal/web/server_test.go`

**Testing:** Unit test verifies POST /prompt handler calls conversation manager correctly. Manual test: edit prompt, blur, verify backend receives update.

---

### 006: Implement focus state tracking to prevent edit conflicts
**Domain:** weave
**Status:** pending
**Depends on:** 004

Add JavaScript to track focus state on prompt textarea. While focused, queue or ignore incoming prompt-update SSE events. On blur, discard queued updates (user edit takes precedence). This prevents jarring text changes while typing.

**Files to modify:**
- `internal/web/templates/index.html`

**Testing:** Manual test. Type in prompt while agent is streaming. Verify text doesn't change mid-typing. Blur, verify user's edit is preserved.

---

### 007: Implement generate button and flow
**Domain:** weave
**Status:** pending
**Depends on:** 005

Add Generate button in UI (prompt pane or chat). Use hx-post="/generate". Disable button during generation. In server.go, implement POST /generate handler that gets conversation manager, reads current prompt, calls compute client (Story 002) with prompt and params, receives image response, stores image (Story 009), sends image-ready SSE event with URL.

**Files to modify:**
- `internal/web/templates/index.html`
- `internal/web/server.go`
- `internal/web/server_test.go`

**Testing:** Integration test triggers generate, verifies image-ready event sent. Manual test: click Generate, verify button disables, image appears.

---

### 008: Implement inline image display in chat
**Domain:** weave
**Status:** pending
**Depends on:** 007

Update HTMX SSE handling for image-ready events. On event, append img tag to chat messages at correct position (after current conversation). Image loads from URL in event payload. Style for reasonable size (fixed or max-width).

**Files to modify:**
- `internal/web/templates/index.html`
- `internal/web/static/style.css`

**Testing:** Manual test. Generate image, verify it appears inline in chat at correct position.

---

### 009: Implement progress indicator during generation
**Domain:** weave
**Status:** pending
**Depends on:** 007

Add spinner or text indicator shown while generation is in progress. Show after clicking Generate, hide when image-ready or error event arrives. Position in chat or near Generate button.

**Files to modify:**
- `internal/web/templates/index.html`
- `internal/web/static/style.css`

**Testing:** Manual test. Click Generate, verify progress indicator appears and disappears on completion.

---

### 010: Implement error handling and display
**Domain:** weave
**Status:** pending
**Depends on:** 003, 007

In server.go, catch errors from ollama and compute clients. Send SSE error events with error messages. In HTML, handle error events by displaying error message in chat (red text or styled box). Errors are dismissible or auto-clear on next action.

**Files to modify:**
- `internal/web/templates/index.html`
- `internal/web/server.go`
- `internal/web/static/style.css`

**Testing:** Unit tests for error event sending. Manual test: trigger errors (ollama down, compute down), verify error messages appear in UI.

---

### 011: Integration test for full user flow
**Domain:** weave
**Status:** pending
**Depends on:** 003, 007, 010

Create integration test that simulates full flow: send chat message, receive streaming response, prompt updates, edit prompt, generate image, image appears. Verify all SSE events fire correctly and conversation state is maintained.

**Files to create:**
- `internal/web/full_flow_test.go` (tagged integration)

**Testing:** Integration test passes. Verifies end-to-end flow with all components.

---

### 012: Update DEVELOPMENT.md with UI walkthrough
**Domain:** documentation
**Status:** pending
**Depends on:** 011

Add section walking through user interaction flow: send message, see streaming response, edit prompt, generate image. Explain how to test each feature. Include screenshots or ASCII diagrams if helpful.

**Files to modify:**
- `docs/DEVELOPMENT.md`

**Verification:** Documentation is clear. Walkthrough is accurate.
