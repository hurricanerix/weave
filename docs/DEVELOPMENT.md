# Development Guide

This document covers development workflows, testing, and debugging for Weave.

## Project Structure

```
weave/
├── backend/            # Go backend service
│   ├── cmd/weave/      # Main application
│   ├── internal/       # Go internal packages
│   ├── test/integration/ # Integration tests
│   ├── go.mod
│   └── go.sum
├── compute/            # C GPU compute component
│   ├── src/            # C source files
│   ├── include/        # C header files
│   ├── test/           # Unit tests
│   └── fuzz/           # Fuzzing infrastructure
├── electron/           # Electron desktop shell
├── packaging/          # Distribution packaging (Flatpak, etc.)
└── docs/               # Documentation
```

The application runs as an Electron app that launches the Go backend, which spawns the C compute process. For development, you can also run weave directly and access the UI via browser.

## Building

Build everything and run:
```bash
make run
```

Or build individual components:
```bash
make backend
make compute
make electron
```

For Flatpak distribution:
```bash
make flatpak
make flatpak-install
```

Clean build artifacts:
```bash
make clean
```

Note: Electron build requires npm dependencies. If not installed:
```bash
cd electron && npm install
```

## Running the application

The recommended way to run Weave is through Electron:
```bash
make run
```

This builds all components and launches the desktop application.

### Development mode

For development, you can run the Go backend directly and access the UI via browser:
```bash
ollama serve              # Start ollama (required for now)
./build/weave-backend     # Start backend (spawns compute automatically)
```

Then open `http://localhost:8080` in your browser.

### Current requirements

- **ollama**: Required for LLM inference (temporary - will be replaced by unified compute layer)

### CLI flags

The weave-backend binary accepts the following command-line flags:

```
--port <PORT>              HTTP server port (default: 8080)
--steps <STEPS>            Number of inference steps (default: 4)
--cfg <CFG>                CFG scale (default: 1.0)
--width <WIDTH>            Image width in pixels (default: 1024)
--height <HEIGHT>          Image height in pixels (default: 1024)
--seed <SEED>              Image generation seed, -1 = random (default: -1)
--llm-seed <SEED>          LLM seed for deterministic responses, 0 = random (default: 0)
--ollama-url <URL>         Ollama API endpoint (default: http://localhost:11434)
--ollama-model <MODEL>     Ollama model name (default: mistral:7b)
--log-level <LEVEL>        Log level: debug, info, warn, error (default: info)
--help                     Show help message
--version                  Show version information
```

### Examples

Start with defaults:
```bash
./build/weave-backend
```

Use custom port:
```bash
./build/weave-backend --port 3000
```

Use deterministic generation:
```bash
./build/weave-backend --seed 42 --llm-seed 123
```

Use different ollama model:
```bash
./build/weave-backend --ollama-model llama3.2:3b
```

Enable debug logging:
```bash
./build/weave-backend --log-level debug
```

### Startup validation

On startup, the backend performs the following:

1. **Ollama connectivity**: HTTP GET to `<ollama-url>/api/tags`
   - If ollama is not running, exits with error and prompts: `ollama serve`

2. **Ollama model availability**: Checks that the configured model exists
   - If model is missing, exits with error and prompts: `ollama pull <model>`

3. **Compute process**: Creates Unix socket and spawns the compute process
   - If spawn fails, exits with error
   - Waits for compute to connect (10 second timeout)

All validation happens before the HTTP server starts listening.

### Startup output

Successful startup:
```
2026/01/04 11:00:00 [INFO] Starting weave...
2026/01/04 11:00:00 [INFO] Connected to ollama at http://localhost:11434 (model: mistral:7b)
2026/01/04 11:00:00 [INFO] Created socket at /run/user/1000/weave/weave.sock
2026/01/04 11:00:00 [INFO] Spawned weave-compute process (PID: 12345)
2026/01/04 11:00:00 [INFO] Accepted connection from weave-compute process
2026/01/04 11:00:00 [INFO] Listening on http://localhost:8080
```

With debug logging (`--log-level debug`):
```
2026/01/04 11:00:00 [INFO] Starting weave...
2026/01/04 11:00:00 [DEBUG] Configuration: port=8080, steps=4, cfg=1.0, width=1024, height=1024, seed=-1, llm-seed=0
2026/01/04 11:00:00 [DEBUG] Ollama: url=http://localhost:11434, model=mistral:7b
2026/01/04 11:00:00 [DEBUG] Log level: debug
2026/01/04 11:00:00 [DEBUG] Validating ollama connection...
2026/01/04 11:00:00 [INFO] Connected to ollama at http://localhost:11434 (model: mistral:7b)
2026/01/04 11:00:00 [DEBUG] Creating socket for weave-compute...
2026/01/04 11:00:00 [INFO] Created socket at /run/user/1000/weave/weave.sock
2026/01/04 11:00:00 [DEBUG] Spawning weave-compute process...
2026/01/04 11:00:00 [INFO] Spawned weave-compute process (PID: 12345)
2026/01/04 11:00:00 [DEBUG] Waiting for compute process to connect...
2026/01/04 11:00:00 [INFO] Accepted connection from weave-compute process
2026/01/04 11:00:00 [DEBUG] Initializing components...
2026/01/04 11:00:00 [INFO] Listening on http://localhost:8080
```

### Graceful shutdown

Weave handles SIGTERM and SIGINT signals for graceful shutdown:

```bash
# Start weave
./build/weave-backend

# In another terminal, send SIGTERM
kill <pid>

# Or press Ctrl+C in the weave terminal
```

On shutdown:
```
^C2026/01/04 11:05:00 Shutting down web server...
2026/01/04 11:05:00 Web server stopped
```

The shutdown process:
1. Stops accepting new HTTP connections
2. Waits for in-flight requests to complete (timeout: 30 seconds)
3. Closes compute client connections
4. Exits with code 0 on clean shutdown

### Troubleshooting startup errors

#### Error: "ollama not running at http://localhost:11434"

**Cause**: ollama is not running or not reachable at the configured URL.

**Solution**:
1. Start ollama: `ollama serve`
2. Verify it's running: `curl http://localhost:11434/api/tags`
3. If using a custom URL, ensure `--ollama-url` matches where ollama is running

#### Error: "model not available in ollama: mistral:7b"

**Cause**: The configured model has not been pulled.

**Solution**:
1. Pull the model: `ollama pull mistral:7b`
2. List available models: `ollama list`
3. Use an available model with `--ollama-model <name>`

#### Error: "failed to spawn compute process"

**Cause**: The compute binary could not be started.

**Solution**:
1. Ensure compute is built: `make compute`
2. Check the binary exists: `ls -la compute/weave-compute`
3. Check for missing libraries: `ldd compute/weave-compute`

#### Error: "failed to accept compute connection"

**Cause**: Compute process started but failed to connect within timeout.

**Solution**:
1. Check compute process logs for errors
2. Ensure no firewall or security software blocking Unix sockets
3. Try running with debug logging: `./build/weave-backend --log-level debug`

#### Error: "XDG_RUNTIME_DIR not set"

**Cause**: XDG_RUNTIME_DIR environment variable is not set.

**Solution**:
1. Check your system's runtime directory: `echo $XDG_RUNTIME_DIR`
2. On most Linux systems, this is set automatically
3. If missing, set it manually: `export XDG_RUNTIME_DIR=/run/user/$(id -u)`

#### Error: "port already in use" or "bind: address already in use"

**Cause**: Another process is using port 8080.

