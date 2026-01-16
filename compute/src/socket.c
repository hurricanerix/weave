/**
 * Weave Socket Module - Unix Domain Socket Implementation
 *
 * This file implements Unix domain socket creation, management, and
 * authentication for weave-compute.
 *
 * Security considerations:
 * - Socket directory: mode 0700 (owner only)
 * - Socket file: mode 0600 (owner read/write only)
 * - Stale socket detection prevents startup failures
 * - No world-readable or group-readable permissions
 * - SO_PEERCRED authentication verifies connecting process UID
 *
 * The socket path is constructed from:
 *   $XDG_RUNTIME_DIR/weave/weave.sock
 *
 * XDG_RUNTIME_DIR is typically /run/user/<uid> on modern Linux systems.
 */

/* Enable GNU extensions for SO_PEERCRED and struct ucred */
#define _GNU_SOURCE

#include <errno.h>
#include <signal.h>
#include <stdarg.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/socket.h>
#include <sys/stat.h>
#include <sys/un.h>
#include <unistd.h>

#include "weave/socket.h"

/**
 * Listen backlog for the socket.
 * Since we process requests serially for MVP, a small backlog is sufficient.
 */
#define LISTEN_BACKLOG 5

/**
 * Global socket path storage.
 * Set during socket_create() for use by socket_cleanup().
 *
 * Note: This global state is acceptable for MVP since weave-compute runs as a
 * single-threaded process with one listening socket. NOT thread-safe.
 * For concurrent operation, refactor to pass socket_context_t explicitly.
 */
static char g_socket_path[SOCKET_PATH_MAX] = {0};

/**
 * Shutdown flag for graceful termination.
 * Using sig_atomic_t for async-signal-safety. The volatile qualifier
 * ensures the compiler doesn't optimize away reads in the accept loop.
 */
static volatile sig_atomic_t g_shutdown_requested = 0;

/**
 * Default timeouts for client connections.
 */
#define DEFAULT_READ_TIMEOUT_S 60
#define DEFAULT_WRITE_TIMEOUT_S 5

/**
 * Logging infrastructure.
 * Default log level is INFO, which means DEBUG messages are not shown.
 * This ensures auth rejections are silent unless explicitly enabled.
 */
static socket_log_level_t g_log_level = SOCKET_LOG_INFO;
static socket_log_callback_t g_log_callback = NULL;

/**
 * socket_log - Internal logging function
 *
 * @param level    Log level of this message
 * @param format   printf-style format string
 * @param ...      Format arguments
 */
static void socket_log(socket_log_level_t level, const char *format, ...) {
    if (level < g_log_level) {
        return;
    }

    char message[512];
    va_list args;
    va_start(args, format);
    vsnprintf(message, sizeof(message), format, args);
    va_end(args);

    if (g_log_callback != NULL) {
        g_log_callback(level, message);
    } else {
        const char *level_str;
        switch (level) {
            case SOCKET_LOG_DEBUG: level_str = "DEBUG"; break;
            case SOCKET_LOG_INFO:  level_str = "INFO";  break;
            case SOCKET_LOG_WARN:  level_str = "WARN";  break;
            case SOCKET_LOG_ERROR: level_str = "ERROR"; break;
            default:               level_str = "???";   break;
        }
        fprintf(stderr, "[socket] %s: %s\n", level_str, message);
    }
}

/**
 * socket_set_log_level - Set the minimum log level
 */
void socket_set_log_level(socket_log_level_t level) {
    g_log_level = level;
}

/**
 * socket_set_log_callback - Set custom log handler
 */
void socket_set_log_callback(socket_log_callback_t callback) {
    g_log_callback = callback;
}

/**
 * socket_get_path - Get the full socket path
 */
