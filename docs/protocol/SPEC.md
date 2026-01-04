# Weave Binary Protocol Specification

Version: 1
Last Updated: 2025-12-31

## Overview

The Weave protocol is a binary format for communication between the Go orchestration layer (weave) and the C GPU daemon (weave-compute) over Unix domain sockets. The protocol is designed for safety, efficiency, and extensibility.

## Design Principles

1. **Safety**: All fields are bounds-checked. No buffer overflows, no integer overflows, no undefined behavior.
2. **Versioning**: Every message includes a protocol version for future compatibility.
3. **Extensibility**: Model-specific details live in separate specifications (SPEC_SD35.md, etc.).
4. **Performance**: Binary encoding with zero-copy where possible.

## Authentication

Authentication occurs at the socket level using SO_PEERCRED, NOT in the protocol itself.

The daemon uses `getsockopt(SOL_SOCKET, SO_PEERCRED)` to verify the connecting process's UID/GID immediately after `accept()`. This happens before any protocol data is read.

**Authorization:**
- Userland mode: Only connections from the same UID are allowed
- System mode: Connections from allowed group members are permitted

**On rejection:**
- Socket is closed immediately
- No protocol response is sent
- Event is logged for audit purposes

**Why socket-level authentication:**
- Kernel guarantees UID/GID authenticity (unforgeable)
- No token management complexity
- Authentication happens before parsing untrusted data
- Simpler protocol (no auth fields in messages)

## Terminology

This specification uses precise terminology for message structure:

- **Common header**: The 16-byte header present in every message (magic, version, msg_type, payload_len, reserved)
- **Payload**: All bytes following the 16-byte common header. Size specified by payload_len field.
- **Common request fields**: The request_id (8 bytes) and model_id (4 bytes) that follow the common header in all requests
- **Model-specific payload**: The remaining payload data after common request fields, format defined by model_id (see SPEC_SD35.md, etc.)

Example structure:
```
┌──────────────────────────────────────┐
│ Common Header (16 bytes)             │
├──────────────────────────────────────┤  ← Start of payload
│ Common Request Fields (12 bytes)     │
│ - request_id (8)                     │
│ - model_id (4)                       │
├──────────────────────────────────────┤
│ Model-Specific Payload (variable)    │
└──────────────────────────────────────┘

Total message size = 16 + payload_len
Payload size (payload_len) = 12 + model-specific payload size
```

## Wire Format Conventions

### Byte Order

All multi-byte integers are encoded in big-endian (network byte order).

Encoding example:
```c
uint32_t value = 0x12345678;
buffer[0] = (value >> 24) & 0xFF;  // 0x12
buffer[1] = (value >> 16) & 0xFF;  // 0x34
buffer[2] = (value >> 8) & 0xFF;   // 0x56
buffer[3] = value & 0xFF;          // 0x78
```

Use standard functions: `htonl()` / `ntohl()` for 32-bit, `htons()` / `ntohs()` for 16-bit.

### String Encoding

Strings are UTF-8 encoded, length-prefixed, NOT null-terminated.

Wire format:
```
┌──────────────────────┬──────────────────────┐
│ Length (2 bytes)     │ String bytes         │
│ Big-endian uint16    │ UTF-8 data           │
└──────────────────────┴──────────────────────┘
```

Example: "hello" encodes as:
```
0x0005 'h' 'e' 'l' 'l' 'o'
```

### Alignment

No padding. No struct alignment. Pack all fields tightly. Manually serialize/deserialize each field.

## Common Message Header

Every message starts with a 16-byte header:

```
┌────────────────────────────────────────────────┐
│ Offset │ Size │ Type    │ Field               │
├────────┼──────┼─────────┼─────────────────────┤
│ 0      │ 4    │ uint32  │ magic               │
│ 4      │ 2    │ uint16  │ version             │
│ 6      │ 2    │ uint16  │ msg_type            │
│ 8      │ 4    │ uint32  │ payload_len         │
│ 12     │ 4    │ uint32  │ reserved            │
└────────┴──────┴─────────┴─────────────────────┘
Total: 16 bytes
```

### Field Descriptions

- **magic** (0x57455645): ASCII "WEVE". Validates message integrity.
- **version**: Protocol version. Current: 0x0001.
- **msg_type**: Message type identifier (see Message Types section).
- **payload_len**: Length of data following the header, in bytes.
- **reserved**: Must be 0x00000000. Reserved for future use.

