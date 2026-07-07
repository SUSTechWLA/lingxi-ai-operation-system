# Changelog

All notable release changes are tracked here. README only carries the current version summary; detailed historical notes live in this file.

## Unreleased

### Added

- Added layered shot material packages with `aigcPlan`, `hyperframesPlan`, and `ffmpegFusionPlan` so AIGC generates text-free background or partial motion, HyperFrames renders exact Chinese text and keyframes, and FFmpeg fuses the layers into complete shots.
- Added Director Studio checks and UI coverage for progressive shot material review, editable prompts/references, prompt approval locks, and AIGC/HyperFrames/FFmpeg layer explanations.

### Changed

- Updated beta, QA, cinematic workflow, and project-introduction documentation to keep code, user-facing docs, and Wiki-style workflow notes aligned.

## v0.1.13 - 2026-07-05

### Changed

- Moved JiMeng CLI, MCP registration, and login-code setup from the project page into the Settings page.
- Settings now presents JiMeng CLI as part of image/video generation configuration, alongside OpenAI-compatible text/image/video providers.
- Project pages now show only a short JiMeng CLI explanation and a button that jumps to Settings, keeping the project workflow focused.

### Fixed

- Added a frontend regression check so the project page no longer owns JiMeng CLI setup actions.

## v0.1.12 - 2026-07-05

### Fixed

- Fixed the packaged desktop local runner restart path after login. The app now cleanly stops the unauthenticated local agent before starting the session-enabled runner, preventing `18080` bind conflicts and project-page "local executor not started" errors.
- Local agent now shuts down its HTTP listener on `SIGTERM`, so Electron restarts and manual operator restarts release the local runner port reliably.

### Changed

- Added beta runbook troubleshooting for stale or manually started local agents blocking the installed desktop app's bundled runner.

## v0.1.11 - 2026-07-04

### Added

- Added an explicit closed beta shot split policy: 3-15 second shots, preferred 6-8 second units, semantic script splitting, visual-change splitting, forced splitting for overlong script spans, and compatible merging for short continuous spans.
- Added shot candidate, candidate QA report, repair plan, accepted shot, final assembly plan, global subtitle timeline, global audio mix plan, final QA report, artifact provenance, and video diagnostics data structures.
- Added final assembly gating so FFmpeg concat consumes only accepted shot candidates and rejects failed, stale, unapproved, or out-of-range shots.
- Added diagnostics coverage for shot list, shot split report, duration validation, candidates, shot QA reports, repair plans, accepted shots, assembly plan, subtitle timeline, audio mix plan, final QA report, artifact manifest, and provenance summary.

### Changed

- Shot QA repair plans now preserve passed dimensions and patch only failed dimensions, with final subtitle, BGM, voiceover, ducking, loudness, and final transcode deferred to global final assembly.
- Video QA MCP repair plans now include severity, source candidate, attempt index, preserve flag, locked dimensions, repair targets, prompt patch, render strategy patch, and next tool call.
- Fallback beta fixture now writes the full closed beta shot-to-final diagnostic chain while preserving fallback provenance instead of marking fallback output as real AIGC.
- Director Studio now shows shot duration, QA status, attempt count, latest candidate, repair action, locked dimensions, accepted candidate, source/fallback status, assembly eligibility, final assembly status, final QA status, and provenance summary.

### Fixed

- Updated legacy cinematic time-window expectations to match the new 6-8 second preferred split policy instead of coarse fixed windows.
- Tightened final export behavior so failed final QA blocks export/publish.

## v0.1.10 - 2026-07-04

### Added

- Added the closed beta runbook for cloud backend, local agent, frontend/Electron, HyperFrames Render Service, FFmpeg, MCP providers, JiMeng/Dreamina, OpenAI-compatible model providers, logs, and diagnostics.
- Added `scripts/beta-smoke-check.sh` and the no-provider fallback fixture so beta operators can validate a 2-shot fallback preview, artifact provenance, shot QA reports, and machine-readable repair plans without real AIGC accounts.
- Added the beta readiness gate: `BETA_READINESS_REQUIRE_AIGC=1 bash scripts/beta-readiness-check.sh` must return `GO` before the environment is described as ready for one-sentence high-quality real AIGC creator trials.
- Added local `beta-diagnostics.zip` export with redacted environment data, logs, MCP provider status, artifact manifests, QA reports, and failure stack indexes.

### Changed

