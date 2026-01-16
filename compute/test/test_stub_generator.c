/**
 * Test Stub Generator
 *
 * This program reads a binary protocol request from stdin, decodes it,
 * generates a test pattern image (checkerboard), encodes a response,
 * and writes it to stdout.
 *
 * This is used for integration testing to verify round-trip encoding/decoding
 * between Go and C without requiring actual GPU computation.
 *
 * Usage:
 *   echo <binary_request> | ./test_stub_generator > response.bin
 */

#include <stdio.h>
#include <stdlib.h>
#include <stdint.h>
#include <string.h>
#include "weave/protocol.h"

#define MAX_MESSAGE_SIZE (10 * 1024 * 1024)

/**
 * generate_checkerboard - Create a checkerboard test pattern
 *
 * Generates alternating blocks of 0x00 (black) and 0xFF (white).
 * Block size is 8x8 pixels for visual clarity at small sizes.
 *
 * @param width     Image width in pixels
 * @param height    Image height in pixels
 * @param channels  Number of channels (3=RGB, 4=RGBA)
 * @param data      Output buffer (must be width * height * channels bytes)
 */
static void generate_checkerboard(uint32_t width, uint32_t height,
                                   uint32_t channels, uint8_t *data) {
    const uint32_t block_size = 8;

    for (uint32_t y = 0; y < height; y++) {
        for (uint32_t x = 0; x < width; x++) {
            uint32_t block_x = x / block_size;
            uint32_t block_y = y / block_size;
            uint8_t value = ((block_x + block_y) % 2) ? 0xFF : 0x00;

            uint32_t pixel_offset = (y * width + x) * channels;
            for (uint32_t c = 0; c < channels; c++) {
                data[pixel_offset + c] = value;
            }
        }
    }
}

/**
 * read_all - Read exactly len bytes from stdin
 *
 * @param buffer    Output buffer
 * @param len       Number of bytes to read
 * @return          0 on success, -1 on error
 */
static int read_all(uint8_t *buffer, size_t len) {
    size_t total_read = 0;
    while (total_read < len) {
        size_t n = fread(buffer + total_read, 1, len - total_read, stdin);
        if (n == 0) {
            if (feof(stdin)) {
                fprintf(stderr, "EOF: expected %zu bytes, got %zu\n", len, total_read);
                return -1;
            }
            if (ferror(stdin)) {
                perror("fread");
                return -1;
            }
        }
        total_read += n;
    }
    return 0;
}

/**
 * write_all - Write exactly len bytes to stdout
 *
 * @param buffer    Input buffer
 * @param len       Number of bytes to write
 * @return          0 on success, -1 on error
 */
static int write_all(const uint8_t *buffer, size_t len) {
    size_t total_written = 0;
    while (total_written < len) {
        size_t n = fwrite(buffer + total_written, 1, len - total_written, stdout);
        if (n == 0) {
            if (ferror(stdout)) {
                perror("fwrite");
                return -1;
            }
        }
        total_written += n;
    }
    return 0;
}

int main(void) {
    uint8_t *request_buffer = NULL;
    uint8_t *response_buffer = NULL;
    uint8_t *image_data = NULL;
    int exit_code = 1;

    /* Read header first to get payload length */
    uint8_t header_buf[16];
    if (read_all(header_buf, 16) != 0) {
        fprintf(stderr, "Failed to read header\n");
        goto cleanup;
    }

    /* Decode magic and payload_len to determine total message size */
    uint32_t magic = ((uint32_t)header_buf[0] << 24) |
                     ((uint32_t)header_buf[1] << 16) |
                     ((uint32_t)header_buf[2] << 8) |
                     ((uint32_t)header_buf[3]);

    uint32_t payload_len = ((uint32_t)header_buf[8] << 24) |
                           ((uint32_t)header_buf[9] << 16) |
                           ((uint32_t)header_buf[10] << 8) |
                           ((uint32_t)header_buf[11]);

    if (magic != PROTOCOL_MAGIC) {
        fprintf(stderr, "Invalid magic: 0x%08X\n", magic);
        goto cleanup;
    }

    if (payload_len > MAX_MESSAGE_SIZE - 16) {
        fprintf(stderr, "Payload too large: %u\n", payload_len);
        goto cleanup;
    }

    size_t total_len = 16 + payload_len;

    /* Allocate buffer and copy header */
    request_buffer = malloc(total_len);
    if (request_buffer == NULL) {
        fprintf(stderr, "Failed to allocate request buffer\n");
        goto cleanup;
    }

    memcpy(request_buffer, header_buf, 16);

    /* Read rest of message */
    if (read_all(request_buffer + 16, payload_len) != 0) {
        fprintf(stderr, "Failed to read payload\n");
        goto cleanup;
    }

    /* Decode request */
    sd35_generate_request_t req;
    error_code_t err = decode_generate_request(request_buffer, total_len, &req);
    if (err != ERR_NONE) {
        fprintf(stderr, "Failed to decode request: error code %d\n", err);
        goto cleanup;
    }

    /* Determine channels (use RGB for simplicity) */
    uint32_t channels = 3; /* RGB */

    /* Calculate image size */
    uint32_t image_size = req.width * req.height * channels;
    image_data = malloc(image_size);
    if (image_data == NULL) {
        fprintf(stderr, "Failed to allocate image data\n");
        goto cleanup;
    }

    /* Generate checkerboard test pattern */
    generate_checkerboard(req.width, req.height, channels, image_data);

    /* Create response */
    sd35_generate_response_t resp = {
        .request_id = req.request_id,
        .status = STATUS_OK,
        .generation_time_ms = 0, /* Instant generation */
        .image_width = req.width,
        .image_height = req.height,
        .channels = channels,
        .image_data_len = image_size,
        .image_data = image_data,
    };

    /* Allocate response buffer (header + payload) */
    size_t response_size = 16 + 16 + 16 + image_size; /* header + common + image metadata + data */
    response_buffer = malloc(response_size);
    if (response_buffer == NULL) {
        fprintf(stderr, "Failed to allocate response buffer\n");
        goto cleanup;
    }

    /* Encode response */
    size_t encoded_len;
    err = encode_generate_response(&resp, response_buffer, response_size, &encoded_len);
    if (err != ERR_NONE) {
        fprintf(stderr, "Failed to encode response: error code %d\n", err);
        goto cleanup;
    }

    /* Write response to stdout */
    if (write_all(response_buffer, encoded_len) != 0) {
        fprintf(stderr, "Failed to write response\n");
        goto cleanup;
    }

    /* Success */
    exit_code = 0;

cleanup:
    free(request_buffer);
    free(response_buffer);
    free(image_data);
    return exit_code;
}
