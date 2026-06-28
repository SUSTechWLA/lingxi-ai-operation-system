# 02 Requirement Spec

## Functional Requirements

- REQ-001 `AgentPlan` must support an optional `knowledgePolicy` field.
- REQ-002 The video agent must classify high-freshness topics such as "最新", "今天", "出线", "世界杯", "2026", "现任", "政策", and similar terms as requiring retrieval.
- REQ-003 `PlanGuard` must reject high-freshness plans that do not use `retrievalPolicy=required`.
- REQ-004 `PlanGuard` must reject required retrieval plans that omit `news_search`, omit `fact_extractor`, omit search queries, or allow empty facts.
- REQ-005 `PlanGuard` must reject `news_search` when retrieval is `none` or `forbidden`.
- REQ-006 `PlanCompiler` must insert `knowledge_search` and `fact_extract` when retrieval is required or optional and the plan lacks an explicit search step.
- REQ-007 `PlanCompiler` must wire `fact_extract` into `video_script_generator` via `knowledgePack`, `knowledgeSources`, `retrievalPolicy`, and `mustUseFreshKnowledge`.
- REQ-008 `video_script_generator` must fail when required retrieval is requested but no facts are provided.
- REQ-009 `video_script_generator` must expose `usedFacts`, `unusedFacts`, `factCheckWarnings`, and `knowledgeTrace` when present in the structured output.

## Non-Goals

- No customer-specific frontend implementation in this loop.
- No production-grade search ranking or source credibility engine in this loop.
- No removal of existing `knowledge_researcher` or fixed video pipeline behavior.
