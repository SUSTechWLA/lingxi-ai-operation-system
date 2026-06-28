# 11 Operations Plan

Monitor:

- Agent runs with `knowledgePolicy.retrievalPolicy=required`.
- `news_search` failure rate.
- `fact_extractor` empty fact failures.
- `video_script_generator` failures with required retrieval.
- Script outputs with `usedFacts` and `knowledgeTrace`.

Runbook:

- If required retrieval fails because `SEARCH_API_KEY` is missing, configure search provider or change the task to manual fact input.
- If non-news content calls `news_search`, inspect `knowledgePolicy` and Guard validation logs.
- If scripts ignore facts, inspect `usedFacts` and `knowledgeTrace` in node output.
