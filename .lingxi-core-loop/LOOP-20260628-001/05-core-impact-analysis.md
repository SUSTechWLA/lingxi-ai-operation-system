# 05 Core Impact Analysis

## Changed Areas

- `internal/core/agentruntime`
  - Adds `KnowledgePolicy` to `AgentPlan`.
  - Adds backend fallback policy inference for video current-event topics.
  - Extends LLM planner schema and prompt.
  - Completes policy-driven knowledge steps before Guard validation and DAG compilation.
  - Validates retrieval policy boundaries in `PlanGuard`.
- `internal/agents/video/knowledgepolicy`
  - Adds a video-facing decider wrapper for topic classification.
- `internal/core/worker/tool/builtin`
  - Registers `news_search` and `fact_extractor`.
  - Adds knowledge-aware `video_script_generator` parameters and output fields.
  - Blocks required retrieval when facts are empty.

## Compatibility

- Existing plans without `knowledgePolicy` remain valid.
- Existing non-news video plans do not automatically add search tools.
- Existing `knowledge_researcher` and `fact_checker` prompt tools are unchanged.
- Planner fallback only emits required retrieval when the tool catalog contains both `news_search` and `fact_extractor`.

## Risks

- `news_search` depends on `SEARCH_API_KEY` / `SEARCH_ENDPOINT`; required retrieval will now fail fast if search is unavailable.
- First version uses simple rule-based high-freshness keywords.
- `fact_extractor` is a minimal black-box fact pack normalizer, not a full source ranking engine.
