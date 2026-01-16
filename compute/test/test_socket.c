/**
 * Weave Socket Module - Unit Tests
 *
 * Tests for Unix domain socket creation, cleanup, and error handling.
 *
 * Test categories:
 * - Path construction tests
 * - Directory creation tests
 * - Socket creation tests
 * - Stale socket handling tests
 * - Cleanup tests
 * - Error condition tests
 *
 * Note: Some tests require specific environment setup (XDG_RUNTIME_DIR).
 * Tests that modify the filesystem use a temporary directory.
 */

/* Enable POSIX and BSD features for various functions */
#define _DEFAULT_SOURCE
#define _POSIX_C_SOURCE 200809L

#include <errno.h>
#include <fcntl.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/socket.h>
#include <sys/stat.h>
#include <sys/time.h>
#include <sys/un.h>
#include <sys/wait.h>
#include <unistd.h>

#include "weave/socket.h"

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

#define ASSERT_NE(not_expected, actual) \
    do { \
        if ((not_expected) == (actual)) { \
            printf("  FAIL: Did not expect %d at line %d\n", \
                   (int)(not_expected), __LINE__); \
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

#define ASSERT_STR_EQ(expected, actual) \
    do { \
        if (strcmp((expected), (actual)) != 0) { \
            printf("  FAIL: Expected '%s', got '%s' at line %d\n", \
                   (expected), (actual), __LINE__); \
            return; \
        } \
    } while(0)

#define ASSERT_STR_CONTAINS(haystack, needle) \
    do { \
        if (strstr((haystack), (needle)) == NULL) { \
            printf("  FAIL: '%s' does not contain '%s' at line %d\n", \
                   (haystack), (needle), __LINE__); \
            return; \
        } \
    } while(0)

#define TEST_PASS() \
    do { \
        tests_passed++; \
        printf("  PASS\n"); \
    } while(0)

/**
 * Helper: Save and restore XDG_RUNTIME_DIR
 */
static char *saved_xdg_runtime_dir = NULL;

static void save_xdg_runtime_dir(void) {
    const char *val = getenv("XDG_RUNTIME_DIR");
    if (val != NULL) {
        saved_xdg_runtime_dir = strdup(val);
    } else {
        saved_xdg_runtime_dir = NULL;
    }
}

static void restore_xdg_runtime_dir(void) {
    if (saved_xdg_runtime_dir != NULL) {
        setenv("XDG_RUNTIME_DIR", saved_xdg_runtime_dir, 1);
        free(saved_xdg_runtime_dir);
        saved_xdg_runtime_dir = NULL;
    } else {
        unsetenv("XDG_RUNTIME_DIR");
    }
}

/**
 * Helper: Create a temporary directory for testing
 * Uses project-local directory instead of /tmp
 */
static char temp_dir[256] = {0};

static int create_temp_dir(void) {
    /* Try to use TMPDIR if set, otherwise use ./tmp */
    const char *base_dir = getenv("TMPDIR");
    if (base_dir == NULL || base_dir[0] == '\0') {
        base_dir = "./tmp";
        /* Ensure ./tmp exists */
        mkdir(base_dir, 0700); /* Ignore errors if it already exists */
    }

    snprintf(temp_dir, sizeof(temp_dir), "%s/weave_test_XXXXXX", base_dir);
    if (mkdtemp(temp_dir) == NULL) {
        return -1;
    }
    return 0;
}

static void cleanup_temp_dir(void) {
    if (temp_dir[0] == '\0') {
        return;
    }

    /*
     * Safe directory cleanup without shell injection risk.
     * We know the structure: temp_dir/weave/weave.sock
     * Remove in reverse order: socket file, weave dir, temp dir.
     */
    char path_buf[512];

    /* Remove socket file if it exists */
    snprintf(path_buf, sizeof(path_buf), "%s/%s/%s",
             temp_dir, SOCKET_DIR_NAME, SOCKET_FILE_NAME);
    unlink(path_buf); /* Ignore errors - file may not exist */

    /* Remove weave directory */
    snprintf(path_buf, sizeof(path_buf), "%s/%s", temp_dir, SOCKET_DIR_NAME);
    rmdir(path_buf); /* Ignore errors - dir may not exist */

    /* Remove temp directory */
    rmdir(temp_dir); /* Ignore errors */

    temp_dir[0] = '\0';
}

/**
 * ==========================================================================
 * Path Construction Tests
 * ==========================================================================
 */

/**
 * Test: socket_get_path returns correct path
 */
void test_get_path_valid(void) {
    TEST("test_get_path_valid");

    save_xdg_runtime_dir();

    setenv("XDG_RUNTIME_DIR", "/run/user/1000", 1);

    char path[SOCKET_PATH_MAX];
    socket_error_t err = socket_get_path(path, sizeof(path));

    ASSERT_EQ(SOCKET_OK, err);
    ASSERT_STR_EQ("/run/user/1000/weave/weave.sock", path);

    restore_xdg_runtime_dir();

    TEST_PASS();
}

/**
 * Test: socket_get_path fails when XDG_RUNTIME_DIR not set
 */
void test_get_path_xdg_not_set(void) {
    TEST("test_get_path_xdg_not_set");

    save_xdg_runtime_dir();

    unsetenv("XDG_RUNTIME_DIR");

    char path[SOCKET_PATH_MAX];
    socket_error_t err = socket_get_path(path, sizeof(path));

    ASSERT_EQ(SOCKET_ERR_XDG_NOT_SET, err);

    restore_xdg_runtime_dir();

    TEST_PASS();
}

/**
 * Test: socket_get_path fails when XDG_RUNTIME_DIR is empty
 */
void test_get_path_xdg_empty(void) {
    TEST("test_get_path_xdg_empty");

    save_xdg_runtime_dir();

    setenv("XDG_RUNTIME_DIR", "", 1);

    char path[SOCKET_PATH_MAX];
    socket_error_t err = socket_get_path(path, sizeof(path));

    ASSERT_EQ(SOCKET_ERR_XDG_NOT_SET, err);

    restore_xdg_runtime_dir();

    TEST_PASS();
}

/**
 * Test: socket_get_path fails with NULL buffer
 */
void test_get_path_null_buffer(void) {
    TEST("test_get_path_null_buffer");

    socket_error_t err = socket_get_path(NULL, SOCKET_PATH_MAX);

    ASSERT_EQ(SOCKET_ERR_NULL_POINTER, err);

    TEST_PASS();
}

/**
 * Test: socket_get_path fails when buffer too small
 */
void test_get_path_buffer_too_small(void) {
    TEST("test_get_path_buffer_too_small");

    save_xdg_runtime_dir();

    setenv("XDG_RUNTIME_DIR", "/run/user/1000", 1);

    char path[10]; /* Too small */
    socket_error_t err = socket_get_path(path, sizeof(path));

    ASSERT_EQ(SOCKET_ERR_PATH_TOO_LONG, err);

    restore_xdg_runtime_dir();

    TEST_PASS();
}

/**
 * Test: socket_get_path handles long XDG_RUNTIME_DIR
 */
void test_get_path_long_xdg(void) {
    TEST("test_get_path_long_xdg");

    save_xdg_runtime_dir();

    /* Create a path that would exceed SOCKET_PATH_MAX when combined */
    char long_path[SOCKET_PATH_MAX];
    memset(long_path, 'a', sizeof(long_path) - 1);
    long_path[sizeof(long_path) - 1] = '\0';

    setenv("XDG_RUNTIME_DIR", long_path, 1);

    char path[SOCKET_PATH_MAX];
    socket_error_t err = socket_get_path(path, sizeof(path));

    ASSERT_EQ(SOCKET_ERR_PATH_TOO_LONG, err);

    restore_xdg_runtime_dir();

    TEST_PASS();
}

/**
 * Test: socket_get_dir_path returns correct path
 */
void test_get_dir_path_valid(void) {
    TEST("test_get_dir_path_valid");

    save_xdg_runtime_dir();

    setenv("XDG_RUNTIME_DIR", "/run/user/1000", 1);

    char path[SOCKET_PATH_MAX];
    socket_error_t err = socket_get_dir_path(path, sizeof(path));

    ASSERT_EQ(SOCKET_OK, err);
    ASSERT_STR_EQ("/run/user/1000/weave", path);

    restore_xdg_runtime_dir();

    TEST_PASS();
}

/**
 * Test: socket_get_dir_path fails with NULL buffer
 */
void test_get_dir_path_null_buffer(void) {
    TEST("test_get_dir_path_null_buffer");

    socket_error_t err = socket_get_dir_path(NULL, SOCKET_PATH_MAX);

    ASSERT_EQ(SOCKET_ERR_NULL_POINTER, err);

    TEST_PASS();
}

/**
 * ==========================================================================
 * Socket Creation Tests
 * ==========================================================================
 */

/**
 * Test: socket_create with NULL pointer
 */
void test_create_null_pointer(void) {
    TEST("test_create_null_pointer");

    socket_error_t err = socket_create(NULL);

    ASSERT_EQ(SOCKET_ERR_NULL_POINTER, err);

    TEST_PASS();
}

/**
 * Test: socket_create fails when XDG_RUNTIME_DIR not set
 */
void test_create_xdg_not_set(void) {
    TEST("test_create_xdg_not_set");

    save_xdg_runtime_dir();

    unsetenv("XDG_RUNTIME_DIR");

    int fd;
    socket_error_t err = socket_create(&fd);

    ASSERT_EQ(SOCKET_ERR_XDG_NOT_SET, err);

    restore_xdg_runtime_dir();

    TEST_PASS();
}

/**
 * Test: socket_create creates socket successfully
 */
void test_create_success(void) {
    TEST("test_create_success");

    save_xdg_runtime_dir();

    if (create_temp_dir() != 0) {
        printf("  SKIP: Could not create temp directory\n");
        restore_xdg_runtime_dir();
        tests_passed++;
        return;
    }

    setenv("XDG_RUNTIME_DIR", temp_dir, 1);

    int fd = -1;
    socket_error_t err = socket_create(&fd);

    ASSERT_EQ(SOCKET_OK, err);
    ASSERT_TRUE(fd >= 0);

    /* Verify socket file exists */
    char socket_path[SOCKET_PATH_MAX];
    socket_get_path(socket_path, sizeof(socket_path));

    struct stat st;
    ASSERT_EQ(0, stat(socket_path, &st));
    ASSERT_TRUE(S_ISSOCK(st.st_mode));

    /* Verify permissions are 0600 */
    ASSERT_EQ(0600, st.st_mode & 0777);

    /* Verify directory permissions are 0700 */
    char dir_path[SOCKET_PATH_MAX];
    socket_get_dir_path(dir_path, sizeof(dir_path));
    ASSERT_EQ(0, stat(dir_path, &st));
    ASSERT_EQ(0700, st.st_mode & 0777);

    close(fd);
    socket_cleanup();
    cleanup_temp_dir();

    restore_xdg_runtime_dir();

    TEST_PASS();
}

/**
 * Test: socket_create removes stale socket
 *
 * A "stale socket" is a socket file left behind by a crashed process that
 * didn't clean up properly. The file exists on disk, but no process is
 * listening on it. When we try to connect(), it fails with ECONNREFUSED.
 * weave-compute should detect this and remove the stale file before binding.
 */
void test_create_removes_stale_socket(void) {
    TEST("test_create_removes_stale_socket");

    save_xdg_runtime_dir();

    if (create_temp_dir() != 0) {
        printf("  SKIP: Could not create temp directory\n");
        restore_xdg_runtime_dir();
        tests_passed++;
        return;
    }

    setenv("XDG_RUNTIME_DIR", temp_dir, 1);

    /* Create socket directory */
    char dir_path[SOCKET_PATH_MAX];
    socket_get_dir_path(dir_path, sizeof(dir_path));
    mkdir(dir_path, 0700);

    char socket_path[SOCKET_PATH_MAX];
    socket_get_path(socket_path, sizeof(socket_path));

    /*
     * Simulate a crashed process: create a socket, bind it, then close
     * without calling listen(). This leaves the socket file on disk,
     * but connect() will fail because no one is listening.
     */
    int stale_fd = socket(AF_UNIX, SOCK_STREAM, 0);
    ASSERT_TRUE(stale_fd >= 0);

    struct sockaddr_un addr;
    memset(&addr, 0, sizeof(addr));
    addr.sun_family = AF_UNIX;
    strncpy(addr.sun_path, socket_path, sizeof(addr.sun_path) - 1);
    bind(stale_fd, (struct sockaddr *)&addr, sizeof(addr));
    close(stale_fd); /* Socket file remains, but no listener - this is "stale" */

    /* Now create our socket - should succeed after removing stale */
    int fd = -1;
    socket_error_t err = socket_create(&fd);

    ASSERT_EQ(SOCKET_OK, err);
    ASSERT_TRUE(fd >= 0);

    close(fd);
    socket_cleanup();
    cleanup_temp_dir();

    restore_xdg_runtime_dir();

    TEST_PASS();
}

/**
 * ==========================================================================
 * Socket Cleanup Tests
 * ==========================================================================
 */

/**
 * Test: socket_cleanup removes socket file
 */
void test_cleanup_removes_socket(void) {
    TEST("test_cleanup_removes_socket");

    save_xdg_runtime_dir();

    if (create_temp_dir() != 0) {
        printf("  SKIP: Could not create temp directory\n");
        restore_xdg_runtime_dir();
        tests_passed++;
        return;
    }

    setenv("XDG_RUNTIME_DIR", temp_dir, 1);

    /* Create socket */
    int fd = -1;
    socket_error_t err = socket_create(&fd);
    ASSERT_EQ(SOCKET_OK, err);

    char socket_path[SOCKET_PATH_MAX];
    socket_get_path(socket_path, sizeof(socket_path));

    /* Verify socket exists */
    struct stat st;
    ASSERT_EQ(0, stat(socket_path, &st));

    /* Cleanup */
    close(fd);
    err = socket_cleanup();
    ASSERT_EQ(SOCKET_OK, err);

    /* Verify socket is removed */
    ASSERT_NE(0, stat(socket_path, &st));

    cleanup_temp_dir();

    restore_xdg_runtime_dir();

    TEST_PASS();
}

/**
 * Test: socket_cleanup is safe to call multiple times
 */
void test_cleanup_idempotent(void) {
    TEST("test_cleanup_idempotent");

    save_xdg_runtime_dir();

    if (create_temp_dir() != 0) {
        printf("  SKIP: Could not create temp directory\n");
        restore_xdg_runtime_dir();
        tests_passed++;
        return;
    }

    setenv("XDG_RUNTIME_DIR", temp_dir, 1);

    /* Create socket */
    int fd = -1;
    socket_error_t err = socket_create(&fd);
    ASSERT_EQ(SOCKET_OK, err);

    close(fd);

    /* Cleanup multiple times */
    err = socket_cleanup();
    ASSERT_EQ(SOCKET_OK, err);

    err = socket_cleanup();
    ASSERT_EQ(SOCKET_OK, err);

    err = socket_cleanup();
    ASSERT_EQ(SOCKET_OK, err);

    cleanup_temp_dir();

    restore_xdg_runtime_dir();

    TEST_PASS();
}

/**
 * Test: socket_cleanup with no socket created
 */
void test_cleanup_no_socket(void) {
    TEST("test_cleanup_no_socket");

    save_xdg_runtime_dir();

    if (create_temp_dir() != 0) {
        printf("  SKIP: Could not create temp directory\n");
        restore_xdg_runtime_dir();
        tests_passed++;
        return;
    }

    setenv("XDG_RUNTIME_DIR", temp_dir, 1);

    /* Cleanup without creating socket */
    socket_error_t err = socket_cleanup();
    ASSERT_EQ(SOCKET_OK, err);

    cleanup_temp_dir();

    restore_xdg_runtime_dir();

    TEST_PASS();
}

/**
 * ==========================================================================
 * Authentication Tests
 * ==========================================================================
 */

/**
 * Captured log messages for testing
 */
static char captured_log_message[512] = {0};
static socket_log_level_t captured_log_level = SOCKET_LOG_NONE;

static void test_log_callback(socket_log_level_t level, const char *message) {
    captured_log_level = level;
    strncpy(captured_log_message, message, sizeof(captured_log_message) - 1);
    captured_log_message[sizeof(captured_log_message) - 1] = '\0';
}

static void clear_captured_log(void) {
    captured_log_message[0] = '\0';
    captured_log_level = SOCKET_LOG_NONE;
}

/**
 * Test: socket_auth_connection with invalid fd
 */
void test_auth_invalid_fd(void) {
    TEST("test_auth_invalid_fd");

    socket_error_t err = socket_auth_connection(-1);
    ASSERT_EQ(SOCKET_ERR_INVALID_FD, err);

    TEST_PASS();
}

/**
 * Test: socket_auth_connection with same UID succeeds
 *
 * This test creates a socket pair and verifies that authentication
 * succeeds when both ends are owned by the same user.
 */
void test_auth_same_uid_succeeds(void) {
    TEST("test_auth_same_uid_succeeds");

    save_xdg_runtime_dir();

    if (create_temp_dir() != 0) {
        printf("  SKIP: Could not create temp directory\n");
        restore_xdg_runtime_dir();
        tests_passed++;
        return;
    }

    setenv("XDG_RUNTIME_DIR", temp_dir, 1);

    /* Create listening socket */
    int listen_fd = -1;
    socket_error_t err = socket_create(&listen_fd);
    ASSERT_EQ(SOCKET_OK, err);
    ASSERT_TRUE(listen_fd >= 0);

    /* Create client socket and connect */
    int client_fd = socket(AF_UNIX, SOCK_STREAM, 0);
    ASSERT_TRUE(client_fd >= 0);

    char socket_path[SOCKET_PATH_MAX];
    socket_get_path(socket_path, sizeof(socket_path));

    struct sockaddr_un addr;
    memset(&addr, 0, sizeof(addr));
    addr.sun_family = AF_UNIX;
    strncpy(addr.sun_path, socket_path, sizeof(addr.sun_path) - 1);

    int connect_result = connect(client_fd, (struct sockaddr *)&addr, sizeof(addr));
    ASSERT_EQ(0, connect_result);

    /* Accept the connection */
    int accepted_fd = accept(listen_fd, NULL, NULL);
    ASSERT_TRUE(accepted_fd >= 0);

    /* Authenticate - should succeed since same user */
    err = socket_auth_connection(accepted_fd);
    ASSERT_EQ(SOCKET_OK, err);

    /* Cleanup */
    close(accepted_fd);
    close(client_fd);
    close(listen_fd);
    socket_cleanup();
    cleanup_temp_dir();

    restore_xdg_runtime_dir();

    TEST_PASS();
}

/**
 * Test: socket_auth_connection logs at DEBUG level only
 *
 * Verifies that auth rejection is logged at DEBUG level and not shown
 * when log level is INFO or higher.
 */
void test_auth_logs_at_debug_level(void) {
    TEST("test_auth_logs_at_debug_level");

    save_xdg_runtime_dir();

    if (create_temp_dir() != 0) {
        printf("  SKIP: Could not create temp directory\n");
        restore_xdg_runtime_dir();
        tests_passed++;
        return;
    }

    setenv("XDG_RUNTIME_DIR", temp_dir, 1);

    /* Create listening socket */
    int listen_fd = -1;
    socket_error_t err = socket_create(&listen_fd);
    ASSERT_EQ(SOCKET_OK, err);

    /* Create client socket and connect */
    int client_fd = socket(AF_UNIX, SOCK_STREAM, 0);
    ASSERT_TRUE(client_fd >= 0);

    char socket_path[SOCKET_PATH_MAX];
    socket_get_path(socket_path, sizeof(socket_path));

    struct sockaddr_un addr;
    memset(&addr, 0, sizeof(addr));
    addr.sun_family = AF_UNIX;
    strncpy(addr.sun_path, socket_path, sizeof(addr.sun_path) - 1);

    connect(client_fd, (struct sockaddr *)&addr, sizeof(addr));
    int accepted_fd = accept(listen_fd, NULL, NULL);
    ASSERT_TRUE(accepted_fd >= 0);

    /* Set up log capture at DEBUG level */
    clear_captured_log();
    socket_set_log_level(SOCKET_LOG_DEBUG);
    socket_set_log_callback(test_log_callback);

    /* Authenticate - should succeed and log */
    err = socket_auth_connection(accepted_fd);
    ASSERT_EQ(SOCKET_OK, err);

    /* Verify log was captured at DEBUG level */
    ASSERT_EQ(SOCKET_LOG_DEBUG, captured_log_level);
    ASSERT_STR_CONTAINS(captured_log_message, "auth accepted");

    /* Now test with INFO level - should not log */
    clear_captured_log();
    socket_set_log_level(SOCKET_LOG_INFO);

    err = socket_auth_connection(accepted_fd);
    ASSERT_EQ(SOCKET_OK, err);

    /* Verify no log was captured (level is NONE means no callback called) */
    ASSERT_EQ(SOCKET_LOG_NONE, captured_log_level);

    /* Reset to defaults */
    socket_set_log_level(SOCKET_LOG_INFO);
    socket_set_log_callback(NULL);

    /* Cleanup */
    close(accepted_fd);
    close(client_fd);
    close(listen_fd);
    socket_cleanup();
    cleanup_temp_dir();

    restore_xdg_runtime_dir();

    TEST_PASS();
}

/**
 * ==========================================================================
 * Timeout Tests
 * ==========================================================================
 */

/**
 * Test: socket_set_timeouts with invalid fd
 */
void test_set_timeouts_invalid_fd(void) {
    TEST("test_set_timeouts_invalid_fd");

    socket_error_t err = socket_set_timeouts(-1, 60, 5);
    ASSERT_EQ(SOCKET_ERR_INVALID_FD, err);

    TEST_PASS();
}

/**
 * Test: socket_set_timeouts success
 */
void test_set_timeouts_success(void) {
    TEST("test_set_timeouts_success");

    /* Create a socket to test with */
    int fd = socket(AF_UNIX, SOCK_STREAM, 0);
    ASSERT_TRUE(fd >= 0);

    socket_error_t err = socket_set_timeouts(fd, 60, 5);
    ASSERT_EQ(SOCKET_OK, err);

    /* Verify the timeouts were set */
    struct timeval tv;
    socklen_t len = sizeof(tv);

    int result = getsockopt(fd, SOL_SOCKET, SO_RCVTIMEO, &tv, &len);
    ASSERT_EQ(0, result);
    ASSERT_EQ(60, tv.tv_sec);

    result = getsockopt(fd, SOL_SOCKET, SO_SNDTIMEO, &tv, &len);
    ASSERT_EQ(0, result);
    ASSERT_EQ(5, tv.tv_sec);

    close(fd);

    TEST_PASS();
}

/**
 * Test: socket_set_timeouts with zero disables timeout
 */
void test_set_timeouts_zero_disables(void) {
    TEST("test_set_timeouts_zero_disables");

    /* Create a socket to test with */
    int fd = socket(AF_UNIX, SOCK_STREAM, 0);
    ASSERT_TRUE(fd >= 0);

    /* Set non-zero timeouts first */
    socket_error_t err = socket_set_timeouts(fd, 60, 5);
    ASSERT_EQ(SOCKET_OK, err);

    /* Now set zero - should not change existing timeouts */
    err = socket_set_timeouts(fd, 0, 0);
    ASSERT_EQ(SOCKET_OK, err);

    /* Verify the original timeouts are still set */
    struct timeval tv;
    socklen_t len = sizeof(tv);

    getsockopt(fd, SOL_SOCKET, SO_RCVTIMEO, &tv, &len);
    ASSERT_EQ(60, tv.tv_sec);

    close(fd);

    TEST_PASS();
}

/**
 * ==========================================================================
 * Shutdown Tests
 * ==========================================================================
 */

/* Forward declaration of internal reset function */
extern void socket_reset_shutdown(void);

/**
 * Test: socket_request_shutdown and socket_is_shutdown_requested
 */
void test_shutdown_flag(void) {
    TEST("test_shutdown_flag");

    /* Reset state from previous tests */
    socket_reset_shutdown();

    /* Initially not requested */
    ASSERT_EQ(0, socket_is_shutdown_requested());

    /* Request shutdown */
    socket_request_shutdown();
    ASSERT_NE(0, socket_is_shutdown_requested());

    /* Reset for other tests */
    socket_reset_shutdown();
    ASSERT_EQ(0, socket_is_shutdown_requested());

    TEST_PASS();
}

/**
 * ==========================================================================
 * Accept Loop Tests
 * ==========================================================================
 */

/**
 * Test: socket_accept_loop with invalid fd
 */
void test_accept_loop_invalid_fd(void) {
    TEST("test_accept_loop_invalid_fd");

    socket_error_t err = socket_accept_loop(-1, NULL);
    ASSERT_EQ(SOCKET_ERR_INVALID_FD, err);

    TEST_PASS();
}

/**
 * Test: socket_accept_loop with null handler
 */
void test_accept_loop_null_handler(void) {
    TEST("test_accept_loop_null_handler");

    /* Create a valid socket */
    int fd = socket(AF_UNIX, SOCK_STREAM, 0);
    ASSERT_TRUE(fd >= 0);

    socket_error_t err = socket_accept_loop(fd, NULL);
    ASSERT_EQ(SOCKET_ERR_NULL_HANDLER, err);

    close(fd);

    TEST_PASS();
}

/**
 * Handler for testing - counts calls and optionally signals shutdown
 */
static int handler_call_count = 0;
static int handler_shutdown_after = 0;

static int test_handler(int client_fd) {
    (void)client_fd;
    handler_call_count++;

    if (handler_shutdown_after > 0 && handler_call_count >= handler_shutdown_after) {
        socket_request_shutdown();
    }

    return 0;
}

/**
 * Test: socket_accept_loop handles shutdown
 *
 * This test creates a listening socket, starts a "client" that connects,
 * and verifies that the accept loop exits when shutdown is requested.
 */
void test_accept_loop_handles_shutdown(void) {
    TEST("test_accept_loop_handles_shutdown");

    save_xdg_runtime_dir();
    socket_reset_shutdown();

    if (create_temp_dir() != 0) {
        printf("  SKIP: Could not create temp directory\n");
        restore_xdg_runtime_dir();
        tests_passed++;
        return;
    }

    setenv("XDG_RUNTIME_DIR", temp_dir, 1);

    /* Create listening socket */
    int listen_fd = -1;
    socket_error_t err = socket_create(&listen_fd);
    ASSERT_EQ(SOCKET_OK, err);

    /* Configure handler to shutdown after first connection */
    handler_call_count = 0;
    handler_shutdown_after = 1;

    /* Fork a child to act as client */
    pid_t pid = fork();
    if (pid == 0) {
        /* Child: connect to socket */
        close(listen_fd);

        /* Small delay to let parent enter accept() */
        usleep(50000);

        char socket_path[SOCKET_PATH_MAX];
        socket_get_path(socket_path, sizeof(socket_path));

        int client_fd = socket(AF_UNIX, SOCK_STREAM, 0);
        struct sockaddr_un addr;
        memset(&addr, 0, sizeof(addr));
        addr.sun_family = AF_UNIX;
        strncpy(addr.sun_path, socket_path, sizeof(addr.sun_path) - 1);

        connect(client_fd, (struct sockaddr *)&addr, sizeof(addr));
        close(client_fd);

        _exit(0);
    }

    /* Parent: run accept loop */
    ASSERT_TRUE(pid > 0);

    err = socket_accept_loop(listen_fd, test_handler);
    ASSERT_EQ(SOCKET_OK, err);
    ASSERT_EQ(1, handler_call_count);

    /* Wait for child */
    int status;
    waitpid(pid, &status, 0);

    /* Cleanup */
    close(listen_fd);
    socket_cleanup();
    cleanup_temp_dir();
    socket_reset_shutdown();

    restore_xdg_runtime_dir();

    TEST_PASS();
}

/**
 * ==========================================================================
 * Socket Connect Tests
 * ==========================================================================
 */

/**
 * Test: socket_connect with NULL socket_path
 */
void test_connect_null_socket_path(void) {
    TEST("test_connect_null_socket_path");

    int fd;
    socket_error_t err = socket_connect(NULL, &fd);

    ASSERT_EQ(SOCKET_ERR_NULL_POINTER, err);

    TEST_PASS();
}

/**
 * Test: socket_connect with NULL connected_fd
 */
void test_connect_null_connected_fd(void) {
    TEST("test_connect_null_connected_fd");

    /* Use project-local path */
    socket_error_t err = socket_connect("./tmp/test.sock", NULL);

    ASSERT_EQ(SOCKET_ERR_NULL_POINTER, err);

    TEST_PASS();
}

/**
 * Test: socket_connect with path too long
 */
void test_connect_path_too_long(void) {
    TEST("test_connect_path_too_long");

    /* Create a path that exceeds sockaddr_un.sun_path limit */
    char long_path[256];
    memset(long_path, 'a', sizeof(long_path) - 1);
    long_path[sizeof(long_path) - 1] = '\0';

    int fd;
    socket_error_t err = socket_connect(long_path, &fd);

    ASSERT_EQ(SOCKET_ERR_PATH_TOO_LONG, err);

    TEST_PASS();
}

/**
 * Test: socket_connect to non-existent socket fails
 */
void test_connect_nonexistent_socket(void) {
    TEST("test_connect_nonexistent_socket");

    int fd;
    /* Use project-local path */
    socket_error_t err = socket_connect("./tmp/weave_nonexistent_socket.sock", &fd);

    ASSERT_EQ(SOCKET_ERR_CONNECT_FAILED, err);

    TEST_PASS();
}

/**
 * Test: socket_connect to existing socket succeeds
 */
void test_connect_success(void) {
    TEST("test_connect_success");

    save_xdg_runtime_dir();

    if (create_temp_dir() != 0) {
        printf("  SKIP: Could not create temp directory\n");
        restore_xdg_runtime_dir();
        tests_passed++;
        return;
    }

    setenv("XDG_RUNTIME_DIR", temp_dir, 1);

    /* Create listening socket */
    int listen_fd = -1;
    socket_error_t err = socket_create(&listen_fd);
    ASSERT_EQ(SOCKET_OK, err);
    ASSERT_TRUE(listen_fd >= 0);

    /* Get the socket path */
    char socket_path[SOCKET_PATH_MAX];
    err = socket_get_path(socket_path, sizeof(socket_path));
    ASSERT_EQ(SOCKET_OK, err);

    /* Connect to the socket */
    int connected_fd = -1;
    err = socket_connect(socket_path, &connected_fd);
    ASSERT_EQ(SOCKET_OK, err);
    ASSERT_TRUE(connected_fd >= 0);

    /* Accept the connection on the listening socket */
    int accepted_fd = accept(listen_fd, NULL, NULL);
    ASSERT_TRUE(accepted_fd >= 0);

    /* Cleanup */
    close(connected_fd);
    close(accepted_fd);
    close(listen_fd);
    socket_cleanup();
    cleanup_temp_dir();

    restore_xdg_runtime_dir();

    TEST_PASS();
}

/**
 * ==========================================================================
 * Request/Response Loop Tests (Client Mode)
 * ==========================================================================
 */

/**
 * Test: Client connection can send and receive data
 *
 * This test verifies that a socket connected via socket_connect() can
 * successfully send and receive data over the connection. This simulates
 * the basic request/response pattern used in client mode.
 */
void test_client_send_receive(void) {
    TEST("test_client_send_receive");

    save_xdg_runtime_dir();

    if (create_temp_dir() != 0) {
        printf("  SKIP: Could not create temp directory\n");
        restore_xdg_runtime_dir();
        tests_passed++;
        return;
    }

    setenv("XDG_RUNTIME_DIR", temp_dir, 1);

    /* Create listening socket */
    int listen_fd = -1;
    socket_error_t err = socket_create(&listen_fd);
    ASSERT_EQ(SOCKET_OK, err);
    ASSERT_TRUE(listen_fd >= 0);

    /* Get socket path */
    char socket_path[SOCKET_PATH_MAX];
    err = socket_get_path(socket_path, sizeof(socket_path));
    ASSERT_EQ(SOCKET_OK, err);

    /* Fork a child to act as client */
    pid_t pid = fork();
    if (pid == 0) {
        /* Child: connect to socket and send data */
        close(listen_fd);

        /* Small delay to let parent enter accept() */
        usleep(50000);

        /* Connect to socket */
        int client_fd = -1;
        socket_error_t connect_err = socket_connect(socket_path, &client_fd);
        if (connect_err != SOCKET_OK) {
            _exit(1);
        }

        /* Send test data */
        const char *test_msg = "test_request";
        if (write(client_fd, test_msg, strlen(test_msg)) != (ssize_t)strlen(test_msg)) {
            close(client_fd);
            _exit(2);
        }

        /* Receive response */
        char response[64];
        ssize_t n = read(client_fd, response, sizeof(response) - 1);
        if (n <= 0) {
            close(client_fd);
            _exit(3);
        }
        response[n] = '\0';

        /* Verify response */
        if (strcmp(response, "test_response") != 0) {
            close(client_fd);
            _exit(4);
        }

        close(client_fd);
        _exit(0);
    }

    /* Parent: accept connection and echo */
    ASSERT_TRUE(pid > 0);

    int accepted_fd = accept(listen_fd, NULL, NULL);
    ASSERT_TRUE(accepted_fd >= 0);

    /* Receive request */
    char request[64];
    ssize_t n = read(accepted_fd, request, sizeof(request) - 1);
    ASSERT_TRUE(n > 0);
    request[n] = '\0';

    /* Verify request */
    ASSERT_STR_EQ("test_request", request);

    /* Send response */
    const char *response = "test_response";
    ssize_t written = write(accepted_fd, response, strlen(response));
    ASSERT_EQ((ssize_t)strlen(response), written);

    /* Wait for child */
    int status;
    waitpid(pid, &status, 0);
    ASSERT_TRUE(WIFEXITED(status));
    ASSERT_EQ(0, WEXITSTATUS(status));

    /* Cleanup */
    close(accepted_fd);
    close(listen_fd);
    socket_cleanup();
    cleanup_temp_dir();

    restore_xdg_runtime_dir();

    TEST_PASS();
}

/**
 * Test: Client connection loop handles multiple requests
 *
 * This test verifies that the same connected socket can handle multiple
 * sequential requests, simulating the persistent connection pattern in
 * client mode.
 */
void test_client_multiple_requests(void) {
    TEST("test_client_multiple_requests");

    save_xdg_runtime_dir();

    if (create_temp_dir() != 0) {
        printf("  SKIP: Could not create temp directory\n");
        restore_xdg_runtime_dir();
        tests_passed++;
        return;
    }

    setenv("XDG_RUNTIME_DIR", temp_dir, 1);

    /* Create listening socket */
    int listen_fd = -1;
    socket_error_t err = socket_create(&listen_fd);
    ASSERT_EQ(SOCKET_OK, err);

    /* Get socket path */
    char socket_path[SOCKET_PATH_MAX];
    err = socket_get_path(socket_path, sizeof(socket_path));
    ASSERT_EQ(SOCKET_OK, err);

    /* Fork a child to act as client */
    pid_t pid = fork();
    if (pid == 0) {
        /* Child: connect and send multiple requests */
        close(listen_fd);

        usleep(50000);

        int client_fd = -1;
        socket_error_t connect_err = socket_connect(socket_path, &client_fd);
        if (connect_err != SOCKET_OK) {
            _exit(1);
        }

        /* Send 3 requests over the same connection */
        for (int i = 1; i <= 3; i++) {
            char request[32];
            snprintf(request, sizeof(request), "request_%d", i);

            if (write(client_fd, request, strlen(request)) != (ssize_t)strlen(request)) {
                close(client_fd);
                _exit(2);
            }

            /* Receive response */
            char response[32];
            ssize_t n = read(client_fd, response, sizeof(response) - 1);
            if (n <= 0) {
                close(client_fd);
                _exit(3);
            }
            response[n] = '\0';

            /* Verify response matches request */
            char expected[32];
            snprintf(expected, sizeof(expected), "response_%d", i);
            if (strcmp(response, expected) != 0) {
                close(client_fd);
                _exit(4);
            }
        }

        close(client_fd);
        _exit(0);
    }

    /* Parent: accept and handle multiple requests */
    ASSERT_TRUE(pid > 0);

    int accepted_fd = accept(listen_fd, NULL, NULL);
    ASSERT_TRUE(accepted_fd >= 0);

    /* Handle 3 requests */
    for (int i = 1; i <= 3; i++) {
        char request[32];
        ssize_t n = read(accepted_fd, request, sizeof(request) - 1);
        ASSERT_TRUE(n > 0);
        request[n] = '\0';

        /* Send matching response */
        char response[32];
        snprintf(response, sizeof(response), "response_%d", i);
        ssize_t written = write(accepted_fd, response, strlen(response));
        ASSERT_EQ((ssize_t)strlen(response), written);
    }

    /* Wait for child */
    int status;
    waitpid(pid, &status, 0);
    ASSERT_TRUE(WIFEXITED(status));
    ASSERT_EQ(0, WEXITSTATUS(status));

    /* Cleanup */
    close(accepted_fd);
    close(listen_fd);
    socket_cleanup();
    cleanup_temp_dir();

    restore_xdg_runtime_dir();

    TEST_PASS();
}

/**
 * Test: Client loop terminates on connection close
 *
 * This test verifies that when the server closes the connection, the
 * client can detect it (via read returning 0) and terminate cleanly.
 * This simulates the shutdown pattern in client mode.
 */
void test_client_terminates_on_close(void) {
    TEST("test_client_terminates_on_close");

    save_xdg_runtime_dir();

    if (create_temp_dir() != 0) {
        printf("  SKIP: Could not create temp directory\n");
        restore_xdg_runtime_dir();
        tests_passed++;
        return;
    }

    setenv("XDG_RUNTIME_DIR", temp_dir, 1);

    /* Create listening socket */
    int listen_fd = -1;
    socket_error_t err = socket_create(&listen_fd);
    ASSERT_EQ(SOCKET_OK, err);

    /* Get socket path */
    char socket_path[SOCKET_PATH_MAX];
    err = socket_get_path(socket_path, sizeof(socket_path));
    ASSERT_EQ(SOCKET_OK, err);

    /* Fork a child to act as client */
    pid_t pid = fork();
    if (pid == 0) {
        /* Child: connect and detect connection close */
        close(listen_fd);

        usleep(50000);

        int client_fd = -1;
        socket_error_t connect_err = socket_connect(socket_path, &client_fd);
        if (connect_err != SOCKET_OK) {
            _exit(1);
        }

        /*
         * Simulate request/response loop that reads from socket.
         * When server closes, read() should return 0 (EOF).
         */
        char buf[64];
        ssize_t n = read(client_fd, buf, sizeof(buf));

        if (n != 0) {
            /* Expected EOF (n=0), but got something else */
            close(client_fd);
            _exit(2);
        }

        /* EOF detected correctly - clean shutdown */
        close(client_fd);
        _exit(0);
    }

    /* Parent: accept and immediately close connection */
    ASSERT_TRUE(pid > 0);

    int accepted_fd = accept(listen_fd, NULL, NULL);
    ASSERT_TRUE(accepted_fd >= 0);

    /* Close connection without sending anything */
    close(accepted_fd);

    /* Wait for child to detect closure and exit cleanly */
    int status;
    waitpid(pid, &status, 0);
    ASSERT_TRUE(WIFEXITED(status));
    ASSERT_EQ(0, WEXITSTATUS(status));

    /* Cleanup */
    close(listen_fd);
    socket_cleanup();
    cleanup_temp_dir();

    restore_xdg_runtime_dir();

    TEST_PASS();
}

/**
 * Test: Client handles partial writes correctly
 *
 * This test verifies that the client connection can handle scenarios
 * where writes might be partial (though unlikely with Unix sockets).
 */
void test_client_handles_partial_io(void) {
    TEST("test_client_handles_partial_io");

    save_xdg_runtime_dir();

    if (create_temp_dir() != 0) {
        printf("  SKIP: Could not create temp directory\n");
        restore_xdg_runtime_dir();
        tests_passed++;
        return;
    }

    setenv("XDG_RUNTIME_DIR", temp_dir, 1);

    /* Create listening socket */
    int listen_fd = -1;
    socket_error_t err = socket_create(&listen_fd);
    ASSERT_EQ(SOCKET_OK, err);

    /* Get socket path */
    char socket_path[SOCKET_PATH_MAX];
    err = socket_get_path(socket_path, sizeof(socket_path));
    ASSERT_EQ(SOCKET_OK, err);

    /* Fork a child to act as client */
    pid_t pid = fork();
    if (pid == 0) {
        /* Child: connect and send larger data */
        close(listen_fd);

        usleep(50000);

        int client_fd = -1;
        socket_error_t connect_err = socket_connect(socket_path, &client_fd);
        if (connect_err != SOCKET_OK) {
            _exit(1);
        }

        /* Send 1KB of data */
        char data[1024];
        memset(data, 'A', sizeof(data));

        size_t total_written = 0;
        while (total_written < sizeof(data)) {
            ssize_t n = write(client_fd, data + total_written,
                            sizeof(data) - total_written);
            if (n <= 0) {
                close(client_fd);
                _exit(2);
            }
            total_written += (size_t)n;
        }

        /* Receive acknowledgment */
        char ack[4];
        ssize_t n = read(client_fd, ack, sizeof(ack));
        if (n != 3 || memcmp(ack, "ACK", 3) != 0) {
            close(client_fd);
            _exit(3);
        }

        close(client_fd);
        _exit(0);
    }

    /* Parent: accept and receive all data */
    ASSERT_TRUE(pid > 0);

    int accepted_fd = accept(listen_fd, NULL, NULL);
    ASSERT_TRUE(accepted_fd >= 0);

    /* Read all 1KB */
    char received[1024];
    size_t total_read = 0;
    while (total_read < sizeof(received)) {
        ssize_t n = read(accepted_fd, received + total_read,
                        sizeof(received) - total_read);
        ASSERT_TRUE(n > 0);
        total_read += (size_t)n;
    }

    /* Verify data */
    for (size_t i = 0; i < sizeof(received); i++) {
        ASSERT_EQ('A', received[i]);
    }

    /* Send ACK */
    ssize_t written = write(accepted_fd, "ACK", 3);
    ASSERT_EQ(3, written);

    /* Wait for child */
    int status;
    waitpid(pid, &status, 0);
    ASSERT_TRUE(WIFEXITED(status));
    ASSERT_EQ(0, WEXITSTATUS(status));

    /* Cleanup */
    close(accepted_fd);
    close(listen_fd);
    socket_cleanup();
    cleanup_temp_dir();

    restore_xdg_runtime_dir();

    TEST_PASS();
}

/**
 * ==========================================================================
 * Error String Tests
 * ==========================================================================
 */

/**
 * Test: socket_error_string returns valid strings
 */
void test_error_strings(void) {
    TEST("test_error_strings");

    ASSERT_STR_CONTAINS(socket_error_string(SOCKET_OK), "success");
    ASSERT_STR_CONTAINS(socket_error_string(SOCKET_ERR_XDG_NOT_SET), "XDG_RUNTIME_DIR");
    ASSERT_STR_CONTAINS(socket_error_string(SOCKET_ERR_PATH_TOO_LONG), "too long");
    ASSERT_STR_CONTAINS(socket_error_string(SOCKET_ERR_MKDIR_FAILED), "directory");
    ASSERT_STR_CONTAINS(socket_error_string(SOCKET_ERR_SOCKET_FAILED), "socket");
    ASSERT_STR_CONTAINS(socket_error_string(SOCKET_ERR_BIND_FAILED), "bind");
    ASSERT_STR_CONTAINS(socket_error_string(SOCKET_ERR_LISTEN_FAILED), "listen");
    ASSERT_STR_CONTAINS(socket_error_string(SOCKET_ERR_CHMOD_FAILED), "permissions");
    ASSERT_STR_CONTAINS(socket_error_string(SOCKET_ERR_UNLINK_FAILED), "remove");
    ASSERT_STR_CONTAINS(socket_error_string(SOCKET_ERR_NULL_POINTER), "null");
    ASSERT_STR_CONTAINS(socket_error_string(SOCKET_ERR_AUTH_FAILED), "authentication");
    ASSERT_STR_CONTAINS(socket_error_string(SOCKET_ERR_AUTH_UID_MISMATCH), "UID");
    ASSERT_STR_CONTAINS(socket_error_string(SOCKET_ERR_INVALID_FD), "file descriptor");
    ASSERT_STR_CONTAINS(socket_error_string(SOCKET_ERR_TIMEOUT_FAILED), "timeout");
    ASSERT_STR_CONTAINS(socket_error_string(SOCKET_ERR_ACCEPT_FAILED), "accept");
    ASSERT_STR_CONTAINS(socket_error_string(SOCKET_ERR_NULL_HANDLER), "handler");
    ASSERT_STR_CONTAINS(socket_error_string(SOCKET_ERR_CONNECT_FAILED), "connect");

    /* Unknown error should not crash */
    const char *unknown = socket_error_string((socket_error_t)-999);
    ASSERT_TRUE(unknown != NULL);

    TEST_PASS();
}

/**
 * ==========================================================================
 * Main Test Runner
 * ==========================================================================
 */

int main(void) {
    printf("Running socket tests...\n\n");

    printf("=== Path Construction Tests ===\n");
    test_get_path_valid();
    test_get_path_xdg_not_set();
    test_get_path_xdg_empty();
    test_get_path_null_buffer();
    test_get_path_buffer_too_small();
    test_get_path_long_xdg();
    test_get_dir_path_valid();
    test_get_dir_path_null_buffer();

    printf("\n=== Socket Creation Tests ===\n");
    test_create_null_pointer();
    test_create_xdg_not_set();
    test_create_success();
    test_create_removes_stale_socket();

    printf("\n=== Socket Cleanup Tests ===\n");
    test_cleanup_removes_socket();
    test_cleanup_idempotent();
    test_cleanup_no_socket();

    printf("\n=== Authentication Tests ===\n");
    test_auth_invalid_fd();
    test_auth_same_uid_succeeds();
    test_auth_logs_at_debug_level();

    printf("\n=== Timeout Tests ===\n");
    test_set_timeouts_invalid_fd();
    test_set_timeouts_success();
    test_set_timeouts_zero_disables();

    printf("\n=== Shutdown Tests ===\n");
    test_shutdown_flag();

    printf("\n=== Accept Loop Tests ===\n");
    test_accept_loop_invalid_fd();
    test_accept_loop_null_handler();
    test_accept_loop_handles_shutdown();

    printf("\n=== Socket Connect Tests ===\n");
    test_connect_null_socket_path();
    test_connect_null_connected_fd();
    test_connect_path_too_long();
    test_connect_nonexistent_socket();
    test_connect_success();

    printf("\n=== Request/Response Loop Tests (Client Mode) ===\n");
    test_client_send_receive();
    test_client_multiple_requests();
    test_client_terminates_on_close();
    test_client_handles_partial_io();

    printf("\n=== Error String Tests ===\n");
    test_error_strings();

    printf("\n========================================\n");
    printf("Tests run: %d\n", tests_run);
    printf("Tests passed: %d\n", tests_passed);
    printf("Tests failed: %d\n", tests_run - tests_passed);
    printf("========================================\n");

    return (tests_run == tests_passed) ? 0 : 1;
}
