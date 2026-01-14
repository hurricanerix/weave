# Story: Electron desktop shell

## Status
Done

## Problem
Weave currently runs as a CLI-launched web server that users access through a browser. This workflow feels technical and fragmented - users must start a terminal, run a command, then switch to a browser. For a conversational image generation tool meant to feel simple and natural, this experience creates unnecessary friction.

## User/Actor
End users who want to generate images through natural conversation without dealing with terminals or browser tabs.

## Desired Outcome
Users launch Weave like any native desktop application. A window appears showing the familiar conversational interface. The Go server runs invisibly in the background, managed automatically by the application lifecycle. When users close the window, everything shuts down cleanly.

## Acceptance Criteria
- [x] Electron application displays splash screen on startup
- [x] Go server binary spawns automatically when Electron starts
- [x] Electron polls `/ready` endpoint until server is available
- [x] Main window loads content from Go server at `/`
- [x] Splash screen closes after main window renders
- [x] Closing the window sends SIGTERM to Go server
- [x] Go server shuts down gracefully within timeout
- [x] If Electron is force-killed, Go server detects stdin EOF and self-terminates
- [x] No orphan Go processes remain after any shutdown scenario

## Out of Scope
- macOS or Windows builds (Linux only for now)
- System tray integration
- Auto-updates
- Multiple window support
- Deep linking or protocol handlers
- Flatpak packaging (separate story)
- Makefile integration (separate story)

## Dependencies
- Existing Go web server serving content at `/`

## Open Questions
None

## Notes

### Running the Electron app

```bash
cd electron
npm install
npm start
```

This will install Electron as a development dependency and launch the desktop application. The Go server will be spawned automatically by Electron.

Architecture overview:

```
┌──────────────────────────────────────────────────────────────┐
│                     Electron main process                     │
│  ┌─────────────┐    ┌─────────────┐    ┌─────────────────┐   │
│  │ Splash      │    │ Go process  │    │ Main window     │   │
│  │ window      │    │ manager     │    │ (BrowserWindow) │   │
│  └─────────────┘    └──────┬──────┘    └────────┬────────┘   │
│                            │                    │             │
└────────────────────────────┼────────────────────┼─────────────┘
                             │                    │
                      spawn  │              HTTP  │
                      stdio  │            GET /   │
                             │                    │
                             ▼                    ▼
                    ┌────────────────────────────────────────┐
                    │            Go HTTP server              │
                    │           localhost:19600              │
                    │  ┌──────────┐ ┌─────────────┐          │
                    │  │ /ready   │ │ /           │          │
                    │  │ endpoint │ │ HTML content│          │
                    │  └──────────┘ └─────────────┘          │
                    └────────────────────────────────────────┘
```

Communication channels:

| Channel | Direction | Purpose |
|---------|-----------|---------|
| HTTP | Electron to Go | Content delivery and health checks |
| stdout/stderr | Go to Electron | Debug logging |
| stdin EOF | Electron to Go | Parent death detection |

Configuration values:

| Constant | Value | Purpose |
|----------|-------|---------|
| `SERVER_URL` | `http://localhost:19600` | Go server address |
| `POLL_INTERVAL_MS` | `100` | Health check frequency |
| `STARTUP_TIMEOUT_MS` | `10000` | Maximum startup wait |
| `MIN_SPLASH_DURATION_MS` | `1500` | Minimum splash display time |
| Shutdown timeout | `5000` | Grace period before SIGKILL |

File structure:

```
electron/
├── package.json
├── main.js
├── preload.js
└── splash.html
```

## Tasks

### 001: Add /ready endpoint to Go web server
**Domain:** weave
**Status:** done
**Depends on:** none

Add a GET /ready endpoint to the web server that returns HTTP 200 with JSON {"status":"ready"} when the server is fully initialized and ready to serve requests. This endpoint is used by Electron for health polling during startup. Add the route in registerRoutes() method in internal/web/server.go, alongside existing routes. The handler should be a simple function that writes the JSON response with no additional checks.

---

### 002: Create Electron project structure and package.json
**Domain:** weave
**Status:** done
**Depends on:** none