socket_error_t socket_get_path(char *path_buf, size_t buf_size) {
    if (path_buf == NULL) {
        return SOCKET_ERR_NULL_POINTER;
    }

    const char *xdg_runtime_dir = getenv("XDG_RUNTIME_DIR");
    if (xdg_runtime_dir == NULL || xdg_runtime_dir[0] == '\0') {
        return SOCKET_ERR_XDG_NOT_SET;
    }

    /* Calculate required length: XDG_RUNTIME_DIR + "/" + SOCKET_DIR_NAME + "/" + SOCKET_FILE_NAME + null */
    size_t xdg_len = strlen(xdg_runtime_dir);
    size_t dir_len = strlen(SOCKET_DIR_NAME);
    size_t file_len = strlen(SOCKET_FILE_NAME);
    size_t total_len = xdg_len + 1 + dir_len + 1 + file_len + 1;

    if (total_len > buf_size || total_len > SOCKET_PATH_MAX) {
        return SOCKET_ERR_PATH_TOO_LONG;
    }

    /* Construct path */
    int written = snprintf(path_buf, buf_size, "%s/%s/%s",
                           xdg_runtime_dir, SOCKET_DIR_NAME, SOCKET_FILE_NAME);
    if (written < 0 || (size_t)written >= buf_size) {
        return SOCKET_ERR_PATH_TOO_LONG;
    }

    return SOCKET_OK;
}

/**
 * socket_get_dir_path - Get the socket directory path
 */
socket_error_t socket_get_dir_path(char *path_buf, size_t buf_size) {
    if (path_buf == NULL) {
        return SOCKET_ERR_NULL_POINTER;
    }

    const char *xdg_runtime_dir = getenv("XDG_RUNTIME_DIR");
    if (xdg_runtime_dir == NULL || xdg_runtime_dir[0] == '\0') {
        return SOCKET_ERR_XDG_NOT_SET;
    }

    /* Calculate required length: XDG_RUNTIME_DIR + "/" + SOCKET_DIR_NAME + null */
    size_t xdg_len = strlen(xdg_runtime_dir);
    size_t dir_len = strlen(SOCKET_DIR_NAME);
    size_t total_len = xdg_len + 1 + dir_len + 1;

    if (total_len > buf_size) {
        return SOCKET_ERR_PATH_TOO_LONG;
    }

    int written = snprintf(path_buf, buf_size, "%s/%s",
                           xdg_runtime_dir, SOCKET_DIR_NAME);
    if (written < 0 || (size_t)written >= buf_size) {
        return SOCKET_ERR_PATH_TOO_LONG;
    }

    return SOCKET_OK;
}

/**
 * create_socket_directory - Create the socket directory with correct permissions
 *
 * @param dir_path  Path to create
 * @return          SOCKET_OK on success or if directory exists, error code on failure
 */
static socket_error_t create_socket_directory(const char *dir_path) {
    struct stat st;

    /* Check if directory already exists */
    if (stat(dir_path, &st) == 0) {
        if (S_ISDIR(st.st_mode)) {
            /* Directory exists, verify permissions */
            if ((st.st_mode & 0777) != 0700) {
                /* Fix permissions */
                if (chmod(dir_path, 0700) != 0) {
                    return SOCKET_ERR_CHMOD_FAILED;
                }
            }
            return SOCKET_OK;
        }
        /* Path exists but is not a directory */
        return SOCKET_ERR_MKDIR_FAILED;
    }

    /* Create directory with mode 0700 */
    if (mkdir(dir_path, 0700) != 0) {
        if (errno == EEXIST) {
            /* Race condition: directory was created between stat and mkdir */
            return SOCKET_OK;
        }
        return SOCKET_ERR_MKDIR_FAILED;
    }

    return SOCKET_OK;
}

/**
 * is_socket_stale - Check if an existing socket file is stale
 *
 * A socket is considered stale if we cannot connect to it.
 *
 * @param socket_path  Path to the socket file
 * @return             1 if stale, 0 if active or not a socket
 */
