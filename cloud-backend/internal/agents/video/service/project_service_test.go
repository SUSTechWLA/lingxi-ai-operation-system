package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/tangying-ai/aios-core/internal/agents/video/model"
)

func TestProjectServiceUsesExpectedRevisionCAS(t *testing.T) {
	store := &fakeProjectCASStore{project: &model.VideoProject{ID: "vp-1", UserID: "u-1", ConfigRevision: 7}}
	svc := NewProjectService(store)
	if err := svc.MarkAgentRunStarted(context.Background(), "u-1", "vp-1", "run-1"); err != nil {
		t.Fatalf("MarkAgentRunStarted: %v", err)
	}
	if store.casCalls != 1 || store.expectedRevision != 7 || store.updateCalls != 0 {
		t.Fatalf("CAS calls=%d expectedRevision=%d unconditional calls=%d", store.casCalls, store.expectedRevision, store.updateCalls)
	}
}

func TestProjectServiceReturnsRevisionConflictWithoutBlindRetry(t *testing.T) {
	store := &fakeProjectCASStore{
		project: &model.VideoProject{ID: "vp-1", UserID: "u-1", ConfigRevision: 7},
		lostCAS: true,
	}
	svc := NewProjectService(store)
	_, err := svc.UpdateProject(context.Background(), "u-1", "vp-1", &model.UpdateProjectRequest{Name: "new"})
	if !errors.Is(err, errProjectRevisionConflict) || store.casCalls != 1 {
		t.Fatalf("error=%v CAS calls=%d", err, store.casCalls)
	}
}

type fakeProjectCASStore struct {
	project          *model.VideoProject
	updateCalls      int
	casCalls         int
	expectedRevision int64
	lostCAS          bool
}

func (f *fakeProjectCASStore) Create(context.Context, *model.VideoProject) error { return nil }
func (f *fakeProjectCASStore) FindByIDForUser(context.Context, string, string) (*model.VideoProject, error) {
	copy := *f.project
	return &copy, nil
}
func (f *fakeProjectCASStore) FindAllForUser(context.Context, string, string, string, int, int) ([]*model.VideoProject, int, error) {
	return nil, 0, nil
}
func (f *fakeProjectCASStore) UpdateForUser(context.Context, string, *model.VideoProject) error {
	f.updateCalls++
	return nil
}
func (f *fakeProjectCASStore) CompareAndSwapForUser(_ context.Context, _ string, project *model.VideoProject, expected int64) (bool, error) {
	f.casCalls++
	f.expectedRevision = expected
	if f.lostCAS {
		return false, nil
	}
	copy := *project
	copy.ConfigRevision = expected + 1
	f.project = &copy
	return true, nil
}
func (f *fakeProjectCASStore) SoftDeleteForUser(context.Context, string, string) error { return nil }

func TestIsValidMode(t *testing.T) {
	tests := []struct {
		mode     model.VideoMode
		expected bool
	}{
		{model.ModeAIGCShot, true},
		{model.ModeVoiceVisual, true},
		{model.ModeCinematicStory, true},
		{model.VideoMode("invalid"), false},
		{model.VideoMode(""), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.mode), func(t *testing.T) {
			if got := model.IsValidMode(tt.mode); got != tt.expected {
				t.Errorf("IsValidMode(%q) = %v, want %v", tt.mode, got, tt.expected)
			}
		})
	}
}

func TestMergeProjectConfigPreservesPinnedIPAssetPackAndCanonicalRuntime(t *testing.T) {
	existing := json.RawMessage(`{"topic":"old","ipAssetPack":{"id":"ip-tangying","version":"2.1.0","contentHash":"sha256:pack"},"runtimePipelineId":"legacy"}`)
	update := json.RawMessage(`{"topic":"new"}`)
	merged := mergeProjectConfig(existing, update, model.VideoProfileTalkingHead)
	var config map[string]interface{}
	if err := json.Unmarshal(merged, &config); err != nil {
		t.Fatal(err)
	}
	pack, ok := config["ipAssetPack"].(map[string]interface{})
	if !ok || pack["version"] != "2.1.0" || pack["contentHash"] != "sha256:pack" {
		t.Fatalf("pinned IP asset pack should survive unrelated config edit: %+v", config)
	}
	if config["canonicalProfileId"] != model.VideoProfileTalkingHead || config["runtimePipelineId"] != model.VideoRuntimePipelineID {
		t.Fatalf("canonical runtime metadata should be authoritative: %+v", config)
	}
}

