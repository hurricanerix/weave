# Story: l5-relay binary

## Status
In Progress

## Problem
l5 needs a way to stream journal entries from inside containers to the orchestrator in real time. Claude Code produces streaming JSON on stdout, but there is no mechanism to forward that to the orchestrator as it happens. Without live forwarding, the human has no visibility into what the agent is doing until the container exits.

## User/Actor
- l5 orchestrator (consumes journal data)
- Human (watches journal in UI)

## Desired Outcome
A small Go binary that reads streaming JSON from stdin and POSTs each entry to a configurable HTTP endpoint. It runs inside the container, piped to Claude Code's stdout, and the orchestrator receives journal entries in real time.

## Acceptance Criteria
- [ ] l5-relay reads JSON objects from stdin one at a time
- [ ] Each JSON object is POSTed to `$ORCHESTRATOR_URL/api/agent/journal`
- [ ] `RUN_ID` from environment variable is included with each request
- [ ] l5-relay exits cleanly when stdin closes (exit code 0)
- [ ] POST failures are logged to stderr and do not stop processing (best-effort delivery)
- [ ] l5-relay builds as a static binary (`go build`)
- [ ] Unit tests cover: valid JSON input, malformed JSON handling, stdin close, POST failure resilience

## Out of Scope
- The orchestrator's journal endpoint (that is story 035)
- Integration with the container image (that is story 036)
- Any transformation or filtering of the JSON data -- l5-relay forwards raw

## Dependencies
None. This is the first piece of l5 code.

## Open Questions
None.

## Notes
See `l5/phase1_details.md` for full design context. l5-relay is intentionally minimal -- it reads stdin, POSTs, exits. Stateless and dumb by design.

## Tasks

### 001: Initialize l5 Go module and directory structure
**Domain:** l5
**Status:** pending
**Depends on:** none

Create the l5 Go module as a separate module from the backend (`l5/go.mod` with module path `github.com/hurricanerix/l5`). Create the directory structure: `l5/cmd/l5-relay/`, `l5/internal/relay/`. Add a stub `main.go` that compiles and exits. Verify `go build ./cmd/l5-relay` succeeds from the `l5/` directory. This establishes the project layout following the same `cmd/` + `internal/` pattern as the backend.

---

### 002: Implement relay core logic
**Domain:** l5
**Status:** pending
**Depends on:** 001

Create `l5/internal/relay/relay.go`. Implement a `Run` function that accepts an `io.Reader` (stdin), an HTTP endpoint URL, a run ID string, and a logger (stderr writer). The function reads JSON objects from the reader using `json.Decoder`, POSTs each raw JSON object to the endpoint URL with the run ID included (as a header or query parameter), logs POST failures to the logger without stopping, and returns nil when the reader reaches EOF. Accept interfaces for testability -- the HTTP posting should go through an interface or function parameter so tests can mock network calls.

---

### 003: Unit tests for relay core
**Domain:** l5
**Status:** pending
**Depends on:** 002

Create `l5/internal/relay/relay_test.go` with table-driven tests covering the acceptance criteria: (1) valid JSON input -- multiple JSON objects are each forwarded, (2) malformed JSON -- relay logs error and continues to next valid object, (3) stdin close -- relay returns nil (clean exit), (4) POST failure -- relay logs error to stderr and continues processing remaining objects. Use `httptest.NewServer` to verify HTTP requests are made correctly and to simulate failures. Verify `RUN_ID` is included in each request.

---

### 004: Wire main entrypoint and add Makefile target
**Domain:** l5
**Status:** pending
**Depends on:** 002

Complete `l5/cmd/l5-relay/main.go`: read `ORCHESTRATOR_URL` and `RUN_ID` from environment variables, validate both are set (exit 1 with clear error if missing), call `relay.Run` with `os.Stdin`, and exit 0 on success or 1 on error. Add an `l5-relay` build target to the top-level `Makefile` that runs `cd l5 && go build -o bin/l5-relay ./cmd/l5-relay`. Verify the binary builds and runs (exits immediately with error when env vars are missing, exits 0 when stdin is empty and env vars are set).

---
