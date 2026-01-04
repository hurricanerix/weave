# Story 006: Web UI Foundation

## Problem

The Go application needs to serve a web interface and push real-time updates to the browser. This requires an HTTP server, HTMX for dynamic updates, and SSE (Server-Sent Events) for streaming agent responses and prompt updates. This story establishes the skeleton that feature stories build on.

## User/Actor

- End user (needs a browser-based interface)
- Weave developer (implementing web server and SSE)

## Desired Outcome

A working web foundation where:
- Go serves static HTML with HTMX
- Browser connects to SSE endpoint for real-time updates
- Split-screen layout renders (left pane, right pane)
- SSE can push updates that HTMX swaps into the DOM
- Foundation is ready for chat and prompt features

## Acceptance Criteria

### HTTP Server

- [ ] Server listens on `localhost:8080` (hardcoded for MVP)
- [ ] Server serves index page at `/`
- [ ] Server handles static assets (CSS, JS if needed)
- [ ] Server provides SSE endpoint at `/events`
- [ ] Server provides API endpoints (placeholders for now):
  - `POST /chat` - Send user message
  - `POST /prompt` - Update prompt (user edit)
  - `POST /generate` - Trigger image generation
- [ ] Graceful shutdown on SIGTERM/SIGINT

### HTML Structure

- [ ] Single page application (index.html)
- [ ] HTMX loaded (CDN or bundled)
- [ ] Split-screen layout with CSS:
  - Left pane: chat area (will hold messages)
  - Right pane: prompt area (will hold editable prompt)
- [ ] Responsive within reason (works on desktop, mobile deferred)
- [ ] Minimal styling (functional, not beautiful—MVP)

### SSE (Server-Sent Events)

- [ ] `/events` endpoint returns `text/event-stream`
- [ ] Connection stays open for streaming
- [ ] Server can push events with named event types:
  - `agent-token` - Single token from streaming response
  - `agent-done` - Agent response complete
  - `prompt-update` - Prompt has changed
  - `image-ready` - Generated image available
  - `error` - Error occurred
- [ ] Events include data payload (JSON)
- [ ] Client reconnects automatically on disconnect (HTMX/EventSource default)
- [ ] Each browser session has its own SSE connection
- [ ] SSE connection tied to session ID (same as conversation manager)

### HTMX Integration

- [ ] HTMX `hx-sse` extension loaded for SSE support
- [ ] SSE events trigger DOM swaps via HTMX
- [ ] Example: `agent-token` event appends to agent message div
- [ ] Example: `prompt-update` event replaces prompt textarea content
- [ ] Forms use `hx-post` for AJAX submission

### Session Handling

- [ ] Session ID generated on first visit (cookie or URL param)
- [ ] Session ID sent with all requests
- [ ] Session ID used to route SSE events to correct connection
- [ ] Session ID links to conversation manager state (from Story 005)

### Testing

- [ ] Integration test: server starts and serves index page
- [ ] Integration test: SSE connection established successfully
- [ ] Integration test: SSE event sent from server appears in browser (manual or headless)
- [ ] Integration test: session ID persists across page reload
- [ ] Unit test: SSE event formatting is correct
- [ ] Unit test: session ID generation works

### Documentation

- [ ] `docs/DEVELOPMENT.md` explains how to run the web server
- [ ] `docs/DEVELOPMENT.md` explains how to access UI in browser
- [ ] Code comments explain SSE event types and payloads

## Out of Scope

- Chat message display (Story 007)
- Prompt editing and blur detection (Story 007)
- Image display (Story 007)
- Generate button functionality (Story 007)
- Beautiful CSS/styling
- Mobile responsiveness
- HTTPS/TLS
- Authentication beyond session ID

## Dependencies

- Story 005: Conversation Manager (session links to conversation state)

## Notes

This is the skeleton. After this story, the browser shows an empty split-screen layout and can receive SSE events. The next story fills in the actual chat and prompt functionality.

HTMX's SSE extension (`hx-sse`) handles most of the client-side complexity. The server just needs to format events correctly:

```
event: agent-token
data: {"token": "Hello"}

event: prompt-update
data: {"prompt": "a cat wearing a hat"}
```

