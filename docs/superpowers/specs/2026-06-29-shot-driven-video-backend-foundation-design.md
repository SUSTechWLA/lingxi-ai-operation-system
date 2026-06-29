# Shot-Driven Video Backend Foundation Design

## Goal

Upgrade the current video creation backend from a stage-oriented "generate video artifacts" flow into a shot-driven foundation that can support one-sentence video creation, per-shot planning, render strategy selection, review, locking, selective regeneration, stale propagation, and final assembly constraints.

This design is the first implementation slice only. It establishes backend contracts and deterministic rules in `cloud-backend`; it does not build the Shot Workbench frontend or fully execute expensive media generation.

## Product Flow

The target backend flow is:

```text
source message
-> VideoCreationSpec
-> script / voiceover script
-> ShotUnit[]
-> VisualPlan per shot
-> RenderStrategy per shot
-> TextLayers / prompts / render inputs
-> shot-level review
-> shot lock / unlock / regenerate
-> approved shot clips
-> final_video
-> publish_package
```

The system must not allow a one-sentence request to jump directly to `final_video`. Every video path must expose reviewable intermediate artifacts before media generation and final assembly.

## Existing Foundations

The repository already has several pieces that should be extended rather than replaced:

- `cloud-backend/internal/agents/video` owns video business behavior.
- `cloud-backend/internal/core/artifact.Artifact` already includes `UnitID`, which can represent a shot ID without adding a new generic `shot_id` column.
- `cloud-backend/internal/core/artifact` already supports versioning, `valid` / `pending` / `stale` / `rejected` statuses, human approval, dependencies, and stale propagation.
- `cloud-backend/internal/core/video/pipeline/render_strategy.go` has an early render strategy planner, but it is too coarse for exact text, TextLayers, and hybrid plans.
- Dynamic Agent Runtime, PlanGuard, PlanCompiler, Tool Registry, Artifact Review, Local Runner, and HyperFrames already exist and should remain the orchestration substrate.

## Scope

Included in this backend foundation slice:

- Add video business models for `VideoCreationSpec`, `ShotPolicy`, `RenderPreference`, `ShotUnit`, `ShotContinuity`, `PromptConstraints`, `ShotArtifactRefs`, `VisualPlan`, `TextLayerSpec`, and the visual sub-specs needed by the decider.
- Add deterministic render strategy decisions for `html_only`, `aigc_only`, `hybrid_aigc_bg_html_overlay`, `html_preview_then_aigc`, and `html_preview_then_hybrid`.
- Add backend validators/checkers for shot duration, scene complexity, text exactness, render strategy legality, AIGC no-text prompts, locked shot regeneration, and final assembly inputs.
- Add shot/spec service methods and HTTP endpoints for CRUD-style operations and review/lock/regenerate actions.
- Extend artifact stage constants and stale propagation to support shot-level stages using `Artifact.UnitID`.
- Store first-slice provenance as artifact metadata and promoted fields where available: provider, model, prompt hash, produced-by fields, input artifact IDs, output hash, cost, latency, retry count, and seed.
- Add unit tests and focused handler/service tests for the requested backend rules.
- Update OpenAPI source and regenerate generated API docs/types if public API changes.

Excluded from this slice:

- No Shot Workbench frontend implementation.
- No local-backend DB, Docker, Redis, Kafka, MinIO, or cloud LLM key dependency.
- No new media-generation provider implementation.
- No dedicated `artifact_provenance` table unless the implementation discovers that metadata cannot satisfy the required behavior.
- No deletion or incompatible rewrite of existing `video-creator/1.0.0`, `aigc-shot-video/1.0.0`, or current guided image-text flows.

## Data Model

### VideoCreationSpec

`VideoCreationSpec` is the project-level creative contract generated from the user's source message and approved before script generation.

Fields:

- `id`, `projectId`, `sourceMessage`
- `topic`, `platform`, `videoType`, `targetDurationSec`
- `aspectRatio`, `language`, `audience`, `tone`, `visualStyle`
- `reviewMode`, `shotPolicy`, `renderPreference`
- `status`, `createdAt`, `updatedAt`

Defaults:

- `reviewMode`: `shot_level_review`
- `aspectRatio`: project aspect ratio or `16:9`
- `language`: project language or `zh-CN`
- `shotPolicy.minDurationSec`: `3`
- `shotPolicy.maxDurationSec`: `15`
- `shotPolicy.preferDurationSec`: `6`
- `shotPolicy.singleSceneRequired`: `true`
- `shotPolicy.lowVisualChangeRequired`: `true`
- `shotPolicy.avoidCrossShotDependency`: `true`
- `renderPreference.defaultRenderStrategy`: `auto`
- `renderPreference.preferHTMLForText`: `true`
- `renderPreference.preferHTMLForCharts`: `true`
- `renderPreference.preferHTMLForUI`: `true`
- `renderPreference.preferAIGCForPeople`: `true`
- `renderPreference.preferAIGCForScene`: `true`
- `renderPreference.allowHybridRender`: `true`
- `renderPreference.preferLowCostPreview`: `true`

### ShotUnit

`ShotUnit` is the independently generatable unit. It may produce an AIGC clip, a HyperFrames clip, or a hybrid composite.

Required behavior:

- `durationSec` must be from 3 to 15 seconds.
- `singleScene` defaults to true.
- `visualChangeLevel` defaults to `low`.
- Each shot must have a start state, end state, main action, scene summary, transition in/out, screen text list, continuity info, prompt constraints, review status, lock status, and version.
- A locked shot cannot be regenerated or overwritten by automatic generation.

The first slice can store shot/spec records in the existing project config or a lightweight repository table depending on the current repository migration pattern. The implementation must choose the option that keeps project loading, tests, and API behavior explicit. If schema changes are made, they must be documented in the cloud backend and covered by tests.

### VisualPlan

`VisualPlan` is the cross-tool visual contract. It is not an AIGC prompt and not an HTML source file.

Core fields:

- `canvas`: aspect ratio, width, height, FPS, duration
- `background`
- `characters`
- `props`
- `textLayers`
- `uiLayers`
- `dataVisuals`
- `motionPlan`
- `cameraPlan`
- `transitionIn`
- `transitionOut`
- `style`
- `constraints`

Canvas defaults:

- `aspectRatio`: `16:9`
- `width`: `1920`
- `height`: `1080`
- `fps`: `30`
- `durationSec`: shot duration or `6`

### TextLayerSpec

All exact text must enter `TextLayerSpec`.

Fields:

- `id`, `text`, `language`, `role`, `position`
- `fontSize`, `fontWeight`, `color`, `background`
- `startSec`, `endSec`, `animation`, `mustBeExact`

Text roles:

```text
title, subtitle, keyword, label, ui_text, caption, data_text,
button_text, file_name, chat_message, code, number, brand_name
```

Rules:

- Every `ShotUnit.screenText` item must be present in `VisualPlan.textLayers`.
- Exact Chinese titles, subtitles, keywords, UI labels, tables, chat records, file names, numbers, tags, buttons, code, lists, and brand names must use `mustBeExact=true`.
- `mustBeExact=true` text must not be assigned to pure AIGC generation.
- Text timing must stay within the shot duration.

## Render Strategy Decider

The decider belongs in video business code, not generic Core orchestration. It consumes:

- `ShotUnit`
- `VisualPlan`
- `VideoCreationSpec.RenderPreference`
- available tool/capability snapshot
- cost/preview preference

Output:

- `RenderStrategy.mode`
- primary and secondary tools
- reason
- booleans for AIGC, HTML, text overlay, and compositing
- AIGC input, HTML input, and composite plan when applicable

Decision rules:

- If exact text, screen text, UI, chart, table, chat, file name, number, label, button, code, list, or brand text exists, HTML is required.
- If characters, scene atmosphere, emotional performance, natural motion, or complex camera movement is required, AIGC is allowed.
- If HTML is required and AIGC is not required, choose `html_only`.
- If AIGC is required and HTML is not required, choose `aigc_only`.
- If both are required, choose `hybrid_aigc_bg_html_overlay`.
- If low-cost preview is preferred and the shot is oral-video, information-dense, high-review-risk, or expensive to regenerate, choose `html_preview_then_aigc` or `html_preview_then_hybrid` as appropriate.
- If the decider has no strong AIGC requirement, default to `html_only` for cost and text reliability.

AIGC prompts for hybrid mode must explicitly prohibit readable text, Chinese characters, English words, UI text, subtitles, labels, and watermarks because HTML/HyperFrames owns exact text.

## Validators

Validators should be small, deterministic, and unit-tested. They should return structured issues that can be surfaced through APIs and Artifact Review.

Required validators:

- `ShotDurationChecker`: rejects shots shorter than 3 seconds or longer than 15 seconds.
- `ShotSceneComplexityChecker`: rejects `singleScene=false`, `visualChangeLevel=high`, multi-scene switching, complex montage, rapid transitions, or too many main actions.
- `PromptCompletenessChecker`: checks AIGC prompts include duration, aspect ratio, start state, end state, scene, subject, action, camera, light/style, continuity constraints, and forbidden items.
- `RenderStrategyChecker`: rejects exact text/UI/chart/table/chat with `aigc_only`; emits a cost warning when `aigc_only` has no people, scene, motion, or emotion requirement.
- `TextLayerExactnessChecker`: ensures all screen text is in TextLayers, exact text is HTML-owned, and text timing stays inside shot duration.
- `AIGCPromptNoTextChecker`: in hybrid mode, verifies the prompt bans readable text.
- `CompositeCompatibilityChecker`: checks background/overlay duration, resolution, FPS, and alpha compatibility from metadata.
- `LockedShotChecker`: blocks automatic regenerate and artifact overwrite for locked shots.
- `FinalAssemblyChecker`: allows only approved, non-stale shot clips in final assembly. Locked approved shots are valid assembly inputs because locking prevents regeneration, not use.

## Artifact Types And Stale Graph

Add or normalize artifact type/stage names for:

```text
video_creation_spec
script
shot_unit
visual_plan
render_strategy
text_layers
keyframe_prompt
keyframe_image
aigc_prompt
html_source
html_preview_video
html_overlay_video
html_overlay_alpha_video
aigc_background_video
aigc_shot_video
composited_shot_video
final_video
publish_package
artifact_provenance
```

Use `Artifact.UnitID` to bind shot-level artifacts to a shot. Project-level artifacts use empty `UnitID`.

Stale propagation:

- Changing `video_creation_spec` marks `script`, `shot_unit`, `visual_plan`, `render_strategy`, and all downstream render artifacts stale.
- Changing `script` marks `shot_unit`, `visual_plan`, `render_strategy`, and downstream render artifacts stale.
- Changing a `shot_unit` for one `UnitID` marks downstream artifacts for the same `UnitID` stale and also marks project-level `final_video` and `publish_package` stale.
- Changing `text_layers` marks HTML source/preview/overlay/composite for the same shot stale and marks final project outputs stale.
- Changing `aigc_prompt` marks AIGC background/shot/composite for the same shot stale and marks final project outputs stale.
- Changing `render_strategy` marks all strategy-dependent artifacts for the same shot stale and marks final project outputs stale.

The existing stage-based graph remains valid for legacy flows. The new graph must not break current `proposal -> script -> storyboard -> composition -> preview -> render -> quality -> package` behavior.

## API Surface

Add public API only where needed for backend foundation tests and future frontend use. Existing routes remain compatible.

Minimum endpoints:

```text
GET    /api/video-projects/:id/spec
POST   /api/video-projects/:id/spec
POST   /api/video-projects/:id/spec/generate
POST   /api/video-projects/:id/spec/approve
POST   /api/video-projects/:id/spec/reject

GET    /api/video-projects/:id/shots
POST   /api/video-projects/:id/shots
POST   /api/video-projects/:id/shots/generate
GET    /api/video-projects/:id/shots/:shotId
PATCH  /api/video-projects/:id/shots/:shotId
POST   /api/video-projects/:id/shots/:shotId/approve
POST   /api/video-projects/:id/shots/:shotId/reject
POST   /api/video-projects/:id/shots/:shotId/lock
POST   /api/video-projects/:id/shots/:shotId/unlock
POST   /api/video-projects/:id/shots/:shotId/regenerate

POST   /api/video-projects/:id/shots/:shotId/visual-plan/generate
POST   /api/video-projects/:id/shots/:shotId/render-strategy/decide
POST   /api/video-projects/:id/shots/:shotId/text-layers/generate
POST   /api/video-projects/:id/assemble
POST   /api/video-projects/:id/publish-package/generate
```

If these are implemented in this slice, update `cloud-backend/internal/core/apispec/cloud_spec.go`, run `make gen-docs`, and run `make api-docs-check`.

## Agent Runtime And Tool Registry

Do not hard-code video business rules into generic Core execution. Instead:

- Add tool metadata where useful: `supports_shot_unit`, `supported_render_modes`, `input_artifact_types`, `output_artifact_types`, duration constraints, scene constraints, and text constraints.
- Extend video plan judging/guarding so video creation plans must include spec, script, shot units, visual plans, render strategies, and review gates before final assembly.
- Keep PlanGuard generic: video-specific constraints can live in video plan judge/director policy and tool manifests.
- The final assembly step must reject unapproved or stale shot clips.

## Skill Compatibility

This backend slice should not delete existing skills. It may add `cloud-backend/skills/video-creator/1.1.0` in a later implementation task or as a small follow-up after backend contracts compile.

