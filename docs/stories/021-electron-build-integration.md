# Story: Electron build integration

## Status
Done

## Problem
The Electron desktop shell (Story 020) introduces new build artifacts that need to integrate with the existing Makefile-based build system. Developers need a single command to build and run the full application, and the default build target should produce the desktop application rather than just the CLI tools.

## User/Actor
Developers building and testing the Weave application locally.

## Desired Outcome
Running `make` produces a complete desktop application with the Go server bundled inside the Electron package. Running `make run` builds and launches the application. Individual components remain buildable separately for development flexibility.

## Acceptance Criteria
- [x] `make` (default target) builds the complete Electron application with bundled Go binary
- [x] `make run` builds and launches the Electron application
- [x] `make weave` builds only the Go server binary
- [x] `make electron` builds only the Electron package (assumes Go binary exists)
- [x] `make clean` removes all build artifacts including Electron dist folder
- [x] Go binary is copied into Electron's resources directory during build
- [x] Build works from clean checkout (no pre-existing artifacts required)
- [x] Build fails with clear error if npm dependencies are missing

## Out of Scope
- CI/CD pipeline configuration
- Release versioning or tagging
- Code signing
- Flatpak build targets (separate story)
- Cross-compilation for other platforms

## Dependencies
- Story 020 (Electron desktop shell)

## Open Questions
None

## Notes

electron-builder configuration in `electron/package.json`:

```json
{
  "build": {
    "appId": "com.placeholder.weave",
    "productName": "Weave",
    "linux": {
      "target": "dir"
    },
    "extraResources": [
      {
        "from": "../build/weave",
        "to": "weave"
      }
    ]
  }
}
```

Expected Makefile targets:

```makefile
.PHONY: all run weave electron clean

all: electron

run: electron
	./electron/dist/linux-unpacked/weave

weave:
	go build -o build/weave ./cmd/weave

electron: weave
	cd electron && npm run build

clean:
	rm -rf build/
	rm -rf electron/dist/
```

Path resolution in Electron main.js:

```javascript
const GO_BINARY_PATH = app.isPackaged
  ? path.join(process.resourcesPath, 'weave')
  : path.join(__dirname, '..', 'build', 'weave');
```

## Tasks

### 001: Add electron-builder configuration to electron/package.json
**Domain:** weave
**Status:** done
**Depends on:** none

Add the "build" section to electron/package.json with electron-builder configuration. Set appId to "com.placeholder.weave", productName to "Weave", linux target to "dir" (unpacked directory for development). Configure extraResources to copy Go binary from ../build/weave to resources/weave. Add electron-builder as a devDependency (version ^25.0.0 or later). Add "build" npm script that runs "electron-builder --linux dir" to build the unpacked application. Verify the config matches the structure shown in story notes.

---

### 002: Update Makefile to use build/ directory instead of bin/
**Domain:** weave
**Status:** done
**Depends on:** none

Change the weave target in Makefile to output to build/weave instead of bin/weave. Update the path: "go build -o build/weave ./cmd/weave". Keep the compute target unchanged (it builds in compute-daemon/). This establishes the new build directory structure that electron-builder expects (per extraResources config in task 001). Do not change clean target yet (task 005 handles that).

---

### 003: Add electron Makefile target that depends on weave
**Domain:** weave
**Status:** done
**Depends on:** 001, 002

Add a new .PHONY target called "electron" to the Makefile. This target depends on "weave" (ensuring Go binary is built first). The target runs "cd electron && npm run build" to invoke electron-builder. If npm dependencies are not installed, this will fail with npm's error (acceptable per acceptance criteria). This target builds the complete Electron package with the bundled Go binary.

---

### 004: Add run Makefile target that builds and launches the app
**Domain:** weave
**Status:** done
**Depends on:** 003

Add a new .PHONY target called "run" to the Makefile. This target depends on "electron" (ensuring the app is built first). The target runs "./electron/dist/linux-unpacked/weave" to launch the packaged application. This is the unpacked Electron app produced by electron-builder with the --linux dir target. Verify the path matches electron-builder's default output location for unpacked Linux builds.

---

### 005: Update clean Makefile target to remove build artifacts
**Domain:** weave
**Status:** done
**Depends on:** 002, 003

Update the clean target in Makefile to remove both build/ and electron/dist/ directories. Replace "rm -f bin/weave" with "rm -rf build/" to remove the new build directory. Add "rm -rf electron/dist/" to remove electron-builder output. Keep the compute-daemon clean call unchanged. After this, "make clean" removes all build artifacts including Electron packages.

---

### 006: Change default Makefile target from build to electron
**Domain:** weave
**Status:** done
**Depends on:** 003, 005

Change the "default" target in Makefile from depending on "clean build" to depending on "electron". This makes "make" (with no arguments) build the complete Electron application instead of just the Go and C binaries. Remove the standalone "build" target if it only builds weave and compute, as "electron" now serves that purpose (it depends on weave, and compute is built separately if needed). Per acceptance criteria, the default target should produce the desktop application.

---

### 007: Update electron/main.js binary path for packaged app
**Domain:** weave
**Status:** done
**Depends on:** 001

Update the GO_BINARY_PATH constant in electron/main.js to use path resolution that works in both development and production. When app.isPackaged is true (production), resolve to path.join(process.resourcesPath, 'weave'). When false (development), resolve to path.join(__dirname, '..', 'build', 'weave'). This matches the extraResources configuration from task 001 and ensures the app finds the Go binary in both scenarios. Verify the path logic matches the story notes example.

---

### 008: Update .gitignore to handle new build directories
**Domain:** weave
**Status:** done
**Depends on:** none

The .gitignore uses an allowlist approach (ignores everything, then allows specific patterns). Verify that build/ directory is already ignored (it should be, since it's not explicitly allowed). Verify that electron/dist/ is ignored. Verify that electron/node_modules/ is ignored. If electron/ directory itself is currently blocked, add allowlist entries for electron package files: !electron/package.json, !electron/package-lock.json, !electron/*.js, !electron/*.html. Do not allow node_modules/ or dist/ subdirectories.

---

### 009: Verify clean build from fresh checkout works
**Domain:** weave
**Status:** done
**Depends on:** 001, 002, 003, 004, 005, 006, 007, 008

Test the complete build process from a clean state. Run "make clean" to remove all artifacts. Verify build/ and electron/dist/ are gone. Run "cd electron && npm install" to install dependencies. Run "make" (default target) from project root. Verify it builds build/weave, then builds electron/dist/linux-unpacked/. Run "make run" to launch the app. Verify the Electron window opens and Go server responds. Close the app and verify clean shutdown. This validates all acceptance criteria for the build integration.

---
