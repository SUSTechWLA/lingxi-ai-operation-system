# Creator Studio Simplification Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the development-heavy default frontend with a simple six-step creator studio backed by real versioned artifact and isolated Shot operations, while preserving the existing developer views behind an independent entry.

**Architecture:** Backend-first: strengthen the existing Shot-driven state and Artifact services, expose creator-facing aggregate contracts, and keep the backend authoritative for step, version, impact, candidate, and task state. The React/Electron frontend then introduces an independent creator shell and developer console, with small feature modules replacing creator use of the 6,000-line `DirectorStudioPage.tsx`; existing developer components are reused rather than rewritten.

**Tech Stack:** Go 1.x, Gin, PostgreSQL artifact index, React 18, TypeScript 5.5, Vite 5, Tailwind CSS 3, Electron 33, esbuild-based logic tests.

## Global Constraints

- Default creator navigation contains only `开始创作` and `我的视频`.
- Developer Trace, roles, raw artifacts, providers, QA provenance, diagnostics, and settings remain available only under `#/developer/*`.
- Preserve the existing palette exactly: primary `#E89412`, deep ink `#2B1606`, canvas `#FFF6D6`, card `#FFFDF6`, divider `#E8CF86`, success `#1F9D62`.
- Every Shot duration must be greater than zero and strictly less than 15 seconds at frontend validation, API validation, persistence, and assembly validation.
- Regenerating one Shot may mutate only that Shot and the final-assembly dirty flag; it must not mutate, stale, approve, reject, or enqueue another Shot.
- Shot mutations require `shotId`, `baseVersion`, `scope`, `locks`, and `Idempotency-Key`.
- Historical artifacts, Shot revisions, and candidates are immutable; restoring history creates a new current version.
- Backend state is authoritative. Frontend selectors may format state but may not infer workflow completion or dependency invalidation.
- Existing generation engines, QA algorithms, rendering engines, and current color system are outside the refactor scope.

---

## File structure

### Backend files to create

- `cloud-backend/internal/agents/video/model/creator_view.go` — creator step, Shot page, impact, mutation, and task DTOs.
- `cloud-backend/internal/agents/video/service/shot_review.go` — paginated Shot review, history, impact, regeneration, candidate acceptance, and restore logic.
- `cloud-backend/internal/agents/video/service/shot_review_test.go` — Shot isolation, history, idempotency, scope, lock, and pagination tests.
- `cloud-backend/internal/agents/video/service/creator_view.go` — six-step aggregation and artifact-to-step mapping.
- `cloud-backend/internal/agents/video/service/creator_view_test.go` — creator step status and impact mapping tests.
- `cloud-backend/internal/agents/video/handler/creator_view_handler.go` — creator view and step endpoints.
- `cloud-backend/internal/agents/video/handler/creator_view_handler_test.go` — auth, validation, and response contract tests.
- `cloud-backend/internal/core/artifact/revision_service.go` — reusable Artifact revision and restore orchestration extracted from the HTTP handler.
- `cloud-backend/internal/core/artifact/revision_service_test.go` — immutable restore and downstream stale tests.
- `cloud-backend/internal/core/agentruntime/shot_regeneration_guard_test.go` — plan-level target-Shot isolation tests.

### Backend files to modify

- `cloud-backend/internal/agents/video/model/creation.go` — strict duration constants and durable Shot revision/task state.
- `cloud-backend/internal/agents/video/service/creation_state.go` — decode defaults for new maps and schema version.
- `cloud-backend/internal/agents/video/service/creation_service.go` — duration validation and delegation to Shot review service.
- `cloud-backend/internal/agents/video/service/creation_service_test.go` — strict duration regression tests.
- `cloud-backend/internal/agents/video/service/generation_plan.go` — keep planned Shot durations below the exclusive limit.
- `cloud-backend/internal/agents/video/service/final_assembly.go` — reject accepted candidates at or above the exclusive limit.
- `cloud-backend/internal/agents/video/service/final_assembly_test.go` — assembly-boundary duration regression tests.
- `cloud-backend/internal/agents/video/handler/creation_handler.go` — page/filter parsing and Shot review mutation routes.
- `cloud-backend/internal/agents/video/handler/creation_handler_test.go` — REST contract tests.
- `cloud-backend/internal/core/artifact/model.go` — explicit forced-version restore request fields.
- `cloud-backend/internal/core/artifact/service.go` — restore and current-version helpers.
- `cloud-backend/internal/core/artifact/handler.go` — delegate revisions to the shared revision service.
- `cloud-backend/internal/core/agentruntime/runner.go` — inject target-Shot isolation context into every regeneration step.
- `cloud-backend/internal/core/agentruntime/plan_guard.go` — reject regeneration plans that can address another Shot.
- `cloud-backend/cmd/tangying-ai-os/main.go` — wire creator services after Artifact service construction.
- `cloud-backend/internal/core/apispec/cloud_spec.go` — publish creator and Shot endpoints.
- `cloud-backend/internal/core/apispec/cloud_spec_test.go` — require the new API surface.

### Frontend files to create

- `frontend/src/features/creator-studio/types.ts` — generated-contract-aligned creator types.
- `frontend/src/features/creator-studio/logic.ts` — pure step, Shot, filtering, conflict, and action selectors.
- `frontend/src/features/creator-studio/CreatorShell.tsx` — two-item creator navigation and route outlet.
- `frontend/src/features/creator-studio/StartCreationPage.tsx` — one-sentence creation and material intake.
- `frontend/src/features/creator-studio/VideoLibraryPage.tsx` — recent and historical video projects.
- `frontend/src/features/creator-studio/ProjectWorkspacePage.tsx` — six-step project container.
- `frontend/src/features/creator-studio/components/CreationStrip.tsx` — replayable six-step strip.
- `frontend/src/features/creator-studio/components/ArtifactReviewPanel.tsx` — review, direct edit, AI revision, impact, and version history.
- `frontend/src/features/creator-studio/components/ShotReviewQueue.tsx` — virtualized/paged long-video Shot queue.
- `frontend/src/features/creator-studio/components/ShotInspector.tsx` — selected candidate, script, adjacent preview, and acceptance.
- `frontend/src/features/creator-studio/components/ShotImprovePanel.tsx` — scopes, field locks, cost/time impact, and regeneration.
- `frontend/src/features/creator-studio/components/TaskRecoveryBanner.tsx` — durable task and reconnect state.
- `frontend/src/features/creator-studio/components/PreviewDeliveryPanel.tsx` — assembled preview, final QA, package, and download surface.
- `frontend/src/features/developer-console/DeveloperConsolePage.tsx` — independent wrapper around existing development views.
- `frontend/src/services/creatorApi.ts` — creator aggregate, material, Shot page, impact, version, and mutation calls.
- `frontend/scripts/creator-studio-logic-check.mjs` — pure contract and 100-Shot behavior tests.

### Frontend files to modify

- `frontend/src/App.tsx` — authenticated hash-route dispatch and production developer gate.
- `frontend/src/pages/DirectorStudioPage.tsx` — accept a developer-console-only initial view and stop serving as the default creator route.
- `frontend/src/index.css` — focused creator layout utilities using existing tokens.
- `frontend/src/utils/types.ts` — align shared project/material fields where generated types do not yet cover them.
- `frontend/package.json` — add `test:creator` and include it in verification.
- `frontend/scripts/director-studio-logic-check.mjs` — assert creator routes contain no developer vocabulary.

---

### Task 1: Enforce strict Shot duration and initialize durable Shot state

