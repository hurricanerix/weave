# Story 023: Rename components for clarity

Status: Draft

## Problem

Current naming is inconsistent and confusing:
- `make weave` builds the Go backend, but "weave" is the product name
- `compute-daemon/` directory still uses "daemon" terminology, but the component is no longer a standalone daemon
- Log messages and code comments still reference "daemon"

## Desired outcome

Clear, consistent naming that reflects the current architecture:
- Makefile target `weave` renamed to `backend`
- Directory `compute-daemon/` renamed to `compute/`
- Binary `weave-compute` renamed to just `compute` (or kept as-is if preferred)
- All references updated throughout codebase and documentation

## Scope

### Directory and build changes
- Rename `compute-daemon/` directory to `compute/`
- Update Makefile target from `weave` to `backend`
- Update all import paths, includes, and references in code

### Code changes
- Update log messages in `cmd/weave/main.go` that say "daemon"
- Update any code comments referencing "daemon"

### Documentation changes
- Update remaining `compute-daemon/` paths in `docs/DEVELOPMENT.md`
- Update remaining "daemon" terminology in technical sections
- Update `compute-daemon/README.md` path

### Files with known references
- `docs/DEVELOPMENT.md` - many `cd compute-daemon` commands
- `cmd/weave/main.go` - log messages saying "Spawned weave-compute daemon"
- `Makefile` - target names
- `.gitignore` files

## Notes

This is a draft story for story-writer to flesh out with acceptance criteria and detailed scope.
