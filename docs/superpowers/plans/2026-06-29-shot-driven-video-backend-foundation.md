# Shot-Driven Video Backend Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the first backend foundation slice for shot-driven video creation: spec defaults, shot/visual-plan models, deterministic render strategy, validators, project-config persistence, shot actions, stale graph helpers, and thin HTTP/API contracts.

**Architecture:** Keep video-specific logic in `cloud-backend/internal/agents/video`. Persist first-slice `VideoCreationSpec` and `ShotUnit[]` inside `VideoProject.Config` under a versioned `shotDrivenState` object to avoid new database migrations. Extend generic artifact stale helpers only through additive stage constants/functions, keeping legacy stage behavior unchanged.

**Tech Stack:** Go, Gin, existing `VideoProject` repository/service, existing `artifact.Artifact` model and stale helpers, generated OpenAPI docs/types.

---

## File Structure

- Create `cloud-backend/internal/agents/video/model/creation.go`: shot-driven domain structs, constants, defaulting helpers, and state container.
- Create `cloud-backend/internal/agents/video/service/render_strategy.go`: deterministic render strategy decider.
- Create `cloud-backend/internal/agents/video/service/validators.go`: deterministic validators/checkers.
- Create `cloud-backend/internal/agents/video/service/creation_state.go`: encode/decode `shotDrivenState` in `VideoProject.Config`.
- Create `cloud-backend/internal/agents/video/service/creation_service.go`: spec, shot, lock, reject, regenerate, visual-plan, text-layer, strategy, assemble, and package service methods.
- Create `cloud-backend/internal/agents/video/handler/creation_handler.go`: HTTP routes for spec and shot foundation endpoints.
- Modify `cloud-backend/cmd/tangying-ai-os/main.go`: register the new creation handler after project routes.
- Modify `cloud-backend/internal/core/artifact/stale.go`: add shot-aware stale graph helpers without changing legacy functions.
- Modify `cloud-backend/internal/core/apispec/cloud_spec.go`: add API routes and schemas for the new endpoints.
- Regenerate `cloud-backend/docs/API_REFERENCE.md` and `frontend/src/utils/api-types.generated.ts` if API routes are added.

## Task 1: Models, Defaults, Render Strategy, And Validators

**Files:**
- Create: `cloud-backend/internal/agents/video/model/creation.go`
- Create: `cloud-backend/internal/agents/video/service/render_strategy.go`
- Create: `cloud-backend/internal/agents/video/service/validators.go`
- Test: `cloud-backend/internal/agents/video/service/creation_foundation_test.go`

- [ ] **Step 1: Write failing model/default and validator tests**

Add tests like:

```go
func TestVideoCreationSpecDefaults(t *testing.T) {
	spec := model.NewVideoCreationSpec("vp-1", "做一个60秒口播视频")

	if spec.ProjectID != "vp-1" {
		t.Fatalf("ProjectID = %q, want vp-1", spec.ProjectID)
	}
	if spec.ShotPolicy.MinDurationSec != 3 || spec.ShotPolicy.MaxDurationSec != 15 || spec.ShotPolicy.PreferDurationSec != 6 {
		t.Fatalf("shot policy defaults = %+v", spec.ShotPolicy)
	}
	if !spec.ShotPolicy.SingleSceneRequired || !spec.ShotPolicy.LowVisualChangeRequired || !spec.ShotPolicy.AvoidCrossShotDependency {
		t.Fatalf("shot policy booleans = %+v", spec.ShotPolicy)
	}
	if spec.RenderPreference.DefaultRenderStrategy != model.RenderStrategyAuto {
		t.Fatalf("default strategy = %q", spec.RenderPreference.DefaultRenderStrategy)
	}
	if !spec.RenderPreference.PreferHTMLForText || !spec.RenderPreference.AllowHybridRender || !spec.RenderPreference.PreferLowCostPreview {
		t.Fatalf("render preference defaults = %+v", spec.RenderPreference)
	}
}

func TestShotDurationCheckerRejectsOutside3To15(t *testing.T) {
	for _, tc := range []struct {
		name string
		sec  int
	}{
		{name: "under", sec: 2},
		{name: "over", sec: 16},
	} {
		t.Run(tc.name, func(t *testing.T) {
			issues := service.CheckShotDuration(model.ShotUnit{ID: "shot-1", DurationSec: tc.sec})
			if len(issues) == 0 {
				t.Fatalf("expected duration issue for %d seconds", tc.sec)
			}
		})
	}
}

func TestRenderStrategyExactTextUsesHybridWithAIGCScene(t *testing.T) {
	shot := model.ShotUnit{ID: "shot-1", DurationSec: 6, VideoType: "口播视频"}
	plan := model.VisualPlan{
		Characters: []model.CharacterVisualSpec{{ID: "host", Motion: "walks through office"}},
		TextLayers: []model.TextLayerSpec{{ID: "txt-1", Text: "几个表格", MustBeExact: true, Role: model.TextRoleKeyword}},
	}
	strategy := service.DecideRenderStrategy(shot, plan, model.DefaultRenderPreference(), service.RenderCapabilities{AIGCAvailable: true, HTMLAvailable: true})
	if strategy.Mode != model.RenderModeHTMLPreviewThenHybrid && strategy.Mode != model.RenderModeHybridAIGCBGHTMLOverlay {
		t.Fatalf("mode = %q, want hybrid or preview-then-hybrid", strategy.Mode)
	}
	if !strategy.HTMLRequired || !strategy.AIGCRequired || !strategy.TextOverlayNeeded || !strategy.NeedsCompositing {
		t.Fatalf("strategy flags = %+v", strategy)
	}
}
```