Keep the HTML minimal. A few divs with IDs that HTMX can target:
- `#chat-messages` - Where messages appear
- `#chat-input` - User input form
- `#prompt-field` - Editable prompt textarea
- `#current-image` - Where generated image appears (if any)

CSS can be inline or a single file. Flexbox for the split layout. Nothing fancy.

## Tasks

### 001: Create HTTP server with basic routes
**Domain:** weave
**Status:** done
**Depends on:** none

Create `internal/web/server.go` with Server type. Implement ListenAndServe() on localhost:8080. Define routes: GET / (index page), GET /events (SSE), POST /chat, POST /prompt, POST /generate (placeholder handlers). Handle graceful shutdown on context cancellation.

**Files to create:**
- `internal/web/server.go`
- `internal/web/server_test.go`

**Testing:** Unit test verifies routes registered. Integration test verifies server starts and responds.

---

### 002: Implement SSE endpoint infrastructure
**Domain:** weave
**Status:** pending
**Depends on:** 001

In server.go, implement /events handler that sets Content-Type: text/event-stream, disables buffering, keeps connection open. Create SSE broker that manages connections per session ID. Implement SendEvent(sessionID, eventType, data) for named events (agent-token, prompt-update, image-ready, error).

**Files to create:**
- `internal/web/sse.go`
- `internal/web/sse_test.go`

**Testing:** Unit tests verify event formatting (event: type, data: json). Integration test connects to SSE, receives test event.

---

### 003: Implement session ID generation and cookie management
**Domain:** weave
**Status:** done
**Depends on:** 001

Create `internal/web/session.go` with function to generate session ID (UUID). On first request without session cookie, generate ID and set cookie. Include session ID in request context for handlers. Cookie has reasonable expiry (24 hours) but no cleanup for MVP.

**Files to create:**
- `internal/web/session.go`
- `internal/web/session_test.go`

**Testing:** Unit tests verify ID generation, cookie setting. Integration test verifies session persists across requests.

---

### 004: Create HTML template with split-screen layout
**Domain:** weave
**Status:** pending
**Depends on:** none

Create `internal/web/templates/index.html` with HTMX loaded from CDN, split-screen layout (left: chat pane, right: prompt pane). Include divs with IDs: chat-messages, chat-input, prompt-field, current-image. Minimal CSS for flexbox layout. Load HTMX SSE extension.

**Files to create:**
- `internal/web/templates/index.html`
- `internal/web/static/style.css` (if external CSS used)

**Testing:** Manual verification in browser. Layout renders correctly.

---

### 005: Wire SSE to HTMX in HTML
**Domain:** weave
**Status:** done
**Depends on:** 002, 004

In index.html, add hx-sse attributes to connect to /events endpoint. Define swap targets for each event type (agent-token -> append to message, prompt-update -> replace prompt field, etc.). Verify HTMX can receive and process SSE events.

**Files to modify:**
- `internal/web/templates/index.html`

**Testing:** Manual test in browser. Send test SSE event, verify HTMX swaps content correctly.

---

### 006: Implement placeholder API handlers
**Domain:** weave
**Status:** pending
**Depends on:** 001, 003

Implement stub handlers for POST /chat, POST /prompt, POST /generate. Each returns 200 OK with JSON response. Extract session ID from context. Log requests at DEBUG level. No actual processing yet (wiring happens in Story 007).

**Files to modify:**
- `internal/web/server.go`
- `internal/web/server_test.go`

**Testing:** Unit tests verify handlers respond correctly. Integration test posts to each endpoint, receives 200.

---

### 007: Integration test for SSE + session flow
**Domain:** weave
**Status:** done
**Depends on:** 002, 003, 006

Create integration test that generates session, connects to SSE endpoint with session ID, sends test event to that session, verifies event received. Test session isolation (events only go to correct session).

**Files to create:**
- `internal/web/integration_test.go`

**Testing:** Integration test passes. Verify session isolation, SSE event delivery.

---

### 008: Update DEVELOPMENT.md with web server instructions
**Domain:** documentation
**Status:** done
**Depends on:** 007

Add section explaining how to run web server, access in browser (http://localhost:8080), view browser console for debugging. Explain session cookie mechanism.

**Files to modify:**
- `docs/DEVELOPMENT.md`

**Verification:** Documentation is clear. Instructions work.
