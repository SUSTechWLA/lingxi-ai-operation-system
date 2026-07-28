# Canonical Shot Timeline and Review Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Preserve one canonical narration interval through every Shot artifact, generate coordinated time-aware IP/HyperFrames/AIGC designs, and replace duplicate inline expansion with a stable detail drawer.

**Architecture:** `AudioMasterTimeline` and `TimeWindowPlan` remain the canonical clock. Downstream plans merge by `shotId` and may enrich but never replace canonical narration or timing. The creator UI projects canonical historical records before derived packages and opens one detail value in an accessible overlay rather than rendering the value twice.

**Tech Stack:** Go 1.24 domain/services and builtin tools, TypeScript 5, React 18, Vite, CSS, Node contract tests.

## Global Constraints

- Do not rewrite archived project artifacts in place.
- Use millisecond timing as the canonical representation; legacy seconds are adapters only.
- A normalized concatenation of Shot narration must equal the approved script/audio-master cues.
- AIGC creative prompts describe feeling, environment, light, motion, and timed visual evolution; renderer mechanics remain separate.
- Every talking-head Shot describes IP A-roll, HyperFrames/HyperKeyframes, AIGC enrichment, and their composition even when an optional layer is not executed.
- The historical review must never show the same full value both before and after a disclosure action.

---

### Task 1: Validate the Canonical Shot Timeline

**Files:**
- Modify: `cloud-backend/internal/agents/video/service/time_window_plan.go`
- Modify: `cloud-backend/internal/agents/video/service/time_window_plan_test.go`
- Modify: `cloud-backend/internal/agents/video/service/creation_service.go`
- Modify: `cloud-backend/internal/agents/video/service/creation_service_test.go`

**Interfaces:**
- Produces: `ValidateCanonicalTimeWindowPlan(master model.AudioMasterTimeline, plan model.TimeWindowPlan) []ValidationIssue`
- Produces: `NormalizeNarrationForComparison(text string) string`
- Consumes: `AudioMasterTimeline.Sentences`, `TimeWindowPlan.Windows`

- [ ] **Step 1: Write failing service tests**

Add literal fixtures proving that ordered windows pass, a gap/overlap fails, repeated full narration fails coverage, and concatenated window text equals the cue text. Add a CreationService regression proving placeholder Shot generation no longer assigns `spec.Topic` to every Shot.

- [ ] **Step 2: Run the focused tests and verify RED**

Run:

```bash
go test ./internal/agents/video/service -run 'TestValidateCanonicalTimeWindowPlan|TestCreationServiceGenerateShotsDoesNotDuplicateTopicNarration' -count=1
```

Expected: FAIL because the validator does not exist and generated Shots still contain the whole topic.

- [ ] **Step 3: Implement the validator and remove the unsafe narration default**

The validator must check timeline revision, interval ordering, contiguity, bounds, cue coverage, and normalized narration equality. `CreationService.GenerateShots` must leave narration empty until a canonical script window is assigned.

- [ ] **Step 4: Run focused and package tests**

```bash
go test ./internal/agents/video/service -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cloud-backend/internal/agents/video/service/time_window_plan.go cloud-backend/internal/agents/video/service/time_window_plan_test.go cloud-backend/internal/agents/video/service/creation_service.go cloud-backend/internal/agents/video/service/creation_service_test.go
git commit -m "fix(video): enforce canonical shot narration timeline"
```

### Task 2: Preserve Canonical Context Through Generation Planning

**Files:**
- Modify: `cloud-backend/internal/agents/video/model/creation.go`
- Modify: `cloud-backend/internal/agents/video/service/generation_plan.go`
- Modify: `cloud-backend/internal/agents/video/service/generation_plan_test.go`
- Modify: `cloud-backend/internal/core/worker/tool/builtin/video_creation_external_tools.go`
- Modify: `cloud-backend/internal/core/worker/tool/builtin/video_creation_external_tools_test.go`

**Interfaces:**
- Extends: `model.ShotGenerationPlan` with `StartMs`, `EndMs`, `DurationMs`, `TimelineRevision`, and `NarrationText`
- Produces: canonical timing and narration in `shotGenerationPlans`, `shotAssetPackages`, `videoPrompts`, external generation requests, voiceover, and subtitle payloads
- Consumes: canonical `shotList`/`timeWindows`, optional `shotGenerationPlans`

