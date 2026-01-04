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
