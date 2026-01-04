/**
 * Corpus Validation Test
 *
 * This test validates that all seed corpus files can be processed by
 * the decoder without crashing. This is useful when clang/libFuzzer
 * is not available for full fuzzing.
 *
 * Build:
 *   gcc -std=c99 -Wall -Wextra -fsanitize=address,undefined -g \
 *       -I../include -o test_corpus test_corpus.c ../src/protocol.c -lm
 *
 * Run:
 *   ./test_corpus corpus/
 */

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <stdint.h>
#include <dirent.h>
#include <sys/stat.h>
#include "weave/protocol.h"

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
 * Test one corpus file
 */
static int test_corpus_file(const char *path) {
    size_t len;
    uint8_t *data = read_file(path, &len);

    if (data == NULL && len == 0) {
        /* Empty file is valid (tests empty input case) */
        printf("  OK (empty)\n");
        return 0;
    }

    if (data == NULL) {
        printf("  FAIL (could not read file)\n");
        return -1;
    }

    /* Try to decode - we don't care about the result, just that it doesn't crash */
    sd35_generate_request_t req;
    error_code_t err = decode_generate_request(data, len, &req);

    /* Decode can fail (that's expected for invalid inputs), but shouldn't crash */
    printf("  OK (decoded with error_code=%d)\n", err);

    free(data);
    return 0;
}

int main(int argc, char **argv) {
    if (argc != 2) {
        fprintf(stderr, "Usage: %s <corpus_dir>\n", argv[0]);
        return 1;
    }

    const char *corpus_dir = argv[1];

    DIR *dir = opendir(corpus_dir);
    if (dir == NULL) {
        fprintf(stderr, "Failed to open directory: %s\n", corpus_dir);
        return 1;
    }

    printf("Testing corpus files in: %s\n\n", corpus_dir);

    int total = 0;
    int failed = 0;
    struct dirent *entry;

    while ((entry = readdir(dir)) != NULL) {
        /* Skip . and .. */
        if (strcmp(entry->d_name, ".") == 0 || strcmp(entry->d_name, "..") == 0) {
            continue;
        }

        char path[1024];
        snprintf(path, sizeof(path), "%s/%s", corpus_dir, entry->d_name);

        /* Skip directories */
        struct stat st;
        if (stat(path, &st) != 0 || S_ISDIR(st.st_mode)) {
            continue;
        }

        printf("Testing: %s\n", entry->d_name);
        if (test_corpus_file(path) != 0) {
            failed++;
        }
        total++;
    }

    closedir(dir);

    printf("\n========================================\n");
    printf("Corpus files tested: %d\n", total);
    printf("Failed: %d\n", failed);
    printf("Passed: %d\n", total - failed);
    printf("========================================\n");

    if (failed > 0) {
        printf("\nWARNING: Some corpus files could not be processed.\n");
        return 1;
    }

    printf("\nAll corpus files processed without crashes. Fuzzer ready.\n");
    return 0;
}
