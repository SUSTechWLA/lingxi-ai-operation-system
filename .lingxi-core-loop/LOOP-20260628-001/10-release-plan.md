# 10 Release Plan

release_scope: DEVELOPMENT_ONLY

## Deployment

No database migration is required.

Deploy the updated cloud backend. Existing local backend and frontend builds remain compatible.

## Configuration

For real news search execution, configure:

- `SEARCH_API_KEY`
- Optional `SEARCH_ENDPOINT`
- Optional `SEARCH_GL`
- Optional `SEARCH_HL`

## Rollback

Revert the cloud backend code changes for:

- `KnowledgePolicy`
- Guard validation
- Compiler knowledge injection
- Built-in knowledge tools

No persisted data migration needs rollback.
