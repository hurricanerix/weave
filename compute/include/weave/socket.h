/**
 * Weave Socket Module - Unix Domain Socket Management
 *
 * This module handles Unix domain socket creation, cleanup, lifecycle
 * management, and authentication for weave-compute. It provides
 * secure socket creation at $XDG_RUNTIME_DIR/weave/weave.sock with
 * appropriate permissions and SO_PEERCRED-based authentication.
 *
 * Security:
 * - Socket directory created with mode 0700 (owner only)
 * - Socket file created with mode 0600 (owner read/write only)
 * - Stale socket detection prevents denial of service
 * - SO_PEERCRED authentication verifies connecting process UID
 *
 * Usage:
 * 1. Call socket_get_path() to get the socket path
 * 2. Call socket_create() to create the listening socket
 * 3. Accept connections with accept()
 * 4. Call socket_auth_connection() to verify client UID
 * 5. Call socket_cleanup() on shutdown to remove socket file
 */

#pragma once

#include <stddef.h>

/**
 * Socket Error Codes
 *
 * These error codes are specific to socket operations and are separate
 * from protocol error codes.
 */
typedef enum {
    SOCKET_OK = 0,                    /**< Success */
    SOCKET_ERR_XDG_NOT_SET = -1,      /**< XDG_RUNTIME_DIR not set */
    SOCKET_ERR_PATH_TOO_LONG = -2,    /**< Socket path exceeds system limit */
    SOCKET_ERR_MKDIR_FAILED = -3,     /**< Failed to create socket directory */
    SOCKET_ERR_SOCKET_FAILED = -4,    /**< Failed to create socket */
    SOCKET_ERR_BIND_FAILED = -5,      /**< Failed to bind socket */
    SOCKET_ERR_LISTEN_FAILED = -6,    /**< Failed to listen on socket */
    SOCKET_ERR_CHMOD_FAILED = -7,     /**< Failed to set socket permissions */
    SOCKET_ERR_UNLINK_FAILED = -8,    /**< Failed to remove socket file */
    SOCKET_ERR_NULL_POINTER = -9,     /**< NULL pointer argument */
    SOCKET_ERR_STALE_SOCKET = -10,    /**< Stale socket removed (not an error, informational) */
    SOCKET_ERR_AUTH_FAILED = -11,     /**< SO_PEERCRED authentication failed */
    SOCKET_ERR_AUTH_UID_MISMATCH = -12, /**< Client UID does not match process UID */
    SOCKET_ERR_INVALID_FD = -13,      /**< Invalid file descriptor */
    SOCKET_ERR_TIMEOUT_FAILED = -14,  /**< Failed to set socket timeout */
    SOCKET_ERR_ACCEPT_FAILED = -15,   /**< Failed to accept connection */
    SOCKET_ERR_NULL_HANDLER = -16,    /**< NULL handler provided to accept loop */
    SOCKET_ERR_CONNECT_FAILED = -17,  /**< Failed to connect to socket */
} socket_error_t;

/**
 * Maximum length of socket path including null terminator.
 * Unix domain sockets have a limit of 108 bytes on Linux.
 */
#define SOCKET_PATH_MAX 108

/**
 * Socket directory name relative to XDG_RUNTIME_DIR.
 */
#define SOCKET_DIR_NAME "weave"

/**
 * Socket filename.
 */
#define SOCKET_FILE_NAME "weave.sock"

/**
 * socket_get_path - Get the full socket path
 *
 * Constructs the socket path from $XDG_RUNTIME_DIR environment variable.
 * The resulting path is: $XDG_RUNTIME_DIR/weave/weave.sock
 *
 * @param path_buf   Output buffer for socket path (must be at least SOCKET_PATH_MAX)
 * @param buf_size   Size of output buffer
 * @return           SOCKET_OK on success, error code on failure
 *
 * Error codes:
 * - SOCKET_ERR_NULL_POINTER: path_buf is NULL
 * - SOCKET_ERR_XDG_NOT_SET: XDG_RUNTIME_DIR environment variable not set
 * - SOCKET_ERR_PATH_TOO_LONG: Resulting path exceeds buf_size or SOCKET_PATH_MAX
 */
socket_error_t socket_get_path(char *path_buf, size_t buf_size);

