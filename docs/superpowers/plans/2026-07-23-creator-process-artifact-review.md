# Creator Process and Artifact Review Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make every durable creation step and intermediate artifact readable, revisitable, and safely regeneratable from the Tangying desktop creator workspace.

**Architecture:** Extend the backend-authoritative `CreationView` with a sanitized process projection and lightweight artifact descriptors derived from current/history artifacts plus the project's durable agent run. Add an idempotent step-regeneration mutation that reuses the existing review runtime and stale propagation. Render the projection in React with typed JSON, Markdown, image, video, audio, and fallback proofing components; the frontend never infers workflow completion.

**Tech Stack:** Go 1.24, Gin, PostgreSQL repositories, React 18, TypeScript 5.5, Vite, Electron, react-markdown, CSS, Node-based contract checks.

## Global Constraints

- Preserve current dirty-worktree changes and stage only files changed for this feature.
- Keep the six public creator steps: `requirements`, `direction`, `script`, `shots`, `preview`, `delivery`.
- Preserve every historical artifact and attempt; regeneration never overwrites an accepted version.
- Mark all downstream public steps stale after regeneration and keep stale artifacts readable.
- Do not expose hidden reasoning, credentials, unrestricted logs, raw tool arguments, or private model prompts.
- Keep API additions backward compatible and regenerate the checked-in TypeScript contract from the Go OpenAPI schema.
- Use TDD: observe each new test fail before implementing its behavior.

---

### Task 1: Backend audit projection contract

**Files:**
- Modify: `cloud-backend/internal/agents/video/model/creator_view.go`
- Create: `cloud-backend/internal/agents/video/service/creator_audit_projection.go`
- Modify: `cloud-backend/internal/agents/video/service/creator_view.go`
- Modify: `cloud-backend/internal/agents/video/service/creator_view_test.go`
- Modify: `cloud-backend/cmd/tangying-ai-os/main.go`

**Interfaces:**
- Consumes: `VideoProject.CurrentRunID`, `agentruntime.Repository.FindRun`, `NodeRepository.FindByTaskID`, artifact current/history readers.
- Produces: `CreatorStep.HasHistory`, `AttemptCount`, `ArtifactCount`, `StartedAt`, `UpdatedAt`, `IsStale`; `CreationView.ProcessTimeline`; `CreationView.StepArtifacts`; `CreatorViewService.WithProcessAudit`.

- [ ] **Step 1: Write failing projection tests**

Add tests that provide a terminal project with `CurrentRunID`, a successful `script_generation` source node and successful review gate, but no script artifact. Assert the script step is `confirmed`, `hasHistory=true`, readable metadata is populated, and the timeline contains only allow-listed summary fields. Add artifact-history fixtures and assert current/historical descriptors are grouped under the mapped public step.

- [ ] **Step 2: Run the focused tests and verify failure**

Run: `go test ./internal/agents/video/service -run 'TestCreatorAudit|TestCreatorViewLegacy' -count=1`

Expected: FAIL because the audit fields and `WithProcessAudit` do not exist.

- [ ] **Step 3: Add the model and projection implementation**

Define stable structs in `creator_view.go`:

```go
type CreatorArtifactDescriptor struct {
    ArtifactID string `json:"artifactId"`
    StepID CreatorStepID `json:"stepId"`
    Name string `json:"name"`
    Kind string `json:"kind"`
    MimeType string `json:"mimeType,omitempty"`
    Version int `json:"version"`
    Attempt int `json:"attempt"`
    IsCurrent bool `json:"isCurrent"`
    IsStale bool `json:"isStale"`
    CreatedAt time.Time `json:"createdAt"`
}

type CreatorProcessEvent struct {
    ID string `json:"id"`
    StepID CreatorStepID `json:"stepId"`
    Attempt int `json:"attempt"`
    State string `json:"state"`
    SourceType string `json:"sourceType"`
    SourceID string `json:"sourceId"`
    Title string `json:"title"`
    Summary string `json:"summary,omitempty"`
    StartedAt *time.Time `json:"startedAt,omitempty"`
    CompletedAt *time.Time `json:"completedAt,omitempty"`
    ArtifactIDs []string `json:"artifactIds"`
}
```

Implement a projection helper that maps only allow-listed stage identifiers and uses node name/status/timestamps without copying node input/output. Merge evidence by explicit state priority, sort timeline events deterministically, and lazy-list artifact history only for creator-mapped current artifacts.

- [ ] **Step 4: Wire process repositories**

Add `WithProcessAudit(runReader, nodeReader)` to `CreatorViewService` and wire `agentRunRepo` plus `nodeRepo` from `main.go`. A missing audit reader must retain the existing artifact-only behavior.

- [ ] **Step 5: Run projection and existing creator tests**

Run: `go test ./internal/agents/video/service -run 'TestCreator' -count=1`

Expected: PASS.

### Task 2: Idempotent regeneration from a completed step