func TestCanonicalProjectConfigPersistsOnlyNonSecretProviderReferences(t *testing.T) {
	raw := json.RawMessage(`{
		"modelProviders":{"text_to_text":{"baseUrl":"https://model.test","model":"writer","apiKey":"sk-raw"}},
		"modelProviderRefs":{"text_to_text":{"source":"local_agent","baseUrl":"https://model.test","model":"writer","apiKey":"sk-nested"}}
	}`)
	encoded := canonicalProjectConfig(raw, model.VideoProfileCinematicStory)
	if strings.Contains(string(encoded), "sk-raw") || strings.Contains(string(encoded), "sk-nested") || strings.Contains(string(encoded), "apiKey") {
		t.Fatalf("canonical project config persisted provider credentials: %s", encoded)
	}
	var config map[string]interface{}
	if err := json.Unmarshal(encoded, &config); err != nil {
		t.Fatal(err)
	}
	if _, exists := config["modelProviders"]; exists {
		t.Fatalf("legacy raw provider config must be removed: %+v", config)
	}
	ref := config["modelProviderRefs"].(map[string]interface{})["text_to_text"].(map[string]interface{})
	if ref["source"] != "local_agent" || ref["baseUrl"] != "https://model.test" || ref["model"] != "writer" {
		t.Fatalf("non-secret provider reference was not preserved: %+v", ref)
	}
}

func TestIsValidGenerationMode(t *testing.T) {
	tests := []struct {
		mode     model.GenerationMode
		expected bool
	}{
		{model.GenProviderAPI, true},
		{model.GenManualImport, true},
		{model.GenerationMode("invalid"), false},
		{model.GenerationMode(""), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.mode), func(t *testing.T) {
			if got := model.IsValidGenerationMode(tt.mode); got != tt.expected {
				t.Errorf("IsValidGenerationMode(%q) = %v, want %v", tt.mode, got, tt.expected)
			}
		})
	}
}

func TestCreateProjectRequest_ModeRequired(t *testing.T) {
	req := &model.CreateProjectRequest{
		Name:         "Test Project",
		Mode:         model.ModeAIGCShot,
		SkillName:    "aigc-shot-video",
		SkillVersion: "1.0.0",
	}
	if req.Name == "" {
		t.Error("name should not be empty")
	}
	if !model.IsValidMode(req.Mode) {
		t.Error("mode should be valid")
	}
}

func TestProjectStatusValues(t *testing.T) {
	statuses := []model.ProjectStatus{
		model.StatusDraft,
		model.StatusRunning,
		model.StatusPaused,
		model.StatusCompleted,
		model.StatusArchived,
	}
	for _, s := range statuses {
		if s == "" {
			t.Error("status should not be empty")
		}
	}
}

func TestVideoProjectModel(t *testing.T) {
	p := &model.VideoProject{
		ID:              "vp-test",
		Name:            "Test Project",
		Mode:            model.ModeAIGCShot,
		Status:          model.StatusDraft,
		SkillName:       "aigc-shot-video",
		SkillVersion:    "1.0.0",
		WorkflowName:    "aigc-shot-video-workflow",
		WorkflowVersion: "1.0.0",
	}
	if p.Mode != model.ModeAIGCShot {
		t.Errorf("expected aigc_shot, got %s", p.Mode)
	}
	if p.SkillName != "aigc-shot-video" {
		t.Errorf("expected aigc-shot-video, got %s", p.SkillName)
	}
}
