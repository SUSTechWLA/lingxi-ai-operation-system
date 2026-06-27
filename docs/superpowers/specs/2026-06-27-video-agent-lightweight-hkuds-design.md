# Video Agent Lightweight HKUDS-Inspired Design

## Goal

Add a small video-domain reasoning layer inspired by HKUDS/VideoAgent without reimplementing the existing Dynamic Agent Runtime, HybridToolRetriever, Director/Stage Guard, Quality Gate, Artifact Review, video models, Local Runner, or HyperFrames path.

## Scope

Included:

- `internal/agents/video/intent`: deterministic VideoIntent extraction for `voice_visual` and `aigc_shot`.
- `internal/agents/video/inputresolver`: defaulting and clarification decisions for topic, video type, platform, duration, and language.
- `internal/agents/video/planjudge`: semantic lint warnings after PlanGuard succeeds.
- `internal/agents/video/assets`: manual import asset manifest generation.
- `evals/video_beta`: at least 10 beta cases and a `go run ./evals/video_beta` report.
- A small Runner extension point that records PlanJudge warnings in run metadata and task input without blocking execution.

Excluded:

- No bid or generic chat restoration.
- No CosyVoice, DiffSinger, ImageBind, fish-speech, seed-vc, VideoRAG, automatic publishing, automatic operation review, video QA, or long-video understanding.
- No replacement of PlanGuard, PlanCompiler, Orchestrator, Director, Quality Gate, Artifact Review, Local Runner, or HyperFrames.
- No Python tools inside the Go backend.

## Design

The new packages are pure Go business-layer helpers. They are deterministic and testable, with no model calls or heavy media dependencies.

`VideoIntent` converts raw user demand into a small schema with `videoType`, `topic`, `platforms`, `durationSec`, `requiredArtifacts`, `excludedCapabilities`, and `reasoning`. It emits an artifact create request for `video_intent` so existing artifact storage can persist the output when a handler or workflow has a project scope.

`MissingInputResolver` handles beta defaults and clarification gates. Topic is the only required clarification in v1. Platform defaults to `xiaohongshu`, duration to `90`, and language to `zh-CN`.

`VideoPlanJudge` reads an `agentruntime.AgentPlan` after PlanGuard has already passed. It does not reject plans. It emits warnings for missing stages, redundant tools, beta-disabled capabilities, render steps without preview dependencies, and missing publish copy.

`ManualAssetManifest` indexes manually imported media. It deliberately does not perform automatic image/video understanding and only captures metadata supplied by the caller.

`video_beta` evals exercise the lightweight layer and produce a pass/fail report for intent correctness, PlanGuard pass, PlanJudge pass, required artifacts, forbidden tool absence, and publish copy completeness.