static int is_socket_stale(const char *socket_path) {
    struct stat st;

    /* Check if file exists and is a socket */
    if (stat(socket_path, &st) != 0) {
        return 0; /* File doesn't exist */
    }

    if (!S_ISSOCK(st.st_mode)) {
        return 0; /* Not a socket */
    }

    /* Try to connect to determine if socket is active */
    int test_fd = socket(AF_UNIX, SOCK_STREAM, 0);
    if (test_fd < 0) {
        return 0; /* Can't create test socket */
    }

    /* Verify path fits in sockaddr_un before copying */
    size_t path_len = strlen(socket_path);
    if (path_len >= sizeof(((struct sockaddr_un *)0)->sun_path)) {
        close(test_fd);
        return 0; /* Path too long - treat as not stale */
    }

    struct sockaddr_un addr;
    memset(&addr, 0, sizeof(addr)); /* Ensures null termination */
    addr.sun_family = AF_UNIX;
    memcpy(addr.sun_path, socket_path, path_len + 1); /* Include null terminator */

    int result = connect(test_fd, (struct sockaddr *)&addr, sizeof(addr));
    close(test_fd);

    if (result == 0) {
        /* Connection succeeded - socket is active (another process is running) */
        return 0;
    }

    /* Connection failed - socket is stale */
    return 1;
}

/**
 * remove_stale_socket - Remove a stale socket file
 *
 * @param socket_path  Path to the socket file
 * @return             SOCKET_OK on success, error code on failure
 */
static socket_error_t remove_stale_socket(const char *socket_path) {
    if (unlink(socket_path) != 0 && errno != ENOENT) {
        return SOCKET_ERR_UNLINK_FAILED;
    }
    return SOCKET_OK;
}

/**
 * create_and_bind_socket - Create socket and bind to address
 *
 * Helper function that creates a socket and binds it to the given address.
 * On success, returns the socket file descriptor. On failure, returns -1.
 *
 * @param addr      Pointer to sockaddr_un structure with address
 * @return          Socket file descriptor on success, -1 on failure
 */
static int create_and_bind_socket(const struct sockaddr_un *addr) {
    int sock_fd = socket(AF_UNIX, SOCK_STREAM, 0);
    if (sock_fd < 0) {
        return -1;
    }

    if (bind(sock_fd, (const struct sockaddr *)addr, sizeof(*addr)) != 0) {
        close(sock_fd);
        return -1;
    }

    return sock_fd;
}

/**
 * socket_create - Create and bind a Unix domain socket
 */
socket_error_t socket_create(int *listen_fd) {
    if (listen_fd == NULL) {
        return SOCKET_ERR_NULL_POINTER;
    }

    *listen_fd = -1;

    /* Get socket path */
    char socket_path[SOCKET_PATH_MAX];
    socket_error_t err = socket_get_path(socket_path, sizeof(socket_path));
    if (err != SOCKET_OK) {
        return err;
    }

    /* Verify path fits in sockaddr_un */
    size_t path_len = strlen(socket_path);
    if (path_len >= sizeof(((struct sockaddr_un *)0)->sun_path)) {
        return SOCKET_ERR_PATH_TOO_LONG;
    }

    /* Get and create socket directory */
    char dir_path[SOCKET_PATH_MAX];
    err = socket_get_dir_path(dir_path, sizeof(dir_path));
    if (err != SOCKET_OK) {
        return err;
    }

    err = create_socket_directory(dir_path);
    if (err != SOCKET_OK) {
        return err;
    }

    /* Check for and remove stale socket */
    if (is_socket_stale(socket_path)) {
        err = remove_stale_socket(socket_path);
        if (err != SOCKET_OK) {
            return SOCKET_ERR_BIND_FAILED;
        }
    }

    /* Prepare address structure */
    struct sockaddr_un addr;
    memset(&addr, 0, sizeof(addr)); /* Ensures null termination */
    addr.sun_family = AF_UNIX;
    memcpy(addr.sun_path, socket_path, path_len + 1); /* Include null terminator */

    /* Create and bind socket */
    int sock_fd = create_and_bind_socket(&addr);
    if (sock_fd < 0) {
        /* If bind failed because socket exists, try to remove and rebind */
        if (errno == EADDRINUSE && is_socket_stale(socket_path)) {
            if (remove_stale_socket(socket_path) == SOCKET_OK) {
                sock_fd = create_and_bind_socket(&addr);
            }
        }

        if (sock_fd < 0) {
            return SOCKET_ERR_BIND_FAILED;
        }
    }

    /* Set socket file permissions to 0600 */
    if (chmod(socket_path, 0600) != 0) {
        close(sock_fd);
        unlink(socket_path);
        return SOCKET_ERR_CHMOD_FAILED;
    }

    /* Start listening */
    if (listen(sock_fd, LISTEN_BACKLOG) != 0) {
        close(sock_fd);
        unlink(socket_path);
        return SOCKET_ERR_LISTEN_FAILED;
    }

    /* Store socket path for cleanup */
    memcpy(g_socket_path, socket_path, path_len + 1);

    *listen_fd = sock_fd;
    return SOCKET_OK;
}

