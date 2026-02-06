# Story: Stop generation

## Status
Ready

## Problem
Once a user starts generating an image, there is no way to stop it. If the preview looks wrong, or if they made a mistake in the prompt, they must wait for the full generation to complete before trying again. On a 28-step generation this means up to 15 seconds of wasted time.

## User/Actor
Web UI user who has started an image generation and wants to stop it.

## Desired outcome
The user can stop a running generation at any point. The last preview image is preserved so they can see what was being generated and decide whether to retry. The application is ready for the next generation without manual intervention.

## Acceptance criteria
- [ ] A stop button is visible during active generation
- [ ] Clicking stop terminates the current generation within 1 second of user action
- [ ] The most recent preview image persists in the chat message and image detail panel after stopping
- [ ] Preview images are saved to disk as `{final_image_filename}_preview.png`, overwritten as each new preview arrives during generation
- [ ] Stopped generation previews survive app restarts
- [ ] The progress bar remains frozen at its current position when generation is stopped (visual indicator of incomplete generation)
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
