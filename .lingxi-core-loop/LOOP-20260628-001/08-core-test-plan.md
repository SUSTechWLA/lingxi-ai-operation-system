# 08 Core Test Plan

## Unit Coverage

- Knowledge policy:
  - Current-event sports topic requires retrieval.
  - Opinion topic forbids news search.
- PlanGuard:
  - Required retrieval without search/fact tools is rejected.
  - `retrievalPolicy=none` with `news_search` is rejected.
- PlanCompiler:
  - Required retrieval inserts `knowledge_search` and `fact_extract`.
  - Script step receives `knowledgePack`, `knowledgeSources`, `retrievalPolicy`, and `mustUseFreshKnowledge`.
- Built-in tools:
  - `news_search` and `fact_extractor` manifests are registered.
  - Script generator blocks required retrieval with empty facts.

## Regression

- `cloud-backend go test ./...`
- `cloud-backend go vet ./...`
- `cloud-backend go test -race ./...`
- `local-backend go test ./...`
- `frontend npm run build`
