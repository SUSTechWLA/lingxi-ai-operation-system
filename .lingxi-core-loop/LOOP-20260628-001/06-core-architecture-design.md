# 06 Core Architecture Design

## Design

`KnowledgePolicy` is part of `AgentPlan` because it controls planner, guard, compiler, and runner behavior.

The flow is:

```text
Planner or fallback policy inference
→ PlanCompiler.PreparePlan inserts required knowledge steps
→ PlanGuard validates retrieval boundaries
→ PlanCompiler.Compile builds the DAG
→ news_search / fact_extractor produce a fact pack
→ video_script_generator consumes knowledgePack and exposes trace fields
```

## Boundary

- Core owns the policy model, validation, and DAG completion.
- Video agent owns content classification through `internal/agents/video/knowledgepolicy`.
- Tool implementations own external search and fact extraction behavior.

## Rejected Alternatives

- Always inserting `news_search` into the fixed video pipeline was rejected because it would pollute opinion and creative tasks.
- Putting news facts directly into `video_script_generator` was rejected because it would bypass planning and traceability.
