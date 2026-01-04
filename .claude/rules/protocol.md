---
paths:
  - "**/protocol/**"
  - "protocol/**"
---

# Binary Protocol Rules for Weave

## Philosophy

The protocol is the contract between Go and C. Get it wrong and both sides break. Get it right and they never think about each other.

**Design Priorities:**
1. **Safety**: No buffer overflows, no integer overflows
2. **Versioning**: Must support protocol evolution
3. **Performance**: Zero-copy where possible
4. **Simplicity**: Easy to implement correctly in both languages

## Protocol Requirements

### Binary Format

**Use binary, not text:**
- ✅ Fixed-size integers (uint32_t, uint64_t)
- ✅ Length-prefixed strings
- ✅ Network byte order (big-endian)
- ❌ NO JSON (parsing in C is a nightmare)
- ❌ NO text protocols (too error-prone)

### Versioning

**Every message includes protocol version:**

```
┌────────────────────────────────────────┐
│ Magic Number (4 bytes): 0x57455645     │  "WEVE"
│ Protocol Version (2 bytes): 0x0001     │  v1
│ Message Type (2 bytes)                 │  Request/Response
│ Payload Length (4 bytes)               │  Size of rest
│ Payload (variable)                     │  Message body
└────────────────────────────────────────┘
```

**Version handling:**
- Client sends version it supports
- Server responds with version it will use
- Minimum version: both must support it
- Reject unsupported versions explicitly

### Message Types

```c
typedef enum {
    MSG_GENERATE_REQUEST = 0x0001,
    MSG_GENERATE_RESPONSE = 0x0002,
    MSG_STATUS_REQUEST = 0x0003,
    MSG_STATUS_RESPONSE = 0x0004,
    MSG_ERROR = 0x00FF,
} message_type_t;
```

## Authentication

### Socket-Level Auth (SO_PEERCRED)

Authentication happens at the socket level, not the protocol level. The daemon uses `SO_PEERCRED` to verify the connecting process's identity.

**Immediately after `accept()`:**
```c
struct ucred cred;
socklen_t len = sizeof(cred);
getsockopt(client_fd, SOL_SOCKET, SO_PEERCRED, &cred, &len);
```

**Authorization check:**
- Userland mode: `cred.uid == getuid()` (same user only)
- System mode: `cred.gid` in allowed group list

**On rejection:**
- Close socket immediately
- Do NOT send any response
- Log rejection for debugging (UID, PID)

**Why socket-level, not protocol-level:**
- Kernel guarantees UID/GID are unforgeable
- No token management, no token files
- Simpler wire format
- Authentication happens before any data is read

## Message Specifications

### Generate Request

```c
struct generate_request {
    // Header (16 bytes)
    uint32_t magic;         // 0x57455645 ("WEVE")
    uint16_t version;       // Protocol version
    uint16_t msg_type;      // MSG_GENERATE_REQUEST
    uint32_t payload_len;   // Length of everything after header

    // Request metadata (16 bytes)
    uint64_t request_id;    // Unique request ID for tracing
    uint32_t model_id;      // Model identifier (from config)
    uint32_t reserved;      // Padding for alignment

    // Generation params (24 bytes)
    uint32_t width;         // Image width
    uint32_t height;        // Image height
    uint32_t steps;         // Inference steps
    float guidance;         // Guidance scale
    uint64_t seed;          // Random seed (0 = random)

    // Prompt (variable)
    uint16_t prompt_len;    // Length of prompt string
    char prompt[];          // UTF-8 encoded, NOT null-terminated
};
```

**Constraints:**
- `prompt_len`: 1 to 2048 bytes
- `width`, `height`: 64 to 2048, multiple of 64
- `steps`: 1 to 100
- `guidance`: 0.0 to 20.0

**Validation:**
```c
int validate_generate_request(const struct generate_request *req) {
    if (req->magic != 0x57455645) {
        return ERR_INVALID_MAGIC;
    }
    if (req->version > MAX_SUPPORTED_VERSION) {
        return ERR_UNSUPPORTED_VERSION;
    }
    if (req->prompt_len == 0 || req->prompt_len > 2048) {
        return ERR_INVALID_PROMPT_LEN;
    }
    if (req->width < 64 || req->width > 2048 || req->width % 64 != 0) {
        return ERR_INVALID_DIMENSIONS;
    }
    // ... more checks
    return OK;
}
```

### Generate Response

```c
struct generate_response {
    // Header (16 bytes)
    uint32_t magic;
    uint16_t version;
    uint16_t msg_type;      // MSG_GENERATE_RESPONSE
    uint32_t payload_len;
    
    // Response metadata
    uint64_t request_id;    // Echoed from request
    uint32_t status;        // 0 = success, else error code
    uint32_t generation_time_ms;
    
    // Image data
    uint32_t image_width;
    uint32_t image_height;
    uint32_t image_format;  // 0 = PNG, 1 = JPEG
    uint32_t image_len;     // Bytes in image data
    uint8_t image_data[];   // Raw image bytes
};
```

