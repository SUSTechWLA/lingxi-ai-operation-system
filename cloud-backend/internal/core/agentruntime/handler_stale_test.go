package agentruntime

import (
	"context"
	"testing"

	"github.com/tangying-ai/aios-core/internal/core/artifact"
	"github.com/tangying-ai/aios-core/internal/core/model"
)

func TestTriggerDownstreamStalePrefersRealArtifactID(t *testing.T) {
	svc := &recordingArtifactService{}
	h := (&Handler{}).
		WithArtifactService(svc).
		WithProjectIDResolver(staticProjectIDResolver{projectID: "project-1"})

	h.triggerDownstreamStale(context.Background(), &Run{TaskID: "task-1"}, &model.Node{
		ID: "script_review",
		Input: map[string]interface{}{
			"artifactId":      "art_real_script",
			"stage":           "script",
			"requiredOutputs": []interface{}{"VIDEO_SCRIPT"},
		},
	}, "用户修改上游产物")

	if svc.markByArtifactID != "art_real_script" {
		t.Fatalf("expected real artifact id stale path, got artifact=%q stage=%q stages=%v",
			svc.markByArtifactID, svc.markByStageName, svc.markByStageNames)
	}
	if svc.markByStageName != "" || len(svc.markByStageNames) != 0 {
		t.Fatalf("artifact id path must not use stage fallback: stage=%q stages=%v",
			svc.markByStageName, svc.markByStageNames)
	}
}

func TestTriggerDownstreamStaleFallsBackToChangedStageName(t *testing.T) {
	svc := &recordingArtifactService{}
	h := (&Handler{}).
		WithArtifactService(svc).
		WithProjectIDResolver(staticProjectIDResolver{projectID: "project-1"})

	h.triggerDownstreamStale(context.Background(), &Run{TaskID: "task-1"}, &model.Node{
		ID: "script_review",
		Input: map[string]interface{}{
			"stage":           "script",
			"requiredOutputs": []interface{}{"VIDEO_SCRIPT"},
		},
	}, "用户修改上游产物")

	if svc.markByStageName != "script" {
		t.Fatalf("expected stale fallback to changed stage name, got artifact=%q stage=%q stages=%v",
			svc.markByArtifactID, svc.markByStageName, svc.markByStageNames)
	}
	if svc.markByArtifactID != "" || len(svc.markByStageNames) != 0 {
		t.Fatalf("stage fallback must not synthesize artifact ids or pass downstream stage list directly: artifact=%q stages=%v",
			svc.markByArtifactID, svc.markByStageNames)
	}
}

type staticProjectIDResolver struct {
	projectID string
}

func (r staticProjectIDResolver) ResolveProjectID(_ context.Context, _ string) (string, error) {
	return r.projectID, nil
}

type recordingArtifactService struct {
	markByArtifactID   string
	markByStageName    string
	markByStageNames   []string
	approvedID         string
	approvedProjectID  string
	approvedStageName  string
	approvedKinds      []string
	approvedReviewerID string
	currentArtifact    *artifact.Artifact
}

func (s *recordingArtifactService) MarkDownstreamStale(_ context.Context, _ string, changedArtifactID string, _ string) ([]string, error) {
	s.markByArtifactID = changedArtifactID
	return nil, nil
}

func (s *recordingArtifactService) MarkDownstreamStaleByStageName(_ context.Context, _ string, stageName string, _ string) ([]string, error) {
	s.markByStageName = stageName
	return nil, nil
}

func (s *recordingArtifactService) ApproveArtifact(_ context.Context, artifactID string, _ string) error {
	s.approvedID = artifactID
	return nil
}

func (s *recordingArtifactService) ApproveCurrentArtifactsByStageAndKinds(_ context.Context, projectID string, stageName string, artifactKinds []string, reviewerID string) ([]string, error) {
	s.approvedProjectID = projectID
	s.approvedStageName = stageName
	s.approvedKinds = artifactKinds
	s.approvedReviewerID = reviewerID
	return []string{"art_preview"}, nil
}

func (s *recordingArtifactService) FindCurrentByKind(_ context.Context, _, _ string) (*artifact.Artifact, error) {
	return s.currentArtifact, nil
}

func (s *recordingArtifactService) FindCurrentByStageAndKind(_ context.Context, _, _, _ string) (*artifact.Artifact, error) {
	return s.currentArtifact, nil
}