Create electron/ directory at project root. Create package.json with project metadata, dependencies (electron as dev dependency), and scripts for running the app. Do not include electron-builder or packaging dependencies yet (out of scope). Use electron version that supports Linux. Set main entry point to main.js. Include minimal metadata: name, version, description, license matching the Go project.

---

### 003: Create splash screen HTML
**Domain:** weave
**Status:** done
**Depends on:** 002

Create electron/splash.html as a minimal HTML page shown during Go server startup. Display project name "Weave" and a loading indicator (CSS spinner or text). No external dependencies. Self-contained HTML with inline CSS. Dimensions should be small (300x200 or similar) and centered. Background color should match application theme. No images or external assets.

---

### 004: Implement Electron main process (main.js) - part 1: Go spawning and health polling
**Domain:** weave
**Status:** done
**Depends on:** 001, 002

Create electron/main.js with constants defined in story (SERVER_URL, POLL_INTERVAL_MS, STARTUP_TIMEOUT_MS, MIN_SPLASH_DURATION_MS). Implement Go binary spawning using Node.js child_process.spawn(). Binary path should be ../bin/weave (relative to electron/ directory) or detect via environment variable. Pass --port=19600 flag to Go process. Capture stdout/stderr and pipe to console. Store child process reference globally for cleanup. Implement health polling: use fetch() or axios to poll GET /ready endpoint at POLL_INTERVAL_MS intervals. Track start time and enforce STARTUP_TIMEOUT_MS. On success, resolve. On timeout, reject with error.

---

### 005: Implement Electron main process (main.js) - part 2: Window management
**Domain:** weave
**Status:** done
**Depends on:** 003, 004

In electron/main.js, implement splash screen window creation using BrowserWindow. Load splash.html. Configure window: no frame, center on screen, always on top. Implement main window creation (deferred until server ready): create BrowserWindow loading http://localhost:19600 after health check succeeds. Configure window: standard frame, 1200x800 default size, center on screen. Enforce MIN_SPLASH_DURATION_MS: track splash display start time, delay main window show if splash has been visible for less than minimum duration. Close splash after main window is ready to show. Handle window ready-to-show event to prevent white flash.

---

### 006: Implement Electron main process (main.js) - part 3: Graceful shutdown
**Domain:** weave
**Status:** done
**Depends on:** 004, 005

In electron/main.js, implement shutdown handling. Listen for app 'window-all-closed' event. When triggered, send SIGTERM to Go child process using child.kill('SIGTERM'). Wait up to 5000ms for Go process to exit (listen for 'exit' event on child process). If timeout expires, send SIGKILL using child.kill('SIGKILL'). After process cleanup completes (or timeout), call app.quit() to exit Electron. Ensure no orphan processes remain in any scenario.

---

### 007: Add stdin EOF detection to Go server
**Domain:** weave
**Status:** done
**Depends on:** none

Add orphan process prevention to cmd/weave/main.go. Spawn a goroutine that reads from os.Stdin in a loop. When stdin reaches EOF (parent Electron process died), log "Parent process died, initiating shutdown" and trigger graceful shutdown by cancelling the server context. This ensures the Go process exits if Electron is force-killed (kill -9) without sending SIGTERM. Use existing shutdown mechanisms (context cancellation) so cleanup proceeds normally (compute daemon shutdown, socket cleanup, etc.).

---

### 008: Create preload.js (empty placeholder)
**Domain:** weave
**Status:** done
**Depends on:** 002

Create electron/preload.js as an empty file or minimal placeholder with a comment explaining its purpose. Preload scripts run in renderer process context and expose APIs to the web page. Currently not needed (web content is served from Go server), but Electron best practices recommend always specifying a preload script path in BrowserWindow options for security. Reference this file in main.js BrowserWindow webPreferences.preload option.

---

### 009: Add npm script for running Electron app
**Domain:** weave
**Status:** done
**Depends on:** 002, 004, 005, 006, 008

Update electron/package.json to add a "start" script that runs electron . to launch the app. Ensure electron is listed as a devDependency. Add .gitignore entry for electron/node_modules/. Document in story file (or README once committed) how to run: cd electron && npm install && npm start. Note: This task does NOT integrate with the Makefile (separate story per out-of-scope section).

---
