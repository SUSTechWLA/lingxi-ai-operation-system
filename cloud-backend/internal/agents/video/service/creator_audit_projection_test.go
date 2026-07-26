package service

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	videoModel "github.com/tangying-ai/aios-core/internal/agents/video/model"
	"github.com/tangying-ai/aios-core/internal/core/artifact"
	coreModel "github.com/tangying-ai/aios-core/internal/core/model"
)

func TestCreatorStepRegenerationUsesDurableReviewLineageAndIsIdempotent(t *testing.T) {
	project := &videoModel.VideoProject{
		ID: "vp-1", UserID: "user-1", Status: videoModel.StatusCompleted, CurrentRunID: "run-1",
	}
	projects := &auditProjectReader{project: project}
	reviews := &fakeCreatorReviewMutations{}
	nodes := fakeCreatorNodeAuditReader{nodes: []*coreModel.Node{
		{
			ID: "script-exec", TaskID: "task-1", Type: coreModel.NodeTypeLLM, Status: coreModel.NodeSuccess,
			Input: map[string]interface{}{"stage": "script_generation"},
		},
		{
			ID: "script-review", TaskID: "task-1", Type: coreModel.NodeTypeReviewGate, Status: coreModel.NodeSuccess,
			Input: map[string]interface{}{"stage": "script_generation", "sourceNode": "script-exec"},
		},
	}}
	svc := NewCreatorViewService(projects, fakeCreatorShotReader{}, fakeCreatorArtifactReader{}).
		WithStepMutations(nil, reviews).
		WithProcessAudit(fakeCreatorRunAuditLookup(&CreatorRunAudit{ID: "run-1", TaskID: "task-1", UserID: "user-1"}), nodes)

	impact, err := svc.PreviewStepRegeneration(context.Background(), "user-1", "vp-1", videoModel.CreatorStepScript)
	if err != nil {
		t.Fatalf("PreviewStepRegeneration() error = %v", err)
	}
	wantAffected := []videoModel.CreatorStepID{videoModel.CreatorStepShots, videoModel.CreatorStepPreview, videoModel.CreatorStepDelivery}
	if !reflect.DeepEqual(impact.AffectedStepIDs, wantAffected) {
		t.Fatalf("affected steps = %v, want %v", impact.AffectedStepIDs, wantAffected)
	}

	request := videoModel.StepRegenerationRequest{
		Instruction:              "重新生成这一阶段",
		ConfirmedAffectedStepIDs: wantAffected,
	}
	if _, err := svc.RegenerateStep(context.Background(), "user-1", "vp-1", videoModel.CreatorStepScript, request, ""); !errors.Is(err, ErrCreatorInvalidRequest) {
		t.Fatalf("missing idempotency error = %v, want ErrCreatorInvalidRequest", err)
	}
	wrongImpact := request
	wrongImpact.ConfirmedAffectedStepIDs = []videoModel.CreatorStepID{videoModel.CreatorStepPreview}
	if _, err := svc.RegenerateStep(context.Background(), "user-1", "vp-1", videoModel.CreatorStepScript, wrongImpact, "regen-1"); !errors.Is(err, ErrCreatorImpactMismatch) {
		t.Fatalf("impact mismatch error = %v, want ErrCreatorImpactMismatch", err)
	}

	result, err := svc.RegenerateStep(context.Background(), "user-1", "vp-1", videoModel.CreatorStepScript, request, "regen-1")
	if err != nil {
		t.Fatalf("RegenerateStep() error = %v", err)
	}
	if result.RunID != "run-1" || result.ReviewID != "script-review" || result.Attempt != 2 {
		t.Fatalf("regeneration result = %+v, want durable review lineage and attempt 2", result)
	}
	if reviews.regeneratedRunID != "run-1" || reviews.regeneratedReviewID != "script-review" || reviews.regenerationHint != request.Instruction || reviews.regenerationKey != "regen-1" {
		t.Fatalf("review mutation = %+v, want resolved lineage and request", reviews)
	}
	if projects.startedRunID != "run-1" || projects.project.Status != videoModel.StatusRunning {
		t.Fatalf("project lifecycle = %+v, want linked RUNNING project", projects)
	}
	if _, err := svc.RegenerateStep(context.Background(), "user-1", "vp-1", videoModel.CreatorStepScript, request, "regen-1"); err != nil {
		t.Fatalf("idempotent replay error = %v", err)
	}
	if reviews.regenerateCalls != 1 {
		t.Fatalf("regenerate calls = %d, want one logical retry", reviews.regenerateCalls)
	}
}