**Files:**
- Modify: `cloud-backend/internal/agents/video/model/creation.go`
- Modify: `cloud-backend/internal/agents/video/service/creation_state.go`
- Modify: `cloud-backend/internal/agents/video/service/creation_service.go`
- Modify: `cloud-backend/internal/core/agentruntime/runner.go`
- Modify: `cloud-backend/internal/core/agentruntime/plan_guard.go`
- Test: `cloud-backend/internal/core/agentruntime/shot_regeneration_guard_test.go`
- Modify: `cloud-backend/cmd/tangying-ai-os/main.go`
- Modify: `cloud-backend/internal/agents/video/service/generation_plan.go`
- Modify: `cloud-backend/internal/agents/video/service/final_assembly.go`
- Test: `cloud-backend/internal/agents/video/service/creation_service_test.go`
- Test: `cloud-backend/internal/agents/video/service/creation_foundation_test.go`
- Test: `cloud-backend/internal/agents/video/service/closed_beta_pipeline_test.go`
- Test: `cloud-backend/internal/agents/video/service/final_assembly_test.go`

**Interfaces:**
- Consumes: existing `model.ShotUnit`, `model.ShotPolicy`, and `model.ShotDrivenState`.
- Produces: `model.MaxShotDurationExclusiveSec`, `model.ValidateShotDuration(model.ShotUnit) error`, initialized `ShotHistory`, `RegenerationTasks`, `IdempotencyTasks`, and `AssemblyDirty` state for later tasks.

- [ ] **Step 1: Write failing strict-duration tests**

Add these assertions to `creation_service_test.go`:

```go
func TestCreationServiceRejectsShotAtFifteenSeconds(t *testing.T) {
	store := newFakeCreationProjectStore()
	store.project = &model.VideoProject{ID: "vp-1", UserID: "u-1"}
	svc := NewCreationService(store)

	_, err := svc.UpsertShot(context.Background(), "u-1", "vp-1", &model.ShotUnit{
		ID: "shot-15", DurationSec: 15,
	})
	if err == nil || !strings.Contains(err.Error(), "less than 15 seconds") {
		t.Fatalf("UpsertShot error = %v, want strict duration error", err)
	}
}

func TestCreationServiceAcceptsFourteenSecondShot(t *testing.T) {
	store := newFakeCreationProjectStore()
	store.project = &model.VideoProject{ID: "vp-1", UserID: "u-1"}
	svc := NewCreationService(store)

	shot, err := svc.UpsertShot(context.Background(), "u-1", "vp-1", &model.ShotUnit{
		ID: "shot-14", DurationSec: 14,
	})
	if err != nil || shot.DurationSec != 14 {
		t.Fatalf("shot = %+v error = %v", shot, err)
	}
}
```

- [ ] **Step 2: Run the service tests and verify failure**

Run: `cd cloud-backend && go test ./internal/agents/video/service -run 'TestCreationService(RejectsShotAtFifteenSeconds|AcceptsFourteenSecondShot)' -count=1`

Expected: FAIL because `UpsertShot` currently accepts a 15-second Shot.

- [ ] **Step 3: Add the strict duration validator and state fields**

Add to `model/creation.go`:

```go
const MaxShotDurationExclusiveSec = 15

func ValidateShotDuration(shot ShotUnit) error {
	durationMs := shot.DurationMs
	if durationMs == 0 && shot.DurationSec > 0 {
		durationMs = int64(shot.DurationSec) * 1000
	}
	if durationMs <= 0 || durationMs >= int64(MaxShotDurationExclusiveSec*1000) {
		return fmt.Errorf("shot duration must be greater than 0 and less than 15 seconds")
	}
	return nil
}

type ShotRevision struct {
	RevisionID string    `json:"revisionId"`
	ShotID     string    `json:"shotId"`
	Version    int       `json:"version"`
	Reason     string    `json:"reason"`
	Snapshot   ShotUnit  `json:"snapshot"`
	CreatedAt  time.Time `json:"createdAt"`
}

type ShotRegenerationTask struct {
	TaskID         string    `json:"taskId"`
	ShotID         string    `json:"shotId"`
	BaseVersion    int       `json:"baseVersion"`
	Scope          string    `json:"scope"`
	Locks          []string  `json:"locks,omitempty"`
	Instruction    string    `json:"instruction,omitempty"`
	IdempotencyKey string    `json:"idempotencyKey"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}
```

Import `fmt` beside `time`, set `ShotPolicy.MaxDurationSec` to `14`, and extend `ShotDrivenState`:

```go
type ShotDrivenState struct {
	SchemaVersion      int                              `json:"schemaVersion"`
	Spec               *VideoCreationSpec               `json:"spec,omitempty"`
	Shots              []ShotUnit                       `json:"shots,omitempty"`
	ShotHistory        map[string][]ShotRevision        `json:"shotHistory,omitempty"`
	RegenerationTasks  map[string]ShotRegenerationTask  `json:"regenerationTasks,omitempty"`
	IdempotencyTasks   map[string]string                `json:"idempotencyTasks,omitempty"`
	AssemblyDirty      bool                             `json:"assemblyDirty"`
	UpdatedAt          time.Time                        `json:"updatedAt"`
}
```

Initialize all three maps in `emptyShotDrivenState` and after decode. Call `model.ValidateShotDuration(normalized)` in `UpsertShot`. Change the shot generation loop to split while `duration >= model.MaxShotDurationExclusiveSec`.

Call the same validator when building generation plans and for every accepted candidate in `BuildFinalAssemblyPlanWithPolicy`. Assembly must return a blocking `shot_duration_out_of_range` issue for a candidate at `15.0` seconds rather than truncating or silently accepting it.

- [ ] **Step 4: Update legacy policy assertions**

Change tests that expect `MaxDurationSec == 15` to expect `14`, while retaining preferred duration `6..8` and minimum duration `3`.

- [ ] **Step 5: Run focused and package tests**

Run: `cd cloud-backend && go test ./internal/agents/video/model ./internal/agents/video/service -count=1`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add cloud-backend/internal/agents/video/model/creation.go cloud-backend/internal/agents/video/service/creation_state.go cloud-backend/internal/agents/video/service/creation_service.go cloud-backend/internal/agents/video/service/generation_plan.go cloud-backend/internal/agents/video/service/final_assembly.go cloud-backend/internal/agents/video/service/creation_service_test.go cloud-backend/internal/agents/video/service/creation_foundation_test.go cloud-backend/internal/agents/video/service/closed_beta_pipeline_test.go cloud-backend/internal/agents/video/service/final_assembly_test.go
git commit -m "feat: enforce strict shot duration"
```

---

### Task 2: Implement isolated Shot history, scoped regeneration, and idempotency

**Files:**
- Create: `cloud-backend/internal/agents/video/service/shot_review.go`
- Create: `cloud-backend/internal/agents/video/service/shot_review_test.go`
- Modify: `cloud-backend/internal/agents/video/service/creation_service.go`

**Interfaces:**
- Consumes: Task 1 `ShotHistory`, `RegenerationTasks`, `IdempotencyTasks`, `AssemblyDirty`, and `ValidateShotDuration`.
- Produces: `RegenerateShotV2`, `GetShotHistory`, `PreviewShotRegeneration`, durable `ShotRegenerationTask` results, and a target-only Agent dispatch guard.

- [ ] **Step 1: Write a 100-Shot isolation test**

Create `shot_review_test.go` with:

```go
func TestRegenerateShotV2MutatesOnlyTargetShot(t *testing.T) {
	store := newFakeCreationProjectStore()
	shots := make([]model.ShotUnit, 100)
	for i := range shots {
		shots[i] = model.ShotUnit{
			ID: fmt.Sprintf("shot-%03d", i+1), ProjectID: "vp-1",
			DurationSec: 6, Version: 1, ReviewStatus: model.ReviewStatusApproved,
		}
	}
	store.project = projectWithShotState(t, shots...)
	svc := NewCreationService(store)
	before := append([]model.ShotUnit(nil), shots...)

	result, err := svc.RegenerateShotV2(context.Background(), "u-1", "vp-1", "shot-012", RegenerateShotRequest{
		BaseVersion: 1, Scope: "base_media", Locks: []string{"duration", "character"},
		Instruction: "晨光更柔和", IdempotencyKey: "regen-shot-012-v1",
	})
	if err != nil {
		t.Fatalf("RegenerateShotV2 error: %v", err)
	}
	if result.Shot.ID != "shot-012" || result.Shot.Version != 2 {
		t.Fatalf("target result = %+v", result.Shot)
	}
	after := decodeStateFromTest(t, store.updated.Config)
	for i, shot := range after.Shots {
		if shot.ID == "shot-012" {
			continue
		}
		if !reflect.DeepEqual(shot, before[i]) {
			t.Fatalf("non-target shot changed: before=%+v after=%+v", before[i], shot)
		}
	}
}
```

Also add a second test that submits the same idempotency key twice and asserts the same `TaskID`, target version `2`, and history length `2`.

- [ ] **Step 2: Run the isolation tests and verify failure**

Run: `cd cloud-backend && go test ./internal/agents/video/service -run 'TestRegenerateShotV2' -count=1`

Expected: FAIL because the new request/result types and method do not exist.

- [ ] **Step 3: Define the scoped request and result**

In `creation_service.go`, replace the current request with:

```go
type RegenerateShotRequest struct {
	BaseVersion    int      `json:"baseVersion"`
	Scope          string   `json:"scope"`
	Locks          []string `json:"locks,omitempty"`
	Instruction    string   `json:"instruction,omitempty"`
	IdempotencyKey string   `json:"-"`
}

type RegenerateShotResult struct {
	Shot model.ShotUnit              `json:"shot"`
	Task model.ShotRegenerationTask  `json:"task"`
}

type ShotGenerationDispatcher interface {
	EnqueueShotRegeneration(ctx context.Context, userID, projectID string, task model.ShotRegenerationTask) (runID string, err error)
}
```

- [ ] **Step 4: Implement scope, lock, version, and idempotency validation**

In `shot_review.go`, define exact allowed values:

```go
var allowedRegenerationScopes = map[string]bool{
	"prompt": true, "reference": true, "base_media": true,
	"overlay": true, "audio_alignment": true, "full_shot": true,
}

var allowedShotLocks = map[string]bool{
	"duration": true, "narration": true, "character": true,
	"wardrobe": true, "scene": true, "camera": true,
	"first_frame": true, "last_frame": true,
	"reference_set": true, "accepted_overlay": true,
}
```

Implement `RegenerateShotV2` so it:

1. loads state once;
2. returns the existing task for an existing idempotency key;
3. rejects a mismatched `BaseVersion` with `ErrShotVersionConflict`;
4. records the unchanged current Shot as a `ShotRevision` before mutation;
5. increments only the target Shot version and clears only its accepted candidate;
6. writes a durable queued task and `AssemblyDirty=true`;
7. persists before dispatch so a crash cannot lose the request;
8. calls `ShotGenerationDispatcher` and stores the returned Agent run ID;
9. marks only that task failed if dispatch fails.

Use `crypto/rand`-backed UUIDs from `github.com/google/uuid`, already in the module.

Add `RunID string` to `ShotRegenerationTask`. Implement `CompleteShotRegeneration(ctx, taskID, candidate)` and `FailShotRegeneration(ctx, taskID, reason)`; both resolve the target from the durable task and reject a candidate whose `ShotID` differs from the task's `ShotID`.

- [ ] **Step 5: Guard the generated Agent plan to one Shot**

In `runner.go`, when request context contains `operation=shot_regeneration`, overwrite these arguments on every plan step before guard validation:

```go
step.Arguments["operation"] = "shot_regeneration"
step.Arguments["targetShotId"] = targetShotID
step.Arguments["allowedShotIds"] = []string{targetShotID}
```

In `plan_guard.go`, reject any step in such a plan when `shotId`, `targetShotId`, or a literal `shotIds` entry names a Shot outside `allowedShotIds`. References such as `{{step.output.shotId}}` are allowed only when the producing step is already target-guarded.

Add a test with two plan steps where the second uses `shotId=shot-013` while the allowed target is `shot-012`; expect `shot regeneration plan escapes target shot shot-012`.

- [ ] **Step 6: Wire the real dispatcher and completion path**

In `main.go`, adapt `agentRunner.StartAsync` with domain `video_creation` and context:

```go
map[string]interface{}{
	"operation": "shot_regeneration",
	"projectId": projectID,
	"targetShotId": task.ShotID,
	"allowedShotIds": []string{task.ShotID},
	"regenerationScope": task.Scope,
	"locks": task.Locks,
	"baseVersion": task.BaseVersion,
}
```

Extend the existing artifact-sync completion callback: when synchronized output metadata contains both `shotRegenerationTaskId` and `relatedShotId`, build one `ShotCandidate`, call `CompleteShotRegeneration`, and reject the callback if `relatedShotId` differs from the durable task target. Failure callbacks call `FailShotRegeneration` and never touch another Shot.

- [ ] **Step 7: Preserve the legacy route through the V2 service**

Change `RegenerateShot` to default a missing base version to the current version, derive a stable server key only for legacy requests, call `RegenerateShotV2`, and return `&result.Shot`. This keeps old clients working while new clients send explicit concurrency and idempotency fields.

- [ ] **Step 8: Run service and guard tests**

Run: `cd cloud-backend && go test ./internal/agents/video/service ./internal/core/agentruntime -run 'Test(RegenerateShotV2|ShotRegenerationPlan|CreationServiceRejectsRegenerateLockedShot|CreationServiceRejectReasonIsUsedByRegenerate)' -count=1`

Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add cloud-backend/internal/agents/video/service/shot_review.go cloud-backend/internal/agents/video/service/shot_review_test.go cloud-backend/internal/agents/video/service/creation_service.go cloud-backend/internal/core/agentruntime/runner.go cloud-backend/internal/core/agentruntime/plan_guard.go cloud-backend/internal/core/agentruntime/shot_regeneration_guard_test.go cloud-backend/cmd/tangying-ai-os/main.go
git commit -m "feat: isolate shot regeneration state"
```

---

### Task 3: Add scalable Shot review queries, candidate acceptance, and restore

**Files:**
- Create: `cloud-backend/internal/agents/video/model/creator_view.go`
- Modify: `cloud-backend/internal/agents/video/service/shot_review.go`
- Modify: `cloud-backend/internal/agents/video/service/shot_review_test.go`
- Modify: `cloud-backend/internal/agents/video/handler/creation_handler.go`
- Modify: `cloud-backend/internal/agents/video/handler/creation_handler_test.go`

**Interfaces:**
- Consumes: Task 2 target-isolated state and task records.
- Produces: `ShotPage`, `ShotSummary`, `ShotWorkspace`, `ShotImpact`, `AcceptCandidate`, and `RestoreCandidate` APIs used by the frontend.

- [ ] **Step 1: Add failing pagination and restore tests**

Add tests that construct 100 Shots and assert:

```go
page, err := svc.ListShotPage(ctx, "u-1", "vp-1", model.ShotPageQuery{
	Limit: 24, Status: model.ReviewStatusPending,
})
if err != nil || len(page.Items) != 24 || page.NextCursor == "" {
	t.Fatalf("page = %+v error = %v", page, err)
}

restored, err := svc.RestoreShotCandidate(ctx, "u-1", "vp-1", "shot-001", "candidate-v1", 3)
if err != nil || restored.Version != 4 || restored.AcceptedCandidateID == "candidate-v1" {
	t.Fatalf("restored shot = %+v error = %v", restored, err)
}
```

The restored candidate must have a new ID, copy the historical candidate's artifact references, and leave the historical candidate unchanged.

- [ ] **Step 2: Run and verify failure**

Run: `cd cloud-backend && go test ./internal/agents/video/service -run 'Test(ListShotPage|RestoreShotCandidate)' -count=1`

Expected: FAIL because the query and restore methods do not exist.

- [ ] **Step 3: Define creator-facing Shot DTOs**

Create `model/creator_view.go` with:

```go
type ShotPageQuery struct {
	Cursor  string
	Limit   int
	Status  string
	Chapter string
	Query   string
}

