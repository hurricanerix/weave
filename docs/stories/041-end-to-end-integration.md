# Story: End-to-end integration

## Status
In Progress

## Problem
All phase 1 components exist individually but have not been tested together. The core thesis of l5 -- "spec in, patch out" -- is unproven. Without an end-to-end test, there is no evidence that the components work together to produce a usable result.

## User/Actor
- Human (submits a spec, evaluates the resulting patch)

## Desired Outcome
A complete run through the entire pipeline: human submits a spec for Weave's backend domain through the UI, a container runs Claude Code against a copy of the backend source, journal entries stream to the UI, and a patch appears that can be applied to the full project.

## Acceptance Criteria
- [ ] Project config exists for Weave with the backend domain defined (path, base image, agent core, rules)
- [ ] Container image is built for the backend domain (golang base + l5 layer)
- [ ] Human creates a job with a spec targeting the backend domain
- [ ] Container runs with correct workspace (backend source at root, .claude/CLAUDE.md, .claude/rules/)
- [ ] Journal entries stream to the UI during execution
- [ ] Patch appears in the Patch Output tab on completion
- [ ] Patch can be successfully applied to the full Weave project with `git apply --directory=backend/`
- [ ] Timeout scenario tested: job exceeds time limit, SIGTERM sent, partial patch captured and visible in UI
- [ ] Starting patch scenario tested: job is created with a patch input, agent starts with that patch applied, result patch contains only new work
- [ ] At least one successful end-to-end run producing a meaningful patch (not just an empty diff)

## Out of Scope
- Automated patch application to the real project (human applies manually in phase 1)
- Multi-domain stories (phase 3)
- Code review or QA containers (phase 2)
- Performance benchmarking

## Dependencies
- All previous l5 stories (032-040)

## Open Questions
- What spec produces a reliable, testable result for the first run? A small, well-defined task against the backend (e.g., "add a health check endpoint") would be ideal.

## Notes
This story validates the phase 1 thesis: isolation works, specs produce patches, and the human evaluates outcomes. See `l5/l5_plan.md` Phase 1 for the hypothesis being tested. A successful run here means phase 1 is complete and phase 2 design can begin.

## Tasks

### 001: Create Weave project config and build container image
**Domain:** l5
**Status:** pending
**Depends on:** none

Create `l5.yaml` at the project root defining Weave's backend domain: project name `weave`, root pointing to the project directory, backend domain with `path: backend/`, `base_image: golang:1.22-bookworm`, `agent: .claude/agents/backend-developer-core.md`, rules `[.claude/rules/go.md, .claude/rules/protocol.md]`, and a short timeout for testing (e.g., 5m). Verify the config parser (story 034) loads it without errors. Build the l5-relay binary (story 032). Build the container image using the l5 Dockerfile (story 036) with `golang:1.22-bookworm` as base, providing the real l5-relay binary and the real Claude Code binary in the build context. Verify the image builds and `podman run --rm <image> --help` or similar shows the entrypoint is reachable. Document the build commands in `l5/README.md`.

---

### 002: Happy path -- spec to patch
**Domain:** l5
**Status:** pending
**Depends on:** 001

Start the orchestrator (`l5-orchestrator --config l5.yaml`). Open the UI in a browser. Create a new job targeting the backend domain with a small, well-defined spec (e.g., "Add a GET /healthz endpoint to the web server that returns HTTP 200 with body ok. Add a unit test."). Run the job. Verify: (1) job status transitions to running in the UI, (2) journal entries stream to the Journal tab in real time, (3) container exits and job transitions to complete with success outcome, (4) Patch Output tab shows a non-empty diff, (5) copy the patch to a file and run `git apply --directory=backend/ patch.diff` on the full Weave project -- it should apply cleanly. Fix any integration issues discovered during this run (mismatched field names, wrong mount paths, timing bugs, etc.). This task is expected to involve debugging and fixing cross-component issues.

---

### 003: Timeout scenario
**Domain:** l5
**Status:** pending
**Depends on:** 002

Create a job with a very short timeout (e.g., override domain timeout to 1m or use a spec complex enough to exceed the limit). Run the job. Verify: (1) when timeout fires, the orchestrator sends SIGTERM to the container, (2) the wrapper captures a partial patch and POSTs it with timeout outcome, (3) job transitions to complete with timeout outcome badge in the UI, (4) Patch Output tab shows the partial patch (may be empty if agent made no changes in the time window, which is acceptable -- the mechanism working is what matters), (5) journal entries up to the point of termination are visible in the UI. If SIGTERM handling does not work (wrapper does not exit within 30 seconds), verify the SIGKILL fallback fires and job outcome is set to killed.

---

### 004: Starting patch scenario
**Domain:** l5
**Status:** pending
**Depends on:** 002

Create a new job targeting the backend domain. In the Patch Input tab, paste a known valid patch (e.g., the patch output from the successful run in task 002, or a hand-crafted diff that adds a comment to a file). Write a spec that builds on the starting patch (e.g., "There is a /healthz endpoint. Add a /readyz endpoint that checks the database connection."). Run the job. Verify: (1) the wrapper applies the starting patch before running Claude Code (check journal for the "starting patch" commit), (2) the result patch in Patch Output contains only the new work (not the starting patch changes), (3) applying the result patch with `git apply --directory=backend/` on a working copy that already has the starting patch applied succeeds. This proves the iterative workflow: a timed-out or partial run can be continued by feeding its output as the next job's input.

---