**Solution**:
1. Use a different port: `./build/weave-backend --port 3000`
2. Find process using port: `lsof -i :8080`
3. Kill the process or choose a different port

## Unix Socket Communication

The Go backend and C compute process communicate via Unix domain sockets with SO_PEERCRED authentication.

### Socket path

The socket is located at:

```
$XDG_RUNTIME_DIR/weave/weave.sock
```

On most Linux systems, `XDG_RUNTIME_DIR` is `/run/user/<uid>`. For example:

```bash
# Check your runtime directory
echo $XDG_RUNTIME_DIR
# Typically: /run/user/1000

# Full socket path
ls -la $XDG_RUNTIME_DIR/weave/weave.sock
```

**Directory structure:**
- `$XDG_RUNTIME_DIR/weave/` - Directory with mode 0700 (owner only)
- `$XDG_RUNTIME_DIR/weave/weave.sock` - Socket file with mode 0600 (owner read/write only)

### How it works

In normal operation, the backend:
1. Creates the socket at `$XDG_RUNTIME_DIR/weave/weave.sock`
2. Spawns the compute process
3. Accepts the connection from compute

For development or debugging, you can run compute manually:

```bash
# Build compute
make compute

# Run manually (it will connect to an existing socket)
./compute/weave-compute --socket $XDG_RUNTIME_DIR/weave/weave.sock
```

### Integration tests

```bash
cd backend
go test -tags=integration -v ./test/integration/...
```

### Socket Authentication (SO_PEERCRED)

The compute process authenticates every connection using `SO_PEERCRED`, which provides kernel-verified credentials of the connecting process. Only processes running as the same user (UID) as the compute process are allowed to connect.

**How it works:**
1. Client connects to socket
2. Compute process calls `getsockopt(SO_PEERCRED)` to get client's UID/PID
3. If client UID matches compute process UID, connection is accepted
4. If UIDs differ, connection is closed immediately with no response

**Testing authentication rejection:**

To verify that different UIDs are rejected, use `sudo -u` to run as a different user:

```bash
# Terminal 1: Start compute process as your user
./weave-compute

# Terminal 2: Try to connect as a different user
sudo -u nobody socat - UNIX-CONNECT:$XDG_RUNTIME_DIR/weave/weave.sock
# Connection will be closed immediately with no response

# Check compute process logs (if DEBUG logging enabled)
# [socket] DEBUG: auth rejected: client uid=65534 pid=12345 (expected uid=1000)
```

**Testing with Go client:**

```bash
# As your normal user - should succeed
cd backend
go run ./cmd/test-connect/

# As a different user - should fail silently (connection closed)
sudo -u nobody go run ./cmd/test-connect/
```

### Log Levels and DEBUG Logging

The socket module supports configurable log levels. By default, authentication rejections are logged at DEBUG level only (silent).

**Log levels:**
| Level | Value | Description |
|-------|-------|-------------|
| DEBUG | 0 | Verbose logging including auth rejections |
| INFO | 1 | Normal operational messages (default) |
| WARN | 2 | Warning conditions |
| ERROR | 3 | Error conditions |
| NONE | 4 | Disable all logging |

**Enabling DEBUG logging in C:**

```c
#include "weave/socket.h"

// Enable debug logging to see auth rejections
socket_set_log_level(SOCKET_LOG_DEBUG);

// Custom log handler (optional)
void my_log_handler(socket_log_level_t level, const char *message) {
    fprintf(stderr, "[%d] %s\n", level, message);
}
socket_set_log_callback(my_log_handler);
```

**Example DEBUG output:**

```
[socket] DEBUG: auth accepted: client uid=1000 pid=12345
[socket] DEBUG: auth rejected: client uid=65534 pid=12346 (expected uid=1000)
[socket] INFO: accept loop started
[socket] INFO: accept loop stopped (shutdown requested)
```

### Graceful Shutdown

The compute process handles SIGTERM and SIGINT for graceful shutdown:

```bash
# Start compute process
./weave-compute &
COMPUTE_PID=$!

# Send SIGTERM to shutdown gracefully
kill $COMPUTE_PID

# Expected output:
# shutting down gracefully
# weave-compute stopped

# Verify socket file is removed
ls $XDG_RUNTIME_DIR/weave/weave.sock
# ls: cannot access '...': No such file or directory
```

### Socket Timeouts

