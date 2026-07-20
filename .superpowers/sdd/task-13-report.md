# Task 13 report

## Status

Implemented the creator preview and delivery surface, default creator routing checks, and a durable assembly-retry path that preserves the legacy `/assemble` validation endpoint.

## TDD evidence

- RED: `cd frontend && npm run test:creator` failed before implementation with `ENOENT ... PreviewDeliveryPanel.tsx` after the new contract assertions were added.
- GREEN: `cd frontend && npm run test:creator` passes after the panel, real creator API call, explicit delivery gate, and contract assertions were implemented.
- Backend integration coverage: `cd cloud-backend && go test ./internal/agents/video/service -run TestCreatorStudio -count=1` passes. It exercises 100 Shots, target-only Shot 12 regeneration, duplicate durable regeneration, candidate acceptance, assembly snapshot, strict 15.0 rejection, and byte-equivalence of every non-target Shot.

## Implementation

- Added `POST /api/video-projects/:id/assembly/rebuild` with an Idempotency-Key. It persists an accepted/current/non-stale assembly snapshot, then uses the existing preview review-gate `Regenerate` path instead of any Shot regeneration API. The old `/assemble` endpoint remains validation-only.
- Added CAS-safe assembly receipts. A fingerprint mismatch returns conflict; only a successful preview-gate dispatch marks the receipt queued and clears `AssemblyDirty`. A concurrent Shot acceptance causes the final CAS to fail rather than clearing new work.
- Creation view consumes queued receipts: while the current preview is still the receipt's base artifact, Preview and Delivery become generating and the durable preview task is exposed, preventing old video media from reappearing after reconnect.
- Preview and Delivery read only current creation-view artifacts through the real artifact-content API. Delivery is fail-closed: it needs a media URL and an explicit passed check in current artifact content or artifact metadata.
- Reassembly uses a per-attempt UUID key retained for ambiguous-network retry; blocked/conflict results clear it for a new user attempt.
- Extended smoke checks with creator contract and developer-console build checks. README and changelog document creator navigation and verified behavior.

## Verification

- `cloud-backend: go test ./... && go vet ./...` — PASS
- `cloud-backend: make gen-ts && make api-types-check` — PASS
- `local-backend: go test ./...` — PASS
- `frontend: npm run test:director && npm run test:creator && npm run test:developer-build && npm run lint && npm run build` — PASS
- `git diff --check` — PASS

## Limits / follow-up

- No live visual capture was performed: launching the authenticated cloud/local/frontend stack requires external service configuration not present in this worktree. CSS retains the established warm-yellow/deep-brown/card tokens; focus styling is present, status has text, and the creator logic check asserts only one selected Shot player.
- The receipt has a queued recovery representation and removes old media once a new preview artifact becomes current. It now reads the source-node status so failed/cancelled/unknown work becomes retryable instead of spinning forever.

## Follow-up fix

- Added the read-only review-source `RegenerationStatus` query and used it in creation-view recovery. Active work remains generating with a durable task; failed/cancelled/unknown work is fail-safe failed with `assemblyDirty` in the view, so old media remains hidden and the creator can retry.
- Ran `bash scripts/beta-smoke-check.sh`: code checks passed; smoke is blocked solely because `hyperframes-render-service/node_modules` is absent. Ran `bash scripts/beta-readiness-check.sh`: BLOCKED because the smoke prerequisite failed; local agent, HyperFrames health, FFmpeg and diagnostics were detected, but no real provider was configured.