type ShotListItem struct {
	ID                  string `json:"id"`
	SequenceIndex       int    `json:"sequenceIndex"`
	Title               string `json:"title"`
	Chapter             string `json:"chapter,omitempty"`
	DurationSec         int    `json:"durationSec"`
	Version             int    `json:"version"`
	ReviewStatus        string `json:"reviewStatus"`
	QAStatus            string `json:"qaStatus,omitempty"`
	GenerationStatus    string `json:"generationStatus"`
	AcceptedCandidateID string `json:"acceptedCandidateId,omitempty"`
	ThumbnailRef        string `json:"thumbnailRef,omitempty"`
}

type ShotPage struct {
	Items      []ShotListItem `json:"items"`
	NextCursor string         `json:"nextCursor,omitempty"`
	Total      int            `json:"total"`
}

type ShotSummary struct {
	Total          int `json:"total"`
	Confirmed      int `json:"confirmed"`
	AwaitingReview int `json:"awaitingReview"`
	Generating     int `json:"generating"`
	NeedsAction    int `json:"needsAction"`
}

type ShotImpact struct {
	ShotID                  string   `json:"shotId"`
	AffectedShotIDs         []string `json:"affectedShotIds"`
	InvalidatesFinalAssembly bool     `json:"invalidatesFinalAssembly"`
	RegeneratesOtherShots   bool     `json:"regeneratesOtherShots"`
	EstimatedDurationSec    int      `json:"estimatedDurationSec"`
	RequiresConfirmation    bool     `json:"requiresConfirmation"`
}
```

- [ ] **Step 4: Implement page, summary, workspace, accept, and restore**

Use a cursor encoding of the last returned `SequenceIndex` with `strconv.Itoa`; clamp limit to `1..50`. Search against title, narration, scene summary, and screen text. Candidate acceptance must require matching `baseVersion`, a candidate belonging to the target Shot, and a passed QA or human-review-required status. Restore copies a historical candidate to a new immutable candidate ID, increments the Shot version, accepts the new candidate, and marks assembly dirty.

- [ ] **Step 5: Add REST routes**

Register:

```go
api.GET("/:id/shots/summary", h.GetShotSummary)
api.GET("/:id/shots/:shotId/workspace", h.GetShotWorkspace)
api.GET("/:id/shots/:shotId/history", h.GetShotHistory)
api.POST("/:id/shots/:shotId/regeneration-impact", h.PreviewShotRegeneration)
api.POST("/:id/shots/:shotId/regenerations", h.RegenerateShotV2)
api.POST("/:id/shots/:shotId/candidates/:candidateId/accept", h.AcceptShotCandidate)
api.POST("/:id/shots/:shotId/candidates/:candidateId/restore", h.RestoreShotCandidate)
```

Update `GET /:id/shots` to accept cursor/filter queries and return `{shots, nextCursor, total}`. Read `Idempotency-Key` from the request header. Map `ErrShotVersionConflict` to HTTP 409 and invalid duration/scope/lock to HTTP 400.

- [ ] **Step 6: Add handler contract tests**

Test an authenticated regeneration request with `Idempotency-Key: request-1`, `baseVersion: 3`, and `scope: base_media`; assert the fake service receives every field. Add a 409 response test for `ErrShotVersionConflict`.

- [ ] **Step 7: Run service and handler tests**

Run: `cd cloud-backend && go test ./internal/agents/video/service ./internal/agents/video/handler -run 'Test.*Shot' -count=1`

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add cloud-backend/internal/agents/video/model/creator_view.go cloud-backend/internal/agents/video/service/shot_review.go cloud-backend/internal/agents/video/service/shot_review_test.go cloud-backend/internal/agents/video/handler/creation_handler.go cloud-backend/internal/agents/video/handler/creation_handler_test.go
git commit -m "feat: add scalable shot review api"
```

---

### Task 4: Extract reusable Artifact revision and immutable restore services

**Files:**
- Create: `cloud-backend/internal/core/artifact/revision_service.go`
- Create: `cloud-backend/internal/core/artifact/revision_service_test.go`
- Modify: `cloud-backend/internal/core/artifact/model.go`
- Modify: `cloud-backend/internal/core/artifact/service.go`
- Modify: `cloud-backend/internal/core/artifact/handler.go`
- Test: `cloud-backend/internal/core/artifact/materializer_test.go`

**Interfaces:**
- Consumes: existing Artifact service, handler revision generator, and downstream stale marking.
- Produces: `RevisionService.Revise`, `RevisionService.Restore`, and `RevisionResult` for both legacy Artifact routes and creator step routes.

- [ ] **Step 1: Write failing restore immutability tests**

Create a repository fake and test:

```go
result, err := revisions.Restore(ctx, RestoreRequest{
	ArtifactID: "artifact-v1", ReviewerID: "user-1", Reason: "恢复第一版",
})
if err != nil {
	t.Fatalf("Restore error: %v", err)
}
if result.Artifact.Version != 4 || result.Artifact.ParentID != "artifact-v3" {
	t.Fatalf("restored artifact = %+v", result.Artifact)
}
if result.Artifact.Metadata["restoredFromArtifactId"] != "artifact-v1" {
	t.Fatalf("metadata = %+v", result.Artifact.Metadata)
}
if !reflect.DeepEqual(repo.byID["artifact-v1"], originalV1) {
	t.Fatal("historical artifact was mutated")
}
```

- [ ] **Step 2: Run and verify failure**

Run: `cd cloud-backend && go test ./internal/core/artifact -run 'TestRevisionService' -count=1`

Expected: FAIL because `RevisionService` does not exist.

- [ ] **Step 3: Add explicit forced-version creation**

Extend `CreateArtifactRequest`:

```go
ForceNewVersion bool
RestoredFromID  string
```

Skip content-hash deduplication only when `ForceNewVersion` is true. Preserve the real content hash and storage reference. Record `restoredFromArtifactId` in metadata.

- [ ] **Step 4: Implement `RevisionService`**

Define:

```go
type RevisionGenerator func(context.Context, string, string, ReviseLLMOptions) (string, error)

type RevisionResult struct {
	Artifact           *Artifact `json:"artifact"`
	StaleStageNames    []string  `json:"staleStageNames"`
}

type ReviseRequest struct {
	ArtifactID     string
	Message        string
	DirectContent  []byte
	ModelProviders map[string]interface{}
}
```

Move original-content resolution, prompt creation, version creation, and downstream stale calls from `Handler.ReviseArtifact` into this service. `DirectContent` bypasses the LLM but still creates a version and marks downstream stale. `Restore` creates a forced version from the historical artifact, based on the current artifact as parent.

- [ ] **Step 5: Make the legacy handler delegate**

Keep request and response JSON backward compatible. `SetRevisionConfig` sets the generator on the shared service. Remove duplicate revision logic from the handler.

- [ ] **Step 6: Run Artifact tests**

