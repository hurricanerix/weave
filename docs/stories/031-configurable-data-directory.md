# Story: Configurable data directory

## Status
Done

## Problem
Weave stores session data and generated images in `~/.config/weave/` by default. During development, this means test sessions and throwaway images accumulate in the developer's home directory, mixed in with data from normal app usage. The developer has to manually clean it out or risk polluting their real config. There is no way to redirect where Weave stores its data.

## User/Actor
Any user who wants to control where Weave stores its data. Primary use case is developers running Weave from the project directory, but also useful for power users who want data on a specific drive or location.

## Desired outcome
The user can set `WEAVE_CONFIG_DIR` to control where Weave stores all persistent data (sessions, images). When unset, Weave uses the platform-appropriate default (`~/.config/weave` on Linux, `~/Library/Application Support/weave` on macOS). The `make run` development workflow automatically uses a project-local directory so dev data never touches the home directory.

## Acceptance criteria
- [x] Setting `WEAVE_CONFIG_DIR=/some/path` causes all session and image data to be stored under `/some/path/sessions/`
- [x] When `WEAVE_CONFIG_DIR` is not set, data is stored under `~/.config/weave/` on Linux (respecting `XDG_CONFIG_HOME` if set) and `~/Library/Application Support/weave/` on macOS
- [x] The `make run` target sets `WEAVE_CONFIG_DIR=./config` so development data stays in the project directory
- [x] The resolved config directory is logged at startup (debug level)
- [x] Existing sessions created under a previous path are not migrated (the user sees a fresh state at the new location)
- [x] All embedded resources (web templates, static files, agent prompts) are consolidated under `backend/internal/resources/`
- [x] Agent prompts live under `{WEAVE_CONFIG_DIR}/agents/` as user-editable files
- [x] When agent prompt files do not exist in the config dir, the embedded defaults are copied there and then used
- [x] When agent prompt files already exist in the config dir, the existing versions are used
- [x] Custom `--agent-prompt` paths that do not exist on disk produce a clear error (no silent fallback)

## Out of scope
- Automatic migration of data between directories
- CLI flag for config directory (environment variable is sufficient)
- Windows support (no Windows build target exists yet)

## Dependencies
None.

## Open questions
None.

## Notes
- The environment variable controls the base directory. Sessions live under `{WEAVE_CONFIG_DIR}/sessions/`, matching the existing directory structure.
- Agent prompts are user-editable configuration that lives under `{WEAVE_CONFIG_DIR}/agents/`. On first run (or when files are missing), the embedded defaults are written to disk so the user can find and modify them.
- If the home directory cannot be determined and no env var is set, falls back to `config` relative to the working directory.
- All embedded resources (web templates, static files, agent prompts) live under `backend/internal/resources/` for a single source of truth.

## Tasks

### 001: Add WEAVE_CONFIG_DIR resolution and wire through to persistence
**Domain:** backend
**Status:** done
**Depends on:** none

Add `ConfigDir` field to `Config` struct in `config/config.go`. Add `resolveConfigDir()` that checks `WEAVE_CONFIG_DIR` env var first, then platform defaults (`~/Library/Application Support/weave` on macOS, `$XDG_CONFIG_HOME/weave` or `~/.config/weave` on Linux), falling back to `"config"` if home dir is unavailable. Call it from `Parse()` after validation. Update `CreateSessionManager` and `CreateImageStore` in `startup/init.go` to accept a `configDir` string parameter and use `filepath.Join(configDir, "sessions")` instead of the hardcoded `"config/sessions"`. Update `InitializeAll` to pass `cfg.ConfigDir`. Fix call sites in `init_test.go` to pass `t.TempDir()`. Add a debug log line in `main.go` that logs the resolved config directory at startup. Add unit tests for `resolveConfigDir`: env var override, XDG_CONFIG_HOME fallback, default `~/.config/weave` path, and the `"config"` fallback when home dir is unavailable.

---

### 002: Update Makefile run target for development workflow
**Domain:** packaging
**Status:** done
**Depends on:** 001

Prefix the `make run` command with `WEAVE_CONFIG_DIR=./config` so that running the app from the project directory stores all session and image data under `./config/sessions/` instead of `~/.config/weave/`. Verify that after `make run`, session data appears in the project-local `config/` directory.

---

### 003: Create centralized resources package and move web assets
**Domain:** backend
**Status:** done
**Depends on:** none

Create `backend/internal/resources/` package. Move web templates from `backend/internal/web/templates/` and static files from `backend/internal/web/static/` into `backend/internal/resources/web/templates/` and `backend/internal/resources/web/static/` respectively. Copy agent prompt files from `config.old/agents/ara.md` and `config.old/agents/ara_tools.md` into `backend/internal/resources/config/agents/`. Create `resources.go` with a `go:embed` directive embedding all resources, exporting `WebFS() (fs.FS, error)` (returns `fs.Sub` rooted at `web/`) and `AgentPrompt(name string) (string, error)` (reads from `config/agents/{name}`). Update `server.go` to remove its `go:embed` directive and `embeddedFS` variable, import the resources package, call `resources.WebFS()` for template parsing and static file serving, and add a `webFS fs.FS` field to the `Server` struct. Delete the old `backend/internal/web/templates/` and `backend/internal/web/static/` directories. Add `resources_test.go` verifying `WebFS()` returns templates and static files and `AgentPrompt()` returns prompt content. Verify `go build ./...` and `go test ./...` pass.

---

### 004: Seed agent prompts into config dir with embedded fallback
**Domain:** backend
**Status:** done
**Depends on:** 001, 003

Agent prompts are user-editable config files that live under `{WEAVE_CONFIG_DIR}/agents/`. Add a `SeedAgentPrompts(configDir string, embeddedFn func(string) (string, error)) error` function (in `startup/init.go` or a new file) that: for each default prompt (`ara.md`, `ara_tools.md`), checks if `{configDir}/agents/{name}` exists; if not, creates the directory with `os.MkdirAll` and writes the embedded content via `embeddedFn(name)`. Call this from `InitializeAll` before creating the web server. Change the default values of `DefaultAgentPrompt` and `DefaultAgentToolsPrompt` in `config.go` to be just the filenames (`ara.md`, `ara_tools.md`), and resolve the full path as `filepath.Join(cfg.ConfigDir, "agents", cfg.AgentPromptPath)` in `NewServerWithDeps` (or in `InitializeAll` before passing to the web server). When `--agent-prompt` is set to a non-default value, treat it as an explicit path and do not seed or fall back. Add tests for `SeedAgentPrompts`: creates files when missing, preserves existing files, and handles the embedded function correctly. Update `server_test.go` to reflect the changed prompt path resolution.
