# 10 Release Plan

release_scope: DEVELOPMENT_ONLY

## Steps

1. Deploy backend with migrations.
2. Set `VIDEO_CREATION_ENABLED=true` and `SKILL_ROOT=skills`.
3. Register external tool endpoints through `/api/tools/register`.
4. Start backend and check `/api/skills` and `/api/workflows`.
5. Start one dry workflow run with fake/stub external tools.

## Rollback

1. Disable `VIDEO_CREATION_ENABLED`.
2. Revert code and `skills/*` package changes.
3. Remove newly upserted workflow templates if needed by deterministic IDs.