- [ ] **Step 1: Write failing propagation tests**

Use two Shots with literal ranges and distinct narration. Assert that generation-plan and video-prompt/package outputs preserve both segments and all millisecond fields, even when optional generation-plan input omits narration.

- [ ] **Step 2: Run the focused tests and verify RED**

```bash
go test ./internal/agents/video/service ./internal/core/worker/tool/builtin -run 'TestBuildShotGenerationPlanPreservesCanonicalContext|TestVideoPromptGeneratorPreservesPerShotNarrationAndTiming' -count=1
```

Expected: FAIL because those fields are currently dropped.

- [ ] **Step 3: Implement Shot-ID merging and propagation**

Populate canonical fields when building `ShotGenerationPlan`. In builtin adapters, merge derived plans into canonical Shot records by `shotId`; canonical narration and timing always win. Remove the full-project-topic fallback when a canonical Shot exists. Copy canonical context into every user-facing and executable Shot payload.

- [ ] **Step 4: Run focused and builtin tool tests**

```bash
go test ./internal/agents/video/service ./internal/core/worker/tool/builtin -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cloud-backend/internal/agents/video/model/creation.go cloud-backend/internal/agents/video/service/generation_plan.go cloud-backend/internal/agents/video/service/generation_plan_test.go cloud-backend/internal/core/worker/tool/builtin/video_creation_external_tools.go cloud-backend/internal/core/worker/tool/builtin/video_creation_external_tools_test.go
git commit -m "fix(video): preserve shot context through prompt generation"
```

### Task 3: Generate One Coordinated Three-Layer Shot Design

**Files:**
- Modify: `cloud-backend/internal/core/worker/tool/builtin/video_creation_external_tools.go`
- Modify: `cloud-backend/internal/core/worker/tool/builtin/video_creation_external_tools_test.go`
- Modify: `cloud-backend/skill-capabilities/video/codex-video-skill/1.0.0/prompts/video_prompt_generator.prompt.md`
- Modify: `cloud-backend/skill-capabilities/video/codex-video-skill/1.0.0/tools/video_prompt_generator.tool.yaml`

**Interfaces:**
- Produces: `visualAnchor` with `thesis`, `primaryReferenceImage`, `baseComposition`, and `timelineBeats`
- Produces: timed `ipArollPlan.timeline`, `hyperframesPlan.timeline`, `aigcPlan.timeline`
- Produces: literary `aigcPlan.prompt` and exact `hyperframesPlan.hyperKeyframes`

- [ ] **Step 1: Write failing contract tests**

Assert from a literal six-second Shot that the three layers reference the same visual anchor; IP beats include placement/camera/action/lighting; HyperFrames beats include exact text/style/position/start/end; and AIGC includes a primary reference plus a literary time-aware prompt without fps/codec/FFmpeg instructions.

- [ ] **Step 2: Run the focused builtin tests and verify RED**

```bash
go test ./internal/core/worker/tool/builtin -run 'TestVideoPromptGeneratorBuildsCoordinatedTimedLayers|TestVibePromptSeparatesCreativeDirectionFromDeliveryMechanics' -count=1
```

Expected: FAIL because current layer plans are generic full-duration descriptions.

- [ ] **Step 3: Implement the visual anchor and timed layer builders**

Create deterministic opening/development/settle beats from Shot duration and authored action beats. Reuse the primary keyframe/reference across layers. Build concise IP direction, exact HyperKeyframes events, and a Vibe-style AIGC prompt describing evolving mood and environment. Keep technical target fields in request metadata only.

- [ ] **Step 4: Update the registered prompt/tool contract**

Require the same structured fields in both the builtin system prompt and skill-capability prompt/YAML so LLM and deterministic paths produce compatible payloads.

- [ ] **Step 5: Run builtin tests**

```bash
go test ./internal/core/worker/tool/builtin -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add cloud-backend/internal/core/worker/tool/builtin/video_creation_external_tools.go cloud-backend/internal/core/worker/tool/builtin/video_creation_external_tools_test.go cloud-backend/skill-capabilities/video/codex-video-skill/1.0.0/prompts/video_prompt_generator.prompt.md cloud-backend/skill-capabilities/video/codex-video-skill/1.0.0/tools/video_prompt_generator.tool.yaml
git commit -m "feat(video): coordinate timed shot visual layers"
```