/**
 * socket_get_dir_path - Get the socket directory path
 *
 * Constructs the socket directory path from $XDG_RUNTIME_DIR.
 * The resulting path is: $XDG_RUNTIME_DIR/weave
 *
 * @param path_buf   Output buffer for directory path
 * @param buf_size   Size of output buffer
 * @return           SOCKET_OK on success, error code on failure
 *
 * Error codes:
 * - SOCKET_ERR_NULL_POINTER: path_buf is NULL
 * - SOCKET_ERR_XDG_NOT_SET: XDG_RUNTIME_DIR environment variable not set
 * - SOCKET_ERR_PATH_TOO_LONG: Resulting path exceeds buf_size
 */
socket_error_t socket_get_dir_path(char *path_buf, size_t buf_size);

/**
 * socket_create - Create and bind a Unix domain socket
 *
 * Creates a listening Unix domain socket at the standard path.
 * This function:
 * 1. Gets the socket path from $XDG_RUNTIME_DIR
 * 2. Creates the socket directory with mode 0700 if needed
 * 3. Checks for and removes stale socket files
 * 4. Creates and binds the socket
 * 5. Sets socket file permissions to 0600
 * 6. Starts listening for connections
 *
 * @param listen_fd  Pointer to store the listening socket file descriptor
 * @return           SOCKET_OK on success, error code on failure
 *
 * Error codes:
 * - SOCKET_ERR_NULL_POINTER: listen_fd is NULL
 * - SOCKET_ERR_XDG_NOT_SET: XDG_RUNTIME_DIR not set
 * - SOCKET_ERR_PATH_TOO_LONG: Socket path too long
 * - SOCKET_ERR_MKDIR_FAILED: Could not create socket directory
 * - SOCKET_ERR_SOCKET_FAILED: Could not create socket
 * - SOCKET_ERR_BIND_FAILED: Could not bind socket (and stale socket removal failed)
 * - SOCKET_ERR_LISTEN_FAILED: Could not listen on socket
 * - SOCKET_ERR_CHMOD_FAILED: Could not set socket permissions
 *
 * On success, the caller owns the socket and must:
 * - Call socket_cleanup() when done
 * - Close the socket file descriptor
 */
socket_error_t socket_create(int *listen_fd);

/**
 * socket_connect - Connect to an existing Unix domain socket
 *
 * Connects to an existing Unix domain socket at the specified path.
 * This function is used by weave-compute when running in "worker mode" where
 * the socket is created by the parent process (weave) rather than by
 * weave-compute itself.
 *
 * Unlike socket_create(), this function does not create or bind a socket.
 * It only connects to an already-listening socket. This is the client-side
 * connection logic for weave-compute to use when spawned by weave.
 *
 * @param socket_path  Path to the existing socket file
 * @param connected_fd Pointer to store the connected socket file descriptor
 * @return             SOCKET_OK on success, error code on failure
 *
 * Error codes:
 * - SOCKET_ERR_NULL_POINTER: socket_path or connected_fd is NULL
 * - SOCKET_ERR_PATH_TOO_LONG: Socket path exceeds system limit
 * - SOCKET_ERR_SOCKET_FAILED: Could not create socket
 * - SOCKET_ERR_CONNECT_FAILED: Could not connect to socket
 *
 * On success, the caller owns the socket and must close it when done.
 * Do NOT call socket_cleanup() when using socket_connect() - the parent
 * process owns the socket file.
 */
socket_error_t socket_connect(const char *socket_path, int *connected_fd);

/**
 * socket_cleanup - Remove socket file
 *
 * Removes the socket file from the filesystem. Should be called during
 * graceful shutdown to allow weave-compute to restart cleanly.
 *
 * This function does NOT close the socket file descriptor - the caller
 * must do that separately.
 *
 * @return  SOCKET_OK on success or if socket doesn't exist,
 *          SOCKET_ERR_UNLINK_FAILED on failure
 *
 * Note: This function is safe to call even if the socket was never created
 * or has already been cleaned up.
 */
socket_error_t socket_cleanup(void);