/**
 * socket_cleanup - Remove socket file
 */
socket_error_t socket_cleanup(void) {
    if (g_socket_path[0] == '\0') {
        /* No socket path stored, try to get it */
        char socket_path[SOCKET_PATH_MAX];
        socket_error_t err = socket_get_path(socket_path, sizeof(socket_path));
        if (err != SOCKET_OK) {
            /* Can't determine path - assume nothing to clean */
            return SOCKET_OK;
        }

        if (unlink(socket_path) != 0 && errno != ENOENT) {
            return SOCKET_ERR_UNLINK_FAILED;
        }
        return SOCKET_OK;
    }

    if (unlink(g_socket_path) != 0 && errno != ENOENT) {
        return SOCKET_ERR_UNLINK_FAILED;
    }

    /* Clear stored path */
    g_socket_path[0] = '\0';

    return SOCKET_OK;
}

/**
 * socket_auth_connection - Authenticate a client connection via SO_PEERCRED
 *
 * This function retrieves the peer credentials of a connected Unix domain
 * socket and verifies that the connecting process has the same UID as
 * weave-compute. This provides kernel-verified authentication that cannot be forged.
 *
 * The function should be called immediately after accept() and before reading
 * any data from the client. On authentication failure, the caller should close
 * the socket without sending any response.
 */
socket_error_t socket_auth_connection(int client_fd) {
    if (client_fd < 0) {
        return SOCKET_ERR_INVALID_FD;
    }

    /* Get peer credentials using SO_PEERCRED */
    struct ucred cred;
    socklen_t cred_len = sizeof(cred);

    if (getsockopt(client_fd, SOL_SOCKET, SO_PEERCRED, &cred, &cred_len) != 0) {
        socket_log(SOCKET_LOG_DEBUG,
                   "auth failed: getsockopt(SO_PEERCRED) failed: %s",
                   strerror(errno));
        return SOCKET_ERR_AUTH_FAILED;
    }

    /* Compare client UID with process UID */
    uid_t process_uid = getuid();

    if (cred.uid != process_uid) {
        socket_log(SOCKET_LOG_DEBUG,
                   "auth rejected: client uid=%u pid=%u (expected uid=%u)",
                   (unsigned int)cred.uid,
                   (unsigned int)cred.pid,
                   (unsigned int)process_uid);
        return SOCKET_ERR_AUTH_UID_MISMATCH;
    }

    /* Authentication successful - log at debug level */
    socket_log(SOCKET_LOG_DEBUG,
               "auth accepted: client uid=%u pid=%u",
               (unsigned int)cred.uid,
               (unsigned int)cred.pid);

    return SOCKET_OK;
}

/**
 * socket_error_string - Get human-readable error message
 */
