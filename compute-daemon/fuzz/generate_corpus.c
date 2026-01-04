/**
 * Corpus Generator for Protocol Fuzzer
 *
 * Generates seed inputs for the protocol fuzzer. These seeds provide
 * good coverage of the protocol surface and help the fuzzer find bugs faster.
 *
 * Build:
 *   gcc -std=c99 -Wall -Wextra -I../include -o generate_corpus \
 *       generate_corpus.c -lm
 *
 * Run:
 *   ./generate_corpus corpus/
 */

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <stdint.h>
#include <math.h>
#include "weave/protocol.h"

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

    /* Common header (16 bytes) */
    write_u32_be(ptr, PROTOCOL_MAGIC);
    ptr += 4;
    write_u16_be(ptr, PROTOCOL_VERSION_1);
    ptr += 2;
    write_u16_be(ptr, MSG_GENERATE_REQUEST);
    ptr += 2;
    write_u32_be(ptr, (uint32_t)payload_len);
    ptr += 4;
    write_u32_be(ptr, 0); /* reserved */
    ptr += 4;

    /* Request metadata (12 bytes) */
    write_u64_be(ptr, request_id);
    ptr += 8;
    write_u32_be(ptr, MODEL_ID_SD35);
    ptr += 4;

    /* Generation parameters (48 bytes) */
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

    /* Prompt offset table (24 bytes) */
    write_u32_be(ptr, 0); /* clip_l_offset */
    ptr += 4;
    write_u32_be(ptr, (uint32_t)prompt_len);
    ptr += 4;
    write_u32_be(ptr, (uint32_t)prompt_len); /* clip_g_offset */
    ptr += 4;
    write_u32_be(ptr, (uint32_t)prompt_len);
    ptr += 4;
    write_u32_be(ptr, (uint32_t)(prompt_len * 2)); /* t5_offset */
    ptr += 4;
    write_u32_be(ptr, (uint32_t)prompt_len);
    ptr += 4;

    /* Prompt data (3 copies) */
    memcpy(ptr, prompt, prompt_len);
    ptr += prompt_len;
    memcpy(ptr, prompt, prompt_len);
    ptr += prompt_len;
    memcpy(ptr, prompt, prompt_len);

    return total_len;
}

/**
 * Helper: Write buffer to file
 */
static int write_corpus_file(const char *dir, const char *name,
                             const uint8_t *data, size_t len) {
    char path[512];
    snprintf(path, sizeof(path), "%s/%s", dir, name);

    FILE *f = fopen(path, "wb");
    if (f == NULL) {
        fprintf(stderr, "Failed to create %s\n", path);
        return -1;
    }

    size_t written = fwrite(data, 1, len, f);
    fclose(f);

    if (written != len) {
        fprintf(stderr, "Failed to write %s (wrote %zu of %zu bytes)\n",
                path, written, len);
        return -1;
    }

    printf("Generated: %s (%zu bytes)\n", name, len);
    return 0;
}

int main(int argc, char **argv) {
    if (argc != 2) {
        fprintf(stderr, "Usage: %s <corpus_dir>\n", argv[0]);
        return 1;
    }

    const char *corpus_dir = argv[1];
    uint8_t buffer[8192];
    size_t len;

    printf("Generating corpus files in: %s\n\n", corpus_dir);

    /* 1. Valid request - typical parameters */
    len = build_valid_request(buffer, sizeof(buffer),
                              12345, 512, 512, 28, 7.0f, 0,
                              "a cat in space");
    if (len == 0 || write_corpus_file(corpus_dir, "valid_typical", buffer, len) != 0) {
        return 1;
    }

    /* 2. Valid request - minimum dimensions */
    len = build_valid_request(buffer, sizeof(buffer),
                              1, 64, 64, 1, 0.0f, 0,
                              "test");
    if (len == 0 || write_corpus_file(corpus_dir, "valid_min_dimensions", buffer, len) != 0) {
        return 1;
    }

    /* 3. Valid request - maximum dimensions */
    len = build_valid_request(buffer, sizeof(buffer),
                              999, 2048, 2048, 100, 20.0f, UINT64_MAX,
                              "test");
    if (len == 0 || write_corpus_file(corpus_dir, "valid_max_dimensions", buffer, len) != 0) {
        return 1;
    }

    /* 4. Valid request - long prompt (near max) */
    char long_prompt[2048];
    memset(long_prompt, 'A', sizeof(long_prompt) - 1);
    long_prompt[sizeof(long_prompt) - 1] = '\0';
    len = build_valid_request(buffer, sizeof(buffer),
                              42, 512, 512, 28, 7.0f, 12345678,
                              long_prompt);
    if (len == 0 || write_corpus_file(corpus_dir, "valid_max_prompt", buffer, len) != 0) {
        return 1;
    }

    /* 5. Valid request - UTF-8 prompt */
    len = build_valid_request(buffer, sizeof(buffer),
                              100, 512, 512, 28, 7.0f, 0,
                              "cat sitting on 火星");
    if (len == 0 || write_corpus_file(corpus_dir, "valid_utf8_prompt", buffer, len) != 0) {
        return 1;
    }

    /* 6. Empty buffer (edge case) */
    if (write_corpus_file(corpus_dir, "empty", buffer, 0) != 0) {
        return 1;
    }

    /* 7. Truncated header (15 bytes) */
    len = build_valid_request(buffer, sizeof(buffer),
                              1, 512, 512, 28, 7.0f, 0, "test");
    if (len == 0 || write_corpus_file(corpus_dir, "truncated_header", buffer, 15) != 0) {
        return 1;
    }

    /* 8. Invalid magic number */
    len = build_valid_request(buffer, sizeof(buffer),
                              1, 512, 512, 28, 7.0f, 0, "test");
    write_u32_be(buffer, 0xDEADBEEF);
    if (len == 0 || write_corpus_file(corpus_dir, "invalid_magic", buffer, len) != 0) {
        return 1;
    }

    /* 9. Unsupported version */
    len = build_valid_request(buffer, sizeof(buffer),
                              1, 512, 512, 28, 7.0f, 0, "test");
    write_u16_be(buffer + 4, 0x9999);
    if (len == 0 || write_corpus_file(corpus_dir, "invalid_version", buffer, len) != 0) {
        return 1;
    }

    /* 10. Invalid dimensions (not aligned) */
    len = build_valid_request(buffer, sizeof(buffer),
                              1, 513, 512, 28, 7.0f, 0, "test");
    if (len == 0 || write_corpus_file(corpus_dir, "invalid_dimensions", buffer, len) != 0) {
        return 1;
    }

    /* 11. Invalid steps (zero) */
    len = build_valid_request(buffer, sizeof(buffer),
                              1, 512, 512, 0, 7.0f, 0, "test");
    if (len == 0 || write_corpus_file(corpus_dir, "invalid_steps", buffer, len) != 0) {
        return 1;
    }

    /* 12. Invalid CFG (NaN) */
    len = build_valid_request(buffer, sizeof(buffer),
                              1, 512, 512, 28, NAN, 0, "test");
    if (len == 0 || write_corpus_file(corpus_dir, "invalid_cfg_nan", buffer, len) != 0) {
        return 1;
    }

    /* 13. Random bytes (fuzzer starting point) */
    for (size_t i = 0; i < 256; i++) {
        buffer[i] = (uint8_t)i;
    }
    if (write_corpus_file(corpus_dir, "random_bytes", buffer, 256) != 0) {
        return 1;
    }

    printf("\nGenerated %d corpus files successfully.\n", 13);
    printf("Run fuzzer with: ./fuzz_protocol %s\n", corpus_dir);

    return 0;
}
