# Video Type Dual Pipeline Design

## Context

The current video agent has two important pieces already in place:

- `skillruntime/router.go` can route user briefs into `talking_head`, `cinematic_short`, `director_pipeline`, and `shot_learning`.
- The video beta runtime can complete a generic chain from script generation to shot splitting, shot generation planning, prompt generation, HyperFrames preview, and final render.

The missing layer is execution-grade profile selection. The route selected at the entrance is not yet used as the first-class driver for task ordering, review gates, artifact contracts, and quality criteria. As a result, talking-head videos and cinematic story videos can pass through a blended chain that is workable for smoke tests but not strict enough for production creation.

This document defines the first two production mainlines:

1. Talking-head / explainer videos.
2. Cinematic / story videos.

`shot_generation_planner` remains part of both flows, but it is not the top-level routing mechanism. It decides how each shot is visually generated after the system has already selected the correct creation profile.

## Problem

Talking-head and cinematic videos have different centers of gravity.

Talking-head videos are script-first. The video exists to make the spoken argument, explanation, or educational content clearer. Visuals, subtitles, cards, B-roll, and background motion must stay aligned with the spoken script. Continuity requirements are usually light: no persistent characters, no dramatic scene blocking, no actor voice continuity, and no plot continuity across scenes.

Cinematic videos are scene-first and continuity-first. The script, characters, settings, props, performance, camera language, soundscape, and editing all need to express a story. The system must preserve character identity, costume, scene geography, major props, lighting logic, emotional progression, and time-window continuity. A generic script-to-shot-to-render chain is too weak for this.

## Goals

- Introduce a `VideoCreationProfile` decision as a first-class artifact and runtime input.
- Use separate DAG templates for talking-head and cinematic videos.
- Preserve the existing dynamic agent path as a fallback, but bias completion by selected profile.
- Keep reviewable intermediate artifacts for both profiles.
- Make tool order and tool choice profile-specific.
- Keep `shot_generation_planner` as a per-shot visual generation planner, not as the global workflow selector.
- Let the frontend display profile-specific review surfaces.
- Keep the first implementation small enough to ship behind current `develop_go` flow.

## Non-Goals

- Do not rewrite the entire dynamic planner.
- Do not implement every stage of the full Markdown-first `video-creator` skill in one pass.
- Do not require configured external AIGC providers before talking-head videos can preview.
- Do not build a full non-linear editor.
- Do not solve final professional sound mix in the first implementation.
- Do not remove the current generic video beta behavior until both profile templates are tested.

## Creation Profiles

### Shared `VideoCreationProfile`

Each video project should carry a profile artifact generated immediately after proposal or route selection.

Fields:

- `profileId`: `talking_head` or `cinematic_story`.
- `sourceRoute`: route from skill router or planner.
- `primaryArtifact`: the artifact that drives downstream correctness.
- `qualityContract`: short list of checks that define success.
- `dagTemplateId`: template used by the plan compiler.
- `reviewGatePolicy`: required review gates and default focus items.
- `toolBias`: preferred and disallowed tools by stage.
- `fallbackProfile`: profile to use when confidence is low.
- `confidence` and `reason`.

The artifact kind should be `VIDEO_CREATION_PROFILE`. It should be stored and surfaced like other reviewable planning artifacts.

### Talking-Head Profile

`profileId`: `talking_head`

Primary artifact: approved spoken script.

Quality contract:

- Spoken script is complete, natural, and matches target duration.
- Every visual segment maps to a script sentence or paragraph.
- Captions cover the script without changing meaning.
- Visuals clarify the script and do not introduce contradictory information.
- Exact text, charts, subtitles, title cards, and data labels are owned by HyperFrames.
- AIGC media is optional and only used when it improves explanation or provides missing B-roll.

Default visual generation bias:

- Prefer `html_only`, `aigc_image_then_hyperframes`, or `external_or_user_asset`.
- Use `hybrid_aigc_bg_html_overlay` when a richer background is useful but exact text is required.
- Use `aigc_video` sparingly, mainly for abstract B-roll or concept illustration.