**Files:**
- Modify: `cloud-backend/internal/agents/video/model/creator_view.go`
- Modify: `cloud-backend/internal/agents/video/service/creator_view.go`
- Modify: `cloud-backend/internal/agents/video/service/creator_view_test.go`
- Modify: `cloud-backend/internal/agents/video/handler/creator_view_handler.go`
- Modify: `cloud-backend/internal/agents/video/handler/creator_view_handler_test.go`

**Interfaces:**
- Consumes: `creatorReviewMutations.RegenerateIdempotent`, process review lineage, `ProjectService.MarkAgentRunStarted`.
- Produces: `PreviewStepRegeneration`, `RegenerateStep`, `POST /api/video-projects/:id/steps/:stepId/regenerations`, `StepRegenerationResult`.

- [ ] **Step 1: Write failing service and handler tests**

Test that regeneration rejects an unknown step, missing idempotency key, mismatched affected-step confirmation, foreign project lineage, and ambiguous review lineage. Test that the same idempotency key calls the runtime once and returns the same logical attempt. Assert later public steps are reported stale while older artifacts remain present.

- [ ] **Step 2: Run the focused tests and verify failure**

Run: `go test ./internal/agents/video/service ./internal/agents/video/handler -run 'Test.*StepRegenerat' -count=1`

Expected: FAIL because the endpoint and service methods do not exist.

- [ ] **Step 3: Implement impact and regeneration**

Use the canonical public step order to calculate downstream step IDs. Resolve the latest matching review gate from the current project's run and require its stage to map to the requested step. Call `RegenerateIdempotent(runID, reviewID, userID, instruction, idempotencyKey)`, rely on the existing artifact stale propagation, mark the project run active, then return the refreshed creation view and attempt metadata.

- [ ] **Step 4: Register and secure the route**

Register `POST /:id/steps/:stepId/regenerations`, require project ownership and `Idempotency-Key`, bind the typed request, and translate version/idempotency/lineage conflicts to `409` without exposing internal errors.

- [ ] **Step 5: Run service and handler tests**

Run: `go test ./internal/agents/video/service ./internal/agents/video/handler -count=1`

Expected: PASS.

### Task 3: OpenAPI and frontend contract

**Files:**
- Modify: `cloud-backend/internal/core/apispec/cloud_schemas.go`
- Modify: `cloud-backend/internal/core/apispec/cloud_paths.go`
- Modify: `cloud-backend/internal/core/apispec/cloud_spec_test.go`
- Modify: `frontend/src/utils/api-types.generated.ts` through the generator
- Modify: `frontend/src/features/creator-studio/types.ts`
- Modify: `frontend/src/services/creatorApi.ts`
- Modify: `frontend/scripts/creator-studio-logic-check.mjs`

**Interfaces:**
- Consumes: backend model from Tasks 1-2.
- Produces: generated TypeScript types, `regenerateStep(projectId, stepId, request, idempotencyKey, signal)`.

- [ ] **Step 1: Write failing OpenAPI and client contract assertions**

Assert the creation view schema contains timeline/artifact arrays, creator steps contain history/freshness fields, and the regeneration path requires an idempotency header. Add frontend script assertions for the generated fields and stable regeneration key.

- [ ] **Step 2: Run contract tests and verify failure**

Run: `go test ./internal/core/apispec -count=1 && npm --prefix ../frontend run test:creator`

Expected: FAIL on missing schemas/path/client function.

- [ ] **Step 3: Extend the API schema and regenerate TypeScript**

Add schemas for descriptors, events, regeneration request/result, and the POST path. Run:

`go run ./cmd/gen-ts -output ../frontend/src/utils/api-types.generated.ts`

- [ ] **Step 4: Add the typed frontend API call**

Implement the client call with positive-version validation only when a base version exists, mandatory idempotency validation, abort signal propagation, and no client-derived downstream impact.

- [ ] **Step 5: Run contract checks**

Run: `go test ./internal/core/apispec -count=1 && npm --prefix ../frontend run test:creator`

Expected: PASS.

### Task 4: Typed artifact presentation primitives

**Files:**
- Create: `frontend/src/features/creator-studio/artifactPresentation.ts`
- Create: `frontend/src/features/creator-studio/components/JsonArtifactViewer.tsx`
- Create: `frontend/src/features/creator-studio/components/MarkdownArtifactViewer.tsx`
- Create: `frontend/src/features/creator-studio/components/ArtifactProofingCanvas.tsx`
- Modify: `frontend/scripts/creator-studio-logic-check.mjs`

**Interfaces:**
- Consumes: `ArtifactContentResponse`, descriptor metadata, `react-markdown`.
- Produces: `classifyArtifactPresentation`, `buildJsonSummary`, `filterJsonTree`, `ArtifactProofingCanvas`.

- [ ] **Step 1: Write failing pure presentation tests**