func TestCreatorAuditProjectsCompletedLegacyRunWithoutCreatorArtifacts(t *testing.T) {
	started := time.Date(2026, time.July, 22, 8, 0, 0, 0, time.UTC)
	completed := started.Add(4 * time.Minute)
	view, err := NewCreatorViewService(
		fakeCreatorProjectReader{project: &videoModel.VideoProject{
			ID: "vp-legacy", UserID: "user-1", Status: videoModel.StatusCompleted, CurrentRunID: "run-1",
		}},
		fakeCreatorShotReader{},
		fakeCreatorArtifactReader{},
	).WithProcessAudit(
		fakeCreatorRunAuditLookup(&CreatorRunAudit{ID: "run-1", TaskID: "task-1", UserID: "user-1"}),
		fakeCreatorNodeAuditReader{nodes: []*coreModel.Node{
			{
				ID: "script-exec", TaskID: "task-1", Type: coreModel.NodeTypeLLM, Name: "Generate the presenter script",
				Status: coreModel.NodeSuccess, Input: map[string]interface{}{"stage": "script_generation"},
				Output:    map[string]interface{}{"hiddenPrompt": "must never be exposed", "apiKey": "secret"},
				CreatedAt: started, StartedAt: &started, CompletedAt: &completed,
			},
			{
				ID: "script-review", TaskID: "task-1", Type: coreModel.NodeTypeReviewGate, Name: "Review presenter script",
				Status: coreModel.NodeSuccess, Input: map[string]interface{}{"stage": "script_generation", "sourceNode": "script-exec"},
				CreatedAt: completed, StartedAt: &completed, CompletedAt: &completed,
			},
		}},
	).GetCreationView(context.Background(), "user-1", "vp-legacy")
	if err != nil {
		t.Fatalf("GetCreationView() error = %v", err)
	}

	script := view.Steps[2]
	if script.State != videoModel.CreatorStepConfirmed || !script.HasHistory || script.AttemptCount != 1 {
		t.Fatalf("script step = %+v, want one confirmed historical attempt", script)
	}
	if script.RunID != "run-1" || script.ReviewID != "script-review" || script.StartedAt == nil || script.UpdatedAt == nil {
		t.Fatalf("script lineage = %+v, want durable run/review/timestamps", script)
	}
	if len(view.ProcessTimeline) != 2 {
		t.Fatalf("timeline = %+v, want source plus review events", view.ProcessTimeline)
	}
	for _, event := range view.ProcessTimeline {
		if event.Summary == "must never be exposed" || event.Summary == "secret" {
			t.Fatalf("timeline leaked node output: %+v", event)
		}
		if event.StepID != videoModel.CreatorStepScript || event.SourceID == "" || event.Title == "" {
			t.Fatalf("timeline event = %+v, want creator-safe script evidence", event)
		}
	}
}

func TestCreatorAuditMapsProductionWorkflowStageVocabulary(t *testing.T) {
	tests := map[videoModel.CreatorStepID][]string{
		videoModel.CreatorStepDirection: {"proposal_generator", "knowledge_researcher"},
		videoModel.CreatorStepScript: {
			"script_generation_quality_gate", "time_window", "audio_master",
		},
		videoModel.CreatorStepShots: {
			"shot_split", "shot_split_quality_gate", "video_prompt",
			"video_prompt_quality_gate", "ip_aroll_generation", "visual_alignment",
		},
		videoModel.CreatorStepPreview:  {"preview", "render", "visual_qa"},
		videoModel.CreatorStepDelivery: {"publish"},
	}
	for wantStep, stages := range tests {
		for _, stage := range stages {
			got, ok := creatorStepForStage(stage)
			if !ok || got != wantStep {
				t.Errorf("creatorStepForStage(%q) = %q, %v; want %q, true", stage, got, ok, wantStep)
			}
		}
	}
}

