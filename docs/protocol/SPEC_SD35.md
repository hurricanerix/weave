# Weave Protocol: Stable Diffusion 3.5 Specification

Model ID: 0x00000000
Version: 1
Last Updated: 2025-12-31

## Overview

This document specifies the payload format for Stable Diffusion 3.5 generation requests and responses. This extends the common protocol defined in `SPEC.md`.

## Model Identifier

```c
#define MODEL_ID_SD35 0x00000000
```

Model ID 0 is reserved for Stable Diffusion 3.5. This is the only supported model in the MVP.

## SD 3.5 Architecture

Stable Diffusion 3.5 uses three text encoders:
- **CLIP-L**: OpenAI CLIP ViT-L/14
- **CLIP-G**: OpenCLIP ViT-G/14
- **T5**: Google T5-XXL

Each encoder processes the same prompt text independently. The protocol requires the prompt to be included three times in the request with an offset table specifying where each copy begins.

## Generation Request Payload

After the common request fields (header + request_id + model_id), the SD 3.5 payload contains:

```
┌─────────────────────────────────────────────────────┐
│ Offset │ Size │ Type    │ Field                      │
├────────┼──────┼─────────┼────────────────────────────┤
│ 0      │ 4    │ uint32  │ width                      │
│ 4      │ 4    │ uint32  │ height                     │
│ 8      │ 4    │ uint32  │ steps                      │
│ 12     │ 4    │ float32 │ cfg_scale                  │
│ 16     │ 8    │ uint64  │ seed                       │
│ 24     │ 4    │ uint32  │ clip_l_offset              │
│ 28     │ 4    │ uint32  │ clip_l_length              │
│ 32     │ 4    │ uint32  │ clip_g_offset              │
│ 36     │ 4    │ uint32  │ clip_g_length              │
│ 40     │ 4    │ uint32  │ t5_offset                  │
│ 44     │ 4    │ uint32  │ t5_length                  │
│ 48     │ var  │ bytes   │ prompt_data                │
└────────┴──────┴─────────┴────────────────────────────┘
Total: 48 bytes + prompt_data length
```

### Generation Parameters

#### width, height

Image dimensions in pixels.

**Constraints:**
- Type: uint32
- Range: 64 to 2048
- Must be multiple of 64 (SD 3.5 requires 64-pixel alignment)
- Recommended: 512, 768, 1024 for performance

**Validation:**
```c
if (width < 64 || width > 2048 || width % 64 != 0) {
    return ERR_INVALID_DIMENSIONS;
}
if (height < 64 || height > 2048 || height % 64 != 0) {
    return ERR_INVALID_DIMENSIONS;
}
```

#### steps

Number of denoising steps.

**Constraints:**
- Type: uint32
- Range: 1 to 100
- Recommended: 28 for SD 3.5 (model default)
- Higher steps = better quality, slower generation

**Validation:**
```c
if (steps < 1 || steps > 100) {
    return ERR_INVALID_STEPS;
}
```

#### cfg_scale

Classifier-Free Guidance scale. Controls prompt adherence.

**Constraints:**
- Type: float32 (IEEE 754 single precision)
- Range: 0.0 to 20.0
- Recommended: 7.0 (typical default)
- Higher CFG = stronger prompt following, potentially less creative

**Validation:**
```c
if (cfg_scale < 0.0f || cfg_scale > 20.0f || isnan(cfg_scale) || isinf(cfg_scale)) {
    return ERR_INVALID_CFG;
}
```

#### seed

Random seed for reproducible generation.

**Constraints:**
- Type: uint64
- Range: 0 to UINT64_MAX
- Special value: 0 = random seed (daemon generates seed)
- Same seed + same params + same prompt = same image

**Behavior:**
- If seed = 0: Daemon generates a random seed and uses it for generation. The response does NOT echo back the actual seed used. This means generations with seed=0 are non-reproducible.
- If seed > 0: Daemon uses the provided seed exactly. The same seed with identical parameters will produce identical output.

