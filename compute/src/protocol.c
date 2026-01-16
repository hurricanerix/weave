/**
 * Weave Binary Protocol - Request Decoder Implementation
 *
 * This file implements the protocol request decoder for weave-compute.
 * It parses incoming binary messages from the Go orchestration layer.
 *
 * Safety principles:
 * - All bounds are checked before access
 * - All integer arithmetic checked for overflow
 * - All input validated before use
 * - No undefined behavior
 *
 * Protocol version: 1
 * Specification: docs/protocol/SPEC.md, docs/protocol/SPEC_SD35.md
 */

#include <stdint.h>
#include <stddef.h>
#include <string.h>
#include <math.h>
#include "weave/protocol.h"

/**
 * Byte-swapping helpers for big-endian decoding
 */

static inline uint16_t read_u16_be(const uint8_t *buf) {
    return ((uint16_t)buf[0] << 8) |
           ((uint16_t)buf[1]);
}

static inline uint32_t read_u32_be(const uint8_t *buf) {
    return ((uint32_t)buf[0] << 24) |
           ((uint32_t)buf[1] << 16) |
           ((uint32_t)buf[2] << 8) |
           ((uint32_t)buf[3]);
}

static inline uint64_t read_u64_be(const uint8_t *buf) {
    return ((uint64_t)buf[0] << 56) |
           ((uint64_t)buf[1] << 48) |
           ((uint64_t)buf[2] << 40) |
           ((uint64_t)buf[3] << 32) |
           ((uint64_t)buf[4] << 24) |
           ((uint64_t)buf[5] << 16) |
           ((uint64_t)buf[6] << 8) |
           ((uint64_t)buf[7]);
}

static inline float read_f32_be(const uint8_t *buf) {
    uint32_t bits = read_u32_be(buf);
    float result;
    memcpy(&result, &bits, sizeof(float));
    return result;
}

/**
 * Byte-swapping helpers for big-endian encoding
 */

static inline void write_u16_be(uint8_t *buf, uint16_t value) {
    buf[0] = (value >> 8) & 0xFF;
    buf[1] = value & 0xFF;
}

static inline void write_u32_be(uint8_t *buf, uint32_t value) {
    buf[0] = (value >> 24) & 0xFF;
    buf[1] = (value >> 16) & 0xFF;
    buf[2] = (value >> 8) & 0xFF;
    buf[3] = value & 0xFF;
}

static inline void write_u64_be(uint8_t *buf, uint64_t value) {
    buf[0] = (value >> 56) & 0xFF;
    buf[1] = (value >> 48) & 0xFF;
    buf[2] = (value >> 40) & 0xFF;
    buf[3] = (value >> 32) & 0xFF;
    buf[4] = (value >> 24) & 0xFF;
    buf[5] = (value >> 16) & 0xFF;
    buf[6] = (value >> 8) & 0xFF;
    buf[7] = value & 0xFF;
}

/**
 * decode_protocol_header - Decode and validate common protocol header
 *
 * @param data      Input buffer (must be at least 16 bytes)
 * @param data_len  Size of input buffer
 * @param header    Output header structure
 * @return          ERR_NONE on success, error code on failure
 */
static error_code_t decode_protocol_header(const uint8_t *data, size_t data_len,
                                           protocol_header_t *header) {
    if (data_len < 16) {
        return ERR_INTERNAL;
    }

    header->magic = read_u32_be(data);
    header->version = read_u16_be(data + 4);
    header->msg_type = read_u16_be(data + 6);
    header->payload_len = read_u32_be(data + 8);
    header->reserved = read_u32_be(data + 12);

    if (header->magic != PROTOCOL_MAGIC) {
        return ERR_INVALID_MAGIC;
    }

    if (header->version < MIN_SUPPORTED_VERSION ||
        header->version > MAX_SUPPORTED_VERSION) {
        return ERR_UNSUPPORTED_VERSION;
    }

    if (header->msg_type != MSG_GENERATE_REQUEST) {
        return ERR_INTERNAL;
    }

    if (header->payload_len > MAX_MESSAGE_SIZE - 16) {
        return ERR_INTERNAL;
    }

    return ERR_NONE;
}

