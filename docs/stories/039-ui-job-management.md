# Story: UI - job management

## Status
In Progress

## Problem
The human needs a web interface to create jobs, write specs, provide starting patches, view job status, and see results (journal entries, output patches). Without a UI, the orchestrator is only usable via API calls.

## User/Actor
- Human (creates and manages jobs through the browser)

## Desired Outcome
An HTMX-based web interface with a job list on the left and a tabbed detail panel on the right. The human can create, edit, run, cancel, and delete jobs entirely through the browser.

## Acceptance Criteria
- [ ] Left column displays all jobs with status badges (pending, running, complete) and outcome indicators on complete jobs
- [ ] Clicking a job shows its detail in the right panel
- [ ] "New Job" action creates a pending job and selects it
- [ ] Pending jobs show: editable Spec tab, editable Patch Input tab, Save/Delete/Run buttons
- [ ] Running jobs show: read-only Spec tab, read-only Patch Input tab, Journal tab, Cancel button
- [ ] Complete jobs show: read-only Spec tab, read-only Patch Input tab, Journal tab, Patch Output tab (if available), Delete button
- [ ] Save button persists spec text and patch input to the database
- [ ] Delete button removes the job from the database and the list
- [ ] Run button triggers workspace assembly and container start, job transitions to running
- [ ] Cancel button sends SIGTERM to the running container
- [ ] Journal tab displays stored journal entries for the job
- [ ] Patch Output tab displays the result diff or an empty state message
- [ ] Domain selection is available when creating a job (populated from project config)
- [ ] UI is server-rendered HTML with HTMX for interactivity, no JavaScript framework

## Out of Scope
- Live SSE updates (that is story 040 -- this story uses page refresh or HTMX polling for status)
- Rich patch viewer (syntax highlighting, etc.) -- plain text diff display is sufficient
- User authentication (single-user, local access only in phase 1)

## Dependencies
- Story 035 (orchestrator skeleton) -- UI is served by the orchestrator
- Story 038 (container lifecycle) -- Run and Cancel actions trigger lifecycle operations

## Open Questions
None.

## Notes
See `l5/phase1_details.md` "UI" section. Complete jobs display the same tabs regardless of outcome -- the outcome badge communicates what happened. HTMX handles form submissions, tab switching, and list updates. Go `html/template` for rendering.

## Tasks

### 001: Template infrastructure and base layout
**Domain:** l5
**Status:** pending
**Depends on:** none

Create `l5/web/templates/` directory for Go HTML templates and `l5/web/static/` for static assets. Create the base layout template (`layout.html`) with: HTML skeleton, HTMX script tag (CDN link is fine for phase 1), minimal CSS for a two-column layout (left column ~250px for job list, right panel fills remaining width). Add a `GET /` route to the orchestrator server that renders the base layout with the job list in the left column and an empty state message in the right panel. Set up Go `html/template` parsing with a template registry that the server loads on startup. Include HTMX attributes on the layout so the left column and right panel can be independently updated.

---

### 002: Job list with status badges and create action
**Domain:** l5
**Status:** pending
**Depends on:** 001

Create the job list template (`job_list.html`) rendered in the left column. Each job entry shows: job ID or short label, domain name, status badge (pending/running/complete), and outcome indicator for complete jobs (success/error/timeout/empty/killed). Clicking a job uses `hx-get="/jobs/{id}"` with `hx-target="#detail-panel"` to load its detail in the right panel. Add a "New Job" button at the top that uses `hx-post="/jobs"` to create a pending job. The POST handler creates a job in the store, returns an HTMX response that refreshes the job list and loads the new job's detail panel. Domain selection (dropdown populated from project config domains) is included in the "New Job" form. The list orders jobs by creation time, newest first.

---

### 003: Job detail panel with status-dependent tabs
**Domain:** l5
**Status:** pending
**Depends on:** 001

Create the job detail template (`job_detail.html`) rendered in the right panel. Implement tab navigation using HTMX: each tab header uses `hx-get="/jobs/{id}/tab/{name}"` with `hx-target="#tab-content"` to load tab content. Tabs displayed depend on job status per the design doc: pending shows Spec and Patch Input; running adds Journal; complete adds Patch Output. Spec tab renders the spec text in a `<textarea>` (editable) for pending jobs or a `<pre>` (read-only) for running/complete. Patch Input tab follows the same editable/read-only pattern. Journal tab renders stored journal entries in chronological order as a scrollable list. Patch Output tab renders the result diff in a `<pre>` block, or an empty state message if no patch. Add `GET /jobs/{id}` route that returns the detail panel HTML and `GET /jobs/{id}/tab/{name}` route that returns individual tab content.

---

### 004: Save and delete routes
**Domain:** l5
**Status:** pending
**Depends on:** 002, 003

Implement `PUT /jobs/{id}` handler for the Save action: reads spec text and patch input from the form body, updates the job in the store, returns HTMX response that shows a brief "Saved" confirmation (can be an `hx-swap="none"` with an `HX-Trigger` header, or a re-render of the detail panel). Implement `DELETE /jobs/{id}` handler: removes the job from the store, returns HTMX response that refreshes the job list and clears the detail panel. Wire Save button (visible on pending jobs) to `hx-put` and Delete button (visible on pending and complete jobs) to `hx-delete`. Both buttons include appropriate `hx-target` and `hx-swap` attributes.

---

### 005: Run and cancel action handlers
**Domain:** l5
**Status:** pending
**Depends on:** 003, 004

Implement `POST /jobs/{id}/run` handler: validates the job is in pending status, calls `JobRunner.StartJob` (from story 038) to assemble workspace and start the container, returns HTMX response that refreshes the job list (status badge updates to running) and reloads the detail panel (tabs switch to running view with Journal tab). Implement `POST /jobs/{id}/cancel` handler: validates the job is in running status, calls `JobRunner.CancelJob` to send SIGTERM, returns HTMX response that indicates cancellation is in progress (the job will transition to complete via the wrapper's POST to `/api/agent/complete`). Wire Run button (pending jobs) to `hx-post` and Cancel button (running jobs) to `hx-post`. Add a confirmation prompt on Cancel using `hx-confirm="Cancel this job?"`.

---
