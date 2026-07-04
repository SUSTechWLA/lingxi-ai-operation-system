package intent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/tangying-ai/aios-core/internal/core/artifact"
)

func TestInferVoiceVisualIntentFromSpokenKnowledgeRequest(t *testing.T) {
	got, err := Infer(context.Background(), "我想做一期90秒口播知识视频，讲AI Agent替代的是工作流程，发小红书和B站")
	if err != nil {
		t.Fatalf("Infer returned error: %v", err)
	}
	if got.VideoType != VideoTypeVoiceVisual {
		t.Fatalf("videoType = %q, want %q", got.VideoType, VideoTypeVoiceVisual)
	}
	if got.Topic == "" || got.DurationSec != 90 {
		t.Fatalf("intent did not extract topic/duration: %+v", got)
	}
	if !contains(got.Platforms, "xiaohongshu") || !contains(got.Platforms, "bilibili") {
		t.Fatalf("platforms = %+v, want xiaohongshu and bilibili", got.Platforms)
	}
	for _, required := range []string{"creative_brief", "voiceover_script", "beat_plan", "visual_component_plan", "publish_copy"} {
		if !contains(got.RequiredArtifacts, required) {
			t.Fatalf("requiredArtifacts missing %q: %+v", required, got.RequiredArtifacts)
		}
	}
	if !contains(got.ExcludedCapabilities, "ImageBind") || !contains(got.ExcludedCapabilities, "VideoRAG") {
		t.Fatalf("excludedCapabilities must include heavy VideoAgent dependencies: %+v", got.ExcludedCapabilities)
	}
}

func TestInferAIGCShotIntentFromStoryRequest(t *testing.T) {
	got, err := Infer(context.Background(), "写一个有角色和场景的镜头式AI短片故事，时长60秒，发抖音")
	if err != nil {
		t.Fatalf("Infer returned error: %v", err)
	}
	if got.VideoType != VideoTypeAIGCShot {
		t.Fatalf("videoType = %q, want %q", got.VideoType, VideoTypeAIGCShot)
	}
	if got.DurationSec != 60 || !contains(got.Platforms, "douyin") {
		t.Fatalf("intent did not extract duration/platform: %+v", got)
	}
	for _, required := range []string{"story_outline", "script", "character_bible", "scene_bible", "shot_list", "video_prompt", "publish_copy"} {
		if !contains(got.RequiredArtifacts, required) {
			t.Fatalf("requiredArtifacts missing %q: %+v", required, got.RequiredArtifacts)
		}
	}
}

func TestBuildArtifactRequestForVideoIntent(t *testing.T) {
	intent := VideoIntent{
		VideoType: VideoTypeVoiceVisual,
		Topic:     "AI workflows",
		Platforms: []string{"xiaohongshu"},
	}
	req, err := BuildArtifactRequest("project-1", "run-1", "task-1", intent)
	if err != nil {
		t.Fatalf("BuildArtifactRequest returned error: %v", err)
	}
	if req.ProjectID != "project-1" || req.StageName != "video_intent" || req.UnitID != "video_intent" {
		t.Fatalf("unexpected artifact scope: %+v", req)
	}
	if req.Kind != artifact.KindJSON || req.Name != "video_intent.json" {
		t.Fatalf("unexpected artifact kind/name: %+v", req)
	}
	var decoded VideoIntent
	if err := json.Unmarshal(req.Data, &decoded); err != nil {
		t.Fatalf("artifact data is not VideoIntent JSON: %v", err)
	}
	if decoded.Topic != intent.Topic {
		t.Fatalf("artifact intent topic = %q, want %q", decoded.Topic, intent.Topic)
	}
}

func TestServiceInferAndSaveWritesVideoIntentArtifact(t *testing.T) {
	sink := &fakeArtifactSink{}
	svc := NewService(sink)
	got, artifactRecord, err := svc.InferAndSave(context.Background(), SaveRequest{
		ProjectID: "project-1",
		TaskID:    "task-1",
		Raw:       "做一期口播知识视频，讲AI工作流",
	})
	if err != nil {
		t.Fatalf("InferAndSave returned error: %v", err)
	}
	if got.VideoType != VideoTypeVoiceVisual {
		t.Fatalf("videoType = %q", got.VideoType)
	}
	if artifactRecord == nil || artifactRecord.StageName != "video_intent" {
		t.Fatalf("unexpected artifact record: %+v", artifactRecord)
	}
	if sink.lastReq == nil || sink.lastReq.UnitID != "video_intent" {
		t.Fatalf("artifact sink was not called with video_intent: %+v", sink.lastReq)
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

type fakeArtifactSink struct {
	lastReq *artifact.CreateArtifactRequest
}

func (s *fakeArtifactSink) CreateArtifact(_ context.Context, req *artifact.CreateArtifactRequest) (*artifact.Artifact, error) {
	s.lastReq = req
	return &artifact.Artifact{
		ID:          "artifact-1",
		ProjectID:   req.ProjectID,
		StageName:   req.StageName,
		UnitID:      req.UnitID,
		Kind:        req.Kind,
		Name:        req.Name,
		StorageType: req.StorageType,
	}, nil
}