/**
 * decode_generate_request - Decode and validate SD 3.5 generation request
 *
 * This function parses a complete binary protocol message containing a
 * generation request. It validates all fields according to the protocol
 * specification and returns appropriate error codes for invalid input.
 *
 * Message structure:
 * - Common header (16 bytes)
 * - Request ID (8 bytes)
 * - Model ID (4 bytes)
 * - SD 3.5 parameters (48 bytes)
 * - Prompt data (variable)
 *
 * @param data      Input buffer containing complete message
 * @param data_len  Size of input buffer (must include header + payload)
 * @param req       Output request structure (populated on success)
 * @return          ERR_NONE on success, error code on failure
 *
 * Error codes:
 * - ERR_INVALID_MAGIC: Magic number mismatch
 * - ERR_UNSUPPORTED_VERSION: Protocol version not supported
 * - ERR_INVALID_MODEL_ID: model_id is not 0 (SD 3.5)
 * - ERR_INVALID_DIMENSIONS: width/height out of range or not aligned
 * - ERR_INVALID_STEPS: steps out of range
 * - ERR_INVALID_CFG: cfg_scale out of range, NaN, or Inf
 * - ERR_INVALID_PROMPT: prompt offset/length out of bounds
 * - ERR_INTERNAL: Truncated message or other structural error
 */
error_code_t decode_generate_request(const uint8_t *data, size_t data_len,
                                     sd35_generate_request_t *req) {
    if (data == NULL || req == NULL) {
        return ERR_INTERNAL;
    }

    if (data_len < 16) {
        return ERR_INTERNAL;
    }

    protocol_header_t header;
    error_code_t err = decode_protocol_header(data, data_len, &header);
    if (err != ERR_NONE) {
        return err;
    }

    if (data_len < 16 + header.payload_len) {
        return ERR_INTERNAL;
    }

    if (header.payload_len < 12 + 48) {
        return ERR_INTERNAL;
    }

    const uint8_t *ptr = data + 16;
    size_t remaining = header.payload_len;

    req->request_id = read_u64_be(ptr);
    ptr += 8;
    remaining -= 8;

    req->model_id = read_u32_be(ptr);
    ptr += 4;
    remaining -= 4;

    if (req->model_id != MODEL_ID_SD35) {
        return ERR_INVALID_MODEL_ID;
    }

    if (remaining < 48) {
        return ERR_INTERNAL;
    }

    req->width = read_u32_be(ptr);
    ptr += 4;

    req->height = read_u32_be(ptr);
    ptr += 4;

    req->steps = read_u32_be(ptr);
    ptr += 4;

    req->cfg_scale = read_f32_be(ptr);
    ptr += 4;

    req->seed = read_u64_be(ptr);
    ptr += 8;

    req->clip_l_offset = read_u32_be(ptr);
    ptr += 4;

    req->clip_l_length = read_u32_be(ptr);
    ptr += 4;

    req->clip_g_offset = read_u32_be(ptr);
    ptr += 4;

    req->clip_g_length = read_u32_be(ptr);
    ptr += 4;

    req->t5_offset = read_u32_be(ptr);
    ptr += 4;

    req->t5_length = read_u32_be(ptr);
    ptr += 4;

    remaining -= 48;

    req->prompt_data = ptr;
    req->prompt_data_len = remaining;

    if (req->width < SD35_MIN_DIMENSION || req->width > SD35_MAX_DIMENSION ||
        req->width % SD35_DIMENSION_ALIGNMENT != 0) {
        return ERR_INVALID_DIMENSIONS;
    }

    if (req->height < SD35_MIN_DIMENSION || req->height > SD35_MAX_DIMENSION ||
        req->height % SD35_DIMENSION_ALIGNMENT != 0) {
        return ERR_INVALID_DIMENSIONS;
    }

    if (req->steps < SD35_MIN_STEPS || req->steps > SD35_MAX_STEPS) {
        return ERR_INVALID_STEPS;
    }

    if (req->cfg_scale < SD35_MIN_CFG || req->cfg_scale > SD35_MAX_CFG ||
        isnan(req->cfg_scale) || isinf(req->cfg_scale)) {
        return ERR_INVALID_CFG;
    }

    if (req->clip_l_length < SD35_MIN_PROMPT_LENGTH ||
        req->clip_l_length > SD35_MAX_PROMPT_LENGTH) {
        return ERR_INVALID_PROMPT;
    }

    if (req->clip_g_length < SD35_MIN_PROMPT_LENGTH ||
        req->clip_g_length > SD35_MAX_PROMPT_LENGTH) {
        return ERR_INVALID_PROMPT;
    }

    if (req->t5_length < SD35_MIN_PROMPT_LENGTH ||
        req->t5_length > SD35_MAX_PROMPT_LENGTH) {
        return ERR_INVALID_PROMPT;
    }

    if (req->clip_l_offset > req->prompt_data_len) {
        return ERR_INVALID_PROMPT;
    }

    if (req->clip_l_length > req->prompt_data_len - req->clip_l_offset) {
        return ERR_INVALID_PROMPT;
    }

    if (req->clip_g_offset > req->prompt_data_len) {
        return ERR_INVALID_PROMPT;
    }

    if (req->clip_g_length > req->prompt_data_len - req->clip_g_offset) {
        return ERR_INVALID_PROMPT;
    }

    if (req->t5_offset > req->prompt_data_len) {
        return ERR_INVALID_PROMPT;
    }

    if (req->t5_length > req->prompt_data_len - req->t5_offset) {
        return ERR_INVALID_PROMPT;
    }

    return ERR_NONE;
}