/**
 * socket_auth_connection - Authenticate a client connection via SO_PEERCRED
 *
 * Verifies that the connecting process has the same UID as weave-compute.
 * This function should be called immediately after accept() and before
 * reading any data from the client socket.
 *
 * On authentication failure:
 * - The client_fd is NOT closed (caller must handle this)
 * - Rejection is logged at DEBUG level with client UID/PID
 * - No data is sent to the client
 *
 * @param client_fd  Connected client socket file descriptor
 * @return           SOCKET_OK if client is authorized, error code otherwise
 *
 * Error codes:
 * - SOCKET_ERR_INVALID_FD: client_fd is negative
 * - SOCKET_ERR_AUTH_FAILED: Could not retrieve peer credentials
 * - SOCKET_ERR_AUTH_UID_MISMATCH: Client UID does not match process UID
 *
 * Security:
 * - Uses SO_PEERCRED which is kernel-verified and unforgeable
 * - Only same-UID processes are authorized (userland mode)
 */
socket_error_t socket_auth_connection(int client_fd);

/**
 * Log levels for socket module.
 * Only DEBUG level logs authentication rejections.
 */
typedef enum {
    SOCKET_LOG_DEBUG = 0,   /**< Debug messages (auth rejections) */
    SOCKET_LOG_INFO = 1,    /**< Informational messages */
    SOCKET_LOG_WARN = 2,    /**< Warning messages */
    SOCKET_LOG_ERROR = 3,   /**< Error messages */
    SOCKET_LOG_NONE = 4,    /**< Disable all logging */
} socket_log_level_t;

/**
 * socket_set_log_level - Set the minimum log level for socket module
 *
 * Messages below this level will not be logged. Default is SOCKET_LOG_INFO,
 * which means DEBUG messages (including auth rejections) are not shown.
 *
 * @param level  Minimum log level to display
 */
void socket_set_log_level(socket_log_level_t level);

/**
 * socket_set_log_callback - Set custom log handler
 *
 * By default, logs go to stderr. Use this to redirect logs elsewhere.
 * Pass NULL to restore default stderr logging.
 *
 * @param callback  Function to call for each log message, or NULL for default
 */
typedef void (*socket_log_callback_t)(socket_log_level_t level,
                                      const char *message);
void socket_set_log_callback(socket_log_callback_t callback);

/**
 * socket_error_string - Get human-readable error message
 *
 * Returns a static string describing the error code.
 *
 * @param err   Error code from socket functions
 * @return      Human-readable error message (never NULL)
 */
const char *socket_error_string(socket_error_t err);

/**
 * Connection handler callback type.
 *
 * Called for each authenticated client connection. The handler should
 * read the request, process it, and write the response.
 *
 * @param client_fd  Connected and authenticated client socket
 * @return           0 on success, non-zero on error
 *
 * Note: The handler should NOT close client_fd - the accept loop handles this.
 */
typedef int (*socket_connection_handler_t)(int client_fd);

/**
 * socket_set_timeouts - Set read and write timeouts on a socket
 *
 * Sets SO_RCVTIMEO and SO_SNDTIMEO socket options.
 *
 * @param fd              Socket file descriptor
 * @param read_timeout_s  Read timeout in seconds (0 = no timeout)
 * @param write_timeout_s Write timeout in seconds (0 = no timeout)
 * @return                SOCKET_OK on success, error code on failure
 */
socket_error_t socket_set_timeouts(int fd, int read_timeout_s, int write_timeout_s);

/**
 * socket_request_shutdown - Request graceful shutdown of accept loop
 *
 * This function is async-signal-safe and can be called from signal handlers.
 * After calling, socket_accept_loop() will exit after completing the current
 * connection (if any).
 */
void socket_request_shutdown(void);

/**
 * socket_is_shutdown_requested - Check if shutdown was requested
 *
 * @return  Non-zero if shutdown was requested, 0 otherwise
 */
int socket_is_shutdown_requested(void);

/**
 * socket_accept_loop - Main accept loop for weave-compute
 *
 * Accepts connections on the listening socket, authenticates each client
 * via SO_PEERCRED, sets timeouts, and calls the handler for each connection.
 *
 * The loop continues until socket_request_shutdown() is called or an
 * unrecoverable error occurs.
 *
 * @param listen_fd  Listening socket from socket_create()
 * @param handler    Callback function to handle each connection
 * @return           SOCKET_OK on graceful shutdown, error code on failure
 *
 * Behavior:
 * - Each connection is processed serially (one at a time)
 * - Read timeout: 60 seconds
 * - Write timeout: 5 seconds
 * - Unauthenticated connections are closed silently
 * - Handler errors are logged but don't stop the loop
 * - EINTR during accept() is handled (continues loop)
 */
socket_error_t socket_accept_loop(int listen_fd, socket_connection_handler_t handler);
