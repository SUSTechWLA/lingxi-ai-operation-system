# 12 Delivery Report

## Result

`CORE_CHANGE`

The P0 news capability integration is implemented.

## Behavior Changes

- Agent plans can now carry `knowledgePolicy`.
- Current-event video topics can be classified as required retrieval.
- Required retrieval plans are completed with `news_search` and `fact_extractor`.
- Guard rejects invalid retrieval plans.
- Script generation receives a fact pack and exposes fact usage trace fields.
- Required retrieval with empty facts fails before LLM generation.

## Files Changed

- `cloud-backend/internal/core/agentruntime/*`
- `cloud-backend/internal/agents/video/knowledgepolicy/*`
- `cloud-backend/internal/core/worker/tool/builtin/video_creation_external_tools.go`
- `cloud-backend/internal/core/worker/tool/builtin/video_creation_external_tools_test.go`

## Verification

All planned verification commands passed. See `evidence/verification.md`.

## Known Risks

- The search backend must be configured for live required retrieval.
- Classification is rule-based in this version.
- Frontend fact trace display is not included in this implementation slice.
