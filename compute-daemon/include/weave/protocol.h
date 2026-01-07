/**
 * Weave Binary Protocol - C Type Definitions
 *
 * This header defines C types and constants for the Weave binary protocol.
 * The protocol is used for communication between the Go orchestration layer
 * (weave) and the C GPU daemon (weave-compute) over Unix domain sockets.
 *
 * Protocol version: 1
 * Specification: docs/protocol/SPEC.md, docs/protocol/SPEC_SD35.md
 *
 * Wire format conventions:
 * - All multi-byte integers are big-endian (network byte order)
 * - Strings are UTF-8 encoded, length-prefixed, NOT null-terminated
 * - No struct padding or alignment assumptions
 * - Manual serialization/deserialization required (see encoding/decoding functions)
 */

#pragma once

#include <stdint.h>
#include <stddef.h>

/**
 * Protocol Constants
 */

/** Protocol magic number: ASCII "WEVE" (0x57455645) */
#define PROTOCOL_MAGIC 0x57455645

/** Current protocol version */
#define PROTOCOL_VERSION_1 0x0001

/** Minimum supported protocol version */
#define MIN_SUPPORTED_VERSION PROTOCOL_VERSION_1

/** Maximum supported protocol version */
#define MAX_SUPPORTED_VERSION PROTOCOL_VERSION_1

/** Maximum total message size: 10 MB */
#define MAX_MESSAGE_SIZE (10 * 1024 * 1024)

/**
 * Model Identifiers
 */

/** Stable Diffusion 3.5 model ID */
#define MODEL_ID_SD35 0x00000000

/**
 * SD 3.5 Parameter Bounds
 */

/** Minimum image dimension (pixels) */
#define SD35_MIN_DIMENSION 64

/** Maximum image dimension (pixels) */
#define SD35_MAX_DIMENSION 2048

/** Required alignment for dimensions (must be multiple of 64) */
#define SD35_DIMENSION_ALIGNMENT 64

/** Minimum denoising steps */
#define SD35_MIN_STEPS 1

/** Maximum denoising steps */
#define SD35_MAX_STEPS 100

/** Minimum CFG scale */
#define SD35_MIN_CFG 0.0f

/** Maximum CFG scale */
#define SD35_MAX_CFG 20.0f

/** Minimum prompt length per encoder (bytes, UTF-8) */
#define SD35_MIN_PROMPT_LENGTH 1

/**
 * Maximum prompt length per encoder (bytes, UTF-8)
 *
 * Note: stable-diffusion.cpp has a bug where T5 producing more tokens than
 * CLIP causes GGML assertion failures. Tokenizers break words into subwords,
 * so character count != token count. Complex words like "photorealistic" may
 * become 3+ tokens. Limiting to 256 bytes (~50-70 tokens) keeps token counts
 * safely below CLIP's limits and prevents CLIP/T5 mismatch.
 *
 * Observed: 379 chars with 58 words still crashed (subword expansion).
 */
#define SD35_MAX_PROMPT_LENGTH 256

/** Maximum total prompt data size (3 encoders × 256 bytes) */
#define SD35_MAX_PROMPT_DATA_SIZE (3 * SD35_MAX_PROMPT_LENGTH)

/**
 * Message Types
 */
typedef enum {
    MSG_GENERATE_REQUEST  = 0x0001,  /**< Generation request */
    MSG_GENERATE_RESPONSE = 0x0002,  /**< Generation response (success) */
    MSG_ERROR             = 0x00FF,  /**< Error response */
} message_type_t;

/**
 * Status Codes
 *
 * HTTP-like status codes for semantic clarity.
 */
typedef enum {
    STATUS_OK                   = 200,  /**< Success */
    STATUS_BAD_REQUEST          = 400,  /**< Client error (invalid params) */
    STATUS_INTERNAL_SERVER_ERROR = 500, /**< Server error (GPU failure, OOM, etc.) */
} status_code_t;

/**
 * Error Codes
 *
 * Machine-readable error identifiers.
 * Error codes map to HTTP status codes:
 * - Client errors (400): ERR_INVALID_MAGIC, ERR_UNSUPPORTED_VERSION,
 *   ERR_INVALID_MODEL_ID, ERR_INVALID_PROMPT, ERR_INVALID_DIMENSIONS,
 *   ERR_INVALID_STEPS, ERR_INVALID_CFG
 * - Server errors (500): ERR_OUT_OF_MEMORY, ERR_GPU_ERROR,
 *   ERR_TIMEOUT, ERR_INTERNAL
 */
