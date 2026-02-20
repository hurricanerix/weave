# Story: Container lifecycle management

## Status
In Progress

## Problem
The orchestrator needs to trigger container runs, track their status, enforce timeouts with graceful shutdown, and clean up after completion. Without lifecycle management, containers would need to be started and monitored manually.

## User/Actor
- Orchestrator (manages containers)
- Human (triggers runs via UI, cancels jobs)

## Desired Outcome
The orchestrator can start a podman container for a job, monitor it, enforce timeouts using two-stage shutdown (SIGTERM then SIGKILL), and update job status on completion.

## Acceptance Criteria
- [ ] Orchestrator calls `podman run` with the assembled workspace volume (read-write), l5 staging volume at `/opt/l5/` (read-only), environment variables, and network access
- [ ] Running container ID is tracked and associated with the job
- [ ] Job status transitions to running when container starts
- [ ] Job status transitions to complete when container exits (wrapper POSTs to `/api/agent/complete`)
- [ ] Timeout triggers SIGTERM to the container when job exceeds its configured timeout
- [ ] If container does not exit within 30 seconds of SIGTERM, SIGKILL is sent
- [ ] SIGKILL sets job outcome to killed
- [ ] Cancel action (from UI) sends SIGTERM (same behavior as timeout)
- [ ] Workspace volume is cleaned up after job completes
- [ ] Orchestrator uses `podman` CLI via `os/exec` (not a library)
- [ ] Multiple jobs can be tracked concurrently (even if only one runs at a time in phase 1)
- [ ] Unit tests cover: normal completion, timeout with graceful shutdown, timeout with force kill, cancel, container start failure

## Out of Scope
- Concurrent container execution (phase 1 runs one at a time, but tracking supports multiple)
- Network allowlisting (future phase)
- Container image building (that is story 036, images are pre-built)

## Dependencies
- Story 035 (orchestrator skeleton) -- lifecycle management runs within the orchestrator
- Story 036 (container image and entrypoint) -- image must exist to run
- Story 037 (workspace assembly) -- workspace must be assembled before container start

## Open Questions
None.

## Notes
See `l5/phase1_details.md` "Graceful shutdown" section. The two-stage shutdown (SIGTERM then SIGKILL) ensures partial work is captured when possible. The orchestrator wraps podman CLI calls behind an interface for testability (mock the container runner in unit tests, use real podman in integration tests).

## Tasks

### 001: ContainerRunner interface and podman implementation
**Domain:** l5
**Status:** pending
**Depends on:** none

Create `l5/internal/container/` package. Define a `ContainerRunner` interface with methods: `Run(ctx, image, opts) (containerID, error)` (combines create+start into one call wrapping `podman run -d`), `Stop(ctx, id) error` (sends SIGTERM via `podman stop`), `Kill(ctx, id) error` (sends SIGKILL via `podman kill`), `Wait(ctx, id) (exitCode, error)` (blocks until container exits via `podman wait`), `Remove(ctx, id) error` (removes container via `podman rm`). Implement `PodmanRunner` struct that shells out to the `podman` CLI via `os/exec` for each method. The `Run` method accepts `RunOpts` struct containing: image name, volume mounts (list of `host:container` strings), environment variables (map), and detach flag. All methods accept `context.Context` and respect cancellation.

---

### 002: Job runner with lifecycle orchestration
**Domain:** l5
**Status:** pending
**Depends on:** 001

Create `l5/internal/runner/` package. Implement a `JobRunner` struct that accepts a `ContainerRunner`, a `Store` (from story 035), and a workspace `Assembler` (from story 037). Implement `StartJob(ctx, job, domainConfig) error` that: (1) assembles the workspace, (2) updates job status to running in the store, (3) calls `ContainerRunner.Run` with the workspace volume mounted read-write at `/workspace/`, l5 staging volume mounted read-only at `/opt/l5/` (so the agent cannot modify the spec, starting patch, or other orchestrator-provided files), env vars (`ANTHROPIC_API_KEY` from orchestrator env, `ORCHESTRATOR_URL`, `RUN_ID`=job.ID), and the domain's container image, (4) tracks the container ID associated with the job ID in a concurrent-safe map, (5) launches a goroutine that waits for container exit and triggers cleanup. Implement `CancelJob(ctx, jobID) error` that looks up the container ID and calls `Stop`. Implement cleanup logic that removes the container and calls workspace `Cleanup()` after the container exits. The concurrent map must support multiple tracked jobs even though phase 1 runs one at a time.

---

### 003: Timeout enforcement with two-stage shutdown
**Domain:** l5
**Status:** pending
**Depends on:** 002

Add timeout enforcement to the job runner. When `StartJob` launches the monitoring goroutine, start a timer based on the job's configured timeout (from domain config, with project default fallback). When the timer fires: (1) call `ContainerRunner.Stop` (SIGTERM), (2) start a 30-second grace period timer, (3) call `ContainerRunner.Wait` with a context that expires after 30 seconds, (4) if Wait returns before the grace period, the wrapper handled the SIGTERM (container exited cleanly, wrapper POSTed completion with timeout outcome), (5) if Wait does not return in 30 seconds, call `ContainerRunner.Kill` (SIGKILL), update the job outcome to `killed` in the store (since the wrapper could not POST). Cancel action from the UI uses the same Stop path as timeout (step 1). Ensure the timeout timer is cancelled if the container exits before the timeout.

---

### 004: Unit tests for container lifecycle
**Domain:** l5
**Status:** pending
**Depends on:** 001, 002, 003

Create tests in `l5/internal/runner/runner_test.go` and `l5/internal/container/container_test.go`. Implement a `MockRunner` that satisfies the `ContainerRunner` interface with configurable behavior (return values, delays, errors). Test cases for the job runner: (1) normal completion -- container exits on its own, workspace cleaned up, (2) timeout with graceful shutdown -- timer fires, Stop called, container exits within grace period, outcome is timeout (set by wrapper via API), (3) timeout with force kill -- timer fires, Stop called, container does not exit in 30 seconds, Kill called, outcome set to killed by runner, (4) cancel -- CancelJob called, Stop invoked on correct container, (5) container start failure -- Run returns error, job status updated to complete with error outcome, workspace cleaned up, (6) multiple jobs tracked concurrently -- two jobs tracked in the map, operations on one do not affect the other. Use in-memory SQLite store for tests that touch the database.

---
