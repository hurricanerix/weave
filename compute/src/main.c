/**
 * Weave Compute - Main Entry Point
 *
 * This is the main entry point for weave-compute. It handles:
 * - Signal setup (SIGTERM, SIGINT for graceful shutdown)
 * - SD model loading
 * - Socket creation or connection
 * - Accept loop (server mode) or request/response loop (client mode)
 * - Stdin monitoring (client mode only) for parent death detection
 * - Cleanup on exit
 *
 * Operational modes:
 * 1. Server mode (no --socket-path): Creates and owns socket, accepts connections
 * 2. Client mode (--socket-path provided): Connects to existing socket, processes
 *    requests over persistent connection, monitors stdin for parent death
 *
 * Usage:
 *   weave-compute                      # Server mode (backward compatibility)
 *   weave-compute --socket-path PATH   # Client mode (spawned by weave)
 *
 * weave-compute authenticates connections using SO_PEERCRED (same-UID only).
 */

#define _GNU_SOURCE

#include <errno.h>
#include <getopt.h>
#include <pthread.h>
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
 * Global socket file descriptor for cleanup in main thread only.
 * In server mode: listening socket for accepting connections.
 * In client mode: connected socket for processing requests.
 * NOT accessed from signal handlers - only socket_request_shutdown() is
 * called from the signal handler, which sets a volatile flag.
 */
static int g_socket_fd = -1;

/**
 * Global flag indicating whether we created the socket (and thus own it).
 * If true, we must call socket_cleanup() on shutdown.
 * If false, socket was created by parent (weave), so we skip cleanup.
 */
static int g_socket_owned = 0;

/**
 * Global SD wrapper context for cleanup.
 * NOT accessed from signal handlers.
 *
 * IMPORTANT: The SD wrapper context is NOT thread-safe. weave-compute handles
 * connections sequentially (one at a time) in the main thread. Do NOT add
 * concurrent request processing without adding proper synchronization.
 */
static sd_wrapper_ctx_t *g_sd_ctx = NULL;

/**
 * Global stdin monitoring thread handle.
 * Used for debugging only - the thread is detached and cleans itself up.
 * NOT used for cleanup or joining. Set to 0 if thread is not running.
 */
static pthread_t g_stdin_monitor_thread = 0;

/**
 * print_usage - Print usage information and exit
 *
 * @param program_name  Name of the program (argv[0])
 * @param exit_code     Exit code (0 for help, non-zero for error)
 */
static void print_usage(const char *program_name, int exit_code) {
    FILE *stream = (exit_code == 0) ? stdout : stderr;

    fprintf(stream, "Usage: %s [OPTIONS]\n", program_name);
    fprintf(stream, "\n");
    fprintf(stream, "Weave compute process for GPU-accelerated image generation.\n");
    fprintf(stream, "\n");
    fprintf(stream, "Options:\n");
    fprintf(stream, "  --socket-path PATH  Unix socket path (default: $XDG_RUNTIME_DIR/weave/weave.sock)\n");
    fprintf(stream, "  -h, --help          Show this help message and exit\n");
    fprintf(stream, "\n");
    fprintf(stream, "weave-compute loads SD 3.5 Medium and processes image generation requests.\n");
    fprintf(stream, "It uses SO_PEERCRED authentication (same-UID only).\n");

    exit(exit_code);
}

/**
 * stdin_monitor_thread - Monitor stdin for closure to detect parent death
 *
 * This thread runs in client mode (when compute connects to weave's socket).
 * It performs a blocking read on stdin. When stdin closes (indicating the
 * parent process has died), it calls socket_request_shutdown() to trigger
 * graceful termination.
 *
 * This is a fallback mechanism. The primary shutdown signal comes from the
 * socket connection closing. However, if the socket somehow remains open but
 * the parent dies, stdin closure provides a reliable detection mechanism.
 *
 * Implementation notes:
 * - Uses blocking read() on stdin (fd 0)
 * - read() returns 0 on EOF (stdin closed)
 * - read() returns -1 on error
 * - Thread detaches itself for automatic cleanup
 * - No mutex needed - socket_request_shutdown() is thread-safe
 *
 * @param arg  Unused (required by pthread signature)
 * @return     NULL (required by pthread signature)
 */
