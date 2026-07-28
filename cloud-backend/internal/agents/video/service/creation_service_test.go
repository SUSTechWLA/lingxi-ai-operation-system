package service

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/tangying-ai/aios-core/internal/agents/video/model"
)

func TestCreationServiceStoresSpecInProjectConfig(t *testing.T) {
	store := newFakeCreationProjectStore()
	store.project = &model.VideoProject{
		ID:             "vp-1",
		UserID:         "u-1",
		AspectRatio:    "16:9",
		Language:       "zh-CN",
		TargetDuration: 60,
	}
	svc := NewCreationService(store)

	spec, err := svc.UpsertSpec(context.Background(), "u-1", "vp-1", &model.VideoCreationSpec{SourceMessage: "做一个60秒口播视频"})
	if err != nil {
		t.Fatalf("UpsertSpec error: %v", err)
	}
	if spec.TargetDurationSec != 60 {
		t.Fatalf("TargetDurationSec = %d, want 60", spec.TargetDurationSec)
	}
	if spec.ProjectID != "vp-1" || spec.AspectRatio != "16:9" || spec.Language != "zh-CN" {
		t.Fatalf("spec project defaults = %+v", spec)
	}
	saved := decodeStateFromTest(t, store.updated.Config)
	if saved.Spec == nil || saved.Spec.SourceMessage != "做一个60秒口播视频" {
		t.Fatalf("saved state = %+v", saved)
	}
}

func TestCreationServiceGenerateSpecDerivesTopicAndVideoType(t *testing.T) {
	store := newFakeCreationProjectStore()
	store.project = &model.VideoProject{ID: "vp-1", UserID: "u-1", AspectRatio: "16:9", Language: "zh-CN", TargetDuration: 60}
	svc := NewCreationService(store)

	spec, err := svc.GenerateSpec(context.Background(), "u-1", "vp-1", GenerateSpecRequest{
		SourceMessage: "做一个60秒口播视频，主题是：AI替代的不是岗位，而是整套工作流程。",
	})
	if err != nil {
		t.Fatalf("GenerateSpec error: %v", err)
	}
	if spec.VideoType != "口播视频" {
		t.Fatalf("VideoType = %q", spec.VideoType)
	}
	if !strings.Contains(spec.Topic, "AI替代的不是岗位") {
		t.Fatalf("Topic = %q", spec.Topic)
	}
}

func TestCreationServiceGenerateShotsFor60SecProduces8To12ValidShots(t *testing.T) {
	store := newFakeCreationProjectStore()
	store.project = &model.VideoProject{ID: "vp-1", UserID: "u-1", TargetDuration: 60}
	svc := NewCreationService(store)
	_, err := svc.GenerateSpec(context.Background(), "u-1", "vp-1", GenerateSpecRequest{
		SourceMessage: "做一个60秒口播视频，主题是：AI替代的不是岗位，而是整套工作流程。画面中需要出现“几个表格”“几份文档”“十几条聊天记录”。",
	})
	if err != nil {
		t.Fatalf("GenerateSpec error: %v", err)
	}

	shots, err := svc.GenerateShots(context.Background(), "u-1", "vp-1")
	if err != nil {
		t.Fatalf("GenerateShots error: %v", err)
	}
	if len(shots) < 8 || len(shots) > 12 {
		t.Fatalf("shot count = %d, want 8..12", len(shots))
	}
	foundExactText := false
	for _, shot := range shots {
		if shot.ProjectID != "vp-1" {
			t.Fatalf("shot ProjectID = %q", shot.ProjectID)
		}
		if shot.DurationSec < 3 || shot.DurationSec > 15 {
			t.Fatalf("shot %s duration = %d", shot.ID, shot.DurationSec)
		}
		if !shot.SingleScene || shot.VisualChangeLevel != model.VisualChangeLow {
			t.Fatalf("shot complexity defaults = %+v", shot)
		}
		if containsString(shot.ScreenText, "几个表格") {
			foundExactText = true
		}
	}
	if !foundExactText {
		t.Fatalf("generated shots did not carry exact screen text: %+v", shots)
	}
}

