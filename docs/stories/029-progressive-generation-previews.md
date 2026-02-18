# Story: Progressive generation previews

## Status
Done

## Problem
When a user generates an image, they see a static pulsating noise animation for 10-15 seconds with no indication of progress. They don't know if generation is at step 2 or step 27, or whether the image is forming as expected. The only feedback is silence until the final image appears.

## User/Actor
Web UI user generating images through the chat interface.

## Desired outcome
During generation, the user sees a progress bar advancing step-by-step and a preview image that progressively resolves, giving continuous feedback that generation is working and forming the expected image.

## Acceptance criteria
- [ ] During generation, a thin progress line appears at the bottom edge of the chat image component, advancing from left to right as each denoising step completes
- [ ] Progress updates arrive every step (e.g., step 1/28, 2/28, ... 28/28)
- [ ] A preview image appears at step 1, replacing the existing pulsating noise animation
- [ ] Subsequent preview images appear every 5 steps (steps 5, 10, 15, 20, 25), each replacing the previous preview in place
- [ ] Preview images display both inline in the chat message and in the image detail panel (the larger center view)
- [ ] The pulsating noise animation remains visible until the first preview image arrives
- [ ] When generation completes, the final image replaces the last preview and the progress line flashes once then fades away
- [ ] The preview step interval (default 5) is configurable in the generate request sent to compute
- [ ] Preview uses projection-based decoding (PREVIEW_PROJ) -- no additional model files required
- [ ] The binary protocol supports two new message types: progress (step counter, every step) and preview (image data, every N steps)
- [ ] The backend receives progress and preview as a stream of messages for a single request ID (multiple responses per request)
- [ ] New SSE events relay progress and preview data from backend to browser

## Out of scope
- TAESD or full VAE preview decoding (PREVIEW_PROJ only for now; upgrade path exists)
- Downscaling preview images before transmission
- Protocol version bump (new message types are forward-compatible)
- Concurrent generation support (compute remains single-threaded)
- Preview interval UI control (hardcoded default of 5 for now, configurable in protocol)

## Dependencies
None. This builds on the existing generation flow and protocol.

## Open questions
- Projection preview quality: PREVIEW_PROJ is the lowest-quality decode mode. If previews are too noisy to be useful, the follow-up is adding TAESD (~5MB model, one-line change in compute). Assess during implementation.

## Notes
- stable-diffusion.cpp already provides `sd_set_progress_callback()` and `sd_set_preview_callback()` with a configurable interval parameter and PREVIEW_PROJ mode. The compute daemon currently uses neither callback.
- The compute daemon is strictly single-threaded (one generation at a time). Callbacks are global, not per-context. This is fine for the current model.
- Preview at 768x768 RGB is ~1.7MB per frame over Unix socket. At 6 previews per 28-step generation, that's ~10MB total. Acceptable for local socket communication.

## Tasks

### 001: Add MSG_PROGRESS and MSG_PREVIEW encoding to compute protocol
**Domain:** compute
**Status:** done
**Depends on:** none

Add two new message types to the C protocol layer. In `protocol.h`: add `MSG_PROGRESS_EVENT = 0x0003` and `MSG_PREVIEW_EVENT = 0x0004` to the message type enum. Add `sd35_progress_event_t` struct (request_id, step, total_steps, step_time_ms) and `sd35_preview_event_t` struct (request_id, step, total_steps, width, height, channels, image_data_len, image_data). In `protocol.c`: add `encode_progress_event()` and `encode_preview_event()` functions following the existing encoding pattern (big-endian, 16-byte header). Also add a `preview_interval` field (uint32) to the end of `sd35_generate_request_t` and update `decode_generate_request()` to read it from trailing bytes after prompt data, defaulting to 0 if absent (backward-compatible). Update `docs/protocol/SPEC_SD35.md` with the new wire formats. Add unit tests for encoding both new message types and for decoding preview_interval from requests.

---

### 002: Add MSG_PROGRESS and MSG_PREVIEW decoding to backend protocol
**Domain:** backend
**Status:** done
**Depends on:** none

