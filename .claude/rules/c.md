---
paths:
  - "**/*.c"
  - "**/*.h"
  - "**/Makefile"
---

# C Language Rules for Weave

## Philosophy

We write C because Go can't give us the performance we need. Every line of C is a liability. Make it count.

**Priorities:**
1. Correctness (no UB, ever)
2. Safety (memory, concurrency, input validation)
3. Performance (why we're here)
4. Simplicity (minimize complexity)

## Language Standard

### Version
- **C99** strictly
- NOT C89 (too old)
- NOT C11 (unnecessary features)
- NOT C++ (too much hidden behavior)

**Why C99?**
- Widely supported
- VLAs if needed (use sparingly)
- `//` comments
- `<stdbool.h>`, `<stdint.h>`
- Inline functions

### Compiler
- **Primary**: GCC 11+ or Clang 14+
- **Flags**: `-std=c99 -Wall -Wextra -Werror -pedantic`
- **Optimization**: `-O2` for production, `-O0 -g` for debug

## Project Structure

```
compute/
├── src/
│   ├── main.c          # Daemon entry point
│   ├── protocol.c      # Protocol parsing
│   ├── auth.c          # Authentication
│   └── compute.c       # Core compute logic
├── include/
│   └── weave/
│       ├── protocol.h
│       ├── auth.h
│       └── compute.h
├── cores/
│   ├── cuda/           # CUDA backend
│   ├── rocm/           # ROCm backend
│   └── cpu/            # CPU fallback
├── test/
│   └── test_*.c        # Unit tests
├── bench/
│   └── bench_*.c       # Performance benchmarks
└── Makefile
```

## Code Style

### Formatting
- **Indentation**: 4 spaces (NO tabs)
- **Braces**: K&R style (opening brace on same line)
- **Line length**: 100 characters max
- **Use clang-format** with provided `.clang-format`

```c
// ✅ GOOD
if (condition) {
    do_something();
}

// ❌ BAD
if (condition)
{
    do_something();
}
```

### Naming

**Functions**: `snake_case`
```c
void compute_generate(struct compute_ctx *ctx);
```

**Types**: `snake_case` with `_t` suffix
```c
typedef struct compute_ctx compute_ctx_t;
typedef enum status status_t;
```

**Constants**: `SCREAMING_SNAKE_CASE`
```c
#define MAX_PROMPT_LENGTH 2048
#define DEFAULT_TIMEOUT_MS 30000
```

**Macros**: `SCREAMING_SNAKE_CASE`
```c
#define MIN(a, b) ((a) < (b) ? (a) : (b))
```

**Variables**: `snake_case`
```c
int socket_fd;
size_t buffer_size;
```

### Header Guards

Use `#pragma once` (simpler, less error-prone):

```c
#pragma once

// header content
```

NOT:
```c
#ifndef WEAVE_PROTOCOL_H
#define WEAVE_PROTOCOL_H
// ...
#endif
```

## Memory Management

### Allocation

**Every allocation must be checked:**

```c
// ✅ GOOD
void *ptr = malloc(size);
if (ptr == NULL) {
    return ERR_OUT_OF_MEMORY;
}

// ❌ BAD - Unchecked allocation
void *ptr = malloc(size);
ptr[0] = 42;  // Might segfault
```

**Match allocations with deallocations:**

```c
// ✅ GOOD
char *buffer = malloc(1024);
if (buffer == NULL) return ERR_OOM;
// ... use buffer ...
free(buffer);
buffer = NULL;  // Prevent use-after-free

// ❌ BAD - Memory leak
char *buffer = malloc(1024);
return;  // Leaked!
```

### Ownership

**Be explicit about ownership:**

```c
// Caller owns returned pointer (must free)
char *create_buffer(size_t size);

// Function takes ownership (will free internally)
void consume_buffer(char *buffer);

// Function borrows (caller still owns)
int process_buffer(const char *buffer);
```

Document ownership in comments:

```c
/**
 * Creates a new buffer. Caller must free with free().
 * Returns NULL on allocation failure.
 */
char *create_buffer(size_t size);
```

### Common Patterns

**RAII-style cleanup with goto:**

```c
int process_file(const char *path) {
    int fd = -1;
    char *buffer = NULL;
    int result = -1;
    
    fd = open(path, O_RDONLY);
    if (fd < 0) {
        goto cleanup;
    }
    
    buffer = malloc(4096);
    if (buffer == NULL) {
        goto cleanup;
    }
    
    // ... do work ...
    
    result = 0;  // Success
    
cleanup:
    if (buffer != NULL) {
        free(buffer);
    }
    if (fd >= 0) {
        close(fd);
    }
    return result;
}
```

## Safety

### Bounds Checking

**Always check array bounds:**

```c
// ✅ GOOD
if (index < array_size) {
    array[index] = value;
} else {
    return ERR_OUT_OF_BOUNDS;
}

// ❌ BAD - Buffer overflow
array[index] = value;
```

### String Handling

**Use safe string functions:**

```c
// ✅ GOOD
strncpy(dest, src, sizeof(dest) - 1);
dest[sizeof(dest) - 1] = '\0';  // Ensure null termination

// ❌ BAD - Buffer overflow risk
strcpy(dest, src);
```

**Better: Use explicit sizes:**

```c
size_t copy_string(char *dest, size_t dest_size, const char *src) {
    size_t src_len = strlen(src);
    if (src_len >= dest_size) {
        return 0;  // Won't fit
    }
    memcpy(dest, src, src_len + 1);  // Include null terminator
    return src_len;
}
```

### Integer Overflow

**Check for overflow in arithmetic:**

```c
// ✅ GOOD
if (a > SIZE_MAX - b) {
    return ERR_OVERFLOW;
}
size_t result = a + b;

// ❌ BAD - Overflow undefined behavior
size_t result = a + b;  // Might wrap
```

### Input Validation

**Validate all external input:**

```c
int validate_prompt(const char *prompt, size_t len) {
    if (prompt == NULL) {
        return ERR_NULL_POINTER;
    }
    if (len == 0 || len > MAX_PROMPT_LENGTH) {
        return ERR_INVALID_LENGTH;
    }
    // Check for valid UTF-8, no null bytes, etc.
    return 0;
}
```

## Undefined Behavior

**Never invoke UB. Period.**

Common UB to avoid:

❌ Signed integer overflow  
❌ Dereferencing NULL  
❌ Out-of-bounds access  
❌ Use-after-free  
❌ Double-free  
❌ Uninitialized reads  
❌ Data races  
❌ Shifting by negative or >= width  
❌ Division by zero

**Use tools to catch UB:**
- AddressSanitizer: `-fsanitize=address`
- UndefinedBehaviorSanitizer: `-fsanitize=undefined`
- Valgrind: `valgrind --leak-check=full`

## Error Handling

### Error Codes

**Return error codes, not magic values:**

```c
// ✅ GOOD
typedef enum {
    OK = 0,
    ERR_INVALID_PARAM = -1,
    ERR_OUT_OF_MEMORY = -2,
    ERR_SOCKET_CLOSED = -3,
    // ...
} error_code_t;

error_code_t do_something(void);

// ❌ BAD
int do_something(void);  // What does -1 mean?
```

### Error Propagation

**Propagate errors explicitly:**

```c
error_code_t process_request(request_t *req, response_t *resp) {
    error_code_t err;
    
    err = validate_request(req);
    if (err != OK) {
        return err;
    }
    
    err = compute_generate(req->prompt, &resp->image);
    if (err != OK) {
        return err;
    }
    
    return OK;
}
```

### Assertions

**Use assertions for invariants:**

```c
#include <assert.h>

void process_buffer(const char *buffer, size_t size) {
    assert(buffer != NULL);  // Programmer error if NULL
    assert(size > 0);
    
    // ... process ...
}
```

**NOT for runtime errors:**

```c
// ❌ BAD - User-controlled input
assert(prompt_length < MAX_PROMPT_LENGTH);

// ✅ GOOD
if (prompt_length >= MAX_PROMPT_LENGTH) {
    return ERR_PROMPT_TOO_LONG;
}
```

## Performance

### Profiling First

**Never optimize without profiling.**

Tools:
- `perf` (Linux perf events)
- `gprof` (GNU profiler)
- `valgrind --tool=callgrind`
- Custom instrumentation

### Hot Path Optimization

**Measure these in benchmarks:**

```c
// Example benchmark output
// Min: 1.2ms, Max: 2.3ms, Avg: 1.5ms, Median: 1.4ms
// Cache misses: 1234
// Allocations: 5 (2048 bytes total)
```

Track:
- ✅ Execution time (min/max/avg/median)
- ✅ Cache misses (L1, L2, L3)
- ✅ Allocation count and size
- ✅ CPU cycles
- ✅ Branch mispredictions

### Memory Layout

**Consider cache lines (64 bytes):**

```c
// ✅ GOOD - Hot data together
struct compute_ctx {
    // Hot path (64 bytes)
    void *gpu_context;
    uint32_t state;
    uint32_t flags;
    // ... frequently accessed fields ...
    
    // Cold path
    char metadata[256];
};

// ❌ BAD - False sharing
struct counter {
    uint64_t count1;  // Different threads
    uint64_t count2;  // Same cache line!
};
```

### Inlining

**Use `inline` for small, hot functions:**

```c
static inline int min(int a, int b) {
    return a < b ? a : b;
}
```

**But profile to verify it helps!**

## Testing

### Unit Tests

Write tests that look like unit tests:

```c
// test/test_protocol.c

void test_encode_valid_request(void) {
    request_t req = {
        .prompt = "test",
        .prompt_len = 4,
    };
    
    uint8_t buffer[1024];
    size_t encoded_len;
    
    int err = protocol_encode(&req, buffer, sizeof(buffer), &encoded_len);
    
    assert(err == OK);
    assert(encoded_len > 0);
    assert(encoded_len < sizeof(buffer));
}

void test_encode_null_prompt(void) {
    request_t req = {
        .prompt = NULL,
        .prompt_len = 0,
    };
    
    uint8_t buffer[1024];
    size_t encoded_len;
    
    int err = protocol_encode(&req, buffer, sizeof(buffer), &encoded_len);
    
    assert(err == ERR_NULL_POINTER);
}

int main(void) {
    test_encode_valid_request();
    test_encode_null_prompt();
    printf("All tests passed\n");
    return 0;
}
```

### Benchmark Tests

Capture detailed profiling data:

```c
// bench/bench_protocol.c

void bench_encode(int iterations) {
    request_t req = {.prompt = "test prompt", .prompt_len = 11};
    uint8_t buffer[1024];
    size_t encoded_len;
    
    uint64_t times[iterations];
    uint64_t start, end;
    
    for (int i = 0; i < iterations; i++) {
        start = rdtsc();  // Read timestamp counter
        protocol_encode(&req, buffer, sizeof(buffer), &encoded_len);
        end = rdtsc();
        times[i] = end - start;
    }
    
    // Calculate stats
    uint64_t min = times[0], max = times[0], sum = 0;
    for (int i = 0; i < iterations; i++) {
        if (times[i] < min) min = times[i];
        if (times[i] > max) max = times[i];
        sum += times[i];
    }
    
    printf("Iterations: %d\n", iterations);
    printf("Min: %lu cycles\n", min);
    printf("Max: %lu cycles\n", max);
    printf("Avg: %lu cycles\n", sum / iterations);
    // ... median, cache misses, etc.
}
```

## Build System

### Makefile

Standard targets:

```makefile
CC = gcc
CFLAGS = -std=c99 -Wall -Wextra -Werror -pedantic -O2
INCLUDES = -Iinclude
LDFLAGS = -lcuda -lpthread

SRC = $(wildcard src/*.c)
OBJ = $(SRC:.c=.o)

.PHONY: all
all: weave-compute

weave-compute: $(OBJ)
	$(CC) $(CFLAGS) -o $@ $^ $(LDFLAGS)

.PHONY: test
test: CFLAGS += -O0 -g
test: test/test_protocol
	./test/test_protocol

.PHONY: bench
bench: bench/bench_protocol
	./bench/bench_protocol 10000

.PHONY: valgrind
valgrind: test
	valgrind --leak-check=full --show-leak-kinds=all ./test/test_protocol

.PHONY: clean
clean:
	rm -f $(OBJ) weave-compute test/*.o bench/*.o

.PHONY: fmt
fmt:
	clang-format -i src/*.c include/**/*.h
```

## Documentation

### Function Comments

Use Doxygen-style comments:

```c
/**
 * Encodes a request into the binary protocol format.
 *
 * @param req       Pointer to request structure (must not be NULL)
 * @param buffer    Output buffer for encoded data
 * @param buf_size  Size of output buffer in bytes
 * @param out_len   Pointer to store actual encoded length
 * @return          OK on success, error code on failure
 *
 * @note Caller must ensure buffer is large enough
 * @warning This function is NOT thread-safe
 */
int protocol_encode(const request_t *req, uint8_t *buffer, 
                    size_t buf_size, size_t *out_len);
```

## Temporary Files

**Always use `./tmp/` (project-local), never `/tmp/`.**

```c
#include <sys/stat.h>
#include <stdio.h>

// ✅ GOOD - Project-local temp directory
mkdir("./tmp", 0755);  // Create if doesn't exist (ignore EEXIST)
FILE *f = fopen("./tmp/workfile.tmp", "wb");

// ❌ BAD - System temp directory
FILE *f = fopen("/tmp/workfile.tmp", "wb");
char *path = tmpnam(NULL);  // Uses system temp
```

**Why:**
- Keeps test artifacts contained to the project
- Easier cleanup
- Avoids permission issues in sandboxed environments (Flatpak)
- Project `.gitignore` already ignores `./tmp/`

## Anti-Patterns

❌ **VLAs on the stack** - Use heap or fixed size  
❌ **`gets()`** - Buffer overflow nightmare  
❌ **Unchecked allocations** - Always check malloc  
❌ **Magic numbers** - Use named constants  
❌ **Global mutable state** - Pass context explicitly  
❌ **`void *` everywhere** - Use typed pointers  
❌ **Macros for functions** - Use inline functions  
❌ **`typedef struct { } name_t;`** - Forward declare instead

## When in Doubt

1. Run Valgrind (must be clean)
2. Run AddressSanitizer
3. Run UBSanitizer  
4. Check for compiler warnings
5. Ask: "Would I trust this code with root privileges?"

If any tool complains, fix it. No exceptions.
