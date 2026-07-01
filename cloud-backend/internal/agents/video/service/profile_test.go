package service

import (
	"testing"

	"github.com/tangying-ai/aios-core/internal/agents/video/model"
)

func TestBuildVideoCreationProfileTalkingHead(t *testing.T) {
	profile := BuildVideoCreationProfile(ProfileRequest{
		Route:       "talking_head",
		Deliverable: "publish_pack",
		Brief:       "做一期60秒口播知识视频，讲AI工作流",
	})

	if profile.ProfileID != model.VideoProfileTalkingHead {
		t.Fatalf("profile = %s, want %s", profile.ProfileID, model.VideoProfileTalkingHead)
	}
	if profile.PrimaryArtifact != "VIDEO_SCRIPT" {
		t.Fatalf("primary artifact = %s", profile.PrimaryArtifact)
	}
	if !containsString(profile.QualityContract, "script_timeline_alignment") {
		t.Fatalf("talking-head quality contract should include script timeline alignment: %#v", profile.QualityContract)
	}
	if profile.DAGTemplateID != "talking_head_v1" {
		t.Fatalf("dag template = %s", profile.DAGTemplateID)
	}
	if profile.FallbackProfile != "" {
		t.Fatalf("normal talking-head fallback profile = %s, want empty", profile.FallbackProfile)
	}
}

func TestBuildVideoCreationProfileCinematic(t *testing.T) {
	profile := BuildVideoCreationProfile(ProfileRequest{
		Route:       "cinematic_short",
		Deliverable: "video_prompt",
		Brief:       "做一个有角色、场景和道具连续性的影视短片",
	})

	if profile.ProfileID != model.VideoProfileCinematicStory {
		t.Fatalf("profile = %s, want %s", profile.ProfileID, model.VideoProfileCinematicStory)
	}
	if profile.PrimaryArtifact != "CONTINUITY_BIBLE" {
		t.Fatalf("primary artifact = %s", profile.PrimaryArtifact)
	}
	if !containsString(profile.QualityContract, "aigc_time_windows_3_15s") {
		t.Fatalf("cinematic quality contract should include AIGC time-window limit: %#v", profile.QualityContract)
	}
	if profile.DAGTemplateID != "cinematic_story_v1" {
		t.Fatalf("dag template = %s", profile.DAGTemplateID)
	}
	if profile.FallbackProfile != "" {
		t.Fatalf("normal cinematic fallback profile = %s, want empty", profile.FallbackProfile)
	}
}

func TestVideoCreationProfileDirectorPipelineUsesCinematicContract(t *testing.T) {
	profile := BuildVideoCreationProfile(ProfileRequest{
		Route:       "director_pipeline",
		Deliverable: "video_prompt",
		Brief:       "按流程做一个视频",
	})

	if profile.ProfileID != model.VideoProfileCinematicStory {
		t.Fatalf("profile = %s, want %s", profile.ProfileID, model.VideoProfileCinematicStory)
	}
	if profile.Confidence != 0.82 {
		t.Fatalf("confidence = %v, want 0.82", profile.Confidence)
	}
	if profile.Reason != "brief requires cinematic continuity" {
		t.Fatalf("reason = %q", profile.Reason)
	}
	assertContainsAllStrings(t, profile.QualityContract, []string{
		"character_scene_prop_consistency",
		"director_reasoning_per_shot",
		"aigc_time_windows_3_15s",
		"time_window_first_frame_lock",
		"sound_design_per_shot",
	})
	assertContainsAllStrings(t, profile.ReviewGatePolicy, []string{
		"story_review",
		"continuity_bible_review",
		"reference_asset_review",
		"shot_design_review",
		"keyframe_storyboard_review",
		"director_cut_review",
	})
	if profile.ToolBias["primaryRenderer"] != "aigc_then_assembly" {
		t.Fatalf("primaryRenderer = %q", profile.ToolBias["primaryRenderer"])
	}
	if profile.ToolBias["hyperframesUse"] != "deterministic_text_overlay" {
		t.Fatalf("hyperframesUse = %q", profile.ToolBias["hyperframesUse"])
	}
}

func TestVideoCreationProfileExplicitTalkingHeadRouteBeatsIncidentalCinematicBrief(t *testing.T) {
	profile := BuildVideoCreationProfile(ProfileRequest{
		Route: "talking_head",
		Brief: "口播讲一个创业故事，镜头感强",
	})

	if profile.ProfileID != model.VideoProfileTalkingHead {
		t.Fatalf("profile = %s, want %s", profile.ProfileID, model.VideoProfileTalkingHead)
	}
	if profile.Confidence != 0.84 {
		t.Fatalf("confidence = %v, want 0.84", profile.Confidence)
	}
	if profile.Reason != "brief is script-led" {
		t.Fatalf("reason = %q", profile.Reason)
	}
}

