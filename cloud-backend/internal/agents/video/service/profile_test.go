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
}
