/**
 * Weave Compute Daemon - Main Entry Point
 *
 * This is the main entry point for the weave-compute daemon. It handles:
 * - Signal setup (SIGTERM, SIGINT for graceful shutdown)
 * - SD model loading
 * - Socket creation
 * - Accept loop with request processing
 * - Cleanup on exit
 *
 * Usage: weave-compute
 *
 * The daemon listens on $XDG_RUNTIME_DIR/weave/weave.sock and authenticates
 * connections using SO_PEERCRED.
 */

#define _GNU_SOURCE

#include <errno.h>
#include <signal.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

#include "weave/generate.h"
#include "weave/protocol.h"
#include "weave/sd_wrapper.h"
#include "weave/socket.h"

/**
 * Maximum message size for reading requests.
 * Must match MAX_MESSAGE_SIZE from protocol.h (10 MB).
 */
#define MAX_REQUEST_SIZE (10 * 1024 * 1024)

/**
 * Model paths (hardcoded for MVP).
 */
#define MODEL_PATH "./models/sd3.5_medium.safetensors"
#define CLIP_L_PATH "./models/clip_l.safetensors"
#define CLIP_G_PATH "./models/clip_g.safetensors"
#define T5XXL_PATH "./models/t5xxl_fp8_e4m3fn.safetensors"

/**
 * Global listen socket for cleanup in main thread only.
 * NOT accessed from signal handlers - only socket_request_shutdown() is
 * called from the signal handler, which sets a volatile flag.
 */
static int g_listen_fd = -1;

/**
 * Global SD wrapper context for cleanup.
 * NOT accessed from signal handlers.
 *
 * IMPORTANT: The SD wrapper context is NOT thread-safe. This daemon handles
 * connections sequentially (one at a time) in the main thread. Do NOT add
 * concurrent request processing without adding proper synchronization.
 */
static sd_wrapper_ctx_t *g_sd_ctx = NULL;

/**
 * signal_handler - Handle SIGTERM and SIGINT for graceful shutdown
 *
 * This is an async-signal-safe signal handler. It only calls
 * socket_request_shutdown() which just sets a volatile flag.
 *
 * @param signum  Signal number (SIGTERM or SIGINT)
 */
static void signal_handler(int signum) {
    (void)signum;
    socket_request_shutdown();
}

/**
 * setup_signals - Install signal handlers for graceful shutdown
 *
 * @return  0 on success, -1 on failure
 */
static int setup_signals(void) {
    struct sigaction sa;

    sa.sa_handler = signal_handler;
    sigemptyset(&sa.sa_mask);
    sa.sa_flags = 0;

    if (sigaction(SIGTERM, &sa, NULL) != 0) {
        perror("sigaction(SIGTERM)");
        return -1;
    }

    if (sigaction(SIGINT, &sa, NULL) != 0) {
        perror("sigaction(SIGINT)");
        return -1;
    }

    return 0;
}

/**
 * read_full - Read exactly n bytes from socket
 *
 * Handles partial reads and EINTR. Respects socket timeout.
 *
 * @param fd     Socket file descriptor
 * @param buf    Buffer to read into
 * @param count  Number of bytes to read
 * @return       0 on success, -1 on error or timeout
 */
static int read_full(int fd, uint8_t *buf, size_t count) {
    size_t total = 0;

    while (total < count) {
        ssize_t n = read(fd, buf + total, count - total);
        if (n < 0) {
            if (errno == EINTR) {
                continue;
            }
            return -1;
        }
        if (n == 0) {
            return -1;
        }
        total += (size_t)n;
    }

    return 0;
}

/**
 * write_full - Write exactly n bytes to socket
 *
 * Handles partial writes and EINTR. Respects socket timeout.
 *
 * @param fd     Socket file descriptor
 * @param buf    Buffer to write from
 * @param count  Number of bytes to write
 * @return       0 on success, -1 on error or timeout
 */
static int write_full(int fd, const uint8_t *buf, size_t count) {
    size_t total = 0;

    while (total < count) {
        ssize_t n = write(fd, buf + total, count - total);
        if (n < 0) {
            if (errno == EINTR) {
                continue;
            }
            return -1;
        }
        total += (size_t)n;
    }

    return 0;
}

/**
 * is_server_error - Check if error code indicates a server-side error
 *
 * Uses explicit mapping instead of fragile numeric comparison.
 * Server errors (500) are issues with the daemon itself.
 * Client errors (400) are issues with the request.
 *
 * @param code  Error code to check
 * @return      true if server error (500), false if client error (400)
 */
