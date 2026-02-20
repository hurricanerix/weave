# Lights-out software factory: implementation phases

Each phase proves a hypothesis before the next phase adds complexity.

## Phase 1 -- Prove the core loop

Can you submit a spec and get a patch back from one containerized agent?

- Orchestrator skeleton: Go web service, SQLite, minimal HTMX UI
- API endpoints: submit spec, receive journal entries, receive patch, `/blocked`
- One container image (backend -- most active domain, proven agent)
- Workspace assembly: copy `backend/` code, `git init`, mount agent config + rules + build files
- Claude Code runs headless in container, posts journal + patch to orchestrator
- UI shows: pipeline status, journal stream, patch when done
- Human manually applies patch and evaluates

**Proves:** Isolation works. Spec in, patch out. Agent can operate autonomously in a container. Journal provides observability.

## Phase 2 -- Review pipeline and approval flow

- Code review container (receives copy + patch, produces verdict)
- QA container (full repo + all patches, produces verdict)
- Retry logic: QA rejects, compacter summarizes, new implementation container with summary context
- Feature branch creation for approved patches (non-AI script)
- +1/0/-1 approval flow in UI
- Dependency validation script (lock file diff gate)
- `/blocked` with optional clarifier agent before human escalation
- Timeout handling: orchestrator kills container if no artifact arrives

**Proves:** The full pipeline works end-to-end for single-domain stories. Human evaluates outcomes, not diffs.

## Phase 3 -- Multi-domain and hardening

- Additional container images (compute, electron)
- Cross-domain negotiation phase before parallel implementation
- Network allowlisting (Podman network config)
- Token tracking via `--output-format json` capture
- Concurrent pipeline runs (multiple specs in flight)
- Orchestrator DB backups

**Proves:** The system handles real-world complexity -- multi-domain stories, parallel agents, resource management.

## Phase 4 -- Rich experience and brainstorm integration

- `ask_dev` MCP tool bridging local Claude Code to orchestrator
- Rich feedback UI: annotated screenshots, video, logs
- Feedback interpreter agent (multimodal, ephemeral)
- Archivist agent for historical questions
- Pipeline analytics: spec effectiveness, agent behavior patterns, cost per run
- Orchestrator project config: project-agnostic, Weave as first consumer

**Proves:** The system is pleasant to use and provides insights that improve spec writing over time.

## Phase 5 -- Capability broker (MCP services)

- Orchestrator exposes MCP tools to containerized agents for authenticated external services
- Agent requests a capability (e.g., "read from GCS", "send email", "query database") via MCP tool call
- Orchestrator holds the credentials and makes the authenticated call on the agent's behalf
- Agent receives the result without ever seeing secrets (no API keys, tokens, or passwords in env vars or context)
- Per-domain and per-job capability policies control which services an agent can access
- Full audit trail since every capability request routes through the orchestrator

**Proves:** Agents can interact with external services safely. Secrets never enter the container. The orchestrator is the trust boundary.
