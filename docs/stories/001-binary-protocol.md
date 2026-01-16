# Story 001: Binary Protocol Implementation

## Problem

The Go application needs to send generation requests to the C daemon and receive image data back. We need a versioned, extensible binary protocol that's safe (no buffer overflows), efficient (minimal parsing), and designed for future expansion (streaming, progress updates, multiple models).

## User/Actor

Both the weave developer (implementing Go client) and compute developer (implementing C daemon). These are internal interfaces, not end-user facing, but they're critical infrastructure.

## Desired Outcome

A working binary protocol where:
- Go can encode a generation request (model ID, prompt, dimensions, steps, CFG, seed)
- C can decode that request safely
- C can encode an image response (raw pixel data)
- Go can decode that response
- Protocol uses HTTP-like status codes (200/400/500)
- Protocol is documented with wire format diagrams and examples in two-level structure (common + model-specific)

## Acceptance Criteria

### Documentation

- [x] `docs/protocol/SPEC.md` exists and contains:
  - Common message header format (magic, version, message type, payload length)
  - Request structure: model ID field specification
  - Response structure: status codes (200/400/500), response payload format
  - Error message format (status 400/500 with error string)
  - Version negotiation strategy
  - Wire format conventions (endianness, alignment, string encoding)

- [x] `docs/protocol/SPEC_SD35.md` exists and contains:
  - Model ID constant (0 = SD 3.5)
  - Generation parameters specific to SD 3.5 (width, height, steps, CFG scale, seed)
  - Prompt offset table structure (CLIP-L, CLIP-G, T5 with offsets and lengths)
  - Prompt duplication requirement (same prompt written three times)
  - Response format: raw image data layout (width, height, channels, pixel bytes)
  - Parameter bounds for SD 3.5 (valid ranges for dimensions, steps, CFG)
  - Example request/response with hex dumps

### Implementation

- [x] Go encoder creates valid `MSG_GENERATE_REQUEST` with common header, model ID 0, and SD 3.5-specific payload
- [x] C decoder safely parses `MSG_GENERATE_REQUEST` and validates model ID (0 = accept, anything else = status 400 "model doesn't exist for the given model id")
- [x] C decoder parses SD 3.5 payload (prompt offset table, generation params) with bounds checking
- [x] C encoder creates valid `MSG_GENERATE_RESPONSE` (status 200) with SD 3.5 raw image data
- [x] Go decoder safely parses `MSG_GENERATE_RESPONSE` and extracts raw image data
- [x] Error responses include status code and error message string
- [x] All bounds checking implemented (prompt length, dimensions, buffer sizes, integer overflows)

### Testing

- [x] Unit tests in Go validate SD 3.5 request encoding (model ID 0)
- [x] Unit tests in C validate SD 3.5 request decoding with parameter bounds checking
- [x] Unit tests in C validate rejection of non-zero model IDs with appropriate error message
- [x] Unit tests in C validate response encoding (stubbed image data)
- [x] Unit tests in Go validate response decoding
- [x] Integration test: Go encodes SD 3.5 request -> C decodes -> C encodes response -> Go decodes (stubbed generation returns test pattern)
- [x] Fuzzing harness for C decoder exists (AFL or libFuzzer setup)

### Development Documentation

- [x] `docs/DEVELOPMENT.md` updated with protocol testing instructions (how to run unit tests, integration tests, fuzzing setup)

## Out of Scope

- PNG/JPEG encoding (happens in Go layer)
- Streaming progress updates
- Request cancellation
- Multiple simultaneous requests
- Runtime model loading/switching (model is hardcoded in daemon)
- Model IDs other than 0

## Dependencies

None. This is the foundation.

## Notes

Model ID 0 is hardcoded to SD 3.5 for MVP. The daemon doesn't support loading different models—it always has SD 3.5 loaded. The prompt is duplicated three times in the wire format (once per text encoder: CLIP-L, CLIP-G, T5). Raw image format is RGB or RGBA pixel data (exact format to be decided during implementation based on SD 3.5 output).

