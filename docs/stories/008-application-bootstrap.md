# Story 008: Application Bootstrap

## Problem

The individual components (ollama client, conversation manager, compute client, web server) need to be wired together into a working application. The main `weave` binary must initialize all components, validate dependencies, accept CLI arguments for hardcoded settings, and coordinate shutdown.

## User/Actor

- End user (starts the application, expects clear feedback if dependencies are missing)
- Weave developer (implementing the main entry point)

## Desired Outcome

A working `weave` binary where:
- User runs `weave` and the application starts
- Application validates ollama is running before accepting requests
- Application validates weave-compute is running before accepting requests
- CLI arguments allow overriding hardcoded defaults (steps, CFG, resolution, seed)
- Application shuts down gracefully on SIGTERM/SIGINT
- Clear error messages guide users when dependencies are missing

## Acceptance Criteria

### CLI Interface

- [ ] Binary is named `weave` and built to `./bin/weave`
- [ ] `weave --help` shows usage with all available flags
- [ ] `weave --version` shows version string
- [ ] Default behavior (no flags) starts the web server

### CLI Flags (MVP Hardcoded Defaults)

- [ ] `--port` - HTTP server port (default: 8080)
- [ ] `--steps` - Inference steps (default: 4)
- [ ] `--cfg` - CFG scale (default: 1.0)
- [ ] `--width` - Image width (default: 1024)
- [ ] `--height` - Image height (default: 1024)
- [ ] `--seed` - Image generation seed, 0 = random (default: 0)
- [ ] `--llm-seed` - LLM seed for deterministic responses, 0 = random (default: 0)
- [ ] `--ollama-url` - ollama endpoint (default: http://localhost:11434)
- [ ] `--ollama-model` - LLM model name (default: llama3.2:1b)

### Startup Validation

- [ ] On startup, check ollama is reachable (HTTP GET to `/api/tags`)
- [ ] If ollama not reachable: exit with error "ollama not running at <url>"
- [ ] On startup, check weave-compute socket exists
- [ ] If socket missing: exit with error "weave-compute not running (socket not found at <path>)"
- [ ] On startup, attempt test connection to weave-compute
- [ ] If connection refused: exit with error "weave-compute not accepting connections"
- [ ] All validation happens before HTTP server starts listening

### Component Initialization

- [ ] Create ollama client with configured URL and model
- [ ] Create session manager (holds conversation manager instances per session)
- [ ] Create compute client with socket path from environment
- [ ] Create HTTP server with configured port
- [ ] Wire components: HTTP handlers receive references to clients and session manager
- [ ] All initialization errors are fatal with clear messages

### Startup Output

- [ ] Log "Starting weave..." at startup
- [ ] Log "Connected to ollama at <url> (model: <model>)" after validation
- [ ] Log "Connected to weave-compute at <path>" after validation
- [ ] Log "Listening on http://localhost:<port>" when ready
- [ ] Log level configurable via `--log-level` flag (default: info)

### Graceful Shutdown

- [ ] Handle SIGTERM and SIGINT signals
- [ ] On signal, log "Shutting down..."
- [ ] Stop accepting new HTTP connections
- [ ] Wait for in-flight requests to complete (timeout: 30 seconds)
- [ ] Close compute client connection
- [ ] Exit with code 0 on clean shutdown

### Configuration Struct

- [ ] All CLI flags parsed into a `Config` struct
- [ ] Config passed to components during initialization
- [ ] Config values logged at DEBUG level on startup

### Testing

- [ ] Unit test: CLI flag parsing works correctly
- [ ] Unit test: default values are applied when flags not provided
- [ ] Integration test: startup succeeds when both dependencies are running
- [ ] Integration test: startup fails with clear error when ollama is not running
- [ ] Integration test: startup fails with clear error when weave-compute is not running
- [ ] Integration test: graceful shutdown completes within timeout
- [ ] Integration test: in-flight request completes before shutdown

### Documentation

- [ ] `docs/DEVELOPMENT.md` includes "Running weave" section
- [ ] `docs/DEVELOPMENT.md` documents all CLI flags with examples
- [ ] `docs/DEVELOPMENT.md` documents startup order (weave-compute first, then ollama, then weave)
- [ ] `docs/DEVELOPMENT.md` includes troubleshooting for common startup errors

## Out of Scope

- Configuration file support (CLI flags only for MVP)
- Hot reloading of configuration
- Health check endpoint
- Metrics endpoint
- Daemon mode (backgrounding)
- Multiple ollama model support

## Dependencies

- Story 004: ollama LLM Client (provides client to initialize)
- Story 005: Conversation Manager (provides session state management)
- Story 006: Web UI Foundation (provides HTTP server)
- Story 002: Unix Socket Communication (provides compute client)

## Notes

This is the "glue" story that brings everything together. The main function should be simple—create components, wire them together, start the server, wait for shutdown signal.

The startup validation is important for user experience. Users should know immediately if dependencies are missing, not discover it when they try to generate an image.

CLI flags mirror the "Hardcoded/Stubbed" table in MVP.md. These values are passed to the compute client when making generation requests.

## Tasks

### 001: Define configuration struct and CLI flags
**Domain:** weave
**Status:** pending
**Depends on:** none

Create `internal/config/config.go` with Config struct containing all CLI flags (port, steps, cfg, width, height, seed, llm-seed, ollama-url, ollama-model). Use standard library flag package or cobra. Implement parsing with defaults per acceptance criteria.

**Files to create:**
- `internal/config/config.go`
- `internal/config/config_test.go`

**Testing:** Unit tests verify flag parsing, default values, validation.

---

### 002: Implement startup validation for ollama
**Domain:** weave
**Status:** pending
**Depends on:** 001

Create `internal/startup/validate.go` with ValidateOllama(url) function. HTTP GET to /api/tags endpoint. Handle connection refused with clear error "ollama not running at <url>". Return nil on success. Call before starting HTTP server.

**Files to create:**
- `internal/startup/validate.go`
- `internal/startup/validate_test.go`

**Testing:** Unit tests verify error messages. Integration test with real ollama (tagged integration).

---

### 003: Implement startup validation for weave-compute
**Domain:** weave
**Status:** pending
**Depends on:** 001

In validate.go, add ValidateCompute() function. Check socket file exists at $XDG_RUNTIME_DIR/weave/weave.sock. Attempt test connection. Handle ENOENT with clear error "weave-compute not running (socket not found)". Handle ECONNREFUSED with clear error. Return nil on success.

**Files to modify:**
- `internal/startup/validate.go`
- `internal/startup/validate_test.go`

**Testing:** Unit tests verify error messages. Integration test with real daemon (tagged integration).

---

### 004: Implement component initialization
**Domain:** weave
**Status:** pending
**Depends on:** 001

Create `internal/startup/init.go` with functions to initialize components: CreateOllamaClient(config), CreateSessionManager(), CreateComputeClient(config), CreateWebServer(config, clients). Wire dependencies (server receives client references). Return initialized components or error.

**Files to create:**
- `internal/startup/init.go`
- `internal/startup/init_test.go`

**Testing:** Unit tests verify initialization, dependency wiring. Integration test verifies components work together.

---

### 005: Implement graceful shutdown handler
**Domain:** weave
**Status:** pending
**Depends on:** 004

Create `internal/startup/shutdown.go` with GracefulShutdown(server, timeout) function. Handle SIGTERM and SIGINT signals. Log "Shutting down...". Stop accepting new connections. Wait for in-flight requests (timeout 30s). Close compute client. Exit with code 0.

**Files to create:**
- `internal/startup/shutdown.go`
- `internal/startup/shutdown_test.go`

**Testing:** Unit tests for signal handling. Integration test verifies graceful shutdown, in-flight request completion.

---

### 006: Implement main entry point
**Domain:** weave
**Status:** pending
**Depends on:** 002, 003, 004, 005

Update `cmd/weave/main.go` to orchestrate startup: parse CLI flags, validate ollama, validate compute, initialize components, start web server, wait for shutdown signal. Log all steps at appropriate levels. Handle errors with clear messages and exit codes.

**Files to modify:**
- `cmd/weave/main.go`

**Testing:** Integration test for full startup/shutdown flow (tagged integration).

---

### 007: Implement --help and --version flags
**Domain:** weave
**Status:** pending
**Depends on:** 001

In config.go, handle --help to show usage with all flags and descriptions. Handle --version to show version string (can be hardcoded "0.1.0-mvp" for now). Both exit cleanly without starting server.

**Files to modify:**
- `internal/config/config.go`
- `internal/config/config_test.go`

**Testing:** Unit tests verify --help and --version output.

---

### 008: Implement structured logging with levels
**Domain:** weave
**Status:** pending
**Depends on:** 001

Create `internal/logging/logger.go` with Logger type supporting DEBUG, INFO, WARN, ERROR levels. Use standard library log package with prefixes or a simple structured logger. Add --log-level flag (default: info). Configure logger early in main().

**Files to create:**
- `internal/logging/logger.go`
- `internal/logging/logger_test.go`

**Testing:** Unit tests verify log level filtering, output formatting.

---

### 009: Add startup logging for all steps
**Domain:** weave
**Status:** pending
**Depends on:** 006, 008

In main.go, add logs for each startup step: "Starting weave...", "Connected to ollama at <url> (model: <model>)", "Connected to weave-compute at <path>", "Listening on http://localhost:<port>". Log config values at DEBUG level.

**Files to modify:**
- `cmd/weave/main.go`

**Testing:** Integration test verifies logs appear correctly at different log levels.

---

### 010: Integration test for dependency validation failures
**Domain:** weave
**Status:** pending
**Depends on:** 002, 003

Create integration tests that simulate missing dependencies (ollama not running, weave-compute not running) and verify startup fails with correct error messages and exit codes.

**Files to create:**
- `internal/startup/validation_failure_test.go` (tagged integration)

**Testing:** Integration tests pass. Verify error messages and exit behavior.

---

### 011: Update DEVELOPMENT.md with startup documentation
**Domain:** documentation
**Status:** pending
**Depends on:** 010

Add "Running weave" section explaining all CLI flags with examples, startup order (weave-compute first, ollama, then weave), troubleshooting for common startup errors (dependencies missing, port conflicts).

**Files to modify:**
- `docs/DEVELOPMENT.md`

**Verification:** Documentation is complete and accurate. Examples work.