### Task 4: Repair Historical Narration Projection

**Files:**
- Modify: `frontend/src/features/creator-studio/completedShotProjection.ts`
- Modify: `frontend/scripts/creator-studio-logic-check.mjs`

**Interfaces:**
- Produces: canonical historical narration selected from `SHOT_LIST` or `TIME_WINDOW_PLAN` before derived packages
- Preserves: read-only archive behavior

- [ ] **Step 1: Add the real failure shape as a regression fixture**

Build five historical Shots where `SHOT_LIST` contains five distinct segments and `VIDEO_PROMPTS` repeats the full script. Assert each review returns its canonical segment and concatenation equals the full script.

- [ ] **Step 2: Run the creator contract test and verify RED**

```bash
npm run test:creator
```

Expected: FAIL because direct derived package records currently win.

- [ ] **Step 3: Implement provenance-aware projection**

Collect canonical planning records separately, add `TIME_WINDOW_PLAN` as shared Shot context, and select narration/timing from canonical records before derived records. Do not mutate archived content.

- [ ] **Step 4: Run creator tests**

```bash
npm run test:creator
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/features/creator-studio/completedShotProjection.ts frontend/scripts/creator-studio-logic-check.mjs
git commit -m "fix(creator): recover canonical historical narration"
```

### Task 5: Replace Inline Duplicate Disclosure with a Detail Drawer

**Files:**
- Create: `frontend/src/features/creator-studio/historicalDetail.ts`
- Modify: `frontend/src/features/creator-studio/components/ShotInspector.tsx`
- Modify: `frontend/src/index.css`
- Modify: `frontend/scripts/creator-studio-logic-check.mjs`

**Interfaces:**
- Produces: `HistoricalDetail` and `buildHistoricalDetail(id, label, text)`
- Consumes: one selected detail in `HistoricalShotInspector`
- UI contract: stable footer button, one dialog value, Escape/backdrop/close dismissal, focus restoration

- [ ] **Step 1: Write the failing detail-model test**

Assert the helper trims labels, preserves full text once, rejects empty details, and assigns stable IDs suitable for `aria-labelledby` and `aria-describedby`.

- [ ] **Step 2: Run the creator test and verify RED**

```bash
npm run test:creator
```

Expected: FAIL because the detail helper does not exist.

- [ ] **Step 3: Implement the helper and drawer UI**

Replace all native `details` instances with fixed-footer buttons. Store the triggering element, open one `role="dialog"` drawer, close on Escape/backdrop/button, and restore focus. Render the full value only inside the drawer while the card keeps a clamped preview.

- [ ] **Step 4: Implement stable responsive layout**

Use equal-height grid cards with flex columns and `margin-top:auto` action footers. Use a right-side drawer at desktop widths and bottom sheet below the mobile breakpoint. Respect light/dark theme tokens and reduced motion.

- [ ] **Step 5: Run frontend verification**

```bash
npm run test:creator
npm run lint
npm run build
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/features/creator-studio/historicalDetail.ts frontend/src/features/creator-studio/components/ShotInspector.tsx frontend/src/index.css frontend/scripts/creator-studio-logic-check.mjs
git commit -m "fix(creator): open shot details in stable drawer"
```

### Task 6: Full Regression and Real Completed-Task Verification

**Files:**
- Modify only if a failing verification exposes a scoped defect.

**Interfaces:**
- Consumes: all prior tasks
- Produces: verified frontend/backend build and real-project review evidence

- [ ] **Step 1: Run all scoped test suites**

```bash
cd cloud-backend && go test ./... -count=1
cd ../frontend && npm run test:creator && npm run test:settings && npm run test:director && npm run test:developer-build && npm run lint && npm run build
```

Expected: PASS.

- [ ] **Step 2: Verify the real completed fixture**

Use the archived `vp-1b8ceb41` data shape to verify that Shot 1-5 show distinct narration and that the drawer opens without card reflow or repeated content.

- [ ] **Step 3: Inspect the final diff**

```bash
git status --short
git diff --check
git log --oneline -8
```

Expected: no uncommitted files, no whitespace errors, and task commits present.

- [ ] **Step 4: Complete the branch using the finishing workflow**

Follow `superpowers:finishing-a-development-branch` and present only verified integration options.