const char *socket_error_string(socket_error_t err) {
    switch (err) {
        case SOCKET_OK:
            return "success";
        case SOCKET_ERR_XDG_NOT_SET:
            return "XDG_RUNTIME_DIR not set";
        case SOCKET_ERR_PATH_TOO_LONG:
            return "socket path too long";
        case SOCKET_ERR_MKDIR_FAILED:
            return "failed to create socket directory";
        case SOCKET_ERR_SOCKET_FAILED:
            return "failed to create socket";
        case SOCKET_ERR_BIND_FAILED:
            return "failed to bind socket";
        case SOCKET_ERR_LISTEN_FAILED:
            return "failed to listen on socket";
        case SOCKET_ERR_CHMOD_FAILED:
            return "failed to set socket permissions";
        case SOCKET_ERR_UNLINK_FAILED:
            return "failed to remove socket file";
        case SOCKET_ERR_NULL_POINTER:
            return "null pointer argument";
        case SOCKET_ERR_STALE_SOCKET:
            return "stale socket removed";
        case SOCKET_ERR_AUTH_FAILED:
            return "authentication failed (could not get peer credentials)";
        case SOCKET_ERR_AUTH_UID_MISMATCH:
            return "authentication failed (UID mismatch)";
        case SOCKET_ERR_INVALID_FD:
            return "invalid file descriptor";
        case SOCKET_ERR_TIMEOUT_FAILED:
            return "failed to set socket timeout";
        case SOCKET_ERR_ACCEPT_FAILED:
            return "failed to accept connection";
        case SOCKET_ERR_NULL_HANDLER:
            return "null handler provided to accept loop";
        case SOCKET_ERR_CONNECT_FAILED:
            return "failed to connect to socket";
        default:
            return "unknown error";
    }
}

/**
 * socket_set_timeouts - Set read and write timeouts on a socket
 */
socket_error_t socket_set_timeouts(int fd, int read_timeout_s, int write_timeout_s) {
    if (fd < 0) {
        return SOCKET_ERR_INVALID_FD;
    }

    struct timeval tv;

    /* Set read timeout */
    if (read_timeout_s > 0) {
        tv.tv_sec = read_timeout_s;
        tv.tv_usec = 0;
        if (setsockopt(fd, SOL_SOCKET, SO_RCVTIMEO, &tv, sizeof(tv)) != 0) {
            socket_log(SOCKET_LOG_ERROR, "setsockopt(SO_RCVTIMEO) failed: %s",
                       strerror(errno));
            return SOCKET_ERR_TIMEOUT_FAILED;
        }
    }

    /* Set write timeout */
    if (write_timeout_s > 0) {
        tv.tv_sec = write_timeout_s;
        tv.tv_usec = 0;
        if (setsockopt(fd, SOL_SOCKET, SO_SNDTIMEO, &tv, sizeof(tv)) != 0) {
            socket_log(SOCKET_LOG_ERROR, "setsockopt(SO_SNDTIMEO) failed: %s",
                       strerror(errno));
            return SOCKET_ERR_TIMEOUT_FAILED;
        }
    }

    return SOCKET_OK;
}

/**
 * socket_request_shutdown - Request graceful shutdown of accept loop
 */
void socket_request_shutdown(void) {
    g_shutdown_requested = 1;
}

/**
 * socket_is_shutdown_requested - Check if shutdown was requested
 */
int socket_is_shutdown_requested(void) {
    return g_shutdown_requested != 0;
}

/**
 * socket_reset_shutdown - Reset shutdown flag (for testing)
 *
 * This is an internal function used to reset state between tests.
 * Not exposed in the public header.
 */
void socket_reset_shutdown(void) {
    g_shutdown_requested = 0;
}

/**
 * socket_connect - Connect to an existing Unix domain socket
 *
 * Connects to an existing Unix domain socket at the specified path.
 * This function is used by weave-compute when running in "worker mode" where
 * the socket is created by the parent process (weave) rather than by
 * weave-compute itself.
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
 */