## Protocol Constants

```c
// Protocol version
#define PROTOCOL_VERSION_1      0x0001
#define MIN_SUPPORTED_VERSION   PROTOCOL_VERSION_1
#define MAX_SUPPORTED_VERSION   PROTOCOL_VERSION_1

// Message size limits
#define MAX_MESSAGE_SIZE        (10 * 1024 * 1024)  // 10 MB
```

Rationale:
- MIN_SUPPORTED_VERSION: Oldest protocol version accepted. Currently v1 is the only version.
- MAX_SUPPORTED_VERSION: Newest protocol version supported. Currently v1 is the only version.
- MAX_MESSAGE_SIZE: 10 MB allows for 2048x2048 RGBA images (16.7 MB uncompressed) with margin for overhead. Implementations should reject messages exceeding this size to prevent denial-of-service attacks.

## Message Types

```c
typedef enum {
    MSG_GENERATE_REQUEST  = 0x0001,
    MSG_GENERATE_RESPONSE = 0x0002,
    MSG_ERROR             = 0x00FF,
} message_type_t;
```

### MSG_GENERATE_REQUEST (0x0001)

Request to generate an image. See model-specific specifications for payload format.

### MSG_GENERATE_RESPONSE (0x0002)

Response containing generated image data or status.

### MSG_ERROR (0x00FF)

Error response with status code and human-readable message.

## Request Structure

All generation requests follow this structure:

```
┌──────────────────────────────────────────────────┐
│ Common Header (16 bytes)                         │
├──────────────────────────────────────────────────┤
│ Request ID (8 bytes, uint64)                     │
├──────────────────────────────────────────────────┤
│ Model ID (4 bytes, uint32)                       │
├──────────────────────────────────────────────────┤
│ Model-Specific Payload (variable)                │
│ ... see SPEC_SD35.md, etc. ...                   │
└──────────────────────────────────────────────────┘
```

### Common Request Fields

- **Request ID**: Client-generated unique identifier for request tracing. Echoed in response.
- **Model ID**: Identifies the model and payload format. See model-specific specs:
  - 0x00000000 = Stable Diffusion 3.5 (see SPEC_SD35.md)
  - Other values reserved for future models

## Response Structure

Responses use different msg_type values based on the status:
- Status 200 (success): msg_type = MSG_GENERATE_RESPONSE (0x0002)
- Status 400/500 (error): msg_type = MSG_ERROR (0x00FF)

### Success Response (Status 200)

```
┌──────────────────────────────────────────────────┐
│ Common Header (16 bytes)                         │
│ - msg_type = MSG_GENERATE_RESPONSE (0x0002)      │
├──────────────────────────────────────────────────┤
│ Request ID (8 bytes, uint64)                     │
│ - Echoed from request                            │
├──────────────────────────────────────────────────┤
│ Status Code (4 bytes, uint32)                    │
│ - 200 = success                                  │
├──────────────────────────────────────────────────┤
│ Generation Time (4 bytes, uint32)                │
│ - Milliseconds elapsed                           │
├──────────────────────────────────────────────────┤
│ Model-Specific Payload (variable)                │
│ ... image data, see model specs ...              │
└──────────────────────────────────────────────────┘
```

### Error Response (Status 400/500)

```
┌──────────────────────────────────────────────────┐
│ Common Header (16 bytes)                         │
│ - msg_type = MSG_ERROR (0x00FF)                  │
├──────────────────────────────────────────────────┤
│ Request ID (8 bytes, uint64)                     │
│ - Echoed from request, or 0 if request invalid   │
├──────────────────────────────────────────────────┤
│ Status Code (4 bytes, uint32)                    │
│ - 400 = client error, 500 = server error         │
├──────────────────────────────────────────────────┤
│ Error Code (4 bytes, uint32)                     │
│ - Machine-readable error identifier              │
├──────────────────────────────────────────────────┤
│ Error Message Length (2 bytes, uint16)           │
├──────────────────────────────────────────────────┤
│ Error Message (variable, UTF-8)                  │
│ - Human-readable error description               │
└──────────────────────────────────────────────────┘
```

## Status Codes

HTTP-like status codes for semantic clarity:

```c
typedef enum {
    STATUS_OK                   = 200,  // Success
    STATUS_BAD_REQUEST          = 400,  // Client error (invalid params)
    STATUS_INTERNAL_SERVER_ERROR = 500, // Server error (GPU failure, OOM, etc.)
} status_code_t;
```

### Status 200 (OK)

Request succeeded. Response contains generated image.

### Status 400 (Bad Request)

Client error. Request is malformed or contains invalid parameters.

Examples:
- Invalid magic number
- Unsupported protocol version
- Invalid model ID
- Out-of-range parameters (dimensions, steps, etc.)
- Prompt too long

### Status 500 (Internal Server Error)

Server error. Request was valid but generation failed.

Examples:
- GPU out of memory
- GPU driver error
- Internal assertion failure
- Unexpected exception

## Error Codes

Machine-readable error identifiers:

```c
typedef enum {
    ERR_NONE                = 0,
    ERR_INVALID_MAGIC       = 1,
    ERR_UNSUPPORTED_VERSION = 2,
    ERR_INVALID_MODEL_ID    = 3,
    ERR_INVALID_PROMPT      = 4,
    ERR_INVALID_DIMENSIONS  = 5,
    ERR_INVALID_STEPS       = 6,
    ERR_INVALID_CFG         = 7,
    ERR_OUT_OF_MEMORY       = 8,
    ERR_GPU_ERROR           = 9,
    ERR_TIMEOUT             = 10,
    ERR_INTERNAL            = 99,
} error_code_t;
```

Error codes are mapped to status codes:
- ERR_INVALID_* → Status 400
- ERR_OUT_OF_MEMORY, ERR_GPU_ERROR, ERR_TIMEOUT, ERR_INTERNAL → Status 500

## Version Negotiation

### Version Support Range

Implementations support a range of protocol versions:
- **MIN_SUPPORTED_VERSION**: Oldest version the implementation can handle
- **MAX_SUPPORTED_VERSION**: Newest version the implementation can handle
- Current: Both are 0x0001 (only v1 exists)

### Client Behavior

1. Client sends request with its MAX_SUPPORTED_VERSION (currently 0x0001).
2. Client reads response header to determine server's chosen version.
3. If server version > client MAX_SUPPORTED_VERSION, reject the response.
4. If server version < client MIN_SUPPORTED_VERSION, reject the response.
5. Otherwise, parse response using the version specified in response header.

### Server Behavior

1. Server reads request version from header.
2. If request version > server MAX_SUPPORTED_VERSION:
   - Return error 400 with ERR_UNSUPPORTED_VERSION
   - Error message: "Protocol version X.Y not supported (max: A.B)"
3. If request version < server MIN_SUPPORTED_VERSION:
   - Return error 400 with ERR_UNSUPPORTED_VERSION
   - Error message: "Protocol version X.Y too old (min: A.B)"
4. Otherwise, respond with request version (use what client requested).

### Version Negotiation Example

