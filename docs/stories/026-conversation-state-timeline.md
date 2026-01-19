# Story: Conversation State Timeline

## Status
Done

## Problem
Today, the conversation is a message log with a single global "current prompt." When Ara updates the prompt, the previous state is lost. Users cannot:
- Scroll back to see what the prompt was at any point in history
- Click a past response to load that state into the input
- See which responses changed the prompt or generated images
- Resume a conversation after restarting the app (sessions are memory-only)

This makes the conversation feel ephemeral rather than a useful timeline of creative exploration.

## User/Actor
A user chatting with Ara to iteratively refine image generation prompts.

## Desired Outcome
Each assistant response that changes the prompt or settings captures a snapshot of that state. Users can scroll through their conversation history and see a visual indicator (preview bubble) on responses that changed generation state. They can click to load historical state, generate a preview for any past prompt, and pick up where they left off after restarting the app.

## Acceptance Criteria

### State-per-message
- [ ] Assistant messages that change prompt or settings store a snapshot (prompt, steps, cfg, seed)
- [ ] Messages that do not change prompt/settings have no snapshot (pure conversation)
- [ ] Each message has a stable integer ID within its session
- [ ] Clicking a message with a snapshot loads that prompt/settings into the input area

### Preview bubble component
- [ ] Messages with snapshots display a preview area in the top-right corner of the bubble
- [ ] Text streams into the bubble and wraps, flowing behind the preview area when needed
- [ ] Preview area is square with three size options: 64x64, 96x96, 128x128 (global user preference, default: medium/96x96)
- [ ] Vertical divider line separates preview area from text (matches preview height only)
- [ ] Preview states:
  - Placeholder icon: snapshot exists but no image generated yet (reuse existing empty-state icon from main preview panel)
  - Mist animation: generation in progress
  - Final image: generation complete
- [ ] Transitions between preview states use cross-fade (300-500ms)
- [ ] Click on preview area opens detail panel with full-size image
- [ ] Click and drag on text area selects text (standard selection behavior)
- [ ] Text area has right padding equal to preview width + margin to prevent text hiding behind preview on short responses

### Session persistence
- [ ] Sessions persist to `config/sessions/{session_id}/`
- [ ] Conversation data (messages with snapshots) saved to JSON file
- [ ] Preview images saved to `images/` subdirectory, keyed by message ID
- [ ] Session survives app restart - reopening app restores conversation state
- [ ] Session ID is a simple incrementing integer (or reuse existing if resuming)

### Generate for historical message
- [ ] User can select a message with a snapshot (loads its state)
- [ ] User can click Generate to create a preview using that message's prompt/settings
- [ ] Generated image is saved and associated with that message
- [ ] Preview bubble updates from placeholder to mist to final image

### Thinking indicator transition
- [ ] When Ara is processing, show "..." thinking bubble
- [ ] When response starts streaming, thinking bubble animates/expands into response bubble
- [ ] If response will have a preview, preview area appears with appropriate initial state

## Out of Scope
- History pane UI for browsing/loading past sessions (separate story)
- Intermediate generation previews during diffusion steps (deferred - mist until final only)
- Session branching/forking (linear conversation only)
- Session cloning
- Automatic session cleanup or size limits

## Dependencies
None - this enhances the existing conversation system.

## Open Questions
None - all questions resolved during story definition.

## Tasks

### 001: Extend message model with IDs and state snapshots
**Domain:** backend
**Status:** done
**Depends on:** none

Add `ID int` and `StateSnapshot *Snapshot` fields to the message type. Create `Snapshot` struct with `Prompt string`, `Steps int`, `CFG float64`, `Seed int64`, and `PreviewStatus string` (none/generating/complete), `PreviewURL string`. Update `ollama.Message` or create a wrapper type in the conversation package that includes these fields.

---

### 002: Update conversation manager for state tracking
**Domain:** backend
**Status:** done
**Depends on:** 001

Modify `Manager.AddAssistantMessage` to accept optional `LLMMetadata`. When metadata is provided, compare against previous snapshot state. If prompt or settings differ, create and attach a `StateSnapshot` to the message. Assign sequential integer IDs to all messages. Track "previous state" to enable comparison.

---

### 003: Add session persistence layer
**Domain:** backend
**Status:** done
**Depends on:** 001, 002

