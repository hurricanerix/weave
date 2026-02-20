# Story: Agent config split

## Status
In Progress

## Problem
Agent definition files (`.claude/agents/*.md`) contain both frontmatter (name, model, allowedTools) and the agent's instructions (role, process, standards, boundaries). l5 containers need only the instructions, mounted as `.claude/CLAUDE.md`. The frontmatter is specific to Claude Code's subagent system and does not apply in headless container mode. Currently there is no way to use the instructions without the frontmatter.

## User/Actor
- l5 orchestrator (mounts core files into containers)
- Developers (use subagent files in the current local workflow)

## Desired Outcome
Each agent definition is split into a core file (instructions only) and a wrapper file (frontmatter + reference to core). The current local subagent workflow continues to work unchanged. l5 can mount the core file directly.

## Acceptance Criteria
- [ ] Each agent has a core file (e.g., `backend-developer-core.md`) containing only the instruction body, no frontmatter
- [ ] Each agent wrapper file (e.g., `backend-developer.md`) contains frontmatter and a reference directing the agent to read its core file
- [ ] The current local subagent workflow continues to function (spawned agent reads the core file as its first action)
- [ ] All existing agents are split: backend-developer, compute-developer, electron-developer, release-engineer, code-reviewer, qa-reviewer, security-reviewer, l5-developer
- [ ] No agent instructions are duplicated between core and wrapper files
- [ ] CLAUDE.md references to agent files remain accurate

## Out of Scope
- Changes to agent instructions or behavior -- this is a structural split only
- Creating new agents
- l5 container mounting logic (that is story 037)

## Dependencies
None. This is a restructuring task independent of l5 code.

## Open Questions
None.

## Notes
See `l5/phase1_details.md` "Agent config split" section for design rationale. The core file is the single source of truth for agent behavior. The wrapper file exists only to provide frontmatter for the local subagent system.

## Tasks

### 001: Split backend-developer as pilot
**Domain:** l5
**Status:** pending
**Depends on:** none

Create `backend-developer-core.md` containing everything after the frontmatter closing `---` from `backend-developer.md`. Replace the body of `backend-developer.md` with a single instruction: `Read and follow all instructions in .claude/agents/backend-developer-core.md.` Preserve the frontmatter (name, description, model, allowedTools) exactly as-is. Verify the core file has no frontmatter and the wrapper file has no instruction content beyond the reference line.

---

### 002: Split remaining 7 agents
**Domain:** l5
**Status:** pending
**Depends on:** 001

Apply the same split to: compute-developer, electron-developer, release-engineer, code-reviewer, qa-reviewer, security-reviewer, l5-developer. For each: create `*-core.md` with the instruction body, replace the wrapper body with the reference line. Each wrapper retains its original frontmatter. Each core file contains no frontmatter. Verify all 16 files exist (8 wrappers + 8 cores) and no instruction content is duplicated between wrapper and core.

---

### 003: Verify CLAUDE.md and agent cross-references
**Domain:** l5
**Status:** pending
**Depends on:** 002

Check that CLAUDE.md agent table references still point to the correct wrapper files. Check that any agent instructions referencing other agents (e.g., "that's compute-developer's job") still work -- these references are in the core files now, which is correct. Verify no broken file references exist in any of the 16 agent files. Update CLAUDE.md if any references need adjustment (e.g., if the "Agent config split" section in phase1_details.md examples need to match actual filenames).

---
