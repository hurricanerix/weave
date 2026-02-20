# Story: Workspace assembly

## Status
In Progress

## Problem
Before a container runs, the orchestrator must prepare an isolated workspace: a copy of the domain's source code with git initialized, the agent config file, and rules files in the locations Claude Code expects. Without proper workspace assembly, the agent would start in an empty or misconfigured directory.

## User/Actor
- Orchestrator (assembles workspace before container start)
- Agent (works inside the assembled workspace)

## Desired Outcome
The orchestrator can assemble a workspace volume from a job's domain config, producing a directory that looks like a normal project to the agent. Source code at the root, `.claude/CLAUDE.md` and `.claude/rules/` in place, git initialized with an initial commit.

## Acceptance Criteria
- [ ] Domain source code is copied to workspace root (domain path prefix stripped)
- [ ] `git init` and initial commit are run in the workspace (provides baseline for `git diff HEAD`)
- [ ] Agent core file is placed at `.claude/CLAUDE.md` in the workspace (read-only in the container)
- [ ] Rules files are placed at `.claude/rules/` in the workspace (read-only in the container)
- [ ] Only rules files specified in the domain's project config are included
- [ ] Spec text is written to `/opt/l5/spec.md` (outside the workspace, in a separate mount path, read-only in the container)
- [ ] Starting patch (if provided by the job) is written to `/opt/l5/starting-patch.diff` (outside workspace, read-only in the container)
- [ ] The l5 staging volume (`/opt/l5/`) is mounted read-only so the agent cannot modify the spec or starting patch
- [ ] Assembly fails with clear error if domain source path does not exist
- [ ] Assembly fails with clear error if agent core file does not exist
- [ ] Workspace is created in a temporary location that can be mounted as a volume
- [ ] Unit tests cover: valid assembly, missing source directory, missing agent config, missing rules file, workspace directory structure verification

## Out of Scope
- Running the container (that is story 038)
- Container image building (that is story 036)
- Patch application to the full project after completion (that is story 041)

## Dependencies
- Story 034 (project config format) -- workspace assembly reads domain config
- Story 033 (agent config split) -- core files must exist to be mounted

## Open Questions
None.

## Notes
See `l5/phase1_details.md` "Workspace volume" section. The workspace source code is a disposable copy -- the agent can freely modify or destroy it. However, the agent config (`.claude/CLAUDE.md`), rules files (`.claude/rules/`), spec, and starting patch are mounted read-only so the agent cannot alter its own instructions or the input specification. The orchestrator uses `./tmp/` (project-local) for workspace staging per project conventions.

## Tasks

### 001: Source code copy and git initialization
**Domain:** l5
**Status:** pending
**Depends on:** none

Create `l5/internal/workspace/` package. Implement the first half of workspace assembly: create a temp directory under `./tmp/` for the workspace, copy the domain's source directory into the workspace root (stripping the domain path prefix so files appear at root level, not under `backend/` etc.), run `git init` in the workspace via `os/exec`, run `git add -A && git commit -m "initial"` to create the baseline commit. Return the workspace directory path. Fail with a clear error if the domain source path does not exist. The copy must be recursive and preserve file permissions.

---

### 002: Agent config placement and spec staging
**Domain:** l5
**Status:** pending
**Depends on:** 001

Extend the workspace assembly to place agent configuration and stage l5 files. In the workspace directory: create `.claude/` directory, copy the agent core file to `.claude/CLAUDE.md`, create `.claude/rules/` directory and copy each rules file listed in the domain config. Set `.claude/CLAUDE.md` and all files under `.claude/rules/` to read-only permissions (0444) so the agent cannot modify its own instructions or rules. Fail with a clear error if the agent core file does not exist. Fail with a clear error if any rules file does not exist. Create a separate l5 staging directory (also under `./tmp/`), write the spec text to `spec.md` in this directory, and if a starting patch is provided, write it to `starting-patch.diff`. Set all files in the l5 staging directory to read-only permissions (0444). The assembly function should accept the domain config (from story 034), spec text (string), and optional starting patch (string), and return a result struct containing paths to both the workspace directory and the l5 staging directory. The result struct should also include the l5 staging path separately so the container runner can mount it read-only. The result struct should also have a `Cleanup()` method that removes both temp directories.

---

### 003: Unit tests for workspace assembly
**Domain:** l5
**Status:** pending
**Depends on:** 001, 002

Create `l5/internal/workspace/workspace_test.go` with tests using `t.TempDir()` to create source directories with known file structures. Cover: (1) valid assembly -- verify workspace has source files at root, `.claude/CLAUDE.md` exists with correct content, `.claude/rules/` contains only specified rules, git repo is initialized with one commit, spec.md exists in l5 staging dir, (2) valid assembly with starting patch -- verify `starting-patch.diff` exists in staging dir with correct content, (3) valid assembly without starting patch -- verify no `starting-patch.diff`, (4) missing source directory -- clear error, (5) missing agent core file -- clear error, (6) missing rules file -- clear error, (7) directory structure verification -- run `git status` in workspace to confirm clean state after initial commit. Call `Cleanup()` and verify temp directories are removed.

---
