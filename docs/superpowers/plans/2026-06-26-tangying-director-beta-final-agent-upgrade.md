# Tangying Director Beta Final Agent Upgrade Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the last beta release gaps for Tangying Director agent runtime: review approval must confirm artifacts, LocalJob completion must sync final video artifacts, render/package guards must check exact stage+kind, and tests must prove the flow.

**Architecture:** Keep cloud orchestration in `cloud-backend` and local execution contracts in `local-backend`. Add exact artifact lookup by `projectID + stageName + artifactKind`, use it for review fallback and dependency guards, and make artifact manifests invalid when required fields are missing instead of silently skipping them.

**Tech Stack:** Go cloud backend, Go local backend, existing React frontend logic, existing OpenAPI generation.

---

### Task 1: Review Approval Fallback

**Files:**
- Modify: `cloud-backend/internal/core/agentruntime/handler.go`
- Modify: `cloud-backend/internal/core/agentruntime/handler_review_test.go`
- Modify: `cloud-backend/internal/core/agentruntime/handler_stale_test.go`
- Modify: `cloud-backend/internal/core/artifact/service.go`
- Modify: `cloud-backend/internal/core/artifact/repository.go`

- [ ] **Step 1: Write failing test**

Add `TestApproveReviewApprovesCurrentArtifactByStageAndKindWhenArtifactIDMissing` with a ready preview review node that has `stage=preview` and `artifactKinds=["PREVIEW_SNAPSHOTS"]`, but no `artifactId`. Assert `ApproveReview` calls `ApproveCurrentArtifactsByStageAndKinds("project-1","preview",["PREVIEW_SNAPSHOTS"],"user-1")`.

- [ ] **Step 2: Verify RED**

Run:
```bash
cd cloud-backend && go test ./internal/core/agentruntime -run TestApproveReviewApprovesCurrentArtifactByStageAndKindWhenArtifactIDMissing -count=1
```
Expected: compile or assertion failure because `ArtifactService` has no fallback approval method.

- [ ] **Step 3: Implement minimal code**

Add `FindCurrentByStageAndKind` to artifact repository/service, add `ApproveCurrentArtifactsByStageAndKinds`, and make `approveReviewedArtifact` prefer `artifactId`, then fallback to `projectID + stage + artifactKinds|requiredOutputs`.

- [ ] **Step 4: Verify GREEN**

Run:
```bash
cd cloud-backend && go test ./internal/core/agentruntime -run 'TestApproveReview|TestTriggerDownstreamStale' -count=1
```
Expected: PASS.

### Task 2: Manifest Strictness And LocalJob UnitID

**Files:**
- Modify: `cloud-backend/internal/core/artifact/materializer.go`
- Modify: `cloud-backend/internal/core/artifact/materializer_test.go`
- Modify: `cloud-backend/cmd/tangying-ai-os/main.go`
- Modify: `cloud-backend/cmd/tangying-ai-os/artifact_sync_test.go`
- Modify: `cloud-backend/internal/core/localrunner/handler.go`
- Modify: `cloud-backend/internal/core/localrunner/handler_test.go`

- [ ] **Step 1: Write failing tests**

Add tests for missing `unitId` in `artifacts[]`, local job render output preserving `unitId=final-video`, and HYPERFRAMES_RENDER normalization adding `unitId`.

- [ ] **Step 2: Verify RED**

Run:
```bash
cd cloud-backend && go test ./internal/core/artifact ./internal/core/localrunner ./cmd/tangying-ai-os -run 'RejectsArtifactWithoutUnitID|HyperFramesRender|LocalJobCompletion' -count=1
```
Expected: failures because current materializer silently skips invalid entries and local job sync writes `UnitID=stage`.

- [ ] **Step 3: Implement minimal code**

Change materializer to expose a checked builder that returns `ARTIFACT_MANIFEST_INVALID` on missing `unitId` or `kind`. Keep existing callers compatible by returning no requests from the legacy wrapper. Change local job artifact conversion to require `unitId`, use it as `CreateArtifactRequest.UnitID`, and return errors to the local runner handler callback.

- [ ] **Step 4: Verify GREEN**

Run:
```bash
cd cloud-backend && go test ./internal/core/artifact ./internal/core/localrunner ./cmd/tangying-ai-os -count=1
```
Expected: PASS.

