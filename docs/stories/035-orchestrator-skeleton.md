# Story: Orchestrator skeleton and agent API

## Status
In Progress

## Problem
l5-relay and the container wrapper need HTTP endpoints to POST journal entries and completion results to. Without the orchestrator service, there is nowhere to receive or store the data that containers produce. The orchestrator also needs persistent storage for jobs, journal entries, and patches.

## User/Actor
- Container (calls agent API endpoints)
- l5-relay (POSTs journal entries)
- Wrapper script (POSTs completion and patch)

## Desired Outcome
A Go web server with SQLite storage and two agent-facing API endpoints. The orchestrator can receive and store journal entries and completion results from running containers.

## Acceptance Criteria
- [ ] Go web server starts and listens on a configurable port
- [ ] SQLite database is created on startup with schema for jobs, journal entries, and patches
- [ ] Job record supports fields: spec text, patch input (optional), status, outcome, patch output
- [ ] Job statuses: pending, running, complete
- [ ] Job outcomes: success, error, timeout, empty, killed
- [ ] `POST /api/agent/journal` accepts a journal entry with run_id and stores it
- [ ] `POST /api/agent/complete` accepts patch file (multipart) and outcome string, updates job status to complete
- [ ] Journal entries are stored in order with timestamps
- [ ] Invalid run_id returns appropriate HTTP error
- [ ] Orchestrator reads project config on startup (from story 034)
- [ ] Unit tests cover: journal ingestion, completion with patch, completion without patch, completion with each outcome type, invalid run_id

## Out of Scope
- Human-facing UI (that is story 039)
- SSE streaming (that is story 040)
- Container lifecycle management (that is story 038)
- Workspace assembly (that is story 037)

## Dependencies
- Story 034 (project config format) -- orchestrator reads config on startup

## Open Questions
None.

## Notes
See `l5/phase1_details.md` "Agent API" section. The orchestrator is a standard Go web server with SQLite. WAL mode and foreign keys enabled on the database. The agent API is intentionally minimal -- two endpoints, simple data in, stored to disk.

## Tasks

### 001: SQLite schema and data store
**Domain:** l5
**Status:** pending
**Depends on:** none

Create `l5/internal/store/` package. Add a SQLite driver dependency to `l5/go.mod` (prefer pure Go driver `modernc.org/sqlite` to avoid CGO). Create the initial migration (`l5/internal/store/migrations/001_initial.sql`) with two tables: `jobs` (id TEXT PRIMARY KEY, domain TEXT, spec TEXT, patch_input TEXT, status TEXT DEFAULT 'pending', outcome TEXT, patch_output TEXT, created_at DATETIME, updated_at DATETIME) and `journal_entries` (id INTEGER PRIMARY KEY AUTOINCREMENT, job_id TEXT REFERENCES jobs(id), body TEXT, created_at DATETIME). Implement `store.New(dbPath string) (*Store, error)` that opens the database, enables WAL mode and foreign keys, and applies migrations. Implement store methods: `CreateJob`, `GetJob`, `ListJobs`, `UpdateJobStatus` (pending→running), `UpdateJobCompletion` (sets status=complete, outcome, patch_output), `AddJournalEntry`, `GetJournalEntries(jobID)`. Status and outcome values should be Go constants, not raw strings.

---

### 002: Agent API HTTP handlers
**Domain:** l5
**Status:** pending
**Depends on:** 001

Create `l5/internal/api/` package. Implement `POST /api/agent/journal` handler: accepts JSON body with `run_id` (string) and `body` (raw JSON), looks up job by run_id, returns 404 if not found, returns 409 if job is not in running status, stores the journal entry via the store, returns 201. Implement `POST /api/agent/complete` handler: accepts multipart form with `outcome` (string field) and `patch` (file part, optional), validates outcome is one of the five valid values, looks up job by run_id (from form field), returns 404 if not found, calls `UpdateJobCompletion` on the store, returns 200. Both handlers should log errors to stderr and return JSON error responses with descriptive messages.

---

### 003: Server skeleton and main entrypoint
**Domain:** l5
**Status:** pending
**Depends on:** 001, 002

Create `l5/internal/server/` package with a `Server` struct that holds the store, project config, router, and HTTP server. Register the agent API routes from task 002. Server listens on a configurable port (flag or env var). Implement graceful shutdown on SIGINT/SIGTERM. Create `l5/cmd/l5-orchestrator/main.go` that reads the project config file path from a CLI flag, loads it using the config package from story 034, initializes the SQLite store (database file in `./tmp/l5.db`), creates and starts the server. Add `l5-orchestrator` build target to the top-level `Makefile`. Verify the server starts, accepts requests on the agent API routes, and shuts down cleanly.

---

### 004: Unit tests for store and handlers
**Domain:** l5
**Status:** pending
**Depends on:** 001, 002

Create `l5/internal/store/store_test.go` with table-driven tests: create job, get job, list jobs, update status, add journal entry, get journal entries in order, complete with each outcome type. Use in-memory SQLite (`:memory:`) for test isolation. Create `l5/internal/api/api_test.go` with handler tests using `httptest`: journal ingestion with valid run_id, journal with invalid run_id (404), journal on non-running job (409), completion with patch file and outcome, completion without patch, completion with each outcome type, completion with invalid outcome (400), completion with invalid run_id (404). Each test creates a fresh in-memory store, seeds it with a job in the appropriate status, and verifies the HTTP response and store state.

---
