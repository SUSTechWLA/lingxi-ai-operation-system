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

- Authenticated start and project-list surfaces were captured at all required breakpoints. The already-running backend did not expose this branch's new creation-view contract, so opening an existing project returned the user-safe retry alert and the live Shot workspace itself could not be inspected against that older process.
- The receipt has durable dispatching and queued recovery representations. Active work is reconnected without redispatch; an explicit terminal failure retries once; unknown state fails safe and a definite server response allows the client to start a new key, while an ambiguous network failure retains the original key.

## Follow-up fix

- Added the read-only review-source `RegenerationStatus` query and used it in creation-view recovery. Active work remains generating with a durable task; failed/cancelled/unknown work is fail-safe failed with `assemblyDirty` in the view, so old media remains hidden and the creator can retry.
- Ran `bash scripts/beta-smoke-check.sh`: EXIT 0, including cloud and local Go checks, Python QA, creator/developer frontend contracts, lint/build, and HyperFrames build. Ran `bash scripts/beta-readiness-check.sh`: EXIT 0 with `CONDITIONAL`; the only remaining readiness warning is that no real AIGC video provider or text-to-video route is configured.

## Browser evidence

- Authenticated 1440x900 start page: document/client/scroll width all 1440; only 开始创作 and 我的视频 navigation; screenshot `/Users/wanglian/.codex/visualizations/2026/07/19/019f7ad3-f7d4-7f00-832a-28a858d009fa/creator-start-1440.png`.
- 1024x768 start page: client/scroll width both 1024, document height 905; screenshot `creator-start-1024.png` in the same folder.
- 390x844 projects page: client/scroll width both 390, twelve cards, no internal-term regex match; screenshot `creator-projects-390.png` in the same folder. Active nav focus outline was `rgb(232,148,18) solid 3px`.
- The running backend returned the user-safe `暂时无法读取创作进度，请稍后重试。` for the existing project's creation-view, so the live Shot workspace could not be inspected.