/**
 * encode_generate_response - Encode SD 3.5 generation response
 *
 * This function encodes a successful generation response into the binary
 * protocol format. The response contains the generated image data in raw
 * RGB format.
 *
 * Message structure:
 * - Common header (16 bytes)
 * - Common response fields: request_id (8), status (4), generation_time_ms (4)
 * - Image metadata: width (4), height (4), channels (4), image_data_len (4)
 * - Raw image data (width * height * channels bytes)
 *
 * @param resp      Response structure to encode
 * @param buffer    Output buffer for encoded message
 * @param buf_size  Size of output buffer in bytes
 * @param out_len   Pointer to store actual encoded length (bytes written)
 * @return          ERR_NONE on success, error code on failure
 *
 * Error codes:
 * - ERR_INTERNAL: NULL pointer, invalid parameters, or buffer too small
 * - ERR_INVALID_DIMENSIONS: Image dimensions invalid or mismatched with data_len
 *
 * Validation performed:
 * - Width/height: 64-2048, multiple of 64
 * - Channels: 3 (RGB) or 4 (RGBA)
 * - image_data_len matches width * height * channels
 * - Buffer has enough space for complete message
 * - No integer overflow in size calculations
 */
error_code_t encode_generate_response(const sd35_generate_response_t *resp,
                                      uint8_t *buffer, size_t buf_size,
                                      size_t *out_len) {
    if (resp == NULL || buffer == NULL || out_len == NULL) {
        return ERR_INTERNAL;
    }

    if (resp->image_data == NULL) {
        return ERR_INTERNAL;
    }

    if (resp->image_width < SD35_MIN_DIMENSION || resp->image_width > SD35_MAX_DIMENSION ||
        resp->image_width % SD35_DIMENSION_ALIGNMENT != 0) {
        return ERR_INVALID_DIMENSIONS;
    }

    if (resp->image_height < SD35_MIN_DIMENSION || resp->image_height > SD35_MAX_DIMENSION ||
        resp->image_height % SD35_DIMENSION_ALIGNMENT != 0) {
        return ERR_INVALID_DIMENSIONS;
    }

    if (resp->channels != 3 && resp->channels != 4) {
        return ERR_INVALID_DIMENSIONS;
    }

    if (resp->image_width > UINT32_MAX / resp->image_height) {
        return ERR_INVALID_DIMENSIONS;
    }
    uint32_t pixels = resp->image_width * resp->image_height;

    if (pixels > UINT32_MAX / resp->channels) {
        return ERR_INVALID_DIMENSIONS;
    }
    uint32_t expected_data_len = pixels * resp->channels;

    if (resp->image_data_len != expected_data_len) {
        return ERR_INVALID_DIMENSIONS;
    }

    if (resp->image_data_len > MAX_MESSAGE_SIZE - 16 - 16) {
        return ERR_INTERNAL;
    }

    uint32_t payload_len = 16 + 16 + resp->image_data_len;
    size_t total_len = 16 + payload_len;

    if (total_len > buf_size) {
        return ERR_INTERNAL;
    }

    uint8_t *ptr = buffer;

    write_u32_be(ptr, PROTOCOL_MAGIC);
    ptr += 4;
    write_u16_be(ptr, PROTOCOL_VERSION_1);
    ptr += 2;
    write_u16_be(ptr, MSG_GENERATE_RESPONSE);
    ptr += 2;
    write_u32_be(ptr, payload_len);
    ptr += 4;
    write_u32_be(ptr, 0);
    ptr += 4;

    write_u64_be(ptr, resp->request_id);
    ptr += 8;
    write_u32_be(ptr, resp->status);
    ptr += 4;
    write_u32_be(ptr, resp->generation_time_ms);
    ptr += 4;

    write_u32_be(ptr, resp->image_width);
    ptr += 4;
    write_u32_be(ptr, resp->image_height);
    ptr += 4;
    write_u32_be(ptr, resp->channels);
    ptr += 4;
    write_u32_be(ptr, resp->image_data_len);
    ptr += 4;

    memcpy(ptr, resp->image_data, resp->image_data_len);
    ptr += resp->image_data_len;

    *out_len = total_len;
    return ERR_NONE;
}