Audio expectation:

- One primary voiceover track.
- Simple background music optional.
- No role-based dialogue continuity in MVP.

### Cinematic Profile

`profileId`: `cinematic_story`

Primary artifact: approved script plus continuity bible.

Quality contract:

- Characters, costumes, major props, and main scenes stay consistent.
- Every shot has director reasoning: composition, lighting, movement, action, emotion, and story function.
- Every continuous time window has a locked first frame or storyboard reference.
- Scene geography and prop state do not drift without an explicit story reason.
- Prompt generation is self-contained and follows cinematic continuity constraints.
- Sound design includes dialogue, environment sound, action sound, or intentional silence per shot.

Default visual generation bias:

- Prefer AIGC image/video generation for core story visuals.
- Require character, scene, and prop references before high-risk generation.
- Use HyperFrames for titles, subtitles, overlays, UI layers, credits, and deterministic text, not as the primary story image source.
- Use `shot_generation_planner` after continuity assets exist, so it can reason from locked references.

Audio expectation:

- Dialogue, environment sound, action sound, and silence are planned per shot.
- Background music is part of director cut planning, not the only sound layer.

## DAG Templates

### Template A: Talking-Head / Explainer

Recommended stage order:

1. `profile_selection`
   - Tool: `video_profile_classifier` or deterministic route bridge.
   - Output: `VIDEO_CREATION_PROFILE`.
2. `proposal`
   - Tools: `pipeline_selector`, `proposal_generator`, optional `knowledge_researcher`.
   - Output: `VIDEO_PROPOSAL`.
3. `script`
   - Tools: `video_script_generator`, `script_quality_checker`, optional `fact_checker`.
   - Output: `VIDEO_SCRIPT`.
   - Human review: script meaning, duration, tone, factual accuracy.
4. `script_segmentation`
   - Tools: `caption_splitter`, `shot_splitter`.
   - Output: `SCRIPT_SEGMENT_PLAN`, `SHOT_LIST`.
   - Contract: every segment references script spans.
5. `visual_alignment`
   - Tools: `visual_alignment_planner`, `asset_decision_agent`.
   - Output: `VISUAL_ALIGNMENT_PLAN`, optional `REFERENCE_ASSET_PLAN`.
   - Contract: every visual element must explain or support a script span.
6. `shot_generation_strategy`
   - Tool: `shot_generation_planner`.
   - Output: `SHOT_GENERATION_PLAN`, `SHOT_ASSET_PACKAGE`.
   - Profile bias: prefer HyperFrames and asset fusion over pure AIGC video.
7. `composition`
   - Tools: `video_composition_builder`, `composition_quality_checker`.
   - Output: `VIDEO_COMPOSITION_SPEC`.
   - Contract: timeline, captions, cards, media, and script spans align.
8. `preview`
   - Tools: `hyperframes_project_generator`, `hyperframes_snapshot`, `preview_quality_checker`.
   - Output: `HYPERFRAMES_PROJECT`, `PREVIEW_SNAPSHOTS`, `PREVIEW_REPORT`.
   - Human review: readability, content alignment, caption coverage.
9. `render`
   - Tools: `render_dependency_guard`, `hyperframes_renderer`.
   - Output: final `VIDEO`.
10. `publish`
   - Tools: `publish_copy_generator`, `artifact_packager`.
   - Output: publish pack based on approved script and final video.

### Template B: Cinematic / Story

Recommended stage order:

1. `profile_selection`
   - Tool: `video_profile_classifier` or deterministic route bridge.
   - Output: `VIDEO_CREATION_PROFILE`.
2. `story_foundation`
   - Tools: `story_foundation_generator`, optional `proposal_generator`.
   - Output: `STORY_BRIEF`, `IP_OR_STORY_DIRECTION`.
   - Human review: story premise, tone, target length.
