# Weave - High-Performance Image Generation System

## What This Is

Production-grade desktop application for conversational image generation with GPU acceleration.

- **Electron**: Desktop shell providing native app experience
- **weave-backend** (Go): Orchestration layer (web server, LLM integration, spawns compute)
- **weave-compute** (C): GPU compute component (spawned by weave-backend, not a standalone process)
- **Protocol**: Binary over Unix sockets between weave-backend and weave-compute

## Architecture

The application runs as an Electron app that launches the Go backend, which in turn spawns the C compute process. Communication between Go and C uses a binary protocol over Unix sockets.

```
Electron (UI) → weave-backend (Go) → weave-compute (C)
                       ↓                     ↓
                  ollama (LLM)          GPU inference
```

## Development Workflow

### Design to Tasks

1. `/architect` - Technical design discussion (how does it fit the system?)
2. `/write-story` - Define the feature (conversational, business-focused)
3. `/plan-tasks` - Break into technical tasks (codebase-aware)

### Implementation

4. **backend-developer** or **compute-developer** - Implement each task
5. **code-reviewer** - Review per-task for code quality

### Story Completion

6. **qa-reviewer** - Verify acceptance criteria + user experience (per-story)
7. **security-reviewer** - Assess security + risk (per-story)
8. **Human approval** before committing

## Commands

Available as slash commands:

| Command | Role | When Used |
|---------|------|-----------|
| `/architect` | Technical design discussion | Before defining a feature |
| `/write-story` | Define features, acceptance criteria | After design is clear |
| `/plan-tasks` | Break stories into tasks | After story is ready |

## Specialized Agents

Available in `.claude/agents/`:

| Agent | Role | When Used |
|-------|------|-----------|
| **backend-developer** | Implement backend (Go) code | Per task |
| **compute-developer** | Implement compute (C) code | Per task |
| **electron-developer** | Implement Electron (JS) code | Per task |
| **release-engineer** | Packaging and distribution | Per task |
| **code-reviewer** | Review code quality | Per task |
| **qa-reviewer** | Verify acceptance criteria, UX | Per story (complete) |
| **security-reviewer** | Assess security, risk | Per story (complete) |

## Stories and Tasks

Stories live in `docs/stories/NNN-title.md`. Each story contains:
- Problem, user, desired outcome
- Acceptance criteria
- Tasks (added by `/plan-tasks`)

Tasks are numbered per-story (001, 002, 003...) and assigned to a domain (backend/compute/electron/packaging).

## Language-Specific Rules

Detailed conventions in `.claude/rules/`:
- **go.md** - Go standards and testing
- **c.md** - C standards and performance
- **electron.md** - Electron security and patterns
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
- **Temp files**: Use `./tmp/` (project-local), never `/tmp/`. Create the directory if it does not exist.
- **Component boundaries**: Each component (`backend/`, `compute/`, `electron/`, `packaging/`) owns its own files. Don't modify files outside your assigned component without asking. Never consolidate or merge configuration files (especially `.gitignore`) across components.

## Implementing Stories

When the user says "Implement Story NNN" (e.g., "Implement Story 015"):

### Workflow

1. **Read the story** from `docs/stories/NNN-*.md`
2. **For each task in order:**
   a. Check task status - skip if already `done`
   b. Spawn the appropriate developer agent based on `Domain:`
      - `backend` → backend-developer
      - `compute` → compute-developer
      - `electron` → electron-developer
      - `packaging` → release-engineer
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
