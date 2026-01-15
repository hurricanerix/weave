# Story: Flatpak packaging

## Status
Done

## Problem
Users need to install Weave through standard Linux software distribution channels. Building from source requires development tools and technical knowledge that conflicts with the goal of a simple, conversational image generation tool.

## User/Actor
Linux users who want to install Weave through Flatpak without building from source.

## Desired Outcome
Users install Weave via `flatpak install` and launch it like any other desktop application. The Flatpak bundle contains everything needed: Electron shell, Go server, and all dependencies. The application integrates with the desktop environment (application menu, icons).

## Acceptance Criteria
- [x] Flatpak manifest file defines complete build process
- [x] `make flatpak` builds the Flatpak bundle
- [x] `make flatpak-install` installs the bundle locally for testing
- [x] Application appears in desktop application menu after install
- [x] Launching from application menu starts the full application
- [x] Application has appropriate permissions (network for localhost, GPU access)
- [x] Desktop entry file includes proper metadata (name, icon, categories)
- [x] Application icon displays correctly in application menu
- [x] `flatpak run <app-id>` launches the application

## Out of Scope
- Publishing to Flathub or other repositories
- Auto-updates within Flatpak
- Flatpak runtime bundling (uses freedesktop runtime)
- Custom Flatpak permissions UI
- macOS or Windows packaging

## Dependencies
- Story 020 (Electron desktop shell)
- Story 021 (Electron build integration)

## Open Questions
None

## Notes

File structure:

```
packaging/
└── flatpak/
    ├── com.placeholder.weave.yml    # Flatpak manifest
    ├── com.placeholder.weave.desktop # Desktop entry
    └── weave.sh                      # Launcher script
```

Flatpak permissions required:

| Permission | Purpose |
|------------|---------|
| `--share=ipc` | X11/Wayland IPC |
| `--socket=x11` | X11 display |
| `--socket=wayland` | Wayland display |
| `--device=dri` | GPU access |
| `--share=network` | localhost communication |
| `--socket=pulseaudio` | Audio support |

Flatpak file layout:

```
/app/
├── bin/
│   └── weave                # Launcher script
├── weave/
│   ├── weave                # Electron executable
│   └── resources/
│       └── weave            # Go binary
└── share/
    ├── applications/
    │   └── com.placeholder.weave.desktop
    └── icons/
        └── hicolor/
            └── 256x256/
                └── apps/
                    └── com.placeholder.weave.png
```

Manifest structure (simplified):

```yaml
app-id: com.placeholder.weave
runtime: org.freedesktop.Platform
runtime-version: '24.08'
sdk: org.freedesktop.Sdk
command: weave

finish-args:
  - --share=ipc
  - --socket=x11
  - --socket=wayland
  - --device=dri
  - --share=network
  - --socket=pulseaudio

modules:
  - name: weave
    buildsystem: simple
    build-commands:
      - cp -r electron/dist/linux-unpacked /app/weave
      - install -Dm755 packaging/flatpak/weave.sh /app/bin/weave
      - install -Dm644 packaging/flatpak/com.placeholder.weave.desktop /app/share/applications/
```

Build commands:

```bash
# Prerequisites
flatpak install flathub org.freedesktop.Platform//24.08 org.freedesktop.Sdk//24.08

# Build and install
make flatpak
make flatpak-install

# Run
flatpak run com.placeholder.weave
```

## Tasks

### 001: Create packaging directory structure
**Domain:** weave
**Status:** done
**Depends on:** none

Create the packaging/flatpak/ directory structure for Flatpak build files. Create packaging/ directory at project root. Create packaging/flatpak/ subdirectory. This establishes the location for all Flatpak packaging files (manifest, desktop entry, launcher script, icon). Verify the directories are created and match the file structure shown in story notes.

---

### 002: Create placeholder icon
**Domain:** weave
**Status:** done
**Depends on:** 001