3. `script`
   - Tools: `cinematic_script_generator`, `script_quality_checker`.
   - Output: `CINEMATIC_SCRIPT`.
   - Human review: plot clarity, scene structure, dialogue or narration.
4. `continuity_bible`
   - Tools: `character_profile_builder`, `scene_profile_builder`, `prop_profile_builder`, `continuity_checker`.
   - Output: `CHARACTER_PROFILES`, `SCENE_PROFILES`, `PROP_PROFILES`, `CONTINUITY_BIBLE`.
   - Human review: character, scene, and prop locks.
5. `reference_assets`
   - Tools: `reference_asset_planner`, `image_asset_generator`, optional material library match.
   - Output: `REFERENCE_ASSET_PLAN`, `CHARACTER_REFERENCE_ASSETS`, `SCENE_REFERENCE_ASSETS`, `PROP_REFERENCE_ASSETS`.
   - Human review: reference quality and consistency.
6. `shot_design`
   - Tools: `cinematic_shot_designer`, `shot_splitter`.
   - Output: `SHOT_LIST`, `DIRECTOR_DESIGN`.
   - Contract: each shot has scene, characters, props, time window, action, emotion, and director reason.
7. `keyframes_storyboards`
   - Tools: `keyframe_prompt_generator`, `storyboard_prompt_generator`, `image_asset_generator`.
   - Output: `KEYFRAME_PROMPTS`, `STORYBOARD_PROMPTS`, keyframe/storyboard artifacts.
   - Human review: frame continuity and scene readability.
8. `shot_generation_strategy`
   - Tool: `shot_generation_planner`.
   - Output: `SHOT_GENERATION_PLAN`, `SHOT_ASSET_PACKAGE`, external generation requests.
   - Profile bias: use AIGC video/image for story visuals; preserve deterministic overlays in HyperFrames.
9. `video_generation`
   - Tools: external AIGC video request tools, upload binding, local artifact packaging.
   - Output: `SHOT_VIDEO_CLIP` or `COMPOSITED_SHOT_VIDEO`.
   - Human review: per-shot continuity and performance.
10. `sound_design`
    - Tools: `sound_design_planner`; audio generation can be optional in MVP.
    - Output: `SOUND_DESIGN_PLAN`.
    - Contract: environment sound, dialogue, action sound, silence, and BGM intent per shot.
11. `director_cut`
    - Tools: `director_cut_planner`, `preview_quality_checker`.
    - Output: `DIRECTOR_CUT_PLAN`, `PREVIEW_REPORT`.
    - Human review: shot order, pacing, emotional arc, sound plan.
12. `assembly_render`
    - Tools: `hyperframes_project_generator`, `hyperframes_renderer`, optional ffmpeg assembler.
    - Output: final `VIDEO`.

## Plan Compiler Changes

The first implementation should add a profile-aware branch before `completeVideoBetaPlan` appends missing video stages.

Recommended components:

- `VideoCreationProfile` model in the video creation model layer.
- `BuildVideoCreationProfile` service function with deterministic fallback rules.
- `video_profile_classifier` builtin tool manifest and executor.
- `PlanCompiler.completeVideoPlanByProfile(plan, profile)` dispatcher.
- `completeTalkingHeadPlan`.
- `completeCinematicStoryPlan`.

Fallback rules:

- Route `talking_head` maps to talking-head template.
- Routes `cinematic_short` and `director_pipeline` map to cinematic template unless deliverable is `publish_pack` and the brief is clearly口播.
- If no route is available, default to current behavior with talking-head bias.
- If a required cinematic tool is not registered, emit reviewable missing-capability artifacts instead of silently downgrading into the talking-head path.

## Tool Registry Changes

MVP can use existing tools where possible and introduce aliases or lightweight wrappers where full implementations do not yet exist. The important first boundary is the manifest, artifact contract, and DAG placement. Cinematic tools may start as deterministic dry-run executors if they emit honest reviewable documents and missing-capability notes.

Required manifests/contracts for MVP:

- `video_profile_classifier`
- `visual_alignment_planner`
- `cinematic_shot_designer`
- `sound_design_planner`

Existing tools to reuse:

- `video_script_generator`
- `script_quality_checker`
- `caption_splitter`
- `shot_splitter`
- `asset_decision_agent`
- `reference_asset_planner`
- `image_asset_generator`
- `keyframe_prompt_generator`
- `video_prompt_generator`
- `shot_generation_planner`
- `video_composition_builder`
- `hyperframes_project_generator`
- `hyperframes_renderer`
- `publish_copy_generator`
- `artifact_packager`

If a new cinematic tool is not yet fully implemented, the executor should return structured placeholder artifacts that clearly say which reviewable document is missing and what the next implementation task is. It should not pretend cinematic continuity was checked.

## Frontend Review Changes

Director Studio should show the selected profile at project and run level.

Talking-head view should emphasize:

- Script approval.
- Script segment coverage.
- Visual-to-script alignment.
- Caption coverage.
- Card/media readability.
- Publish copy derived from script.

Cinematic view should emphasize:

- Story/script approval.
- Character, scene, and prop profiles.
- Continuity bible.
- Reference assets.
- Shot director design.
- Keyframes and storyboards.
- Per-shot generation plan.
- Sound design and director cut plan.

Existing shot slots (`prompt`, `reference`, `storyboard`, `base-media`, `overlay`, `video`) should remain, but their priority and labels can be profile-aware.

## Error Handling

- Low profile confidence: create `VIDEO_CREATION_PROFILE` with `needsUserReview=true` and offer the two profile choices.
- Missing talking-head dependencies: keep producing HyperFrames placeholder preview if script and captions exist.
- Missing cinematic dependencies: block AIGC shot generation until required continuity assets or user-approved fallback are present.
- External provider unavailable: produce reviewable external generation requests and placeholder preview, not a fake final.
- Mixed brief: choose the primary profile, then allow profile-specific sections. Example: a documentary essay with cinematic intro should use talking-head as the main profile with cinematic shots only for selected segments.

## Testing Strategy

Unit tests:

- Profile classifier maps clear口播 briefs to `talking_head`.
- Profile classifier maps剧情、角色、场景、导演级、短片 briefs to `cinematic_story`.
- Plan compiler inserts talking-head stages in the expected order.
- Plan compiler inserts cinematic stages in the expected order.
- Existing generic video beta plan tests continue to pass.
- Missing optional cinematic tools produce missing-capability artifacts instead of panics or silent downgrade.

Integration tests:

- Talking-head E2E smoke: script -> segment plan -> visual alignment -> shot generation -> HyperFrames preview -> final video.
- Cinematic dry-run smoke: script -> continuity bible -> reference plan -> shot design -> keyframe/storyboard requests -> shot generation plan, without requiring external provider execution.
- Frontend logic test surfaces profile-specific review groups and artifact labels.

Manual browser test:

- Talking-head preview remains playable in the client.
- Cinematic dry-run shows required continuity and reference review gates before video generation.

## Rollout Plan

1. Add model and service contract for `VideoCreationProfile`.
2. Add builtin `video_profile_classifier`.
3. Add profile-aware plan compiler branch while keeping current behavior as fallback.
4. Implement talking-head template first because it is closest to current production path.
5. Implement cinematic dry-run template with reviewable placeholder artifacts for unimplemented provider stages.
6. Update frontend review grouping for profile-specific artifacts.
7. Add E2E smoke tests for both mainlines.

## Open Decisions Resolved for MVP

- The first two production profiles are `talking_head` and `cinematic_story`.
- `cinematic_short` and `director_pipeline` entrance routes both map into `cinematic_story` for DAG purposes.
- `shot_generation_planner` is shared by both profiles but receives profile-specific bias and upstream context.
- Talking-head can render with deterministic HyperFrames even when AIGC providers are unavailable.
- Cinematic generation should not silently degrade into a talking-head card video when continuity assets are missing.
