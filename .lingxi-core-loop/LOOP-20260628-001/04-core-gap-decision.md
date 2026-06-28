# 04 Core Gap Decision

Decision: `CORE_CHANGE_CANDIDATE`

## Why Core Change Is Required

The missing behavior is not only a video script prompt issue. The system needs a platform-level way to express and enforce whether a plan may or must retrieve fresh knowledge.

This belongs in Core because:

- `AgentPlan` is the contract between Planner, Guard, Compiler, and Runner.
- `PlanGuard` is the authoritative enforcement point for tool boundaries.
- `PlanCompiler` is the correct place to insert mandatory DAG steps when the plan is incomplete but policy requires them.
- The same policy shape can later support other domains beyond video.

## What Stays Outside Core

- News provider internals, ranking, and source credibility scoring stay in tool implementations.
- Video-specific classification rules live under `internal/agents/video/knowledgepolicy`.
- Frontend display of fact trace is a later client task.

## Approval

Self-review decision: `PROVISIONALLY_APPROVED`.

Rationale: the change satisfies Core admission rules, keeps business-specific logic outside `internal/core/agentruntime`, and is testable without production external services.
