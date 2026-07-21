# Changelog

All notable release changes are tracked here. README only carries the current version summary; detailed historical notes live in this file.

## Unreleased

### Added

- Added a production Creator settings route, accessible from the user-avatar menu, for separate local text, image, and video generation providers.
- Added persisted system, light, and dark appearance modes backed by a shared semantic color system.

### Changed

- Removed internal three-layer Shot terminology from the start-creation page while preserving the canonical backend layer contract and orchestration.
- Reused the same settings implementation in the production Creator shell and the gated developer console.
- Clarified that provider keys are persisted only by the local agent, transmitted only for the authenticated generation request that needs them, and never written into project configuration or cloud persistence.
- Added explicit removal for locally saved provider keys and accessible keyboard navigation for settings tabs.

## v0.2.1 - 2026-07-21

### Added

- Added `scripts/one-click-deploy.sh` to install locked dependencies, start and health-check the Docker backend and HyperFrames, package the desktop client, and optionally open the installed app.
- Bundled the canonical sloth/studio assets and `ip_avatar_3d` MCP provider in the macOS application.
- Added creator-facing dynamic Agent review gates and a verified local-only talking-head execution option.
- Added the canonical `shot_visual_layers_v1` contract: every Shot separately describes IP A-roll, HyperFrames/HyperKeyframes text and effects, AIGC enrichment, and composition.
- Persisted `shotGenerationPlans`, readable three-layer summaries, execution policy, and required layers into every Shot in the generated HyperFrames project so the editor and renderer share the same contract.
- Added the local `storyboard_ip_composite` fallback: it preserves the requested canvas, uses continuous IP A-roll as the picture and audio base, and overlays deterministic HyperFrames text instead of replacing the character with a static storyboard.

### Changed

- Local IP preview renders now honor the requested duration and use a practical 1280x720 15fps render profile; publish rendering remains independent.
- Talking-head and cinematic routes now use an independent AIGC execution policy. Talking-head defaults to automatic per-Shot enrichment; local-only mode keeps the AIGC design but creates no executable AIGC requests.
- The Shot review UI now exposes all three visual layers, their execution state, safety constraints, prompts, and composition order.
- Updated root, frontend, and HyperFrames Render Service versions to `0.2.1` and expanded CI coverage for one-click deployment and creator contracts.

### Fixed

- Fixed packaged applications failing to discover macOS speech and Homebrew FFmpeg/FFprobe tools.
- Fixed dynamic planning dropping `ipRenderMode`, AIGC policy, route, required-layer, and three-layer Shot context.
- Fixed dynamically selected `ip_aroll_director` steps running before narration existed; the compiler now orders them after the canonical script and injects the script output.
- Fixed manually retried local nodes remaining blocked by a terminal parent run, completed Agent runs not closing their video projects, local-only videos not loading in the packaged player, and completed projects displaying `0/6` progress.
- Fixed `aigcProvider=disabled` still producing executable external-generation requests during prompt planning.
- Fixed required local IP renders being marked successful when their MCP output did not satisfy the required video asset count.
- Fixed Docker images missing video pipeline manifests, idempotency key truncation, bigint CAS binding, and duplicate plan steps.

## v0.2.0 - 2026-07-21

### Added

- Added the default six-step creator studio with two persistent creator destinations, isolated Shot review, and a separately gated developer console.
- Added a durable, idempotent assembly retry that snapshots only accepted current Shot candidates and restarts the real preview review gate without regenerating any Shot.
- Added fail-closed delivery UI: final media and download controls require an explicit passed server-side final-review result on the current delivery artifact.
- Added layered shot material packages with `aigcPlan`, `hyperframesPlan`, and `ffmpegFusionPlan` so AIGC generates text-free background or partial motion, HyperFrames renders exact Chinese text and keyframes, and FFmpeg fuses the layers into complete shots.
- Added Director Studio checks and UI coverage for progressive shot material review, editable prompts/references, prompt approval locks, and AIGC/HyperFrames/FFmpeg layer explanations.
- Added a creator-facing shot workspace that auto-selects the voice / knowledge workflow or cinematic / AIGC shot workflow, then presents each shot as a linear 1-6 review flow instead of a two-column technical artifact index.
- Added image and video enlargement dialogs in the shot artifact preview, including large video playback for AIGC clips, HyperFrames overlays, and final complete shot artifacts.
- Added image regeneration instructions for selected reference/storyboard frames so users can request local AI edits while preserving locked role, scene, prop, and continuity constraints.
- Added `LocalIpTalkingAvatarRenderTool`, a deterministic local IP talking-avatar renderer for oral-video A-roll. It supports `bobo` / `aster` character assets, `svg2d` high-fidelity reference-image puppets, audio-driven lip-sync, motion timelines, HyperGen control metadata, voice profiles, FFmpeg composition, QA reports, and local demo generation.
- Added segmented prosody preview audio and richer limb motion for local IP talking avatars, including varied local preview speech rate/pauses, `narration_prosody_plan.json`, left/right arm gestures, both-hands presentation, foot bounce, and body weight shift.
- Added the canonical 3D sloth A-roll asset pair: one Blender character master, one shared warm-studio scene, one runtime GLB compatibility export, one material system, and one versioned asset manifest.
- Added tag-driven macOS arm64 and x64 desktop packaging with English artifact names and automatic GitHub Release publication.

### Changed

- Updated beta, QA, cinematic workflow, and project-introduction documentation to keep code, user-facing docs, and Wiki-style workflow notes aligned.
- Changed shot artifact cards to show playable/readable media, subtitle timelines, prompt text, and upload slots directly, while hiding success-only storage messages such as "preview ready", "artifact registered", and normal `valid` badges.
- Changed external video generation calls so JiMeng/Dreamina receives the AIGC layer prompt first (`aigcPrompt` / `aigcVideoPrompt` / `aigcPlan.prompt`) instead of HyperFrames text-layer or full-shot engineering descriptions.
- Changed local artifact preview loading so web dev uses the same-origin `/api/local` proxy, avoiding `Failed to fetch` for local media previews when the desktop/web app is served from `127.0.0.1:3000`.
- Changed backend project and artifact reads to coalesce nullable legacy fields, reducing scan failures for older project/artifact rows during shot workspace loading.
- Consolidated all default IP assets under the English-only `ip-assets/main-ip/` path and removed obsolete preview renders, duplicate character files, external studio textures, demo audio strips, and stale generated-video assets.
- Unified root, frontend, and HyperFrames Render Service versions at `0.2.0`, with CI checks that reject version, README, release-status, or tag drift.

### Fixed

- Fixed the GitHub repository Releases panel being stuck at `v0.1.9` by restoring a real GitHub Release publishing path instead of creating tags without release records.
- Fixed desktop package filenames so downloadable artifacts use stable English names containing version, platform, and CPU architecture.

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
