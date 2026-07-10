package model

import "strings"

const (
	VideoRuntimePipelineID      = "dynamic-agent-video-creation"
	VideoRuntimePipelineVersion = "2.0"
	VideoRuntimePipelineSource  = "agentruntime.PlanCompiler"
	VideoProfileSchemaVersion   = 2
)

// NormalizeVideoProfileID maps historical profile, project-mode, workflow, and
// pipeline identifiers to the two canonical production profile IDs. It does
// not rewrite persisted project modes; callers use PersistedModeForProfile at
// the storage boundary.
func NormalizeVideoProfileID(value string) (string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case VideoProfileTalkingHead,
		"talking-head",
		"voice_visual",
		"voice-visual",
		"voiceover",
		"knowledge-video",
		"guided_image_text",
		"guided-image-text",
		"guided-image-text-video",
		"wf-guided-image-text-video":
		return VideoProfileTalkingHead, true
	case VideoProfileCinematicStory,
		"cinematic-story",
		"cinematic_short",
		"cinematic-short",
		"aigc_shot",
		"aigc-shot",
		"aigc-shot-video",
		"wf-aigc-shot-video",
		"cinematic-aigc-shot-video",
		"director_pipeline",
		"director-pipeline":
		return VideoProfileCinematicStory, true
	default:
		return "", false
	}
}

// PersistedModeForProfile returns the legacy project mode used by existing
// rows and clients. New canonical profile IDs therefore remain compatible with
// existing video_projects data.
func PersistedModeForProfile(value string) (VideoMode, bool) {
	profileID, ok := NormalizeVideoProfileID(value)
	if !ok {
		return "", false
	}
	if profileID == VideoProfileCinematicStory {
		return ModeAIGCShot, true
	}
	return ModeVoiceVisual, true
}

func CanonicalProfileForMode(mode VideoMode) string {
	profileID, _ := NormalizeVideoProfileID(string(mode))
	return profileID
}