static void *stdin_monitor_thread(void *arg) {
    char buf[1];
    ssize_t n;

    (void)arg; /* Unused parameter */

    /*
     * Perform a blocking read on stdin. This will block until:
     * 1. stdin closes (parent died) - read() returns 0
     * 2. Data is available (unexpected) - read() returns >0
     * 3. Error occurs - read() returns -1
     *
     * Retry on EINTR (signal interruption). This handles cases where
     * the read() is interrupted by a non-fatal signal.
     */
    do {
        n = read(STDIN_FILENO, buf, sizeof(buf));
    } while (n == -1 && errno == EINTR);

    /*
     * If read returns 0, stdin closed (parent died).
     * If read returns -1, an error occurred.
     * If read returns >0, unexpected data on stdin (parent sent something).
     *
     * In all cases, request shutdown. The parent should never send data to
     * compute's stdin in normal operation, so any activity is abnormal.
     */
    if (n <= 0) {
        /*
         * stdin closed or error - parent likely died.
         * Request graceful shutdown.
         */
        if (n == 0) {
            fprintf(stderr, "stdin closed, parent process died\n");
        } else {
            fprintf(stderr, "stdin read error: %s\n", strerror(errno));
        }
    } else {
        /*
         * Unexpected: parent sent data to stdin.
         * This should not happen in normal operation.
         */
        fprintf(stderr, "unexpected data on stdin, shutting down\n");
    }

    socket_request_shutdown();
    return NULL;
}

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
 * Server errors (500) are issues with weave-compute itself.
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
 * handle_connection - Process a single request on a client connection
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
 * Return value semantics:
 * - 0: Request processed successfully, connection still active (continue loop)
 * - -1: Connection closed by peer or fatal error (exit loop)
 *
 * Protocol errors (invalid requests) are handled by sending error responses
 * and returning 0 to continue processing. Only connection loss or fatal
 * errors return -1.
 *
 * @param client_fd  Authenticated client socket
 * @return           0 on success (continue), -1 on connection close/fatal error (exit)
 */