func TestCreatorAuditCompletedProjectConfirmsPersistedBriefAndApprovedPreview(t *testing.T) {
	created := time.Date(2026, time.July, 22, 8, 0, 0, 0, time.UTC)
	current := &artifact.Artifact{
		ID: "video-v1", ProjectID: "vp-complete", StageName: "render", UnitID: "final-video",
		Kind: artifact.KindVideo, Name: "final.mp4", MimeType: "video/mp4", Version: 1, IsCurrent: true,
		Status: "pending", CreatedAt: created.Add(time.Hour), UpdatedAt: created.Add(time.Hour),
	}
	view, err := NewCreatorViewService(
		fakeCreatorProjectReader{project: &videoModel.VideoProject{
			ID: "vp-complete", UserID: "user-1", Name: "Demo", Description: "A saved creator brief",
			Status: videoModel.StatusCompleted, CurrentRunID: "run-1", CreatedAt: created, UpdatedAt: created.Add(2 * time.Hour),
		}},
		fakeCreatorShotReader{},
		auditArtifactReader{current: []*artifact.Artifact{current}, history: map[string][]*artifact.Artifact{"render/final-video": {current}}},
	).WithProcessAudit(
		fakeCreatorRunAuditLookup(&CreatorRunAudit{ID: "run-1", TaskID: "task-1", UserID: "user-1"}),
		fakeCreatorNodeAuditReader{nodes: []*coreModel.Node{{
			ID: "render-review", TaskID: "task-1", Type: coreModel.NodeTypeReviewGate, Status: coreModel.NodeSuccess,
			Input: map[string]interface{}{"stage": "render"}, CreatedAt: created.Add(90 * time.Minute),
		}}},
	).GetCreationView(context.Background(), "user-1", "vp-complete")
	if err != nil {
		t.Fatalf("GetCreationView() error = %v", err)
	}
	if requirements := view.Steps[0]; requirements.State != videoModel.CreatorStepConfirmed || !requirements.HasHistory {
		t.Fatalf("requirements step = %+v, want persisted brief history", requirements)
	}
	if preview := view.Steps[4]; preview.State != videoModel.CreatorStepConfirmed {
		t.Fatalf("preview step = %+v, want approved review to outrank legacy pending artifact status", preview)
	}
	if len(view.ProcessTimeline) < 3 || view.ProcessTimeline[0].SourceType != "project" {
		t.Fatalf("timeline = %+v, want project brief plus artifact and review evidence", view.ProcessTimeline)
	}
}

func TestCreatorViewProjectsAuthoritativeFinalDeliveryVideoAcrossMultipleCurrentDeliveryArtifacts(t *testing.T) {
	created := time.Date(2026, time.July, 26, 8, 0, 0, 0, time.UTC)
	finalVideo := &artifact.Artifact{
		ID: "final-video", ProjectID: "vp-complete", StageName: "render", UnitID: "final-video",
		Kind: artifact.KindVideo, Name: "final.mp4", MimeType: "video/mp4", Version: 2, IsCurrent: true,
		Status: "valid", HumanApproved: true, CreatedAt: created,
		Metadata: map[string]interface{}{"artifactType": "external_generation_result", "generationKind": "video", "tags": []interface{}{"final_video"}},
	}
	publishCopy := &artifact.Artifact{
		ID: "publish-copy", ProjectID: "vp-complete", StageName: "publish", UnitID: "publish-copy",
		Kind: artifact.KindMarkdown, Name: "publish.md", Version: 9, IsCurrent: true,
		Status: "valid", HumanApproved: true, CreatedAt: created.Add(time.Minute), Metadata: map[string]interface{}{"artifactType": "publish_copy"},
	}
	exportBundle := &artifact.Artifact{
		ID: "export-bundle", ProjectID: "vp-complete", StageName: "export", UnitID: "package",
		Kind: artifact.KindBundle, Name: "export.zip", Version: 11, IsCurrent: true,
		Status: "valid", HumanApproved: true, CreatedAt: created.Add(2 * time.Minute),
	}
	reader := auditArtifactReader{current: []*artifact.Artifact{publishCopy, exportBundle, finalVideo}, history: map[string][]*artifact.Artifact{
		"render/final-video": {finalVideo}, "publish/publish-copy": {publishCopy}, "export/package": {exportBundle},
	}}
	view, err := NewCreatorViewService(
		fakeCreatorProjectReader{project: &videoModel.VideoProject{ID: "vp-complete", UserID: "user-1", Status: videoModel.StatusCompleted}},
		fakeCreatorShotReader{}, reader,
	).GetCreationView(context.Background(), "user-1", "vp-complete")
	if err != nil {
		t.Fatalf("GetCreationView() error = %v", err)
	}
	if view.FinalDeliveryArtifactID != finalVideo.ID {
		t.Fatalf("finalDeliveryArtifactId = %q, want %q", view.FinalDeliveryArtifactID, finalVideo.ID)
	}
	if delivery := view.Steps[5]; delivery.CurrentArtifactID != finalVideo.ID || delivery.CurrentVersion != finalVideo.Version {
		t.Fatalf("delivery authority = %+v, want final video instead of publish/export artifacts", delivery)
	}
	descriptors := view.StepArtifacts[videoModel.CreatorStepDelivery]
	if len(descriptors) == 0 || descriptors[0].ArtifactID != finalVideo.ID {
		t.Fatalf("delivery descriptors = %+v, want authoritative final video first", descriptors)
	}
}

