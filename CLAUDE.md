# Weave - High-Performance Image Generation System

## What This Is

Production-grade image generation daemon with GPU acceleration.

- **weave** (Go): CLI + web server for orchestration
- **weave-compute** (C): GPU daemon with auth
- **Protocol**: Binary over Unix sockets

## Architecture

Available in `docs/ARCHITECTURE.md`

## Development Workflow

### Story to Tasks

1. **story-writer** - Define the feature (conversational, business-focused)
2. **task-planner** - Break into technical tasks (codebase-aware)

### Implementation

3. **weave-developer** or **compute-developer** - Implement each task
4. **code-reviewer** - Review per-task for code quality

### Story Completion

5. **qa-reviewer** - Verify acceptance criteria + user experience (per-story)
6. **security-reviewer** - Assess security + risk (per-story)
7. **Human approval** before committing

## Specialized Agents

Available in `.claude/agents/`:

| Agent | Role | When Used |
|-------|------|-----------|
| **story-writer** | Define features, acceptance criteria | New feature planning |
| **task-planner** | Break stories into tasks | After story is ready |
| **weave-developer** | Implement weave (Go) code | Per task |
| **compute-developer** | Implement compute (C) code | Per task |
| **code-reviewer** | Review code quality | Per task |
| **qa-reviewer** | Verify acceptance criteria, UX | Per story (complete) |
| **security-reviewer** | Assess security, risk | Per story (complete) |

## Stories and Tasks

Stories live in `docs/stories/NNN-title.md`. Each story contains:
- Problem, user, desired outcome
- Acceptance criteria
- Tasks (added by task-planner)

Tasks are numbered per-story (001, 002, 003...) and assigned to a domain (weave/compute).

## Language-Specific Rules

Detailed conventions in `.claude/rules/`:
- **go.md** - Go standards and testing
- **c.md** - C standards and performance
- **protocol.md** - Binary protocol specification
- **documentation.md** - Documentation standards

## Agent Philosophy

Agents are **contrarian experts**:
- Conversational, one question at a time
- Push back on bad ideas
- Direct feedback, not too agreeable
- Explain risks clearly
- "Disagree and commit" when user accepts risk

## Performance Targets

RTX 4070 SUPER (12GB VRAM):
- SDXL-Lightning: 1-2s @ 512x512
- FLUX.1-dev: 10-15s @ 1024x1024

## Critical Principles

- **Go**: Idiomatic, table-driven tests, boring code
- **C**: C99, no hidden costs, Valgrind clean
- **Security**: Auth on socket, input validation, no UB
- **Testing**: Fast unit tests, slow integration tests (tagged), detailed benchmarks

## Implementing Stories

When the user says "Implement Story NNN" (e.g., "Implement Story 015"):

### Workflow

1. **Read the story** from `docs/stories/NNN-*.md`
2. **For each task in order:**
   a. Check task status - skip if already `done`
   b. Spawn the appropriate developer agent based on `Domain:`
      - `weave` → weave-developer
      - `compute` → compute-developer
   c. Developer implements the task
   d. Spawn code-reviewer to review the changes
   e. **If CHANGES REQUESTED:**
      - Developer fixes the issues
      - Re-run code-reviewer
      - Repeat up to 3 times total
      - If still failing after 3 attempts: **STOP** (see below)
   f. **If APPROVED:** Update task status to `done`, continue to next task

3. **After all tasks are done:**
   a. Spawn qa-reviewer to verify acceptance criteria
   b. Spawn security-reviewer to assess security (run in parallel with qa)
   c. **If either requests changes:**
      - Developer fixes the issues
      - Re-run code-reviewer on fixes
      - Re-run both qa-reviewer and security-reviewer
      - Repeat up to 3 times total for the story-level review cycle
      - If still failing: **STOP** (see below)

4. **After both reviewers approve:**
   a. Update CHANGELOG.md with the story (add to `[Unreleased]` section)
   b. Update story status to `Done`
   c. Print: "Story NNN complete."

### On Failure (3 failed attempts)

Do NOT create a file or update the story. Print to terminal:

```
BLOCKED: Story NNN - Task XXX (or "QA/Security Review")

Phase: [code-review | qa-review | security-review]
Issue: [Brief description of the recurring problem]
Attempts: 3

Review needed before continuing.
```

### CHANGELOG Updates

Add entries to the `[Unreleased]` section in CHANGELOG.md:
- Use the story title as the entry
- Categorize as Added, Changed, Fixed, or Removed based on the story's nature
- Format: `- Story title (Story NNN)`
