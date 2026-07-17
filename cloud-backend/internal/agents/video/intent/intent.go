package intent

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/tangying-ai/aios-core/internal/agents/video/inputresolver"
	"github.com/tangying-ai/aios-core/internal/core/artifact"
)

type VideoType string

const (
	VideoTypeVoiceVisual VideoType = "voice_visual"
	VideoTypeAIGCShot    VideoType = "aigc_shot"
)

type VideoIntent struct {
	VideoType            VideoType `json:"videoType"`
	Topic                string    `json:"topic"`
	Platforms            []string  `json:"platforms"`
	DurationSec          int       `json:"durationSec"`
	Language             string    `json:"language,omitempty"`
	RequiredArtifacts    []string  `json:"requiredArtifacts"`
	ExcludedCapabilities []string  `json:"excludedCapabilities"`
	Reasoning            string    `json:"reasoning"`
}

type ArtifactSink interface {
	CreateArtifact(ctx context.Context, req *artifact.CreateArtifactRequest) (*artifact.Artifact, error)
}

type Service struct {
	sink ArtifactSink
}

type SaveRequest struct {
	ProjectID     string `json:"projectId,omitempty"`
	WorkflowRunID string `json:"workflowRunId,omitempty"`
	TaskID        string `json:"taskId,omitempty"`
	Raw           string `json:"raw"`
}

func NewService(sink ArtifactSink) *Service {
	return &Service{sink: sink}
}

func (s *Service) InferAndSave(ctx context.Context, req SaveRequest) (VideoIntent, *artifact.Artifact, error) {
	if s == nil || s.sink == nil {
		return VideoIntent{}, nil, fmt.Errorf("artifact sink is required")
	}
	videoIntent, err := Infer(ctx, req.Raw)
	if err != nil {
		return VideoIntent{}, nil, err
	}
	artifactReq, err := BuildArtifactRequest(req.ProjectID, req.WorkflowRunID, req.TaskID, videoIntent)
	if err != nil {
		return VideoIntent{}, nil, err
	}
	record, err := s.sink.CreateArtifact(ctx, artifactReq)
	if err != nil {
		return VideoIntent{}, nil, err
	}
	return videoIntent, record, nil
}

func Infer(_ context.Context, raw string) (VideoIntent, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return VideoIntent{}, fmt.Errorf("raw requirement is required")
	}

	videoType := inferVideoType(raw)
	resolved := inputresolver.Resolve(inputresolver.Request{
		Topic:       inferTopic(raw),
		VideoType:   string(videoType),
		Platforms:   inferPlatforms(raw),
		DurationSec: inferDuration(raw),
	})

	return VideoIntent{
		VideoType:            videoType,
		Topic:                resolved.Topic,
		Platforms:            resolved.Platforms,
		DurationSec:          resolved.DurationSec,
		Language:             resolved.Language,
		RequiredArtifacts:    requiredArtifacts(videoType),
		ExcludedCapabilities: excludedCapabilities(),
		Reasoning:            reasoning(videoType, raw),
	}, nil
}

func BuildArtifactRequest(projectID, workflowRunID, taskID string, intent VideoIntent) (*artifact.CreateArtifactRequest, error) {
	data, err := json.MarshalIndent(intent, "", "  ")
	if err != nil {
		return nil, err
	}
	return &artifact.CreateArtifactRequest{
		ProjectID:     projectID,
		WorkflowRunID: workflowRunID,
		TaskID:        taskID,
		StageName:     "video_intent",
		UnitID:        "video_intent",
		Kind:          artifact.KindJSON,
		Name:          "video_intent.json",
		StorageType:   artifact.StorageLocal,
		Data:          data,
		MimeType:      "application/json",
		SizeBytes:     int64(len(data)),
		Provider:      "video-intent",
		Metadata: map[string]interface{}{
			"artifactType": "video_intent",
			"videoType":    string(intent.VideoType),
		},
	}, nil
}

func inferVideoType(raw string) VideoType {
	lower := strings.ToLower(raw)
	aigcHints := []string{"故事", "剧情", "角色", "场景", "镜头", "分镜", "短片", "aigc", "shot", "story", "character", "scene"}
	for _, hint := range aigcHints {
		if strings.Contains(lower, strings.ToLower(hint)) {
			return VideoTypeAIGCShot
		}
	}
	return VideoTypeVoiceVisual
}

func inferTopic(raw string) string {
	topic := strings.TrimSpace(raw)
	for _, cut := range []string{"，发", ",发", " 发", "，时长", ",时长", " 时长"} {
		if idx := strings.Index(topic, cut); idx > 0 {
			topic = topic[:idx]
		}
	}
	return strings.Trim(topic, " ，,。.")
}

func inferPlatforms(raw string) []string {
	platforms := []string{}
	add := func(platform string) {
		for _, existing := range platforms {
			if existing == platform {
				return
			}
		}
		platforms = append(platforms, platform)
	}
	if strings.Contains(raw, "小红书") || strings.Contains(strings.ToLower(raw), "xiaohongshu") {
		add("xiaohongshu")
	}
	if strings.Contains(raw, "B站") || strings.Contains(raw, "b站") || strings.Contains(strings.ToLower(raw), "bilibili") {
		add("bilibili")
	}
	if strings.Contains(raw, "抖音") || strings.Contains(strings.ToLower(raw), "douyin") {
		add("douyin")
	}
	if strings.Contains(raw, "视频号") {
		add("wechat_channels")
	}
	return platforms
}

func inferDuration(raw string) int {
	re := regexp.MustCompile(`([0-9]{1,4})\s*(秒|s|S|sec|seconds|分钟|分|min|minutes)`)
	match := re.FindStringSubmatch(raw)
	if len(match) < 3 {
		return 0
	}
	value, _ := strconv.Atoi(match[1])
	switch strings.ToLower(match[2]) {
	case "分钟", "分", "min", "minutes":
		return value * 60
	default:
		return value
	}
}

func requiredArtifacts(videoType VideoType) []string {
	if videoType == VideoTypeAIGCShot {
		return []string{
			"creative_brief",
			"story_outline",
			"script",
			"character_bible",
			"scene_bible",
			"shot_list",
			"keyframe_prompt",
			"video_prompt",
			"publish_copy",
		}
	}
	return []string{
		"creative_brief",
		"voiceover_script",
		"beat_plan",
		"visual_component_plan",
		"hyperframes_project",
		"publish_copy",
	}
}

func excludedCapabilities() []string {
	return []string{"CosyVoice", "DiffSinger", "ImageBind", "fish-speech", "seed-vc", "VideoRAG", "auto_publish", "long_video_understanding"}
}

func reasoning(videoType VideoType, raw string) string {
	if videoType == VideoTypeAIGCShot {
		return "Request contains story/shot/scene signals, so beta routes it to aigc_shot without adding heavy video understanding dependencies."
	}
	return "Request fits spoken knowledge/opinion video production, so beta routes it to voice_visual and reuses the existing HyperFrames path."
}
