# Story: Project config format

## Status
In Progress

## Problem
The l5 orchestrator needs to know about a project's domains -- where source code lives, what container image to use, which agent config and rules files to mount, and timeout values. This information must come from a configuration file so l5 remains project-agnostic. Without a config format, domain knowledge would be hardcoded.

## User/Actor
- Human (writes and maintains config file)
- l5 orchestrator (reads and validates config)

## Desired Outcome
A YAML config file format with a Go parser that the orchestrator can use to understand project structure. Defines project-level defaults and per-domain settings.

## Acceptance Criteria
- [ ] Config file is YAML format
- [ ] Supports project-level fields: project name, root path, default timeout
- [ ] Supports per-domain fields: source path, base image, agent core file path, rules files list, optional timeout override
- [ ] Default timeout is 10 minutes if not specified at any level
- [ ] Domain timeout overrides project-level timeout
- [ ] All paths are relative to root (except root itself which is absolute)
- [ ] Config parser rejects invalid configs with clear error messages (missing required fields, nonexistent paths)
- [ ] Config parser validates that referenced files (agent core, rules) exist on disk
- [ ] Unit tests cover: valid config, missing required fields, invalid timeout, path validation, timeout override precedence

## Out of Scope
- Orchestrator web server (that is story 035)
- Multi-domain pipeline orchestration (phase 3)
- Reviewer or pipeline stage configuration (future phases)

## Dependencies
None. Config parsing is independent of other l5 components.

## Open Questions
None.

## Notes
See `l5/phase1_details.md` "Project config" section for the config format specification. Example config:

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
```

## Tasks

### 001: Define config types and add YAML dependency
**Domain:** l5
**Status:** pending
**Depends on:** none

Create `l5/internal/config/` package. Define Go structs for the project config: `ProjectConfig` (project name, root path, default timeout as `time.Duration`, domains map) and `DomainConfig` (path, base image, agent path, rules list, optional timeout override). Add `yaml` struct tags matching the YAML field names from `phase1_details.md`. Add `gopkg.in/yaml.v3` dependency to `l5/go.mod`. Verify the package compiles.

---

### 002: Implement config loader and validation
**Domain:** l5
**Status:** pending
**Depends on:** 001

Add a `Load(path string) (*ProjectConfig, error)` function to the config package. It reads the YAML file, unmarshals it, then validates: (1) required fields are present -- project, root, at least one domain, and each domain's path, base_image, and agent, (2) root is an absolute path and exists on disk, (3) domain paths resolve relative to root and exist, (4) agent core file and each rules file resolve relative to root and exist on disk, (5) timeout parsing -- use `time.ParseDuration`, default to 10 minutes if not specified at project level, domain timeout overrides project timeout. Return clear error messages indicating which field failed and why (e.g., `domain "backend": agent file not found: .claude/agents/backend-developer-core.md`).

---

### 003: Unit tests for config parsing and validation
**Domain:** l5
**Status:** pending
**Depends on:** 002

Create `l5/internal/config/config_test.go` with table-driven tests. Use `t.TempDir()` to create real filesystem structures for path validation tests. Cover: (1) valid config with all fields, (2) valid config with minimal fields (no optional timeout), (3) missing project name, (4) missing root path, (5) non-absolute root path, (6) nonexistent root path, (7) missing domain required fields (path, base_image, agent), (8) nonexistent agent file, (9) nonexistent rules file, (10) invalid timeout string, (11) domain timeout overrides project timeout, (12) default 10m timeout when none specified, (13) empty domains map.

---
