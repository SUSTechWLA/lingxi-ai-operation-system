# AIGC Shot Runtime Design

## Problem

The current video creation flow exposes too much structured planning data to the user at the storyboard review step. A single review can contain the full `shotList`, `shotAssetPackages`, prompts, asset requirements, concat plans, and future shot details. This is difficult to review and does not match how AIGC video creation actually evolves.

For AIGC-first videos, shot production cannot start from a static, all-at-once DAG. The system must first establish global consistency assets, then let the user review and refine one shot at a time. Each step must be observable, reversible, and scoped.

## Goals

- Avoid presenting users with full-shot JSON payloads during review.
- Build AIGC videos through a controlled, linear, per-shot workflow.
- Generate and approve global consistency references before shot-level work.
- Support image/video API generation when providers are configured.
- Support external web generation plus upload when providers are missing.
- Keep each shot traceable to script text, references, prompts, generated media, uploads, and approvals.
- Allow rerunning a single shot without invalidating approved shots unless global assets change.

## Non-Goals

- Do not rewrite the entire orchestrator.
- Do not require `LLMPlanner` to know every final shot node at run start.
- Do not remove existing DAG execution, artifact storage, or review APIs.
- Do not add cloud storage of user media payloads beyond existing artifact metadata and storage references.

## Core Design

The video creation run is split into three runtime levels:

1. **Initialization DAG**
   Creates the project direction, script, global creative bible, and consistency asset pack. This DAG is mostly static and runs before shot production.

2. **Shot Runtime Controller**
   Owns a persistent shot queue. It activates exactly one shot at a time, creates a transient sub-DAG for that shot, pauses for user review or upload, and unlocks the next shot only after the current shot is approved.

3. **Final Assembly DAG**
   Runs after all shots are approved. It assembles confirmed shot clips, checks technical quality, and prepares the delivery package.

This preserves DAG-based execution while avoiding a single static DAG that tries to predict all future creative decisions.

## Runtime Flow

```text
Project start
  -> Creative Bible
  -> Consistency Asset Pack
  -> Script
  -> Shot Queue Initialization
  -> Shot Runtime Controller
       -> SHOT_01 sub-DAG
          -> shot brief review
          -> keyframe prompt
          -> keyframe API generation or external image request
          -> keyframe review/upload
          -> video prompt
          -> video API generation or external video request
          -> shot media review
          -> approve SHOT_01
       -> SHOT_02 sub-DAG
       -> ...
  -> Final Assembly DAG
  -> Quality Review
  -> Export Package
```

## Data Model

### CreativeBible

`CreativeBible` is the global creative contract for the video.

Fields:

- `projectId`
- `runId`
- `topic`
- `targetDurationSec`
- `aspectRatio`
- `videoType`
- `narrativeStyle`
- `visualStyle`
- `tone`
- `factBoundaries`
- `negativeConstraints`
- `approvedAt`
- `version`

Rules:

- Must be approved before consistency assets are generated.
- Changing it marks all downstream consistency assets and shots stale.

### ConsistencyAssetPack

`ConsistencyAssetPack` contains global references needed for AIGC shot consistency.

Fields:

- `characters`
- `scenes`
- `props`
- `styleFrames`
- `colorPalette`
- `cameraLanguage`
- `referenceRequests`
- `approvedReferences`
- `approvedAt`
- `version`

Each character, scene, and prop reference should support:

- `id`
- `displayName`
- `role`
- `views`: `front`, `side`, `back`, and optional detail views
- `storageRef`
- `source`: `api_generated`, `external_upload`, or `user_upload`
- `approvalStatus`

Rules:

- Must be approved before shot queue initialization.
- Each shot can reference this pack but cannot silently invent new main characters, main scenes, or main props.
- If required reference images cannot be generated through API, the system creates `external_generation_request` artifacts and asks the user to upload results.

### ShotQueue

`ShotQueue` is a persistent ordered list of lightweight shot briefs.

Fields:

- `projectId`
- `runId`
- `shots`
- `activeShotId`
- `status`: `INITIALIZED`, `RUNNING`, `PAUSED`, `COMPLETED`

Each shot contains:

- `shotId`
- `order`
- `narrationText`
- `durationSec`
- `visualGoal`
- `requiredGlobalReferences`
- `status`: `PENDING`, `ACTIVE`, `WAITING_USER`, `APPROVED`, `REJECTED`, `STALE`
- `currentSubRunId`
- `version`

Rules:

- Shot queue is initialized only after script and consistency assets are approved.
- The queue shows the user progress without exposing full prompt and asset package internals.
- Only one shot is active unless a future explicit batch mode is added.

### ShotRuntimeState

`ShotRuntimeState` records execution and review state for the active shot.

Fields:

- `shotId`
- `subRunId`
- `phase`
- `inputRefs`
- `promptRefs`
- `externalRequestRefs`
- `uploadedAssetRefs`
- `generatedAssetRefs`
- `reviewRefs`
- `traceRefs`
- `status`

Phases:

- `BRIEF_REVIEW`
- `KEYFRAME_PROMPT`
- `KEYFRAME_GENERATION_OR_UPLOAD`
- `KEYFRAME_REVIEW`
- `VIDEO_PROMPT`
- `VIDEO_GENERATION_OR_UPLOAD`
- `SHOT_FINAL_REVIEW`
- `APPROVED`

## Backend Architecture

### Initialization DAG Changes

The initial dynamic plan should include these conceptual stages:

- `creative_bible_generator`
- `consistency_asset_planner`
- `global_reference_request_generator`
- `video_script_generator`
- `shot_queue_initializer`

