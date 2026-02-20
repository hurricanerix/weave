# Level5

Container-isolated development pipeline. Spec in, patch out. Human evaluates outcomes, not diffs.

## Motivation

["The 5 Levels of AI Coding (Why Most of You Won't Make It Past Level 2)"](https://www.youtube.com/watch?v=bDcgHzCBgmQ) video by Nate Jones discussing Dan Shapiro's "5 Levels of AI Coding" framework describes a progression from AI-as-autocomplete (Level 0) through fully autonomous development (Level 5). Most teams operate at Level 2-3: AI handles multi-file changes, but a human still reviews every line. The bottleneck shifts from implementation speed to spec quality and evaluation strategy.

l5 targets enabling **Level 5** patterns in a safe way: the human writes a spec, an agent implements it autonomously in an isolated container, and the human evaluates the result.


## Goals

* Projects can be created using Level 5, where specs go in a black box and working software (outcomes) comes out.
* Agents work in isolation with copies of code such that they can not alter the original source.
* Strong controls layerd outside of the agents control to ensure that agents only have access to what they need and no access to anything they don't.

## Future Features

- Code review and QA review containers (receive patch, produce verdict)
- Retry logic with journal summarization
- Feature branch creation for approved patches
- Approval flow in UI
- Multi-domain container support
- Cross-domain negotiation before parallel implementation
- Network allowlisting per container
- Token usage tracking
- Concurrent pipeline runs
- MCP capability broker (agents request services, orchestrator holds credentials)
- Pipeline analytics (spec effectiveness, cost per run, spec improvement suggestions)

## Relationship to Weave

l5 is project-agnostic infrastructure. Weave is the first consumer. The orchestrator does not know anything about image generation, Go, or C -- it knows about domains, base images, agent configs, and patches. Weave is being used as a test for this process, if successful, Level5 will move to a dedicated project and Weave will be updated to use it as a dependency.
