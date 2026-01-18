# Changelog

All notable changes to Weave will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added
- Flatpak packaging for Linux desktop distribution (Story 022)
- Electron build integration with Makefile targets (Story 021)
- Electron desktop shell with native app experience (Story 020)
- Update to Mistral 7B with structured output (Story 010)
- Generation settings UI controls (Story 011)
- Agent-triggered image generation (Story 012)
- App shell layout with three-panel structure (Story 013)
- CSS component library and design system (Story 014)

### Changed
- Agent reliability overhaul: function calling replaces delimiter parsing, upgraded to Llama 3.1 8B (128k context), externalized agent prompt to config file, added thinking indicator (Story 025)
- Refactor Go backend to backend/ directory for cleaner project organization (Story 024)
- Component naming standardization: backend binary renamed to weave-backend, compute-daemon directory renamed to compute, obsolete daemon terminology removed (Story 023)
- Socket lifecycle management: weave now creates and owns the Unix socket, spawns compute as child process (Story 018)
- History sidebar defaults to closed on page load for better focus and screen space (Story 019)