Run: `cd cloud-backend && go test ./internal/core/artifact -count=1`

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add cloud-backend/internal/core/artifact/revision_service.go cloud-backend/internal/core/artifact/revision_service_test.go cloud-backend/internal/core/artifact/model.go cloud-backend/internal/core/artifact/service.go cloud-backend/internal/core/artifact/handler.go cloud-backend/internal/core/artifact/materializer_test.go
git commit -m "refactor: share artifact revision service"
```

---

### Task 5: Build the six-step creator view read model

**Files:**
- Modify: `cloud-backend/internal/agents/video/model/creator_view.go`
- Create: `cloud-backend/internal/agents/video/service/creator_view.go`
- Create: `cloud-backend/internal/agents/video/service/creator_view_test.go`
- Create: `cloud-backend/internal/agents/video/handler/creator_view_handler.go`
- Create: `cloud-backend/internal/agents/video/handler/creator_view_handler_test.go`
- Modify: `cloud-backend/cmd/tangying-ai-os/main.go`

**Interfaces:**
- Consumes: Project service, Creation service, Artifact `ListCurrentByProject`, and Shot summary.
- Produces: `GET /api/video-projects/:id/creation-view` and creator step version/history reads.

- [ ] **Step 1: Write step-mapping tests**

Test a project with an approved script artifact, a stale storyboard artifact, 31 confirmed Shots, and one failed Shot. Assert exactly six steps and these states:

```go
want := []model.CreatorStepState{
	model.CreatorStepConfirmed,
	model.CreatorStepConfirmed,
	model.CreatorStepConfirmed,
	model.CreatorStepNeedsAttention,
	model.CreatorStepNotStarted,
	model.CreatorStepNotStarted,
}
if got := stepStates(view.Steps); !reflect.DeepEqual(got, want) {
	t.Fatalf("step states = %v, want %v", got, want)
}
```

- [ ] **Step 2: Run and verify failure**

Run: `cd cloud-backend && go test ./internal/agents/video/service -run 'TestCreatorView' -count=1`

Expected: FAIL because creator view types and service do not exist.

- [ ] **Step 3: Define stable creator step types**

Add:

```go
type CreatorStepID string
type CreatorStepState string

const (
	CreatorStepRequirements CreatorStepID = "requirements"
	CreatorStepDirection    CreatorStepID = "direction"
	CreatorStepScript       CreatorStepID = "script"
	CreatorStepShots        CreatorStepID = "shots"
	CreatorStepPreview      CreatorStepID = "preview"
	CreatorStepDelivery     CreatorStepID = "delivery"

	CreatorStepNotStarted     CreatorStepState = "not_started"
	CreatorStepGenerating     CreatorStepState = "generating"
	CreatorStepNeedsReview    CreatorStepState = "needs_review"
	CreatorStepConfirmed      CreatorStepState = "confirmed"
	CreatorStepNeedsAttention CreatorStepState = "needs_attention"
	CreatorStepFailed         CreatorStepState = "failed"
)

type CreatorStep struct {
	ID                CreatorStepID    `json:"id"`
	Label             string           `json:"label"`
	State             CreatorStepState `json:"state"`
	CurrentArtifactID string           `json:"currentArtifactId,omitempty"`
	CurrentVersion    int              `json:"currentVersion,omitempty"`
	ReviewID          string           `json:"reviewId,omitempty"`
	RunID             string           `json:"runId,omitempty"`
	AllowedActions    []string         `json:"allowedActions"`
}

type CreationView struct {
	Project     *VideoProject `json:"project"`
	ActiveStep  CreatorStepID `json:"activeStep"`
	Steps       []CreatorStep `json:"steps"`
	ShotSummary ShotSummary   `json:"shotSummary"`
	ActiveTasks []CreatorTask `json:"activeTasks"`
	AssemblyDirty bool         `json:"assemblyDirty"`
}

type CreatorTask struct {
	ID      string `json:"id"`
	Scope   string `json:"scope"`
	ShotID  string `json:"shotId,omitempty"`
	Status  string `json:"status"`
	Label   string `json:"label"`
}
```

- [ ] **Step 4: Implement explicit stage mapping**

Use a fixed map from normalized internal stage names to creator steps. Unknown stages never create a seventh step; they remain visible in the developer console. State priority is `failed > needs_attention > needs_review > generating > confirmed > not_started`. Shot step state comes from Shot summary and assembly dirtiness, not from client calculation.

- [ ] **Step 5: Add the authenticated view route**

Create `CreatorViewHandler.GetCreationView`, authenticate with the same middleware as project routes, verify project ownership through `ProjectService.GetProject`, and return `httpx.OK(view)`.

- [ ] **Step 6: Wire after Artifact service construction**

In `main.go`, construct creator view service after `artifactSvc` exists, passing project, creation, artifact, and Shot readers. Register the handler once.

- [ ] **Step 7: Run service and handler tests**

Run: `cd cloud-backend && go test ./internal/agents/video/service ./internal/agents/video/handler -run 'TestCreatorView' -count=1`

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add cloud-backend/internal/agents/video/model/creator_view.go cloud-backend/internal/agents/video/service/creator_view.go cloud-backend/internal/agents/video/service/creator_view_test.go cloud-backend/internal/agents/video/handler/creator_view_handler.go cloud-backend/internal/agents/video/handler/creator_view_handler_test.go cloud-backend/cmd/tangying-ai-os/main.go
git commit -m "feat: add creator step read model"
```

---

### Task 6: Add creator step impact, revision, confirmation, and restore endpoints

**Files:**
- Modify: `cloud-backend/internal/agents/video/model/creator_view.go`
- Modify: `cloud-backend/internal/agents/video/service/creator_view.go`
- Modify: `cloud-backend/internal/agents/video/service/creator_view_test.go`
- Modify: `cloud-backend/internal/agents/video/handler/creator_view_handler.go`
- Modify: `cloud-backend/internal/agents/video/handler/creator_view_handler_test.go`
- Modify: `cloud-backend/internal/core/agentruntime/handler.go`
- Create: `cloud-backend/internal/core/agentruntime/review_mutation_service.go`
- Create: `cloud-backend/internal/core/agentruntime/review_mutation_service_test.go`

**Interfaces:**
- Consumes: Task 4 `RevisionService`, Task 5 current Artifact/Review IDs, and existing Agent review mutation logic.
- Produces: user-level impact, revision, confirm, and restore endpoints whose responses include the refreshed six-step view.

- [ ] **Step 1: Write failing confirmed-step revision test**

Test a confirmed script step revision with `mode=direct`, `baseVersion=3`, and content. Assert a version `4` Artifact, downstream steps `shots`, `preview`, `delivery` in the impact, and the script review reset for re-review.

```go
result, err := svc.ReviseStep(ctx, "user-1", "vp-1", model.CreatorStepScript, model.StepRevisionRequest{
	ArtifactID: "script-v3", BaseVersion: 3, Mode: "direct",
	DirectContent: "新版脚本", RunID: "run-1", ReviewID: "script-review",
})
if err != nil || result.View.Steps[2].CurrentVersion != 4 {
	t.Fatalf("result = %+v error = %v", result, err)
}
```

- [ ] **Step 2: Run and verify failure**

Run: `cd cloud-backend && go test ./internal/agents/video/service ./internal/core/agentruntime -run 'Test.*StepRevision' -count=1`

Expected: FAIL because step mutations and shared review mutation service do not exist.

- [ ] **Step 3: Extract review mutation behavior from the HTTP handler**

Move the state-machine operations used by approve, submit-edited, and regenerate into `ReviewMutationService`. Keep handler responses and status codes unchanged by making the existing handlers call the new service. Export:

```go
type ReviewMutationService interface {
	Confirm(ctx context.Context, runID, reviewID, reviewerID, comment string) error
	ReopenWithArtifact(ctx context.Context, runID, reviewID, artifactID, reviewerID, reason string) error
}
```

`ReopenWithArtifact` updates the review gate to reference the new Artifact and returns that gate to `READY` without resetting or rerunning the already-successful source execution node. It writes the decision log and does not modify unrelated review gates. Confirming the reopened gate is the only action that resumes downstream execution.

- [ ] **Step 4: Define step mutation contracts**

