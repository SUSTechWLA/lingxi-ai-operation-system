# Creator Studio Simplification Design

## Status

Approved in conversation on 2026-07-19. This design covers both the creator-facing frontend and the backend contracts required to make every visible action real and recoverable.

## Intent

Simplify the current director studio for non-technical video professionals. A creator should be able to describe a video in one sentence, attach source material, review a small number of understandable creation steps, improve intermediate work, revisit confirmed versions, review many independent Shots, and export a finished video.

The current development-oriented evidence views remain available in an independent developer console. They do not appear in the default creator navigation.

## Confirmed product decisions

- The default product surface is the creator studio.
- The developer console is a separate entry and navigation hierarchy.
- A confirmed step remains reviewable. Editing it creates a new version and explicitly reports affected downstream work.
- Intermediate artifacts support contextual natural-language revision. Text artifacts additionally support direct editing.
- The main project model is a replayable six-step creation strip.
- Long videos use a review queue plus a focused Shot inspector.
- Every Shot is shorter than 15 seconds, versioned independently, and regenerated independently.
- Regenerating one Shot must not regenerate other Shots. It may only invalidate that Shot's accepted candidate and the final assembly.
- Existing warm-yellow, dark-brown, light-card, rounded visual language is preserved.
- Backend state is the source of truth. The frontend must not infer workflow completion or simulate unsupported mutations.

## Current-state audit

The current creator and developer concerns are mixed in one seven-item navigation: Project, Review, Trace, Artifacts, Roles, Export, and Settings. The overview exposes implementation vocabulary such as Provider, Runner, Trace, Final QA, provenance, role boundaries, and stale artifact indexes. The most valuable creator behavior already exists but is distributed across multiple pages.

The current `DirectorStudioPage.tsx` also combines project loading, workflow polling, review actions, artifact inspection, Shot production, trace debugging, export, and system settings in one very large file. The redesign separates these responsibilities without changing unrelated brand styling.

## Product surfaces

| Surface | Audience | Default entry | Primary jobs |
|---|---|---|---|
| Creator studio | Video creators and production staff | Yes | Start, review, revise, approve, revisit, and deliver videos |
| Developer console | Internal development and operations | No | Inspect Runs, Trace, roles, raw artifacts, providers, QA, provenance, and diagnostics |

The creator build exposes only `Start creating` and `My videos` as persistent navigation. Connection preferences and account actions live in the profile menu. A blocking connection problem is explained in context with one corrective action; healthy system details stay hidden.

The developer console uses its own route hierarchy. It is enabled in development builds with `VITE_ENABLE_DEVELOPER_CONSOLE=1` and can later be gated by an authenticated developer role. Production creator navigation never links to it.

## Creator information architecture

### Start creating

The landing screen has one dominant prompt: “Describe the video you want to make.” It accepts a sentence or paragraph and optional images, videos, audio, and documents. Duration, aspect ratio, and target platform are progressive options, not required upfront fields.

Recent projects appear below the prompt as a small continuation section. System readiness appears only if it blocks creation.

### Project workspace

Every project uses this six-step strip:

1. Requirements
2. Creative direction
3. Script
4. Shots and materials
5. Video preview
6. Delivery

Internal stages map into these user steps:

| Creator step | Internal responsibilities hidden behind it | Primary review object |
|---|---|---|
| Requirements | Creation profile, brief, source materials, target duration and format | A plain-language production brief |
| Creative direction | Proposal, research, style and feasibility | Direction, audience, tone and visual reference |
| Script | Script generation, timing and audio-master preparation | Readable script with estimated timing |
| Shots and materials | Shot split, storyboard, references, prompts, candidates, overlays and per-Shot QA | Independent Shot queue and Shot inspector |
| Video preview | Accepted-Shot gate, assembly preview, captions, audio and final QA preview | Playable assembled preview with time-coded comments |
| Delivery | Final render, package, export and publishing copy | Downloadable video and delivery files |

Each step displays only the artifact or media the creator can judge. Internal nodes, tool names, storage references, provider job IDs, and raw QA payloads stay in the developer console.

## Visual direction

### Subject, audience, and single job

- Subject: an AI-assisted video production desk.
- Audience: video professionals who understand scripts, Shots and edits but not software infrastructure.
- Single job: help the creator judge the current production artifact and confidently move the video forward.

### Tokens

The implementation reuses the current Tailwind tokens:

| Role | Value |
|---|---|
| Primary action | `#E89412` |
| Deep ink | `#2B1606` |
| Warm canvas | `#FFF6D6` |
| Card paper | `#FFFDF6` |
| Dividers | `#E8CF86` |
| Confirmed state | `#1F9D62` |

No new gradient system, dark theme, or broad palette replacement is part of this work.

### Type roles

