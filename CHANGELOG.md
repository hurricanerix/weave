# Changelog

All notable changes to Weave will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added
- Update to Mistral 7B with structured output (Story 010)
- Generation settings UI controls (Story 011)
- Agent-triggered image generation (Story 012)
- App shell layout with three-panel structure (Story 013)
- CSS component library and design system (Story 014)

### Changed
- Socket lifecycle management: weave now creates and owns the Unix socket, spawns compute as child process (Story 018)
