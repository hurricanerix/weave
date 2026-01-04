# Story 009: Image Encoding and Serving

## Problem

The weave-compute daemon returns raw pixel data (RGB/RGBA bytes). The browser needs a displayable image format (PNG). The Go application must convert the raw data to PNG, store it temporarily, and serve it via HTTP so the browser can display it inline in chat.

## User/Actor

- End user (sees generated images in chat)
- Weave developer (implementing image pipeline)

## Desired Outcome

A working image pipeline where:
- Raw pixel data from weave-compute is converted to PNG
- PNG is stored temporarily and served via HTTP
- Browser can display the image via standard `<img>` tag
- SSE `image-ready` event includes the image URL
- Old images are cleaned up to prevent disk/memory bloat

## Acceptance Criteria

### PNG Encoding

- [ ] Go receives raw pixel data from compute client (format defined in Story 001)
- [ ] Raw data converted to PNG using Go's `image/png` package
- [ ] Conversion handles both RGB and RGBA formats (based on protocol spec)
- [ ] Conversion validates image dimensions match expected values
- [ ] Encoding errors return clear error message to caller

### Image Storage

- [ ] Each generated image gets a unique ID (UUID or similar)
- [ ] Images stored in memory for MVP (map of ID to PNG bytes)
- [ ] Alternative: stored as temp files in `$XDG_RUNTIME_DIR/weave/images/`
- [ ] Image URL format: `/images/<id>.png`
- [ ] Images accessible immediately after generation completes

### HTTP Serving

- [ ] `GET /images/<id>.png` returns PNG with correct Content-Type
- [ ] Non-existent image ID returns 404
- [ ] Malformed image ID returns 400
- [ ] Images served with Cache-Control headers (immutable, long expiry)

### SSE Integration

- [ ] After image is stored, `image-ready` SSE event is sent
- [ ] Event payload includes image URL: `{"url": "/images/<id>.png", "width": 1024, "height": 1024}`
- [ ] Event includes image dimensions for layout before load
- [ ] Event sent only after image is ready to serve (no race condition)

### Cleanup

- [ ] Images older than 1 hour are eligible for cleanup
- [ ] Cleanup runs periodically (every 10 minutes)
- [ ] Cleanup is non-blocking (runs in background goroutine)
- [ ] At most 100 images retained (LRU eviction if limit exceeded)
- [ ] Cleanup logged at DEBUG level

### Error Handling

- [ ] If PNG encoding fails, error returned to caller (no partial image stored)
- [ ] If storage fails (disk full, etc.), error returned to caller
- [ ] Errors propagated to chat as error messages (Story 007 handles display)

### Testing

- [ ] Unit test: PNG encoding produces valid PNG from raw RGB data
- [ ] Unit test: PNG encoding produces valid PNG from raw RGBA data
- [ ] Unit test: invalid raw data (wrong size) returns error
- [ ] Unit test: image retrieval by ID works
- [ ] Unit test: non-existent ID returns appropriate error
- [ ] Integration test: generate image, retrieve via HTTP, verify PNG is valid
- [ ] Integration test: cleanup removes old images
- [ ] Integration test: LRU eviction works when limit exceeded

### Documentation

- [ ] Code comments explain image ID generation
- [ ] Code comments explain cleanup strategy
- [ ] `docs/DEVELOPMENT.md` mentions where images are stored (for debugging)

## Out of Scope

- JPEG encoding (PNG only for MVP)
- Image compression optimization
- Image resizing/thumbnails
- Persistent image storage (database, S3, etc.)
- Image download button in UI
- Image metadata (prompt used, generation params, etc.)

## Dependencies

- Story 001: Binary Protocol Implementation (defines raw image format)
- Story 002: Unix Socket Communication (transport for receiving image data)
- Story 006: Web UI Foundation (HTTP server to mount image endpoint)
- Story 007: Chat and Prompt Panes (sends `image-ready` SSE event, displays image)

## Notes

For MVP, in-memory storage is simplest. A `map[string][]byte` protected by a mutex works fine. The cleanup goroutine prevents unbounded memory growth.

The image URL is relative (`/images/...`) so it works regardless of host/port. The browser requests it like any other static asset.

The `image-ready` SSE event connects this story to Story 007. When the event fires, the chat UI appends an `<img>` tag with the provided URL. The browser then fetches the image via a separate HTTP request.