func TestCreatorViewMissingFinalVideoNeverSubstitutesPublishOrExportArtifact(t *testing.T) {
	created := time.Date(2026, time.July, 26, 9, 0, 0, 0, time.UTC)
	publishCopy := &artifact.Artifact{
		ID: "publish-copy", ProjectID: "vp-complete", StageName: "publish", UnitID: "publish-copy",
		Kind: artifact.KindMarkdown, Name: "publish.md", Version: 9, IsCurrent: true,
		Status: "valid", HumanApproved: true, CreatedAt: created, Metadata: map[string]interface{}{"artifactType": "publish_copy"},
	}
	exportBundle := &artifact.Artifact{
		ID: "export-bundle", ProjectID: "vp-complete", StageName: "export", UnitID: "package",
		Kind: artifact.KindBundle, Name: "export.zip", Version: 11, IsCurrent: true,
		Status: "valid", HumanApproved: true, CreatedAt: created.Add(time.Minute),
	}
	reader := auditArtifactReader{current: []*artifact.Artifact{publishCopy, exportBundle}, history: map[string][]*artifact.Artifact{
		"publish/publish-copy": {publishCopy}, "export/package": {exportBundle},
	}}
	project := &videoModel.VideoProject{ID: "vp-complete", UserID: "user-1", Status: videoModel.StatusCompleted, CurrentRunID: "run-delivery"}
	projects := &auditProjectReader{project: project}
	reviews := &fakeCreatorReviewMutations{}
	nodes := fakeCreatorNodeAuditReader{nodes: []*coreModel.Node{
		{
			ID: "delivery-exec", TaskID: "task-delivery", Type: coreModel.NodeTypeTool, Status: coreModel.NodeSuccess,
			Input: map[string]interface{}{"stage": "delivery"},
		},
		{
			ID: "delivery-review", TaskID: "task-delivery", Type: coreModel.NodeTypeReviewGate, Status: coreModel.NodeSuccess,
			Input: map[string]interface{}{"stage": "delivery", "sourceNode": "delivery-exec"},
		},
	}}
	svc := NewCreatorViewService(
		projects,
		fakeCreatorShotReader{}, reader,
	).WithStepMutations(nil, reviews).WithProcessAudit(
		fakeCreatorRunAuditLookup(&CreatorRunAudit{ID: "run-delivery", TaskID: "task-delivery", UserID: "user-1"}), nodes,
	)
	view, err := svc.GetCreationView(context.Background(), "user-1", "vp-complete")
	if err != nil {
		t.Fatalf("GetCreationView() error = %v", err)
	}
	if view.FinalDeliveryArtifactID != "" || view.Steps[5].CurrentArtifactID != "" || view.Steps[5].CurrentVersion != 0 {
		t.Fatalf("missing final video exposed generic delivery authority: id=%q step=%+v", view.FinalDeliveryArtifactID, view.Steps[5])
	}
	if view.Steps[5].State != videoModel.CreatorStepNeedsAttention {
		t.Fatalf("missing final video state = %q, want recovery state", view.Steps[5].State)
	}
	if _, err := svc.currentArtifactForStep(context.Background(), "vp-complete", videoModel.CreatorStepDelivery); !errors.Is(err, ErrCreatorArtifactNotFound) {
		t.Fatalf("delivery authority without final video error = %v, want not found", err)
	}
	impact, err := svc.PreviewStepRegeneration(context.Background(), "user-1", "vp-complete", videoModel.CreatorStepDelivery)
	if err != nil || impact.RequiresConfirmation || len(impact.AffectedStepIDs) != 0 || len(impact.AffectedShotIDs) != 0 {
		t.Fatalf("delivery recovery impact = %+v, error = %v; upstream must be preserved", impact, err)
	}
	result, err := svc.RegenerateStep(context.Background(), "user-1", "vp-complete", videoModel.CreatorStepDelivery, videoModel.StepRegenerationRequest{
		Instruction: "仅重新生成当前成片交付文件，保留所有上游内容",
	}, "delivery-recovery-1")
	if err != nil {
		t.Fatalf("delivery regeneration without fake base error = %v", err)
	}
	if result.RunID != "run-delivery" || result.ReviewID != "delivery-review" || reviews.regenerateCalls != 1 || projects.startedRunID != "run-delivery" {
		t.Fatalf("delivery recovery did not preserve durable lineage: result=%+v reviews=%+v project=%+v", result, reviews, project)
	}
}

