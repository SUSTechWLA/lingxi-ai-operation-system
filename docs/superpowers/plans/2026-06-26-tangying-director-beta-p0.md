# Tangying Director Beta P0 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the Tangying Director v1.0 beta backend release-ready for true artifact approval, downstream stale tracking, local render artifact output, and runtime acceptance coverage.

**Architecture:** Keep stage names and artifact kinds separate. Review gates carry real artifact IDs after artifacts are materialized; stale tracking uses real artifact IDs first and falls back to stage-name based invalidation. Local runner completion normalizes HyperFrames render output before syncing it into the ArtifactIndex.

**Tech Stack:** Go cloud backend, existing artifact repository/service, agentruntime review handler, localrunner handler, worker render dependency checker, generated OpenAPI docs.

---

### Task 1: Stale Tracking Naming And Real Artifact Fallback

**Files:**
- Modify: `cloud-backend/internal/core/artifact/service.go`
- Modify: `cloud-backend/internal/core/artifact/repository.go`
- Modify: `cloud-backend/internal/core/artifact/stale.go`
- Modify: `cloud-backend/internal/core/agentruntime/handler.go`
- Test: `cloud-backend/internal/core/agentruntime/handler_stale_test.go`
- Test: `cloud-backend/internal/core/artifact/beta_index_test.go`

- [ ] **Step 1: Write failing tests**

Add tests proving `triggerDownstreamStale` calls `MarkDownstreamStale` with a real `artifactId`, falls back to `MarkDownstreamStaleByStageName`, and repository naming uses stage names.

- [ ] **Step 2: Verify tests fail**

Run: `cd cloud-backend && go test ./internal/core/agentruntime ./internal/core/artifact -run 'TestTriggerDownstreamStale|TestDownstreamStaleArtifactKinds'`
Expected: FAIL because the handler interface does not expose `MarkDownstreamStaleByStageName` and repository/service still call `MarkStaleByKind`.

- [ ] **Step 3: Implement minimal code**

Rename repository method to `MarkStaleByStageNames`, add `Service.MarkDownstreamStaleByStageName`, update handler to prefer `artifactId`, and update stale comments.

- [ ] **Step 4: Verify pass**

Run: `cd cloud-backend && go test ./internal/core/agentruntime ./internal/core/artifact -run 'TestTriggerDownstreamStale|TestDownstreamStaleArtifactKinds'`

### Task 2: Review Gate Artifact ID Binding

**Files:**
- Modify: `cloud-backend/internal/core/agentruntime/handler.go`
- Modify: `cloud-backend/internal/core/agentruntime/artifact_review.go`
- Modify: `cloud-backend/internal/core/model/repository/interfaces.go`
- Modify: `cloud-backend/internal/core/model/repository/repository.go`
- Modify: `cloud-backend/cmd/tangying-ai-os/main.go`
- Test: `cloud-backend/internal/core/agentruntime/handler_test.go`
- Test: `cloud-backend/internal/core/agentruntime/e2e_test.go`

- [ ] **Step 1: Write failing tests**

Add tests proving a review node with `artifactId` approves that artifact, and local artifact sync backfills review node input with the real artifact ID.

- [ ] **Step 2: Verify tests fail**

Run: `cd cloud-backend && go test ./internal/core/agentruntime -run 'TestApproveReview|TestE2E_TangyingDirector_FirstUserBeta'`
Expected: FAIL until `artifactId` is surfaced in review models and backfilled from materialized artifacts.

- [ ] **Step 3: Implement minimal code**

Expose `artifactId` in `Review`, persist it in `artifact_reviews`, add `UpdateInputFields` to node repo, and wire local sync to write the created artifact ID into matching review gates.

- [ ] **Step 4: Verify pass**

Run: `cd cloud-backend && go test ./internal/core/agentruntime -run 'TestApproveReview|TestE2E_TangyingDirector_FirstUserBeta'`

### Task 3: HyperFrames Render Artifact Output Contract

**Files:**
- Modify: `cloud-backend/internal/core/localrunner/handler.go`
- Test: `cloud-backend/internal/core/localrunner/handler_test.go`

- [ ] **Step 1: Write failing test**

Add a test proving completing `HYPERFRAMES_RENDER` emits a normalized `VIDEO` artifact with `final.mp4`, local storage ref, mime type, status, human approval false, dependsOn, producer, and metadata.

- [ ] **Step 2: Verify test fails**

Run: `cd cloud-backend && go test ./internal/core/localrunner -run TestHandlerCompleteHyperFramesRenderNormalizesVideoArtifact`

- [ ] **Step 3: Implement minimal code**

Normalize `CompleteJobRequest.Output` in `completeJob` for `HYPERFRAMES_RENDER` before state-machine success and artifact sync.

- [ ] **Step 4: Verify pass**

Run: `cd cloud-backend && go test ./internal/core/localrunner -run TestHandlerCompleteHyperFramesRenderNormalizesVideoArtifact`

### Task 4: Runtime Acceptance E2E

**Files:**
- Modify: `cloud-backend/internal/core/agentruntime/runtime_e2e_test.go`
- Modify: `cloud-backend/internal/core/worker/service/render_dependency_checker.go`
- Test: `cloud-backend/internal/core/agentruntime/runtime_e2e_test.go`

- [ ] **Step 1: Write failing tests**

Extend `TestRuntime_TangyingDirector_FirstBeta` to cover six cases: preview unapproved blocks render, preview approved allows render, script edit stales downstream, local runner offline blocks render, failed final review blocks package, passed final review allows package.

- [ ] **Step 2: Verify tests fail**

Run: `cd cloud-backend && go test ./internal/core/agentruntime -run TestRuntime_TangyingDirector_FirstBeta`

- [ ] **Step 3: Implement minimal code**

Adjust render/package guard checks and stale utilities only as needed for these runtime cases.

- [ ] **Step 4: Verify pass**

Run: `cd cloud-backend && go test ./internal/core/agentruntime -run TestRuntime_TangyingDirector_FirstBeta`

### Task 5: Release Verification, Docs, Commit, Push

**Files:**
- Regenerate: `cloud-backend/docs/API_REFERENCE.md`
- Regenerate: `frontend/src/utils/api-types.generated.ts`

- [ ] **Step 1: Format and generate docs**

Run: `cd cloud-backend && gofmt -w <changed-go-files> && make gen-docs && make api-docs-check`

- [ ] **Step 2: Run full verification**

Run:
`cd frontend && node --experimental-strip-types scripts/directorStudioLogic.test.ts && npm run build`
`cd cloud-backend && go test ./...`
`cd local-backend && go test ./...`

- [ ] **Step 3: Commit and push**

Run:
`git status --short`
`git add <changed-files>`
`git commit -m "feat: prepare tangying director beta release"`
`git push origin develop_go`