**Future enhancement:** A future protocol version may add an `actual_seed` field to the response to enable reproducibility even when seed=0 is requested.

### Prompt Offset Table

The prompt text is duplicated three times in `prompt_data`, once for each text encoder. The offset table specifies where each copy begins.

#### clip_l_offset, clip_l_length

Offset and length of CLIP-L prompt within `prompt_data`.

**Constraints:**
- Offset: Byte offset from start of `prompt_data` field
- Length: 1 to 2048 bytes (UTF-8 encoded)
- Offset + Length must not exceed total `prompt_data` size

#### clip_g_offset, clip_g_length

Offset and length of CLIP-G prompt within `prompt_data`.

**Constraints:**
- Same as CLIP-L

#### t5_offset, t5_length

Offset and length of T5 prompt within `prompt_data`.

**Constraints:**
- Same as CLIP-L

### Prompt Data Layout

The `prompt_data` field contains all three prompt copies. For the MVP, all three prompts MUST be identical.

**Maximum size constraints:**
- Per-encoder maximum: 2048 bytes (UTF-8)
- Total prompt_data maximum: 3 × 2048 = 6144 bytes
- Implementations must reject requests exceeding these limits with ERR_INVALID_PROMPT

**Example layout for prompt "a cat in space":**

```
Offset  Length  Content
------  ------  -----------------
0       14      "a cat in space"  (CLIP-L)
14      14      "a cat in space"  (CLIP-G)
28      14      "a cat in space"  (T5)
```

Offset table values:
- clip_l_offset = 0, clip_l_length = 14
- clip_g_offset = 14, clip_g_length = 14
- t5_offset = 28, t5_length = 14

**Total prompt_data size: 42 bytes**

### Prompt Duplication Requirement

For the MVP, the daemon expects all three prompts to be identical. Future versions may support different prompts per encoder for advanced use cases.

Encoder implementation must:
1. Verify all three offsets and lengths are valid (within bounds)
2. Extract three prompt strings
3. Optionally verify all three are identical (can log warning if different, but proceed)

### Validation Rules

Complete validation for SD 3.5 request:

```c
int validate_sd35_request(const struct sd35_request *req) {
    // Validate dimensions
    if (req->width < 64 || req->width > 2048 || req->width % 64 != 0) {
        return ERR_INVALID_DIMENSIONS;
    }
    if (req->height < 64 || req->height > 2048 || req->height % 64 != 0) {
        return ERR_INVALID_DIMENSIONS;
    }

    // Validate steps
    if (req->steps < 1 || req->steps > 100) {
        return ERR_INVALID_STEPS;
    }

    // Validate CFG
    if (req->cfg_scale < 0.0f || req->cfg_scale > 20.0f ||
        isnan(req->cfg_scale) || isinf(req->cfg_scale)) {
        return ERR_INVALID_CFG;
    }

    // Validate prompt lengths
    if (req->clip_l_length < 1 || req->clip_l_length > 2048) {
        return ERR_INVALID_PROMPT;
    }
    if (req->clip_g_length < 1 || req->clip_g_length > 2048) {
        return ERR_INVALID_PROMPT;
    }
    if (req->t5_length < 1 || req->t5_length > 2048) {
        return ERR_INVALID_PROMPT;
    }

    // Validate prompt offsets (bounds checking)
    // Note: prompt_data_len is calculated as:
    //   payload_len - 12 (common request fields) - 48 (SD35 params)
    // This represents the total size of the prompt_data field.
    size_t total_prompt_size = req->prompt_data_len;

    // Check CLIP-L bounds
    if (req->clip_l_offset > total_prompt_size ||
        req->clip_l_length > total_prompt_size - req->clip_l_offset) {
        return ERR_INVALID_PROMPT;
    }

    // Check CLIP-G bounds
    if (req->clip_g_offset > total_prompt_size ||
        req->clip_g_length > total_prompt_size - req->clip_g_offset) {
        return ERR_INVALID_PROMPT;
    }

    // Check T5 bounds
    if (req->t5_offset > total_prompt_size ||
        req->t5_length > total_prompt_size - req->t5_offset) {
        return ERR_INVALID_PROMPT;
    }

    return OK;
}
```