### Error Response

```c
struct error_response {
    uint32_t magic;
    uint16_t version;
    uint16_t msg_type;      // MSG_ERROR
    uint32_t payload_len;
    
    uint64_t request_id;    // Original request ID
    uint32_t error_code;    // Error code
    uint16_t error_msg_len; // Length of error message
    char error_msg[];       // Human-readable error (UTF-8)
};
```

**Standard Error Codes:**
```c
typedef enum {
    ERR_NONE = 0,
    ERR_INVALID_MAGIC = 1,
    ERR_UNSUPPORTED_VERSION = 2,
    ERR_INVALID_AUTH = 3,
    ERR_INVALID_PROMPT = 4,
    ERR_INVALID_DIMENSIONS = 5,
    ERR_OUT_OF_MEMORY = 6,
    ERR_GPU_ERROR = 7,
    ERR_TIMEOUT = 8,
    ERR_INTERNAL = 99,
} error_code_t;
```

## Wire Format

### Encoding Rules

**1. All multi-byte integers are big-endian (network byte order):**

```c
// Encoding
uint32_t value = 0x12345678;
buffer[0] = (value >> 24) & 0xFF;  // 0x12
buffer[1] = (value >> 16) & 0xFF;  // 0x34
buffer[2] = (value >> 8) & 0xFF;   // 0x56
buffer[3] = value & 0xFF;          // 0x78

// Decoding
uint32_t value = ((uint32_t)buffer[0] << 24) |
                 ((uint32_t)buffer[1] << 16) |
                 ((uint32_t)buffer[2] << 8) |
                 ((uint32_t)buffer[3]);
```

**Or use `htonl()` / `ntohl()`:**

```c
uint32_t value_host = 0x12345678;
uint32_t value_network = htonl(value_host);
```

**2. Strings are length-prefixed, NOT null-terminated:**

```
┌──────────────────────┬──────────────────────┐
│ Length (2 bytes)     │ String bytes         │
│ 0x0005               │ "hello" (5 bytes)    │
└──────────────────────┴──────────────────────┘
```

**3. No padding, no alignment:**

Structs in memory ≠ wire format. Manually pack/unpack.

### Example Encoding (Go)

```go
func EncodeGenerateRequest(req *GenerateRequest) ([]byte, error) {
    buf := new(bytes.Buffer)

    // Header (16 bytes)
    binary.Write(buf, binary.BigEndian, uint32(0x57455645))
    binary.Write(buf, binary.BigEndian, uint16(1))
    binary.Write(buf, binary.BigEndian, uint16(MSG_GENERATE_REQUEST))

    // Compute payload length: metadata + params + prompt_len + prompt
    payloadLen := 16 + 24 + 2 + len(req.Prompt)
    binary.Write(buf, binary.BigEndian, uint32(payloadLen))

    // Request metadata (16 bytes)
    binary.Write(buf, binary.BigEndian, req.RequestID)
    binary.Write(buf, binary.BigEndian, req.ModelID)
    binary.Write(buf, binary.BigEndian, uint32(0)) // reserved

    // Params (24 bytes)
    binary.Write(buf, binary.BigEndian, req.Width)
    binary.Write(buf, binary.BigEndian, req.Height)
    binary.Write(buf, binary.BigEndian, req.Steps)
    binary.Write(buf, binary.BigEndian, math.Float32bits(req.Guidance))
    binary.Write(buf, binary.BigEndian, req.Seed)

    // Prompt
    binary.Write(buf, binary.BigEndian, uint16(len(req.Prompt)))
    buf.WriteString(req.Prompt)

    return buf.Bytes(), nil
}
```

### Example Decoding (C)

```c
int decode_generate_request(const uint8_t *data, size_t len,
                            generate_request_t *req) {
    if (len < 56) {  // Minimum size: header + metadata + params + prompt_len
        return ERR_TRUNCATED;
    }

    const uint8_t *ptr = data;

    // Header (16 bytes)
    req->magic = read_u32_be(ptr); ptr += 4;
    req->version = read_u16_be(ptr); ptr += 2;
    req->msg_type = read_u16_be(ptr); ptr += 2;
    req->payload_len = read_u32_be(ptr); ptr += 4;

    if (req->magic != 0x57455645) {
        return ERR_INVALID_MAGIC;
    }

    // Request metadata (16 bytes)
    req->request_id = read_u64_be(ptr); ptr += 8;
    req->model_id = read_u32_be(ptr); ptr += 4;
    ptr += 4;  // Skip reserved

    // Params (24 bytes)
    req->width = read_u32_be(ptr); ptr += 4;
    req->height = read_u32_be(ptr); ptr += 4;
    req->steps = read_u32_be(ptr); ptr += 4;
    req->guidance = read_f32_be(ptr); ptr += 4;
    req->seed = read_u64_be(ptr); ptr += 8;

    // Prompt
    req->prompt_len = read_u16_be(ptr); ptr += 2;
    if (req->prompt_len > 2048) {
        return ERR_INVALID_PROMPT_LEN;
    }
    req->prompt = (const char *)ptr; ptr += req->prompt_len;

    return OK;
}
```

