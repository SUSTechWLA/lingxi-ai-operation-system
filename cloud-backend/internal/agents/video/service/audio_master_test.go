package service

import (
	"encoding/json"
	"testing"

	"github.com/tangying-ai/aios-core/internal/agents/video/model"
)

func TestNormalizeLegacyShotTimingConvertsSecondsToMilliseconds(t *testing.T) {
	raw := []byte(`{"id":"SHOT_01","startSec":1.25,"endSec":7.75,"durationSec":6}`)
	var shot model.ShotUnit
	if err := json.Unmarshal(raw, &shot); err != nil {
		t.Fatal(err)
	}
	NormalizeShotTiming(&shot)
	if shot.StartMs != 1250 || shot.EndMs != 7750 || shot.DurationMs != 6500 {
		t.Fatalf("normalized timing = start %d end %d duration %d", shot.StartMs, shot.EndMs, shot.DurationMs)
	}
	if shot.SchemaVersion != model.TalkingHeadSchemaVersion {
		t.Fatalf("schema version = %d", shot.SchemaVersion)
	}
}

func TestNormalizeShotTimingPrefersCanonicalMilliseconds(t *testing.T) {
	shot := model.ShotUnit{StartMs: 500, EndMs: 7250, DurationMs: 6750, StartSec: 99, EndSec: 100, DurationSec: 100}
	NormalizeShotTiming(&shot)
	if shot.StartMs != 500 || shot.EndMs != 7250 || shot.DurationMs != 6750 {
		t.Fatalf("canonical milliseconds were overwritten: %+v", shot)
	}
	if shot.StartSec != 0.5 || shot.EndSec != 7.25 {
		t.Fatalf("legacy adapter seconds = %.3f..%.3f", shot.StartSec, shot.EndSec)
	}
}

func TestBuildEstimatedAudioMasterUsesOneRevisionForAllCues(t *testing.T) {
	master, issues := BuildAudioMasterTimeline(AudioMasterRequest{
		ScriptRevision: "script-r2",
		VoiceRevision:  "voice-estimated-r1",
		Language:       "zh-CN",
		ScriptSpans: []model.ScriptSpan{
			{ID: "s1", StartSec: 0, EndSec: 6.2, Text: "开头钩子。"},
			{ID: "s2", StartSec: 6.2, EndSec: 13, Text: "核心观点。"},
		},
	})
	if len(issues) != 0 {
		t.Fatalf("audio master issues: %+v", issues)
	}
	if master.TimelineSource != model.TimelineSourceEstimated || !master.Estimated {
		t.Fatalf("timeline source = %+v", master)
	}
	if master.Revision == "" || master.Fingerprint == "" || master.DurationMs != 13000 {
		t.Fatalf("audio master revision/fingerprint/duration missing: %+v", master)
	}
	for _, sentence := range master.Sentences {
		if sentence.TimelineRevision != master.Revision {
			t.Fatalf("sentence timeline revision = %q, want %q", sentence.TimelineRevision, master.Revision)
		}
	}
}

func TestTimeWindowPlanUsesAudioMasterRevisionAndMilliseconds(t *testing.T) {
	master, issues := BuildAudioMasterTimeline(AudioMasterRequest{
		ScriptRevision: "script-r1",
		ScriptSpans: []model.ScriptSpan{
			{ID: "s1", StartSec: 0, EndSec: 6, Text: "第一句。"},
			{ID: "s2", StartSec: 6, EndSec: 13, Text: "第二句。"},
		},
	})
	if len(issues) != 0 {
		t.Fatalf("audio master issues: %+v", issues)
	}
	plan := BuildTimeWindowPlan(TimeWindowRequest{
		Profile:     model.VideoCreationProfile{ProfileID: model.VideoProfileTalkingHead},
		AudioMaster: &master,
	})
	if plan.TimelineRevision != master.Revision || plan.SchemaVersion != model.TalkingHeadSchemaVersion {
		t.Fatalf("time window plan is not bound to audio master: %+v", plan)
	}
	if len(plan.Windows) != 2 || plan.Windows[1].StartMs != 6000 || plan.Windows[1].EndMs != 13000 {
		t.Fatalf("millisecond windows = %+v", plan.Windows)
	}
	for _, window := range plan.Windows {
		if window.TimelineRevision != master.Revision {
			t.Fatalf("window timeline revision mismatch: %+v", window)
		}
	}
}

func TestAudioMasterValidationRejectsInvalidCueAndDuration(t *testing.T) {
	master := model.AudioMasterTimeline{
		SchemaVersion: model.TalkingHeadSchemaVersion,
		Revision:      "audio-r1",
		DurationMs:    2000,
		Sentences: []model.TimedTextCue{{
			ID: "bad", StartMs: 1800, EndMs: 1000, Text: "反向时间",
		}},
	}
	issues := ValidateAudioMasterTimeline(master)
	if !hasIssueCode(issues, "audio_master_duration_too_short") || !hasIssueCode(issues, "audio_master_cue_invalid") {
		t.Fatalf("expected duration and cue issues, got %+v", issues)
	}
}
