# Shot Generation Routing and Asset Fusion Design

## Context

This document extends `2026-07-01-aigc-shot-runtime-design.md`.

The existing runtime design defines global consistency assets, an active shot queue, per-shot sub-DAGs, external generation requests, upload binding, and final assembly. The current implementation already has pieces of this model: `ShotUnit`, `VisualPlan`, `RenderStrategy`, `ShotArtifactRefs`, `DecideRenderStrategy`, `render_strategy_planner`, `asset_decision_agent`, `image_asset_generator`, `text_image_to_video_generator`, `SHOT_ASSET_PACKAGE`, and frontend shot upload slots.

The missing capability is execution-grade routing: the system can describe that a shot may need AIGC or HyperFrames, but the default video agent path does not yet force every shot through an explicit generation strategy, asset dependency graph, and fusion step before final render.

## Problem

The current flow can produce reviewable prompts and a HyperFrames preview, but shot generation is not yet smart enough to consistently answer:

- Which visual elements need AIGC image or video generation.
- Which elements should stay in deterministic HyperFrames layers.
- Which elements need user-provided or externally generated media.
- Which shots need hybrid compositing.
- Which generated or uploaded assets should be merged into the final video.

Without this, a high-quality video path falls back to either prompt-only external work or a mostly text/card HyperFrames render. That is acceptable for smoke tests, but not enough for an internal beta promise of one-sentence video creation with reviewable intermediates and better final quality through asset fusion.

## Goals

- Add an explicit per-shot generation plan before media generation or rendering.
- Route each shot to one of several production modes: HyperFrames-only, AIGC video, AIGC image plus HyperFrames, hybrid AIGC background plus HyperFrames overlay, external/user asset, or placeholder preview.
- Preserve exact text, UI, charts, subtitles, filenames, brand names, and data labels in HyperFrames instead of asking AIGC to render them.
- Use AIGC for people, character motion, cinematic scenes, natural movement, camera movement, and mood-heavy visual material.
- Use generated images, uploaded media, and shot clips as timed media assets inside HyperFrames or an assembler.
- Keep every generated dependency reviewable and attributable to a shot.
- Produce a final playable `VIDEO` artifact through the existing local artifact path.

## Non-Goals

- Do not replace the existing dynamic agent planner wholesale.
- Do not require all users to configure video generation providers before the flow can run.
- Do not make release branch changes directly.
- Do not build a full non-linear editor in this iteration.
- Do not solve advanced audio, BGM, or mixing beyond preserving existing shot audio/subtitle fields.

## Generation Modes

### `html_only`

Use when the shot is mostly exact text, UI, charts, data visualization, title cards, subtitles, quote cards, or explainers. HyperFrames owns the complete shot.

### `aigc_video`

Use when the shot is mostly natural motion, character performance, scene atmosphere, camera movement, or cinematic B-roll, with no exact text requirement inside the visual.

### `aigc_image_then_hyperframes`

Use when a stable generated image or uploaded image is enough for the visual base, while HyperFrames adds motion, text, captions, zoom, pan, callouts, or data layers.

### `hybrid_aigc_bg_html_overlay`

Use when AIGC should generate the moving background or character/scene clip, while HyperFrames must overlay exact text, subtitles, cards, UI, brand labels, or annotations.

### `external_or_user_asset`

Use when the shot depends on logos, user-provided files, licensed material, news screenshots, product images, or other assets that should not be hallucinated.

### `placeholder_preview`

Use when required media is missing but the system should still produce a low-cost preview so the user can review structure and timing.

## Data Model Additions

### `ShotGenerationPlan`

Each shot gets a generation plan before prompt generation and rendering.

Fields:

- `shotId`
- `mode`
- `primaryTool`
- `secondaryTools`
- `reason`
- `confidence`
- `riskLevel`
- `requiredAssets`
- `renderInputs`
- `fusionPlan`
- `fallbackPlan`
- `reviewFocus`

### `ShotAssetNeed`

Represents one dependency required by the shot.

Fields:

- `id`
- `kind`: `image`, `video`, `audio`, `subtitle`, `html_overlay`, `user_asset`
- `role`: `character_reference`, `scene_reference`, `prop_reference`, `keyframe`, `storyboard`, `background_video`, `overlay`, `logo`, `uploaded_media`
- `source`: `aigc_image`, `aigc_video`, `hyperframes`, `user_upload`, `external_generation`, `open_asset`, `placeholder`
- `required`
- `approvalStatus`
- `storageRef`
- `relatedShotId`
- `locks`

### `FusionPlan`

Defines how media assets become the shot or final video.

Fields:

- `shotId`
- `baseLayer`: video, image, or generated HTML background.
- `overlayLayers`: text, subtitle, UI, chart, callout, image, or mask layers.
- `timedMedia`: media clips with start, duration, track index, crop, fit, and opacity.
- `assembler`: `hyperframes`, `ffmpeg`, or `hybrid`.
- `outputArtifactKind`: usually `SHOT_VIDEO_CLIP`, `HYPERFRAMES_SHOT`, or `composited_shot_video`.

## Decision Rules

The first implementation should use deterministic scoring plus optional LLM enrichment.

### AIGC Score

Increase when the shot has:

- character movement or facial performance
- natural scene motion
- camera movement that affects viewer emotion
- cinematic B-roll
- realistic object interaction in a stylized non-real human world
- mood/atmosphere that cannot be represented by cards alone

### HyperFrames Score

Increase when the shot has:

- exact text
- subtitles
- charts or data labels
- UI, code, filenames, chat messages, product labels, or brand names
- deterministic timing requirements
- low visual change

### User or External Asset Score

Increase when the shot mentions:

- logo, product screenshot, user upload, real document, news image, website capture, local file, or copyrighted/public identity material

### Hybrid Trigger

Use hybrid when both AIGC score and HyperFrames score are high. The default split is:

- AIGC owns people, scene, natural movement, camera feel, and atmosphere.
- HyperFrames owns text, captions, UI, diagrams, exact labels, and final overlay timing.

## Runtime Flow

1. `shot_splitter` creates lightweight shots.
2. `shot_generation_planner` reads each shot, visual plan, provider capabilities, and available uploaded/generated artifacts.
3. It writes `SHOT_GENERATION_PLAN` artifacts and updates `shotAssetPackages`.
4. `asset_decision_agent` creates concrete `ShotAssetNeed` items from the plan.
5. The DAG branches by mode:
   - `html_only` -> HyperFrames shot/project generation.
   - `aigc_video` -> video prompt -> video provider or external request.
   - `aigc_image_then_hyperframes` -> image provider/external request -> HyperFrames media composition.
   - `hybrid_aigc_bg_html_overlay` -> video provider/external request + HyperFrames overlay -> compositor.
   - `external_or_user_asset` -> upload gate -> HyperFrames/assembler.
   - `placeholder_preview` -> generated placeholder media -> HyperFrames preview.
6. `shot_fusion_builder` turns approved assets into a shot-level composition or composited shot clip.
7. `final_assembly` concatenates or nests approved shot clips and mirrors the final MP4 into the client-fetchable local artifact namespace.

## Backend Changes

### Planner and Compiler

- Add `shot_generation_planner` as a first-class builtin prompt/tool.
- Insert it after `shot_splitter` and before `video_prompt_generator` in `completeVideoBetaPlan`.
- Pass `shotGenerationPlans` and `shotAssetPackages` into `video_prompt_generator` and `hyperframes_project_generator`.
- Allow downstream steps to depend on generation strategy when present.

### Service Layer

- Reuse `DecideRenderStrategy` as the deterministic core.
- Extend it from HTML/AIGC flags into concrete generation modes and asset needs.
- Keep provider capability awareness: when video provider is missing, create external generation requests instead of failing the whole flow.

### Local Tooling

- Extend `HyperFramesProjectExecutor` to read `shotAssetPackages`, generation plans, and registered local artifacts.
- Support timed `<video>` and `<img>` clips in generated HyperFrames HTML.
- Add a `shot_fusion_builder` or extend `hyperframes_project_generator` for hybrid mode:
  - base AIGC clip as muted video.
  - HyperFrames overlay for exact text and captions.
  - optional image/keyframe backgrounds with pan/zoom.
- Keep existing final video mirroring behavior for `final-video`.

### Artifacts

Add or normalize kinds:

- `SHOT_GENERATION_PLAN`
- `SHOT_ASSET_NEED`
- `SHOT_MEDIA_FUSION_PLAN`
- `HYPERFRAMES_SHOT`
- `COMPOSITED_SHOT_VIDEO`

Existing `EXTERNAL_GENERATION_REQUEST`, `SHOT_ASSET_PACKAGE`, `SHOT_KEYFRAME`, and `SHOT_VIDEO_CLIP` remain valid.

## Frontend Changes

- In the shot view, show a compact strategy label: `HyperFrames`, `AIGC`, `Hybrid`, `User Asset`, or `Preview`.
- Show why the strategy was selected.
- Group dependencies by shot and mode: needed, generated, uploaded, approved.
- For hybrid shots, show separate slots for base video/image and overlay/text.
- Keep raw JSON in debug views only.

## Error Handling

- Missing provider produces `external_generation_request` artifacts when the asset is required.
- Missing uploaded asset blocks only the affected shot, not the whole project.
- Failed AIGC generation can fall back to `aigc_image_then_hyperframes` or `placeholder_preview`.
- If a shot requires exact text but strategy is `aigc_video`, mark the plan invalid and reroute to hybrid or HTML.
- If a referenced media artifact is missing at render time, render a visible placeholder card that names the missing dependency.

## Testing

Backend tests:

- Exact text shot routes to `html_only`.
- Character/cinematic motion shot routes to `aigc_video`.
- Exact text plus cinematic motion routes to `hybrid_aigc_bg_html_overlay`.
- Logo/user upload shot routes to `external_or_user_asset`.
- Missing video provider creates external generation request instead of a hard failure.
- `completeVideoBetaPlan` inserts generation planning before prompt and preview/render steps.

Local tool tests:

- HyperFrames project includes timed media clips from `shotAssetPackages`.
- Hybrid shot renders base media plus overlay layers.
- Missing media renders a placeholder instead of producing broken HTML.
- Final render output is mirrored to `local://projects/<id>/artifacts/final-video/.../final.mp4`.

Frontend tests:

- Shot group shows strategy and reason.
- Hybrid shot shows separate base and overlay slots.
- Uploaded external result binds to the correct request and shot.

E2E smoke:

- Build a three-shot test video:
  - Shot 1: HyperFrames-only data/text card.
  - Shot 2: AIGC/video-like base media.
  - Shot 3: Hybrid base video or generated image plus exact text overlay.
- Use generated or synthetic local media if real providers are unavailable.
- Render final MP4 and verify browser client loads `final-video`.

## Acceptance Criteria

- Every shot has a visible generation strategy before media generation.
- The system can explain which parts are AIGC, which are HyperFrames, which require uploaded/external assets, and which are fused.
- Exact text is never delegated to AIGC-only rendering.
- Hybrid shots produce a composited output, not just separate prompt artifacts.
- Missing providers or assets pause the affected shot with a clear user action.
- The final video artifact is playable in the user client.