func TestVideoCreationProfileFallbackMetadata(t *testing.T) {
	profile := BuildVideoCreationProfile(ProfileRequest{
		Route:       "unknown",
		Deliverable: "unknown",
		Brief:       "做一个内容",
	})

	if profile.ProfileID != model.VideoProfileTalkingHead {
		t.Fatalf("profile = %s, want %s", profile.ProfileID, model.VideoProfileTalkingHead)
	}
	if profile.Confidence != 0.56 {
		t.Fatalf("confidence = %v, want 0.56", profile.Confidence)
	}
	if profile.Reason != "low-confidence fallback to script-led preview" {
		t.Fatalf("reason = %q", profile.Reason)
	}
	if !profile.NeedsUserReview {
		t.Fatalf("NeedsUserReview = false, want true")
	}
	if profile.FallbackProfile != model.VideoProfileCinematicStory {
		t.Fatalf("FallbackProfile = %s, want %s", profile.FallbackProfile, model.VideoProfileCinematicStory)
	}
}

func TestVideoCreationProfileTalkingHeadContractGatesAndToolBias(t *testing.T) {
	profile := BuildVideoCreationProfile(ProfileRequest{
		Route:       "",
		Deliverable: "video_script",
		Brief:       "做一条观点解说，解释AI工作流",
	})

	if profile.ProfileID != model.VideoProfileTalkingHead {
		t.Fatalf("profile = %s, want %s", profile.ProfileID, model.VideoProfileTalkingHead)
	}
	if profile.Confidence != 0.84 {
		t.Fatalf("confidence = %v, want 0.84", profile.Confidence)
	}
	if profile.Reason != "brief is script-led" {
		t.Fatalf("reason = %q", profile.Reason)
	}
	assertContainsAllStrings(t, profile.QualityContract, []string{
		"script_timeline_alignment",
		"caption_coverage",
		"visuals_support_script",
		"exact_text_in_hyperframes",
		"aigc_time_windows_3_15s",
	})
	assertContainsAllStrings(t, profile.ReviewGatePolicy, []string{
		"script_review",
		"visual_alignment_review",
		"preview_review",
	})
	if profile.ToolBias["primaryRenderer"] != "hyperframes" {
		t.Fatalf("primaryRenderer = %q", profile.ToolBias["primaryRenderer"])
	}
	if profile.ToolBias["aigcUse"] != "optional_broll_or_concept_visual" {
		t.Fatalf("aigcUse = %q", profile.ToolBias["aigcUse"])
	}
}

func TestVideoCreationProfilePublishPackScriptLedOverridesCinematicRoute(t *testing.T) {
	profile := BuildVideoCreationProfile(ProfileRequest{
		Route:       "cinematic_short",
		Deliverable: "publish_pack",
		Brief:       "做一期观点口播，讲AI工作流",
	})

	if profile.ProfileID != model.VideoProfileTalkingHead {
		t.Fatalf("profile = %s, want %s", profile.ProfileID, model.VideoProfileTalkingHead)
	}
	if profile.Confidence != 0.74 {
		t.Fatalf("confidence = %v, want 0.74", profile.Confidence)
	}
	if profile.Reason != "publish pack brief is script-led" {
		t.Fatalf("reason = %q", profile.Reason)
	}
}

func TestVideoCreationProfileKeywordCoverage(t *testing.T) {
	for _, brief := range []string{
		"做一期口播视频",
		"做一期观点视频",
		"讲AI工作流",
		"解释AI工作流",
		"做一个知识分享",
		"做一个解说",
		"做一篇图文脚本",
		"写一个小红书内容",
		"做一个B站内容",
	} {
		t.Run("talking_head_"+brief, func(t *testing.T) {
			profile := BuildVideoCreationProfile(ProfileRequest{Brief: brief})
			if profile.ProfileID != model.VideoProfileTalkingHead {
				t.Fatalf("brief %q profile = %s, want %s", brief, profile.ProfileID, model.VideoProfileTalkingHead)
			}
			if profile.Confidence != 0.84 {
				t.Fatalf("brief %q confidence = %v, want 0.84", brief, profile.Confidence)
			}
			if profile.Reason != "brief is script-led" {
				t.Fatalf("brief %q reason = %q", brief, profile.Reason)
			}
		})
	}

	for _, brief := range []string{
		"做一个影视短片",
		"做一个影视剧情短片",
		"设计角色成长",
		"设计场景变化",
		"安排道具细节",
		"用导演视角规划镜头",
		"写一个故事分镜",
		"保持连续性",
	} {
		t.Run("cinematic_"+brief, func(t *testing.T) {
			profile := BuildVideoCreationProfile(ProfileRequest{Brief: brief})
			if profile.ProfileID != model.VideoProfileCinematicStory {
				t.Fatalf("brief %q profile = %s, want %s", brief, profile.ProfileID, model.VideoProfileCinematicStory)
			}
			if profile.Confidence != 0.82 {
				t.Fatalf("brief %q confidence = %v, want 0.82", brief, profile.Confidence)
			}
			if profile.Reason != "brief requires cinematic continuity" {
				t.Fatalf("brief %q reason = %q", brief, profile.Reason)
			}
		})
	}
}

func assertContainsAllStrings(t *testing.T, values []string, wants []string) {
	t.Helper()
	for _, want := range wants {
		if !containsString(values, want) {
			t.Fatalf("missing %q in %#v", want, values)
		}
	}
}