### Task 3: Exact Render And Package Guards

**Files:**
- Modify: `cloud-backend/internal/core/worker/service/render_dependency_checker.go`
- Modify: `cloud-backend/internal/core/agentruntime/runtime_e2e_test.go`
- Modify: `cloud-backend/cmd/tangying-ai-os/main.go`

- [ ] **Step 1: Write failing tests**

Update runtime E2E fixture keys to `stage|kind` and add cases where an unrelated preview or quality artifact exists but the required kind is missing. Assert render/package are blocked.

- [ ] **Step 2: Verify RED**

Run:
```bash
cd cloud-backend && go test ./internal/core/agentruntime -run TestRuntime_TangyingDirector_FirstBeta -count=1
```
Expected: failures because current guard uses stage-only lookup.

- [ ] **Step 3: Implement minimal code**

Replace `FindCurrentByKind(projectID, stage)` with `FindCurrentByStageAndKind(projectID, stage, kind)` in worker guard contracts. Check `composition + VIDEO_COMPOSITION_SPEC`, `preview + HYPERFRAMES_PROJECT`, `preview + PREVIEW_SNAPSHOTS`, `render + VIDEO`, `quality + FFMPEG_PROBE_REPORT`, and `quality + FINAL_REVIEW`.

- [ ] **Step 4: Verify GREEN**

Run:
```bash
cd cloud-backend && go test ./internal/core/agentruntime ./internal/core/worker/service -count=1
```
Expected: PASS.

### Task 4: Local Executor Artifact Contracts

**Files:**
- Modify: `local-backend/internal/localtool/hyperframes_project.go`
- Modify: `local-backend/internal/localtool/hyperframes_snapshot.go`
- Modify: `local-backend/internal/localtool/hyperframes_render.go`
- Modify: `local-backend/internal/localtool/ffmpeg_probe.go`
- Modify: `local-backend/internal/localtool/final_review.go`
- Modify: `local-backend/internal/localtool/artifact_package.go`

- [ ] **Step 1: Write or extend focused contract tests where services can run without external dependencies**

Use existing localtool tests and cloud localrunner normalization tests to assert `unitId`, `status`, `humanApproved`, `dependsOn`, `producedByTool`, `producedByRole`, and `metadata`.

- [ ] **Step 2: Implement minimal code**

Add the required `unitId` and missing `metadata` fields to each executor artifact output. For render, set `unitId=final-video` and include render metrics in artifact metadata.

- [ ] **Step 3: Verify**

Run:
```bash
cd local-backend && go test ./internal/localtool -count=1
```
Expected: PASS.

### Task 5: Frontend Compatibility Check

**Files:**
- Inspect: `frontend/src/pages/DirectorStudioPage.tsx`
- Inspect: `frontend/src/pages/directorStudioLogic.ts`

- [ ] **Step 1: Confirm existing UI gates use ArtifactIndex data**

Check whether render/export actions derive from `artifacts` and `review` data rather than local optimistic state.

- [ ] **Step 2: Apply minimal UI fix only if needed**

If render/export buttons expose actions without `humanApproved` or `VIDEO` artifact state, wire them to ArtifactIndex-derived checks.

- [ ] **Step 3: Verify**

Run:
```bash
cd frontend && npm run build
```
Expected: PASS.

### Task 6: Full Verification

**Files:**
- Generated docs/types only if API schema changes.

- [ ] **Step 1: Format and targeted tests**

Run:
```bash
gofmt -w cloud-backend/internal/core/agentruntime cloud-backend/internal/core/artifact cloud-backend/internal/core/worker/service cloud-backend/internal/core/localrunner cloud-backend/cmd/tangying-ai-os local-backend/internal/localtool
cd cloud-backend && go test ./internal/core/agentruntime ./internal/core/artifact ./internal/core/worker/service ./internal/core/localrunner ./cmd/tangying-ai-os -count=1
cd ../local-backend && go test ./internal/localtool -count=1
```

- [ ] **Step 2: Full release checks**

Run:
```bash
cd cloud-backend && go test ./...
cd ../local-backend && go test ./...
cd ../frontend && npm run build
```
Expected: all PASS. Run `make gen-docs && make api-docs-check` only if route/schema output changes.