Create a 256x256 PNG icon at packaging/flatpak/com.placeholder.weave.png. Generate a simple placeholder: solid color square (blue #5865F2 or purple #7B68EE) with white "W" letter centered. Use Python with PIL (Pillow) or a shell script with ImageMagick if available. The icon just needs to be valid PNG format that displays in the application menu. Verify the file is exactly 256x256 pixels and renders correctly.

---

### 003: Create launcher script
**Domain:** weave
**Status:** done
**Depends on:** 001

Create packaging/flatpak/weave.sh bash script that launches the Electron application. The script should be: #!/bin/bash followed by exec /app/weave/weave "$@" to execute the Electron binary with all arguments passed through. Set executable permissions (chmod +x). This script becomes /app/bin/weave in the Flatpak and serves as the entry point specified by the manifest's "command" field.

---

### 004: Create desktop entry file
**Domain:** weave
**Status:** done
**Depends on:** 001

Create packaging/flatpak/com.placeholder.weave.desktop file following freedesktop.org desktop entry specification. Set Type=Application, Name=Weave, Comment=Image Generation Tool. Set Icon=com.placeholder.weave (without .png extension, this references the installed icon). Set Exec=weave (the launcher script installed to /app/bin/weave). Set Terminal=false. Set Categories=Graphics;Utility; per the story's focus on image generation. Verify the format matches the desktop entry specification.

---

### 005: Create Flatpak manifest
**Domain:** weave
**Status:** done
**Depends on:** 001

Create packaging/flatpak/com.placeholder.weave.yml Flatpak manifest file. Set app-id to com.placeholder.weave. Use runtime org.freedesktop.Platform version 24.08. Use sdk org.freedesktop.Sdk. Set command to weave (the launcher script). Add finish-args for permissions: --share=ipc, --socket=x11, --socket=wayland, --device=dri, --share=network, --socket=pulseaudio (as listed in story notes). Add a single module named "weave" with buildsystem simple. The build-commands should: 1) Copy electron/dist/linux-unpacked to /app/weave, 2) Install weave.sh to /app/bin/weave with mode 755, 3) Install desktop file to /app/share/applications/, 4) Install icon to /app/share/icons/hicolor/256x256/apps/. Sources should reference the local directory (type: dir, path: ../..). Verify the YAML structure is valid and matches the simplified example in story notes.

---

### 006: Add Makefile targets for Flatpak
**Domain:** weave
**Status:** done
**Depends on:** 002, 003, 004, 005

Add two new .PHONY targets to Makefile: "flatpak" and "flatpak-install". The flatpak target should depend on "electron" (from Story 021) to ensure the Electron package is built first. It runs "flatpak-builder --force-clean build-dir packaging/flatpak/com.placeholder.weave.yml" to build the Flatpak bundle. The flatpak-install target depends on "flatpak" and runs "flatpak-builder --user --install --force-clean build-dir packaging/flatpak/com.placeholder.weave.yml" to install locally for testing. Add build-dir to the clean target to remove Flatpak build artifacts. Verify the targets work and produce a runnable Flatpak.

---

### 007: Update .gitignore for packaging directory
**Domain:** weave
**Status:** done
**Depends on:** 001

Update .gitignore to allow the new packaging/ directory and its files while ignoring Flatpak build artifacts. Add allowlist entries: !packaging/, !packaging/**/*.yml, !packaging/**/*.desktop, !packaging/**/*.sh, !packaging/**/*.png. These patterns allow the packaging directory structure and all Flatpak configuration files. Verify build-dir/ (Flatpak builder output) remains ignored by default. Verify git status shows the new packaging files as untracked (ready to add).

---

### 008: Verify Flatpak build and install
**Domain:** weave
**Status:** done
**Depends on:** 006, 007

Test the complete Flatpak workflow from clean state. Prerequisites: Verify flatpak-builder is installed. Verify org.freedesktop.Platform//24.08 and org.freedesktop.Sdk//24.08 runtimes are installed (document if missing). Run "make clean" to remove artifacts. Run "make flatpak" to build the Flatpak bundle. Verify build-dir/ contains the built application. Run "make flatpak-install" to install locally. Run "flatpak run com.placeholder.weave" to launch. Verify the Electron window opens with the Go server running. Check application menu to verify "Weave" appears with the placeholder icon. Verify all acceptance criteria: manifest builds, make targets work, app launches, icon displays, desktop integration works.

---