static int is_server_error(error_code_t code) {
    switch (code) {
    /* Server-side errors (500) */
    case ERR_OUT_OF_MEMORY:
    case ERR_GPU_ERROR:
    case ERR_TIMEOUT:
    case ERR_INTERNAL:
        return 1;

    /* Client-side errors (400) - everything else */
    case ERR_NONE:
    case ERR_INVALID_MAGIC:
    case ERR_UNSUPPORTED_VERSION:
    case ERR_INVALID_MODEL_ID:
    case ERR_INVALID_PROMPT:
    case ERR_INVALID_DIMENSIONS:
    case ERR_INVALID_STEPS:
    case ERR_INVALID_CFG:
    default:
        return 0;
    }
}

/**
 * send_error_response - Send error response to client
 *
 * @param client_fd  Client socket
 * @param request_id Request ID to echo (0 if request was invalid)
 * @param error_code Protocol error code
 * @param error_msg  Human-readable error message
 * @return           0 on success, -1 on failure
 */
static int send_error_response(int client_fd, uint64_t request_id,
                                error_code_t error_code, const char *error_msg) {
    /* All variable declarations at top for C99 compliance */
    error_response_t err_resp;
    uint8_t response_buf[4096];
    size_t response_len;
    error_code_t encode_err;

    err_resp.request_id = request_id;
    err_resp.error_code = (uint32_t)error_code;
    err_resp.error_msg = error_msg;
    err_resp.error_msg_len = (uint16_t)strlen(error_msg);

    if (is_server_error(error_code)) {
        err_resp.status = STATUS_INTERNAL_SERVER_ERROR;
    } else {
        err_resp.status = STATUS_BAD_REQUEST;
    }

    /* Error responses are small: header (16) + metadata (16) + msg (<1KB) */
    encode_err = encode_error_response(&err_resp, response_buf,
                                       sizeof(response_buf),
                                       &response_len);
    if (encode_err != ERR_NONE) {
        return -1;
    }

    if (write_full(client_fd, response_buf, response_len) != 0) {
        return -1;
    }

    return 0;
}

/**
 * handle_connection - Process a client connection
 *
 * This function:
 * 1. Reads request from socket (header then payload)
 * 2. Decodes and validates request
 * 3. Processes generation request
 * 4. Encodes and sends response (reusing request buffer)
 *
 * Note: We reuse request_buf for the response to save memory (10MB).
 * This is safe because we're done with the request data by the time we encode.
 *
 * @param client_fd  Authenticated client socket
 * @return           0 on success, non-zero on error
 */
static int handle_connection(int client_fd) {
    /* All variable declarations at top for C99 compliance */
    uint8_t header[16];
    uint8_t *buffer = NULL;
    int result = -1;
    uint32_t magic;
    uint32_t payload_len;
    size_t total_size;
    sd35_generate_request_t req;
    sd35_generate_response_t resp;
    error_code_t err;
    size_t response_len;

    /*
     * Security: Read header into small stack buffer first, validate payload
     * length, then allocate only the exact size needed. This prevents memory
     * exhaustion attacks where an attacker sends headers claiming large
     * payloads to force expensive allocations.
     */

    /* Step 1: Read 16-byte header into stack buffer */
    if (read_full(client_fd, header, 16) != 0) {
        fprintf(stderr, "failed to read request header\n");
        return -1;
    }

    /* Step 2: Validate magic number before any allocation */
    magic = (uint32_t)header[0] << 24 |
            (uint32_t)header[1] << 16 |
            (uint32_t)header[2] << 8 |
            (uint32_t)header[3];

    if (magic != PROTOCOL_MAGIC) {
        fprintf(stderr, "invalid magic number: 0x%08x\n", magic);
        send_error_response(client_fd, 0, ERR_INVALID_MAGIC, "invalid magic number");
        return -1;
    }

    /* Step 3: Validate payload length before allocation */
    payload_len = (uint32_t)header[8] << 24 |
                  (uint32_t)header[9] << 16 |
                  (uint32_t)header[10] << 8 |
                  (uint32_t)header[11];

    if (payload_len > MAX_REQUEST_SIZE - 16) {
        fprintf(stderr, "request payload too large: %u bytes\n", payload_len);
        send_error_response(client_fd, 0, ERR_INTERNAL, "payload too large");
        return -1;
    }

    /* Step 4: Allocate exact size needed (header + payload) */
    total_size = 16 + payload_len;
    buffer = malloc(total_size);
    if (buffer == NULL) {
        fprintf(stderr, "failed to allocate buffer (%zu bytes)\n", total_size);
        return -1;
    }

    /* Copy header into buffer */
    memcpy(buffer, header, 16);

    /* Step 5: Read payload if present */
    if (payload_len > 0) {
        if (read_full(client_fd, buffer + 16, payload_len) != 0) {
            fprintf(stderr, "failed to read request payload\n");
            goto cleanup;
        }
    }

    err = decode_generate_request(buffer, 16 + payload_len, &req);
    if (err != ERR_NONE) {
        fprintf(stderr, "failed to decode request: %d\n", err);
        send_error_response(client_fd, 0, err, "invalid request");
        goto cleanup;
    }

    memset(&resp, 0, sizeof(resp));

    err = process_generate_request(g_sd_ctx, &req, &resp);
    if (err != ERR_NONE) {
        fprintf(stderr, "generation failed: %d\n", err);
        send_error_response(client_fd, req.request_id, err, "generation failed");
        goto cleanup;
    }

    /*
     * Response may be larger than request (contains image data).
     * Reallocate buffer to hold the response.
     * Response size = header (16) + response metadata (16) + image metadata (16) + image data.
     */
    {
        size_t response_buf_size = 16 + 16 + 16 + resp.image_data_len;
        uint8_t *response_buf = realloc(buffer, response_buf_size);
        if (response_buf == NULL) {
            fprintf(stderr, "failed to allocate response buffer (%zu bytes)\n",
                    response_buf_size);
            free_generate_response(&resp);
            goto cleanup;
        }
        buffer = response_buf;

        err = encode_generate_response(&resp, buffer, response_buf_size, &response_len);
    }
    free_generate_response(&resp);

    if (err != ERR_NONE) {
        fprintf(stderr, "failed to encode response: %d\n", err);
        goto cleanup;
    }

    if (write_full(client_fd, buffer, response_len) != 0) {
        fprintf(stderr, "failed to write response\n");
        goto cleanup;
    }

    result = 0;

cleanup:
    if (buffer != NULL) {
        free(buffer);
    }
    return result;
}