Create persistence package or extend session manager to serialize conversation state to `config/sessions/{session_id}/conversation.json`. Include all messages with IDs, snapshots, and preview status. Implement `Save()` and `Load()` functions. Save on each message addition (or debounced). Create session directory structure if it doesn't exist.

---

### 004: Add disk-based image storage for sessions
**Domain:** backend
**Status:** done
**Depends on:** 001

Extend or create image storage that saves preview images to `config/sessions/{session_id}/images/{message_id}.png`. Update the storage interface to accept session ID and message ID instead of generating UUIDs. Handle image overwrites (same message ID replaces existing file).

---

### 005: Session recovery on startup
**Domain:** backend
**Status:** done
**Depends on:** 003, 004

On server startup, scan `config/sessions/` directory. For the current session (or most recent), load `conversation.json` and reconstruct in-memory state. Verify referenced images exist. Set next message ID based on highest existing ID + 1. Handle missing/corrupt session files gracefully.

---

### 006: Update SSE events with message IDs
**Domain:** backend
**Status:** done
**Depends on:** 001, 002

Include message ID in `EventAgentDone`, `EventImageReady`, and new `EventMessageSnapshot` event. Frontend needs message IDs to know which bubble to update when an image completes. Add message ID to the event data structures.

---

### 007: Add endpoint to load historical message state
**Domain:** backend
**Status:** done
**Depends on:** 002, 003

Create `GET /message/{id}/state` endpoint that returns the snapshot for a given message ID. Response includes prompt, steps, cfg, seed. Frontend calls this when user clicks a message with a snapshot, then populates the input fields with the returned values.

---

### 008: Update generation to associate images with messages
**Domain:** backend
**Status:** done
**Depends on:** 004, 006

Modify `handleGenerate` to accept an optional `message_id` parameter. When provided, save the generated image using that message ID (to session-specific storage), update the message's preview status to "complete", and include the message ID in the `EventImageReady` event. Persist the updated conversation state.

---

### 009: Preview bubble component structure
**Domain:** backend
**Status:** done
**Depends on:** 006

Add HTML/CSS to web templates for the preview bubble component. Messages with snapshots display a square preview area (96x96 default) positioned absolute in top-right corner. Add vertical divider line on left edge of preview. Add right padding to text area equal to preview width + 16px. Preview area overlays text (higher z-index).

---

### 010: Preview states and transitions
**Domain:** backend
**Status:** done
**Depends on:** 009

Implement three preview states in CSS/JS: placeholder (reuse empty-state icon), mist (animated fog/loading effect), and final image. Add cross-fade transitions (300-500ms) between states. JS listens for `EventImageReady` with message ID and updates the corresponding preview bubble from mist to image.

---

### 011: Message click to load historical state
**Domain:** backend
**Status:** done
**Depends on:** 007, 009

Add click handler to messages with snapshots. On click (not drag), call `GET /message/{id}/state` endpoint. Populate prompt input, steps, cfg, seed fields with returned values. Visual feedback that state was loaded (brief highlight or similar). Clicking a message also sets it as the "active" message for generation association.

---

### 012: Thinking indicator transition
**Domain:** backend
**Status:** done
**Depends on:** 009

Animate the "..." thinking bubble expanding into the response bubble when first token arrives. If the response will have a snapshot (detected when snapshot event arrives), show preview area with placeholder state. Smooth CSS transition for bubble expansion.

---

### 013: Preview size preference
**Domain:** backend
**Status:** done
**Depends on:** 009

Add global preference for preview size (small: 64px, medium: 96px, large: 128px). Store in session preferences or global config. Add UI control (in settings panel or similar) to change size. Apply size via CSS custom property. Default to medium (96px). Persist preference across sessions.

---

## Notes
This story emerged from architecture discussion about the animated response bubble. The preview bubble UI depends on state-per-message being in place - without stored snapshots, there's nothing to display or load.

The "restore from here" behavior is simple: clicking a historical message loads its snapshot as the current state. The conversation continues linearly from the current position. This is not a fork - it just reuses old state as a starting point.

Storage structure:
```
config/sessions/
  {session_id}/
    conversation.json
    images/
      {message_id}.png
```

From the architecture session, key technical notes:
- Backend uses SSE for streaming (agent-token events)
- Current `Conversation` struct has single `currentPrompt` - needs `PromptSnapshot` per message
- `LLMMetadata` already captures prompt/steps/cfg/seed - store this with assistant messages
- Preview shown when prompt OR settings differ from previous state
