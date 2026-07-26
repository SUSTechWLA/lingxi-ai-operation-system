# Task 5 report: completed delivery recovery

## Outcome

- Completed and archived task cards now enter the `delivery` workspace regardless of whether the delivery media is playable, missing, or unsupported.
- A missing or unsupported completed delivery shows one `重新生成成片` action. It calls the existing delivery-step regeneration API with the current delivery artifact identity and the server-projected affected-step confirmation.
- The recovery refuses to continue if the server says it would affect another step, preserving requirements, creative direction, script, and Shots. The generic step-regeneration control is hidden for this completed-delivery case so it cannot offer a second recovery action.
- After queueing, the workspace refreshes the authoritative creation view so durable task progress and active tasks update immediately.

## RED evidence

`npm run test:creator` exited 1 before production changes:

`TypeError: logic.completedTaskLandingStep is not a function`

The new executable checks covered a completed task with playable and missing delivery media, the delivery-only missing/unsupported repair scope, one recovery button, current delivery base identity, and server-projected affected-step confirmation.

## GREEN evidence

- `npm run test:creator` — exit 0: `creator studio logic and client contract checks passed`.
- `npm run build` — exit 0: strict TypeScript checks and Vite production build passed.
- `git diff --check` — exit 0.

`npm run lint` was also run. It still exits 1 only because of three pre-existing `no-useless-escape` errors in `frontend/scripts/creator-studio-logic-check.mjs` lines 282 and 286, plus one pre-existing fast-refresh warning in `ReviewableTextSurface.tsx`; this task introduced no lint diagnostics.

## Files changed

- `frontend/src/features/creator-studio/logic.ts`
- `frontend/src/features/creator-studio/VideoLibraryPage.tsx`
- `frontend/src/features/creator-studio/ProjectWorkspacePage.tsx`
- `frontend/src/features/creator-studio/components/PreviewDeliveryPanel.tsx`
- `frontend/scripts/creator-studio-logic-check.mjs`

## Scope notes

- The plan referenced `ProjectsHomePage.tsx`; the current implementation uses `VideoLibraryPage.tsx` for completed-task cards, so that is the actual routing surface updated.
- Existing assembly rebuilding remains separate and continues to avoid regenerating individual Shots.

## Review fix round 1

- Recovery now requires a loaded, current delivery artifact identity and version. Loading, load errors, historical selections, and version mismatches remain diagnostic-only and never expose a regeneration control.
- A selected video is now probed before final-QA metadata is present, so missing files and unsupported codecs receive a concrete diagnosis. Final delivery, the confirmed badge, and the download package remain gated by both passed final QA and browser-playable metadata.
- On a successful delivery regeneration, the workspace adopts `result.view` immediately, including its active task. The follow-up authoritative refresh runs in the background and cannot convert a successfully queued regeneration into a false user-facing failure.

### Fix-round RED evidence

`npm run test:creator` failed before each corresponding implementation change:

- `a delivery artifact still loading is not conclusive missing media` returned a recovery scope.
- `logic.canDeliverCreatorFinalVideo is not a function` proved the final-QA delivery gate was not represented independently from probing.
- `recovery never uses an older version that shares the current artifact identity` returned a recovery scope.

### Fix-round GREEN evidence

- `npm run test:creator` — exit 0: `creator studio logic and client contract checks passed`.
- `npm run build` — exit 0: TypeScript checks and Vite production build passed.
- `git diff --check` — exit 0.
- `npm run lint` remains exit 1 only for the same pre-existing three `no-useless-escape` errors in `frontend/scripts/creator-studio-logic-check.mjs` lines 282 and 286, plus the pre-existing Fast Refresh warning in `ReviewableTextSurface.tsx`; this round introduced no lint diagnostics.

## Review fix round 2

- Delivery media remains available for diagnostic probing and playback before final QA, but the player receives `allowDownload={finalVideoReady}` so neither normal controls nor the unsupported-codec state can offer a download until final review and browser metadata both pass.
- The visible final-delivery checklist now uses the same `finalVideoReady` gate as the confirmed badge and delivery package, so a merely decodable diagnostic video cannot show green completion checks.
- The source-contract test now proves the successful delivery-regeneration sequence adopts `result.view` with `onViewChanged(result.view)` before it launches the best-effort `void onAssemblyUpdated().catch(...)` refresh.

### Fix-round RED evidence

`npm run test:creator` exited 1 before the implementation change with:

`AssertionError [ERR_ASSERTION]: a diagnostic delivery video keeps playback but cannot expose a download before final QA passes`

The new checks also require the checklist to gate green marks with `finalVideoReady`, both player download locations to honor `allowDownload`, and the immediate view-adoption call to precede the asynchronous refresh.

### Fix-round GREEN evidence

- `npm run test:creator` — exit 0: `creator studio logic and client contract checks passed`.
- `npm run build` — exit 0: strict TypeScript checks and Vite production build passed.
- `git diff --check` — exit 0.
