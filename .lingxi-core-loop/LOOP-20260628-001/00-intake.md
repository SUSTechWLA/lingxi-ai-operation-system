# 00 Intake

## Requirement

Upgrade the current video agent according to `.lingxi-core-loop/增加新闻能力并融入系统.md`.

The first implementation slice is the document's P0 closed loop:

- Add `KnowledgePolicy` to `AgentPlan`.
- Decide whether a video request requires fresh knowledge.
- Have `PlanGuard` enforce retrieval boundaries.
- Have `PlanCompiler` insert `news_search` and `fact_extractor` when required.
- Pass a `knowledgePack` into `video_script_generator`.
- Ensure script output can expose `usedFacts`, `factCheckWarnings`, and `knowledgeTrace`.

## Initial Classification

This is a Core update with a video-agent business adapter:

- Core: plan model, guard validation, compiler injection, traceable DAG arguments.
- Video agent: request classification and query rewriting.
- Tool layer: built-in manifests for `news_search` and `fact_extractor`.
- External capability: real search backend is a tool capability, not business logic in Core.

## Assumptions

- Implement only the P0 behavior from the requirement document in this loop.
- Existing fixed video pipelines remain intact.
- Current `knowledge_researcher` remains supported; new `news_search` and `fact_extractor` provide a stricter current-events path.
- Single-developer self review is acceptable for this development loop.