static int handle_connection(int client_fd) {
    /* All variable declarations at top for C99 compliance */
    uint8_t header[16];
    uint8_t *buffer = NULL;
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

    /*
     * Step 1: Read 16-byte header into stack buffer.
     * If read fails, it's either connection close or I/O error.
     * In both cases, we return -1 to exit the request loop.
     */
    if (read_full(client_fd, header, 16) != 0) {
        /* Connection closed or I/O error - exit loop */
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
        /* Protocol error - send error response and continue processing */
        return 0;
    }

    /* Step 3: Validate payload length before allocation */
    payload_len = (uint32_t)header[8] << 24 |
                  (uint32_t)header[9] << 16 |
                  (uint32_t)header[10] << 8 |
                  (uint32_t)header[11];

    if (payload_len > MAX_REQUEST_SIZE - 16) {
        fprintf(stderr, "request payload too large: %u bytes\n", payload_len);
        send_error_response(client_fd, 0, ERR_INTERNAL, "payload too large");
        /* Protocol error - send error response and continue processing */
        return 0;
    }

    /* Step 4: Allocate exact size needed (header + payload) */
    total_size = 16 + payload_len;
    buffer = malloc(total_size);
    if (buffer == NULL) {
        fprintf(stderr, "failed to allocate buffer (%zu bytes)\n", total_size);
        /* Out of memory - fatal error, exit loop */
        return -1;
    }

    /* Copy header into buffer */
    memcpy(buffer, header, 16);

    /* Step 5: Read payload if present */
    if (payload_len > 0) {
        if (read_full(client_fd, buffer + 16, payload_len) != 0) {
            /* Connection closed or I/O error - exit loop */
            free(buffer);
            return -1;
        }
    }

    err = decode_generate_request(buffer, 16 + payload_len, &req);
    if (err != ERR_NONE) {
        fprintf(stderr, "failed to decode request: %d\n", err);
        send_error_response(client_fd, 0, err, "invalid request");
        free(buffer);
        /* Protocol error - send error response and continue processing */
        return 0;
    }

    memset(&resp, 0, sizeof(resp));

    err = process_generate_request(g_sd_ctx, &req, &resp);
    if (err != ERR_NONE) {
        fprintf(stderr, "generation failed: %d\n", err);
        send_error_response(client_fd, req.request_id, err, "generation failed");
        free(buffer);
        /* Generation error - send error response and continue processing */
        return 0;
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
            free(buffer);
            /* Out of memory - fatal error, exit loop */
            return -1;
        }
        buffer = response_buf;

        err = encode_generate_response(&resp, buffer, response_buf_size, &response_len);
    }
    free_generate_response(&resp);

    if (err != ERR_NONE) {
        fprintf(stderr, "failed to encode response: %d\n", err);
        free(buffer);
        /* Encoding error - fatal error, exit loop */
        return -1;
    }

    if (write_full(client_fd, buffer, response_len) != 0) {
        /* Connection closed or I/O error - exit loop */
        free(buffer);
        return -1;
    }

    /* Success - request processed, connection still active */
    free(buffer);
    return 0;
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

    if (g_socket_fd >= 0) {
        close(g_socket_fd);
        g_socket_fd = -1;
    }

    /*
     * Only clean up socket file if we created it.
     * If we connected to a socket created by weave, weave owns the cleanup.
     */
    if (g_socket_owned) {
        socket_cleanup();
    }
}

