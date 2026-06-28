# 03 Acceptance and NFR

## Acceptance Criteria

- AC-001 A Cape Verde World Cup qualification request produces `retrievalPolicy=required`.
- AC-002 A required retrieval plan without `news_search` fails Guard validation.
- AC-003 A `retrievalPolicy=none` plan containing `news_search` fails Guard validation.
- AC-004 A required retrieval script plan is compiled with `knowledge_search -> fact_extract -> video_script_generator`.
- AC-005 The script step receives `knowledgePack`, `knowledgeSources`, `retrievalPolicy`, and `mustUseFreshKnowledge` arguments.
- AC-006 `video_script_generator` returns failure before calling the LLM when `retrievalPolicy=required` and the provided facts are empty.
- AC-007 Built-in tool registration includes `news_search` and `fact_extractor` manifests.

## NFR

- NFR-001 Local backend remains free of DB, Docker, Kafka, Redis, MinIO, and LLM API key dependencies.
- NFR-002 Existing video workflows and prompt tools remain backward compatible.
- NFR-003 Validation errors must be explicit enough to debug missing retrieval.
- NFR-004 The implementation must be covered by focused Go tests.