## Generation Response Payload

After the common response fields (header + request_id + status + generation_time), the SD 3.5 success response (status 200) contains:

```
┌─────────────────────────────────────────────────────┐
│ Offset │ Size │ Type    │ Field                      │
├────────┼──────┼─────────┼────────────────────────────┤
│ 0      │ 4    │ uint32  │ image_width                │
│ 4      │ 4    │ uint32  │ image_height               │
│ 8      │ 4    │ uint32  │ channels                   │
│ 12     │ 4    │ uint32  │ image_data_len             │
│ 16     │ var  │ bytes   │ image_data                 │
└────────┴──────┴─────────┴────────────────────────────┘
Total: 16 bytes + image_data_len
```

### Response Fields

#### image_width, image_height

Actual dimensions of generated image. Should match request dimensions.

**Type:** uint32
**Range:** Same as request (64 to 2048, multiple of 64)

#### channels

Number of color channels in image data.

**Type:** uint32
**Values:**
- 3 = RGB (8 bits per channel, 24 bits per pixel)
- 4 = RGBA (8 bits per channel, 32 bits per pixel)

SD 3.5 outputs RGB (channels = 3).

#### image_data_len

Size of raw image data in bytes.

**Type:** uint32
**Calculation:**
```c
image_data_len = image_width * image_height * channels
```

**Example:** 512x512 RGB image:
```c
image_data_len = 512 * 512 * 3 = 786432 bytes
```

**Validation:**
```c
// Check for integer overflow
if (width > UINT32_MAX / height ||
    width * height > UINT32_MAX / channels) {
    return ERR_OVERFLOW;
}
uint32_t expected_len = width * height * channels;
if (image_data_len != expected_len) {
    return ERR_INVALID_DIMENSIONS;
}
```

#### image_data

Raw pixel data in packed RGB format.

**Format:** Scanline order, top-to-bottom, left-to-right.

**Layout for RGB (channels = 3):**
```
Pixel[0,0]:   R G B
Pixel[1,0]:   R G B
...
Pixel[W-1,0]: R G B
Pixel[0,1]:   R G B
...
Pixel[W-1,H-1]: R G B
```

**Pixel indexing:**
```c
// Get pixel at (x, y)
size_t offset = (y * width + x) * channels;
uint8_t red   = image_data[offset + 0];
uint8_t green = image_data[offset + 1];
uint8_t blue  = image_data[offset + 2];
```

**No stride/padding:** Each scanline is tightly packed. No alignment padding between rows.

## Example Request

Generate 512x512 image with prompt "a cat in space", 28 steps, CFG 7.0, random seed:

```
Offset  Hex                                 ASCII     Field
------  ----------------------------------  --------  ------------------
Common Header (16 bytes)
0000    57 45 56 45                         WEVE      magic
0004    00 01                               ..        version (1)
0006    00 01                               ..        msg_type (REQUEST)
0008    00 00 00 66                         ....      payload_len (102)
000C    00 00 00 00                         ....      reserved

Common Request Fields (12 bytes)
0010    00 00 00 00 00 00 00 01             ........  request_id (1)
0018    00 00 00 00                         ....      model_id (0 = SD35)

SD 3.5 Payload (70 bytes)
001C    00 00 02 00                         ....      width (512)
0020    00 00 02 00                         ....      height (512)
0024    00 00 00 1C                         ....      steps (28)
0028    40 E0 00 00                         @...      cfg_scale (7.0)
002C    00 00 00 00 00 00 00 00             ........  seed (0 = random)
0034    00 00 00 00                         ....      clip_l_offset (0)
0038    00 00 00 0E                         ....      clip_l_length (14)
003C    00 00 00 0E                         ....      clip_g_offset (14)
0040    00 00 00 0E                         ....      clip_g_length (14)
0044    00 00 00 1C                         ....      t5_offset (28)
0048    00 00 00 0E                         ....      t5_length (14)

Prompt Data (42 bytes)
004C    61 20 63 61 74 20 69 6E 20 73 70    a cat in sp
0057    61 63 65                            ace       CLIP-L prompt
005A    61 20 63 61 74 20 69 6E 20 73 70    a cat in sp
0065    61 63 65                            ace       CLIP-G prompt
0068    61 20 63 61 74 20 69 6E 20 73 70    a cat in sp
0073    61 63 65                            ace       T5 prompt

Payload breakdown:
- Common request fields: request_id (8) + model_id (4) = 12 bytes
- SD 3.5 params: width (4) + height (4) + steps (4) + cfg (4) + seed (8) + offset table (24) = 48 bytes
- Prompt data: 42 bytes
Total payload: 12 + 48 + 42 = 102 bytes (not including 16-byte common header)
Total message: 16 (common header) + 102 (payload) = 118 bytes
```

## Example Response

Success response with 512x512 RGB image (simplified, actual image data truncated):

```
Offset  Hex                                 ASCII     Field
------  ----------------------------------  --------  ------------------
Common Header (16 bytes)
0000    57 45 56 45                         WEVE      magic
0004    00 01                               ..        version (1)
0006    00 02                               ..        msg_type (RESPONSE)
0008    00 0C 00 20                         ....      payload_len (786464)
000C    00 00 00 00                         ....      reserved

Common Response Fields (16 bytes)
0010    00 00 00 00 00 00 00 01             ........  request_id (1)
0018    00 00 00 C8                         ....      status (200 = OK)
001C    00 00 27 10                         ..'.      generation_time (10000ms)

SD 3.5 Response Payload (16 bytes + 786432 bytes image)
0020    00 00 02 00                         ....      image_width (512)
0024    00 00 02 00                         ....      image_height (512)
0028    00 00 00 03                         ....      channels (3 = RGB)
002C    00 0C 00 00                         ....      image_data_len (786432)
0030    [RGB pixel data: 786432 bytes]

Total: 16 (header) + 16 (response) + 16 (image header) + 786432 (pixels)
     = 786480 bytes
```

## Example Error Response

Error: Invalid model ID (client sent model_id = 1):

```
Offset  Hex                                 ASCII     Field
------  ----------------------------------  --------  ------------------
Common Header (16 bytes)
0000    57 45 56 45                         WEVE      magic
0004    00 01                               ..        version (1)
0006    00 FF                               ..        msg_type (ERROR)
0008    00 00 00 26                         ....      payload_len (38)
000C    00 00 00 00                         ....      reserved

Error Response Fields (18 bytes + message)
0010    00 00 00 00 00 00 00 01             ........  request_id (1)
0018    00 00 01 90                         ....      status (400)
001C    00 00 00 03                         ....      error_code (3 = INVALID_MODEL)
0020    00 14                               ..        error_msg_len (20)
0022    75 6E 73 75 70 70 6F 72 74 65 64    unsupported
002D    20 6D 6F 64 65 6C 20 49 44           model ID

Total: 16 (header) + 38 (payload) = 54 bytes

Payload breakdown:
- request_id: 8 bytes
- status: 4 bytes
- error_code: 4 bytes
- error_msg_len: 2 bytes
- error_msg: 20 bytes
Total payload: 8 + 4 + 4 + 2 + 20 = 38 bytes
```

## Parameter Bounds Summary