```go
type StepRevisionRequest struct {
	ArtifactID    string `json:"artifactId"`
	BaseVersion   int    `json:"baseVersion"`
	Mode          string `json:"mode"`
	Instruction   string `json:"instruction,omitempty"`
	DirectContent string `json:"directContent,omitempty"`
	RunID         string `json:"runId"`
	ReviewID      string `json:"reviewId"`
	ConfirmedAffectedShotIDs []string           `json:"confirmedAffectedShotIds,omitempty"`
	Selection                *ArtifactSelection `json:"selection,omitempty"`
}

type ArtifactSelection struct {
	Kind    string   `json:"kind"`
	X       *float64 `json:"x,omitempty"`
	Y       *float64 `json:"y,omitempty"`
	Width   *float64 `json:"width,omitempty"`
	Height  *float64 `json:"height,omitempty"`
	StartMs *int64   `json:"startMs,omitempty"`
	EndMs   *int64   `json:"endMs,omitempty"`
}

type StepImpact struct {
	AffectedStepIDs []CreatorStepID `json:"affectedStepIds"`
	AffectedShotIDs []string        `json:"affectedShotIds,omitempty"`
	RequiresConfirmation bool       `json:"requiresConfirmation"`
}
```

Reject stale `BaseVersion` with HTTP 409. Permit modes `instruction` and `direct` only. Project-level script, character, audio-master, or style changes must return exact `AffectedShotIDs`; the revision endpoint rejects the mutation unless the request echoes those IDs as the confirmed impact set.

- [ ] **Step 5: Register creator step routes**

```go
api.GET("/:id/steps/:stepId/versions", h.GetStepVersions)
api.POST("/:id/steps/:stepId/revision-impact", h.PreviewStepRevision)
api.POST("/:id/steps/:stepId/revisions", h.ReviseStep)
api.POST("/:id/steps/:stepId/confirm", h.ConfirmStep)
api.POST("/:id/steps/:stepId/versions/:version/restore", h.RestoreStepVersion)
```

Revision and restore responses contain `{artifact, impact, view}` so the frontend never guesses the next state.

- [ ] **Step 6: Run review, service, and handler tests**

Run: `cd cloud-backend && go test ./internal/core/agentruntime ./internal/agents/video/service ./internal/agents/video/handler -run 'Test.*(ReviewMutation|StepRevision|StepRestore|ConfirmStep)' -count=1`

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add cloud-backend/internal/agents/video/model/creator_view.go cloud-backend/internal/agents/video/service/creator_view.go cloud-backend/internal/agents/video/service/creator_view_test.go cloud-backend/internal/agents/video/handler/creator_view_handler.go cloud-backend/internal/agents/video/handler/creator_view_handler_test.go cloud-backend/internal/core/agentruntime/handler.go cloud-backend/internal/core/agentruntime/review_mutation_service.go cloud-backend/internal/core/agentruntime/review_mutation_service_test.go
git commit -m "feat: add creator step mutations"
```

---

### Task 7: Register optional project source materials

**Files:**
- Modify: `cloud-backend/internal/agents/video/assets/handler.go`
- Modify: `cloud-backend/internal/agents/video/assets/handler_test.go`
- Modify: `cloud-backend/internal/agents/video/assets/manifest.go`

**Interfaces:**
- Consumes: existing local-agent storage references and Artifact service.
- Produces: `POST /api/video-projects/:id/materials` for image, video, audio, and document metadata registration.

- [ ] **Step 1: Write a failing material registration test**

Post this authenticated request and assert an Artifact with `stageName=requirements`, `unitId=source-materials`, `relatedProjectId=vp-1`, and no cloud payload:

```json
{
  "name": "采访录音.wav",
  "kind": "audio",
  "storageRef": "local://vp-1/materials/interview-audio",
  "mimeType": "audio/wav",
  "sizeBytes": 102400,
  "contentHash": "sha256:audio-1"
}
```

- [ ] **Step 2: Run and verify failure**

Run: `cd cloud-backend && go test ./internal/agents/video/assets -run 'Test.*ProjectMaterial' -count=1`

Expected: FAIL with route not found.

- [ ] **Step 3: Implement material registration**

Accept only `image`, `video`, `audio`, and `document`. Require `storageRef` to start with `local://`, require name, MIME type, non-negative size, and content hash. Store metadata only through Artifact service and return `{material, artifact}`.

- [ ] **Step 4: Run tests**

Run: `cd cloud-backend && go test ./internal/agents/video/assets -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cloud-backend/internal/agents/video/assets/handler.go cloud-backend/internal/agents/video/assets/handler_test.go cloud-backend/internal/agents/video/assets/manifest.go
git commit -m "feat: register project source materials"
```

---

### Task 8: Publish API contracts and build the typed frontend client

**Files:**
- Modify: `cloud-backend/internal/core/apispec/cloud_spec.go`
- Modify: `cloud-backend/internal/core/apispec/cloud_spec_test.go`
- Create: `frontend/src/features/creator-studio/types.ts`
- Create: `frontend/src/features/creator-studio/logic.ts`
- Create: `frontend/src/services/creatorApi.ts`
- Create: `frontend/scripts/creator-studio-logic-check.mjs`
- Modify: `frontend/package.json`

**Interfaces:**
- Consumes: Tasks 3, 5, 6, and 7 endpoint JSON.
- Produces: TypeScript types and API functions consumed by every creator component.

- [ ] **Step 1: Require every new route in the OpenAPI test**

Add exact method/path expectations for creation view, step versions/impact/revision/confirm/restore, materials, Shot summary/page/workspace/history/impact/regeneration, and candidate accept/restore.

- [ ] **Step 2: Run the API spec test and verify failure**

Run: `cd cloud-backend && go test ./internal/core/apispec -run TestCloudSpec -count=1`

Expected: FAIL listing missing creator routes.

- [ ] **Step 3: Document the routes and regenerate generated types**

Add request/response schemas and run: `cd cloud-backend && make gen-docs`

Expected: OpenAPI output and `frontend/src/utils/api-types.generated.ts` update without drift.

- [ ] **Step 4: Add frontend types and pure selectors**

Define discriminated unions matching the Go constants. Implement:

```ts
export function nextCreatorAction(view: CreationView): CreatorAction {
  const step = view.steps.find(item =>
    ['failed', 'needs_attention', 'needs_review', 'generating'].includes(item.state),
  ) ?? view.steps.find(item => item.state === 'not_started') ?? view.steps.at(-1)
  if (!step) return { kind: 'start', stepId: 'requirements', label: '开始创作' }
  if (step.state === 'needs_review') return { kind: 'review', stepId: step.id, label: `审核${step.label}` }
  if (step.state === 'generating') return { kind: 'wait', stepId: step.id, label: `${step.label}生成中` }
  if (step.state === 'failed' || step.state === 'needs_attention') return { kind: 'fix', stepId: step.id, label: `处理${step.label}` }
  return { kind: 'continue', stepId: step.id, label: `继续${step.label}` }
}

export function canSubmitShotDuration(durationSec: number): boolean {
  return durationSec > 0 && durationSec < 15
}
```

- [ ] **Step 5: Add the esbuild logic test**

Bundle `logic.ts`, create 100 Shot fixtures, and assert filtering, stable sort, action labels, `<15` validation, conflict copy, and target-only regeneration impact. Add `"test:creator": "node scripts/creator-studio-logic-check.mjs"`.

- [ ] **Step 6: Implement `creatorApi.ts`**

Export one function per API route with typed return values. Include `registerProjectMaterial(projectId, material)`. Mutation functions must accept `baseVersion`; Shot regeneration must require an idempotency key and send it as `Idempotency-Key`.

- [ ] **Step 7: Run backend and frontend contract tests**