Assert JSON summary root type/count/depth, safe recursive filtering, homogeneous-array table detection, malformed-JSON fallback, Markdown classification, MIME-first media classification, and HTML-not-trusted Markdown configuration.

- [ ] **Step 2: Run the frontend logic test and verify failure**

Run: `npm run test:creator`

Expected: FAIL because presentation helpers are missing.

- [ ] **Step 3: Implement JSON and Markdown viewers**

Render JSON branches as semantic nested lists with `<button aria-expanded>`, cap default expansion depth, provide search and raw tabs, and render safe scalar values. Render Markdown through `react-markdown` with raw HTML disabled, semantic table/code styles, generated heading navigation, source mode, copy, and download actions.

- [ ] **Step 4: Implement media and fallback proofing**

Render image/video/audio with native controls and existing selection callbacks. On media error, retain a metadata/download card. Never render unknown binary content as text.

- [ ] **Step 5: Run presentation tests**

Run: `npm run test:creator`

Expected: PASS.

### Task 5: Process rail, artifact drawer, and regeneration interaction

**Files:**
- Modify: `frontend/src/features/creator-studio/logic.ts`
- Modify: `frontend/src/features/creator-studio/ProjectWorkspacePage.tsx`
- Modify: `frontend/src/features/creator-studio/components/CreationStrip.tsx`
- Modify: `frontend/src/features/creator-studio/components/ArtifactReviewPanel.tsx`
- Modify: `frontend/src/features/creator-studio/components/AgentReviewGatePanel.tsx`
- Create: `frontend/src/features/creator-studio/components/CreatorProcessTimeline.tsx`
- Create: `frontend/src/features/creator-studio/components/StepArtifactDrawer.tsx`
- Create: `frontend/src/features/creator-studio/components/StepRegenerationDialog.tsx`
- Modify: `frontend/src/index.css`
- Modify: `frontend/scripts/creator-studio-logic-check.mjs`

**Interfaces:**
- Consumes: typed proofing canvas and backend audit projection.
- Produces: revisitable process rail, complete timeline, lazy artifact selection, artifact drawer, impact-confirmed regeneration.

- [ ] **Step 1: Write failing navigation and state tests**

Assert `isCreatorStepReadable` accepts `hasHistory=true`, artifact selections use descriptor ID/version rather than only `currentArtifactId`, stale responses cannot replace the current selection, and regeneration keys change when instructions or confirmed impact changes.

- [ ] **Step 2: Run the frontend test and verify failure**

Run: `npm run test:creator`

Expected: FAIL on history readability and new selection/regeneration behavior.

- [ ] **Step 3: Implement process and artifact navigation**

Make history-bearing steps clickable, preserve the route-selected step, lazy-load selected descriptor content, and show the timeline plus drawer without allowing a pending review to hide all other process evidence. Keep review actions prominent within the selected step.

- [ ] **Step 4: Implement regeneration interaction**

Show downstream impact before dispatch, require explicit confirmation in the dialog, send one idempotent request, adopt the returned view, switch active polling on, and keep old artifacts visible with stale/history badges.

- [ ] **Step 5: Apply accessible proofing styles**

Add the ink process rail, warm proofing canvas, restrained amber accents, visible focus rings, light/dark theme contrast, responsive two-column layout, and reduced-motion rules. Status labels must include text/icons, not color alone.

- [ ] **Step 6: Run frontend checks**

Run: `npm run test:creator && npm run build && npm run lint`

Expected: PASS.

### Task 6: Full regression and installed-client acceptance

**Files:**
- Modify only if a failing regression reveals a scoped defect in files from Tasks 1-5.

**Interfaces:**
- Consumes: completed backend/frontend feature.
- Produces: release evidence for the known completed demo project.

- [ ] **Step 1: Run backend regression suites**

Run: `go test ./internal/agents/video/... ./internal/core/apispec/... ./internal/core/agentruntime/... -count=1`

Expected: PASS.

- [ ] **Step 2: Run frontend release checks**

Run: `npm run test:creator && npm run test:settings && npm run build && npm run lint`

Expected: PASS.

- [ ] **Step 3: Rebuild and launch the desktop client**

Run the repository's existing macOS client packaging/installation script, start the existing Docker backend, and open the installed Tangying client. Do not replace backend data.

- [ ] **Step 4: Verify the known completed demo through the UI**

Open project `vp-1b8ceb41`. Confirm the final video renders, every durable completed step is clickable, JSON is structured, Markdown is rendered, intermediate media appears in the drawer, and refresh preserves the same state.

- [ ] **Step 5: Verify regeneration safely**

Use a completed non-delivery step with a harmless instruction. Confirm the impact dialog, start one regeneration, verify a new attempt appears, downstream steps become stale, and the old accepted artifact remains readable.

- [ ] **Step 6: Record final evidence**

Capture the exact test/build commands, installed app version, project route, and UI observations. Run `git diff --check` and inspect `git status --short` to ensure unrelated dirty files were not staged.