## Tasks

### 001: Write protocol specification documents
**Domain:** documentation
**Status:** done
**Depends on:** none

Create `docs/protocol/SPEC.md` with common message structure (magic number 0x57455645 "WEVE", version field, message type enum, payload length, endianness rules). Create `docs/protocol/SPEC_SD35.md` with model-specific details (model ID 0, prompt offset table for CLIP-L/CLIP-G/T5, generation parameters with bounds, raw image layout). Include wire format diagrams and hex dump examples.

**Files to create:**
- `docs/protocol/SPEC.md`
- `docs/protocol/SPEC_SD35.md`

**Verification:** Both files exist, contain all required sections from acceptance criteria, include example hex dumps.

---

### 002: Define Go protocol types and constants
**Domain:** weave
**Status:** done
**Depends on:** 001

Create `internal/protocol/types.go` with message type constants (MSG_GENERATE_REQUEST, MSG_GENERATE_RESPONSE, MSG_ERROR), status codes (200/400/500), and Go structs for request/response. Define SD35GenerateRequest with prompt offset table structure, generation params (width, height, steps, cfg, seed), and model ID field.

**Files created:**
- `internal/protocol/types.go` - Protocol types, constants, and structs
- `internal/protocol/types_test.go` - Comprehensive unit tests

**Testing:** Unit tests verify:
- Header struct size (16 bytes) and field offsets match protocol spec
- All protocol constants have correct values
- Message types, status codes, error codes match spec
- GenerateRequest structure with correct field order
- SD35GenerateRequest has all required fields
- SD35 parameter bounds constants
- Parameter validation logic (dimensions, steps, CFG, prompts)
- Prompt offset table bounds checking
- Image data length calculations
- All tests pass with 100% coverage on testable code

---

### 003: Implement Go request encoder
**Domain:** weave
**Status:** done
**Depends on:** 002

Implement `internal/protocol/encode.go` with function to encode SD35GenerateRequest to bytes. Write common header (magic, version, type, length), then model ID and SD35-specific payload. Duplicate prompt text three times per spec. Use big-endian byte order. Include bounds checking for all fields.

**Files created:**
- `internal/protocol/encode.go` - Encoder implementation with validation
- `internal/protocol/encode_test.go` - Comprehensive table-driven tests

**Implementation details:**
- `EncodeSD35GenerateRequest()` encodes SD35GenerateRequest to bytes with full validation
- `validateSD35Request()` validates all parameters before encoding (dimensions, steps, CFG, prompts, offsets)
- `NewSD35GenerateRequest()` convenience function that automatically duplicates prompt three times
- All multi-byte integers encoded in big-endian using encoding/binary
- Float32 CFG encoded using math.Float32bits for IEEE 754 compliance
- Comprehensive bounds checking: dimensions (64-2048, multiple of 64), steps (1-100), CFG (0.0-20.0, not NaN/Inf)
- Prompt offset table validation ensures no out-of-bounds access
- Total message size checked against MaxMessageSize (10 MB)

**Testing:** Table-driven tests with 88.2% coverage:
- Valid requests with min/max/typical parameters
- All invalid parameter cases (dimensions, steps, CFG, model ID, prompts)
- Byte format verification against spec hex dumps
- Prompt duplication correctness
- Offset table calculation
- IEEE 754 float encoding
- NaN and infinity rejection
- Buffer bounds validation

---

### 004: Implement Go response decoder
**Domain:** weave
**Status:** done
**Depends on:** 002

Implement `internal/protocol/decode.go` with function to decode MSG_GENERATE_RESPONSE and MSG_ERROR. Parse common header, validate magic and version, extract status code and payload. For status 200, extract raw image data. For status 400/500, extract error message string.

**Files created:**
- `internal/protocol/decode.go` - Response decoder with validation
- `internal/protocol/decode_test.go` - Comprehensive table-driven tests