- [ ] **Step 2: Run tests and verify they fail**

Run:

```bash
cd cloud-backend && go test ./internal/agents/video/service -run 'TestVideoCreationSpecDefaults|TestShotDurationCheckerRejectsOutside3To15|TestRenderStrategyExactTextUsesHybridWithAIGCScene' -count=1
```

Expected: FAIL because `NewVideoCreationSpec`, `CheckShotDuration`, `DecideRenderStrategy`, and new model types do not exist.

- [ ] **Step 3: Add model structs and defaults**

Implement `model/creation.go` with concrete exported structs and constants:

```go
const (
	ReviewModeShotLevel = "shot_level_review"
	RenderStrategyAuto  = "auto"

	RenderModeHTMLOnly              = "html_only"
	RenderModeAIGCOnly              = "aigc_only"
	RenderModeHybridAIGCBGHTMLOverlay = "hybrid_aigc_bg_html_overlay"
	RenderModeHTMLPreviewThenAIGC   = "html_preview_then_aigc"
	RenderModeHTMLPreviewThenHybrid = "html_preview_then_hybrid"

	ReviewStatusPending  = "pending"
	ReviewStatusApproved = "approved"
	ReviewStatusRejected = "rejected"
	ReviewStatusStale    = "stale"
)

type VideoCreationSpec struct {
	ID                string           `json:"id"`
	ProjectID         string           `json:"projectId"`
	SourceMessage     string           `json:"sourceMessage"`
	Topic             string           `json:"topic,omitempty"`
	Platform          string           `json:"platform,omitempty"`
	VideoType         string           `json:"videoType,omitempty"`
	TargetDurationSec int              `json:"targetDurationSec,omitempty"`
	AspectRatio       string           `json:"aspectRatio"`
	Language          string           `json:"language"`
	Audience          string           `json:"audience,omitempty"`
	Tone              string           `json:"tone,omitempty"`
	VisualStyle       string           `json:"visualStyle,omitempty"`
	ReviewMode        string           `json:"reviewMode"`
	ShotPolicy        ShotPolicy       `json:"shotPolicy"`
	RenderPreference  RenderPreference `json:"renderPreference"`
	Status            string           `json:"status"`
	CreatedAt         time.Time        `json:"createdAt"`
	UpdatedAt         time.Time        `json:"updatedAt"`
}
```

Include `NewVideoCreationSpec`, `DefaultShotPolicy`, `DefaultRenderPreference`, `ShotUnit`, `VisualPlan`, `TextLayerSpec`, `RenderStrategy`, `AIGCInputSpec`, `HTMLInputSpec`, `CompositePlan`, `ShotDrivenState`, and minimal visual sub-spec structs used by tests.

- [ ] **Step 4: Add decider and validators**

Implement:

```go
func DecideRenderStrategy(shot model.ShotUnit, plan model.VisualPlan, pref model.RenderPreference, caps RenderCapabilities) model.RenderStrategy
func CheckShotDuration(shot model.ShotUnit) []ValidationIssue
func CheckShotSceneComplexity(shot model.ShotUnit) []ValidationIssue
func CheckTextLayerExactness(shot model.ShotUnit, plan model.VisualPlan, strategy model.RenderStrategy) []ValidationIssue
func CheckRenderStrategy(shot model.ShotUnit, plan model.VisualPlan, strategy model.RenderStrategy) []ValidationIssue
func CheckAIGCPromptNoText(strategy model.RenderStrategy, prompt string) []ValidationIssue
func CheckFinalAssembly(shots []model.ShotUnit) []ValidationIssue
```

Use deterministic text checks: exact text or screen text always sets `HTMLRequired`; character motion, scene motion, emotional performance, or complex camera sets `AIGCRequired`.

- [ ] **Step 5: Run tests and commit**

Run:

```bash
cd cloud-backend && go test ./internal/agents/video/model ./internal/agents/video/service -count=1
```

Expected: PASS.

Commit:

```bash
git add cloud-backend/internal/agents/video/model/creation.go cloud-backend/internal/agents/video/service/render_strategy.go cloud-backend/internal/agents/video/service/validators.go cloud-backend/internal/agents/video/service/creation_foundation_test.go
git commit -m "feat: add shot-driven video foundation models"
```

## Task 2: Project Config Persistence And Shot Actions

**Files:**
- Create: `cloud-backend/internal/agents/video/service/creation_state.go`
- Create: `cloud-backend/internal/agents/video/service/creation_service.go`
- Test: `cloud-backend/internal/agents/video/service/creation_service_test.go`

- [ ] **Step 1: Write failing config/service tests**

Add tests for config persistence and lock semantics:

```go
func TestCreationServiceStoresSpecInProjectConfig(t *testing.T) {
	store := newFakeCreationProjectStore()
	store.project = &model.VideoProject{ID: "vp-1", UserID: "u-1", AspectRatio: "16:9", Language: "zh-CN", TargetDuration: 60}
	svc := service.NewCreationService(store)

	spec, err := svc.UpsertSpec(context.Background(), "u-1", "vp-1", &model.VideoCreationSpec{SourceMessage: "做一个60秒口播视频"})
	if err != nil {
		t.Fatalf("UpsertSpec error: %v", err)
	}
	if spec.TargetDurationSec != 60 {
		t.Fatalf("TargetDurationSec = %d, want 60", spec.TargetDurationSec)
	}
	saved := decodeStateFromTest(t, store.updated.Config)
	if saved.Spec == nil || saved.Spec.SourceMessage != "做一个60秒口播视频" {
		t.Fatalf("saved state = %+v", saved)
	}
}

func TestCreationServiceRejectsRegenerateLockedShot(t *testing.T) {
	store := newFakeCreationProjectStore()
	store.project = projectWithShotState(t, model.ShotUnit{ID: "shot-1", ProjectID: "vp-1", DurationSec: 6, Locked: true})
	svc := service.NewCreationService(store)

	_, err := svc.RegenerateShot(context.Background(), "u-1", "vp-1", "shot-1", service.RegenerateShotRequest{Scope: "text_layers"})
	if err == nil || !strings.Contains(err.Error(), "locked") {
		t.Fatalf("RegenerateShot error = %v, want locked error", err)
	}
}
```

- [ ] **Step 2: Run tests and verify they fail**

Run:

```bash
cd cloud-backend && go test ./internal/agents/video/service -run 'TestCreationServiceStoresSpecInProjectConfig|TestCreationServiceRejectsRegenerateLockedShot' -count=1
```

Expected: FAIL because `CreationService`, config state helpers, and action methods do not exist.

- [ ] **Step 3: Implement state helpers and service**

Implement a versioned config object:

```json
{
  "shotDrivenState": {
    "schemaVersion": 1,
    "spec": {},
    "shots": [],
    "updatedAt": "..."
  }
}
```

Expose service methods:

```go
func NewCreationService(store CreationProjectStore) *CreationService
func (s *CreationService) GetSpec(ctx context.Context, userID, projectID string) (*model.VideoCreationSpec, error)
func (s *CreationService) UpsertSpec(ctx context.Context, userID, projectID string, spec *model.VideoCreationSpec) (*model.VideoCreationSpec, error)
func (s *CreationService) GenerateSpec(ctx context.Context, userID, projectID string, req GenerateSpecRequest) (*model.VideoCreationSpec, error)
func (s *CreationService) ApproveSpec(ctx context.Context, userID, projectID string) (*model.VideoCreationSpec, error)
func (s *CreationService) RejectSpec(ctx context.Context, userID, projectID string, reason string) (*model.VideoCreationSpec, error)
func (s *CreationService) ListShots(ctx context.Context, userID, projectID string) ([]model.ShotUnit, error)
func (s *CreationService) UpsertShot(ctx context.Context, userID, projectID string, shot *model.ShotUnit) (*model.ShotUnit, error)
func (s *CreationService) GenerateShots(ctx context.Context, userID, projectID string) ([]model.ShotUnit, error)
func (s *CreationService) ApproveShot(ctx context.Context, userID, projectID, shotID string) (*model.ShotUnit, error)
func (s *CreationService) RejectShot(ctx context.Context, userID, projectID, shotID string, reason string) (*model.ShotUnit, error)
func (s *CreationService) LockShot(ctx context.Context, userID, projectID, shotID string) (*model.ShotUnit, error)
func (s *CreationService) UnlockShot(ctx context.Context, userID, projectID, shotID string) (*model.ShotUnit, error)
func (s *CreationService) RegenerateShot(ctx context.Context, userID, projectID, shotID string, req RegenerateShotRequest) (*model.ShotUnit, error)
```

`GenerateShots` should deterministically split `TargetDurationSec` by preferred duration into 8-12 shots for 60 seconds, clamp each shot to 3-15 seconds, keep `singleScene=true`, `visualChangeLevel=low`, and copy exact phrases from the source message into shot `ScreenText` when present.

- [ ] **Step 4: Run tests and commit**

Run:

```bash
cd cloud-backend && go test ./internal/agents/video/service -count=1
```

Expected: PASS.

Commit:

```bash
git add cloud-backend/internal/agents/video/service/creation_state.go cloud-backend/internal/agents/video/service/creation_service.go cloud-backend/internal/agents/video/service/creation_service_test.go
git commit -m "feat: persist shot-driven video state"
```

## Task 3: Shot-Aware Artifact Stale Helpers

**Files:**
- Modify: `cloud-backend/internal/core/artifact/stale.go`
- Test: `cloud-backend/internal/core/artifact/stale_test.go`

- [ ] **Step 1: Write failing stale graph tests**

Add tests:

```go
func TestShotDownstreamStagesForTextLayers(t *testing.T) {
	got := DownstreamShotStageNamesForStage("text_layers")
	want := []string{"html_source", "html_preview_video", "html_overlay_video", "html_overlay_alpha_video", "composited_shot_video"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("downstream = %#v, want %#v", got, want)
	}
}

func TestProjectStagesInvalidatedByShotChange(t *testing.T) {
	got := ProjectStageNamesForShotStageChange("aigc_prompt")
	want := []string{"final_video", "publish_package"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("project downstream = %#v, want %#v", got, want)
	}
}
```

- [ ] **Step 2: Run tests and verify they fail**

Run:

```bash
cd cloud-backend && go test ./internal/core/artifact -run 'TestShotDownstreamStagesForTextLayers|TestProjectStagesInvalidatedByShotChange' -count=1
```

Expected: FAIL because the new helper functions do not exist.

- [ ] **Step 3: Add additive helper functions**

Add:

```go
func DownstreamShotStageNamesForStage(stageName string) []string
func ProjectStageNamesForShotStageChange(stageName string) []string
```

Do not change `DownstreamStageNamesForStage` legacy order.

- [ ] **Step 4: Run tests and commit**

Run:

```bash
cd cloud-backend && go test ./internal/core/artifact -count=1
```

Expected: PASS.

Commit:

```bash
git add cloud-backend/internal/core/artifact/stale.go cloud-backend/internal/core/artifact/stale_test.go
git commit -m "feat: add shot-aware artifact stale graph"
```

