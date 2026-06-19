# External Capability Requirements

Core only requires black-box capabilities. External teams/services must register manifests through `/api/tools/register`.

## Required External Tools

- `skill_stage_agent`: execute text/vision agent stage from `instruction_ref`, project context, artifact refs, and schema refs.
- `image_asset_generator`: create/register images and return artifact refs, prompt hash, dimensions, thumbnails.
- `hyperframes_project_builder`: build HyperFrames project bundle from approved reference and assets.
- `hyperframes_renderer`: render HyperFrames bundle to MP4 with logs and diagnostics.
- `video_shot_extractor`: split/prepare input video shots.
- `shot_motion_analyzer`: generate motion/composition/light analysis from sampled frames.
- `storyboard_assembler`: deterministically assemble approved frames into contact sheets.
- `asset_guard`: verify final package does not contain source frames or forbidden identity assets.
- `material_library_importer`: import abstract shooting grammar after approval.
- `material_library_matcher`: find reusable abstract shooting grammar candidates.
- `video_keyframe_prompt_builder`: produce keyframe prompts and final video prompts.
- `text_image_to_video_generator`: generate or register per-shot video clips.
- `video_final_assembler`: assemble final/rough cut from approved clips.
- `voice_post_process`: normalize, process, and merge narration audio.
- `audio_artifact_packager`: package final voice output and processing logs.

## Error Contract

Every external tool should return structured errors:

```json
{"code":"VALIDATION_ERROR|PROVIDER_ERROR|TIMEOUT|RETRYABLE|FATAL","message":"...","retryable":true}
```

## Trace Contract

Every external tool request receives `task_id` and `node_id` from Core and must echo them in logs/results.
