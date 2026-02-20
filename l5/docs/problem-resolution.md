# Solutions: Container-isolated development pipeline

## Problem

All agents share the same environment, context, and file access. This means:

- No isolation between design, implementation, and verification phases
- Developer agents can see the specs they will be evaluated against (no holdout set)
- Long conversations blow out context reading files (92% consumed by one transcript)
- Agents have full host filesystem access with no blast radius protection
- Agents block on human answers during implementation

## Solution: Ephemeral Podman containers + orchestrator service

Use short-lived containers instead of long-running pods. An orchestrator service manages the pipeline -- it accepts specs, triggers containers, receives journal entries, routes questions, and displays everything through an HTMX interface.

**Orchestrator:** A Go web service that lives alongside the existing components (e.g. `pipeline/` next to `backend/`, `compute/`, `electron/`). Project-agnostic from day one -- takes a config file describing domains, container images, mount paths, and agent configs. Weave is the first project to use it. Own agent derived from backend-developer with Weave specifics stripped out. Developed with the current local workflow until mature enough to manage itself.

**Three phases:**

1. **Brainstorm** -- runs locally, human-interactive. When it needs technical input, the orchestrator spins up an ephemeral dev container with a read-only code copy. The container answers the question and terminates. Brainstorm gets a short summary, not the full codebase exploration.

2. **Implementation** -- runs in a container with a disposable copy of domain code and the task spec, but no access to the verification spec. Cannot damage the host -- it only has a copy. Produces a git patch as its deliverable.

3. **QA** -- runs in a container with a copy of the code (with patches applied) and the verification spec the developer never saw. Produces a pass/fail verdict.

**Container lifecycle:** Every container follows the same pattern -- start, do bounded work, produce an artifact (patch, journal entry, verdict, or stuck report), terminate. The orchestrator manages sequencing, routing, and retry logic.

**Why ephemeral over long-running pods:** No turn management, no persistent state, no message bus infrastructure, no context blowout, zero cost when idle. Trade-off is container startup latency per invocation.

## Resolved questions

### 1. Brainstorm pod producing worse specs without technical context

Brainstorm does not run in a container. It runs locally as a normal human-interactive session. When it needs technical feasibility input, an MCP tool (`ask_dev`) triggers an ephemeral dev container with read-only code access. The container answers the question, returns a summary, and terminates. Brainstorm gets concise technical input without blowing out its own context reading the full codebase.

### 2. Codebase damage from containers

Copy-and-patch model instead of mounts. The tooling copies the domain's code into the container workspace, runs `git init` + initial commit to give the agent a fully working git repo. The agent has unrestricted access -- it can delete everything, commit, reset, whatever. It only has a copy, so nothing on the host is affected.

When done, the tooling extracts `git diff HEAD` as a patch file and submits it to the orchestrator. Review and QA containers receive a fresh copy with the patch applied. Patches can be reviewed, split, or rejected before applying to the real codebase. A release engineer step reviews the final patches and commits.

No submodules needed. The tooling handles `git init` at copy time. The agent loses access to historical git log, but historical context belongs in the spec or journal, not raw commit history. If history access proves necessary, an archivist agent (ephemeral, full repo copy, read-only pattern) can answer historical questions on demand.

### 3. Who writes the verification spec

The human writes it. This is a skill that improves with practice. If it proves too difficult or time-consuming, revisit with tooling or AI assistance later. For MVP, keep it simple.

### 4. Iteration cost when QA fails (stateless pods lose context)

Agents post reasoning and progress to the orchestrator API as they work. This serves three purposes:

- **Recovery.** If QA rejects the work or the pod crashes, the next pod pulls the journal and resumes with context of what was tried and why.
- **Observability.** The human can monitor agent reasoning in real time and spot patterns where agents get stuck or make poor decisions.
- **Feedback loop.** Journal data reveals which specs produce clean runs and which cause confusion, informing better spec writing over time.

The journal is a task-scoped log that persists in the orchestrator, associated with the spec that triggered the pipeline run.

### 5. GPU passthrough in rootless Podman

Punt GPU access for MVP. Split QA into two tiers:

- **Container QA:** Reviews code, evaluates unit test quality, verifies implementation matches the spec. No GPU or running app required. Keeps container images simple. If the app supports a replay/script mode, QA can also author functional test scripts for the human to run.
- **Human QA:** Runs the built app outside the container, executes any QA-authored test scripts, confirms end-to-end behavior.

GPU passthrough can be revisited if automated integration testing becomes a priority.

### 6. Container image size and maintenance

One image per domain, not one monolithic image:

- **backend:** Go, golangci-lint
- **compute:** GCC/Clang, CUDA headers, Valgrind, clang-format
- **electron:** Node.js, npm