func TestCreatorAuditGroupsCurrentAndHistoricalArtifactsByStep(t *testing.T) {
	created := time.Date(2026, time.July, 22, 9, 0, 0, 0, time.UTC)
	current := &artifact.Artifact{
		ID: "script-v2", ProjectID: "vp-1", StageName: "script", UnitID: "main", Kind: artifact.KindMarkdown,
		Name: "script.md", MimeType: "text/markdown", Version: 2, IsCurrent: true, Status: "valid", HumanApproved: true,
		CreatedAt: created.Add(time.Minute), UpdatedAt: created.Add(time.Minute),
	}
	historical := &artifact.Artifact{
		ID: "script-v1", ProjectID: "vp-1", StageName: "script", UnitID: "main", Kind: artifact.KindMarkdown,
		Name: "script.md", MimeType: "text/markdown", Version: 1, Status: "stale", CreatedAt: created, UpdatedAt: created,
	}
	reader := auditArtifactReader{
		current: []*artifact.Artifact{current},
		history: map[string][]*artifact.Artifact{"script/main": {current, historical}},
	}
	view, err := NewCreatorViewService(
		fakeCreatorProjectReader{project: &videoModel.VideoProject{ID: "vp-1", UserID: "user-1"}},
		fakeCreatorShotReader{}, reader,
	).GetCreationView(context.Background(), "user-1", "vp-1")
	if err != nil {
		t.Fatalf("GetCreationView() error = %v", err)
	}

	items := view.StepArtifacts[videoModel.CreatorStepScript]
	if got := []string{items[0].ArtifactID, items[1].ArtifactID}; !reflect.DeepEqual(got, []string{"script-v2", "script-v1"}) {
		t.Fatalf("artifact IDs = %v, want current then history", got)
	}
	if !items[0].IsCurrent || items[0].IsStale || items[1].IsCurrent || !items[1].IsStale {
		t.Fatalf("artifact freshness = %+v, want explicit current/history state", items)
	}
	if view.Steps[2].ArtifactCount != 2 || !view.Steps[2].HasHistory {
		t.Fatalf("script step = %+v, want two readable artifacts", view.Steps[2])
	}
}

func TestCreatorAuditProjectsSafeClassificationHints(t *testing.T) {
	created := time.Date(2026, time.July, 24, 9, 0, 0, 0, time.UTC)
	current := &artifact.Artifact{
		ID: "image-request", ProjectID: "vp-1", StageName: "script", UnitID: "main",
		Kind: artifact.KindJSON, Name: "shot-02-image-request.json", MimeType: "application/json",
		Version: 1, IsCurrent: true, Status: "valid", CreatedAt: created, UpdatedAt: created,
		Metadata: map[string]interface{}{
			"artifactType":   "external_generation_request",
			"generationKind": "image",
			"relatedShotId":  "SHOT_02",
			"apiKey":         "must-not-leak",
		},
	}
	view, err := NewCreatorViewService(
		fakeCreatorProjectReader{project: &videoModel.VideoProject{ID: "vp-1", UserID: "user-1"}},
		fakeCreatorShotReader{},
		auditArtifactReader{current: []*artifact.Artifact{current}, history: map[string][]*artifact.Artifact{"script/main": {current}}},
	).GetCreationView(context.Background(), "user-1", "vp-1")
	if err != nil {
		t.Fatalf("GetCreationView() error = %v", err)
	}

	got := view.StepArtifacts[videoModel.CreatorStepScript][0]
	if got.ArtifactType != "external_generation_request" || got.GenerationKind != "image" || got.RelatedShotID != "SHOT_02" {
		t.Fatalf("classification hints = %+v, want allow-listed artifact type, generation kind, and related Shot", got)
	}
	payload, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal descriptor: %v", err)
	}
	if strings.Contains(string(payload), "apiKey") || strings.Contains(string(payload), "must-not-leak") || strings.Contains(string(payload), "metadata") {
		t.Fatalf("descriptor leaked metadata: %s", payload)
	}
}