func TestCreationServiceGenerateShotsDoesNotDuplicateTopicNarration(t *testing.T) {
	store := newFakeCreationProjectStore()
	store.project = &model.VideoProject{ID: "vp-1", UserID: "u-1", TargetDuration: 30}
	svc := NewCreationService(store)
	_, err := svc.GenerateSpec(context.Background(), "u-1", "vp-1", GenerateSpecRequest{
		SourceMessage: "做一个30秒口播视频，主题是：这是完整主题，不是任何单个 Shot 的旁白。",
	})
	if err != nil {
		t.Fatalf("GenerateSpec error: %v", err)
	}

	shots, err := svc.GenerateShots(context.Background(), "u-1", "vp-1")
	if err != nil {
		t.Fatalf("GenerateShots error: %v", err)
	}
	for _, shot := range shots {
		if shot.Narration != "" {
			t.Fatalf("shot %s narration = %q, want empty until canonical timeline assignment", shot.ID, shot.Narration)
		}
	}
}

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

func TestDecodeShotDrivenStateInitializesDurableMaps(t *testing.T) {
	state, err := DecodeShotDrivenState(nil)
	if err != nil {
		t.Fatalf("DecodeShotDrivenState error: %v", err)
	}
	if state.ShotHistory == nil || state.RegenerationTasks == nil || state.IdempotencyTasks == nil {
		t.Fatalf("durable state maps must be initialized: %+v", state)
	}
}

func TestCreationServiceRejectsRegenerateLockedShot(t *testing.T) {
	store := newFakeCreationProjectStore()
	store.project = projectWithShotState(t, model.ShotUnit{ID: "shot-1", ProjectID: "vp-1", DurationSec: 6, Locked: true, ReviewStatus: model.ReviewStatusApproved})
	svc := NewCreationService(store)

	_, err := svc.RegenerateShot(context.Background(), "u-1", "vp-1", "shot-1", RegenerateShotRequest{Scope: "text_layers"})
	if err == nil || !strings.Contains(err.Error(), "locked") {
		t.Fatalf("RegenerateShot error = %v, want locked error", err)
	}
}

func TestCreationServiceRejectReasonIsUsedByRegenerate(t *testing.T) {
	store := newFakeCreationProjectStore()
	store.project = projectWithShotState(t, model.ShotUnit{ID: "shot-1", ProjectID: "vp-1", DurationSec: 6, SingleScene: true, VisualChangeLevel: model.VisualChangeLow, ReviewStatus: model.ReviewStatusPending})
	svc := NewCreationService(store)

	_, err := svc.RejectShot(context.Background(), "u-1", "vp-1", "shot-1", "文字太小")
	if err != nil {
		t.Fatalf("RejectShot error: %v", err)
	}
	shot, err := svc.RegenerateShot(context.Background(), "u-1", "vp-1", "shot-1", RegenerateShotRequest{Scope: "text_layers"})
	if err != nil {
		t.Fatalf("RegenerateShot error: %v", err)
	}
	if !strings.Contains(shot.PromptConstraints.MustInclude[0], "文字太小") {
		t.Fatalf("PromptConstraints = %+v", shot.PromptConstraints)
	}
	if !shot.Stale || shot.ReviewStatus != model.ReviewStatusPending {
		t.Fatalf("regenerated shot state = %+v", shot)
	}
}

func TestCreationServiceGenerateVisualPlanAndTextLayers(t *testing.T) {
	store := newFakeCreationProjectStore()
	store.project = projectWithShotState(t, model.ShotUnit{
		ID:                "shot-1",
		ProjectID:         "vp-1",
		DurationSec:       6,
		SingleScene:       true,
		VisualChangeLevel: model.VisualChangeLow,
		ScreenText:        []string{"几个表格", "几份文档"},
		MainAction:        "主持人在办公室解释流程变化",
		VideoType:         "口播视频",
	})
	svc := NewCreationService(store)

	plan, err := svc.GenerateVisualPlan(context.Background(), "u-1", "vp-1", "shot-1")
	if err != nil {
		t.Fatalf("GenerateVisualPlan error: %v", err)
	}
	if plan.Canvas.Width != 1920 || plan.Canvas.Height != 1080 || plan.Canvas.DurationSec != 6 {
		t.Fatalf("canvas = %+v", plan.Canvas)
	}
	for _, text := range []string{"几个表格", "几份文档"} {
		if !textLayerMustBeExact(plan.TextLayers, text) {
			t.Fatalf("missing exact text layer %q in %+v", text, plan.TextLayers)
		}
	}
}