int main(int argc, char *argv[]) {
    /* Variable declarations at top for C99 compliance */
    int exit_code = EXIT_FAILURE;
    socket_error_t err;
    const char *custom_socket_path = NULL;
    char socket_path[SOCKET_PATH_MAX];
    int opt;

    /* Long options for getopt_long */
    static struct option long_options[] = {
        {"socket-path", required_argument, 0, 's'},
        {"help",        no_argument,       0, 'h'},
        {0, 0, 0, 0}
    };

    /* Parse command line arguments */
    while ((opt = getopt_long(argc, argv, "hs:", long_options, NULL)) != -1) {
        switch (opt) {
        case 's':
            custom_socket_path = optarg;
            break;
        case 'h':
            print_usage(argv[0], 0);
            break;
        default:
            print_usage(argv[0], 1);
            break;
        }
    }

    /*
     * Validate custom socket path length if provided.
     */
    if (custom_socket_path != NULL) {
        size_t path_len = strlen(custom_socket_path);
        if (path_len == 0) {
            fprintf(stderr, "error: socket path cannot be empty\n");
            return EXIT_FAILURE;
        }
        if (path_len >= SOCKET_PATH_MAX) {
            fprintf(stderr, "error: socket path too long (max %d bytes)\n",
                    SOCKET_PATH_MAX - 1);
            return EXIT_FAILURE;
        }
    }

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

    /*
     * Socket initialization: connect to existing socket if path is provided,
     * otherwise create our own socket (backward compatibility).
     */
    if (custom_socket_path != NULL) {
        /* Connect to existing socket created by weave */
        err = socket_connect(custom_socket_path, &g_socket_fd);
        if (err != SOCKET_OK) {
            fprintf(stderr, "failed to connect to socket: %s\n", socket_error_string(err));
            fprintf(stderr, "ensure socket exists at: %s\n", custom_socket_path);
            cleanup();
            return EXIT_FAILURE;
        }
        g_socket_owned = 0; /* Socket owned by parent process */
        snprintf(socket_path, sizeof(socket_path), "%s", custom_socket_path);
        fprintf(stderr, "connected to socket: %s\n", socket_path);
    } else {
        /* Create and own the socket (backward compatibility) */
        err = socket_create(&g_socket_fd);
        if (err != SOCKET_OK) {
            fprintf(stderr, "failed to create socket: %s\n", socket_error_string(err));
            cleanup();
            return EXIT_FAILURE;
        }
        g_socket_owned = 1; /* We own this socket */

        /* Get socket path for logging */
        if (socket_get_path(socket_path, sizeof(socket_path)) != SOCKET_OK) {
            snprintf(socket_path, sizeof(socket_path), "(unknown)");
        }
        fprintf(stderr, "listening on %s\n", socket_path);
    }

    /*
     * Connection mode: client mode (connected to existing socket) or
     * server mode (created own socket and accepting connections).
     */
    if (custom_socket_path != NULL) {
        /*
         * Client mode: Run request/response loop on the connected socket.
         * The socket is already connected from socket_connect() above.
         * Keep processing requests until connection closes or shutdown.
         *
         * Architecture note: In client mode, compute connects once to weave's
         * listening socket and handles multiple requests over that single
         * persistent connection. This inverts the traditional client/server
         * roles - weave is the server (owns socket), compute is the client
         * (connects and processes work).
         *
         * handle_connection() processes ONE request and returns:
         * - 0: Request processed (success or protocol error), continue loop
         * - -1: Connection closed or fatal error, exit loop
         */

        /*
         * Start stdin monitoring thread for parent death detection.
         *
         * When weave (parent) dies, its stdout/stderr pipes close, which
         * causes compute's stdin to close. By monitoring stdin, we can detect
         * parent death even if the socket connection stays open.
         *
         * The thread is detached, so it cleans itself up automatically when
         * it exits. We don't need to join it during shutdown.
         *
         * Note: We always start the thread. If stdin is already closed, the
         * thread will immediately detect it and trigger shutdown. This is
         * simpler than trying to detect the closed state beforehand, and
         * correctly handles the edge case where the parent died during
         * our initialization.
         *
         * C99 compliance: All variables declared at top of this block.
         */
        pthread_attr_t attr;
        int thread_err;

        pthread_attr_init(&attr);
        pthread_attr_setdetachstate(&attr, PTHREAD_CREATE_DETACHED);

        thread_err = pthread_create(&g_stdin_monitor_thread, &attr,
                                   stdin_monitor_thread, NULL);
        pthread_attr_destroy(&attr);

        if (thread_err != 0) {
            fprintf(stderr, "warning: failed to start stdin monitor thread: %s\n",
                    strerror(thread_err));
            fprintf(stderr, "warning: parent death detection disabled\n");
            /*
             * Non-fatal error. Continue without stdin monitoring.
             * Socket closure will still trigger shutdown.
             */
            g_stdin_monitor_thread = 0;
        } else {
            fprintf(stderr, "stdin monitor thread started\n");
        }

        fprintf(stderr, "entering request/response loop\n");

        while (!socket_is_shutdown_requested()) {
            int result = handle_connection(g_socket_fd);
            if (result != 0) {
                /*
                 * Connection closed or fatal error. This is normal when:
                 * - weave shuts down and closes the connection
                 * - client disconnects
                 * - I/O error on socket
                 */
                break;
            }
            /* Request processed successfully, continue to next request */
        }

        if (socket_is_shutdown_requested()) {
            fprintf(stderr, "shutting down gracefully (signal received)\n");
        } else {
            fprintf(stderr, "shutting down gracefully (connection closed)\n");
        }
        exit_code = EXIT_SUCCESS;
    } else {
        /*
         * Server mode: Accept connections from clients (backward compatibility).
         * This mode is used when compute creates and owns the socket.
         */
        err = socket_accept_loop(g_socket_fd, handle_connection);
        if (err == SOCKET_OK) {
            fprintf(stderr, "shutting down gracefully\n");
            exit_code = EXIT_SUCCESS;
        } else {
            fprintf(stderr, "accept loop failed: %s\n", socket_error_string(err));
        }
    }

    cleanup();

    fprintf(stderr, "weave-compute stopped\n");
    return exit_code;
}
