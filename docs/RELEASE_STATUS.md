# Release Status

Current release: `v0.2.1`

Status: **initial closed beta launch-ready for controlled technical users; real creator trials require readiness `GO`.**

## Current Position

The `release` branch is the initial closed beta launch baseline for controlled technical users. The system can run the fallback smoke path without real AIGC provider accounts, and it can produce local preview artifacts, semantic shot split metadata, structured shot QA reports, machine-readable repair plans, accepted-shot assembly plans, provenance labels, and diagnostics packages.

`v0.2.1` adds a tested one-command source deployment, packaged canonical IP assets and MCP provider, the canonical three-layer Shot contract, an independent AIGC execution policy, dynamic Agent review gates, strict preview-duration handling, and fail-closed required local media generation.

The release branch now also includes the Shot workspace refresh: each Shot opens as a linear 1-6 creator review flow, automatically switching between voice/knowledge video and cinematic/AIGC Shot video. Every Shot visibly distinguishes IP A-roll, HyperFrames/HyperKeyframes text and effects, AIGC enrichment, and the final composition plan, including whether a layer executes now or remains designed for later. The workspace shows readable scripts, reference images, prompts, subtitle timelines, upload slots, and playable final media instead of exposing raw `local://` storage references or success-only artifact status messages.

The production Creator shell exposes one settings page from the user-avatar menu. It keeps text, image, and video generation providers separate, persists their credentials only in the local agent, and supports persisted system, light, and dark appearances. A generation request transmits only the runtime credentials required for that authenticated cloud orchestration request; project configuration and cloud persistence are sanitized. The settings page can explicitly clear each locally saved key. The start-creation page uses creator-facing labels only; the canonical three-layer Shot design continues in backend orchestration without exposing implementation terminology at project intake.

The release candidate also includes the canonical sloth A-roll path. It loads the approved Blender master into the warm studio, drives body, wrist, independent three-segment digits, visemes and facial controls from the script timeline, uses the pinned GPT-SoVITS voice, and composes a final MP4 locally with FFmpeg. This layer is deterministic local rendering; the same talking-head Shot may independently add AIGC background/B-roll enrichment when its policy and provider allow it.

Desktop installers are built from the matching `v0.2.1` tag. macOS artifacts use English filenames containing the version and architecture; unsigned closed-beta builds may still require the tester to approve the app in macOS Privacy & Security.

The settings and appearance work above remains `Unreleased` until a new semantic version tag is created. The currently downloadable desktop installers are still the `v0.2.1` artifacts.

For creator-facing real AIGC trials, run the readiness gate in the target environment and require `GO` before inviting users.

Do not describe the environment as "one sentence creates high-quality real AIGC video" unless the readiness gate confirms all live dependencies.

## Creator Trial Gate

Before inviting real creators:

```bash
BETA_READINESS_REQUIRE_AIGC=1 bash scripts/beta-readiness-check.sh
```

Required result:

```text
decision: GO
```

`CONDITIONAL` means the fallback preview and engineering loop are verifiable, but real AIGC readiness is incomplete.

`BLOCKED` means at least one core gate is missing and users should not be invited.

## Required Gates

- `bash scripts/beta-smoke-check.sh` passes.
- Local agent is running and can export `beta-diagnostics.zip`.
- HyperFrames Render Service is healthy.
- FFmpeg is available.
- Video QA MCP produces structured shot reports.
- Each shot QA report includes a machine-readable `repairPlan`.
- Shot split policy is present and enforces `minShotDurationSec=3`, `maxShotDurationSec=15`, `preferredShotDurationSec=6-8`, `splitByScriptSemantics=true`, and `splitByVisualChange=true`.
- Shot candidates are versioned by `attemptIndex`; failed candidates are retained for diagnostics and must not overwrite accepted candidates.
- Final assembly consumes only accepted candidate artifacts, normalizes clips with FFmpeg, then handles global voiceover, BGM ducking, subtitle timeline, loudness, final transcode, and final QA.
- Shot material packages expose `aigcPlan`, `hyperframesPlan`, and `ffmpegFusionPlan` so users can generate text-free AIGC backgrounds or partial videos, keep exact Chinese text in HyperFrames, and merge layers with FFmpeg.
- Shot pages let users inspect the actual generated media: reference/storyboard images can be enlarged, video artifacts can be played in a large modal, subtitles are displayed as a readable timeline, and only actionable status messages are shown.
- External AIGC video calls use the AIGC layer prompt (`aigcPrompt`, `aigcVideoPrompt`, or `aigcPlan.prompt`) before falling back to legacy prompt fields, so JiMeng/Dreamina is not sent HyperFrames text-layer copy.
- Local IP A-roll rendering can generate a 1080p CFR 30fps video, motion and viseme plans, rig/render reports, and media QA evidence from the canonical sloth master without AIGC video calls.
- At least one AIGC video MCP provider is healthy when real creator trials are planned.
- OpenAI-compatible model provider routes are configured for the selected workflow.
- Artifact provenance distinguishes real AIGC from fallback storyboard/preview.

## Real AIGC vs Fallback

Treat an artifact as real AIGC only when:

- `sourceType` is `aigc_video` or `aigc_image`.
- `isFallback` is `false`.
- `providerName` is set.
- `providerJobId` or a ready `storageRef` exists.

Treat an artifact as fallback when:

- `sourceType` is `fallback_storyboard` or `fallback_preview`.
- `isFallback` is `true`.
- `fallbackReason` is non-empty.
- `sourceSummary.readyVideoCount=0` while video requests exist.

## Current Known Limits

- Real AIGC generation depends on local provider login, credits, rate limits, and provider CLI/UI stability.
- Local IP avatar preview narration can fall back to segmented macOS `say` with varied rate, pauses, and emphasis. It is better for timing and lip-sync preview but is still not production-quality voice acting.
- The current local IP animation mode is `svg2d` high-fidelity puppet. Full 1:1 body articulation requires a future Live2D Cubism model package and renderer bridge.
- Video QA is deterministic frame metrics plus shot spec lint. OCR, ASR, PyIQA, and VLM judging are schema-ready but not bundled, so subtle story/identity failures may still need human review.
- The automated repair loop is policy-complete for closed beta, but actual provider-side regeneration quality still depends on JiMeng/Dreamina behavior and reference asset quality.
- Windows packaging is not a primary closed beta path.
- `CONDITIONAL` readiness is not enough for a high-quality real AIGC promise.

## Release Documentation

- Current setup: [Closed Beta Runbook](BETA_RUNBOOK.md)
- Historical changes: [Changelog](../CHANGELOG.md)
- Version rules: [Version Management](version-management.md)
- Local IP talking avatar: [Local IP Talking Avatar Render](local-ip-talking-avatar-render.md)
