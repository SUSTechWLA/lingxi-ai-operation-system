# Video Agent Lightweight HKUDS-Inspired Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add lightweight video-domain intent, input resolution, plan judging, manual asset manifest, and beta eval support without duplicating existing runtime/orchestration capabilities.

**Architecture:** New logic lives in `cloud-backend/internal/agents/video/*` packages and one generic non-blocking Runner warning hook. The packages are deterministic Go code with artifact request builders, so existing handlers/workflows can persist outputs through the current artifact service. The eval runner imports the new packages and existing PlanGuard rather than creating a new planner or orchestrator.

**Tech Stack:** Go 1.25, existing `agentruntime.AgentPlan`, existing `artifact.CreateArtifactRequest`, standard library JSON/testing.

---

### Task 1: VideoIntent

Add tests and implementation for `cloud-backend/internal/agents/video/intent`.

Required behavior:

- Voice/knowledge/opinion/presentation requests infer `voice_visual`.
- Story/shot/scene/character requests infer `aigc_shot`.
- Platforms, duration, required artifacts, excluded capabilities, and reasoning are populated.
- Artifact request uses stage/unit `video_intent`.

### Task 2: MissingInputResolver

Add tests and implementation for `cloud-backend/internal/agents/video/inputresolver`.

Required behavior:

- Empty topic requires clarification.
- Platform defaults to `xiaohongshu`.
- Duration defaults to `90`.
- Language defaults to `zh-CN`.
- Only `voice_visual` and `aigc_shot` are accepted.

### Task 3: VideoPlanJudge

Add tests and implementation for `cloud-backend/internal/agents/video/planjudge`.

Required behavior:

- Missing `publish_copy_generator` warns.
- Beta-disabled tools warn.
- Render without preview dependency warns.
- Duplicate non-quality tools warn.
- Good beta plans pass with no warnings.

### Task 4: ManualAssetManifest

Add tests and implementation for `cloud-backend/internal/agents/video/assets`.

Required behavior:

- Manual image/audio/video imports produce deterministic `assetId`.
- Manifest contains type, storage type, storage ref, duration, description, tags, related shot, and source.
- Artifact request uses stage/unit `asset_manifest`.

### Task 5: Runner Warning Hook

Add tests and implementation in `internal/core/agentruntime`.

Required behavior:

- Runner calls the judge only after PlanGuard passes.
- Warnings are stored in `Run.Metadata["planJudgeWarnings"]`.
- Warnings are included in the orchestrator task input.
- Warnings never block DAG compilation or submission.

### Task 6: Video Beta Eval

Add `cloud-backend/evals/video_beta` with at least 10 cases.

Required behavior:

- `go test ./evals/video_beta -count=1` validates report semantics.
- `go run ./evals/video_beta` prints pass/fail report.
- Metrics include `intent_correct`, `plan_guard_passed`, `plan_judge_passed`, `required_artifacts_complete`, `forbidden_tool_absent`, and `publish_copy_complete`.

