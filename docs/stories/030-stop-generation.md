# Story: Stop generation

## Status
Ready

## Problem
Once a user starts generating an image, there is no way to stop it. If the preview looks wrong, or if they made a mistake in the prompt, they must wait for the full generation to complete before trying again. On a 28-step generation this means up to 15 seconds of wasted time.

## User/Actor
Web UI user who has started an image generation and wants to stop it.

## Desired outcome
The user can stop a running generation at any point. The last preview image is preserved (shown in grayscale to indicate incomplete) so they can see what was being generated. A restart button replaces the stop button, allowing immediate retry. The application is ready for the next generation without manual intervention.

## Acceptance criteria
- [ ] A footer below the large preview shows a stop button and "Generating step X of Y" text during active generation
- [ ] Clicking stop terminates the current generation within 1 second of user action
- [ ] The most recent preview image persists in the chat message (grayscale) and image detail panel after stopping
- [ ] After stopping, the stop button changes to a restart button that re-triggers generation with current settings
- [ ] Preview images are saved to disk as `{final_image_filename}_preview.png`, overwritten as each new preview arrives during generation
- [ ] Stopped generation previews survive app restarts
- [ ] The progress bar remains frozen at its current position when generation is stopped (visual indicator of incomplete generation)
- [ ] The footer (stop button and step text) is hidden when generation completes successfully
- [ ] The application is ready to generate again after stopping (compute process recovers automatically)
- [ ] If stop is clicked before the first preview arrives, the pulsating noise animation is removed and no image is shown for that message

## Out of scope
- Graceful in-process cancellation (stable-diffusion.cpp has no abort API; current implementation kills and respawns the compute process)
- Resume or continue a stopped generation from where it left off
- Undo or remove the stopped generation message from chat

## Dependencies
- Story 029 (progressive generation previews) -- provides the progress bar, preview images, and streaming protocol that this story builds on

## Open questions
None.

## Notes
- The backend already spawns the compute process. Stopping kills it and respawns. Model reload after respawn takes 2-3 seconds.
- All naming (HTTP endpoint, SSE events, UI labels) uses "stop" not "kill" or "cancel." This keeps the door open for a future graceful stop via protocol message without renaming anything.
- The stop mechanism is entirely in the Go backend -- no protocol message to compute is needed since the process is killed directly.

## Tasks

### 001: Add preview persistence and event types
**Domain:** backend
**Status:** done
**Depends on:** none

Add data layer support for persistent preview images and the stopped generation state. In `persistence/image.go`: add `SavePreview(sessionID, messageID, pngData)` that writes to `{basePath}/{sessionID}/images/{messageID}_preview.png`, `LoadPreview(sessionID, messageID)` that reads it, `DeletePreview(sessionID, messageID)` that removes it, `GetPreviewURL(sessionID, messageID)` returning `/sessions/{sessionID}/images/{messageID}_preview.png`, and `PreviewExists(sessionID, messageID)` that checks existence. In `conversation/types.go`: add `PreviewStatusStopped = "stopped"` constant. In `web/sse.go`: add `EventGenerationStopped = "generation-stopped"` constant and `GenerationStoppedData` struct with `MessageID int` and `PreviewURL string` fields. Update `handleSessionImage` in `server.go` to serve `_preview.png` files: when the filename matches `{messageID}_preview.png`, call `LoadPreview` instead of `Load`. Add unit tests for all new persistence methods.

---

### 002: Save previews to persistent storage during generation
**Domain:** backend
**Status:** done
**Depends on:** 001

Change preview image storage from temporary `./tmp/` to persistent session storage so previews survive restarts. In `generateImage()`: replace the `./tmp/` preview write with `imageStore.SavePreview()`. Change preview URL from `/tmp/preview_{session}_{message}.png?step=N` to `imageStore.GetPreviewURL()` with step query parameter for cache-busting. On successful generation completion (inside the `SD35GenerateResponse` case): call `imageStore.DeletePreview()` to clean up the preview file since the final image replaces it. Remove the `defer` block that deletes from `./tmp/`. Add unit tests verifying preview is saved to persistent storage, that the URL format is correct, and that preview is deleted on completion.

---

### 003: Add stop generation endpoint with compute restart
**Domain:** backend
**Status:** pending
**Depends on:** 002

Add the server-side stop generation capability and compute process lifecycle management. In `Server` struct: add `activeGenMu sync.Mutex`, `activeGenCancel map[string]context.CancelFunc` for tracking active generation contexts per session, and `computeRestartFunc func(ctx context.Context) (*client.Conn, error)` for killing and respawning compute. Add `SetComputeClient(c *client.Conn)` method. Update `NewServerWithDeps` to accept the restart func and initialize the cancel map. In `generateImage()`: create a cancellable child context and store its cancel func in `activeGenCancel`. Remove from map when the response loop exits. After the loop, check for context cancellation: if stopped, update message preview status to "stopped" with the persistent preview URL, and send `EventGenerationStopped` SSE event. Add `POST /stop-generation` handler: look up and call the session's cancel func, then launch a goroutine that calls `computeRestartFunc` and calls `SetComputeClient` with the new connection. Register route with rate limiting. In `main.go`: implement the compute restart closure that kills the old process (SIGKILL), closes old connection and listener, removes old socket file, creates new socket+listener, spawns new compute process, accepts new connection, and returns the new `*client.Conn`. Add unit tests for the handler, generation tracking, and context cancellation behavior.

---

### 004: Frontend stop/restart UI and generation footer
**Domain:** backend
**Status:** pending
**Depends on:** 003

Add UI for stopping generation, restarting, and displaying stopped state. In `index.html`:

**Image panel footer.** Add a `.image-panel-footer` div inside `.image-panel` (after `.image-container`), absolutely positioned at the bottom. Contains a stop button and a `#generation-step-text` span. Hidden by default; shown via `.visible` class. `showImagePanelFooter()` adds the class, `hideImagePanelFooter()` removes it and clears text.

**Stop button.** The stop button (`#stop-generation-btn`) sends `POST /stop-generation` via fetch (not HTMX). Styled to match existing overlay buttons (dark translucent background, white text).

**Step progress text.** `handleGenerationProgress` calls `updateStepText(step, totalSteps)` to display "Generating step X of Y" next to the stop button.

**Generation lifecycle.** `handleGenerationStarted` calls `showImagePanelFooter()`. `handleImageReady` and `handleError` call `hideImagePanelFooter()`.

**Stop-to-restart transition.** `setFooterMode(mode)` switches the button between two states: `'stop'` mode (square icon, calls `stopGeneration()`) and `'restart'` mode (circular arrow icon, calls `restartGeneration()`). `handleGenerationStopped` calls `setFooterMode('restart')` instead of hiding the footer, and sets the step text to "Generation stopped". `restartGeneration()` hides the footer and programmatically clicks the generate button.

**Stopped event handling.** Register `generation-stopped` in HTMX SSE (`sse-swap` element). Add `'generation-stopped'` case to the SSE event switch calling `handleGenerationStopped(data)`. The handler calls `updatePreviewState(data.message_id, 'stopped', data.preview_url)` if a preview URL exists, otherwise sets data-status to "stopped" with no image (removes mist animation).

**Stopped state CSS.** `.message-preview[data-status="stopped"] img`: `opacity: 1`, `filter: grayscale(100%)` to visually distinguish stopped previews from completed images. Stopped previews are clickable with hover effect matching complete state.