**Implementation details:**
- `DecodeResponse()` main entry point returns either `*SD35GenerateResponse` or `*ErrorResponse`
- `decodeHeader()` validates and decodes 16-byte common header
- `decodeGenerateResponse()` decodes MSG_GENERATE_RESPONSE (status 200)
- `decodeErrorResponse()` decodes MSG_ERROR (status 400/500)
- All bounds checked before reads, integer overflow protection
- Validates magic (0x57455645), version range, message type
- Image validation: dimensions (64-2048, multiple of 64), channels (3/4)
- 83.6% test coverage

**Testing:** Table-driven tests for valid responses (various dimensions, RGB/RGBA), error responses, truncated data, malformed headers, integer overflow scenarios, round-trip verification.

---

### 005: Define C protocol types and constants
**Domain:** compute
**Status:** done
**Depends on:** 001

Create `compute/include/weave/protocol.h` with message type enum, status codes, and C structs for wire format. Define generate_request_t with header, model_id, prompt offset table, generation params. Define generate_response_t and error_response_t. Use fixed-width types (uint32_t, uint16_t) and document byte order.

**Files created:**
- `compute/include/weave/protocol.h` - Protocol types, constants, and structs for C

**Implementation details:**
- Uses `#pragma once` for header guard (C99 standard)
- All multi-byte integer types from `<stdint.h>` (uint32_t, uint16_t, uint64_t)
- Comprehensive Doxygen-style documentation
- Protocol constants: PROTOCOL_MAGIC (0x57455645), PROTOCOL_VERSION_1, MAX_MESSAGE_SIZE (10 MB)
- Model identifiers: MODEL_ID_SD35 (0x00000000)
- SD 3.5 parameter bounds: dimensions (64-2048, multiple of 64), steps (1-100), CFG (0.0-20.0), prompt lengths (1-2048 bytes per encoder)
- Enums: message_type_t (MSG_GENERATE_REQUEST, MSG_GENERATE_RESPONSE, MSG_ERROR)
- Enums: status_code_t (STATUS_OK = 200, STATUS_BAD_REQUEST = 400, STATUS_INTERNAL_SERVER_ERROR = 500)
- Enums: error_code_t (ERR_NONE through ERR_INTERNAL with 12 distinct error codes)
- Structs: protocol_header_t (16-byte common header)
- Structs: sd35_generate_request_t (in-memory representation with prompt offset table)
- Structs: sd35_generate_response_t (in-memory representation with image metadata)
- Structs: error_response_t (in-memory representation)
- All structs documented as in-memory representations, NOT wire format (encoding/decoding handles conversion)
- Byte order documented as big-endian (network byte order)

**Testing:** Verified compilation with strict C99 flags:
- gcc -std=c99 -Wall -Wextra -Werror -pedantic: Clean compilation
- Additional warnings tested: -Wconversion -Wshadow -Wstrict-prototypes: Clean compilation
- Test program created to verify all types, constants, and enums are accessible
- No compiler warnings or errors

---

### 006: Implement C request decoder
**Domain:** compute
**Status:** done
**Depends on:** 005

Implement `compute/src/protocol.c` with decode_generate_request() function. Parse header (validate magic 0x57455645, check version), extract model_id, validate model_id == 0 (return error 400 for non-zero). Parse SD35 payload with prompt offset table and generation params. Perform ALL bounds checking (prompt length, dimensions, integer overflow, buffer sizes).

**Files created:**
- `compute/src/protocol.c` - Request decoder implementation
- `compute/test/test_protocol.c` - 25 unit tests
- `compute/Makefile` - Build system with test targets

**Implementation details:**
- `decode_generate_request()` main decoder function
- Big-endian helpers: `read_u16_be()`, `read_u32_be()`, `read_u64_be()`, `read_f32_be()`
- Validates magic (0x57455645), version, message type
- Rejects non-zero model ID with ERR_INVALID_MODEL_ID
- Validates all SD35 parameters: dimensions, steps, CFG (including NaN/Inf), prompt offsets
- Integer overflow protection using subtraction pattern
- All 25 tests pass with ASan/UBSan clean

