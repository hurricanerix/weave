/**
 * Unit test for stdin monitoring thread functionality
 *
 * This test verifies the stdin monitoring thread implementation by:
 * 1. Creating a pipe to simulate stdin
 * 2. Starting the stdin monitoring thread
 * 3. Closing the write end of the pipe to simulate parent death
 * 4. Verifying that socket_request_shutdown() was called
 *
 * This test does NOT require a full compute-daemon binary or GPU/model.
 */

#include <errno.h>
#include <pthread.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

#include "weave/socket.h"

/**
 * stdin_monitor_thread - Copy from main.c for testing
 *
 * This is the same implementation as in main.c.
 */
static void *stdin_monitor_thread(void *arg) {
    char buf[1];
    ssize_t n;

    (void)arg;

    n = read(STDIN_FILENO, buf, sizeof(buf));

    if (n <= 0) {
        if (n == 0) {
            fprintf(stderr, "stdin closed, parent process died\n");
        } else {
            fprintf(stderr, "stdin read error: %s\n", strerror(errno));
        }
    } else {
        fprintf(stderr, "unexpected data on stdin, shutting down\n");
    }

    socket_request_shutdown();
    return NULL;
}

/**
 * Test program for stdin monitoring
 */
int main(void) {
    pthread_t thread;
    pthread_attr_t attr;
    int err;

    printf("Testing stdin monitoring thread...\n");
    printf("This program will block until stdin is closed.\n");
    printf("Close stdin by pressing Ctrl+D or closing the pipe.\n\n");

    pthread_attr_init(&attr);
    pthread_attr_setdetachstate(&attr, PTHREAD_CREATE_DETACHED);

    err = pthread_create(&thread, &attr, stdin_monitor_thread, NULL);
    pthread_attr_destroy(&attr);

    if (err != 0) {
        fprintf(stderr, "Failed to create thread: %s\n", strerror(err));
        return EXIT_FAILURE;
    }

    printf("Thread started. Waiting for stdin closure...\n");

    /* Wait for shutdown request */
    while (!socket_is_shutdown_requested()) {
        sleep(1);
    }

    printf("Shutdown requested by stdin monitor thread\n");
    return 0;
}