| Parameter  | Type    | Min   | Max    | Notes                        |
|------------|---------|-------|--------|------------------------------|
| width      | uint32  | 64    | 2048   | Multiple of 64               |
| height     | uint32  | 64    | 2048   | Multiple of 64               |
| steps      | uint32  | 1     | 100    | Recommended: 28              |
| cfg_scale  | float32 | 0.0   | 20.0   | Recommended: 7.0             |
| seed       | uint64  | 0     | MAX    | 0 = random                   |
| prompt_len | uint16  | 1     | 2048   | Per encoder, UTF-8 bytes     |

## Implementation Checklist

### Encoder (Go)

- [ ] Compute payload_len correctly (12 + 48 + prompt_data_len)
  - Common request fields: 12 bytes (request_id + model_id)
  - SD 3.5 params: 48 bytes
  - Prompt data: 3 * prompt_len bytes (three copies of prompt)
- [ ] Write generation params in big-endian
- [ ] Encode float32 cfg_scale using IEEE 754 (math.Float32bits)
- [ ] Duplicate prompt three times
- [ ] Set offset table correctly (0, len, 2*len)
- [ ] Validate all parameters before encoding

### Decoder (C)

- [ ] Validate model_id == 0, reject others with status 400
- [ ] Parse all generation params with bounds checking
- [ ] Validate dimensions: range and 64-pixel alignment
- [ ] Validate steps: 1 to 100
- [ ] Validate cfg_scale: 0.0 to 20.0, not NaN/Inf
- [ ] Validate prompt offsets: no overflow, within bounds
- [ ] Extract three prompt strings (CLIP-L, CLIP-G, T5)
- [ ] No buffer overflows when copying prompts

### Response Encoder (C)

- [ ] Compute image_data_len = width * height * channels
- [ ] Check for integer overflow in size calculation
- [ ] Set channels = 3 for RGB output
- [ ] Pack pixel data tightly (no padding)
- [ ] Set generation_time_ms to actual elapsed time

### Response Decoder (Go)

- [ ] Parse response header, check status
- [ ] If status 200: extract image dimensions, channels, data
- [ ] Validate image_data_len matches dimensions
- [ ] If status 400/500: extract error code and message
- [ ] Handle truncated responses gracefully

## Testing

### Unit Test Cases

#### Request Encoding (Go)

1. Valid request with all parameters in range
2. Boundary values (min/max dimensions, steps, CFG)
3. Prompt duplication correctness
4. Offset table calculation
5. Payload length calculation

#### Request Decoding (C)

1. Valid SD 3.5 request
2. Invalid model ID (should return status 400)
3. Out-of-range dimensions (< 64, > 2048, not multiple of 64)
4. Out-of-range steps (0, 101)
5. Out-of-range CFG (< 0, > 20, NaN, Inf)
6. Invalid prompt offsets (out of bounds)
7. Overlapping prompt regions
8. Truncated request (missing prompt data)
9. Integer overflow in offset + length

#### Response Encoding (C)

1. Stubbed 512x512 RGB image (test pattern)
2. Different dimensions (64x64, 1024x1024)
3. Error response (status 400 with message)
4. Generation time tracking

#### Response Decoding (Go)

1. Valid 200 response with image data
2. Error response (status 400)
3. Error response (status 500)
4. Truncated image data
5. Mismatched image_data_len

### Integration Test

1. Go encodes SD 3.5 request (512x512, "test prompt")
2. C decodes request, validates all fields
3. C generates stubbed image (checkerboard or gradient)
4. C encodes response with stubbed data
5. Go decodes response
6. Verify image dimensions match request
7. Verify pixel data integrity (checksum or pattern match)

### Fuzzing

Target: C request decoder
- Random byte sequences
- Valid headers with corrupt payloads
- Extreme parameter values
- Overlapping/invalid offset tables

Goal: 1M+ iterations without crash, leak, or UB.

## Future Enhancements

These are out of scope for MVP but documented for reference:

- Different prompts per encoder (for advanced use cases)
- Negative prompts
- Additional models (model_id > 0)
- Streaming progress updates
- Request cancellation
- LoRA/ControlNet extensions

## Revision History

- Version 1 (2025-12-31): Initial specification for MVP
