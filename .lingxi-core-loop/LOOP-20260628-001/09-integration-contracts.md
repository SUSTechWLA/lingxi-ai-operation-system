# 09 Integration Contracts

## news_search

Input:

- `queries`: array, required.
- `freshnessDays`: number, optional.
- `topK`: number, optional.

Output:

- `results`: array of `{title, url, source, publishedAt, snippet, language, credibility}`.
- `queryUsed`: array.
- `searchedAt`: string.

Failure:

- Missing query.
- Search provider unavailable.
- Empty search results.

## fact_extractor

Input:

- `searchResults`: array, required.
- `outputMode`: optional, defaults to fact pack behavior.

Output:

- `facts`: array of traceable claims.
- `sources`: array.
- `warnings`: array.
- `freshness`: object.
- `knowledgePackHash`: string.

## video_script_generator

Additional input:

- `knowledgePack`
- `knowledgeSources`
- `retrievalPolicy`
- `mustUseFreshKnowledge`
- `currentDate`

Additional output:

- `usedFacts`
- `unusedFacts`
- `factCheckWarnings`
- `knowledgeTrace`
