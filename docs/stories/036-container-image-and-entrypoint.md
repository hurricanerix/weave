# Story: Container image and entrypoint

## Status
In Progress

## Problem
Claude Code needs to run in an isolated container with l5 tooling layered on top of a project-chosen base image. The container needs an entrypoint script (wrapper) that handles: running Claude Code, piping output to l5-relay, applying starting patches, collecting the result patch, reporting outcomes, and graceful shutdown on timeout.

## User/Actor
- Orchestrator (builds and runs containers)

## Desired Outcome
A parameterized Dockerfile and entrypoint script that can run Claude Code headlessly in any Debian-based container, handle all completion scenarios, and report results back to the orchestrator.

## Acceptance Criteria
- [ ] Dockerfile accepts `BASE_IMAGE` as a build argument
- [ ] All l5 tooling is installed under `/opt/l5/` (Claude Code binary, l5-relay binary, wrapper script)
- [ ] Git is installed via apt
- [ ] Wrapper runs Claude Code with `-p` flag, pipes `--output-format json` output to l5-relay
- [ ] Wrapper applies starting patch if `/opt/l5/starting-patch.diff` exists (git apply, git add, git commit)
- [ ] Wrapper runs `git add -A && git diff HEAD` after Claude Code exits to capture the patch
- [ ] Wrapper POSTs patch and outcome to `$ORCHESTRATOR_URL/api/agent/complete`
- [ ] Wrapper detects outcome: success (exit 0, non-empty diff), error (non-zero exit), empty (exit 0, empty diff)
- [ ] Wrapper handles SIGTERM: stops Claude Code, captures partial patch, POSTs with timeout outcome, exits cleanly
- [ ] Image builds successfully with `golang:1.22-bookworm` as base
- [ ] Image builds successfully with `gcc:14-bookworm` as base (proves base-agnostic)

## Out of Scope
- Orchestrator triggering podman run (that is story 038)
- Workspace assembly (that is story 037)
- Network allowlisting (future phase)

## Dependencies
- Story 032 (l5-relay binary) -- l5-relay is copied into the image

## Open Questions
None.

## Notes
See `l5/phase1_details.md` "Container image" and "Container entrypoint" sections for full design. The wrapper handles SIGTERM with a trap, giving it a chance to capture partial work before the orchestrator escalates to SIGKILL. Environment variables (`ANTHROPIC_API_KEY`, `ORCHESTRATOR_URL`, `RUN_ID`) are passed at runtime, never baked in.

## Tasks

### 001: Create parameterized Dockerfile
**Domain:** l5
**Status:** pending
**Depends on:** none

Create `l5/container/Dockerfile` matching the design in `phase1_details.md`. Accept `BASE_IMAGE` as a build argument (`ARG BASE_IMAGE`). Install `git` and `curl` via `apt-get` (curl is needed by the wrapper for POSTing results). Set `L5_HOME=/opt/l5` as env var. COPY `claude`, `l5-relay`, and `l5-entrypoint.sh` into `$L5_HOME/bin/`. Make the entrypoint script executable. Set `ENTRYPOINT ["/opt/l5/bin/l5-entrypoint"]`. Add `/opt/l5/bin` to `PATH` so `claude` and `l5-relay` are available without full paths.

---

### 002: Create entrypoint wrapper script
**Domain:** l5
**Status:** pending
**Depends on:** none

Create `l5/container/l5-entrypoint.sh` following the design in `phase1_details.md`. The script must handle five scenarios: (1) Starting patch -- if `/opt/l5/starting-patch.diff` exists (read-only mount), `cd /workspace && git apply` it, `git add -A`, `git commit -m "starting patch"`. (2) Normal execution -- run `claude -p "$(cat /opt/l5/spec.md)" --output-format json | l5-relay`, capture exit code via `PIPESTATUS[0]`. (3) Patch collection -- `git add -A && git diff HEAD > /tmp/patch.diff` (write to `/tmp/` since `/opt/l5/` is read-only). (4) Outcome detection -- exit code non-zero→error, exit 0 + non-empty diff→success, exit 0 + empty diff→empty. (5) SIGTERM trap -- catch SIGTERM, capture partial patch, POST with outcome=timeout, exit 0. Both normal exit and SIGTERM paths POST to `$ORCHESTRATOR_URL/api/agent/complete` with `-F "run_id=$RUN_ID" -F "outcome=$OUTCOME" -F "patch=@/tmp/patch.diff"`. Use `set -euo pipefail` but handle the pipe exit code manually (set +e around the pipe). Validate required env vars (`ORCHESTRATOR_URL`, `RUN_ID`) at script start and exit 1 with clear error if missing.

---

### 003: Verify image builds with two base images
**Domain:** l5
**Status:** pending
**Depends on:** 001, 002

Build the container image twice to verify base-agnostic behavior: once with `golang:1.22-bookworm` and once with `gcc:14-bookworm`. For each build, provide stub binaries in the build context (empty files or minimal scripts that exit 0) for `claude` and `l5-relay` since the real binaries are not needed to test the Dockerfile. Verify both builds succeed, the entrypoint script is at `/opt/l5/bin/l5-entrypoint` and is executable, `git` and `curl` are available in the image, and environment variable `L5_HOME` is set. Document the build commands in a `l5/container/README.md` (build instructions only, no marketing).

---