func TestCreatorAuditClassificationHintsAllowListAndSanitization(t *testing.T) {
	long := strings.Repeat("界", 121)
	tests := []struct {
		name                                        string
		metadata                                    map[string]interface{}
		artifactType, generationKind, relatedShotID string
	}{
		{
			name: "trims and limits allow-listed strings",
			metadata: map[string]interface{}{
				"artifactType": "  shot_keyframe  ", "generationKind": long,
				"relatedShotId": "  SHOT_02  ", "apiKey": "must-not-leak",
			},
			artifactType: "shot_keyframe", generationKind: strings.Repeat("界", 120), relatedShotID: "SHOT_02",
		},
		{
			name: "uses shot ID fallback and rejects non strings",
			metadata: map[string]interface{}{
				"artifactType": 12, "generationKind": []string{"image"}, "relatedShotId": 4, "shotId": " SHOT_03 ",
			},
			relatedShotID: "SHOT_03",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			artifactType, generationKind, relatedShotID := creatorArtifactClassificationHints(&artifact.Artifact{Metadata: tt.metadata})
			if artifactType != tt.artifactType || generationKind != tt.generationKind || relatedShotID != tt.relatedShotID {
				t.Fatalf("creatorArtifactClassificationHints() = (%q, %q, %q), want (%q, %q, %q)", artifactType, generationKind, relatedShotID, tt.artifactType, tt.generationKind, tt.relatedShotID)
			}
		})
	}
}

func fakeCreatorRunAuditLookup(run *CreatorRunAudit) creatorRunAuditLookup {
	return func(context.Context, string) (*CreatorRunAudit, error) { return run, nil }
}

type fakeCreatorNodeAuditReader struct {
	nodes []*coreModel.Node
	err   error
}

func (f fakeCreatorNodeAuditReader) FindByTaskID(context.Context, string) ([]*coreModel.Node, error) {
	return f.nodes, f.err
}

type auditArtifactReader struct {
	current []*artifact.Artifact
	history map[string][]*artifact.Artifact
}

type auditProjectReader struct {
	project      *videoModel.VideoProject
	startedRunID string
}

func (f *auditProjectReader) GetProject(_ context.Context, userID, projectID string) (*videoModel.VideoProject, error) {
	if f.project == nil || f.project.UserID != userID || f.project.ID != projectID {
		return nil, nil
	}
	return f.project, nil
}

func (f *auditProjectReader) MarkAgentRunStarted(_ context.Context, userID, projectID, runID string) error {
	if f.project == nil || f.project.UserID != userID || f.project.ID != projectID {
		return errors.New("project not found")
	}
	f.startedRunID = runID
	f.project.Status = videoModel.StatusRunning
	f.project.CurrentRunID = runID
	return nil
}

func (f auditArtifactReader) ListCurrentByProject(context.Context, string) ([]*artifact.Artifact, error) {
	return f.current, nil
}

func (f auditArtifactReader) GetByID(_ context.Context, id string) (*artifact.Artifact, error) {
	for _, items := range f.history {
		for _, item := range items {
			if item != nil && item.ID == id {
				return item, nil
			}
		}
	}
	return nil, nil
}

func (f auditArtifactReader) GetCurrent(_ context.Context, _ string, stageName, unitID string) (*artifact.Artifact, error) {
	for _, item := range f.current {
		if item != nil && item.StageName == stageName && item.UnitID == unitID {
			return item, nil
		}
	}
	return nil, nil
}

func (f auditArtifactReader) GetHistory(_ context.Context, _ string, stageName, unitID string) ([]*artifact.Artifact, error) {
	return f.history[stageName+"/"+unitID], nil
}
