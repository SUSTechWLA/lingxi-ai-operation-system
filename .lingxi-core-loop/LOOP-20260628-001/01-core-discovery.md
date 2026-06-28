# 01 Core Discovery

## Relevant Modules

- `cloud-backend/internal/core/agentruntime/plan.go`
  - `AgentPlan` currently has `Goal`, `Domain`, `Mode`, `Steps`, `Budget`, and `StopPolicy`.
  - No first-class knowledge or retrieval policy exists.
- `cloud-backend/internal/core/agentruntime/plan_guard.go`
  - Validates tools, dependency order, required parameters, parameter types, references, stage rules, risk, and cost.
  - Suitable hard boundary for retrieval policy enforcement.
- `cloud-backend/internal/core/agentruntime/plan_compiler.go`
  - Converts `AgentStep` to DAG nodes and already injects quality gates.
  - Suitable place to complete required knowledge steps before DAG compilation.
- `cloud-backend/internal/core/agentruntime/planner.go`
  - Heuristic planner fallback selects tools and wires required inputs.
  - Needs rule-based `KnowledgePolicy` fallback.
- `cloud-backend/internal/core/agentruntime/llm_planner.go`
  - LLM planner schema/prompt currently lacks `KnowledgePolicy`.
  - Prompt currently suggests `knowledge_researcher` for broad factual topics, but does not express retrieval constraints.
- `cloud-backend/internal/core/worker/tool/builtin/video_creation_external_tools.go`
  - Registers video creation tool manifests.
  - Dynamic prompt tools include `knowledge_researcher`, `fact_checker`, and `video_script_generator`.
  - `knowledge_researcher` can inject web search results internally, but it is not planned or enforced as a retrieval policy.

## Current Limitations

- High-freshness topics can reach `video_script_generator` without a search step.
- Non-news opinion topics are not explicitly protected from news search pollution.
- The DAG has no standard `knowledge_pack` artifact/argument contract.
- `video_script_generator` output schema does not expose `usedFacts`, `factCheckWarnings`, or `knowledgeTrace`.

## Quality Gates

Existing Go tests cover planner, guard, compiler, workflow, and built-in video tools. This loop will add focused tests before implementation.