- UI and navigation: existing system sans stack headed by PingFang SC and Microsoft YaHei.
- Script reading: system Songti stack for the artifact body only, so the script reads like a production document without restyling the whole product.
- Shot numbers, versions, and timecodes: system monospace stack.

No network font dependency is introduced.

### Layout

Landing screen:

```text
┌──────────────┬─────────────────────────────────────────────┐
│ 躺营          │  What video do you want to make?            │
│              │  [ large prompt + attach materials ]        │
│ + Start      │  [ Start creating ]                         │
│ My videos    │                                             │
│              │  Continue recent videos                     │
└──────────────┴─────────────────────────────────────────────┘
```

Project workspace:

```text
┌──────────────┬─────────────────────────────────────────────┐
│ Start        │  Requirements → Direction → Script → Shots  │
│ My videos    │                  → Preview → Delivery        │
│              ├──────────────┬──────────────────┬───────────┤
│              │ review queue │ current artifact │ improve   │
│              │ or versions  │ or Shot preview  │ inspector │
└──────────────┴──────────────┴──────────────────┴───────────┘
```

### Signature element and self-critique

The memorable element is a production filmstrip that records six real creation steps and allows replay of confirmed work. Within the Shot step, it becomes a contact-sheet-like review queue.

A generic sidebar dashboard would not be specific enough to video production. The revision therefore removes five developer-oriented primary destinations and concentrates product identity in the filmstrip and Shot contact sheet. This is the only aesthetic risk; the rest of the visual system remains deliberately familiar.

## Intermediate artifact interaction

Every step has the same predictable structure:

- Current artifact or playable media is the dominant surface.
- Primary action: `Confirm and continue`.
- Secondary actions: `Tell AI how to change`, `Edit directly` when text is editable, and `View versions`.
- A confirmed step remains clickable and readable.
- Starting a revision first opens an impact summary.
- Confirming the impact creates a new immutable version.
- Old versions remain visible and restorable.
- Restoring an old version creates a new current version derived from it; it never rewrites history.

For image and video artifacts, the creator can select an image region or add a time-coded comment before describing the requested change. The revision request carries this selection as structured context.

## Long-video Shot workspace

### Main layout

The `Shots and materials` step uses three regions:

1. Review queue: virtualized, filterable, and grouped by chapter or scene.
2. Shot inspector: large preview, script segment, time range, candidate tabs, references, and adjacent-Shot preview.
3. Improvement inspector: natural-language request, lock controls, impact, estimated time, and generation action.

The header summarizes total, confirmed, awaiting review, generating, and failed Shots. Default filtering is `Needs my attention`, not `All`.

Supported filters are status, chapter, scene, generation state, QA state, and transcript keyword. The list supports cursor pagination and virtualization so 100 or more Shots do not create 100 active video players.

### Shot review behavior

- A Shot is independently reviewable and has its own immutable candidate history.
- The creator can compare candidates, inspect a candidate's QA summary, accept one candidate, or derive a new candidate.
- Generating one Shot runs asynchronously. The creator continues reviewing other Shots.
- Bulk confirmation is allowed for selected already-reviewed candidates.
- Bulk regeneration is not offered as a default action.
- Adjacent Shots can be previewed for continuity, but they are not mutated by the current Shot action.

### Isolation guarantees

- `durationSec` must be greater than zero and strictly less than 15 seconds at frontend validation, API validation, persistence, and assembly validation.
- A regeneration request identifies one `shotId`, one `baseVersion`, one scope, one lock set, and one idempotency key.
- The backend appends a candidate and never overwrites an earlier candidate.
- A local Shot change only invalidates that Shot's accepted-candidate state and the final assembly.
- Other Shot candidates, approvals, and jobs remain unchanged.
- Rebuilding final assembly does not regenerate Shots.
- A project-level script, character, audio-master, or style change may affect multiple Shots. The backend must return the exact impact list and require a separate confirmation before any affected Shot is queued.
- No global cascade is allowed without a visible impact confirmation.

### Regeneration scopes and locks

Supported scopes are `prompt`, `reference`, `base_media`, `overlay`, `audio_alignment`, and `full_shot`. The current backend request accepts `scope` but does not enforce it; this redesign makes scope functional throughout planning and execution.

Field-level locks are separate from the current whole-Shot lock. Locks can preserve duration, narration, character identity, wardrobe, scene, camera, first frame, last frame, reference set, and accepted overlay. Whole-Shot lock remains an administrative guard.

## Frontend architecture

The current page is decomposed into focused feature boundaries:

```text
src/
  app/
    CreatorRouter.tsx
    CreatorShell.tsx
  features/creator-studio/
    start/
    projects/
    steps/
    artifacts/
    shots/
    preview/
    delivery/
  features/developer-console/
    trace/
    roles/
    artifacts/
    diagnostics/
    settings/
  services/
    creatorApi.ts
    developerApi.ts
```

