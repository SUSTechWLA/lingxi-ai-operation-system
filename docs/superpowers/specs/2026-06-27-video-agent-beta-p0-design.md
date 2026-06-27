# Video Agent Beta P0 Design

## Goal

Bring the repository into a consistent first beta shape for single-user video creation: no active bid or generic chat product surface, a video-first architecture document, generated API docs that match the current routes, and a verification path that proves the desktop/cloud/local build still works.

## Scope

This pass covers P0 cleanup and beta readiness only.

Included:

- Remove stale Bid and Chat groups from the authoritative cloud OpenAPI spec.
- Regenerate generated API markdown and frontend TypeScript types from the OpenAPI source.
- Rewrite `docs/ARCHITECTURE.md` around Video Agent v4.0: video creation, current publish compatibility, local runner, HyperFrames, artifact review, and operation/distribution roadmap.
- Keep the existing `publish` backend module operational for beta use, documenting it as the current compatibility publishing layer that later migrates to `distribution`.
- Verify cloud backend tests, local backend tests, frontend build, and API docs drift checks.

Excluded:

- No `publish` to `distribution` package rename in this pass.
- No new `operation` module implementation in this pass.
- No endpoint migration from `/api/video-projects` to `/api/video/projects` in this pass.
- No deletion of historical upgrade notes under `docs/upgrade/` or prior Superpowers specs/plans.

## Design

The safest beta cut is to align source-of-truth documents and generated contracts with the code that already runs. The live product path is `frontend` Director Studio plus `cloud-backend` Dynamic Agent Runtime, video projects, artifact review, local runner, HyperFrames service, and the current publish compatibility APIs. The core remains generic internally, but current product documentation must not present bid or generic chat as active business lines.

OpenAPI is the hard contract. Tests should fail if the cloud spec advertises `Bid`, `Chat`, `/api/bid/*`, or `/api/chat/*`. Once that red test is in place, the implementation removes the stale tags from `cloud_spec.go` and regenerates `cloud-backend/docs/API_REFERENCE.md` plus `frontend/src/utils/api-types.generated.ts`.

Architecture documentation should state a beta-operable full flow:

```text
Idea / brief
→ Dynamic Agent run or video project workflow
→ Script / storyboard / prompt artifacts
→ Artifact review and stale tracking
→ Local runner / HyperFrames render path
→ Publish compatibility package
→ Manual data import and operation roadmap
```

## Acceptance

- `go test ./internal/core/apispec -count=1` fails before the OpenAPI cleanup test is implemented, then passes after cleanup.
- `cd cloud-backend && make gen-docs` updates generated docs/types.
- `cd cloud-backend && make api-docs-check` passes.
- `cd cloud-backend && go test ./...` passes.
- `cd local-backend && go test ./...` passes.
- `cd frontend && npm run build` passes.
- `rg "/api/bid|/api/chat|Tag\\(\"Bid\"|Tag\\(\"Chat\"" cloud-backend/internal/core/apispec cloud-backend/docs/API_REFERENCE.md frontend/src/utils/api-types.generated.ts` has no matches.