Add two new message types to the Go protocol layer. In `types.go`: add `MsgProgressEvent = 0x0003` and `MsgPreviewEvent = 0x0004` constants. Add `SD35ProgressEvent` struct (RequestID, Step, TotalSteps, StepTimeMs) and `SD35PreviewEvent` struct (RequestID, Step, TotalSteps, ImageWidth, ImageHeight, Channels, ImageDataLen, ImageData). In `decode.go`: add `decodeProgressEvent()` and `decodePreviewEvent()` functions, and route the new message types in `DecodeResponse()`. In `encode.go`: append `preview_interval` (uint32, big-endian) after the prompt data in `EncodeSD35GenerateRequest()`. Add table-driven tests for decoding both new message types and for encoding preview_interval.

---

### 003: Hook stable-diffusion.cpp callbacks in compute
**Domain:** compute
**Status:** done
**Depends on:** 001

Wire the stable-diffusion.cpp progress and preview callbacks in `sd_wrapper.cpp`. Define a callback context struct containing the socket fd, request_id, and preview_interval. Before calling `generate_image()`, call `sd_set_progress_callback()` with a function that encodes and writes `MSG_PROGRESS_EVENT` to the socket on every step. Call `sd_set_preview_callback()` with `PREVIEW_PROJ` mode and the request's preview_interval, with a function that encodes and writes `MSG_PREVIEW_EVENT` (including pixel data from the `sd_image_t`) to the socket. Pass the context struct via the `void* data` parameter. The socket write is safe because compute is single-threaded and callbacks fire synchronously during `generate_image()`. Verify with a test that generates an image and confirms progress/preview messages appear on the socket before the final response.

---

### 004: Stream responses in backend socket client
**Domain:** backend
**Status:** done
**Depends on:** 002

Change the multiplexed socket client to support multiple responses per request ID. In `socket.go`, modify `responseReader()`: before deleting from `pendingRequests`, check the message type in header bytes 6-7. Only delete and close the channel on `MsgGenerateResponse` or `MsgError`. For `MsgProgressEvent` and `MsgPreviewEvent`, deliver to the channel but keep the entry in `pendingRequests` for subsequent messages. Add a `SendStream()` method that returns a `<-chan []byte` receiving all messages for a request ID. The channel is closed when the final response or error arrives. Preserve the existing `Send()` method for backward compatibility (it can call `SendStream()` internally and return only the last message). Add tests: send a mock stream of progress, preview, and final response messages through the multiplexer and verify they all arrive on the channel in order.

---

### 005: Relay progress and preview via SSE in backend web server
**Domain:** backend
**Status:** done
**Depends on:** 004

Add new SSE events and change `generateImage()` to use streaming. In `sse.go`: add `EventGenerationProgress` and `EventGenerationPreview` event type constants. Add `GenerationProgressData` struct (Step, TotalSteps, MessageID) and `GenerationPreviewData` struct (URL, Width, Height, MessageID). In `server.go`: change `generateImage()` from calling `Send()` to `SendStream()`. Loop over the returned channel. For `MsgProgressEvent`: decode and send `EventGenerationProgress` SSE. For `MsgPreviewEvent`: decode pixel data, encode as PNG, store as a temporary preview image (using the pattern `{final_image_filename}_preview.png`), and send `EventGenerationPreview` SSE with the preview URL. For `MsgGenerateResponse`: existing `EventImageReady` flow. For `MsgError`: existing error flow. Set preview_interval to 5 in the outgoing generate request.

---

### 006: Frontend progress bar
**Domain:** backend
**Status:** done
**Depends on:** 005

Add a progress bar to the chat image component. In `index.html`: add CSS for a thin (2-3px) progress line positioned absolutely at the bottom edge of the `.message-preview` component. The line starts at width 0% and grows to 100% based on step/total_steps. Add a JS handler for the `generation-progress` SSE event that finds the message by message_id and updates the progress bar width. Add CSS keyframe animation for completion: when `image-ready` fires, the progress line flashes (brief brightness pulse) then fades to transparent over ~500ms. The progress bar should be visible from the first progress event through completion.

---

### 007: Frontend preview image display
**Domain:** backend
**Status:** done
**Depends on:** 005

Display preview images during generation. In `index.html`: add a JS handler for the `generation-preview` SSE event. On the first preview: find the message by message_id, change the `.message-preview` element's `data-status` from `"generating"` to `"preview"` (new state), and set the img src to the preview URL. This replaces the pulsating noise animation. On subsequent previews: update the img src in place (no transition, just swap). Also update the image detail panel (the larger center view) with each preview so both views stay in sync. When `image-ready` fires, the existing handler replaces the preview with the final image and sets `data-status` to `"complete"`. Add the `"preview"` CSS state to `.message-preview` that shows the image without the mist animation.
