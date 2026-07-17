package model

const TalkingHeadSchemaVersion = 2

const ArtifactKindAudioMasterTimeline = "AUDIO_MASTER_TIMELINE"

type TimelineSource string

const (
	TimelineSourceEstimated TimelineSource = "estimated"
	TimelineSourceAligned   TimelineSource = "aligned"
	TimelineSourceImported  TimelineSource = "imported"
)

// TimedTextCue is a canonical millisecond cue bound to exactly one audio-master
// revision. Optional marker fields keep older and less capable aligners valid.
type TimedTextCue struct {
	ID               string   `json:"id"`
	Index            int      `json:"index,omitempty"`
	Text             string   `json:"text"`
	StartMs          int64    `json:"startMs"`
	EndMs            int64    `json:"endMs"`
	DurationMs       int64    `json:"durationMs"`
	TimelineRevision string   `json:"timelineRevision"`
	Confidence       *float64 `json:"confidence,omitempty"`
	Emphasis         string   `json:"emphasis,omitempty"`
	Emotion          string   `json:"emotion,omitempty"`
	Tone             string   `json:"tone,omitempty"`
}

type TimelineMarker struct {
	ID               string `json:"id,omitempty"`
	Type             string `json:"type"`
	AtMs             int64  `json:"atMs"`
	EndMs            int64  `json:"endMs,omitempty"`
	Value            string `json:"value,omitempty"`
	TimelineRevision string `json:"timelineRevision"`
}

// AudioMasterTimeline is the only canonical clock for talking-head production.
// Seconds remain only on legacy adapters such as ScriptSpan and ShotUnit.
type AudioMasterTimeline struct {
	SchemaVersion        int              `json:"schemaVersion"`
	ScriptRevision       string           `json:"scriptRevision"`
	VoiceRevision        string           `json:"voiceRevision"`
	Revision             string           `json:"revision"`
	Fingerprint          string           `json:"fingerprint"`
	TimelineSource       TimelineSource   `json:"timelineSource"`
	Estimated            bool             `json:"estimated"`
	VoiceoverArtifactRef string           `json:"voiceoverArtifactRef,omitempty"`
	VoiceProfileID       string           `json:"voiceProfileId,omitempty"`
	VoiceProfileVersion  string           `json:"voiceProfileVersion,omitempty"`
	Language             string           `json:"language,omitempty"`
	SampleRate           int              `json:"sampleRate,omitempty"`
	DurationMs           int64            `json:"durationMs"`
	Sentences            []TimedTextCue   `json:"sentences,omitempty"`
	Words                []TimedTextCue   `json:"words,omitempty"`
	Phonemes             []TimedTextCue   `json:"phonemes,omitempty"`
	Visemes              []TimedTextCue   `json:"visemes,omitempty"`
	PauseMarkers         []TimelineMarker `json:"pauseMarkers,omitempty"`
	EmphasisMarkers      []TimelineMarker `json:"emphasisMarkers,omitempty"`
	EmotionMarkers       []TimelineMarker `json:"emotionMarkers,omitempty"`
	Provider             string           `json:"provider,omitempty"`
	Model                string           `json:"model,omitempty"`
	ToolVersion          string           `json:"toolVersion,omitempty"`
}
