# Story: Sidebar defaults to closed on page load

## Status
In Progress

## Problem
When users open the web UI, the history sidebar is open by default. This takes up valuable screen space and distracts from the primary focus: the current conversation and generated image. Users who need to see past conversations can easily open the sidebar, but by default the interface should prioritize the current work.

## User/Actor
Web UI user opening the application or starting a new session.

## Desired Outcome
When the web UI loads, the history sidebar is closed by default. Users see more screen space for the chat conversation and image display. The sidebar toggle button remains visible and functional, so users can open the sidebar when they want to view conversation history.

## Acceptance Criteria
- [ ] Sidebar is visually collapsed when page first loads
- [ ] Sidebar toggle button in header remains visible and functional
- [ ] Clicking toggle button opens the sidebar as expected
- [ ] Closing and reopening the sidebar still works correctly
- [ ] On mobile/tablet (responsive view), sidebar remains closed by default
- [ ] No JavaScript errors in console related to sidebar initialization

## Out of Scope
- Remembering sidebar state across page reloads (localStorage persistence)
- Adding keyboard shortcuts for sidebar toggle
- Animation timing or transition effects (keep existing behavior)
- Changing sidebar width or responsive breakpoints

## Dependencies
None

## Open Questions
None

## Notes
Current implementation in `internal/web/templates/index.html`:
- Sidebar HTML: lines 1571-1587
- Toggle function: lines 2503-2508
- CSS for `.sidebar.collapsed`: lines 334-337

The fix is straightforward: add the `collapsed` class to the sidebar element's initial class list in the HTML.

## Tasks

### 001: Set sidebar to collapsed by default
**Domain:** weave
**Status:** pending
**Depends on:** none

Update the sidebar HTML element in `internal/web/templates/index.html` (line 1571) to include the `collapsed` class in its initial class attribute. Change `<aside class="sidebar" id="sidebar">` to `<aside class="sidebar collapsed" id="sidebar">`. Verify the change by starting the web server and testing in browser: (1) sidebar should be visually collapsed on page load, (2) toggle button should open/close sidebar correctly, (3) no JavaScript console errors should appear. Test at desktop and mobile viewport sizes.

---
