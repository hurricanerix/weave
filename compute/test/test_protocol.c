/**
 * Weave Binary Protocol - Request Decoder Tests
 *
 * Comprehensive unit tests for the protocol decoder.
 * Tests cover all validation paths, error conditions, and edge cases.
 *
 * Test organization:
 * - Valid requests (happy path)
 * - Invalid magic/version/type
 * - Invalid model ID
 * - Out-of-bounds parameters
 * - Buffer overflow attempts
 * - Integer overflow attempts
 * - Truncated messages
 */

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <stdint.h>
#include <assert.h>
#include <math.h>
#include "weave/protocol.h"

extern error_code_t decode_generate_request(const uint8_t *data, size_t data_len,
                                            sd35_generate_request_t *req);
extern error_code_t encode_generate_response(const sd35_generate_response_t *resp,
                                             uint8_t *buffer, size_t buf_size,
                                             size_t *out_len);
extern error_code_t encode_error_response(const error_response_t *resp,
                                          uint8_t *buffer, size_t buf_size,
                                          size_t *out_len);

/**
 * Test result tracking
 */
static int tests_run = 0;
static int tests_passed = 0;

#define TEST(name) \
    do { \
        printf("Running: %s\n", name); \
        tests_run++; \
    } while(0)

#define ASSERT_EQ(expected, actual) \
    do { \
        if ((expected) != (actual)) { \
            printf("  FAIL: Expected %d, got %d at line %d\n", \
                   (int)(expected), (int)(actual), __LINE__); \
            return; \
        } \
    } while(0)