## Task 4: HTTP Handler And Route Wiring

**Files:**
- Create: `cloud-backend/internal/agents/video/handler/creation_handler.go`
- Test: `cloud-backend/internal/agents/video/handler/creation_handler_test.go`
- Modify: `cloud-backend/cmd/tangying-ai-os/main.go`

- [ ] **Step 1: Write failing handler tests**

Add tests:

```go
func TestCreationHandlerRejectsUnauthenticatedSpecAccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewCreationHandler(&fakeCreationService{}).RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodGet, "/api/video-projects/vp-1/spec", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreationHandlerUsesAuthenticatedUserForShotLock(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fake := &fakeCreationService{}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(auth.ContextWithUser(c.Request.Context(), "u-auth"))
		c.Next()
	})
	NewCreationHandler(fake).RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodPost, "/api/video-projects/vp-1/shots/shot-1/lock", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if fake.userID != "u-auth" || fake.projectID != "vp-1" || fake.shotID != "shot-1" {
		t.Fatalf("captured = user:%s project:%s shot:%s", fake.userID, fake.projectID, fake.shotID)
	}
}
```

- [ ] **Step 2: Run tests and verify they fail**

Run:

```bash
cd cloud-backend && go test ./internal/agents/video/handler -run 'TestCreationHandlerRejectsUnauthenticatedSpecAccess|TestCreationHandlerUsesAuthenticatedUserForShotLock' -count=1
```

Expected: FAIL because `NewCreationHandler` does not exist.

- [ ] **Step 3: Implement handler and main registration**

Register:

```go
api := r.Group("/api/video-projects", h.middleware...)
api.GET("/:id/spec", h.GetSpec)
api.POST("/:id/spec", h.UpsertSpec)
api.POST("/:id/spec/generate", h.GenerateSpec)
api.POST("/:id/spec/approve", h.ApproveSpec)
api.POST("/:id/spec/reject", h.RejectSpec)
api.GET("/:id/shots", h.ListShots)
api.POST("/:id/shots", h.UpsertShot)
api.POST("/:id/shots/generate", h.GenerateShots)
api.GET("/:id/shots/:shotId", h.GetShot)
api.PATCH("/:id/shots/:shotId", h.UpdateShot)
api.POST("/:id/shots/:shotId/approve", h.ApproveShot)
api.POST("/:id/shots/:shotId/reject", h.RejectShot)
api.POST("/:id/shots/:shotId/lock", h.LockShot)
api.POST("/:id/shots/:shotId/unlock", h.UnlockShot)
api.POST("/:id/shots/:shotId/regenerate", h.RegenerateShot)
api.POST("/:id/shots/:shotId/visual-plan/generate", h.GenerateVisualPlan)
api.POST("/:id/shots/:shotId/render-strategy/decide", h.DecideRenderStrategy)
api.POST("/:id/shots/:shotId/text-layers/generate", h.GenerateTextLayers)
api.POST("/:id/assemble", h.Assemble)
api.POST("/:id/publish-package/generate", h.GeneratePublishPackage)
```

In `main.go`, create `videoCreationSvc := videoSvc.NewCreationService(videoProjectRepo)` and register `videoHandler.NewCreationHandler(videoCreationSvc, authMiddleware.RequireAuth()).RegisterRoutes(r)`.

- [ ] **Step 4: Run tests and commit**

Run:

```bash
cd cloud-backend && go test ./internal/agents/video/handler ./cmd/tangying-ai-os -count=1
```

Expected: PASS.

Commit:

```bash
git add cloud-backend/internal/agents/video/handler/creation_handler.go cloud-backend/internal/agents/video/handler/creation_handler_test.go cloud-backend/cmd/tangying-ai-os/main.go
git commit -m "feat: add shot-driven video API routes"
```

## Task 5: OpenAPI, Fake Demo, And Verification

**Files:**
- Modify: `cloud-backend/internal/core/apispec/cloud_spec.go`
- Modify: `cloud-backend/docs/API_REFERENCE.md`
- Modify: `frontend/src/utils/api-types.generated.ts`
- Test: `cloud-backend/internal/agents/video/service/creation_demo_test.go`

- [ ] **Step 1: Write fake demo test**

