# Story: App shell layout with three-panel structure

## Status
Complete

## Problem
The current web UI uses a basic split-screen layout (chat left, prompt right) that looks like a developer prototype. Users see a functional but unprofessional interface that does not convey quality or polish. There is no visual hierarchy, no app chrome, and no sense of structure. Users who want to create professional creative work expect an application that matches their workflow, not a debug console.

The split-screen layout also does not scale for the features being added. As more controls and panels are introduced, the current flat layout becomes cramped and confusing. Users need a proper application structure with collapsible panels, dedicated spaces for different concerns, and responsive behavior.

## User/Actor
Users of the Weave web interface who want a professional creative tool, not a prototype. This includes both novice users exploring image generation and power users who expect polished software for their creative work.

## Desired Outcome
Users open the Weave web interface and see a professional application with clear visual structure. The interface has an app header with branding and controls, a collapsible left sidebar for session history, a central workspace split between chat and image display, and a collapsible right panel for settings. Users can toggle the sidebar and settings panel to focus on their work. The layout feels intentional, polished, and appropriate for a creative tool.

## Acceptance Criteria
- [ ] App header is visible at the top of the page with fixed height
- [ ] Header contains sidebar toggle button on the left side
- [ ] Header displays "Weave" branding using MarckScript font in the center
- [ ] Header contains settings toggle button on the right side
- [ ] Left sidebar is visible by default, showing placeholder "Current Session" item
- [ ] Sidebar toggle button collapses/expands the left sidebar
- [ ] When sidebar is collapsed, it slides out of view and main content expands
- [ ] Main workspace is divided into three sections: chat panel, image panel, settings panel
- [ ] Chat panel is on the left side of the workspace (after sidebar)
- [ ] Image panel is in the center of the workspace, taking most horizontal space
- [ ] Settings panel is on the right side of the workspace
- [ ] Settings panel is collapsed by default (hidden off-screen to the right)
- [ ] Settings toggle button expands/collapses the settings panel
- [ ] When settings panel opens, it slides in from the right
- [ ] When settings panel closes, it slides out to the right
- [ ] Layout uses flexbox for responsive panel sizing
- [ ] All panels are scrollable independently when content overflows
- [ ] Existing chat and generation functionality continues to work with new layout
- [ ] No visual regressions in existing features (messages, input, generation)

## Out of Scope
- Resizable panels with drag handles (nice to have, not MVP)
- Multiple session items in history sidebar (placeholder only for MVP)
- App settings modal content (deferred)
- Mobile responsive breakpoints (basic responsive behavior only)
- Thumbnail grids in messages (separate story)
- Progress indicators (separate story)
- Jump-to-end button (separate story)
- Enhanced message bubbles (separate story - this story keeps existing message display)
- Image overlay actions (separate story - this story just displays the image)
- Settings panel content (separate story - this story creates empty panel structure)

## Dependencies
- Story 006: Web UI foundation (HTTP server, basic HTML)
- Story 007: Chat prompt panes (existing chat display)
- Story 011: Generation settings UI controls (existing settings)
- Story 012: Agent-triggered generation (SSE events)

## Open Questions
None.

## Notes
This story focuses purely on the structural layout and toggling behavior. The content of each panel (enhanced chat messages, image display with actions, settings form) is handled in subsequent stories. After this story, the UI will have the correct structure but the panels will contain the existing basic content.

The sidebar shows a single "Current Session" placeholder item for MVP. Future stories will add multiple session support and history management.

The MarckScript font is referenced in the header. Story 014 will import the actual font file and CSS.

Existing JavaScript for SSE, chat submission, and generation continues to work. This story only restructures the HTML and adds toggle button handlers.

## Tasks

### 001: Add app header with branding and toggle buttons
**Domain:** weave
**Status:** done
**Depends on:** none

Add the `.app-header` structure with sidebar toggle, "Weave" title text, and settings toggle button. The header uses flexbox with fixed 48px height, positioned at the top of the page. Toggle buttons use `.icon-btn` class with SVG icons for panel toggles. The title uses MarckScript font family reference (font file will be added in story 014). Structure follows demo.html lines 1318-1332.

---

### 002: Create sidebar with placeholder session item
**Domain:** weave
**Status:** done
**Depends on:** 001

Add the `.sidebar` element with `.sidebar-content` containing a single `.conversation-item` placeholder labeled "Current Session". The sidebar is 280px wide, uses `.collapsed` class for toggle state, and transitions via `margin-left: calc(-1 * var(--sidebar-width))` when collapsed. Add data attribute `id="sidebar"` for JavaScript targeting. Structure follows demo.html lines 1339-1407.

---

### 003: Restructure main workspace into three sections
**Domain:** weave
**Status:** done
**Depends on:** 002

Wrap the existing chat pane content (messages container + input) in a `.chat-panel` div. Move the current image display into an `.image-panel` div. Create an empty `.settings-panel` div. Wrap all three in a `.main` container using flexbox. The chat panel is fixed width (400px), the image panel takes remaining space with `flex: 1`, and the settings panel is 360px wide but hidden by default with `margin-right: -360px` and `visibility: hidden`. Structure follows demo.html lines 1409-1670.

---

### 004: Move generation settings into settings panel
**Domain:** weave
**Status:** pending
**Depends on:** 003

Move the existing steps/cfg/seed inputs from the current inline location into the `.settings-panel` structure. Wrap inputs in `.settings-section` with title "Generation Settings". Keep the same input IDs and form structure so HTMX and SSE handlers continue to work. The prompt field stays in its current location for now. Settings panel structure follows demo.html lines 1578-1670 (adapt to keep existing settings inputs).

---

### 005: Add toggle button JavaScript handlers
**Domain:** weave
**Status:** done
**Depends on:** 004

Add `toggleSidebar()` function that toggles the `.collapsed` class on the `#sidebar` element. Add `toggleSettings()` function that toggles the `.open` class on the `#settings-panel` element and updates `margin-right` style (0 when open, negative when closed). Wire these functions to the header button `onclick` attributes. Verify smooth CSS transitions without layout jumps. Implementation follows demo.html lines 1753-1773.

---

### 006: Add minimal layout CSS for positioning and transitions
**Domain:** weave
**Status:** pending
**Depends on:** 005

Add CSS rules for `.app`, `.app-header`, `.app-body`, `.main`, `.sidebar`, `.chat-panel`, `.image-panel`, `.settings-panel` positioning. Include transition properties for smooth collapse/expand (`transition: margin-left 200ms ease`, `transition: margin-right 200ms ease`). Use CSS custom properties `--sidebar-width: 280px` and `--header-height: 48px` for maintainability. Keep existing message and input styling intact. CSS structure follows demo.html lines 188-896.

---

### 007: Verify existing functionality with new layout
**Domain:** weave
**Status:** pending
**Depends on:** 006

Manual testing to ensure chat submission, SSE events (agent-token, prompt-update, image-ready, error), generation button click, and all existing features work correctly with the new HTML structure. Verify HTMX includes target correct element IDs. Check that chat auto-scrolls, prompt saves on blur, and settings updates apply. Test sidebar and settings panel toggling does not break event handlers or state management. No automated tests needed - this is visual and integration verification.

Layout CSS will be minimal in this story - just enough to position panels correctly. Story 014 will import the full CSS component library including colors, typography, spacing, and dark mode.