typedef enum {
    ERR_NONE                = 0,   /**< No error */
    ERR_INVALID_MAGIC       = 1,   /**< Invalid magic number (400) */
    ERR_UNSUPPORTED_VERSION = 2,   /**< Unsupported protocol version (400) */
    ERR_INVALID_MODEL_ID    = 3,   /**< Invalid or unsupported model ID (400) */
    ERR_INVALID_PROMPT      = 4,   /**< Invalid prompt (empty, too long, or bad offset) (400) */
    ERR_INVALID_DIMENSIONS  = 5,   /**< Invalid dimensions (out of range or not aligned) (400) */
    ERR_INVALID_STEPS       = 6,   /**< Invalid steps (out of range) (400) */
    ERR_INVALID_CFG         = 7,   /**< Invalid CFG scale (out of range, NaN, or Inf) (400) */
    ERR_OUT_OF_MEMORY       = 8,   /**< Out of memory (500) */
    ERR_GPU_ERROR           = 9,   /**< GPU error (500) */
    ERR_TIMEOUT             = 10,  /**< Operation timeout (500) */
    ERR_INTERNAL            = 99,  /**< Internal error (500) */
} error_code_t;

/**
 * Common Message Header
 *
 * In-memory representation of the protocol header.
 * This struct is NOT for wire format - use encoding/decoding functions.
 *
 * Wire format: 16 bytes, big-endian (see docs/protocol/SPEC.md)
 */
typedef struct {
    uint32_t magic;        /**< Magic number (0x57455645 = "WEVE") */
    uint16_t version;      /**< Protocol version */
    uint16_t msg_type;     /**< Message type (message_type_t) */
    uint32_t payload_len;  /**< Length of data following header */
    uint32_t reserved;     /**< Reserved (must be 0) */
} protocol_header_t;

/**
 * SD 3.5 Generation Request
 *
 * In-memory representation of a Stable Diffusion 3.5 generation request.
 * This struct is NOT for wire format - use encoding/decoding functions.
 *
 * Wire format payload structure (after common header):
 * - request_id: 8 bytes (uint64)
 * - model_id: 4 bytes (uint32, must be 0x00000000 for SD 3.5)
 * - width: 4 bytes (uint32)
 * - height: 4 bytes (uint32)
 * - steps: 4 bytes (uint32)
 * - cfg_scale: 4 bytes (float32, IEEE 754)
 * - seed: 8 bytes (uint64, 0 = random)
 * - clip_l_offset: 4 bytes (uint32)
 * - clip_l_length: 4 bytes (uint32)
 * - clip_g_offset: 4 bytes (uint32)
 * - clip_g_length: 4 bytes (uint32)
 * - t5_offset: 4 bytes (uint32)
 * - t5_length: 4 bytes (uint32)
 * - prompt_data: variable bytes (UTF-8 encoded prompts)
 */
typedef struct {
    /* Common request fields */
    uint64_t request_id;   /**< Unique request identifier (echoed in response) */
    uint32_t model_id;     /**< Model identifier (must be MODEL_ID_SD35) */

    /* Generation parameters */
    uint32_t width;        /**< Image width (64-2048, multiple of 64) */
    uint32_t height;       /**< Image height (64-2048, multiple of 64) */
    uint32_t steps;        /**< Denoising steps (1-100, recommended: 28) */
    float cfg_scale;       /**< CFG scale (0.0-20.0, recommended: 7.0) */
    uint64_t seed;         /**< Random seed (0 = random) */

    /* Prompt offset table */
    uint32_t clip_l_offset; /**< Byte offset of CLIP-L prompt in prompt_data */
    uint32_t clip_l_length; /**< Length of CLIP-L prompt (1-1024 bytes) */
    uint32_t clip_g_offset; /**< Byte offset of CLIP-G prompt in prompt_data */
    uint32_t clip_g_length; /**< Length of CLIP-G prompt (1-1024 bytes) */
    uint32_t t5_offset;     /**< Byte offset of T5 prompt in prompt_data */
    uint32_t t5_length;     /**< Length of T5 prompt (1-1024 bytes) */

    /* Prompt data (not owned by this struct, points into received buffer) */
    const uint8_t *prompt_data;  /**< Pointer to prompt data buffer */
    size_t prompt_data_len;      /**< Total size of prompt_data buffer */
} sd35_generate_request_t;