Run: `cd cloud-backend && go test ./internal/core/apispec -count=1 && cd ../frontend && npm run test:creator && npm run build`

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add cloud-backend/internal/core/apispec frontend/src/utils/api-types.generated.ts frontend/src/features/creator-studio/types.ts frontend/src/features/creator-studio/logic.ts frontend/src/services/creatorApi.ts frontend/scripts/creator-studio-logic-check.mjs frontend/package.json
git commit -m "feat: add creator studio api contracts"
```

---

### Task 9: Separate creator routing from the developer console

**Files:**
- Create: `frontend/src/features/creator-studio/CreatorShell.tsx`
- Create: `frontend/src/features/developer-console/DeveloperConsolePage.tsx`
- Modify: `frontend/src/App.tsx`
- Modify: `frontend/src/pages/DirectorStudioPage.tsx`
- Modify: `frontend/src/index.css`
- Modify: `frontend/scripts/director-studio-logic-check.mjs`

**Interfaces:**
- Consumes: authenticated user and existing `DirectorStudioPage` development views.
- Produces: hash routes `#/create`, `#/videos`, `#/videos/:projectId/steps/:stepId`, and gated `#/developer/*`.

- [ ] **Step 1: Add failing source-boundary assertions**

In `director-studio-logic-check.mjs`, read `CreatorShell.tsx` and assert it contains `开始创作` and `我的视频` but does not contain `追踪`, `角色`, `原始产物`, `Provider`, or `Run`. Assert `DeveloperConsolePage.tsx` imports `DirectorStudioPage`.

- [ ] **Step 2: Run and verify failure**

Run: `cd frontend && npm run test:director`

Expected: FAIL because the new shell files do not exist.

- [ ] **Step 3: Implement a small hash router without a new dependency**

Use `window.location.hash`, a `hashchange` listener, and exact route parsing in `App.tsx`. Default authenticated users to `#/create`. Gate `#/developer/*` with:

```ts
const developerConsoleEnabled = import.meta.env.DEV || import.meta.env.VITE_ENABLE_DEVELOPER_CONSOLE === '1'
```

When disabled, replace the hash with `#/create`. This avoids Electron deep-link and server fallback changes.

- [ ] **Step 4: Build the creator and developer shells**

Creator persistent navigation has two buttons only. Profile menu contains logout and a contextual connection/settings action. `DeveloperConsolePage` renders the existing page and its seven development destinations without changing their internals.

- [ ] **Step 5: Preserve current visual tokens**

Add only `.creator-*` layout classes derived from the existing color values. Do not change `tailwind.config.js` tokens or current `.card`, `.glass`, and status colors.

- [ ] **Step 6: Run logic, lint, and build**

Run: `cd frontend && npm run test:director && npm run test:creator && npm run lint && npm run build`

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add frontend/src/features/creator-studio/CreatorShell.tsx frontend/src/features/developer-console/DeveloperConsolePage.tsx frontend/src/App.tsx frontend/src/pages/DirectorStudioPage.tsx frontend/src/index.css frontend/scripts/director-studio-logic-check.mjs
git commit -m "feat: separate creator and developer surfaces"
```

---

### Task 10: Build the one-sentence start page and video library

**Files:**
- Create: `frontend/src/features/creator-studio/StartCreationPage.tsx`
- Create: `frontend/src/features/creator-studio/VideoLibraryPage.tsx`
- Modify: `frontend/src/features/creator-studio/CreatorShell.tsx`
- Modify: `frontend/src/services/creatorApi.ts`
- Modify: `frontend/scripts/creator-studio-logic-check.mjs`

**Interfaces:**
- Consumes: existing `createVideoProject`, `startAgentRun`, local material upload, Task 7 material registration, and Task 9 hash routing.
- Produces: real project creation, material registration, recent-project continuation, and navigation into the project workspace.

- [ ] **Step 1: Add failing creation-flow logic tests**

Extract and test `buildCreationRequest` so a prompt, optional duration, aspect ratio, and material count map to the existing project/run context without Provider, Run, Trace, or Artifact terms in user copy.

- [ ] **Step 2: Run and verify failure**

Run: `cd frontend && npm run test:creator`

Expected: FAIL because `buildCreationRequest` is absent.

- [ ] **Step 3: Implement the start page**

The page contains one textarea, `添加素材`, progressive options for duration/aspect/platform, and `开始创作`. After project creation, upload materials sequentially to the local agent, register each with the cloud, start the Agent run, and navigate to `#/videos/:id/steps/requirements`. Show per-file success/failure and allow retry without recreating the project.

- [ ] **Step 4: Implement the video library**

Use `fetchVideoProjects`; group active before completed/archived, show human labels, last updated time, progress from `creation-view`, and one `继续创作` action. Do not show run IDs or pipeline names.

- [ ] **Step 5: Run frontend verification**

Run: `cd frontend && npm run test:creator && npm run lint && npm run build`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/features/creator-studio/StartCreationPage.tsx frontend/src/features/creator-studio/VideoLibraryPage.tsx frontend/src/features/creator-studio/CreatorShell.tsx frontend/src/services/creatorApi.ts frontend/src/features/creator-studio/logic.ts frontend/scripts/creator-studio-logic-check.mjs
git commit -m "feat: add simple video creation entry"
```

---

### Task 11: Build the six-step workspace and versioned artifact review

**Files:**
- Create: `frontend/src/features/creator-studio/ProjectWorkspacePage.tsx`
- Create: `frontend/src/features/creator-studio/components/CreationStrip.tsx`
- Create: `frontend/src/features/creator-studio/components/ArtifactReviewPanel.tsx`
- Create: `frontend/src/features/creator-studio/components/TaskRecoveryBanner.tsx`
- Modify: `frontend/src/features/creator-studio/CreatorShell.tsx`
- Modify: `frontend/src/services/creatorApi.ts`
- Modify: `frontend/scripts/creator-studio-logic-check.mjs`

**Interfaces:**
- Consumes: Task 5 creation view and Task 6 impact/revision/confirm/restore APIs.
- Produces: replayable steps, current artifact review, AI/direct revisions, impact confirmation, version history, and reconnect recovery.

- [ ] **Step 1: Add failing step and impact selectors**

Test that confirmed steps remain clickable, a stale downstream step remains readable but cannot confirm, and a script revision impact renders `分镜与素材、成片预览、交付需要更新`.

- [ ] **Step 2: Run and verify failure**

Run: `cd frontend && npm run test:creator`

Expected: FAIL because the step review selectors and components do not exist.

- [ ] **Step 3: Implement `CreationStrip`**

Render exactly six ordered buttons. Use text plus icon for status, visible keyboard focus, `aria-current="step"`, and reduced-motion-safe transitions. Clicking a confirmed step changes the route without mutating backend state.

- [ ] **Step 4: Implement `ArtifactReviewPanel`**

Load current content and versions. Provide `确认并继续`, `告诉 AI 怎么改`, `直接编辑` for text, and `查看版本`. Revision first calls impact, renders the exact affected steps and Shot IDs, and requires confirmation. Restore calls the restore endpoint and renders the returned view. Image review serializes a normalized rectangular selection; video review serializes `startMs` and `endMs` for a time-coded comment. Both selections are included in the revision request rather than flattened into display text.

- [ ] **Step 5: Implement task recovery**

Refetch `creation-view` with exponential intervals capped at 5 seconds while `ActiveTasks` is non-empty and the document is visible; refetch the selected Shot workspace when its task changes. On remount, load `ActiveTasks` from the backend instead of reusing client-only state. Show `生成仍在后台继续` during reconnect.

- [ ] **Step 6: Run frontend verification**

Run: `cd frontend && npm run test:creator && npm run lint && npm run build`

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add frontend/src/features/creator-studio/ProjectWorkspacePage.tsx frontend/src/features/creator-studio/components/CreationStrip.tsx frontend/src/features/creator-studio/components/ArtifactReviewPanel.tsx frontend/src/features/creator-studio/components/TaskRecoveryBanner.tsx frontend/src/features/creator-studio/CreatorShell.tsx frontend/src/services/creatorApi.ts frontend/src/features/creator-studio/logic.ts frontend/scripts/creator-studio-logic-check.mjs
git commit -m "feat: add versioned creator workspace"
```

