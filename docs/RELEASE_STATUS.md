# Release Status

Current release: `v0.1.13`

Status: **initial closed beta launch-ready for controlled technical users; real creator trials require readiness `GO`.**

## Current Position

The `release` branch is the initial closed beta launch baseline for controlled technical users. The system can run the fallback smoke path without real AIGC provider accounts, and it can produce local preview artifacts, semantic shot split metadata, structured shot QA reports, machine-readable repair plans, accepted-shot assembly plans, provenance labels, and diagnostics packages.

`v0.1.13` keeps the packaged desktop local runner restart fix and consolidates JiMeng CLI/MCP setup into Settings, alongside text-to-image and text-to-video provider configuration. Project pages now keep JiMeng guidance minimal and link to Settings instead of owning setup actions.

The release branch now also includes the shot workspace refresh: each shot opens as a linear 1-6 creator review flow, automatically switching between voice/knowledge video and cinematic/AIGC shot video. The workspace shows readable scripts, reference images, AIGC layer prompts, HyperFrames layer prompts, subtitle timelines, upload slots, and playable final media instead of exposing raw `local://` storage references or success-only artifact status messages.

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
- Video QA is deterministic frame metrics plus shot spec lint. OCR, ASR, PyIQA, and VLM judging are schema-ready but not bundled, so subtle story/identity failures may still need human review.
- The automated repair loop is policy-complete for closed beta, but actual provider-side regeneration quality still depends on JiMeng/Dreamina behavior and reference asset quality.
- Windows packaging is not a primary closed beta path.
- `CONDITIONAL` readiness is not enough for a high-quality real AIGC promise.

## Release Documentation

- Current setup: [Closed Beta Runbook](BETA_RUNBOOK.md)
- Historical changes: [Changelog](../CHANGELOG.md)
- Version rules: [Version Management](version-management.md)