func TestCreationServiceDecideRenderStrategyRejectsAIGCOnlyForExactText(t *testing.T) {
	store := newFakeCreationProjectStore()
	store.project = projectWithShotState(t, model.ShotUnit{
		ID:                "shot-1",
		ProjectID:         "vp-1",
		DurationSec:       6,
		SingleScene:       true,
		VisualChangeLevel: model.VisualChangeLow,
		ScreenText:        []string{"几个表格"},
		MainAction:        "主持人在办公室解释流程变化",
		VideoType:         "口播视频",
	})
	svc := NewCreationService(store)

	if _, err := svc.GenerateVisualPlan(context.Background(), "u-1", "vp-1", "shot-1"); err != nil {
		t.Fatalf("GenerateVisualPlan error: %v", err)
	}
	strategy, err := svc.DecideShotRenderStrategy(context.Background(), "u-1", "vp-1", "shot-1")
	if err != nil {
		t.Fatalf("DecideShotRenderStrategy error: %v", err)
	}
	if strategy.Mode == model.RenderModeAIGCOnly {
		t.Fatalf("strategy = %+v, must not be aigc_only", strategy)
	}
}

func decodeStateFromTest(t *testing.T, raw json.RawMessage) model.ShotDrivenState {
	t.Helper()
	state, err := DecodeShotDrivenState(raw)
	if err != nil {
		t.Fatalf("DecodeShotDrivenState error: %v", err)
	}
	return state
}

func projectWithShotState(t *testing.T, shots ...model.ShotUnit) *model.VideoProject {
	t.Helper()
	state := model.ShotDrivenState{
		SchemaVersion: 1,
		Shots:         shots,
	}
	raw, err := EncodeShotDrivenState(nil, state)
	if err != nil {
		t.Fatalf("EncodeShotDrivenState error: %v", err)
	}
	return &model.VideoProject{ID: "vp-1", UserID: "u-1", Config: raw, TargetDuration: 60, AspectRatio: "16:9", Language: "zh-CN"}
}

type fakeCreationProjectStore struct {
	mu                    sync.Mutex
	project               *model.VideoProject
	updated               *model.VideoProject
	casConflicts          int
	casCalls              int
	onCASConflict         func(*model.VideoProject)
	persistedTaskStatuses []string
	pendingRegenerations  []PendingShotRegeneration
	pendingLimit          int
}

func (f *fakeCreationProjectStore) FindPendingShotRegenerations(_ context.Context, limit int) ([]PendingShotRegeneration, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pendingLimit = limit
	return append([]PendingShotRegeneration(nil), f.pendingRegenerations...), nil
}

func newFakeCreationProjectStore() *fakeCreationProjectStore {
	return &fakeCreationProjectStore{}
}

func (f *fakeCreationProjectStore) FindByIDForUser(ctx context.Context, userID string, id string) (*model.VideoProject, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.project == nil {
		return nil, errProjectNotFoundForTest()
	}
	project := *f.project
	return &project, nil
}

func (f *fakeCreationProjectStore) UpdateForUser(ctx context.Context, userID string, p *model.VideoProject) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	updated := *p
	f.updated = &updated
	f.project = &updated
	return nil
}

func (f *fakeCreationProjectStore) hasPersistedTaskStatus(status string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, persisted := range f.persistedTaskStatuses {
		if persisted == status {
			return true
		}
	}
	return false
}

func (f *fakeCreationProjectStore) CompareAndSwapForUser(_ context.Context, _ string, p *model.VideoProject, expectedRevision int64) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.casCalls++
	if f.project == nil || f.project.ConfigRevision != expectedRevision {
		return false, nil
	}
	if f.casConflicts > 0 {
		f.casConflicts--
		if f.onCASConflict != nil {
			f.onCASConflict(f.project)
		}
		f.project.ConfigRevision++
		return false, nil
	}
	updated := *p
	updated.ConfigRevision = expectedRevision + 1
	f.updated = &updated
	f.project = &updated
	if state, err := DecodeShotDrivenState(updated.Config); err == nil {
		for _, task := range state.RegenerationTasks {
			f.persistedTaskStatuses = append(f.persistedTaskStatuses, task.Status)
		}
	}
	p.ConfigRevision = updated.ConfigRevision
	return true, nil
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func textLayerMustBeExact(layers []model.TextLayerSpec, text string) bool {
	for _, layer := range layers {
		if layer.Text == text && layer.MustBeExact {
			return true
		}
	}
	return false
}

func errProjectNotFoundForTest() error {
	return &testError{s: "project not found"}
}

type testError struct {
	s string
}

func (e *testError) Error() string {
	return e.s
}