socket_error_t socket_connect(const char *socket_path, int *connected_fd) {
    if (socket_path == NULL || connected_fd == NULL) {
        return SOCKET_ERR_NULL_POINTER;
    }

    *connected_fd = -1;

    /* Verify path fits in sockaddr_un */
    size_t path_len = strlen(socket_path);
    if (path_len >= sizeof(((struct sockaddr_un *)0)->sun_path)) {
        socket_log(SOCKET_LOG_ERROR, "socket path too long: %zu bytes (max %zu)",
                   path_len, sizeof(((struct sockaddr_un *)0)->sun_path) - 1);
        return SOCKET_ERR_PATH_TOO_LONG;
    }

    /* Create socket */
    int sock_fd = socket(AF_UNIX, SOCK_STREAM, 0);
    if (sock_fd < 0) {
        socket_log(SOCKET_LOG_ERROR, "socket() failed: %s", strerror(errno));
        return SOCKET_ERR_SOCKET_FAILED;
    }

    /* Prepare address structure */
    struct sockaddr_un addr;
    memset(&addr, 0, sizeof(addr)); /* Ensures null termination */
    addr.sun_family = AF_UNIX;
    memcpy(addr.sun_path, socket_path, path_len + 1); /* Include null terminator */

    /* Connect to the socket */
    if (connect(sock_fd, (const struct sockaddr *)&addr, sizeof(addr)) != 0) {
        socket_log(SOCKET_LOG_ERROR, "connect() failed for %s: %s",
                   socket_path, strerror(errno));
        close(sock_fd);
        return SOCKET_ERR_CONNECT_FAILED;
    }

    socket_log(SOCKET_LOG_INFO, "connected to socket: %s", socket_path);

    *connected_fd = sock_fd;
    return SOCKET_OK;
}

/**
 * socket_accept_loop - Main accept loop for weave-compute
 */
socket_error_t socket_accept_loop(int listen_fd, socket_connection_handler_t handler) {
    if (listen_fd < 0) {
        return SOCKET_ERR_INVALID_FD;
    }

    if (handler == NULL) {
        return SOCKET_ERR_NULL_HANDLER;
    }

    socket_log(SOCKET_LOG_INFO, "accept loop started");

    while (!g_shutdown_requested) {
        /* Accept a connection */
        int client_fd = accept(listen_fd, NULL, NULL);
        if (client_fd < 0) {
            if (errno == EINTR) {
                /* Interrupted by signal, check shutdown flag and continue */
                continue;
            }
            socket_log(SOCKET_LOG_ERROR, "accept() failed: %s", strerror(errno));
            return SOCKET_ERR_ACCEPT_FAILED;
        }

        /* Authenticate the client */
        socket_error_t auth_err = socket_auth_connection(client_fd);
        if (auth_err != SOCKET_OK) {
            /* Auth failure is logged at DEBUG level by socket_auth_connection */
            close(client_fd);
            continue;
        }

        /* Set timeouts on the client socket */
        socket_error_t timeout_err = socket_set_timeouts(client_fd,
                                                          DEFAULT_READ_TIMEOUT_S,
                                                          DEFAULT_WRITE_TIMEOUT_S);
        if (timeout_err != SOCKET_OK) {
            socket_log(SOCKET_LOG_WARN, "failed to set client timeouts, continuing anyway");
            /* Don't fail - timeouts are nice-to-have, not critical */
        }

        /* Process the connection */
        socket_log(SOCKET_LOG_DEBUG, "handling connection");
        int handler_result = handler(client_fd);
        if (handler_result != 0) {
            socket_log(SOCKET_LOG_WARN, "handler returned error: %d", handler_result);
            /* Don't stop the loop - just log and continue */
        }

        /* Close the client connection */
        close(client_fd);
    }

    socket_log(SOCKET_LOG_INFO, "accept loop stopped (shutdown requested)");
    return SOCKET_OK;
}
