/**
 * Stress Test for Protocol Decoder
 *
 * This test runs the decoder 1 million times on corpus files to simulate
 * fuzzing iterations. Useful when libFuzzer is not available but we need
 * to verify stability under repeated execution.
 *
 * Build:
 *   gcc -std=c99 -Wall -Wextra -fsanitize=address,undefined -O2 -g \
 *       -I../include -o stress_test stress_test.c ../src/protocol.c -lm
 *
 * Run:
 *   ./stress_test corpus/ 1000000
 */

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <stdint.h>
#include <time.h>
#include <dirent.h>
#include <sys/stat.h>
#include "weave/protocol.h"

#define MAX_CORPUS_FILES 100

typedef struct {
    uint8_t *data;
    size_t len;
    char name[256];
} corpus_file_t;

/**
 * Read file into buffer
 */
static uint8_t* read_file(const char *path, size_t *out_len) {
    FILE *f = fopen(path, "rb");
    if (f == NULL) {
        return NULL;
    }

    fseek(f, 0, SEEK_END);
    long size = ftell(f);
    fseek(f, 0, SEEK_SET);

    if (size < 0 || size > MAX_MESSAGE_SIZE) {
        fclose(f);
        return NULL;
    }

    uint8_t *buffer = malloc((size_t)size);
    if (buffer == NULL) {
        fclose(f);
        return NULL;
    }

    size_t read_bytes = fread(buffer, 1, (size_t)size, f);
    fclose(f);

    if (read_bytes != (size_t)size) {
        free(buffer);
        return NULL;
    }

    *out_len = (size_t)size;
    return buffer;
}

/**
 * Load all corpus files
 */
static int load_corpus(const char *corpus_dir, corpus_file_t *files, int *count) {
    DIR *dir = opendir(corpus_dir);
    if (dir == NULL) {
        return -1;
    }

    *count = 0;
    struct dirent *entry;

    while ((entry = readdir(dir)) != NULL && *count < MAX_CORPUS_FILES) {
        if (strcmp(entry->d_name, ".") == 0 || strcmp(entry->d_name, "..") == 0) {
            continue;
        }

        char path[1024];
        snprintf(path, sizeof(path), "%s/%s", corpus_dir, entry->d_name);

        struct stat st;
        if (stat(path, &st) != 0 || S_ISDIR(st.st_mode)) {
            continue;
        }

        size_t len;
        uint8_t *data = read_file(path, &len);
        if (data == NULL && len == 0) {
            /* Empty file - create a dummy entry */
            data = malloc(1);
            if (data == NULL) continue;
            len = 0;
        } else if (data == NULL) {
            continue;
        }

        files[*count].data = data;
        files[*count].len = len;
        strncpy(files[*count].name, entry->d_name, sizeof(files[*count].name) - 1);
        files[*count].name[sizeof(files[*count].name) - 1] = '\0';
        (*count)++;
    }

    closedir(dir);
    return 0;
}

/**
 * Free corpus files
 */
static void free_corpus(corpus_file_t *files, int count) {
    for (int i = 0; i < count; i++) {
        free(files[i].data);
    }
}

int main(int argc, char **argv) {
    if (argc != 3) {
        fprintf(stderr, "Usage: %s <corpus_dir> <iterations>\n", argv[0]);
        fprintf(stderr, "Example: %s corpus/ 1000000\n", argv[0]);
        return 1;
    }

    const char *corpus_dir = argv[1];
    long iterations = atol(argv[2]);

    if (iterations <= 0) {
        fprintf(stderr, "Invalid iteration count: %ld\n", iterations);
        return 1;
    }

    printf("Protocol Decoder Stress Test\n");
    printf("========================================\n");
    printf("Corpus directory: %s\n", corpus_dir);
    printf("Target iterations: %ld\n\n", iterations);

    /* Load corpus */
    corpus_file_t files[MAX_CORPUS_FILES];
    int file_count;

    printf("Loading corpus files...\n");
    if (load_corpus(corpus_dir, files, &file_count) != 0) {
        fprintf(stderr, "Failed to load corpus\n");
        return 1;
    }

    if (file_count == 0) {
        fprintf(stderr, "No corpus files found\n");
        return 1;
    }

    printf("Loaded %d corpus files\n\n", file_count);

    /* Run stress test */
    printf("Running decoder stress test...\n");
    printf("(This will take a while - press Ctrl+C to abort)\n\n");

    clock_t start = clock();
    long successful = 0;
    long failed = 0;

    for (long i = 0; i < iterations; i++) {
        /* Rotate through corpus files */
        int file_idx = (int)(i % file_count);
        corpus_file_t *file = &files[file_idx];

        /* Decode */
        sd35_generate_request_t req;
        error_code_t err = decode_generate_request(file->data, file->len, &req);

        if (err == ERR_NONE) {
            successful++;
        } else {
            failed++;
        }

        /* Progress report every 100k iterations */
        if ((i + 1) % 100000 == 0) {
            clock_t now = clock();
            double elapsed = (double)(now - start) / CLOCKS_PER_SEC;
            double rate = (i + 1) / elapsed;

            printf("Progress: %ld / %ld (%.1f%%) - %.0f exec/s\n",
                   i + 1, iterations, 100.0 * (i + 1) / iterations, rate);
        }
    }

    clock_t end = clock();
    double elapsed = (double)(end - start) / CLOCKS_PER_SEC;

    /* Report results */
    printf("\n========================================\n");
    printf("Stress Test Complete\n");
    printf("========================================\n");
    printf("Total iterations: %ld\n", iterations);
    printf("Successful decodes: %ld\n", successful);
    printf("Failed decodes: %ld\n", failed);
    printf("Time elapsed: %.2f seconds\n", elapsed);
    printf("Execution rate: %.0f exec/s\n", iterations / elapsed);
    printf("\n");

    if (elapsed > 0) {
        printf("No crashes detected!\n");
        printf("Decoder is stable under stress.\n");
    }

    free_corpus(files, file_count);
    return 0;
}