The backend contracts should be named so skill stage outputs can map directly:

- `creation_spec`
- `script`
- `shot_unit_design`
- `visual_plan_generation`
- `render_strategy_decision`
- `text_layer_generation`
- `keyframe_prompt_generation`
- `aigc_prompt_generation`
- `html_preview_generation`
- `html_overlay_generation`
- `aigc_video_generation`
- `hybrid_composite`
- `shot_clip_review`
- `final_assembly`
- `publish_package`

## Tests

Implementation must start with focused tests for the deterministic rules.

Required tests:

- `TestVideoCreationSpec_Defaults`
- `TestShotDurationChecker_RejectUnder3Sec`
- `TestShotDurationChecker_RejectOver15Sec`
- `TestShotDurationChecker_Accept3To15Sec`
- `TestShotSceneComplexityChecker_RejectMultiScene`
- `TestShotSceneComplexityChecker_RejectHighChange`
- `TestPromptCompletenessChecker_RejectMissingFields`
- `TestRenderStrategy_TextOnly_UsesHTMLOnly`
- `TestRenderStrategy_AIGCSceneNoText_UsesAIGCOnly`
- `TestRenderStrategy_AIGCSceneWithExactText_UsesHybrid`
- `TestTextLayerExactness_ScreenTextMustBecomeTextLayer`
- `TestAIGCPromptNoText_WhenHybrid`
- `TestCompositeCompatibility`
- `TestLockedShot_CannotRegenerate`
- `TestLockedShot_CannotOverwriteArtifact`
- `TestTextLayerChange_MarksHTMLAndCompositeStale`
- `TestAIGCPromptChange_MarksBackgroundAndCompositeStale`
- `TestShotChange_MarksDownstreamArtifactsStale`
- `TestRejectReason_UsedByRegenerate`
- `TestFinalAssembly_UsesOnlyApprovedShots`
- `TestOralVideo_PreferHTMLPreview`

Fake demo acceptance input:

```text
做一个60秒口播视频，主题是：AI替代的不是岗位，而是整套工作流程。画面中需要出现“几个表格”“几份文档”“十几条聊天记录”。
```

Expected backend behavior:

- Generates a spec and script.
- Produces 8 to 12 shots for a 60 second target.
- All shots are 3 to 15 seconds.
- The text phrases `几个表格`, `几份文档`, and `十几条聊天记录` appear in TextLayers with `mustBeExact=true`.
- Any shot containing those exact phrases uses `html_only`, `hybrid_aigc_bg_html_overlay`, or a preview-then-hybrid strategy, never `aigc_only`.
- Hybrid AIGC prompts ban readable text.
- Individual shots can be approved, rejected with reason, locked, unlocked, and selectively regenerated.
- Final assembly rejects stale or unapproved shots.

Verification commands:

```bash
cd cloud-backend && go test ./...
cd cloud-backend && go test -race ./...
cd local-backend && go test ./...
cd frontend && npm run build
```

When API docs change:

```bash
cd cloud-backend
make gen-docs
make api-docs-check
```

## Rollout Plan

Implement in small commits:

1. Add models, defaults, decider, validators, and unit tests.
2. Add shot/spec services and repository persistence with tests.
3. Add artifact stage constants and shot-aware stale propagation tests.
4. Add thin HTTP handlers and OpenAPI updates.
5. Add plan/tool guard metadata and fake demo tests.

Each step should keep existing beta flows passing before moving to the next one.

## Risks

- API surface can grow too quickly. Keep endpoints thin and back them with service tests before adding frontend behavior.
- Storing shot records in project config may be faster but can become hard to query. Use it only if it keeps this slice simpler and still supports shot list retrieval, lock checks, and stale propagation.
- A dedicated provenance table may be useful later. For this slice, metadata is acceptable because artifact columns already carry provider, model, prompt hash, content hash, dependencies, and produced-by fields.
- PlanGuard should not become video-specific Core code. Video-specific constraints belong in video plan judging, directors, manifests, and validators.
- Legacy stage-based stale tracking must remain intact.

## Acceptance

This design is complete when the implementation can prove:

- Backend models and defaults match the requested shot policy and render preferences.
- Render strategy cannot choose `aigc_only` for exact text, UI, tables, charts, chat records, code, numbers, or other exact text layers.
- Locked shots cannot be automatically regenerated or overwritten.
- Shot-level upstream edits stale only the correct shot downstream artifacts plus project-level final outputs.
- Final assembly uses only approved, non-stale shot clips.
- Existing cloud/local/frontend verification still passes.