Raw pixel format (RGB vs RGBA, byte order) is defined in the protocol spec from Story 001. This story just implements whatever format the protocol specifies.

## Tasks

### 001: Implement PNG encoding from raw pixel data
**Domain:** weave
**Status:** pending
**Depends on:** none

Create `internal/image/encode.go` with EncodePNG(width, height, pixels, format) function. Use Go's image/png package to convert raw RGB/RGBA bytes to PNG. Handle both RGB and RGBA formats based on protocol spec (Story 001). Validate dimensions match pixel data length. Return PNG bytes or error.

**Files to create:**
- `internal/image/encode.go`
- `internal/image/encode_test.go`

**Testing:** Unit tests with test pixel data (solid colors, gradients). Verify PNG is valid, dimensions match, format is correct.

---

### 002: Implement in-memory image storage
**Domain:** weave
**Status:** pending
**Depends on:** 001

Create `internal/image/storage.go` with Storage type. Map of image ID (UUID) to PNG bytes, protected by mutex. Implement Store(pngBytes) returns ID, Get(id) returns bytes or error. Thread-safe operations.

**Files to create:**
- `internal/image/storage.go`
- `internal/image/storage_test.go`

**Testing:** Unit tests verify store/retrieve, thread safety, non-existent ID returns error.

---

### 003: Implement image cleanup with LRU eviction
**Domain:** weave
**Status:** pending
**Depends on:** 002

In storage.go, add timestamp to each stored image. Implement periodic cleanup goroutine (runs every 10 minutes) that removes images older than 1 hour. Implement LRU eviction when storage exceeds 100 images. Log cleanup at DEBUG level.

**Files to modify:**
- `internal/image/storage.go`
- `internal/image/storage_test.go`

**Testing:** Unit tests verify age-based cleanup, LRU eviction, cleanup logging.

---

### 004: Implement HTTP image serving endpoint
**Domain:** weave
**Status:** pending
**Depends on:** 002

In server.go, add GET /images/:id.png handler. Extract ID from path, call Storage.Get(id), return PNG with Content-Type: image/png. Handle 404 for non-existent ID, 400 for malformed ID. Set Cache-Control headers (immutable, long expiry).

**Files to modify:**
- `internal/web/server.go`
- `internal/web/server_test.go`

**Testing:** Unit tests verify correct responses for valid/invalid IDs, headers. Integration test stores image, retrieves via HTTP.

---

### 005: Wire image encoding into generate handler
**Domain:** weave
**Status:** pending
**Depends on:** 001, 002

In POST /generate handler (Story 007), after receiving raw pixel data from compute client, call EncodePNG() to convert to PNG. Call Storage.Store() to save and get ID. Construct image URL (/images/<id>.png). Send image-ready SSE event with URL and dimensions.

**Files to modify:**
- `internal/web/server.go`
- `internal/web/server_test.go`

**Testing:** Integration test triggers generation, verifies PNG encoding, storage, image-ready event with correct URL.

---

### 006: Implement error handling for encoding failures
**Domain:** weave
**Status:** pending
**Depends on:** 005

In generate handler, catch PNG encoding errors (invalid dimensions, wrong pixel data size). Catch storage errors (disk full, if using filesystem). Return appropriate HTTP status and send SSE error event. No partial images stored on failure.

**Files to modify:**
- `internal/web/server.go`
- `internal/web/server_test.go`

**Testing:** Unit tests for encoding error handling. Integration test simulates encoding failure, verifies error event sent.

---

### 007: Integration test for full image pipeline
**Domain:** weave
**Status:** pending
**Depends on:** 004, 005, 006

Create integration test that generates image (or uses stubbed raw data), verifies PNG encoding, storage, retrieval via HTTP, SSE event delivery, cleanup behavior. Test complete image flow end-to-end.

**Files to create:**
- `internal/image/pipeline_test.go` (tagged integration)

**Testing:** Integration test passes. Verifies encoding, storage, serving, cleanup.

---

### 008: Update DEVELOPMENT.md with image storage documentation
**Domain:** documentation
**Status:** pending
**Depends on:** 007

Add section explaining where images are stored (in-memory for MVP), cleanup behavior (1 hour age, 100 image limit), how to debug image issues (check logs at DEBUG level, verify storage state).

**Files to modify:**
- `docs/DEVELOPMENT.md`

**Verification:** Documentation is clear. Explains image lifecycle.