#define ASSERT_TRUE(expr) \
    do { \
        if (!(expr)) { \
            printf("  FAIL: Assertion failed: %s at line %d\n", #expr, __LINE__); \
            return; \
        } \
    } while(0)

#define TEST_PASS() \
    do { \
        tests_passed++; \
        printf("  PASS\n"); \
    } while(0)

/**
 * Helper: Read big-endian integers from buffer (for verification)
 */

static uint16_t read_u16_be(const uint8_t *buf) {
    return ((uint16_t)buf[0] << 8) |
           ((uint16_t)buf[1]);
}

static uint32_t read_u32_be(const uint8_t *buf) {
    return ((uint32_t)buf[0] << 24) |
           ((uint32_t)buf[1] << 16) |
           ((uint32_t)buf[2] << 8) |
           ((uint32_t)buf[3]);
}

static uint64_t read_u64_be(const uint8_t *buf) {
    return ((uint64_t)buf[0] << 56) |
           ((uint64_t)buf[1] << 48) |
           ((uint64_t)buf[2] << 40) |
           ((uint64_t)buf[3] << 32) |
           ((uint64_t)buf[4] << 24) |
           ((uint64_t)buf[5] << 16) |
           ((uint64_t)buf[6] << 8) |
           ((uint64_t)buf[7]);
}

/**
 * Helper: Write big-endian integers to buffer
 */

static void write_u16_be(uint8_t *buf, uint16_t value) {
    buf[0] = (value >> 8) & 0xFF;
    buf[1] = value & 0xFF;
}

static void write_u32_be(uint8_t *buf, uint32_t value) {
    buf[0] = (value >> 24) & 0xFF;
    buf[1] = (value >> 16) & 0xFF;
    buf[2] = (value >> 8) & 0xFF;
    buf[3] = value & 0xFF;
}

static void write_u64_be(uint8_t *buf, uint64_t value) {
    buf[0] = (value >> 56) & 0xFF;
    buf[1] = (value >> 48) & 0xFF;
    buf[2] = (value >> 40) & 0xFF;
    buf[3] = (value >> 32) & 0xFF;
    buf[4] = (value >> 24) & 0xFF;
    buf[5] = (value >> 16) & 0xFF;
    buf[6] = (value >> 8) & 0xFF;
    buf[7] = value & 0xFF;
}

static void write_f32_be(uint8_t *buf, float value) {
    uint32_t bits;
    memcpy(&bits, &value, sizeof(float));
    write_u32_be(buf, bits);
}

/**
 * Helper: Build a valid SD 3.5 request
 */
static size_t build_valid_request(uint8_t *buffer, size_t buffer_size,
                                  uint64_t request_id,
                                  uint32_t width, uint32_t height,
                                  uint32_t steps, float cfg_scale,
                                  uint64_t seed,
                                  const char *prompt) {
    size_t prompt_len = strlen(prompt);
    size_t prompt_data_size = prompt_len * 3;
    size_t payload_len = 12 + 48 + prompt_data_size;
    size_t total_len = 16 + payload_len;

    if (total_len > buffer_size) {
        return 0;
    }

    uint8_t *ptr = buffer;

    write_u32_be(ptr, PROTOCOL_MAGIC);
    ptr += 4;
    write_u16_be(ptr, PROTOCOL_VERSION_1);
    ptr += 2;
    write_u16_be(ptr, MSG_GENERATE_REQUEST);
    ptr += 2;
    write_u32_be(ptr, (uint32_t)payload_len);
    ptr += 4;
    write_u32_be(ptr, 0);
    ptr += 4;

    write_u64_be(ptr, request_id);
    ptr += 8;
    write_u32_be(ptr, MODEL_ID_SD35);
    ptr += 4;

    write_u32_be(ptr, width);
    ptr += 4;
    write_u32_be(ptr, height);
    ptr += 4;
    write_u32_be(ptr, steps);
    ptr += 4;
    write_f32_be(ptr, cfg_scale);
    ptr += 4;
    write_u64_be(ptr, seed);
    ptr += 8;

    write_u32_be(ptr, 0);
    ptr += 4;
    write_u32_be(ptr, (uint32_t)prompt_len);
    ptr += 4;
    write_u32_be(ptr, (uint32_t)prompt_len);
    ptr += 4;
    write_u32_be(ptr, (uint32_t)prompt_len);
    ptr += 4;
    write_u32_be(ptr, (uint32_t)(prompt_len * 2));
    ptr += 4;
    write_u32_be(ptr, (uint32_t)prompt_len);
    ptr += 4;

    memcpy(ptr, prompt, prompt_len);
    ptr += prompt_len;
    memcpy(ptr, prompt, prompt_len);
    ptr += prompt_len;
    memcpy(ptr, prompt, prompt_len);

    return total_len;
}

/**
 * Test: Valid request with typical parameters
 */
void test_valid_request_typical(void) {
    TEST("test_valid_request_typical");

    uint8_t buffer[4096];
    size_t len = build_valid_request(buffer, sizeof(buffer),
                                     12345,
                                     512, 512,
                                     28, 7.0f,
                                     0,
                                     "a cat in space");

    ASSERT_TRUE(len > 0);

    sd35_generate_request_t req;
    error_code_t err = decode_generate_request(buffer, len, &req);

    ASSERT_EQ(ERR_NONE, err);
    ASSERT_EQ(12345, req.request_id);
    ASSERT_EQ(MODEL_ID_SD35, req.model_id);
    ASSERT_EQ(512, req.width);
    ASSERT_EQ(512, req.height);
    ASSERT_EQ(28, req.steps);
    ASSERT_TRUE(fabsf(req.cfg_scale - 7.0f) < 0.001f);
    ASSERT_EQ(0, req.seed);
    ASSERT_EQ(14, req.clip_l_length);
    ASSERT_EQ(14, req.clip_g_length);
    ASSERT_EQ(14, req.t5_length);
    ASSERT_TRUE(memcmp(req.prompt_data, "a cat in space", 14) == 0);

    TEST_PASS();
}

/**
 * Test: Valid request with boundary dimensions
 */
void test_valid_request_min_dimensions(void) {
    TEST("test_valid_request_min_dimensions");

    uint8_t buffer[4096];
    size_t len = build_valid_request(buffer, sizeof(buffer),
                                     1,
                                     64, 64,
                                     1, 0.0f,
                                     0,
                                     "test");

    sd35_generate_request_t req;
    error_code_t err = decode_generate_request(buffer, len, &req);

    ASSERT_EQ(ERR_NONE, err);
    ASSERT_EQ(64, req.width);
    ASSERT_EQ(64, req.height);
    ASSERT_EQ(1, req.steps);
    ASSERT_TRUE(fabsf(req.cfg_scale - 0.0f) < 0.001f);

    TEST_PASS();
}

void test_valid_request_max_dimensions(void) {
    TEST("test_valid_request_max_dimensions");

    uint8_t buffer[8192];
    size_t len = build_valid_request(buffer, sizeof(buffer),
                                     1,
                                     2048, 2048,
                                     100, 20.0f,
                                     UINT64_MAX,
                                     "test");

    sd35_generate_request_t req;
    error_code_t err = decode_generate_request(buffer, len, &req);

    ASSERT_EQ(ERR_NONE, err);
    ASSERT_EQ(2048, req.width);
    ASSERT_EQ(2048, req.height);
    ASSERT_EQ(100, req.steps);
    ASSERT_TRUE(fabsf(req.cfg_scale - 20.0f) < 0.001f);
    ASSERT_EQ(UINT64_MAX, req.seed);

    TEST_PASS();
}

/**
 * Test: Invalid magic number
 */
void test_invalid_magic(void) {
    TEST("test_invalid_magic");

    uint8_t buffer[4096];
    size_t len = build_valid_request(buffer, sizeof(buffer),
                                     1, 512, 512, 28, 7.0f, 0, "test");

    write_u32_be(buffer, 0xDEADBEEF);

    sd35_generate_request_t req;
    error_code_t err = decode_generate_request(buffer, len, &req);

    ASSERT_EQ(ERR_INVALID_MAGIC, err);

    TEST_PASS();
}

/**
 * Test: Unsupported protocol version
 */
void test_unsupported_version_too_high(void) {
    TEST("test_unsupported_version_too_high");

    uint8_t buffer[4096];
    size_t len = build_valid_request(buffer, sizeof(buffer),
                                     1, 512, 512, 28, 7.0f, 0, "test");

    write_u16_be(buffer + 4, 0x9999);

    sd35_generate_request_t req;
    error_code_t err = decode_generate_request(buffer, len, &req);

    ASSERT_EQ(ERR_UNSUPPORTED_VERSION, err);

    TEST_PASS();
}

void test_unsupported_version_zero(void) {
    TEST("test_unsupported_version_zero");

    uint8_t buffer[4096];
    size_t len = build_valid_request(buffer, sizeof(buffer),
                                     1, 512, 512, 28, 7.0f, 0, "test");

    write_u16_be(buffer + 4, 0x0000);

    sd35_generate_request_t req;
    error_code_t err = decode_generate_request(buffer, len, &req);

    ASSERT_EQ(ERR_UNSUPPORTED_VERSION, err);

    TEST_PASS();
}

/**
 * Test: Invalid model ID
 */
void test_invalid_model_id(void) {
    TEST("test_invalid_model_id");

    uint8_t buffer[4096];
    size_t len = build_valid_request(buffer, sizeof(buffer),
                                     1, 512, 512, 28, 7.0f, 0, "test");

    write_u32_be(buffer + 24, 1);

    sd35_generate_request_t req;
    error_code_t err = decode_generate_request(buffer, len, &req);

    ASSERT_EQ(ERR_INVALID_MODEL_ID, err);

    TEST_PASS();
}

/**
 * Test: Invalid dimensions
 */
void test_dimensions_too_small(void) {
    TEST("test_dimensions_too_small");

    uint8_t buffer[4096];
    size_t len = build_valid_request(buffer, sizeof(buffer),
                                     1, 32, 512, 28, 7.0f, 0, "test");

    sd35_generate_request_t req;
    error_code_t err = decode_generate_request(buffer, len, &req);

    ASSERT_EQ(ERR_INVALID_DIMENSIONS, err);

    TEST_PASS();
}

void test_dimensions_too_large(void) {
    TEST("test_dimensions_too_large");

    uint8_t buffer[4096];
    size_t len = build_valid_request(buffer, sizeof(buffer),
                                     1, 512, 4096, 28, 7.0f, 0, "test");

    sd35_generate_request_t req;
    error_code_t err = decode_generate_request(buffer, len, &req);

    ASSERT_EQ(ERR_INVALID_DIMENSIONS, err);

    TEST_PASS();
}

void test_dimensions_not_aligned(void) {
    TEST("test_dimensions_not_aligned");

    uint8_t buffer[4096];
    size_t len = build_valid_request(buffer, sizeof(buffer),
                                     1, 513, 512, 28, 7.0f, 0, "test");

    sd35_generate_request_t req;
    error_code_t err = decode_generate_request(buffer, len, &req);

    ASSERT_EQ(ERR_INVALID_DIMENSIONS, err);

    TEST_PASS();
}

/**
 * Test: Invalid steps
 */
void test_steps_zero(void) {
    TEST("test_steps_zero");

    uint8_t buffer[4096];
    size_t len = build_valid_request(buffer, sizeof(buffer),
                                     1, 512, 512, 0, 7.0f, 0, "test");

    sd35_generate_request_t req;
    error_code_t err = decode_generate_request(buffer, len, &req);

    ASSERT_EQ(ERR_INVALID_STEPS, err);

    TEST_PASS();
}

void test_steps_too_high(void) {
    TEST("test_steps_too_high");

    uint8_t buffer[4096];
    size_t len = build_valid_request(buffer, sizeof(buffer),
                                     1, 512, 512, 101, 7.0f, 0, "test");

    sd35_generate_request_t req;
    error_code_t err = decode_generate_request(buffer, len, &req);

    ASSERT_EQ(ERR_INVALID_STEPS, err);

    TEST_PASS();
}

/**
 * Test: Invalid CFG scale
 */
void test_cfg_negative(void) {
    TEST("test_cfg_negative");

    uint8_t buffer[4096];
    size_t len = build_valid_request(buffer, sizeof(buffer),
                                     1, 512, 512, 28, -1.0f, 0, "test");

    sd35_generate_request_t req;
    error_code_t err = decode_generate_request(buffer, len, &req);

    ASSERT_EQ(ERR_INVALID_CFG, err);

    TEST_PASS();
}

void test_cfg_too_high(void) {
    TEST("test_cfg_too_high");

    uint8_t buffer[4096];
    size_t len = build_valid_request(buffer, sizeof(buffer),
                                     1, 512, 512, 28, 21.0f, 0, "test");

    sd35_generate_request_t req;
    error_code_t err = decode_generate_request(buffer, len, &req);

    ASSERT_EQ(ERR_INVALID_CFG, err);

    TEST_PASS();
}

void test_cfg_nan(void) {
    TEST("test_cfg_nan");

    uint8_t buffer[4096];
    size_t len = build_valid_request(buffer, sizeof(buffer),
                                     1, 512, 512, 28, NAN, 0, "test");

    sd35_generate_request_t req;
    error_code_t err = decode_generate_request(buffer, len, &req);

    ASSERT_EQ(ERR_INVALID_CFG, err);

    TEST_PASS();
}

void test_cfg_inf(void) {
    TEST("test_cfg_inf");

    uint8_t buffer[4096];
    size_t len = build_valid_request(buffer, sizeof(buffer),
                                     1, 512, 512, 28, INFINITY, 0, "test");

    sd35_generate_request_t req;
    error_code_t err = decode_generate_request(buffer, len, &req);

    ASSERT_EQ(ERR_INVALID_CFG, err);

    TEST_PASS();
}

/**
 * Test: Invalid prompt offsets
 */
void test_prompt_offset_out_of_bounds(void) {
    TEST("test_prompt_offset_out_of_bounds");

    uint8_t buffer[4096];
    size_t len = build_valid_request(buffer, sizeof(buffer),
                                     1, 512, 512, 28, 7.0f, 0, "test");

    write_u32_be(buffer + 28 + 24, 9999);

    sd35_generate_request_t req;
    error_code_t err = decode_generate_request(buffer, len, &req);

    ASSERT_EQ(ERR_INVALID_PROMPT, err);

    TEST_PASS();
}

void test_prompt_length_exceeds_data(void) {
    TEST("test_prompt_length_exceeds_data");

    uint8_t buffer[4096];
    size_t len = build_valid_request(buffer, sizeof(buffer),
                                     1, 512, 512, 28, 7.0f, 0, "test");

    write_u32_be(buffer + 28 + 28, 9999);

    sd35_generate_request_t req;
    error_code_t err = decode_generate_request(buffer, len, &req);

    ASSERT_EQ(ERR_INVALID_PROMPT, err);

    TEST_PASS();
}

void test_prompt_length_zero(void) {
    TEST("test_prompt_length_zero");

    uint8_t buffer[4096];
    size_t len = build_valid_request(buffer, sizeof(buffer),
                                     1, 512, 512, 28, 7.0f, 0, "test");

    write_u32_be(buffer + 28 + 28, 0);

    sd35_generate_request_t req;
    error_code_t err = decode_generate_request(buffer, len, &req);

    ASSERT_EQ(ERR_INVALID_PROMPT, err);

    TEST_PASS();
}

void test_prompt_length_too_large(void) {
    TEST("test_prompt_length_too_large");

    uint8_t buffer[4096];
    size_t len = build_valid_request(buffer, sizeof(buffer),
                                     1, 512, 512, 28, 7.0f, 0, "test");

    write_u32_be(buffer + 28 + 28, 3000);

    sd35_generate_request_t req;
    error_code_t err = decode_generate_request(buffer, len, &req);

    ASSERT_EQ(ERR_INVALID_PROMPT, err);

    TEST_PASS();
}

/**
 * Test: Truncated messages
 */
void test_truncated_header(void) {
    TEST("test_truncated_header");

    uint8_t buffer[10];
    memset(buffer, 0, sizeof(buffer));

    sd35_generate_request_t req;
    error_code_t err = decode_generate_request(buffer, sizeof(buffer), &req);

    ASSERT_EQ(ERR_INTERNAL, err);

    TEST_PASS();
}

void test_truncated_payload(void) {
    TEST("test_truncated_payload");

    uint8_t buffer[4096];
    size_t len = build_valid_request(buffer, sizeof(buffer),
                                     1, 512, 512, 28, 7.0f, 0, "test");

    sd35_generate_request_t req;
    error_code_t err = decode_generate_request(buffer, len - 10, &req);

    ASSERT_EQ(ERR_INTERNAL, err);

    TEST_PASS();
}

/**
 * Test: NULL pointer handling
 */
void test_null_data_pointer(void) {
    TEST("test_null_data_pointer");

    sd35_generate_request_t req;
    error_code_t err = decode_generate_request(NULL, 100, &req);

    ASSERT_EQ(ERR_INTERNAL, err);

    TEST_PASS();
}

void test_null_request_pointer(void) {
    TEST("test_null_request_pointer");

    uint8_t buffer[4096];
    size_t len = build_valid_request(buffer, sizeof(buffer),
                                     1, 512, 512, 28, 7.0f, 0, "test");

    error_code_t err = decode_generate_request(buffer, len, NULL);

    ASSERT_EQ(ERR_INTERNAL, err);

    TEST_PASS();
}

/**
 * Test: Integer overflow in prompt offset calculation
 */
void test_prompt_offset_overflow(void) {
    TEST("test_prompt_offset_overflow");

    uint8_t buffer[4096];
    size_t len = build_valid_request(buffer, sizeof(buffer),
                                     1, 512, 512, 28, 7.0f, 0, "test");

    write_u32_be(buffer + 28 + 24, UINT32_MAX - 5);
    write_u32_be(buffer + 28 + 28, 10);

    sd35_generate_request_t req;
    error_code_t err = decode_generate_request(buffer, len, &req);

    ASSERT_EQ(ERR_INVALID_PROMPT, err);

    TEST_PASS();
}

/**
 * ============================================================================
 * Encoder Tests
 * ============================================================================
 */

/**
 * Test: Encode valid generate response with test pattern
 */
void test_encode_generate_response_valid(void) {
    TEST("test_encode_generate_response_valid");

    uint8_t test_image[512 * 512 * 3];
    for (size_t i = 0; i < sizeof(test_image); i++) {
        test_image[i] = (uint8_t)(i % 256);
    }

    sd35_generate_response_t resp = {
        .request_id = 12345,
        .status = STATUS_OK,
        .generation_time_ms = 10000,
        .image_width = 512,
        .image_height = 512,
        .channels = 3,
        .image_data_len = 512 * 512 * 3,
        .image_data = test_image,
    };

    uint8_t buffer[1024 * 1024];
    size_t encoded_len;

    error_code_t err = encode_generate_response(&resp, buffer, sizeof(buffer), &encoded_len);

    ASSERT_EQ(ERR_NONE, err);

    size_t expected_len = 16 + 16 + 16 + (512 * 512 * 3);
    ASSERT_EQ(expected_len, encoded_len);

    ASSERT_EQ(PROTOCOL_MAGIC, read_u32_be(buffer));
    ASSERT_EQ(PROTOCOL_VERSION_1, read_u16_be(buffer + 4));
    ASSERT_EQ(MSG_GENERATE_RESPONSE, read_u16_be(buffer + 6));
    ASSERT_EQ(16 + 16 + (512 * 512 * 3), read_u32_be(buffer + 8));

    ASSERT_EQ(12345, read_u64_be(buffer + 16));
    ASSERT_EQ(STATUS_OK, read_u32_be(buffer + 24));
    ASSERT_EQ(10000, read_u32_be(buffer + 28));

    ASSERT_EQ(512, read_u32_be(buffer + 32));
    ASSERT_EQ(512, read_u32_be(buffer + 36));
    ASSERT_EQ(3, read_u32_be(buffer + 40));
    ASSERT_EQ(512 * 512 * 3, read_u32_be(buffer + 44));

    ASSERT_TRUE(memcmp(buffer + 48, test_image, sizeof(test_image)) == 0);

    TEST_PASS();
}

/**
 * Test: Encode minimum dimensions (64x64)
 */
void test_encode_generate_response_min_dimensions(void) {
    TEST("test_encode_generate_response_min_dimensions");

    uint8_t test_image[64 * 64 * 3];
    memset(test_image, 0xFF, sizeof(test_image));

    sd35_generate_response_t resp = {
        .request_id = 1,
        .status = STATUS_OK,
        .generation_time_ms = 100,
        .image_width = 64,
        .image_height = 64,
        .channels = 3,
        .image_data_len = 64 * 64 * 3,
        .image_data = test_image,
    };

    uint8_t buffer[64 * 1024];
    size_t encoded_len;

    error_code_t err = encode_generate_response(&resp, buffer, sizeof(buffer), &encoded_len);

    ASSERT_EQ(ERR_NONE, err);
    ASSERT_EQ(64, read_u32_be(buffer + 32));
    ASSERT_EQ(64, read_u32_be(buffer + 36));

    TEST_PASS();
}

/**
 * Test: Encode large dimensions (1024x1024)
 */
void test_encode_generate_response_max_dimensions(void) {
    TEST("test_encode_generate_response_max_dimensions");

    size_t image_size = 1024 * 1024 * 3;
    uint8_t *test_image = malloc(image_size);
    ASSERT_TRUE(test_image != NULL);
    memset(test_image, 0xAA, image_size);

    sd35_generate_response_t resp = {
        .request_id = 999,
        .status = STATUS_OK,
        .generation_time_ms = 15000,
        .image_width = 1024,
        .image_height = 1024,
        .channels = 3,
        .image_data_len = (uint32_t)image_size,
        .image_data = test_image,
    };

    size_t buffer_size = 16 * 1024 * 1024;
    uint8_t *buffer = malloc(buffer_size);
    ASSERT_TRUE(buffer != NULL);

    size_t encoded_len;
    error_code_t err = encode_generate_response(&resp, buffer, buffer_size, &encoded_len);

    ASSERT_EQ(ERR_NONE, err);
    ASSERT_EQ(1024, read_u32_be(buffer + 32));
    ASSERT_EQ(1024, read_u32_be(buffer + 36));

    free(test_image);
    free(buffer);

    TEST_PASS();
}

/**
 * Test: Encode with RGBA (4 channels)
 */
void test_encode_generate_response_rgba(void) {
    TEST("test_encode_generate_response_rgba");

    uint8_t test_image[512 * 512 * 4];
    memset(test_image, 0x80, sizeof(test_image));

    sd35_generate_response_t resp = {
        .request_id = 42,
        .status = STATUS_OK,
        .generation_time_ms = 5000,
        .image_width = 512,
        .image_height = 512,
        .channels = 4,
        .image_data_len = 512 * 512 * 4,
        .image_data = test_image,
    };

    uint8_t buffer[2 * 1024 * 1024];
    size_t encoded_len;

    error_code_t err = encode_generate_response(&resp, buffer, sizeof(buffer), &encoded_len);

    ASSERT_EQ(ERR_NONE, err);
    ASSERT_EQ(4, read_u32_be(buffer + 40));
    ASSERT_EQ(512 * 512 * 4, read_u32_be(buffer + 44));

    TEST_PASS();
}

/**
 * Test: Encode fails with NULL pointer
 */
void test_encode_generate_response_null_pointers(void) {
    TEST("test_encode_generate_response_null_pointers");

    uint8_t test_image[64 * 64 * 3];
    sd35_generate_response_t resp = {
        .request_id = 1,
        .status = STATUS_OK,
        .generation_time_ms = 100,
        .image_width = 64,
        .image_height = 64,
        .channels = 3,
        .image_data_len = 64 * 64 * 3,
        .image_data = test_image,
    };

    uint8_t buffer[64 * 1024];
    size_t encoded_len;

    ASSERT_EQ(ERR_INTERNAL, encode_generate_response(NULL, buffer, sizeof(buffer), &encoded_len));
    ASSERT_EQ(ERR_INTERNAL, encode_generate_response(&resp, NULL, sizeof(buffer), &encoded_len));
    ASSERT_EQ(ERR_INTERNAL, encode_generate_response(&resp, buffer, sizeof(buffer), NULL));

    TEST_PASS();
}

/**
 * Test: Encode fails with invalid dimensions
 */
void test_encode_generate_response_invalid_dimensions(void) {
    TEST("test_encode_generate_response_invalid_dimensions");

    uint8_t test_image[512 * 512 * 3];
    uint8_t buffer[1024 * 1024];
    size_t encoded_len;

    sd35_generate_response_t resp = {
        .request_id = 1,
        .status = STATUS_OK,
        .generation_time_ms = 100,
        .image_width = 32,
        .image_height = 512,
        .channels = 3,
        .image_data_len = 32 * 512 * 3,
        .image_data = test_image,
    };

    ASSERT_EQ(ERR_INVALID_DIMENSIONS, encode_generate_response(&resp, buffer, sizeof(buffer), &encoded_len));

    resp.image_width = 513;
    resp.image_height = 512;
    ASSERT_EQ(ERR_INVALID_DIMENSIONS, encode_generate_response(&resp, buffer, sizeof(buffer), &encoded_len));

    resp.image_width = 512;
    resp.image_height = 4096;
    ASSERT_EQ(ERR_INVALID_DIMENSIONS, encode_generate_response(&resp, buffer, sizeof(buffer), &encoded_len));

    TEST_PASS();
}

/**
 * Test: Encode fails with invalid channels
 */
void test_encode_generate_response_invalid_channels(void) {
    TEST("test_encode_generate_response_invalid_channels");

    uint8_t test_image[512 * 512 * 3];
    uint8_t buffer[1024 * 1024];
    size_t encoded_len;

    sd35_generate_response_t resp = {
        .request_id = 1,
        .status = STATUS_OK,
        .generation_time_ms = 100,
        .image_width = 512,
        .image_height = 512,
        .channels = 2,
        .image_data_len = 512 * 512 * 2,
        .image_data = test_image,
    };

    ASSERT_EQ(ERR_INVALID_DIMENSIONS, encode_generate_response(&resp, buffer, sizeof(buffer), &encoded_len));

    resp.channels = 5;
    resp.image_data_len = 512 * 512 * 5;
    ASSERT_EQ(ERR_INVALID_DIMENSIONS, encode_generate_response(&resp, buffer, sizeof(buffer), &encoded_len));

    TEST_PASS();
}

/**
 * Test: Encode fails with mismatched image_data_len
 */
void test_encode_generate_response_mismatched_data_len(void) {
    TEST("test_encode_generate_response_mismatched_data_len");

    uint8_t test_image[512 * 512 * 3];
    uint8_t buffer[1024 * 1024];
    size_t encoded_len;

    sd35_generate_response_t resp = {
        .request_id = 1,
        .status = STATUS_OK,
        .generation_time_ms = 100,
        .image_width = 512,
        .image_height = 512,
        .channels = 3,
        .image_data_len = 512 * 512 * 3 - 1,
        .image_data = test_image,
    };

    ASSERT_EQ(ERR_INVALID_DIMENSIONS, encode_generate_response(&resp, buffer, sizeof(buffer), &encoded_len));

    resp.image_data_len = 512 * 512 * 3 + 100;
    ASSERT_EQ(ERR_INVALID_DIMENSIONS, encode_generate_response(&resp, buffer, sizeof(buffer), &encoded_len));

    TEST_PASS();
}

/**
 * Test: Encode fails with buffer too small
 */
void test_encode_generate_response_buffer_too_small(void) {
    TEST("test_encode_generate_response_buffer_too_small");

    uint8_t test_image[512 * 512 * 3];
    sd35_generate_response_t resp = {
        .request_id = 1,
        .status = STATUS_OK,
        .generation_time_ms = 100,
        .image_width = 512,
        .image_height = 512,
        .channels = 3,
        .image_data_len = 512 * 512 * 3,
        .image_data = test_image,
    };

    uint8_t buffer[1024];
    size_t encoded_len;

    ASSERT_EQ(ERR_INTERNAL, encode_generate_response(&resp, buffer, sizeof(buffer), &encoded_len));

    TEST_PASS();
}

/**
 * Test: Encode valid error response
 */
void test_encode_error_response_valid(void) {
    TEST("test_encode_error_response_valid");

    const char *error_msg = "invalid model id";
    error_response_t resp = {
        .request_id = 12345,
        .status = STATUS_BAD_REQUEST,
        .error_code = ERR_INVALID_MODEL_ID,
        .error_msg_len = (uint16_t)strlen(error_msg),
        .error_msg = error_msg,
    };

    uint8_t buffer[4096];
    size_t encoded_len;

    error_code_t err = encode_error_response(&resp, buffer, sizeof(buffer), &encoded_len);

    ASSERT_EQ(ERR_NONE, err);

    size_t expected_len = 16 + 8 + 4 + 4 + 2 + strlen(error_msg);
    ASSERT_EQ(expected_len, encoded_len);

    ASSERT_EQ(PROTOCOL_MAGIC, read_u32_be(buffer));
    ASSERT_EQ(PROTOCOL_VERSION_1, read_u16_be(buffer + 4));
    ASSERT_EQ(MSG_ERROR, read_u16_be(buffer + 6));
    ASSERT_EQ(8 + 4 + 4 + 2 + strlen(error_msg), read_u32_be(buffer + 8));

    ASSERT_EQ(12345, read_u64_be(buffer + 16));
    ASSERT_EQ(STATUS_BAD_REQUEST, read_u32_be(buffer + 24));
    ASSERT_EQ(ERR_INVALID_MODEL_ID, read_u32_be(buffer + 28));
    ASSERT_EQ(strlen(error_msg), read_u16_be(buffer + 32));

    ASSERT_TRUE(memcmp(buffer + 34, error_msg, strlen(error_msg)) == 0);

    TEST_PASS();
}

/**
 * Test: Encode error response with zero-length message
 */
void test_encode_error_response_empty_message(void) {
    TEST("test_encode_error_response_empty_message");

    error_response_t resp = {
        .request_id = 0,
        .status = STATUS_INTERNAL_SERVER_ERROR,
        .error_code = ERR_INTERNAL,
        .error_msg_len = 0,
        .error_msg = NULL,
    };

    uint8_t buffer[4096];
    size_t encoded_len;

    error_code_t err = encode_error_response(&resp, buffer, sizeof(buffer), &encoded_len);

    ASSERT_EQ(ERR_NONE, err);

    size_t expected_len = 16 + 8 + 4 + 4 + 2;
    ASSERT_EQ(expected_len, encoded_len);

    ASSERT_EQ(0, read_u16_be(buffer + 32));

    TEST_PASS();
}

/**
 * Test: Encode error response with long message
 */
void test_encode_error_response_long_message(void) {
    TEST("test_encode_error_response_long_message");

    char long_msg[1000];
    memset(long_msg, 'A', sizeof(long_msg) - 1);
    long_msg[sizeof(long_msg) - 1] = '\0';

    error_response_t resp = {
        .request_id = 999,
        .status = STATUS_BAD_REQUEST,
        .error_code = ERR_INVALID_PROMPT,
        .error_msg_len = (uint16_t)strlen(long_msg),
        .error_msg = long_msg,
    };

    uint8_t buffer[4096];
    size_t encoded_len;

    error_code_t err = encode_error_response(&resp, buffer, sizeof(buffer), &encoded_len);

    ASSERT_EQ(ERR_NONE, err);
    ASSERT_EQ(strlen(long_msg), read_u16_be(buffer + 32));
    ASSERT_TRUE(memcmp(buffer + 34, long_msg, strlen(long_msg)) == 0);

    TEST_PASS();
}

/**
 * Test: Encode error response fails with NULL pointers
 */
void test_encode_error_response_null_pointers(void) {
    TEST("test_encode_error_response_null_pointers");

    error_response_t resp = {
        .request_id = 1,
        .status = STATUS_BAD_REQUEST,
        .error_code = ERR_INVALID_MODEL_ID,
        .error_msg_len = 4,
        .error_msg = "test",
    };

    uint8_t buffer[4096];
    size_t encoded_len;

    ASSERT_EQ(ERR_INTERNAL, encode_error_response(NULL, buffer, sizeof(buffer), &encoded_len));
    ASSERT_EQ(ERR_INTERNAL, encode_error_response(&resp, NULL, sizeof(buffer), &encoded_len));
    ASSERT_EQ(ERR_INTERNAL, encode_error_response(&resp, buffer, sizeof(buffer), NULL));

    TEST_PASS();
}

/**
 * Test: Encode error response fails with buffer too small
 */
void test_encode_error_response_buffer_too_small(void) {
    TEST("test_encode_error_response_buffer_too_small");

    const char *error_msg = "this is a long error message";
    error_response_t resp = {
        .request_id = 1,
        .status = STATUS_BAD_REQUEST,
        .error_code = ERR_INVALID_PROMPT,
        .error_msg_len = (uint16_t)strlen(error_msg),
        .error_msg = error_msg,
    };

    uint8_t buffer[32];
    size_t encoded_len;

    ASSERT_EQ(ERR_INTERNAL, encode_error_response(&resp, buffer, sizeof(buffer), &encoded_len));

    TEST_PASS();
}

/**
 * Main test runner
 */
int main(void) {
    printf("Running protocol tests...\n\n");

    printf("=== Decoder Tests ===\n");
    test_valid_request_typical();
    test_valid_request_min_dimensions();
    test_valid_request_max_dimensions();

    test_invalid_magic();
    test_unsupported_version_too_high();
    test_unsupported_version_zero();

    test_invalid_model_id();

    test_dimensions_too_small();
    test_dimensions_too_large();
    test_dimensions_not_aligned();

    test_steps_zero();
    test_steps_too_high();

    test_cfg_negative();
    test_cfg_too_high();
    test_cfg_nan();
    test_cfg_inf();

    test_prompt_offset_out_of_bounds();
    test_prompt_length_exceeds_data();
    test_prompt_length_zero();
    test_prompt_length_too_large();

    test_truncated_header();
    test_truncated_payload();

    test_null_data_pointer();
    test_null_request_pointer();

    test_prompt_offset_overflow();

    printf("\n=== Encoder Tests ===\n");
    test_encode_generate_response_valid();
    test_encode_generate_response_min_dimensions();
    test_encode_generate_response_max_dimensions();
    test_encode_generate_response_rgba();
    test_encode_generate_response_null_pointers();
    test_encode_generate_response_invalid_dimensions();
    test_encode_generate_response_invalid_channels();
    test_encode_generate_response_mismatched_data_len();
    test_encode_generate_response_buffer_too_small();

    test_encode_error_response_valid();
    test_encode_error_response_empty_message();
    test_encode_error_response_long_message();
    test_encode_error_response_null_pointers();
    test_encode_error_response_buffer_too_small();

    printf("\n========================================\n");
    printf("Tests run: %d\n", tests_run);
    printf("Tests passed: %d\n", tests_passed);
    printf("Tests failed: %d\n", tests_run - tests_passed);
    printf("========================================\n");

    return (tests_run == tests_passed) ? 0 : 1;
}
