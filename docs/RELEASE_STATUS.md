# Release Status

Current release: `v0.1.11`

Status: **initial closed beta launch-ready for controlled technical users; real creator trials require readiness `GO`.**

## Current Position

The `release` branch is the initial closed beta launch baseline for controlled technical users. The system can run the fallback smoke path without real AIGC provider accounts, and it can produce local preview artifacts, semantic shot split metadata, structured shot QA reports, machine-readable repair plans, accepted-shot assembly plans, provenance labels, and diagnostics packages.

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