Electron-friendly hash routes provide independent entries:

- `#/create`
- `#/videos`
- `#/videos/:projectId/steps/:stepId`
- `#/developer/*`

The creator route does not request Trace, role-agent, raw artifact-index, or diagnostics data. The developer route can initially reuse the existing Trace, Roles, raw Artifacts, Export diagnostics, and Desktop settings components while they are moved behind the new shell.

Server-derived view models drive the creator UI. Frontend selectors may format labels but do not calculate authoritative step or dependency status.

## Backend capabilities and gaps

### Existing capabilities to reuse

- Artifact revision creation with parent and version metadata.
- Artifact history lookup.
- LLM-assisted artifact revision.
- Downstream stale marking.
- Agent review approve, reject, submit-edited, and regenerate actions.
- Per-Shot list, get, update, approve, reject, lock, unlock, and regenerate endpoints.
- Shot candidates, QA reports, repair plans, accepted candidate IDs, and final assembly gating.

### Required additions

| Gap | Required behavior |
|---|---|
| Creator step aggregation | Map internal stages and artifacts into six stable user steps on the server |
| Revision impact preview | Return exact affected steps or Shot IDs before mutation |
| Confirmed direct edit | Permit a confirmed artifact to create a new version and resume from the correct stage |
| Version restore | Restore a historical artifact or Shot candidate by deriving a new current version |
| Shot list scale | Cursor pagination, filters, summary counts, and lightweight thumbnail DTOs |
| Shot history | Preserve full Shot revision metadata, not only the current `ShotUnit.Version` counter |
| Candidate acceptance | Accept an explicit candidate ID with optimistic concurrency |
| Scoped regeneration | Enforce regeneration scope and field locks through execution |
| Async task recovery | Return durable task IDs and resume status after disconnect |
| Isolation enforcement | Prevent targeted Shot mutation from invalidating or queuing other Shots |

## Proposed creator APIs

### Project steps

- `GET /api/video-projects/:id/creation-view`
- `GET /api/video-projects/:id/steps/:stepId/versions`
- `POST /api/video-projects/:id/steps/:stepId/revision-impact`
- `POST /api/video-projects/:id/steps/:stepId/revisions`
- `POST /api/video-projects/:id/steps/:stepId/confirm`
- `POST /api/video-projects/:id/steps/:stepId/versions/:version/restore`

`creation-view` returns a project summary and six step summaries. A step summary contains stable creator-facing state, current artifact references, version, review actions, and counts. It does not expose internal tool topology.

### Shot review

- `GET /api/video-projects/:id/shots/summary`
- `GET /api/video-projects/:id/shots?cursor=&limit=&status=&chapter=&query=`
- `GET /api/video-projects/:id/shots/:shotId/workspace`
- `GET /api/video-projects/:id/shots/:shotId/history`
- `POST /api/video-projects/:id/shots/:shotId/regeneration-impact`
- `POST /api/video-projects/:id/shots/:shotId/regenerations`
- `POST /api/video-projects/:id/shots/:shotId/candidates/:candidateId/accept`
- `POST /api/video-projects/:id/shots/:shotId/candidates/:candidateId/restore`

Mutation requests carry `baseVersion`. The server rejects stale writes with HTTP 409 and returns the current version summary. Regeneration requests carry an `Idempotency-Key`; retrying the same request returns the existing task instead of charging for a duplicate generation.

### Regeneration request

```json
{
  "baseVersion": 3,
  "scope": "base_media",
  "instruction": "镜头推进慢一点，让晨光更柔和",
  "locks": ["duration", "character", "narration", "first_frame", "last_frame"],
  "selection": {
    "startMs": 2100,
    "endMs": 5300
  }
}
```

### Impact response

```json
{
  "shotId": "shot-02",
  "affectedShotIds": ["shot-02"],
  "invalidatesFinalAssembly": true,
  "regeneratesOtherShots": false,
  "estimatedDurationSec": 360,
  "requiresConfirmation": true
}
```

## Data flow

### Revise a confirmed project step

1. Frontend requests revision impact with the current version and proposed change.
2. Backend resolves artifact dependencies and returns affected creator steps.
3. Creator confirms the impact.
4. Backend creates a new artifact version, marks affected downstream artifacts stale, and resets the correct execution node.
5. Backend returns the new version, durable task ID, and updated six-step view.
6. Frontend renders server state and resumes polling after reconnect.

### Regenerate one Shot

