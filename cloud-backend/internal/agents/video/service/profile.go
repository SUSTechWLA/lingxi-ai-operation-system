package service

import (
	"strings"

	"github.com/tangying-ai/aios-core/internal/agents/video/model"
)

type ProfileRequest struct {
	Route       string
	Deliverable string
	Brief       string
}

func BuildVideoCreationProfile(req ProfileRequest) model.VideoCreationProfile {
	switch {
	case isCinematicProfileRequest(req):
		return cinematicStoryProfile(req)
	case isTalkingHeadProfileRequest(req):
		return talkingHeadProfile(req)
	default:
		profile := talkingHeadProfile(req)
		profile.Confidence = 0.58
		profile.Reason = "defaulted to talking-head profile"
		profile.NeedsUserReview = true
		return profile
	}
}

func talkingHeadProfile(req ProfileRequest) model.VideoCreationProfile {
	return model.VideoCreationProfile{
		ProfileID:       model.VideoProfileTalkingHead,
		SourceRoute:     strings.TrimSpace(req.Route),
		PrimaryArtifact: "VIDEO_SCRIPT",
		QualityContract: []string{
			"script_timeline_alignment",
			"exact_text_rendering",
			"shot_duration_3_15s",
		},
		DAGTemplateID: "talking_head_v1",
		ReviewGatePolicy: []string{
			"script_review",
			"shot_plan_review",
			"final_publish_pack_review",
		},
		ToolBias: map[string]string{
			"script":  "script_writer",
			"preview": "hyperframes_renderer",
		},
		FallbackProfile: model.VideoProfileTalkingHead,
		Confidence:      0.9,
		Reason:          "talking-head route or brief detected",
	}
}

func cinematicStoryProfile(req ProfileRequest) model.VideoCreationProfile {
	return model.VideoCreationProfile{
		ProfileID:       model.VideoProfileCinematicStory,
		SourceRoute:     strings.TrimSpace(req.Route),
		PrimaryArtifact: "CONTINUITY_BIBLE",
		QualityContract: []string{
			"aigc_time_windows_3_15s",
			"character_continuity",
			"scene_prop_continuity",
			"shot_duration_3_15s",
		},
		DAGTemplateID: "cinematic_story_v1",
		ReviewGatePolicy: []string{
			"continuity_bible_review",
			"keyframe_review",
			"video_prompt_review",
		},
		ToolBias: map[string]string{
			"continuity": "continuity_bible_builder",
			"prompt":     "video_prompt_writer",
		},
		FallbackProfile: model.VideoProfileTalkingHead,
		Confidence:      0.9,
		Reason:          "cinematic route or brief detected",
	}
}

func isTalkingHeadProfileRequest(req ProfileRequest) bool {
	route := normalizeProfileText(req.Route)
	if route == "talking_head" || route == "talking-head" {
		return true
	}
	return containsAnyString(normalizeProfileText(req.Brief), []string{
		"口播",
		"知识视频",
		"讲解",
		"talking head",
		"talking_head",
	})
}

func isCinematicProfileRequest(req ProfileRequest) bool {
	route := normalizeProfileText(req.Route)
	if route == "cinematic_short" || route == "cinematic_story" {
		return true
	}
	return containsAnyString(normalizeProfileText(req.Brief), []string{
		"影视短片",
		"电影感",
		"角色",
		"场景",
		"道具",
		"连续性",
		"cinematic",
	})
}

func containsAnyString(value string, candidates []string) bool {
	for _, candidate := range candidates {
		if strings.Contains(value, normalizeProfileText(candidate)) {
			return true
		}
	}
	return false
}

func normalizeProfileText(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