Add:

```go
func TestFakeDemoOneSentenceOralVideoExactTextFlow(t *testing.T) {
	store := newFakeCreationProjectStore()
	store.project = &model.VideoProject{ID: "vp-1", UserID: "u-1", AspectRatio: "16:9", Language: "zh-CN", TargetDuration: 60}
	svc := service.NewCreationService(store)

	spec, err := svc.GenerateSpec(context.Background(), "u-1", "vp-1", service.GenerateSpecRequest{
		SourceMessage: "做一个60秒口播视频，主题是：AI替代的不是岗位，而是整套工作流程。画面中需要出现“几个表格”“几份文档”“十几条聊天记录”。",
	})
	if err != nil {
		t.Fatalf("GenerateSpec error: %v", err)
	}
	if spec.ReviewMode != model.ReviewModeShotLevel {
		t.Fatalf("review mode = %q", spec.ReviewMode)
	}

	shots, err := svc.GenerateShots(context.Background(), "u-1", "vp-1")
	if err != nil {
		t.Fatalf("GenerateShots error: %v", err)
	}
	if len(shots) < 8 || len(shots) > 12 {
		t.Fatalf("shot count = %d, want 8..12", len(shots))
	}
	for _, shot := range shots {
		if shot.DurationSec < 3 || shot.DurationSec > 15 {
			t.Fatalf("shot %s duration = %d", shot.ID, shot.DurationSec)
		}
	}

	textShot := firstShotWithScreenText(shots, "几个表格")
	plan, err := svc.GenerateVisualPlan(context.Background(), "u-1", "vp-1", textShot.ID)
	if err != nil {
		t.Fatalf("GenerateVisualPlan error: %v", err)
	}
	strategy, err := svc.DecideShotRenderStrategy(context.Background(), "u-1", "vp-1", textShot.ID)
	if err != nil {
		t.Fatalf("DecideShotRenderStrategy error: %v", err)
	}
	if strategy.Mode == model.RenderModeAIGCOnly {
		t.Fatalf("exact text shot used aigc_only: %+v plan=%+v", strategy, plan)
	}
	for _, layer := range plan.TextLayers {
		if layer.Text == "几个表格" && !layer.MustBeExact {
			t.Fatalf("exact text layer not marked mustBeExact: %+v", layer)
		}
	}
}
```

- [ ] **Step 2: Run demo test and verify it fails if OpenAPI work is not complete**

Run:

```bash
cd cloud-backend && go test ./internal/agents/video/service -run TestFakeDemoOneSentenceOralVideoExactTextFlow -count=1
```

Expected: PASS once Tasks 1-4 are complete. If it fails, fix the business logic before updating docs.

- [ ] **Step 3: Update OpenAPI and regenerate docs**

Add the new `Video Projects` routes to `cloud_spec.go` and define minimal schemas:

- `VideoCreationSpec`
- `ShotUnit`
- `VisualPlan`
- `RenderStrategy`
- `RejectShotRequest`
- `RegenerateShotRequest`

Run:

```bash
cd cloud-backend && make gen-docs && make api-docs-check
```

Expected: PASS and generated files updated.

- [ ] **Step 4: Run final verification**

Run:

```bash
cd cloud-backend && go test ./...
cd cloud-backend && go test -race ./...
cd local-backend && go test ./...
cd frontend && npm run build
```

Expected: all commands PASS.

- [ ] **Step 5: Final commit**

Commit:

```bash
git add cloud-backend/internal/core/apispec/cloud_spec.go cloud-backend/docs/API_REFERENCE.md frontend/src/utils/api-types.generated.ts cloud-backend/internal/agents/video/service/creation_demo_test.go
git commit -m "test: cover shot-driven video backend demo"
```

## Plan Self-Review

- Spec coverage: models/defaults are covered by Task 1; config persistence and actions by Task 2; shot-aware stale graph by Task 3; HTTP routes by Task 4; OpenAPI and fake demo by Task 5.
- Type consistency: `VideoCreationSpec`, `ShotUnit`, `VisualPlan`, `TextLayerSpec`, and `RenderStrategy` are introduced in Task 1 and reused by later tasks.
- Scope control: frontend UI and real media provider execution remain excluded.
