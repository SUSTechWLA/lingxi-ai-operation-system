package service

import (
	"encoding/json"
	"testing"

	"github.com/tangying-ai/aios-core/internal/agents/video/model"
)

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
		Name:        "Test Project",
		Mode:        model.ModeAIGCShot,
		SkillName:   "aigc-shot-video",
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