1. Frontend submits `shotId`, `baseVersion`, scope, locks, selection, and idempotency key.
2. Backend validates ownership, version, duration, scope, and locks.
3. Backend creates a Shot revision event and a durable generation task.
4. The worker appends a candidate to that Shot only.
5. QA runs for the new candidate only.
6. Creator accepts a candidate explicitly.
7. Backend updates that Shot's accepted candidate and marks final assembly dirty.
8. Other Shots remain byte-for-byte and state-for-state unchanged.

## Error and recovery design

- Failed generation: the current Shot shows a plain-language reason and `Retry this Shot`; other reviews continue.
- Offline or closed app: durable tasks continue; reopening reloads task and candidate status from the backend.
- Version conflict: HTTP 409 displays the newer version and requires the creator to reapply or discard the attempted change.
- Duplicate submission: idempotency returns the existing task.
- Missing material: the Shot stays actionable and identifies the exact missing material with an upload action.
- Provider unavailable: explain that generation cannot start and link to the one relevant connection setting; do not expose provider internals.
- Partial assembly failure: keep accepted Shot states and retry assembly only.
- Expensive action: show affected Shot count and estimated duration before confirmation.
- Empty project: return to the one-sentence creation prompt with a specific example.

## Performance, accessibility, and responsive behavior

- Shot lists use server pagination and UI virtualization.
- List responses contain thumbnail metadata, not full media payloads.
- Only the selected Shot mounts a full video player.
- Generation status uses bounded polling initially; a server event stream may replace it without changing view contracts.
- Keyboard navigation supports step switching, Shot movement, preview play/pause, confirm, and opening revision controls.
- Focus rings remain visible, actions have text labels, and status is never color-only.
- Reduced-motion mode removes step-entry and progress animations.
- Desktop is the primary dense Shot-review layout. Narrow widths collapse the improvement inspector below the selected Shot and keep the queue accessible as a drawer.

## Testing strategy

### Backend

- Unit tests for duration validation at every ingress and assembly boundary.
- Unit tests for `baseVersion` conflicts and idempotent retries.
- Unit tests for field locks and every regeneration scope.
- Artifact and Shot history tests proving previous versions are immutable.
- Restore tests proving restoration creates a new version.
- Isolation tests proving Shot 12 regeneration does not mutate, stale, approve, reject, or queue Shots 1–11 and 13–100.
- Assembly tests proving only accepted, non-stale candidates are consumed.
- Project-level impact tests proving a multi-Shot cascade requires explicit confirmation.

### Contract and integration

- Go API schema and TypeScript client contract tests for step, Shot, candidate, impact, and task states.
- Integration tests for revise-impact → revise → stale → resume → re-review.
- Integration tests for regenerate Shot → candidate → QA → accept → assembly dirty.
- Restart tests proving durable tasks and view state recover after frontend and backend reconnect.

### Frontend and end-to-end

- Component tests for the six-step strip, version drawer, impact confirmation, filters, Shot inspector, and conflict state.
- E2E with 100 Shots covering cursor loading, virtualization, filtering, background generation, continued review, candidate comparison, acceptance, and reconnect.
- E2E asserting developer terms and navigation are absent from creator routes.
- E2E asserting developer routes retain Trace, roles, raw artifacts, diagnostics, and settings when enabled.
- Visual regression checks confirm the existing palette, card language, and responsive behavior remain intact.

## Acceptance criteria

- A new creator can start a project from one sentence and optional materials without seeing technical configuration.
- Creator navigation contains only `Start creating` and `My videos`.
- Every project shows six understandable steps and the current required action.
- Confirmed steps are readable, versioned, revisable, and restorable.
- Modifying an upstream step shows impact before mutation.
- A project with 100 Shots remains usable and does not mount 100 video players.
- Every Shot is strictly shorter than 15 seconds.
- One Shot can generate, fail, retry, compare, accept, and restore without changing any other Shot.
- The creator can continue reviewing while generation tasks run.
- Final assembly consumes exactly one accepted current candidate per Shot.
- Developer Trace, roles, raw artifact indexes, provider details, QA provenance, and diagnostics remain accessible only through the independent developer console.
- Frontend state after reload matches backend state with no client-only workflow assumptions.

## Non-goals

- Rebranding the product or replacing the current color system.
- Rewriting generation models, Shot-splitting semantics, QA algorithms, or rendering engines.
- Adding automatic publishing.
- Adding default bulk regeneration.
- Removing development evidence or diagnostics.

## Rollout

1. Add server view contracts, Shot isolation tests, and missing mutation endpoints behind feature flags.
2. Build the creator shell and six-step read-only view against those contracts.
3. Add revision, version, restore, and per-Shot mutation flows.
4. Move existing development pages into the independent developer console.
5. Run contract, integration, 100-Shot E2E, visual regression, and existing frontend/backend suites.
6. Make the creator studio the default route only after the new backend contracts pass isolation and recovery tests.
