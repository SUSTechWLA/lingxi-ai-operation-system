package artifact

import (
	"reflect"
	"testing"
)

func TestShotDownstreamStagesForTextLayers(t *testing.T) {
	got := DownstreamShotStageNamesForStage("text_layers")
	want := []string{"html_source", "html_preview_video", "html_overlay_video", "html_overlay_alpha_video", "composited_shot_video"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("downstream = %#v, want %#v", got, want)
	}
}

func TestProjectStagesInvalidatedByShotChange(t *testing.T) {
	got := ProjectStageNamesForShotStageChange("aigc_prompt")
	want := []string{"final_video", "publish_package"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("project downstream = %#v, want %#v", got, want)
	}
}

func TestLegacyDownstreamStageNamesRemainUnchanged(t *testing.T) {
	got := DownstreamStageNamesForStage("script")
	want := []string{"storyboard", "composition", "reference", "continuity", "preview", "render", "quality", "package"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("legacy downstream = %#v, want %#v", got, want)
	}
}