**Testing:** Unit tests for valid requests, invalid model IDs, out-of-bounds parameters, buffer overflow attempts, integer overflow, truncated messages, NULL pointers.

---

### 007: Implement C response encoder
**Domain:** compute
**Status:** done
**Depends on:** 005

Implement encode_generate_response() and encode_error_response() in protocol.c. For success (status 200), write header + image dimensions + raw pixel data. For errors (status 400/500), write header + error code + error message string. Include length-prefix for error messages. All multi-byte integers in big-endian.

**Files updated:**
- `compute/src/protocol.c` - Added encoder functions
- `compute/test/test_protocol.c` - Added 14 encoder tests (total now 39 tests)

**Implementation details:**
- `encode_generate_response()` encodes successful SD 3.5 responses with image data
- `encode_error_response()` encodes error responses (status 400/500)
- Big-endian write helpers: `write_u16_be()`, `write_u32_be()`, `write_u64_be()`
- Validates dimensions (64-2048, multiple of 64), channels (3/4)
- Validates image_data_len matches width × height × channels
- Integer overflow protection in size calculations
- Buffer size validated before writing
- All 39 tests pass with ASan/UBSan clean

**Testing:** Unit tests verify encoding with stubbed image data (test pattern), error messages, boundary conditions (empty messages, various dimensions, RGB/RGBA).

---

### 008: Create integration test harness
**Domain:** weave + compute
**Status:** done
**Depends on:** 003, 004, 006, 007

Create integration test that encodes request in Go, passes bytes to C decoder (via test shim or file), C encodes stubbed response, Go decodes response. Verify round-trip works. Use stubbed generation that returns a test pattern (e.g., checkerboard or gradient) instead of real model inference.

**Files to create:**
- `test/integration/protocol_roundtrip_test.go`
- `compute/test/test_stub_generator.c` (returns test pattern)

**Testing:** Integration test passes. Verify image dimensions match request. Verify test pattern data integrity.

---

### 009: Set up fuzzing harness for C decoder
**Domain:** compute
**Status:** done
**Depends on:** 006

Create fuzzing harness using libFuzzer or AFL for decode_generate_request(). Fuzz with random byte inputs to find crashes, hangs, or undefined behavior. Configure to run for at least 1 million iterations. Document how to run fuzzer in DEVELOPMENT.md.

**Files created:**
- `compute/fuzz/fuzz_protocol.c` - libFuzzer/AFL harness
- `compute/fuzz/generate_corpus.c` - Generates 13 seed corpus files
- `compute/fuzz/test_corpus.c` - Validates corpus with ASan/UBSan
- `compute/fuzz/stress_test.c` - 1M+ iteration stress test
- `compute/fuzz/README.md` - Comprehensive fuzzing documentation
- Updated `compute/Makefile` - Added fuzz targets

**Implementation details:**
- Implements `LLVMFuzzerTestOneInput()` for libFuzzer
- Also supports AFL via `-DAFL_MODE` compile flag
- 13-file seed corpus covering valid, invalid, and edge cases
- Stress test provides 1M+ iteration verification when clang unavailable
- Makefile targets: `make fuzz`, `make test-corpus`, `make stress-test`

**Testing:**
- Corpus validation: 13/13 files pass with ASan/UBSan
- Stress test: 1,000,000 iterations completed in 0.12 seconds (8.4M exec/s)
- Zero crashes, hangs, or undefined behavior detected

---

### 010: Update DEVELOPMENT.md with protocol testing instructions
**Domain:** documentation
**Status:** done
**Depends on:** 008, 009

Add section to `docs/DEVELOPMENT.md` explaining how to run protocol unit tests (Go and C), integration tests, and fuzzing. Include commands, expected output, and troubleshooting tips.

**Files to modify:**
- `docs/DEVELOPMENT.md`

**Verification:** Documentation is clear, commands work as documented.