/**
 * encode_error_response - Encode error response
 *
 * This function encodes an error response into the binary protocol format.
 * Error responses are used to communicate validation failures and server
 * errors to the client.
 *
 * Message structure:
 * - Common header (16 bytes, msg_type = MSG_ERROR)
 * - request_id (8 bytes, 0 if request was invalid)
 * - status (4 bytes, 400 or 500)
 * - error_code (4 bytes, machine-readable error identifier)
 * - error_msg_len (2 bytes, length of error message)
 * - error_msg (variable, UTF-8 encoded human-readable message)
 *
 * @param resp      Error response structure to encode
 * @param buffer    Output buffer for encoded message
 * @param buf_size  Size of output buffer in bytes
 * @param out_len   Pointer to store actual encoded length (bytes written)
 * @return          ERR_NONE on success, ERR_INTERNAL on failure
 *
 * Error codes:
 * - ERR_INTERNAL: NULL pointer, buffer too small, or message too long
 *
 * Validation performed:
 * - error_msg_len must fit in uint16 (max 65535 bytes)
 * - Buffer has enough space for complete message
 * - Total message size <= MAX_MESSAGE_SIZE
 */
error_code_t encode_error_response(const error_response_t *resp,
                                   uint8_t *buffer, size_t buf_size,
                                   size_t *out_len) {
    if (resp == NULL || buffer == NULL || out_len == NULL) {
        return ERR_INTERNAL;
    }

    if (resp->error_msg == NULL && resp->error_msg_len > 0) {
        return ERR_INTERNAL;
    }

    uint32_t payload_len = 8 + 4 + 4 + 2 + resp->error_msg_len;

    if (payload_len > MAX_MESSAGE_SIZE - 16) {
        return ERR_INTERNAL;
    }

    size_t total_len = 16 + payload_len;

    if (total_len > buf_size) {
        return ERR_INTERNAL;
    }

    uint8_t *ptr = buffer;

    write_u32_be(ptr, PROTOCOL_MAGIC);
    ptr += 4;
    write_u16_be(ptr, PROTOCOL_VERSION_1);
    ptr += 2;
    write_u16_be(ptr, MSG_ERROR);
    ptr += 2;
    write_u32_be(ptr, payload_len);
    ptr += 4;
    write_u32_be(ptr, 0);
    ptr += 4;

    write_u64_be(ptr, resp->request_id);
    ptr += 8;
    write_u32_be(ptr, resp->status);
    ptr += 4;
    write_u32_be(ptr, resp->error_code);
    ptr += 4;
    write_u16_be(ptr, resp->error_msg_len);
    ptr += 2;

    if (resp->error_msg_len > 0) {
        memcpy(ptr, resp->error_msg, resp->error_msg_len);
        ptr += resp->error_msg_len;
    }

    *out_len = total_len;
    return ERR_NONE;
}