Client supports v1-v2, Server supports v1-v3:
1. Client sends request with version = 0x0002 (its max)
2. Server sees 0x0002, which is ≤ server max (0x0003)
3. Server responds with version = 0x0002 (use client's version)
4. Both use v2 for this exchange

### Forward Compatibility

New versions must be backward compatible:
- New fields appended to end of messages
- Old fields retain same offset and semantics
- Decoders ignore unknown trailing data
- Version bump only for breaking changes

### Deprecation Policy

When a version is deprecated:
1. Announce deprecation in release notes
2. Support for MIN_SUPPORTED_VERSION + 1 release cycles (at least 6 months)
3. Increment MIN_SUPPORTED_VERSION in a major release
4. Document migration path in changelog

## Example: Minimal Request

Minimal request header with model ID 0 (SD 3.5):

```
Offset  Hex                                 ASCII     Field
------  ----------------------------------  --------  ------------------
0000    57 45 56 45                         WEVE      magic
0004    00 01                               ..        version (1)
0006    00 01                               ..        msg_type (REQUEST)
0008    00 00 00 0C                         ....      payload_len (12)
000C    00 00 00 00                         ....      reserved
0010    00 00 00 00 00 00 00 01             ........  request_id (1)
0018    00 00 00 00                         ....      model_id (0)
001C    ... model-specific payload follows ...

Note: payload_len = 12 bytes (request_id + model_id).
Model-specific payload would add to this total.
```

## Example: Error Response

Error response for invalid model ID:

```
Offset  Hex                                 ASCII     Field
------  ----------------------------------  --------  ------------------
0000    57 45 56 45                         WEVE      magic
0004    00 01                               ..        version (1)
0006    00 FF                               ..        msg_type (ERROR)
0008    00 00 00 22                         ....      payload_len (34)
000C    00 00 00 00                         ....      reserved
0010    00 00 00 00 00 00 00 01             ........  request_id (1)
0018    00 00 01 90                         ....      status (400)
001C    00 00 00 03                         ....      error_code (3)
0020    00 10                               ..        msg_len (16)
0022    69 6E 76 61 6C 69 64 20 6D 6F 64    invalid mod
002D    65 6C 20 69 64                      el id     error_msg

Total: 16 (header) + 34 (payload) = 50 bytes

Payload breakdown:
- request_id: 8 bytes
- status: 4 bytes
- error_code: 4 bytes
- msg_len: 2 bytes
- error_msg: 16 bytes
Total payload: 8 + 4 + 4 + 2 + 16 = 34 bytes
```

## Safety Requirements

### Input Validation

All decoders MUST validate:

1. **Magic number**: Exactly 0x57455645
2. **Version**: <= MAX_SUPPORTED_VERSION
3. **Message type**: Known value from enum
4. **Payload length**: > 0 and <= MAX_MESSAGE_SIZE (define per implementation)
5. **All lengths**: Check for integer overflow before allocation
6. **Buffer bounds**: Verify data fits in buffer before copy

### Buffer Overflow Prevention

```c
// Check bounds before copy
if (data_len > buffer_size) {
    return ERR_BUFFER_TOO_SMALL;
}
memcpy(buffer, data, data_len);
```

### Integer Overflow Prevention

```c
// Check before arithmetic
if (payload_len > UINT32_MAX - header_len) {
    return ERR_OVERFLOW;
}
uint32_t total = header_len + payload_len;
```

### Defense in Depth

1. Protocol layer validates message structure
2. Model-specific layer validates parameters
3. Compute layer validates GPU constraints
4. All layers reject invalid input explicitly

## Testing Requirements

### Unit Tests

- Encode/decode round-trip for all message types
- Bounds checking (oversized fields)
- Integer overflow detection
- Truncated message handling
- Invalid magic/version/type rejection

### Fuzzing

- Fuzz all decoders with random input
- Target: 1M+ iterations without crash
- Use AFL or libFuzzer
- Fix all crashes before production

### Integration Tests

- End-to-end request/response over Unix socket
- Error condition handling
- Timeout handling
- Multiple sequential requests

## Implementation Notes

### Go Implementation

Use `encoding/binary` with `binary.BigEndian`:

```go
import "encoding/binary"

// Encode header
buf := new(bytes.Buffer)
binary.Write(buf, binary.BigEndian, uint32(0x57455645))
binary.Write(buf, binary.BigEndian, uint16(1))
binary.Write(buf, binary.BigEndian, uint16(MSG_GENERATE_REQUEST))
binary.Write(buf, binary.BigEndian, uint32(payloadLen))
binary.Write(buf, binary.BigEndian, uint32(0))
```

### C Implementation

Manual byte-by-byte packing:

```c
static inline uint32_t read_u32_be(const uint8_t *buf) {
    return ((uint32_t)buf[0] << 24) |
           ((uint32_t)buf[1] << 16) |
           ((uint32_t)buf[2] << 8) |
           ((uint32_t)buf[3]);
}

static inline void write_u32_be(uint8_t *buf, uint32_t value) {
    buf[0] = (value >> 24) & 0xFF;
    buf[1] = (value >> 16) & 0xFF;
    buf[2] = (value >> 8) & 0xFF;
    buf[3] = value & 0xFF;
}
```

Or use standard functions:

```c
#include <arpa/inet.h>

uint32_t value_host = 0x12345678;
uint32_t value_network = htonl(value_host);
```

## Model-Specific Specifications

- **Stable Diffusion 3.5**: See `SPEC_SD35.md`
- Future models: To be documented as added

## Revision History

- Version 1 (2025-12-31): Initial specification