## Transport

### Unix Domain Socket

**Socket path**:
- Userland mode: `$XDG_RUNTIME_DIR/weave/weave.sock`
- System mode: `/run/weave/weave.sock`

**Permissions**:
- Socket: 0600 (owner only) in userland mode
- Socket: 0660 (owner + group) in system mode
- Directory: 0700 (owner only)

**Connection lifetime**:
- Client connects for each request
- Send request → receive response → close
- OR: Keep connection open, send multiple requests
- Server must handle both patterns

**Timeout**:
- Read timeout: 60 seconds (generation can be slow)
- Write timeout: 5 seconds
- Connection idle timeout: 5 minutes

## Safety Considerations

### Buffer Overflows

**Always check bounds before writing:**

```c
// ❌ BAD
memcpy(buffer, data, data_len);

// ✅ GOOD
if (data_len > buffer_size) {
    return ERR_BUFFER_TOO_SMALL;
}
memcpy(buffer, data, data_len);
```

### Integer Overflows

**Check before arithmetic:**

```c
// ❌ BAD
uint32_t total = header_len + payload_len;

// ✅ GOOD
if (payload_len > UINT32_MAX - header_len) {
    return ERR_OVERFLOW;
}
uint32_t total = header_len + payload_len;
```

### Malicious Input

**Assume all input is hostile:**

- Validate magic number
- Validate version
- Validate message type
- Validate all lengths
- Validate all parameters
- Check for integer overflows
- Check for out-of-bounds access

**Defense in depth:**
1. Input validation at protocol layer
2. Input validation at compute layer
3. Bounds checking everywhere
4. Timeouts on all operations

## Testing Protocol

### Unit Tests

Test encoding/decoding:

```c
void test_encode_decode_generate_request(void) {
    generate_request_t req = {
        .magic = 0x57455645,
        .version = 1,
        .prompt = "test prompt",
        .prompt_len = 11,
        .width = 512,
        .height = 512,
    };
    
    uint8_t buffer[4096];
    size_t encoded_len;
    
    int err = encode_generate_request(&req, buffer, sizeof(buffer), &encoded_len);
    assert(err == OK);
    
    generate_request_t decoded;
    err = decode_generate_request(buffer, encoded_len, &decoded);
    assert(err == OK);
    
    assert(decoded.magic == req.magic);
    assert(decoded.width == req.width);
    assert(memcmp(decoded.prompt, req.prompt, req.prompt_len) == 0);
}
```

### Fuzzing

Use AFL or libFuzzer:

```c
int LLVMFuzzerTestOneInput(const uint8_t *data, size_t size) {
    generate_request_t req;
    decode_generate_request(data, size, &req);
    return 0;  // Return value doesn't matter
}
```

### Integration Tests

Test actual socket communication:

```go
func TestRoundTrip(t *testing.T) {
    // Start daemon
    daemon := startTestDaemon(t)
    defer daemon.Stop()
    
    // Send request
    client := NewClient(daemon.SocketPath())
    img, err := client.Generate(context.Background(), "test")
    require.NoError(t, err)
    require.NotNil(t, img)
}
```

## Evolution

### Adding New Fields

**Append fields to end of message:**

```c
// v1
struct generate_request_v1 {
    // ... existing fields ...
};

// v2 - adds new field at end
struct generate_request_v2 {
    // ... existing fields from v1 ...
    uint32_t new_field;  // New in v2
};
```

**Decoder handles both versions:**

```c
int decode_generate_request(const uint8_t *data, size_t len, 
                            generate_request_t *req) {
    // Decode v1 fields
    // ...
    
    if (req->version >= 2 && remaining_bytes >= 4) {
        req->new_field = read_u32_be(ptr);
    } else {
        req->new_field = DEFAULT_VALUE;
    }
    
    return OK;
}
```

### Deprecating Old Versions

1. Add support for new version
2. Run both versions for N releases
3. Announce deprecation
4. Remove old version

**Never break wire compatibility within a major version.**

## Documentation

Document protocol in `protocol/spec.md` with:
- Wire format diagrams
- Example messages (hex dumps)
- Error codes and meanings
- Version history
- Migration guides

Keep `protocol/spec.md` synchronized with code. If they diverge, the implementation is authoritative.