---

### Task 12: Build the long-video Shot review queue and inspector

**Files:**
- Create: `frontend/src/features/creator-studio/components/ShotReviewQueue.tsx`
- Create: `frontend/src/features/creator-studio/components/ShotInspector.tsx`
- Create: `frontend/src/features/creator-studio/components/ShotImprovePanel.tsx`
- Modify: `frontend/src/features/creator-studio/ProjectWorkspacePage.tsx`
- Modify: `frontend/src/features/creator-studio/logic.ts`
- Modify: `frontend/scripts/creator-studio-logic-check.mjs`

**Interfaces:**
- Consumes: Task 3 page/summary/workspace/history/impact/regeneration/candidate APIs.
- Produces: scalable Needs-my-attention queue, one mounted player, candidate review, local regeneration, locks, and uninterrupted parallel review.

- [ ] **Step 1: Add a 100-Shot rendering-model test**

Test pure windowing output so 100 Shot items with a 480px viewport and 64px rows return no more than 12 visible rows including overscan. Assert selected Shot persists across page append and filters default to `needs_attention`.

- [ ] **Step 2: Run and verify failure**

Run: `cd frontend && npm run test:creator`

Expected: FAIL because the windowing selectors do not exist.

- [ ] **Step 3: Implement the paged review queue**

Use fixed-height rows and manual windowing to avoid adding a dependency. Fetch 24 items per page, prefetch when the scroll window reaches the final 6 items, group visible rows by chapter, and mount thumbnails only. Default status filter is `needs_attention`; include `all`, `confirmed`, `generating`, and `failed`.

- [ ] **Step 4: Implement the Shot inspector**

Mount one video player for the selected candidate. Render candidate tabs, QA summary in user language, narration, time range, references, and `前后镜头`. Accept requires `baseVersion` and an explicit candidate ID.

- [ ] **Step 5: Implement local improvement controls**

Show six scopes and field locks. Call impact before generation. The confirmation must state `只会新增 Shot N 的候选，不影响其他 X 个 Shot` when `regeneratesOtherShots=false`. Send a UUID idempotency key and continue allowing queue selection while the task runs.

- [ ] **Step 6: Handle conflict and failure**

HTTP 409 reloads the selected Shot and displays `这个 Shot 已有更新，请基于最新版本重试`. A failed task remains scoped to its Shot and offers `重试这个 Shot`; it never clears queue state.

- [ ] **Step 7: Run frontend verification**

Run: `cd frontend && npm run test:creator && npm run lint && npm run build`

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add frontend/src/features/creator-studio/components/ShotReviewQueue.tsx frontend/src/features/creator-studio/components/ShotInspector.tsx frontend/src/features/creator-studio/components/ShotImprovePanel.tsx frontend/src/features/creator-studio/ProjectWorkspacePage.tsx frontend/src/features/creator-studio/logic.ts frontend/scripts/creator-studio-logic-check.mjs
git commit -m "feat: add long video shot review queue"
```

---

### Task 13: Complete preview/delivery, harden integration, and switch the default route

**Files:**
- Modify: `frontend/src/features/creator-studio/ProjectWorkspacePage.tsx`
- Create: `frontend/src/features/creator-studio/components/PreviewDeliveryPanel.tsx`
- Modify: `frontend/src/App.tsx`
- Modify: `frontend/src/index.css`
- Modify: `frontend/scripts/creator-studio-logic-check.mjs`
- Modify: `scripts/beta-smoke-check.sh`
- Modify: `scripts/beta-readiness-check.sh`
- Modify: `README.md`
- Modify: `CHANGELOG.md`
- Create: `cloud-backend/internal/agents/video/service/creator_studio_integration_test.go`

**Interfaces:**
- Consumes: all prior backend and frontend tasks.
- Produces: final preview/delivery steps, default creator route, developer-console regression coverage, and full readiness evidence.

- [ ] **Step 1: Add final-preview and delivery assertions**

Test that preview consumes only accepted current candidates, assembly dirty blocks final confirmation without blocking Shot review, assembly retry does not submit Shot regeneration, and Delivery exposes final video/package only after final QA.

- [ ] **Step 2: Run and verify failure**

Run: `cd frontend && npm run test:creator`

Expected: FAIL until preview and delivery actions use the creator view contract.

- [ ] **Step 3: Implement preview and delivery states**

Reuse current large video preview, caption/audio review, export, and publishing-copy components behind creator copy. Remove provenance and raw file indexes from creator routes. Keep assembly retry separate from any Shot regeneration request.

- [ ] **Step 4: Add backend 100-Shot integration coverage**

Create a service integration test that regenerates Shot 12, accepts its new candidate, rebuilds assembly, and compares serialized Shots 1–11 and 13–100 before and after. Only Shot 12 and `AssemblyDirty` may differ before assembly; after assembly, other Shots still match.

- [ ] **Step 5: Extend smoke and readiness checks**

Add checks for authenticated creation view, 15-second rejection, paged 100-Shot fixture, idempotent target regeneration, candidate acceptance, assembly, creator build, and developer-console gate.

- [ ] **Step 6: Run full verification**

Run:

```bash
cd cloud-backend && go test ./...
cd ../local-backend && go test ./...
cd ../frontend && npm run test:director && npm run test:creator && npm run lint && npm run build
cd .. && bash scripts/beta-smoke-check.sh
```

Expected: all commands PASS. Smoke output must show the creator view and target-only Shot regeneration checks as passed.

- [ ] **Step 7: Perform visual and responsive review**

Start the backend/local agent/frontend, capture creator pages at 1440px, 1024px, and 390px widths, and verify the palette values match Global Constraints, keyboard focus is visible, status is not color-only, only one Shot player mounts, and the developer terms are absent from creator routes.

- [ ] **Step 8: Update user-facing documentation**

Document the two-item creator navigation, six steps, independent developer console, strict `<15s` Shot rule, Shot isolation, version restore, and commands used for verification. Record the change in `CHANGELOG.md` without describing unverified provider quality.

- [ ] **Step 9: Commit**

```bash
git add frontend/src/features/creator-studio/ProjectWorkspacePage.tsx frontend/src/features/creator-studio/components/PreviewDeliveryPanel.tsx frontend/src/App.tsx frontend/src/index.css frontend/scripts/creator-studio-logic-check.mjs scripts/beta-smoke-check.sh scripts/beta-readiness-check.sh README.md CHANGELOG.md cloud-backend/internal/agents/video/service/creator_studio_integration_test.go
git commit -m "feat: make creator studio the default experience"
```

---

## Final verification checklist

- [ ] Creator navigation has exactly two persistent destinations.
- [ ] `#/developer/*` retains all current development views and is absent when the production flag is off.
- [ ] All six creator steps come from one backend `creation-view` response.
- [ ] Confirmed step revision and restore create new immutable versions.
- [ ] Step impact is shown before any downstream invalidation.
- [ ] A 15.0-second Shot is rejected and a 14.0-second Shot is accepted.
- [ ] A 100-Shot project remains responsive and mounts one full video player.
- [ ] Regenerating Shot 12 changes no other Shot.
- [ ] Duplicate regeneration submissions return the same durable task.
- [ ] Candidate restore creates a new candidate and preserves old candidates.
- [ ] Final assembly consumes one accepted current candidate per Shot.
- [ ] Existing frontend palette tokens and development diagnostics remain intact.
- [ ] Full Go, frontend, smoke, lint, and build suites pass.