The storyboard stage should no longer output a full `shotAssetPackages` payload for all shots as review text. It should output a lightweight shot queue and a human-readable summary.

### Shot Runtime Controller

Add a backend service under `cloud-backend/internal/agents/video/service` or a closely scoped package:

- `StartNextShot(ctx, projectID, runID)`
- `StartShot(ctx, projectID, runID, shotID)`
- `PauseShotForUser(ctx, shotID, reason)`
- `RegisterShotUpload(ctx, shotID, artifactID)`
- `ApproveShot(ctx, shotID)`
- `RejectShot(ctx, shotID, feedback)`
- `GetShotRuntimeState(ctx, projectID, runID)`

Responsibilities:

- Reads approved `CreativeBible`, `ConsistencyAssetPack`, script, and active shot brief.
- Builds a transient per-shot DAG.
- Submits it to the orchestrator.
- Pauses on review or external upload requirements.
- Advances the queue after approval.

### Per-Shot Sub-DAG

The per-shot sub-DAG is created at runtime from current approved inputs.

Required nodes:

- `shot_brief_review`
- `shot_keyframe_prompt_generator`
- `shot_keyframe_generation_or_external_request`
- `shot_keyframe_review`
- `shot_video_prompt_generator`
- `shot_video_generation_or_external_request`
- `shot_final_review`

Provider handling:

- If image API is configured, call the image provider and create a `SHOT_KEYFRAME` artifact.
- If image API is not configured, create an `external_generation_request` artifact with `generationKind=image`.
- If video API is configured, call the video provider and create a `SHOT_VIDEO_CLIP` artifact.
- If video API is not configured, create an `external_generation_request` artifact with `generationKind=video`.

### Final Assembly DAG

This DAG runs only after every shot is approved.

Required nodes:

- `shot_asset_collector`
- `concat_plan_builder`
- `hyperframes_or_ffmpeg_assembler`
- `video_quality_checker`
- `final_review_generator`
- `artifact_packager`

## Frontend Experience

### Review Page

The review page should show one decision at a time.

For global stages:

- Creative Bible review
- Consistency asset pack review
- Script review

For shot stages:

- Active shot header: `SHOT_01 / 08`
- Shot narration text
- Visual goal
- Required global references
- Current phase
- Relevant prompt or generated media
- Action buttons: approve, regenerate, edit, upload result

The page should not display raw full JSON unless the user opens an advanced debug drawer.

### Assets Page

The assets page should group artifacts by:

- Global references
- Active shot
- Approved shots
- External generation requests
- Final assembly outputs

Each external request should have:

- Copy prompt
- Reference list
- Upload result
- Register result against the correct shot and generation kind

### Trace Page

Trace should show:

- Initialization DAG trace
- Current shot sub-DAG trace
- Per-shot history
- Final assembly trace

Users should be able to see why a shot is waiting: provider missing, upload required, review pending, or generation running.

## Artifact and Review Rules

- Review content should be human-readable by default.
- Structured JSON remains available in artifact content and debug views.
- `SHOT_ASSET_PACKAGE` must represent one shot only.
- `external_generation_request` must always include `relatedShotId`, `generationKind`, prompt limits, and reference limits.
- Uploaded external results must bind to the request ID and shot ID.
- Changing a global reference marks dependent unapproved shots stale.
- Approved shots remain locked unless the user explicitly chooses to rerun them.

## Observability

Every shot phase should write trace data:

- input artifact IDs
- prompt artifact ID
- provider mode: `api` or `external_upload`
- generated artifact IDs
- user approval/rejection
- rerun count
- error details

The UI should surface concise state labels:

- `准备中`
- `等待审核`
- `等待上传`
- `生成中`
- `已通过`
- `需重做`

## Migration Strategy

### Phase 1: Review Experience and Output Shaping

- Stop exposing full `shotAssetPackages` as primary review text.
- Add human-readable review summaries for shot queue and shot artifacts.
- Add frontend rendering for `CreativeBible`, `ConsistencyAssetPack`, and active shot cards.

### Phase 2: Persistent Shot Queue

- Add shot queue persistence and APIs.
- Initialize shot queue from approved script and consistency assets.
- Show active shot state in the frontend.

### Phase 3: Runtime Per-Shot Sub-DAG

- Add `ShotRuntimeController`.
- Submit one transient sub-DAG per active shot.
- Pause for upload/review and advance only after approval.

### Phase 4: Final Assembly

- Trigger final assembly DAG after all shots are approved.
- Keep existing artifact package and quality review mechanisms where possible.

## Test Strategy

Backend tests:

- Shot queue initializes only after script and consistency assets are approved.
- Per-shot sub-DAG includes only the active shot.
- Missing image provider creates image external request.
- Missing video provider creates video external request.
- Approving a shot advances the queue.
- Rejecting a shot keeps the same shot active and records feedback.
- Final assembly starts only when all shots are approved.

Frontend tests:

- Review page renders human-readable global asset review instead of raw JSON.
- Review page shows only active shot details.
- External request upload binds to the correct shot.
- Approved shot is locked and next shot becomes active.
- Trace view separates initialization, shot runtime, and final assembly.

## Acceptance Criteria

- A user never has to review the full multi-shot JSON payload.
- AIGC video runs require approved global consistency assets before shot generation.
- The UI shows one current decision at a time.
- Each shot can be regenerated independently.
- Provider-missing flows create copyable external generation requests and upload controls.
- The system can explain the current status of every shot.
- Existing release branch can still build, test, and package successfully.
