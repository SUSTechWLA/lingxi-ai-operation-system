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
	case isPublishPackScriptLedOverride(req):
		return talkingHeadProfileWithMetadata(req, 0.74, "publish pack brief is script-led", false, "")
	case isCinematicProfileRequest(req):
		return cinematicStoryProfile(req)
	case isTalkingHeadProfileRequest(req):
		return talkingHeadProfile(req)
	default:
		return fallbackTalkingHeadProfile(req)
	}
}

func talkingHeadProfile(req ProfileRequest) model.VideoCreationProfile {
	return talkingHeadProfileWithMetadata(req, 0.84, "brief is script-led", false, model.VideoProfileTalkingHead)
}

func talkingHeadProfileWithMetadata(req ProfileRequest, confidence float64, reason string, needsUserReview bool, fallbackProfile string) model.VideoCreationProfile {
	return model.VideoCreationProfile{
		ProfileID:       model.VideoProfileTalkingHead,
		SourceRoute:     strings.TrimSpace(req.Route),
		PrimaryArtifact: "VIDEO_SCRIPT",
		QualityContract: []string{
			"script_timeline_alignment",
			"caption_coverage",
			"visuals_support_script",
			"exact_text_in_hyperframes",
			"aigc_time_windows_3_15s",
		},
		DAGTemplateID: "talking_head_v1",
		ReviewGatePolicy: []string{
			"script_review",
			"visual_alignment_review",
			"preview_review",
		},
		ToolBias: map[string]string{
			"primaryRenderer": "hyperframes",
			"aigcUse":         "optional_broll_or_concept_visual",
		},
		FallbackProfile: fallbackProfile,
		Confidence:      confidence,
		Reason:          reason,
		NeedsUserReview: needsUserReview,
	}
}

func cinematicStoryProfile(req ProfileRequest) model.VideoCreationProfile {
	return model.VideoCreationProfile{
		ProfileID:       model.VideoProfileCinematicStory,
		SourceRoute:     strings.TrimSpace(req.Route),
		PrimaryArtifact: "CONTINUITY_BIBLE",
		QualityContract: []string{
			"character_scene_prop_consistency",
			"director_reasoning_per_shot",
			"aigc_time_windows_3_15s",
			"time_window_first_frame_lock",
			"sound_design_per_shot",
		},
		DAGTemplateID: "cinematic_story_v1",
		ReviewGatePolicy: []string{
			"story_review",
			"continuity_bible_review",
			"reference_asset_review",
			"shot_design_review",
			"keyframe_storyboard_review",
			"director_cut_review",
		},
		ToolBias: map[string]string{
			"primaryRenderer": "aigc_then_assembly",
			"hyperframesUse":  "deterministic_text_overlay",
		},
		FallbackProfile: model.VideoProfileTalkingHead,
		Confidence:      0.82,
		Reason:          "brief requires cinematic continuity",
	}
}

func fallbackTalkingHeadProfile(req ProfileRequest) model.VideoCreationProfile {
	return talkingHeadProfileWithMetadata(
		req,
		0.56,
		"low-confidence fallback to script-led preview",
		true,
		model.VideoProfileCinematicStory,
	)
}

func isPublishPackScriptLedOverride(req ProfileRequest) bool {
	return normalizeProfileText(req.Deliverable) == "publish_pack" &&
		isCinematicRoute(req.Route) &&
		isTalkingHeadBrief(req.Brief) &&
		!isCinematicBrief(req.Brief)
}

func isTalkingHeadProfileRequest(req ProfileRequest) bool {
	return isTalkingHeadRoute(req.Route) || isTalkingHeadBrief(req.Brief)
}

func isCinematicProfileRequest(req ProfileRequest) bool {
	return isCinematicRoute(req.Route) || isCinematicBrief(req.Brief)
}

func isTalkingHeadRoute(route string) bool {
	route = normalizeProfileText(route)
	return route == "talking_head" || route == "talking-head"
}

func isCinematicRoute(route string) bool {
	route = normalizeProfileText(route)
	switch route {
	case "cinematic_short", "cinematic-story", "cinematic_story", "director_pipeline", "director-pipeline":
		return true
	default:
		return false
	}
}

func isTalkingHeadBrief(brief string) bool {
	return containsAnyString(brief, []string{
		"口播",
		"观点",
		"讲",
		"解释",
		"知识",
		"解说",
		"图文",
		"小红书",
		"b站",
		"talking head",
		"talking_head",
	})
}

func isCinematicBrief(brief string) bool {
	return containsAnyString(brief, []string{
		"影视",
		"剧情",
		"短片",
		"角色",
		"场景",
		"道具",
		"导演",
		"镜头",
		"故事",
		"连续性",
		"影视短片",
		"电影感",
		"cinematic",
	})
}

func containsAnyString(value string, candidates []string) bool {
	value = normalizeProfileText(value)
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
