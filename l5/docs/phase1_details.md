# Phase 1: Design details

Detailed design decisions made during architecture discussions. Supplements `l5_phase1_plan.md` with implementation-level specifics.

## Container image

The project owns the base image. l5 layers its tooling on top via a single parameterized Dockerfile.

```dockerfile
ARG BASE_IMAGE
FROM ${BASE_IMAGE}

RUN apt-get update && apt-get install -y git

ENV L5_HOME=/opt/l5
COPY claude $L5_HOME/bin/claude
COPY l5-relay $L5_HOME/bin/l5-relay
COPY l5-entrypoint.sh $L5_HOME/bin/l5-entrypoint

ENTRYPOINT ["/opt/l5/bin/l5-entrypoint"]
```

**Key decisions:**
- Claude Code is a native binary. No Node.js dependency.
- All l5 tooling lives under `/opt/l5/` to avoid conflicting with the project's toolchain.
- One Dockerfile, parameterized with `BASE_IMAGE`. Any Debian-based image works.
- Phase 1 uses `golang:1.22-bookworm` as the base for the backend domain.
- Constraint: Debian-based images only for now (`apt-get` assumed). Acceptable for phase 1.

**l5 layer contents:**
- `claude` -- Claude Code CLI binary
- `l5-relay` -- Small Go binary that forwards journal entries to the orchestrator
- `l5-entrypoint.sh` -- Wrapper script (container entrypoint)
- `git` -- Installed via apt

## Container entrypoint (wrapper script)

The wrapper script is the container's entrypoint. It runs Claude Code, captures output, collects the patch, and reports results to the orchestrator.

```bash
#!/bin/bash
set -euo pipefail

# Apply starting patch if provided
if [ -f /opt/l5/starting-patch.diff ]; then
    cd /workspace
    git apply /opt/l5/starting-patch.diff
    git add -A
    git commit -m "starting patch"
fi

# Handle graceful shutdown on timeout
trap 'cleanup_and_exit timeout' SIGTERM

cleanup_and_exit() {
    cd /workspace
    git add -A
    git diff HEAD > /tmp/patch.diff
    curl -X POST "$ORCHESTRATOR_URL/api/agent/complete" \
      -F "patch=@/tmp/patch.diff" \
      -F "outcome=$1"
    exit 0
}

# Run Claude Code, pipe output to l5-relay for live journal streaming
cd /workspace
claude -p "$(cat /opt/l5/spec.md)" --output-format json | l5-relay
EXIT_CODE=${PIPESTATUS[0]}

# Collect patch
git add -A
git diff HEAD > /tmp/patch.diff

# Determine outcome
if [ "$EXIT_CODE" -ne 0 ]; then
    OUTCOME="error"
elif [ -s /tmp/patch.diff ]; then
    OUTCOME="success"
else
    OUTCOME="empty"
fi

curl -X POST "$ORCHESTRATOR_URL/api/agent/complete" \
  -F "patch=@/tmp/patch.diff" \
  -F "outcome=$OUTCOME"
```

**Key decisions:**
- Claude Code runs in print mode (`-p`) and exits naturally when done. No explicit termination needed.
- `--output-format json` produces streaming JSON to stdout.
- Output is piped to `l5-relay`, which forwards each JSON chunk to the orchestrator's journal endpoint in real time.
- After Claude Code exits, the wrapper stages all changes (`git add -A`) and generates a patch (`git diff HEAD`).
- The wrapper POSTs the patch and outcome to the orchestrator's completion endpoint.
- The agent has no awareness of the pipeline. It writes to stdout like normal. The pipe is invisible to it.
- If a starting patch is provided, it is applied and committed before Claude Code runs. This means the result patch contains only new work.

### Graceful shutdown (timeout)

The orchestrator uses a two-stage shutdown when a job exceeds its time limit:

1. **SIGTERM to wrapper.** The wrapper catches it, runs `git add -A && git diff HEAD` to capture whatever partial patch exists, POSTs it with outcome `timeout`, and exits cleanly.
2. **SIGKILL after grace period.** If the wrapper does not exit within the grace period (e.g., 30 seconds), the orchestrator force-kills the container. No patch is captured.

This ensures partial work is preserved on timeout. Force-kill is the last resort.

## l5-relay

A small Go binary that reads JSON from stdin and POSTs each chunk to the orchestrator's journal endpoint as it arrives.