- Unified artifact provenance for real AIGC, HyperFrames, FFmpeg composite, uploaded assets, fallback storyboards, and fallback previews.
- Expanded Video QA MCP shot reports with structured decisions and `repairPlan` actions for AIGC regeneration, reference-based regeneration, HTML rerendering, FFmpeg recomposite, prompt revision, or human review.
- Frontend artifact views now clearly label fallback preview/storyboard outputs instead of presenting them as real JiMeng/Dreamina results.

### Security

- Release/production startup now rejects weak auth secrets, default database/MinIO/admin credentials, wildcard CORS, disabled sandbox, or sandbox fallback.

## v0.1.9 - 2026-07-04

### Changed

- Hardened the release build path with CI coverage for Go test/vet, frontend lint/build, Electron runtime tests, HyperFrames Render Service build, and runtime dependency audits.
- Cloud production config now rejects weak defaults and Docker images no longer embed `.env`.
- Local runner and Electron file access now fail closed for sandbox-required tools and user-granted paths.
- Removed unused helpers, isolated frontend utility files, redundant root `electron-builder`, and unused direct `@hyperframes/core`; added direct `esbuild` coverage where needed.

## v0.1.8 - 2026-07-04

### Added

- Added local preflight QA before every AIGC `kind=video` MCP call.
- Unclear prompts, internal production terms, and unusable references are blocked before JiMeng/Dreamina is called.
- `generationResults` and `assetProvenance` now include `preflightQa` details for repair and diagnosis.

## v0.1.7 - 2026-07-04

### Changed

- Changed Dreamina/JiMeng video prompts from internal engineering templates into Vibe Creator visual story descriptions.
- Prevented terms such as `AIGC_VIDEO`, `b-roll`, `ffmpeg`, `SHOT_VIDEO_CLIP`, and internal shot-material labels from leaking into external video prompts.
- Added regression coverage to keep provider prompts focused on visible timed story beats.

## v0.1.6 - 2026-07-04

### Added

- Added `sourceSummary` and `assetProvenance` to MCP AIGC generation results.
- Required at least one ready AIGC video asset before treating cinematic MCP generation as externally satisfied.
- Documented ready / failed / deferred / fallback states.

## v0.1.5 - 2026-07-04

### Added

- Added the `cinematic_story` workflow: story outline, detailed script, character/scene/prop dossiers, references, shot design, MCP generation, render, QA, and publish copy.
- Upgraded shot-level QA with script matching, director reasoning, reference coverage, action beats, visual complexity, text safe area, and repair decisions.

### Fixed

- Fixed cinematic workflow drift around missing references, empty MCP image parameters, Dreamina image resolution mapping, and render timeout handling.

## v0.1.4 - 2026-07-03

### Added

- Moved video frame QA into the standard Python MCP service `mcp/video_qa/server.py`.
- `VIDEO_FRAME_QA` now delegates through MCP and emits `SHOT_QA_REPORT` plus `SHOT_REPAIR_PLAN`.
- Added Python MCP self-tests, Go stdio MCP integration tests, and Video QA MCP E2E coverage.

## v0.1.3 - 2026-07-03

### Added

- Added shot-level visual QA summaries, metrics, scores, regeneration flags, conclusions, and recommendations.
- Added aggregate `repairPlan` actions: `approve`, `manual_review`, or `regenerate_shots`.

## v0.1.2 - 2026-07-03

### Added

- Added final-render frame QA with `video_frame_qa.json` and contact sheets.
- Inserted `visual_qa` automatically between render and publish.

### Fixed

- Fixed local artifact path propagation for downstream local tools.
- Reduced stale local runner failure reports.

## v0.1.1 - 2026-07-03

### Added

- Ran the cinematic / AIGC shot workflow from UI start, review gates, JiMeng MCP submission, local render, and final artifact.
- Added HyperFrames storyboard fallback rendering.

### Fixed

- Fixed successful runs being overwritten by stale failures.
- Fixed stale local runner jobs blocking new projects.
- Fixed pending-material banners after MCP-generated clip artifacts became available.

## v0.1.0 - 2026-07-03

### Added

- Standardized CLI integrations around MCP providers.
- Added `mcp/` as the home for MCP services.
- Migrated Dreamina/JiMeng CLI integration to `mcp/jimeng/server.py`.

### Changed

- Cloud orchestration now uses `mcp_generation_runner`; local execution uses standard MCP provider calls instead of provider-specific direct tools.
