# Story: UI - live updates (SSE)

## Status
In Progress

## Problem
When a job is running, the human needs to see journal entries appear in real time and status badges update across the job list without manually refreshing the page. Without live updates, the human must repeatedly refresh to check progress, which defeats the purpose of the journal streaming design.

## User/Actor
- Human (watches running jobs in the browser)

## Desired Outcome
SSE-powered live updates for journal streaming and status changes. The human sees the agent's work appear in real time and job statuses update across the UI automatically.

## Acceptance Criteria
- [ ] Single SSE connection per browser session
- [ ] Journal entries appear in the Journal tab in real time as they arrive from l5-relay
- [ ] Status badges in the left column update when a job transitions (pending to running, running to complete)
- [ ] Outcome badges appear on complete jobs without page refresh
- [ ] SSE reconnects automatically if connection drops
- [ ] Multiple browser tabs or sessions receive updates independently
- [ ] SSE uses HTMX SSE extension for DOM updates
- [ ] Journal entries are appended to the display, not replaced (streaming append)

## Out of Scope
- Journal entry filtering or search
- Notification sounds or desktop notifications
- WebSocket support (SSE is sufficient for server-to-client streaming)

## Dependencies
- Story 039 (UI job management) -- the UI must exist for SSE to update
- Story 035 (orchestrator skeleton) -- SSE endpoint is served by the orchestrator

## Open Questions
None.

## Notes
See `l5/phase1_details.md` "Live updates (SSE)" section. One SSE stream carries two event types: journal entries (for the detail panel) and status changes (for the job list). HTMX SSE extension routes events to the correct DOM elements based on event type.

## Tasks

### 001: SSE broker with broadcast delivery
**Domain:** l5
**Status:** pending
**Depends on:** none

Create `l5/internal/sse/` package. Implement a broadcast `Broker` struct that manages SSE connections: `ServeHTTP` handler for `GET /events` that sets SSE headers (`Content-Type: text/event-stream`, `Cache-Control: no-cache`, `Connection: keep-alive`), disables write deadline, verifies flush support, and holds the connection open until client disconnects (`request.Context().Done()`). Implement `AddConnection`/`RemoveConnection` with a `sync.RWMutex`-protected slice of connections. Implement `Broadcast(eventType string, htmlData string)` that writes `event: {type}\ndata: {html}\n\n` to all connected clients and flushes. Each connection tracks its own `done` channel. The broker supports multiple independent connections (multiple browser tabs). Add `Shutdown` method that closes all connections. Unit test: verify event formatting matches SSE spec, verify broadcast reaches multiple connections.

---

### 002: Emit SSE events from agent API and job lifecycle
**Domain:** l5
**Status:** pending
**Depends on:** 001

Wire the SSE broker into the existing agent API handlers and job lifecycle. In the `POST /api/agent/journal` handler (story 035): after storing the journal entry, render it as an HTML fragment using the journal entry template and broadcast as a `journal-entry` SSE event. In the `POST /api/agent/complete` handler: after updating job status, re-render the job list HTML and broadcast as a `job-status` SSE event. In the job runner (story 038): when `StartJob` transitions a job to running, broadcast a `job-status` SSE event with the updated job list HTML. Pass the SSE broker to the API handlers and job runner via dependency injection (constructor parameter or server field). The broker must be the same instance shared across all components.

---

### 003: Wire HTMX SSE extension into UI templates
**Domain:** l5
**Status:** pending
**Depends on:** 001, 002

Update the base layout template (story 039) to include the HTMX SSE extension script (`sse.js` from HTMX extensions CDN). Add `hx-ext="sse"` and `sse-connect="/events"` to the layout's main container so a single SSE connection is established per browser tab. In the job list template: add `sse-swap="job-status"` with `hx-swap="innerHTML"` on the job list container so status badge updates replace the list contents. In the journal tab template: add `sse-swap="journal-entry"` with `hx-swap="beforeend"` on the journal entries container so new entries are appended (streaming append, not replaced). Verify: SSE connection establishes on page load, journal entries appear in real time when viewing a running job's Journal tab, job list badges update when a job transitions status, opening a second browser tab establishes an independent SSE connection. SSE reconnects automatically on connection drop (HTMX SSE extension handles this by default).

---
