# Story: Socket lifecycle management

## Status
Done

## Problem
When weave spawns the compute daemon, compute creates and owns the Unix socket. This creates several production issues:

1. **Orphaned processes** - If weave crashes or is killed, compute keeps running independently, wasting GPU resources and causing conflicts when weave restarts.
2. **Stale socket files** - If compute crashes, socket files are left behind in `/tmp`, causing connection failures on restart.
3. **Startup race conditions** - weave must poll for the socket file to appear after spawning compute, adding latency and unreliability.
4. **No process supervision** - weave cannot easily detect if compute has died or crashed.

This violates the principle that weave is the orchestrator and compute is the worker. The worker should not outlive its manager.

## User/Actor
Developer or operator running weave who expects reliable process management and clean shutdown semantics.

## Desired Outcome
When weave starts:
- Socket is created immediately by weave (no race, no polling)
- Compute spawns and connects to the existing socket
- Compute automatically terminates if weave dies or the socket closes
- No orphaned processes consuming GPU resources
- No stale socket files requiring manual cleanup

## Acceptance Criteria
- [x] weave creates the Unix socket before spawning compute
- [x] weave passes the socket path to compute via CLI argument
- [x] compute connects to the existing socket (does not create it)
- [x] compute monitors both the socket connection and stdin for closure
- [x] compute terminates cleanly if either the socket closes or stdin closes
- [x] When weave exits (clean or crash), compute terminates within 2 seconds
- [x] No orphaned compute processes remain after weave exits
- [x] No stale socket files remain after normal shutdown
- [x] Socket creation errors are reported clearly to the user
- [x] Existing image generation functionality continues to work

## Out of Scope
- Multiple compute instances or socket pooling
- Systemd integration or daemon mode
- Socket permissions or authentication changes
- Process restart policies or recovery strategies
- Migration of existing running instances (restart required)

## Dependencies
None. This is a refactoring of existing socket management code.

## Open Questions
None.

## Notes
This inverts both the ownership model AND the client/server roles:
- **Before:** compute creates socket and accepts connections, weave connects as client
- **After:** weave creates socket and accepts connections, compute connects as client

Key architectural points:
- weave is the server (creates listening socket, accepts connections)
- compute is the client (connects once, persistent connection)
- All requests/responses flow over single connection
- compute runs request/response loop until connection or stdin closes
- When connection closes, compute terminates

This is a standard parent-child process pattern. The parent (weave) owns the resource (socket), and the child (compute) uses it. When the parent dies, the child should clean up.

The stdin monitoring provides a fallback mechanism - if the socket somehow remains open but weave dies, compute will still detect the pipe closure and terminate.

This change requires coordinated updates to both weave (Go) and compute (C) codebases.

## Tasks

### 001: Add socket_connect function to compute
**Domain:** compute
**Status:** done
**Depends on:** none

Add a new function `socket_connect()` to `compute-daemon/src/socket.c` that connects to an existing Unix socket (instead of creating it). The function should take a socket path as a parameter, create a socket file descriptor, connect to it, and return the connected socket. This mirrors the client connection logic but is for the daemon's server-side use. Update `weave/socket.h` with the function declaration. Include error handling for connection failures.

---

### 002: Add CLI argument parsing to compute main
**Domain:** compute
**Status:** done
**Depends on:** none

Modify `compute-daemon/src/main.c` to accept a `--socket-path` CLI argument. Parse the argument using standard getopt or manual parsing. Store the socket path for use during socket initialization. If the argument is not provided, fall back to the current behavior (calling `socket_get_path()` to construct the default path). This maintains backward compatibility during the transition.

---

### 003: Implement client connection loop in compute
**Domain:** compute
**Status:** done
**Depends on:** 001, 002

Modify `compute-daemon/src/main.c` to connect as a client to weave's listening socket using `socket_connect()`. After connecting, replace the accept loop with a request/response loop that reads requests from the connected socket, processes them, and sends responses back over the same connection. The loop continues until the connection closes or stdin closes. Remove `socket_create()` and `socket_cleanup()` calls since compute no longer owns the socket file. Update error messages to reflect client connection failures.

---

### 004: Add stdin monitoring thread to compute
**Domain:** compute
**Status:** done
**Depends on:** 003

Add a background monitoring mechanism in `compute-daemon/src/main.c` that watches stdin for closure. When stdin is closed (indicating parent process death), call `socket_request_shutdown()` to trigger graceful termination. Use a simple read loop in a separate thread (pthread) or select/poll on stdin. This provides the fallback mechanism for detecting parent process death even if the socket connection remains open.

---

### 005: Add socket creation to weave startup
**Domain:** weave
**Status:** done
**Depends on:** none

Create a new function in `internal/startup/init.go` that creates the Unix socket before spawning compute. The function should construct the socket path from XDG_RUNTIME_DIR, create the socket directory with mode 0700, create and bind the socket, and start listening. Store the listening socket file descriptor for later use. Handle and report socket creation errors clearly to the user. This uses Go's `net.Listen("unix", socketPath)`. Note: CreateSocket() already exists but needs integration into startup flow.

---

### 006: Add compute process spawning to weave startup
**Domain:** weave
**Status:** done
**Depends on:** 005

Add a function to `internal/startup/init.go` that spawns the compute daemon as a child process using `exec.Command()`. Pass the socket path via the `--socket-path` CLI argument. Set up proper stdio pipes so stdin can be used for lifecycle monitoring. Start the process and store the `*exec.Cmd` in the Components struct for later cleanup. Handle spawn failures with clear error messages.

---

### 007: Implement request multiplexing in weave
**Domain:** weave
**Status:** done
**Depends on:** 006

Modify `internal/client/socket.go` to accept compute's single persistent connection and multiplex requests over it. Add `AcceptConnection(listener net.Listener)` that accepts compute's connection and returns a `*Conn`. The connection should persist for the lifetime of the compute process. Implement request/response matching (likely using request IDs) to handle concurrent requests over the single connection. Update web handlers to send requests to compute via this persistent connection. This replaces the current dial-per-request pattern.

---

### 008: Add compute process cleanup to weave shutdown
**Domain:** weave
**Status:** done
**Depends on:** 006

Add cleanup logic to `internal/startup/shutdown.go` that terminates the compute process when weave exits. Send SIGTERM to the compute process, wait up to 2 seconds for graceful shutdown, then send SIGKILL if still running. Close the listening socket file descriptor. Remove the socket file from the filesystem. Ensure cleanup happens on both normal shutdown and crash paths.

---

### 009: Update integration tests for new lifecycle
**Domain:** weave
**Status:** done
**Depends on:** 003, 007

Update `test/integration/socket_test.go` to test the new client/server role reversal. Add tests that verify: weave creates listening socket, weave accepts compute's connection, compute runs persistent request/response loop, compute terminates when weave closes connection, compute terminates when weave exits, no orphaned processes remain, no stale socket files remain after shutdown. Test request multiplexing over the single persistent connection. Update helper functions to work with the new architecture.

---

### 010: Update compute socket tests for client mode
**Domain:** compute
**Status:** done
**Depends on:** 001, 003

Update `compute-daemon/test/test_socket.c` to test the new client connection pattern. Add tests that verify: `socket_connect()` connects to existing listening socket, connection to non-existent socket fails with appropriate error, request/response loop reads and writes over connected socket, loop terminates cleanly on connection close. Test the client request loop in isolation from the full daemon. Ensure existing `socket_create()` tests still pass for backward compatibility.

---
