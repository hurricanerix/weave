# Story: Enhanced Visual Feedback for Two-Stage Generation

## Status
Ready

## Problem
With the two-stage LLM flow, there are now distinct phases during response generation. Users need visual feedback that something is happening at each phase, especially when image generation and conversation response happen in parallel.

Currently, the thinking indicator doesn't communicate progress through these stages, leaving users uncertain about what's happening.

## User/Actor
End user watching Ara respond to their image generation request.

## Desired Outcome
Users see smooth visual transitions that communicate progress:
1. Small thinking indicator when request is sent
2. Expanded bubble when extraction completes (signals progress)
3. Mist animation in preview area when image generation starts
4. Text streaming in while image generates
5. Image appearing when generation completes

The experience feels responsive even when operations take time.

## Acceptance Criteria
- [ ] When user sends a message, small thinking indicator (3 dots) appears immediately
- [ ] When extraction stage completes, bubble expands to full size with thinking indicator in text area
- [ ] When image generation starts, preview area shows mist animation
- [ ] When conversation response begins streaming, thinking dots clear and text appears incrementally
- [ ] When image completes, it replaces the mist animation in preview area
- [ ] When no image is being generated, bubble still expands after extraction (no preview area shown)
- [ ] Text can finish streaming before or after image completes - both orderings work correctly

## Out of Scope
- Backend LLM orchestration (Story 027)
- New SSE event types beyond modifying `agent-thinking`
- Cancellation UI

## Dependencies
- Story 027 (Two-Stage LLM Flow) must be implemented first

## Open Questions
None.

## Notes
Technical approach: Extend `agent-thinking` event with an `expanded` boolean parameter:
- `agent-thinking { expanded: false }` - show small thinking bubble
- `agent-thinking { expanded: true }` - expand to full bubble, keep thinking dots in text area

The `generation-started` event triggers adding the preview area with mist animation.

Existing events (`agent-token`, `image-ready`, `agent-done`) continue to work as-is.

## Tasks

### Task 001: Add CSS for mist animation in preview area
**Domain:** electron
**Status:** pending

Add new `@keyframes mist` animation using existing shimmer/pulse patterns. Style `.message-preview[data-status="generating"]` with mist animation instead of current `loading.webp` background. CSS-only solution, no new image files.

**Files:**
- `backend/internal/web/templates/index.html` (embedded CSS)

**Test:** Preview area shows animated mist effect when generation is in progress.

---

### Task 002: Add CSS for expanded thinking state
**Domain:** electron
**Status:** pending

Add `.thinking-bubble.expanded` styles for full-width bubble with text area appearance. Ensure thinking dots display correctly inside expanded bubble (centered in text area).

**Files:**
- `backend/internal/web/templates/index.html` (embedded CSS)

**Test:** Expanded thinking bubble has proper dimensions and thinking dots are visible.

---

### Task 003: Update handleAgentThinking for expanded parameter
**Domain:** electron
**Status:** pending

Modify `handleAgentThinking()` to check `data.expanded` boolean:
- When `expanded: false` (or missing): show small thinking bubble (current behavior)
- When `expanded: true`: expand existing bubble to full size, keep dots in text area

Handle both initial event and transition from small to expanded state.

**Files:**
- `backend/internal/web/templates/index.html` (JavaScript)

**Test:** Small bubble appears initially, expands when second event with `expanded: true` arrives.

---

### Task 004: Update handleGenerationStarted to create preview area
**Domain:** electron
**Status:** pending

Move preview area creation from `handleAgentDone` to `handleGenerationStarted`. When `generation-started` fires with `message_id`:
- Find the message bubble (or the thinking bubble if tokens haven't arrived yet)
- Create preview area with mist animation
- Set `data-status="generating"`

**Files:**
- `backend/internal/web/templates/index.html` (JavaScript)

**Test:** Preview area appears when generation starts, even while text is still streaming.

---

### Task 005: Update handleAgentDone to not create preview area
**Domain:** electron
**Status:** pending

Remove preview area creation logic from `handleAgentDone`. The `has_snapshot` field may still be useful for other purposes but should NOT trigger preview creation. Preview creation is now handled by `handleGenerationStarted`.

**Files:**
- `backend/internal/web/templates/index.html` (JavaScript)

**Test:** `agent-done` event no longer creates preview areas.

---

### Task 006: Handle agent-token transition from expanded thinking state
**Domain:** electron
**Status:** pending

Update `handleAgentToken` to detect expanded thinking state and handle transition:
- If message bubble doesn't exist, convert thinking bubble to message bubble (existing logic)
- Clear thinking dots from text area
- Start streaming text content in place

Ensure smooth transition with no visual glitches.

**Files:**
- `backend/internal/web/templates/index.html` (JavaScript)

**Test:** Text appears smoothly in place of thinking dots, no duplicate elements.