/**
 * cleanup - Clean up resources before exit
 */
static void cleanup(void) {
    if (g_sd_ctx != NULL) {
        fprintf(stderr, "unloading model...\n");
        sd_wrapper_free(g_sd_ctx);
        g_sd_ctx = NULL;
    }

    if (g_listen_fd >= 0) {
        close(g_listen_fd);
        g_listen_fd = -1;
    }

    socket_cleanup();
}

int main(void) {
    int exit_code = EXIT_FAILURE;
    socket_error_t err;

    fprintf(stderr, "weave-compute starting...\n");

    if (setup_signals() != 0) {
        fprintf(stderr, "failed to set up signal handlers\n");
        return EXIT_FAILURE;
    }

    fprintf(stderr, "loading model from %s...\n", MODEL_PATH);
    sd_wrapper_config_t config;
    sd_wrapper_config_init(&config);
    config.model_path = MODEL_PATH;
    config.clip_l_path = CLIP_L_PATH;
    config.clip_g_path = CLIP_G_PATH;
    config.t5xxl_path = T5XXL_PATH;
    config.n_threads = -1;
    config.keep_clip_on_cpu = true;   /* Text encoders on CPU to save VRAM */
    config.keep_vae_on_cpu = false;
    config.enable_flash_attn = true;

    g_sd_ctx = sd_wrapper_create(&config);
    if (g_sd_ctx == NULL) {
        fprintf(stderr, "failed to load model: %s\n", MODEL_PATH);
        fprintf(stderr, "ensure model file exists and is a valid SD 3.5 Medium model\n");
        return EXIT_FAILURE;
    }

    fprintf(stderr, "model loaded successfully\n");

    err = socket_create(&g_listen_fd);
    if (err != SOCKET_OK) {
        fprintf(stderr, "socket_create failed: %s\n", socket_error_string(err));
        cleanup();
        return EXIT_FAILURE;
    }

    char socket_path[SOCKET_PATH_MAX];
    if (socket_get_path(socket_path, sizeof(socket_path)) == SOCKET_OK) {
        fprintf(stderr, "listening on %s\n", socket_path);
    }

    err = socket_accept_loop(g_listen_fd, handle_connection);
    if (err == SOCKET_OK) {
        fprintf(stderr, "shutting down gracefully\n");
        exit_code = EXIT_SUCCESS;
    } else {
        fprintf(stderr, "accept loop failed: %s\n", socket_error_string(err));
    }

    cleanup();

    fprintf(stderr, "weave-compute stopped\n");
    return exit_code;
}