Each image is small and only changes when its domain's toolchain changes. Each container receives a copy of only its own domain code. The tooling assembles the workspace: domain code + agent config (e.g. `.claude/agents/backend.md` mounted as `.claude/CLAUDE.md`) + relevant rules files (e.g. `go.md`, `protocol.md`) + task spec + build files (`go.mod`, `go.sum`). This enforces component boundaries at the filesystem level rather than by convention, and protects each agent's context -- the backend agent never wastes context reading compute or electron code. If an agent needs cross-domain knowledge, it posts a question through the orchestrator, the same pattern as brainstorm's `ask_dev`.

### 7. API cost of separate context windows

Likely comparable to the current workflow, which already spawns multiple subagents (developer, code-reviewer, qa-reviewer, security-reviewer, retries). The container model reorganizes where costs land rather than adding fundamentally new ones. Domain isolation also means each agent reads fewer files, so per-agent context is smaller.

Track token usage by capturing `--output-format json` from the `podman run` command externally. More reliable than having agents self-report via the journal -- the agent can't influence what the host captures from stdout.

### 8. Pipeline latency

This is the cost of lights-out execution. Course correction happens at the end when evaluating outcomes, not mid-stream. The feedback loop shifts from "fix it while it runs" to "write better specs next time." Latency per run is higher, but human attention per run is near zero.

### 9. Debugging across container boundaries

QA is the cross-domain debugger. It has visibility into the user specs and features, and can ask targeted questions to each domain's dev agent to isolate problems -- same ephemeral question pattern, driven by QA instead of brainstorm. Humans can also feed stack traces and logs to QA to kick off triage. If this becomes too broad a role for QA, split it into a dedicated bug triage agent.

### 10. API keys and Claude Code in containers

Standard container patterns. Claude Code is installed in the Dockerfile. Each container gets a domain-specific `.claude/` directory via compose mounts. API keys live in `.env` files (not committed) and are passed to containers through compose environment configuration.

### 11-12. Over-engineering for current scale / solving a problem you don't have yet

This is a problem that exists now, not in the future. Agents currently run with broad host access and the human approves actions on trust. The container pipeline makes it safe to grant full autonomy -- no permission prompts, no rubber-stamping, no wondering what the agent just did to the filesystem. The goal is not scaling up, it is making the autonomy you already want to grant actually safe to grant.

### 13. QA without source code may be too strict

QA does black-box verification against the spec, but gets read-only code access so it can evaluate test quality and coverage. It can see everything but change nothing.

### 14. Container startup latency

Not a concern. Container image builds are cached, startup is seconds. The pipeline goal is "provide a spec and walk away" -- a few seconds per container is irrelevant when the human is not watching. Even `ask_dev` calls during brainstorm are just a brief wait for a feasibility answer.

### 15. Agents blocking on human answers

Agents that hit ambiguity they cannot resolve call `/blocked` on the orchestrator API with the question and their current context, then terminate. The orchestrator marks the pipeline as "needs human input" and notifies the human. Optionally, a clarifier agent gets one attempt to resolve the question from existing context before escalating. When the human responds (minutes or days later), the orchestrator triggers a new container with the original spec + clarification + journal from the first attempt.

The agent should not wait for a response. Terminate-and-restart keeps one lifecycle pattern for everything: container starts, does bounded work, exits with an artifact.

### 16. Cross-domain story coordination

Cross-domain stories (e.g. a protocol change touching both backend and compute) add a negotiation phase before implementation. The orchestrator detects multi-domain specs and triggers ephemeral question containers for each affected domain: "given this spec, what interface do you need?" The orchestrator mediates proposals and counter-proposals between domains until both agree or a round limit (e.g. 3) is reached. If they do not converge, `/blocked` to the human with both positions.

The agreed interface becomes part of each domain's implementation spec. Implementation then proceeds in parallel against the shared contract. QA verifies the integrated result. The negotiation artifact is stored in the journal for review.

Specs that touch multiple domains should define interface contracts explicitly. The orchestrator could enforce this -- reject multi-domain specs without an interface section.

### 17. Story specs and orchestrator state

Specs are submitted to the orchestrator through its HTMX interface and live in its database, associated with pipeline runs, agent contexts, and outcomes. They do not need to live in the git repo. Commit messages, CHANGELOG entries, and architecture docs serve repo cloners who need to understand why code is the way it is. The orchestrator UI serves humans reviewing spec effectiveness and agent behavior.

Agent context and journal data are operational -- useful for tuning, not for understanding the codebase. Back up the orchestrator DB for durability. Export on demand for archival. If a specific interaction reveals something architecturally important, write that up as a proper doc and commit that, not the raw dump.

### 18. Journal recovery fidelity