**Connection timeout (client):** 5 seconds
**Read timeout (client):** 65 seconds (slightly longer than compute process's 60s)
**Write timeout (compute process):** 5 seconds
**Read timeout (compute process):** 60 seconds

### Troubleshooting Socket Issues

**XDG_RUNTIME_DIR not set**

```
Error: XDG_RUNTIME_DIR not set
```

Solution:
```bash
# Check if set
echo $XDG_RUNTIME_DIR

# If not set, set it manually (for testing)
export XDG_RUNTIME_DIR=/run/user/$(id -u)

# Or create a temp directory
export XDG_RUNTIME_DIR=$(mktemp -d)
```

**Socket not found (ENOENT)**

If running compute manually and the socket doesn't exist:
```bash
# Check if socket exists
ls -la $XDG_RUNTIME_DIR/weave/weave.sock

# The backend must be running first to create the socket
./build/weave-backend
```

**Connection refused (ECONNREFUSED)**

This indicates a stale socket file:
```bash
# Remove stale socket
rm -f $XDG_RUNTIME_DIR/weave/weave.sock

# Restart the backend
./build/weave-backend
```

**Permission denied**

```
Error: permission denied
```

Check socket and directory permissions:
```bash
# Directory should be 0700 (owner only)
ls -ld $XDG_RUNTIME_DIR/weave/
# drwx------ 2 user user ...

# Socket should be 0600 (owner read/write)
ls -l $XDG_RUNTIME_DIR/weave/weave.sock
# srw------- 1 user user ...
```

**Authentication failures (silent)**

If connections are rejected silently, enable DEBUG logging:
```c
socket_set_log_level(SOCKET_LOG_DEBUG);
```

Then check logs for rejection messages:
```
[socket] DEBUG: auth rejected: client uid=65534 pid=12345 (expected uid=1000)
```

**Connection timeout**

```
Error: weave-compute process connection timeout
```

The compute process may be overloaded or hung. Check:
```bash
# Is compute process running?
pgrep weave-compute

# Check compute process logs for errors
# Restart if necessary
```

## Testing

### Protocol Testing

The binary protocol is the critical interface between Go and C components. Comprehensive testing ensures correctness across this boundary.

#### Go Protocol Unit Tests

Test Go encoder and decoder in isolation:

```bash
# Run all protocol package tests
cd backend
go test ./internal/protocol/...

# Verbose output
go test -v ./internal/protocol/...

# Run specific test
go test -v ./internal/protocol/... -run TestEncodeSD35GenerateRequest
```

Expected output:
```
PASS
ok  	github.com/hurricanerix/weave/internal/protocol	0.003s
```

The Go tests verify:
- Encoding produces correct binary format
- Decoding handles all valid inputs
- Validation catches invalid parameters
- Error conditions are handled properly

#### C Protocol Unit Tests

Test C decoder and encoder in isolation:

```bash
cd compute

# Build and run tests
make test

# Expected output:
# Running unit tests...
# [test_header_decode] PASS
# [test_request_decode_valid] PASS
# ...
# All tests passed.
```

The C tests verify:
- Decoding handles all valid protocol messages
- Encoding produces correct binary format
- Bounds checking prevents buffer overflows
- Invalid input is rejected safely

#### Integration Tests

Integration tests verify the complete round-trip: Go encodes → C decodes → C encodes → Go decodes.

**Prerequisites:**

Build the C stub generator first:
```bash
cd compute
make test-stub

# Verify it was built
ls -la test/test_stub_generator
```

The stub generator is a minimal C program that:
- Reads protocol requests from stdin
- Decodes using the C implementation
- Generates a test pattern image
- Encodes response using the C implementation
- Writes to stdout

**Run integration tests:**

```bash
# From project root
cd backend
go test -v -tags=integration ./test/integration/

# Run specific test
go test -v -tags=integration ./test/integration/ -run TestProtocolRoundTrip_512x512_RGB
```

Expected output:
```
=== RUN   TestProtocolRoundTrip_64x64_RGB
--- PASS: TestProtocolRoundTrip_64x64_RGB (0.01s)
=== RUN   TestProtocolRoundTrip_512x512_RGB
--- PASS: TestProtocolRoundTrip_512x512_RGB (0.02s)
...
PASS
ok  	github.com/hurricanerix/weave/test/integration	0.123s
```

Integration tests verify:
- Request ID is echoed correctly
- Image dimensions match request
- Pixel data integrity across boundary
- Multiple sequential requests work

See `test/integration/README.md` for details on the test architecture.

#### Fuzzing

See the [Fuzzing](#fuzzing) section below for protocol fuzzing with libFuzzer.

#### Protocol Testing Checklist

When modifying the protocol, run all tests in order:

1. `cd backend && go test ./internal/protocol/...` - Go unit tests
2. `cd compute && make test` - C unit tests
3. `cd compute && make test-asan` - C with sanitizers
4. `cd backend && go test -tags=integration ./test/integration/` - Round-trip tests
5. `cd compute && make test-corpus` - Corpus validation
6. `cd compute && make fuzz` - 60-second fuzz run

All must pass before committing protocol changes.

### Unit Tests

**Go:**
```bash
cd backend
go test ./...
```

**C:**
```bash
cd compute
make test
```

### Sanitizer Tests

Run C code with AddressSanitizer and UndefinedBehaviorSanitizer:

```bash
cd compute
make test-asan
```

This detects:
- Buffer overflows
- Use-after-free
- Memory leaks
- Integer overflow
- Null pointer dereferences
- Undefined behavior

All sanitizer tests must pass before committing.

## Fuzzing

Fuzzing tests the protocol decoder with millions of random inputs to find crashes and undefined behavior.

### Quick Test (10 seconds)

Uses gcc/ASan to validate all corpus files:

```bash
cd compute
make test-corpus
```

This runs the decoder on all seed inputs with sanitizers enabled. Useful when clang is not available.

### Full Fuzzing (requires clang)

**Install clang:**
```bash
# Ubuntu/Debian
sudo apt-get install clang

# Fedora
sudo dnf install clang
```

**Run fuzzer for 60 seconds:**
```bash
cd compute
make fuzz
```

**Extended fuzzing (1 hour):**
```bash
make fuzz-long
```

**See `compute/fuzz/README.md` for detailed fuzzing documentation.**

### Expected Results

Fuzzing should complete without crashes:

```
INFO: -max_total_time=60 seconds reached
#1234567 DONE   cov: 245 ft: 789 corp: 52/3456b
```

If crashes are found:
1. The crashing input is saved to `fuzz/crash-<hash>`
2. Review the AddressSanitizer output
3. Fix the bug before proceeding
4. Re-run fuzzer to verify fix

### Continuous Fuzzing

For critical code paths, run fuzzing overnight or continuously in CI:

```bash
# 8 hour run (overnight)
cd compute && cd fuzz
./fuzz_protocol corpus/ -max_total_time=28800 -jobs=4
```

## Code Quality

### Formatting

**Go:**
```bash
cd backend
go fmt ./...
goimports -w .
```

**C:**
```bash
cd compute
make fmt
```

Uses `clang-format` with project-specific style.

### Linting

**Go:**
```bash
cd backend
golangci-lint run
```

**C:**
```bash
# Static analysis with clang-tidy (if available)
clang-tidy src/*.c -- -I./include
```

## Debugging

### C Debugging with GDB

```bash
cd compute
gcc -O0 -g -I./include -o debug_program src/protocol.c test/test_protocol.c -lm
gdb ./debug_program

# In GDB
(gdb) break decode_generate_request
(gdb) run
(gdb) print req
(gdb) x/32xb data
```

### Memory Debugging with Valgrind

```bash
cd compute
make test-asan  # First try ASan (faster)

# Or use Valgrind (slower but thorough)
gcc -O0 -g -I./include -o test_valgrind test/test_protocol.c src/protocol.c -lm
valgrind --leak-check=full --show-leak-kinds=all ./test_valgrind
```

### Debugging Protocol Messages

Use `hexdump` to inspect binary protocol messages:

```bash
# View message in hex
hexdump -C message.bin

# Or use test stub generator
cd compute
make test-stub
./test/test_stub_generator > test_message.bin
hexdump -C test_message.bin
```

## Performance Profiling

### Go Profiling

```bash
cd backend
# CPU profiling
go test -cpuprofile=cpu.prof -bench=.
go tool pprof cpu.prof

# Memory profiling
go test -memprofile=mem.prof -bench=.
go tool pprof mem.prof
```

### C Profiling

```bash
# Build with profiling
gcc -pg -O2 -I./include -o profiled src/protocol.c bench/bench_protocol.c -lm
./profiled
gprof profiled gmon.out > analysis.txt
```

## Benchmarking

### Go Benchmarks

```bash
cd backend
go test -bench=. -benchmem ./...
```

### C Benchmarks

```bash
cd compute
make bench  # If benchmark target exists
```

Expected performance for protocol operations:
- Encode request: <1us
- Decode request: <2us
- Validate parameters: <100ns

## Common Workflows

### Adding a New Protocol Field

1. Update protocol specification in `docs/protocol/SPEC_SD35.md`
2. Update C types in `compute/include/weave/protocol.h`
3. Update decoder in `compute/src/protocol.c`
4. Add tests in `compute/test/test_protocol.c`
5. Update Go encoder/decoder in `backend/internal/protocol/`
6. Add integration test
7. Run fuzzer for 1 hour to verify stability

### Fixing a Bug

1. Write a failing test that reproduces the bug
2. Run test with ASan/UBSan to identify root cause
3. Fix the bug
4. Verify test passes
5. Run fuzzer to ensure no new issues
6. Commit test and fix together

### Performance Optimization

1. Profile first - identify bottleneck
2. Write benchmark to measure current performance
3. Optimize
4. Verify benchmark improvement
5. Verify correctness with tests and fuzzer
6. Document optimization in commit message

## Troubleshooting

### Protocol Testing Issues

**Integration tests fail: "stub generator not found"**

```
protocol_roundtrip_test.go:158: stub generator failed: stub generator not found
```

Solution:
```bash
cd compute
make test-stub
ls -la test/test_stub_generator  # Verify it exists
```

**Fuzzer fails: "clang not found"**

```
Error: clang not found. Install with: sudo apt-get install clang
```

Solution (use corpus testing instead):
```bash
# Use gcc-based corpus testing if clang unavailable
cd compute
make test-corpus

# Or install clang
sudo apt-get install clang  # Ubuntu/Debian
sudo dnf install clang      # Fedora
```

**C tests compile but crash immediately**

Check for missing sanitizer libraries:
```bash
cd compute

# Try building with debug flags only
gcc -O0 -g -I./include -o test/test_protocol test/test_protocol.c src/protocol.c -lm
./test/test_protocol

# If that works but ASan crashes, install sanitizer runtime
sudo apt-get install libasan6  # Ubuntu/Debian
```

**Protocol test passes but integration fails**

Indicates encoder/decoder mismatch. Debug:
```bash
# Generate a request and inspect it
cd compute
make test-stub

# Create test input from Go side
cd backend
go run -tags=integration ./test/integration/debug_encode.go > /tmp/request.bin
hexdump -C /tmp/request.bin

# Pass through C stub
./test/test_stub_generator < /tmp/request.bin > /tmp/response.bin
hexdump -C /tmp/response.bin

# Check for differences in byte layout
```

**Fuzzer finds crashes**

Crashes are saved to `compute/fuzz/crash-*` files:
```bash
cd compute/fuzz

# List crashes
ls -la crash-*

# Reproduce crash under debugger
gdb ../test/test_protocol
(gdb) run < crash-abc123

# Or with ASan for better diagnostics
../test/test_protocol_asan < crash-abc123
```

Fix the bug, then verify:
```bash
# Verify crash is fixed
./test/test_protocol_asan < fuzz/crash-abc123

# Re-run fuzzer to ensure no new crashes
make fuzz
```

**Test performance is slow**

Integration tests spawn processes, which can be slow:
```bash
# Run only fast unit tests
cd backend
go test ./internal/protocol/...

# Skip integration tests during development
go test ./... -short

# Run integration tests only when needed
go test -tags=integration ./test/integration/
```

### Build Issues

**Go module checksum mismatch**

```bash
cd backend
go clean -modcache
go mod download
go mod verify
```

**C compilation fails with linking errors**

Ensure math library is linked:
```bash
# Makefile should include: -lm
gcc ... -lm
```

### Runtime Issues

**Socket connection issues**

See [Troubleshooting Socket Issues](#troubleshooting-socket-issues) in the Unix Socket Communication section for detailed guidance on:
- XDG_RUNTIME_DIR not set
- Compute process not running (ENOENT)
- Connection refused (stale socket)
- Permission denied
- Authentication failures
- Connection timeout

**Out of memory errors**

Check available GPU memory:
```bash
nvidia-smi  # For NVIDIA GPUs
rocm-smi    # For AMD GPUs
```

## CI/CD Checks

All PRs must pass:

- [ ] Go unit tests (`cd backend && go test ./...`)
- [ ] Go protocol tests (`cd backend && go test ./internal/protocol/...`)
- [ ] C unit tests (`cd compute && make test`)
- [ ] ASan/UBSan tests (`cd compute && make test-asan`)
- [ ] Integration tests (`cd backend && go test -tags=integration ./test/integration/`)
- [ ] Corpus validation (`cd compute && make test-corpus`)
- [ ] Fuzzing (5-minute smoke test)
- [ ] Go linting (`cd backend && golangci-lint run`)
- [ ] Formatting (`cd backend && go fmt`, `cd compute && make fmt`)

## Documentation Standards

See `.claude/rules/documentation.md` for full documentation standards.

**Key points:**
- No emoji in documentation
- Professional tone
- Code examples must be complete and tested
- Keep README.md concise, detailed docs in `docs/`

## Security Considerations

**Critical for protocol implementation:**
- Validate all input before processing
- Check bounds before all array access
- Use sanitizers during development
- Fuzz all parsing code
- Never trust client input

## Vulkan Backend

### Implementation Approach

After evaluating multiple Vulkan implementation approaches for SD 3.5 Medium inference, we selected **stable-diffusion.cpp** with the GGML Vulkan backend.

### Evaluation Summary

#### Options Evaluated

**1. stable-diffusion.cpp (GGML + Vulkan)**
- Pure C/C++ implementation with GGML tensor library
- SD 3.5 Medium and Large support confirmed
- Mature Vulkan backend supporting NVIDIA, AMD, and Intel GPUs
- SafeTensors support via direct loading or GGUF conversion
- Active development with recent SD 3.5 support
- C API compatible (C99 interoperability)

**2. ncnn (Tencent)**
- High-performance mobile-optimized framework
- Excellent Vulkan support across vendors
- No SD 3.5 support found (last updates target SD 1.x)
- Would require significant porting effort for SD 3.5 architecture

**3. Vulkan-Kompute**
- Lightweight general-purpose GPU compute framework
- Good cross-vendor support (backed by Linux Foundation)
- No pre-built diffusion model support
- Would require implementing SD 3.5 from scratch

**4. Raw Vulkan Compute Shaders**
- Maximum control and optimization potential
- Cross-vendor support via standard Vulkan API
- Requires implementing entire inference pipeline from scratch
- Development time incompatible with MVP timeline

**5. GGML Ecosystem (llama.cpp family)**
- Vulkan backend mature and well-tested
- stable-diffusion.cpp is part of this ecosystem
- Same benefits as stable-diffusion.cpp but integrated

#### Decision: stable-diffusion.cpp

**Rationale:**

1. **SD 3.5 Support**: Only option with confirmed SD 3.5 Medium and Large support out of the box.

2. **Format Support**: Handles both SafeTensors (.safetensors) and GGUF (.gguf) formats. Can load models directly without conversion or convert to quantized GGUF for reduced VRAM.

3. **Vulkan Maturity**: Built on GGML's battle-tested Vulkan backend used by llama.cpp. Supports NVIDIA, AMD, and Intel GPUs with active maintenance.

4. **C Compatibility**: Pure C/C++ implementation with straightforward C interoperability. No Python dependencies, no complex bindings.

5. **Performance**: Designed for consumer hardware. SD 3.5 Medium requires only 9.9GB VRAM (excluding text encoders), well within RTX 4070 Super's 12GB.

6. **Integration Path**: Well-documented API for model loading, encoding, and generation. Text encoders (CLIP-L, CLIP-G, T5-XXL) can run on CPU to conserve VRAM.

7. **Community**: Active development with Easy Diffusion v3 using it as backend. Recent fixes for SD 3.5 and Vulkan issues.

**Known Issues:**

- Vulkan performance slower than expected on some configurations (Issue #1114)
- SD 3.5 Large crashes reported on Arch Linux with Vulkan (Issue #1105)
- GGML memory management can be overly conservative with Vulkan, allocating to shared memory when VRAM is low

These are being actively addressed by the project and mostly affect SD 3.5 Large. SD 3.5 Medium has better stability.

#### Rejected Options

**ncnn**: Excellent framework but no SD 3.5 support. Porting SD 3.5's MMDiT-X architecture would take longer than MVP timeline.

**Vulkan-Kompute**: Too low-level. Would need to implement diffusion pipeline, attention mechanisms, and model architecture from scratch.

**Raw Vulkan**: Maximum flexibility but months of development. Not viable for MVP.

### Implementation Plan

#### Phase 1: Integration
- Add stable-diffusion.cpp as submodule or vendored dependency
- Build with Vulkan support enabled (`-DSD_VULKAN=ON`)
- Create C wrapper interface for compute process

#### Phase 2: Model Loading
- Download SD 3.5 Medium from Hugging Face:
  - Main model: `stabilityai/stable-diffusion-3.5-medium`
  - Files: `sd3.5_medium.safetensors`, `clip_l.safetensors`, `clip_g.safetensors`, `t5xxl_fp16.safetensors`
- Load SafeTensors directly or convert to GGUF for quantization
- Hardcoded path: `./models/sd3.5_medium.safetensors`

#### Phase 3: Text Encoding
- Load CLIP-L, CLIP-G, and T5-XXL encoders
- Run on CPU with `keep_clip_on_cpu=true` flag
- Generate embeddings from prompt text

#### Phase 4: Diffusion Inference
- Initialize Vulkan device (first available GPU)
- Load diffusion model weights into VRAM
- Execute inference with parameters: width, height, steps, guidance, seed
- Return raw pixel data (RGB or RGBA per protocol spec)

#### Phase 5: Error Handling
- Model file missing: exit with clear error
- Model file corrupted: exit with clear error
- No Vulkan GPU: exit with clear error
- OOM during generation: return protocol error 500
- GPU errors: return protocol error 500 with message

### Expected Performance

**Target Hardware**: RTX 4070 Super (12GB VRAM)

**SD 3.5 Medium**:
- Resolution: 1024x1024
- Steps: 4-8
- Expected time: 2-4 seconds (estimated based on TensorRT 1.7x speedup over PyTorch)
- VRAM usage: ~10GB (9.9GB model + overhead)

**Optimizations**:
- TensorRT available for NVIDIA GPUs (1.7x speedup over PyTorch)
- BF16 precision for faster inference
- Text encoders on CPU to conserve VRAM
- Model stays loaded in VRAM (no per-request loading)

### Vulkan Driver Requirements

**Minimum Vulkan Version**: 1.2

**Verification**:
```bash
# Check Vulkan installation
vulkaninfo | grep "Vulkan Instance Version"

# List available GPUs
vulkaninfo | grep "deviceName"

# Verify compute queue support
vulkaninfo | grep -A 5 "queueFlags"
```

**Installation**:

NVIDIA:
```bash
# Install latest NVIDIA driver (525+ recommended)
sudo ubuntu-drivers install

# Verify
nvidia-smi
vulkaninfo | grep NVIDIA
```

AMD:
```bash
# AMDGPU-PRO driver or Mesa (23.0+)
sudo apt-get install mesa-vulkan-drivers

# Verify
vulkaninfo | grep AMD
```

Intel:
```bash
# Mesa 23.0+ with ANV driver
sudo apt-get install mesa-vulkan-drivers intel-media-va-driver

# Verify
vulkaninfo | grep Intel
```

### Model Download Instructions

**SD 3.5 Medium from Hugging Face**:

```bash
# Install Hugging Face CLI
pip install huggingface-hub

# Login (required for SD 3.5)
huggingface-cli login

# Download model files
mkdir -p models
cd models

# Main model (2.5B parameters, ~10GB)
huggingface-cli download stabilityai/stable-diffusion-3.5-medium \
  sd3.5_medium.safetensors --local-dir .

# Text encoders
huggingface-cli download stabilityai/stable-diffusion-3.5-medium \
  text_encoders/clip_l.safetensors --local-dir .

huggingface-cli download stabilityai/stable-diffusion-3.5-medium \
  text_encoders/clip_g.safetensors --local-dir .

huggingface-cli download stabilityai/stable-diffusion-3.5-medium \
  text_encoders/t5xxl_fp16.safetensors --local-dir .
```

**Directory Structure**:
```
weave/
├── compute/
│   └── weave-compute
└── models/
    ├── sd3.5_medium.safetensors      # Main diffusion model
    ├── clip_l.safetensors             # CLIP-L text encoder
    ├── clip_g.safetensors             # CLIP-G text encoder
    └── t5xxl_fp16.safetensors         # T5-XXL text encoder
```

**Total disk space**: ~16GB for SD 3.5 Medium

### Troubleshooting GPU/Vulkan Issues

**No Vulkan devices found**:
```bash
# Check Vulkan installation
vulkaninfo

# If not found, install Vulkan SDK
wget -qO - https://packages.lunarg.com/lunarg-signing-key-pub.asc | sudo apt-key add -
sudo wget -qO /etc/apt/sources.list.d/lunarg-vulkan-jammy.list \
  https://packages.lunarg.com/vulkan/lunarg-vulkan-jammy.list
sudo apt update
sudo apt install vulkan-sdk
```

**VRAM out of memory**:
```bash
# Check available VRAM
nvidia-smi  # NVIDIA
rocm-smi    # AMD

# Try quantized model (GGUF Q8_0, ~50% smaller)
# Convert SafeTensors to GGUF
./sd-convert -m models/sd3.5_medium.safetensors \
  -o models/sd3.5_medium_q8.gguf --type q8_0
```

**Vulkan validation errors**:
```bash
# Disable validation layers for production
export VK_LAYER_PATH=""
export VK_INSTANCE_LAYERS=""

# Or enable for debugging
export VK_INSTANCE_LAYERS=VK_LAYER_KHRONOS_validation
```

**Slow generation (>10s per image)**:
- Check GPU is being used: `nvidia-smi` should show GPU utilization
- Verify Vulkan backend enabled at compile time
- Consider switching to CUDA backend for NVIDIA GPUs (faster but not cross-vendor)
- Check for memory thrashing (VRAM usage near limit)

**AMD GPU not working**:
- Verify ROCm not interfering: `unset ROCR_VISIBLE_DEVICES`
- Use Vulkan drivers only, not ROCm
- Update to Mesa 23.0+ for better Vulkan support

**Intel Arc GPU issues**:
- Update to latest Intel compute runtime
- Verify Arc GPU supports Vulkan 1.2+ compute
- Some older integrated GPUs may not have sufficient compute support

### Alternative: GGUF Quantized Models

For reduced VRAM usage, use GGUF quantized models:

**Benefits**:
- 30-50% less VRAM (Q8_0: ~6GB vs 10GB)
- Faster loading
- Minimal quality loss

**Drawbacks**:
- Conversion step required
- Slightly slower inference (quantization overhead)

**Conversion**:
```bash
# Convert SafeTensors to GGUF Q8_0 (8-bit quantization)
./sd-convert -m models/sd3.5_medium.safetensors \
  -o models/sd3.5_medium_q8.gguf --type q8_0

# Use in compute process (modify model path)
# Load: models/sd3.5_medium_q8.gguf instead of .safetensors
```

**GGUF support**: stable-diffusion.cpp natively supports GGUF loading. No additional dependencies.

### Cross-GPU Compatibility

**Tested Hardware:**

| GPU | Vendor | Status | Notes |
|-----|--------|--------|-------|
| RTX 4070 Super | NVIDIA | Tested | Primary development platform, 12GB VRAM |
| AMD GPUs | AMD | Untested | Expected to work via Vulkan portability |
| Intel Arc GPUs | Intel | Untested | Expected to work via Vulkan portability |

**NVIDIA GPUs (Tested)**

Generation has been verified on RTX 4070 Super (12GB VRAM):
- 512x512, 4 steps: ~3.9s
- VRAM usage: ~5.8GB (diffusion model + VAE)
- RAM usage: ~10.6GB (text encoders on CPU)

Performance breakdown:
- Text encoding (T5-XXL on CPU): ~1.67s (43% of total)
- Diffusion sampling (GPU): ~1.11s
- VAE decode (GPU): ~0.54s

**AMD GPUs (Untested)**

Expected to work via Vulkan 1.2+ support. stable-diffusion.cpp uses the GGML Vulkan backend which has AMD support.

Requirements:
- Mesa 23.0+ or AMDGPU-PRO driver
- Vulkan 1.2+ compute support
- Sufficient VRAM (8GB+ recommended)

To test:
```bash
# Verify Vulkan works
vulkaninfo | grep AMD

# Run compute process
./weave-compute

# Run benchmark
./bench/bench_generate models/sd3.5_medium.safetensors 5
```

Known issues:
- Do not mix ROCm and Vulkan drivers: `unset ROCR_VISIBLE_DEVICES`
- GGML may fall back to shared memory if VRAM detection fails

**Intel Arc GPUs (Untested)**

Expected to work via Vulkan 1.2+ support on Intel Arc discrete GPUs. Integrated GPUs may lack sufficient compute capability.

Requirements:
- Intel compute runtime (latest)
- Mesa 23.0+ with ANV driver
- Vulkan 1.2+ compute support
- Intel Arc discrete GPU recommended (8GB+ VRAM)

To test:
```bash
# Verify Vulkan works
vulkaninfo | grep Intel

# Check for compute queue
vulkaninfo | grep -A 5 "queueFlags" | grep COMPUTE

# Run compute process
./weave-compute
```

Known issues:
- Older integrated GPUs may not support required compute features
- Arc GPUs have different performance characteristics than NVIDIA/AMD

**Cross-Vendor Expectations**

The Vulkan backend provides cross-vendor GPU support through the standard Vulkan API. Key considerations:

1. **Shader Compilation**: GGML compiles Vulkan compute shaders at runtime. First generation may be slower due to shader caching.

2. **Memory Management**: VRAM allocation differs per vendor. GGML attempts to detect available memory but may be conservative.

3. **Performance Variance**: Different GPU architectures may have varying performance. NVIDIA typically optimized first by upstream.

4. **Driver Quality**: Vulkan driver maturity varies. Use latest stable drivers.

If generation fails on non-NVIDIA hardware, please file an issue with:
- GPU model and driver version
- `vulkaninfo` output
- Error message from compute process
- VRAM available (`nvidia-smi`, `rocm-smi`, or equivalent)

### References

- [stable-diffusion.cpp GitHub](https://github.com/leejet/stable-diffusion.cpp)
- [GGML Vulkan Backend](https://github.com/ggerganov/ggml/pull/904)
- [SD 3.5 Medium on Hugging Face](https://huggingface.co/stabilityai/stable-diffusion-3.5-medium)
- [Vulkan SDK Downloads](https://www.lunarg.com/vulkan-sdk/)
- [NVIDIA TensorRT for SD 3.5](https://blogs.nvidia.com/blog/rtx-ai-garage-gtc-paris-tensorrt-rtx-nim-microservices/)

## ollama LLM Setup

Weave currently uses ollama to power the conversational interface for image generation. ollama runs locally and provides fast LLM inference. This dependency will be removed when the compute layer supports LLM inference directly.

### Installing ollama

**Linux:**
```bash
curl -fsSL https://ollama.com/install.sh | sh
```

**macOS:**
```bash
brew install ollama
```

**Windows:**

Download the installer from https://ollama.com/download

### Starting ollama

ollama runs as a service. Start it with:

```bash
ollama serve
```

Or run as a background service:
```bash
# Linux (systemd)
sudo systemctl start ollama

# macOS (Homebrew)
brew services start ollama
```

### Pulling the Required Model

Weave uses `mistral:7b` for the conversational agent:

```bash
ollama pull mistral:7b
```

This downloads approximately 4.1GB. The model provides better instruction following and format adherence than smaller models.

### Verifying ollama is Running

```bash
# Check if ollama is running
curl http://localhost:11434/api/tags

# List available models
ollama list

# Expected output should include:
# NAME              ID              SIZE      MODIFIED
# mistral:7b        ...             4.1 GB    ...
```

### Running Integration Tests

With ollama running and the model pulled:

```bash
# Run ollama integration tests
cd backend
go test -tags=integration -v ./internal/ollama/...
```

### Troubleshooting ollama Issues

**ollama not running**

```
Error: ollama not running at http://localhost:11434
```

Solution:
```bash
# Start ollama
ollama serve

# Or check if already running
pgrep ollama
```

**Model not found**

```
Error: model mistral:7b not available in ollama
```

Solution:
```bash
# Pull the model
ollama pull mistral:7b

# Verify it's available
ollama list
```

**Port already in use**

```
Error: listen tcp 127.0.0.1:11434: bind: address already in use
```

Solution:
```bash
# Check what's using the port
lsof -i :11434

# Kill existing ollama process
pkill ollama

# Restart
ollama serve
```

**Connection refused**

```
Error: connection refused
```

This usually means ollama crashed or was not started. Check:
```bash
# View ollama logs (Linux systemd)
journalctl -u ollama -f

# Restart the service
sudo systemctl restart ollama
```

**Slow first response**

The first request after model load is slower due to model initialization. Subsequent requests are faster. This is normal behavior.

**Out of memory (OOM)**

mistral:7b requires approximately 5GB RAM. If you see OOM errors:
```bash
# Check available memory
free -h

# Close other applications to free memory
```

### ollama Configuration

ollama listens on `localhost:11434` by default. The weave client uses this hardcoded endpoint.

To verify the endpoint:
```bash
curl http://localhost:11434/api/tags
# Should return JSON with available models
```

### Model Selection

Weave currently uses `mistral:7b` which provides excellent instruction following and format adherence for conversational tasks. This model:
- Runs on CPU (no GPU required)
- Uses ~5GB RAM
- Generates responses in 2-5 seconds
- Better at maintaining structured output format than smaller models
- Suitable for clarifying questions and prompt generation

**Why Mistral 7B:**

Mistral 7B was chosen over smaller models like Llama 3.2 1B due to superior instruction following and format adherence. The structured output format (see below) requires the model to consistently end every message with a delimiter and JSON. Smaller models tend to drift from this format after 3-4 conversation turns, causing prompts to appear in the chat window instead of being extracted correctly. Mistral 7B maintains format consistency across multi-turn conversations.

For alternative models, use the `--ollama-model` flag:
```bash
# Smaller, faster (may have format drift issues)
./build/weave-backend --ollama-model llama3.2:1b

# Larger, higher quality
./build/weave-backend --ollama-model llama3.1:8b
```

The default model can be changed by updating `defaultOllamaModel` in `backend/internal/config/config.go`.

### Structured Output Format

Weave uses a structured output format to separate conversational text (shown in chat) from machine-readable metadata (prompt extraction and generation settings). Every LLM response follows this format:

```
<Conversational text visible to user>
---
{"prompt": "...", "generate_image": true, "steps": 4, "cfg": 1.0, "seed": -1}
```

**Format specification:**

1. **Conversational text**: Appears before the `---` delimiter. This is displayed in the chat pane and provides a natural conversation experience.

2. **Delimiter**: Three hyphens (`---`) on their own line. This signals the end of user-visible text and the start of metadata.

3. **JSON metadata**: Appears after the delimiter. This is parsed but not shown in the chat pane.

**JSON schema:**

```json
{
  "prompt": "string",           // Image generation prompt (empty string if not ready)
  "generate_image": boolean,    // True to trigger generation, false to just update prompt
  "steps": integer,             // Number of inference steps (typically 4-28)
  "cfg": float,                 // CFG scale (classifier-free guidance, typically 1.0-7.5)
  "seed": integer               // Generation seed (-1 for random, >=0 for reproducible)
}
```

**Field descriptions:**

- **prompt**: The image generation prompt. Empty string when still asking clarifying questions.
- **generate_image**: Controls automatic generation. When `true`, the backend automatically generates the image (same as user clicking generate button). When `false`, just updates prompt and settings without generating.
- **steps**: Number of diffusion steps. More steps = higher quality but slower. Typical range: 4-28.
- **cfg**: Classifier-free guidance scale. Higher values follow prompt more strictly. Typical range: 1.0-7.5.
- **seed**: Random seed for reproducibility. Use -1 for random generation, or a specific value (0 or higher) for deterministic results.

**Examples:**

Agent asking clarifying questions:
```
A cat in a hat! Let me ask a few questions:
- What kind of cat?
- What style of hat?
---
{"prompt": "", "generate_image": false, "steps": 4, "cfg": 1.0, "seed": -1}
```

Agent ready to generate:
```
Perfect! Generating your image now.
---
{"prompt": "a tabby cat wearing a blue wizard hat", "generate_image": true, "steps": 4, "cfg": 1.0, "seed": -1}
```

Agent adjusting settings:
```
I'll increase the quality with more steps.
---
{"prompt": "a tabby cat wearing a blue wizard hat", "generate_image": true, "steps": 28, "cfg": 1.0, "seed": -1}
```

### Automatic Retry Logic

The structured format requires precise adherence. If the LLM fails to provide the delimiter or valid JSON, weave automatically retries with escalating recovery strategies. This happens transparently without user intervention.

**Three-level retry strategy:**

**Level 1: Format Reminder (2 attempts)**

If the response is missing the `---` delimiter, has invalid JSON, or is missing required fields, weave appends a system message reminding the LLM of the format:

```
Please end your response with `---` followed by JSON using this format:
{"prompt": "your prompt here", "generate_image": true, "steps": 4, "cfg": 1.0, "seed": -1}
```

The LLM gets up to 2 chances to correct the format. This recovers most format errors.

**Level 2: Context Compaction (1 attempt)**

If format reminders fail, weave assumes the conversation context has grown too large and confused the model. It compacts the context by:

1. Extracting key details from user messages (subject, style, setting)
2. Replacing entire conversation history with a single system message
3. Requesting JSON-only response (no conversational text)

Compacted format:
```
User wants: [key details extracted from conversation].
Respond with ONLY JSON (no conversational text): {"prompt": "...", "generate_image": true, "steps": 4, "cfg": 1.0, "seed": -1}
```

This gives the LLM a fresh start with minimal context. One retry attempt is made with compacted context.

**Level 3: Error and Reset**

If compaction retry fails, weave shows an error to the user:

```
I'm having trouble understanding the format. Let's start fresh.
```

The conversation history is cleared and the user can start a new conversation immediately. The full error with message history is logged for debugging.

**Retry behavior:**

- Retries are transparent to the user (no error shown during retry)
- Retry count resets on successful parse (not cumulative across conversation)
- Maximum retries: 2 format reminders + 1 compaction = 3 total attempts before reset
- Retry logs are visible at DEBUG level: `--log-level debug`

### Format Error Debugging

To debug format errors, enable DEBUG logging:

```bash
./build/weave-backend --log-level debug
```

Look for log messages indicating:
- Format errors (missing delimiter, invalid JSON, missing fields)
- Retry attempts (format reminder, context compaction)
- Full LLM response text (before parsing)

Example DEBUG output:
```
[DEBUG] LLM response missing delimiter, appending format reminder (attempt 1/2)
[DEBUG] Retry with format reminder successful
[DEBUG] Parsed metadata: prompt="a cat", ready=true
```

If you see repeated format errors, the model may not support this format well. Try a larger model:
```bash
./build/weave-backend --ollama-model llama3.1:8b
```

## Web UI

The backend serves a web interface using HTMX for dynamic updates and Server-Sent Events (SSE) for real-time streaming. The same UI is displayed in both the Electron desktop app and browser.

For development, you can access the UI directly via browser at `http://localhost:8080` instead of through Electron. This is useful for:
- Using browser developer tools
- Faster iteration (no Electron rebuild needed)
- Debugging SSE and network requests

### Running for development

```bash
ollama serve              # In one terminal
./build/weave-backend             # In another terminal
```

Then open `http://localhost:8080` in your browser.

### Accessing the UI

Open your browser and navigate to:

```
http://localhost:8080
```

You should see a split-screen layout:
- Left pane: Chat area for conversational interaction
- Right pane: Prompt area for editing generation prompts

### Session Management

The web server uses session cookies to track user sessions and route SSE events correctly.

**Session cookie details:**
- Cookie name: `weave_session`
- Expiry: 24 hours from creation
- Format: 32 character hex string (128 bits of entropy)
- Flags: `HttpOnly`, `SameSite=Strict`

**How it works:**
1. First visit: Server generates a new session ID and sets a cookie
2. Subsequent requests: Browser sends session cookie automatically
3. SSE connection: Tied to session ID for event routing
4. Cookie expiry: After 24 hours, a new session is created

Session IDs are cryptographically random and ensure that SSE events are only sent to the correct browser connection.

### Available Endpoints

**Web routes:**
- `GET /` - Index page (main UI)
- `GET /static/*` - Static assets (CSS, JS)

**SSE endpoint:**
- `GET /events` - Server-Sent Events for real-time updates
  - Content-Type: `text/event-stream`
  - Streams events: `agent-token`, `agent-done`, `prompt-update`, `image-ready`, `error`

**API endpoints:**
- `POST /chat` - Send user message to conversational agent
- `POST /prompt` - Update generation prompt
- `POST /generate` - Trigger image generation

All API endpoints require a valid session cookie and return JSON responses.

### Debugging SSE Events

The browser console shows SSE connection status and events. To debug:

**Open browser developer tools:**
- Chrome/Edge: F12 or Ctrl+Shift+I
- Firefox: F12 or Ctrl+Shift+I
- Safari: Cmd+Option+I

**Check Network tab:**
1. Open Network tab
2. Look for `events` connection
3. Check status (should be 200 OK and stay open)
4. Click on `events` to see streaming data

**Check Console tab:**

The HTMX library logs SSE events to the console. Look for messages like:
```
htmx:sseMessage {event: 'agent-token', data: '{"token":"Hello"}'}
htmx:sseMessage {event: 'prompt-update', data: '{"prompt":"a cat"}'}
```

**Enable verbose logging:**

Add `htmx.logAll()` to the browser console for detailed HTMX activity:
```javascript
htmx.logAll()
```

This logs all HTMX operations including SSE event handling, DOM swaps, and AJAX requests.

**Common issues:**

**SSE connection not established**
```
EventSource failed: network error
```

Solution:
- Verify server is running (`curl http://localhost:8080`)
- Check browser console for detailed error
- Ensure no browser extensions blocking SSE (ad blockers, security tools)

**Session cookie not set**

If session ID is missing from requests, check:
```bash
# Verify server sets cookie
curl -v http://localhost:8080 2>&1 | grep -i set-cookie
# Should show: Set-Cookie: weave_session=...
```

**Events not appearing in UI**

Check HTMX is loaded:
```javascript
// Run in browser console
typeof htmx
// Should return "object", not "undefined"
```

### Integration Testing

Test the web server programmatically:

```bash
# Run web integration tests
cd backend
go test -tags=integration -v ./internal/web/...
```

These tests verify:
- Server starts and serves index page
- Session cookies are set correctly
- SSE connections establish successfully
- Events route to correct session
- API endpoints respond correctly

### User Interaction Walkthrough

This section describes the complete user flow for interacting with the web UI.

#### Starting a Conversation

1. Open `http://localhost:8080` in your browser
2. Type a message in the chat input at the bottom left (e.g., "I want a picture of a cat")
3. Press Enter or click "Send"
4. Your message appears on the right side of the chat
5. The input is disabled while the agent responds
6. Agent tokens stream in token-by-token on the left side
7. When complete, the input re-enables and focuses

The agent asks clarifying questions to understand what you want (style, setting, details). Continue the conversation until the agent provides a prompt.

#### Prompt Updates

When the agent decides you have enough information:

1. The agent includes a line starting with "Prompt: " in its response
2. This prompt appears automatically in the right-side prompt textarea
3. The prompt pane updates in real-time as the agent types

#### Editing the Prompt

You can manually edit the prompt:

1. Click on the prompt textarea in the right pane
2. Make your changes (e.g., add "wearing a top hat")
3. Click outside the textarea (blur)
4. The textarea briefly flashes green to confirm save
5. Your edit is injected into the conversation so the agent knows

While you're typing in the prompt field, SSE updates from the agent are ignored to prevent clobbering your changes.

#### Generating an Image

Once you have a prompt:

1. Click the "Generate Image" button below the prompt
2. The button shows "Generating..." and a spinner appears in chat
3. When complete:
   - The image appears inline in the chat
   - The image also appears in the "Generated Image" preview on the right
   - The button re-enables

If generation fails:
- An error message appears in the chat (red background)
- The button re-enables so you can retry

#### Error Handling

Errors display directly in the chat pane:

- **Agent errors**: If the LLM fails, you see "Agent error: <message>"
- **Generation errors**: If image generation fails, you see "Generation failed: <message>"
- **Network errors**: "Connection error, please try again"

Errors have a red background and appear at the bottom of the chat. After an error, all inputs re-enable so you can continue.

#### Testing Each Feature

To verify each feature works:

**Chat streaming:**
```
1. Open browser dev tools (F12)
2. Go to Network tab, filter by "events"
3. Send a message
4. Watch SSE events appear: agent-token, agent-done
```

**Prompt updates:**
```
1. Continue chatting until agent provides a "Prompt: " line
2. Watch the prompt textarea update automatically
```

**Prompt editing:**
```
1. Click in prompt textarea
2. Type something
3. Click outside (blur)
4. Watch for brief green flash (save confirmation)
```

**Image generation:**
```
1. Click "Generate Image" with a prompt set
2. Watch spinner appear in chat
3. See image appear (or error if compute process not running)
```

**Session persistence:**
```
1. Refresh the page
2. Previous session is lost (sessions are in-memory for MVP)
3. New session ID assigned (check cookie)
```

### Shutting Down

The server handles graceful shutdown on interrupt signals:

```bash
# Press Ctrl+C to stop server
^C
Shutting down web server...
Web server stopped
```

The shutdown process:
1. Stops accepting new connections
2. Waits for in-flight requests to complete (up to 10 seconds)
3. Closes all SSE connections cleanly
4. Exits

## Image Storage and Serving

Weave stores generated images in memory for serving to the browser. This section explains the image lifecycle, storage limits, and cleanup behavior.

### Storage Architecture

**In-Memory Storage**

Generated images are stored in a map keyed by UUID. Each image entry contains:
- PNG-encoded image data ([]byte)
- Image dimensions (width, height)
- Creation timestamp
- Last accessed timestamp

Storage is per-server instance. Restarting the server clears all images.

**Image URL Format**

Images are served at: `/images/<uuid>.png`

Example: `/images/550e8400-e29b-41d4-a716-446655440000.png`

The .png extension is optional. Both URLs work:
```
/images/550e8400-e29b-41d4-a716-446655440000
/images/550e8400-e29b-41d4-a716-446655440000.png
```

### Cleanup Behavior

**Automatic Cleanup**

A background goroutine runs every 10 minutes to clean up old images:

1. **Age-based removal**: Images older than 1 hour are deleted
2. **LRU eviction**: If image count exceeds 100, least recently accessed images are removed

Cleanup happens automatically. No manual intervention required.

**Cleanup Logging**

Cleanup events are logged at DEBUG level:

```bash
./build/weave-backend --log-level debug
```

Example cleanup log output:
```
[DEBUG] Removed 5 images older than 1h0m0s
[DEBUG] LRU eviction removed 3 images (limit: 100)
[DEBUG] Cleanup complete: 108 -> 100 images
```

**Storage Limits**

| Limit | Value | Behavior |
|-------|-------|----------|
| Max age | 1 hour | Images older than 1 hour are deleted |
| Max count | 100 | LRU eviction when limit exceeded |
| Cleanup interval | 10 minutes | Background cleanup frequency |

These limits prevent unbounded memory growth for long-running servers.

### Image Generation Flow

1. User triggers generation via "Generate Image" button
2. Server generates placeholder gradient (or calls compute process when ready)
3. Raw pixel data (RGB) encoded to PNG using Go's image/png package
4. PNG stored in memory, UUID generated
5. SSE event sent with image URL: `{"url": "/images/<uuid>.png"}`
6. Browser fetches image via HTTP GET
7. Server returns PNG with caching headers

**Placeholder Images**

Currently, the server generates a colored gradient placeholder since the compute process integration is not complete. This will be replaced with actual generated images from weave-compute.

Placeholder format:
- Size: 512x512 pixels
- Format: RGB (3 bytes per pixel)
- Pattern: Gradient with red increasing left-to-right, green top-to-bottom

### HTTP Caching Headers

Images are served with aggressive caching headers since they never change:

```
Content-Type: image/png
Cache-Control: public, max-age=31536000, immutable
```

This allows browsers to cache images indefinitely, reducing server load.

### Debugging Image Storage

**Check image count:**

Enable DEBUG logging to see storage statistics:
```bash
./build/weave-backend --log-level debug
```

Cleanup events show current image count.

**View stored images:**

Images are stored in memory only. To inspect:

```bash
# Watch HTTP requests to see image URLs
curl -v http://localhost:8080/images/<uuid>.png

# Save image for inspection
curl http://localhost:8080/images/<uuid>.png > test.png
file test.png  # Should show: PNG image data
```

**Test image retrieval:**

```bash
# Valid UUID (returns 200 OK)
curl -I http://localhost:8080/images/550e8400-e29b-41d4-a716-446655440000.png

# Invalid UUID (returns 400 Bad Request)
curl -I http://localhost:8080/images/not-a-uuid

# Non-existent UUID (returns 404 Not Found)
curl -I http://localhost:8080/images/00000000-0000-0000-0000-000000000000.png
```

**Test cleanup:**

Storage cleanup can be tested by:
1. Generating multiple images (>100 to trigger LRU)
2. Waiting 1 hour for age-based cleanup
3. Watching DEBUG logs for cleanup events

Integration tests in `backend/internal/web/image_pipeline_integration_test.go` verify cleanup behavior programmatically.

### Troubleshooting Image Issues

**Image not displaying in browser**

Check browser developer console (F12) for errors:
```
Failed to load resource: net::ERR_NAME_NOT_RESOLVED
```

Ensure server is running and URL is correct.

**Image returns 404**

The image may have been cleaned up (>1 hour old or evicted by LRU):
- Regenerate the image
- Images are ephemeral, not persistent across restarts

**Image returns 400 Bad Request**

The image ID is malformed (not a valid UUID):
- Check the URL format
- Ensure SSE event provided correct UUID

**Memory usage growing unbounded**

If cleanup is not working:
- Verify DEBUG logs show cleanup running every 10 minutes
- Check for goroutine leaks (cleanup goroutine should be running)
- File an issue with logs and `pprof` output

### Future Improvements

Current limitations (to be addressed post-MVP):

- Images are not persisted across server restarts
- No disk-based storage option
- No image metadata stored (prompt, generation params)
- No image download endpoint
- No image history or gallery view

These will be added based on user feedback and usage patterns.

## Getting Help

- Check documentation in `docs/`
- Review code examples in `test/`
- See `.claude/rules/` for language-specific standards
- File issues for bugs or unclear documentation
