package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/tangying-ai/aios-core/internal/agents/video/model"
)

const defaultAudioSampleRate = 48000

type AudioMasterRequest struct {
	ScriptRevision       string
	VoiceRevision        string
	Language             string
	TimelineSource       model.TimelineSource
	VoiceoverArtifactRef string
	VoiceProfileID       string
	VoiceProfileVersion  string
	SampleRate           int
	Provider             string
	Model                string
	ToolVersion          string
	ScriptSpans          []model.ScriptSpan
}

func MillisecondsFromSeconds(seconds float64) int64 {
	return int64(math.Round(seconds * 1000))
}

func SecondsFromMilliseconds(milliseconds int64) float64 {
	return float64(milliseconds) / 1000
}

// NormalizeShotTiming migrates legacy second-based JSON at the domain boundary.
// A canonical millisecond field wins for that field, while missing fields are
// recovered independently from legacy seconds before duration is derived.
func NormalizeShotTiming(shot *model.ShotUnit) {
	if shot == nil {
		return
	}
	if shot.StartMs == 0 && shot.StartSec != 0 {
		shot.StartMs = MillisecondsFromSeconds(shot.StartSec)
	}
	if shot.EndMs == 0 && shot.EndSec != 0 {
		shot.EndMs = MillisecondsFromSeconds(shot.EndSec)
	}
	if shot.DurationMs <= 0 && shot.EndMs > shot.StartMs {
		shot.DurationMs = shot.EndMs - shot.StartMs
	}
	if shot.DurationMs <= 0 && shot.DurationSec > 0 {
		shot.DurationMs = MillisecondsFromSeconds(float64(shot.DurationSec))
	}
	if shot.EndMs <= shot.StartMs && shot.DurationMs > 0 {
		shot.EndMs = shot.StartMs + shot.DurationMs
	}
	shot.SchemaVersion = model.TalkingHeadSchemaVersion
	shot.StartSec = SecondsFromMilliseconds(shot.StartMs)
	shot.EndSec = SecondsFromMilliseconds(shot.EndMs)
	shot.DurationSec = int(math.Ceil(SecondsFromMilliseconds(shot.DurationMs)))
}

func BuildAudioMasterTimeline(req AudioMasterRequest) (model.AudioMasterTimeline, []ValidationIssue) {
	source := req.TimelineSource
	if source == "" {
		if strings.TrimSpace(req.VoiceoverArtifactRef) == "" {
			source = model.TimelineSourceEstimated
		} else {
			source = model.TimelineSourceImported
		}
	}
	language := strings.TrimSpace(req.Language)
	if language == "" {
		language = "zh-CN"
	}
	sampleRate := req.SampleRate
	if sampleRate <= 0 {
		sampleRate = defaultAudioSampleRate
	}

	fingerprintInput := struct {
		SchemaVersion        int                  `json:"schemaVersion"`
		ScriptRevision       string               `json:"scriptRevision"`
		VoiceRevision        string               `json:"voiceRevision"`
		Language             string               `json:"language"`
		TimelineSource       model.TimelineSource `json:"timelineSource"`
		VoiceoverArtifactRef string               `json:"voiceoverArtifactRef,omitempty"`
		VoiceProfileID       string               `json:"voiceProfileId,omitempty"`
		VoiceProfileVersion  string               `json:"voiceProfileVersion,omitempty"`
		SampleRate           int                  `json:"sampleRate"`
		Provider             string               `json:"provider,omitempty"`
		Model                string               `json:"model,omitempty"`
		ToolVersion          string               `json:"toolVersion,omitempty"`
		ScriptSpans          []model.ScriptSpan   `json:"scriptSpans"`
	}{
		SchemaVersion:        model.TalkingHeadSchemaVersion,
		ScriptRevision:       req.ScriptRevision,
		VoiceRevision:        req.VoiceRevision,
		Language:             language,
		TimelineSource:       source,
		VoiceoverArtifactRef: req.VoiceoverArtifactRef,
		VoiceProfileID:       req.VoiceProfileID,
		VoiceProfileVersion:  req.VoiceProfileVersion,
		SampleRate:           sampleRate,
		Provider:             req.Provider,
		Model:                req.Model,
		ToolVersion:          req.ToolVersion,
		ScriptSpans:          req.ScriptSpans,
	}
	data, _ := json.Marshal(fingerprintInput)
	sum := sha256.Sum256(data)
	fingerprint := hex.EncodeToString(sum[:])
	revision := "audio-master-" + fingerprint[:12]

	master := model.AudioMasterTimeline{
		SchemaVersion:        model.TalkingHeadSchemaVersion,
		ScriptRevision:       strings.TrimSpace(req.ScriptRevision),
		VoiceRevision:        strings.TrimSpace(req.VoiceRevision),
		Revision:             revision,
		Fingerprint:          fingerprint,
		TimelineSource:       source,
		Estimated:            source == model.TimelineSourceEstimated,
		VoiceoverArtifactRef: strings.TrimSpace(req.VoiceoverArtifactRef),
		VoiceProfileID:       strings.TrimSpace(req.VoiceProfileID),
		VoiceProfileVersion:  strings.TrimSpace(req.VoiceProfileVersion),
		Language:             language,
		SampleRate:           sampleRate,
		Provider:             strings.TrimSpace(req.Provider),
		Model:                strings.TrimSpace(req.Model),
		ToolVersion:          strings.TrimSpace(req.ToolVersion),
		Sentences:            make([]model.TimedTextCue, 0, len(req.ScriptSpans)),
	}
	for index, span := range req.ScriptSpans {
		startMs := MillisecondsFromSeconds(span.StartSec)
		endMs := MillisecondsFromSeconds(span.EndSec)
		cueID := strings.TrimSpace(span.ID)
		if cueID == "" {
			cueID = fmt.Sprintf("sentence-%02d", index+1)
		}
		master.Sentences = append(master.Sentences, model.TimedTextCue{
			ID:               cueID,
			Index:            index,
			Text:             span.Text,
			StartMs:          startMs,
			EndMs:            endMs,
			DurationMs:       endMs - startMs,
			TimelineRevision: revision,
			Emotion:          span.Emotion,
		})
		if endMs > master.DurationMs {
			master.DurationMs = endMs
		}
	}
	return master, ValidateAudioMasterTimeline(master)
}

func ValidateAudioMasterTimeline(master model.AudioMasterTimeline) []ValidationIssue {
	issues := make([]ValidationIssue, 0)
	if master.DurationMs < 3000 {
		issues = append(issues, ValidationIssue{
			Code: "audio_master_duration_too_short", Field: "durationMs",
			Message: "audio master duration must be at least 3000 milliseconds", Severity: "error",
		})
	}
	for _, cue := range master.Sentences {
		if cue.StartMs < 0 || cue.EndMs <= cue.StartMs || cue.EndMs > master.DurationMs ||
			(cue.TimelineRevision != "" && cue.TimelineRevision != master.Revision) {
			issues = append(issues, ValidationIssue{
				Code: "audio_master_cue_invalid", Field: "sentences",
				Message: fmt.Sprintf("audio cue %s is outside the master timeline or has a mismatched revision", cue.ID), Severity: "error",
			})
		}
	}
	return issues
}