/**
 * SD 3.5 Generation Response
 *
 * In-memory representation of a successful SD 3.5 generation response.
 * This struct is NOT for wire format - use encoding/decoding functions.
 *
 * Wire format payload structure (after common header):
 * - request_id: 8 bytes (uint64)
 * - status: 4 bytes (uint32, must be STATUS_OK = 200)
 * - generation_time_ms: 4 bytes (uint32)
 * - image_width: 4 bytes (uint32)
 * - image_height: 4 bytes (uint32)
 * - channels: 4 bytes (uint32, 3 = RGB, 4 = RGBA)
 * - image_data_len: 4 bytes (uint32)
 * - image_data: variable bytes (raw pixel data)
 */
typedef struct {
    /* Common response fields */
    uint64_t request_id;         /**< Request ID (echoed from request) */
    uint32_t status;             /**< Status code (STATUS_OK = 200) */
    uint32_t generation_time_ms; /**< Generation time in milliseconds */

    /* Image metadata */
    uint32_t image_width;   /**< Image width (should match request) */
    uint32_t image_height;  /**< Image height (should match request) */
    uint32_t channels;      /**< Number of channels (3 = RGB, 4 = RGBA) */
    uint32_t image_data_len; /**< Size of image_data in bytes */

    /* Image data (not owned by this struct, points into buffer) */
    const uint8_t *image_data; /**< Pointer to raw pixel data (RGB/RGBA) */
} sd35_generate_response_t;

/**
 * Error Response
 *
 * In-memory representation of an error response.
 * This struct is NOT for wire format - use encoding/decoding functions.
 *
 * Wire format payload structure (after common header with msg_type = MSG_ERROR):
 * - request_id: 8 bytes (uint64, 0 if request was invalid)
 * - status: 4 bytes (uint32, 400 or 500)
 * - error_code: 4 bytes (uint32)
 * - error_msg_len: 2 bytes (uint16)
 * - error_msg: variable bytes (UTF-8 encoded, human-readable)
 */
typedef struct {
    uint64_t request_id;  /**< Request ID (0 if request was invalid) */
    uint32_t status;      /**< Status code (400 or 500) */
    uint32_t error_code;  /**< Error code (error_code_t) */
    uint16_t error_msg_len; /**< Length of error message */

    /* Error message (not owned by this struct, points into buffer) */
    const char *error_msg; /**< Pointer to error message (UTF-8) */
} error_response_t;

/**
 * Encoding and Decoding Functions
 */

/**
 * decode_generate_request - Decode and validate SD 3.5 generation request
 *
 * @param data      Input buffer containing complete message
 * @param data_len  Size of input buffer
 * @param req       Output request structure (populated on success)
 * @return          ERR_NONE on success, error code on failure
 */
error_code_t decode_generate_request(const uint8_t *data, size_t data_len,
                                     sd35_generate_request_t *req);

/**
 * encode_generate_response - Encode SD 3.5 generation response
 *
 * @param resp      Response structure to encode
 * @param buffer    Output buffer for encoded message
 * @param buf_size  Size of output buffer in bytes
 * @param out_len   Pointer to store actual encoded length
 * @return          ERR_NONE on success, error code on failure
 */
error_code_t encode_generate_response(const sd35_generate_response_t *resp,
                                      uint8_t *buffer, size_t buf_size,
                                      size_t *out_len);

/**
 * encode_error_response - Encode error response
 *
 * @param resp      Error response structure to encode
 * @param buffer    Output buffer for encoded message
 * @param buf_size  Size of output buffer in bytes
 * @param out_len   Pointer to store actual encoded length
 * @return          ERR_NONE on success, ERR_INTERNAL on failure
 */
error_code_t encode_error_response(const error_response_t *resp,
                                   uint8_t *buffer, size_t buf_size,
                                   size_t *out_len);