Each container run produces a compacted summary as an artifact (alongside its patch or verdict). A compacter agent (ephemeral, haiku-tier model) takes the full session transcript and compresses it to a fixed-size summary. Summaries are append-only -- the orchestrator feeds all prior summaries to the next container as context. Context growth is linear with retry count, not with total token usage. With a 3-retry cap, worst case is 3 summaries. Claude's API is stateless, so re-sending history is unavoidable; this bounds the cost.

### 19. Compute domain viability without GPU

Containerization provides isolation value independent of GPU access. The compute agent writes C code, compiles with GCC/Clang (CUDA headers are for compilation, not execution), runs unit tests with faked GPU interfaces, runs Valgrind on non-GPU code paths, and runs clang-format. No GPU passthrough needed for any of this. The compute container image from resolved question #6 is correct as-is.

### 20. Journal security

The worst case of any agent misbehavior -- misleading journal entries, bad code, wasted tokens -- is a bad patch. Bad patches don't get committed. The container prevents host damage, the review pipeline prevents bad code from landing. The security boundary is "nothing inside the container affects the real codebase until a human approves it." The journal is context for retries, not a trusted artifact.

### 21. Container network access

Containers need network access for Anthropic's API and package registries. Unrestricted network access risks supply chain attacks -- a compromised dependency could attempt container escape. Mitigations: rootless Podman limits escape impact, lock files (`go.sum`, `package-lock.json`) surface unexpected dependencies in the patch diff, and agents should not add dependencies unless the spec requires it. Additionally, allowlist network access via Podman network config: containers can reach `api.anthropic.com`, `proxy.golang.org`, `registry.npmjs.org`, and the orchestrator. Nothing else. This is a few lines of network config, not a proxy service. Compromised packages on legitimate registries remain a risk, but that applies to all software development, containerized or not. As an additional gate, the orchestrator runs a non-AI script on each patch that diffs lock files (`go.sum`, `package-lock.json`) against the baseline. If dependencies changed and the spec does not include an explicit "new dependencies" section, the patch is rejected automatically before it reaches review. No tokens spent, agent cannot game it.

### 22. `ask_dev` cold-start latency

Each `ask_dev` call spins up a fresh container. Container startup is fast (cached images), but the Claude Code session inside needs to read domain code to build context before answering -- this takes minutes per question. Accepted as the cost of this workflow. The alternative (brainstorm reads the code directly) blows out its context, which is the problem being solved. The trade-off is latency per question vs. context blowout in the main session.

### 23. Escape hatch

The `/blocked` pattern covers this. If the container is missing a tool or hitting an unexpected problem, the agent calls `/blocked`, describes what it needs, and terminates. The human fixes the container image or provides guidance, and the orchestrator re-triggers. If a pattern emerges (agents frequently need X), update the image. The escape hatch is not "fall back to local" -- it is "stop, report what is wrong, human fixes the environment, retry."

### 24. Safe commit process

No AI agent has write access to the real repository. The release agent (containerized) writes only the commit message and CHANGELOG entry, which are posted back to the orchestrator. The orchestrator applies the patch to a feature branch via a non-AI script. The riskiest step is a deterministic script, not an agent.

### 25. QA cross-domain visibility

QA is a special case -- it needs cross-domain visibility to evaluate the integrated result. The QA container receives a full repo copy with all domain patches applied. It reviews code and evaluates test quality but does not compile or run the application. Still a disposable copy, still zero risk to the real codebase.

### 26. Human approval flow

The orchestrator applies the final patches to a feature branch in the real repo. The human reviews through the orchestrator UI, which shows the spec, outcomes, and changes. The human can build, run, and test the feature from the branch. Three verdicts:

- **+1 (merge):** Orchestrator merges to main. Pipeline complete.
- **0 (needs changes):** Human provides feedback as a new spec describing issues. Orchestrator kicks off a new pipeline run with existing patches and journal context.
- **-1 (reject):** Orchestrator discards the branch. Human revises the original spec and resubmits.

### 27. Rich human feedback for needs-changes verdicts

The `0` (needs changes) verdict supports more than text. The human can provide annotated screenshots, video clips, logs, and stack traces through the orchestrator UI. A feedback interpreter agent (ephemeral, multimodal) processes these into structured text summaries for the implementation agents. The raw media never reaches the dev agent's context -- the interpreter extracts actionable descriptions (e.g. "sidebar overlaps main content at 768px width" or "cancel button unresponsive after second click"). This keeps human feedback natural (point at what is wrong) while protecting agent context budgets.

## Open questions

### 1. MVP scope boundary

What constitutes the first usable version? Container images alone are useless without the orchestrator. The orchestrator needs at minimum: spec upload, container triggering, journal endpoint, patch collection, and a basic UI. Scope needs definition before implementation planning.
