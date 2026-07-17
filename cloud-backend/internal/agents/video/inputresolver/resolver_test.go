package inputresolver

import "testing"

func TestResolveDefaultsPlatformDurationAndLanguage(t *testing.T) {
	got := Resolve(Request{Topic: "AI workflows", VideoType: "voice_visual"})
	if got.NeedsClarification {
		t.Fatalf("did not expect clarification: %+v", got)
	}
	if got.Platforms[0] != "xiaohongshu" || got.DurationSec != 90 || got.Language != "zh-CN" {
		t.Fatalf("defaults not applied: %+v", got)
	}
}

func TestResolveRequiresClarificationWhenTopicMissing(t *testing.T) {
	got := Resolve(Request{VideoType: "voice_visual"})
	if !got.NeedsClarification {
		t.Fatalf("expected clarification for missing topic: %+v", got)
	}
	if !contains(got.MissingFields, "topic") {
		t.Fatalf("missing fields = %+v, want topic", got.MissingFields)
	}
	if len(got.Questions) == 0 {
		t.Fatalf("expected a clarification question")
	}
}

func TestResolveAcceptsCinematicStoryVideoType(t *testing.T) {
	got := Resolve(Request{Topic: "AI workflows as a cinematic short", VideoType: "cinematic_story"})
	if got.NeedsClarification {
		t.Fatalf("did not expect clarification for cinematic story: %+v", got)
	}
}

func TestResolveRejectsUnsupportedVideoType(t *testing.T) {
	got := Resolve(Request{Topic: "AI workflows", VideoType: "long_video_qa"})
	if !got.NeedsClarification {
		t.Fatalf("expected clarification for unsupported video type: %+v", got)
	}
	if !contains(got.MissingFields, "videoType") {
		t.Fatalf("missing fields = %+v, want videoType", got.MissingFields)
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
