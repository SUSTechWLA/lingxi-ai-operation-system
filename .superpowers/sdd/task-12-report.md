# Task 12 Report: Long-video Shot review queue

Completed the scalable Shot review queue and inspector while preserving the existing warm-yellow, deep-brown creator workspace cards.

## TDD evidence

- RED: `cd frontend && npm run test:creator` failed with `logic.shotQueueWindow is not a function` after the 100-Shot window/default-filter/append-selection contract was added.
- RED: `go test ./internal/agents/video/service -run '^TestListShotPageFriendlyStatusFiltersFullSetBeforePagination$' -count=1` initially returned an empty `needs_attention` page because the endpoint only matched `reviewStatus` exactly.
- GREEN: manual fixed 64px virtualization renders at most 12 rows for 100 Shots and a 480px viewport; append preserves selection and the queue defaults to `needs_attention`.
- GREEN: the backend applies `all|needs_attention|confirmed|generating|failed` to the complete sorted durable Shot set before cursor pagination. Attention and failure include rejected/stale/pending, human review, terminal regeneration failures, and `SHOT_QA_FAILED` candidate/QA state; legacy review-status filters remain compatible. Handler coverage verifies all friendly queue query values are forwarded intact.

## Delivered behavior

- 24-item server-paged queue with six-item prefetch, fixed-row windowing, chapter labels inside rows, local search/chapter controls, and a bounded (three-way) visible-thumbnail artifact resolver. The queue never mounts a video player.
- One selected-candidate video player in the inspector, artifact URL fallback, safe unavailable-preview state, Chinese quality summary, narration, timing, reference count, neighboring Shots, explicit candidate acceptance, strict `<15s` defensive UI gating, and conflict reload copy.
- Local scoped improvement with six scopes, locks, target-only impact validation, accessible confirmation dialog, a stable UUID idempotency key per logical operation, target-only update/reload, and failed-Shot retry copy. Queue selection remains enabled during work.
- Selected Shot durable-task polling now only reloads the selected workspace; request tokens and abort controllers suppress stale workspace/media operations.

## Verification

- `cd frontend && npm run test:creator && npm run lint && npm run test:director && npm run test:developer-build && npm run build`
- `cd cloud-backend && go test ./internal/agents/video/service ./internal/agents/video/handler ./internal/core/apispec -count=1 && make api-types-check && git diff --check`

## Review-fix follow-up

- RED: the new creator logic contract failed because replacement-page selection and candidate/QA failure retry selectors did not exist.
- GREEN: reset pages now preserve the selected Shot only while it remains in the replacement result, including safe fallback from a stale external selection; candidate status and nested QA report failures both make retry available.
- A regeneration response now remains durable client state immediately: the target row adopts its returned task status and the returned task is merged into the shared creation-view task set. The existing single workspace poller supports multiple concurrent Shots; task signature changes refresh queue and summary, while terminal selected-task transitions reload only that workspace.
- The Shot conflict copy is held by the parent workspace, so reloading the inspector cannot erase `这个 Shot 已有更新，请基于最新版本重试`.
- Follow-up race fix: adopting a new durable Shot task increments the creation-view request token before merging it into `activeTasks`; an older in-flight poll response therefore fails the latest-token guard and cannot discard concurrent newly adopted Shot work.
