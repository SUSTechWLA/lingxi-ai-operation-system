package model

import "testing"

func TestNormalizeVideoProfileIDAliases(t *testing.T) {
	tests := map[string]string{
		"talking_head":               VideoProfileTalkingHead,
		"voice_visual":               VideoProfileTalkingHead,
		"knowledge-video":            VideoProfileTalkingHead,
		"wf-guided-image-text-video": VideoProfileTalkingHead,
		"cinematic_story":            VideoProfileCinematicStory,
		"aigc_shot":                  VideoProfileCinematicStory,
		"wf-aigc-shot-video":         VideoProfileCinematicStory,
		"director-pipeline":          VideoProfileCinematicStory,
	}
	for input, want := range tests {
		t.Run(input, func(t *testing.T) {
			got, ok := NormalizeVideoProfileID(input)
			if !ok || got != want {
				t.Fatalf("NormalizeVideoProfileID(%q) = %q, %v; want %q, true", input, got, ok, want)
			}
		})
	}
	if got, ok := NormalizeVideoProfileID("unknown"); ok || got != "" {
		t.Fatalf("unknown profile = %q, %v; want empty, false", got, ok)
	}
}

func TestPersistedModeForCanonicalProfilePreservesLegacyStorageIDs(t *testing.T) {
	if got, ok := PersistedModeForProfile(VideoProfileTalkingHead); !ok || got != ModeVoiceVisual {
		t.Fatalf("talking-head persisted mode = %q, %v", got, ok)
	}
	if got, ok := PersistedModeForProfile(VideoProfileCinematicStory); !ok || got != ModeAIGCShot {
		t.Fatalf("cinematic persisted mode = %q, %v", got, ok)
	}
	if got := CanonicalProfileForMode(ModeVoiceVisual); got != VideoProfileTalkingHead {
		t.Fatalf("legacy voice_visual canonical profile = %q", got)
	}
}

func TestCanonicalRuntimePipelineIdentity(t *testing.T) {
	if VideoRuntimePipelineID != "dynamic-agent-video-creation" {
		t.Fatalf("runtime pipeline id = %q", VideoRuntimePipelineID)
	}
	if VideoRuntimePipelineSource != "agentruntime.PlanCompiler" {
		t.Fatalf("runtime pipeline source = %q", VideoRuntimePipelineSource)
	}
	if VideoRuntimePipelineVersion == "" {
		t.Fatal("runtime pipeline version must be explicit")
	}
}
