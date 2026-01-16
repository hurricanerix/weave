# Story: Refactor Go backend to backend/ directory

## Status
Done

## Problem

Go backend files currently live at the project root (go.mod, go.sum, cmd/, internal/), while the C compute component has its own compute/ directory. This organizational inconsistency makes the project structure harder to navigate for developers. When opening the project, backend-specific files are mixed with top-level project files (README.md, Makefile, LICENSE, etc.), creating visual clutter and making it less obvious where each component lives.

## User/Actor

Developers working on the Weave codebase (contributors, maintainers, new developers onboarding).

## Desired Outcome

Clear, consistent component organization where both major components have their own directories:
- Backend component: backend/ (contains go.mod, cmd/, internal/)
- Compute component: compute/ (already exists)
- Project root: Only top-level project files (README, Makefile, docs/, .claude/, packaging/)

Developers can immediately identify where backend code lives. New contributors onboarding to the project see a clean, logical structure. Navigation between components becomes more intuitive.

## Acceptance Criteria

- [ ] backend/ directory created at project root
- [ ] go.mod and go.sum moved to backend/
- [ ] cmd/ directory moved to backend/cmd/
- [ ] internal/ directory moved to backend/internal/
- [ ] Makefile updated to reference backend/ paths for Go builds
- [ ] Makefile backend target builds binary to build/weave-backend (output path unchanged)
- [ ] Go module path remains unchanged in go.mod (no import path changes needed)
- [ ] Electron configuration updated to reference new backend binary path
- [ ] test/ directory structure updated if it contains Go tests that import internal packages
- [ ] All documentation updated to reflect new directory structure
- [ ] .gitignore updated to reference backend/ directory
- [ ] All builds succeed: make all completes successfully
- [ ] All tests pass: Go tests, compute tests, integration tests
- [ ] Application runs: Electron launches, spawns backend, generates images
- [ ] Grepping codebase for old paths (cmd/, internal/ at root) finds zero references except in CHANGELOG

## Out of Scope

- Historical CHANGELOG entries remain unmodified
- Git history and commit messages are not rewritten
- Changing compute/ directory structure or location
- Refactoring Go code structure within backend/
- Changing the build output paths or binary names

## Dependencies

None.

## Open Questions

None.

## Notes

This is a pure organizational refactoring - no code changes, only moving files into a subdirectory and updating paths.

Directory structure before:

```
weave/
├── cmd/
├── internal/
├── go.mod
├── go.sum
├── compute/
├── electron/
├── docs/
├── Makefile
└── README.md
```

Directory structure after:

```
weave/
├── backend/
│   ├── cmd/
│   ├── internal/
│   ├── go.mod
│   └── go.sum
├── compute/
├── electron/
├── docs/
├── Makefile
└── README.md
```

Build process changes:
- Go commands in Makefile change from `go build ./cmd/weave` to `cd backend && go build ./cmd/weave`
- Output path remains `build/weave-backend`
- No changes to binary behavior or functionality

Module path considerations:
- go.mod module path can remain as-is (e.g., `module weave`)
- No import path changes needed since internal/ moves with go.mod
- go.sum moves with go.mod to maintain dependency integrity

## Tasks

### 001: Create backend directory and move Go files
**Domain:** weave
**Status:** done
**Depends on:** none

Create the backend/ directory at project root. Move go.mod, go.sum, cmd/, and internal/ into backend/. The Go module path stays the same, so no import changes are needed. Verify the directory structure matches the story's "after" diagram.

---

### 002: Update Makefile for backend directory
**Domain:** weave
**Status:** done
**Depends on:** 001

Update the backend target in Makefile to build from backend/ directory. Change `go build -o build/weave-backend ./cmd/weave` to `cd backend && go build -o ../build/weave-backend ./cmd/weave`. Output path remains build/weave-backend (same as before). Verify `make backend` builds successfully and produces the binary at the expected location.

---

### 003: Update Electron configuration for new backend path
**Domain:** weave
**Status:** done
**Depends on:** 001

Update electron/package.json extraResources to reference backend binary at ../build/weave-backend (path unchanged, but document it clearly). No changes needed to electron/main.js since binary path is already relative to build/. Verify Electron build (`make electron`) succeeds and packages the backend binary correctly.

---

### 004: Update integration tests for new Go module location
**Domain:** weave
**Status:** done
**Note:** Moved test/integration/ to backend/test/integration/ and updated path calculations
**Depends on:** 001

Update test/integration/*.go files to import from the backend module. Since the module path stays the same (github.com/hurricanerix/weave), imports remain unchanged. Update any relative path calculations (like stubGeneratorPath) if they reference the project root structure. Verify integration tests pass: `go test -tags=integration ./test/integration/`.

---

### 005: Update .gitignore for backend directory
**Domain:** weave
**Status:** done
**Depends on:** 001

Update root .gitignore to allow backend/ directory by adding `!/backend` to the allowlist section. Add `!backend/**/*.go`, `!backend/go.mod`, `!backend/go.sum` to allow backend Go files. The allowlist structure means we explicitly permit the backend/ directory and its contents. Verify `git status` shows backend files are tracked.

---

### 006: Update documentation for new directory structure
**Domain:** weave
**Status:** done
**Depends on:** 001

Update docs/DEVELOPMENT.md to reflect the new backend/ directory in the project structure diagram. Update any command examples that reference ./cmd/weave to backend/cmd/weave or explain the new location. Update .claude/rules/go.md project layout example to show backend/ instead of cmd/ and internal/ at root. Update .claude/agents/weave-developer.md if it references specific paths. Ensure all documentation consistently shows the new structure.

---

### 007: Verify all builds and tests pass
**Domain:** weave
**Status:** done
**Depends on:** 002, 003, 004

Run the full build and test suite to verify nothing is broken. Execute: `make clean && make all` to build all components. Run `go test ./...` from the backend/ directory for Go unit tests. Run `go test -tags=integration ./test/integration/` for integration tests. Run `make compute && cd compute && make test` for C tests. Verify the application launches with `make run` and can generate images. All must pass without errors.

---

### 008: Grep codebase for stale path references
**Domain:** weave
**Status:** done
**Depends on:** 006

Search the codebase for any remaining references to old paths. Run `grep -r "\\./cmd/weave" --exclude-dir=.git --exclude-dir=build` to find old command paths. Run `grep -r "^import.*internal/" --include="*.go" --exclude-dir=backend` to find imports outside backend. Run `grep -r "/internal/" --include="*.md" docs/` to find documentation references. Verify all findings are either in CHANGELOG (allowed) or already updated. Fix any missed references.