**Responsibilities:**
- Read streaming JSON from stdin (Claude Code's `--output-format json` output)
- POST each JSON object to `$ORCHESTRATOR_URL/api/agent/journal`
- Exit when stdin closes (Claude Code finished)

**Key decisions:**
- Stateless. Reads stdin, POSTs, done.
- Env vars provide orchestrator URL and run ID.
- This is the first piece of l5 code to build. Small, testable, useful immediately.

## Observability

**Design principle:** The orchestrator controls the pipeline, the agent is just cargo. Anything captured without relying on the agent is more reliable than anything that requires agent cooperation.

- **Live journal:** l5-relay taps Claude Code's stdout pipe and forwards JSON to the orchestrator in real time. The agent does not know this is happening.
- **Post-hoc session data:** The orchestrator already has the full session because l5-relay forwarded every chunk. No separate `session.json` artifact needed.
- **Crash recovery:** If the container dies mid-run, the orchestrator has everything up to the crash point. No lost data.

## Isolation and safety

**Copy-and-patch model.** The agent only has a copy of domain source code. The original is never at risk.

- The orchestrator copies domain source into a volume.
- The volume is mounted into the container.
- The agent can freely modify, delete, or corrupt anything in the volume.
- Worst case outcome: a bad patch (rejected by human) or an error (wrapper can't generate patch).
- The original source is never mounted into the container.

## Workspace volume (`/workspace/`)

The orchestrator prepares the workspace volume before container start:

1. Copy domain source code (e.g., contents of `backend/`)
2. Run `git init` + initial commit (provides baseline for `git diff HEAD`)
3. Place `.claude/CLAUDE.md` (agent config for this domain)
4. Place `.claude/rules/` (only the rules files relevant to this domain)

**Key decisions:**
- Source is copied at the workspace root. The domain path prefix (e.g., `backend/`) is stripped. The agent works as if the domain is the whole project.
- When the orchestrator applies the patch to the full project, it uses `git apply --directory=<domain-prefix>/` to restore the correct paths.
- `.claude/CLAUDE.md` is copied from the domain's agent core file (see "Agent config split" below) so Claude Code picks it up automatically. Set to read-only (0444) so the agent cannot modify its own instructions.
- Rules files under `.claude/rules/` are set to read-only (0444) so the agent cannot alter them.
- Only relevant rules files are included (e.g., backend domain gets `go.md`, `protocol.md`, not `c.md`).
- Skills (slash commands) are not mounted. They are interactive and do not apply in headless container runs.

## Spec file (`/opt/l5/spec.md`)

The human's specification is mounted outside the workspace volume so it does not appear in the agent's file listing or in the generated patch.

- Orchestrator writes the spec to a file.
- File is mounted at `/opt/l5/spec.md`.
- The entire `/opt/l5/` staging volume is mounted read-only so the agent cannot modify the spec or any other orchestrator-provided files.
- The wrapper reads it and passes the content as Claude Code's `-p` prompt argument.
- The agent receives the spec as its instruction, never sees it as a file.

## Starting patch (`/opt/l5/starting-patch.diff`)

Optional. When the human creates a new job, they can provide a starting patch (e.g., partial work from a previous timed-out run).

- If present, the wrapper applies it to the workspace and commits before running Claude Code.
- The result patch contains only the new work (baseline is after starting patch).
- Mounted at `/opt/l5/starting-patch.diff`, outside the workspace (read-only with the rest of `/opt/l5/`).

## Environment variables

Passed to the container at `podman run` time. Never baked into the image.

| Variable | Purpose |
|----------|---------|
| `ANTHROPIC_API_KEY` | Claude Code authentication |
| `ORCHESTRATOR_URL` | Where l5-relay sends journal entries and wrapper sends completion |
| `RUN_ID` | Identifies this run in all callbacks to the orchestrator |

## Networking

- Container must reach Anthropic's API (for Claude Code).
- Container must reach the orchestrator (for l5-relay and wrapper callbacks).
- Strict network allowlisting deferred to a later phase. Acceptable for phase 1 since the blast radius is bounded by the copy-and-patch model.

## Patch application

Patches generated inside the container have paths relative to the workspace root (domain source at root). The full project has domain source under a subdirectory.

**Mapping:** `git apply --directory=<domain-prefix>/ patch.diff`

This prepends the domain path prefix to every path in the patch. Creates, deletes, and renames all get the prefix applied. The orchestrator knows the prefix from the project config.

**Example:** Workspace diff says `internal/scheduler/scheduler.go`. Applied with `--directory=backend/`, it becomes `backend/internal/scheduler/scheduler.go`.

## Job lifecycle and outcomes

### Job status

A job has three statuses:

| Status | Meaning |
|--------|---------|
| Pending | Created, not yet run |
| Running | Container is active |
| Complete | Container exited (one way or another) |

Every job that runs reaches "complete." The container always finishes -- either on its own, via graceful shutdown, or via force-kill.

### Job outcomes

The outcome describes how the job completed:

| Outcome | Condition |
|---------|-----------|
| Success | Exit 0, patch generated |
| Error | Non-zero exit or patch generation failed |
| Timeout | Orchestrator signaled wrapper, wrapper captured partial patch and exited |
| Empty | Exit 0, no changes made |
| Killed | Wrapper did not respond to signal, container was force-killed. No patch captured. |

Timeout is a normal operational outcome -- the agent ran out of time but partial work is preserved. Killed is an anomaly worth investigating.

### Data model

Each job record contains:

| Field | Description |
|-------|-------------|
| Spec text | The human's specification |
| Patch input | Optional starting patch (applied before agent runs) |
| Status | Pending, running, complete |
| Outcome | Success, error, timeout, empty, killed (set when complete) |
| Journal entries | Streaming log from l5-relay |
| Patch output | Result diff (when available) |

## Agent API

Endpoints called by the container (l5-relay and wrapper). Not human-facing.

**`POST /api/agent/journal`** -- Called by l5-relay during the run. One journal entry per request.

**`POST /api/agent/complete`** -- Called by wrapper after Claude Code exits (or on SIGTERM). Carries the patch (if any) and the outcome.

## UI

### Layout

**Left column:** Job list with status badges (pending, running, complete) and outcome indicators on complete jobs.

**Right panel:** Tabbed detail view. Tab content and editability depend on job status.

### Tabs by status

| Status | Tabs |
|--------|------|
| Pending | Spec (editable), Patch Input (editable) |
| Running | Spec (read-only), Patch Input (read-only), Journal (live) |
| Complete | Spec (read-only), Patch Input (read-only), Journal, Patch Output (if available) |

Complete jobs display the same tabs regardless of outcome (success, error, timeout, empty, killed). The outcome badge tells the human what happened. Patch Output tab is present but may be empty for error/killed outcomes.

### Actions by status

| Status | Actions |
|--------|---------|
| Pending | Save, Delete, Run |
| Running | Cancel |
| Complete | Delete |

Cancel sends SIGTERM to the container (graceful shutdown). The job completes with timeout outcome.

### Live updates (SSE)

One SSE connection carries two event types:
- **Journal entries** -- for the right panel when viewing a running job
- **Status/outcome changes** -- for the left column so all job badges update in real time

### Retry workflow

No built-in retry mechanism. To retry a failed/timed-out job:
1. Create a new job
2. Copy the spec from the old job (modify as needed)
3. Optionally paste the old job's patch output as the new job's patch input
4. Run the new job

Each job is independent. No parent-child relationships.

## Agent config split

Agent definitions are split into two files to serve both the current local workflow and l5 containers:

**Core file** (e.g., `backend-developer-core.md`): The agent's complete instructions -- role, domain, process, code standards, boundary rules. No frontmatter. This is the single source of truth for the agent's behavior.

**Agent file** (e.g., `backend-developer.md`): Frontmatter (name, model, allowedTools) plus a reference to the core file. Used by Claude Code's subagent system in the current local workflow.

```markdown
---
name: backend-developer
model: sonnet
allowedTools: ["Read", "Write", "Edit", "Bash", "Grep", "Glob"]
---

Read and follow all instructions in `.claude/agents/backend-developer-core.md`.
```

**l5 usage:** The project config points to the core file. The orchestrator copies it into the workspace as `.claude/CLAUDE.md`. Claude Code in headless mode reads it automatically.

**Why this split:**
- One source of truth for agent instructions (the core file).
- No duplication between local and container workflows.
- Agent config is mounted at runtime, not baked into the container image. Four Go projects can share the same built image with different agent configs.
- Changing agent instructions does not require a container image rebuild.

## Project config

YAML format. Defines what the orchestrator needs to know about the project and its domains.

```yaml
project: weave
root: /home/user/workspace/weave
timeout: 10m

domains:
  backend:
    path: backend/
    base_image: golang:1.22-bookworm
    agent: .claude/agents/backend-developer-core.md
    rules:
      - .claude/rules/go.md
      - .claude/rules/protocol.md
    timeout: 30m

  compute:
    path: compute/
    base_image: gcc:14-bookworm
    agent: .claude/agents/compute-developer-core.md
    rules:
      - .claude/rules/c.md
      - .claude/rules/protocol.md
```

| Field | Level | Description |
|-------|-------|-------------|
| `project` | Top | Project name (display only) |
| `root` | Top | Absolute path to project root |
| `timeout` | Top | Default timeout for all domains (default: 10m) |
| `domains` | Top | Map of domain name to domain config |
| `path` | Domain | Source directory relative to project root |
| `base_image` | Domain | Docker base image for this domain |
| `agent` | Domain | Path to agent core file, relative to project root |
| `rules` | Domain | List of rules files to mount, relative to project root |
| `timeout` | Domain | Override timeout for this domain (optional) |

**Key decisions:**
- All paths are relative to `root` except `root` itself.
- `timeout` at domain level overrides the project default.
- Phase 1 only supports `domains` (implementation agents). Future phases will add sibling sections for other agent roles (see below).

### Future config evolution

The `domains` section covers implementation agents -- they receive source code and produce patches. Future phases introduce agents with different inputs and outputs:

| Phase | Agent role | Input | Output |
|-------|-----------|-------|--------|
| 1 | Implementation | Domain source + spec | Patch |
| 2 | Code review | Domain source + patch | Verdict |
| 2 | QA review | Full repo + patches | Verdict |
| 2 | Security review | Full repo + patches | Verdict |
| 3 | Cross-domain negotiation | Spec + domain list | Interface contract |

These will be added as new config sections (e.g., `reviewers`, `pipeline`) rather than overloading `domains`. The container mechanics are the same -- base image + l5 layer, agent config mounted, Claude Code runs, artifact produced. The difference is what gets mounted and what artifact is expected.

The current config structure will not need to change. It grows by addition.
