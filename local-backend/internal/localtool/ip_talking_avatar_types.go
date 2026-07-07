package localtool

import "time"

type LocalIpTalkingAvatarRenderInput struct {
	CharacterID      string       `json:"characterId"`
	Script           string       `json:"script,omitempty"`
	AudioPath        string       `json:"audioPath,omitempty"`
	SubtitlePath     string       `json:"subtitlePath,omitempty"`
	BackgroundPath   string       `json:"backgroundPath,omitempty"`
	BgmPath          string       `json:"bgmPath,omitempty"`
	OutputDir        string       `json:"outputDir"`
	RenderMode       string       `json:"renderMode,omitempty"`
	InteractionLevel string       `json:"interactionLevel,omitempty"`
	Resolution       Resolution   `json:"resolution,omitempty"`
	FPS              int          `json:"fps,omitempty"`
	Style            RenderStyle  `json:"style,omitempty"`
	MotionPolicy     MotionPolicy `json:"motionPolicy,omitempty"`
	VoiceProfile     VoiceProfile `json:"voiceProfile,omitempty"`
}

type Resolution struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

type RenderStyle struct {
	Position               string  `json:"position,omitempty"`
	Scale                  float64 `json:"scale,omitempty"`
	SubtitleEnabled        bool    `json:"subtitleEnabled"`
	BackgroundEnabled      bool    `json:"backgroundEnabled"`
	TransparentAvatarVideo bool    `json:"transparentAvatarVideo"`
}

type MotionPolicy struct {
	AutoBlink      bool `json:"autoBlink"`
	AutoBreath     bool `json:"autoBreath"`
	SentenceNod    bool `json:"sentenceNod"`
	KeywordGesture bool `json:"keywordGesture"`
}

type VoiceProfile struct {
	Persona        string `json:"persona,omitempty"`
	DisplayName    string `json:"displayName,omitempty"`
	VoiceName      string `json:"voiceName,omitempty"`
	Locale         string `json:"locale,omitempty"`
	SpeakingRate   int    `json:"speakingRate,omitempty"`
	Tone           string `json:"tone,omitempty"`
	StylePrompt    string `json:"stylePrompt,omitempty"`
	Provider       string `json:"provider,omitempty"`
	PreviewOnly    bool   `json:"previewOnly,omitempty"`
	FallbackPolicy string `json:"fallbackPolicy,omitempty"`
}

type LocalIpTalkingAvatarRenderOutput struct {
	Success          bool           `json:"success"`
	CharacterID      string         `json:"characterId"`
	VideoPath        string         `json:"videoPath"`
	AvatarVideoPath  string         `json:"avatarVideoPath,omitempty"`
	TimelinePath     string         `json:"timelinePath,omitempty"`
	ScenePath        string         `json:"scenePath,omitempty"`
	SubtitlePath     string         `json:"subtitlePath,omitempty"`
	VoiceProfilePath string         `json:"voiceProfilePath,omitempty"`
	DurationSec      float64        `json:"durationSec"`
	QA               RenderQAResult `json:"qa"`
	ErrorMessage     string         `json:"errorMessage,omitempty"`
}

type RenderQAResult struct {
	AudioExists             bool `json:"audioExists"`
	VideoExists             bool `json:"videoExists"`
	DurationMatched         bool `json:"durationMatched"`
	SubtitleExists          bool `json:"subtitleExists"`
	MouthTimelineGenerated  bool `json:"mouthTimelineGenerated"`
	MotionTimelineGenerated bool `json:"motionTimelineGenerated"`
	SceneGenerated          bool `json:"sceneGenerated"`
	ControlRigLoaded        bool `json:"controlRigLoaded"`
	ReferenceSVGLoaded      bool `json:"referenceSvgLoaded"`
	FinalVideoGenerated     bool `json:"finalVideoGenerated"`
}

type AudioFrame struct {
	TimeSec float64 `json:"timeSec"`
	RMS     float64 `json:"rms"`
	Silent  bool    `json:"silent"`
}

type AudioAnalysis struct {
	AudioPath   string       `json:"audioPath"`
	DurationSec float64      `json:"durationSec"`
	FPS         int          `json:"fps"`
	SampleRate  int          `json:"sampleRate"`
	Frames      []AudioFrame `json:"frames"`
	GeneratedAt string       `json:"generatedAt"`
}

type LipSyncFrame struct {
	TimeSec float64 `json:"timeSec"`
	Mouth   string  `json:"mouth"`
	Open    float64 `json:"open"`
}

type MotionEvent struct {
	TimeSec  float64 `json:"timeSec"`
	Motion   string  `json:"motion"`
	Duration float64 `json:"duration"`
	Strength float64 `json:"strength,omitempty"`
}

type AvatarScene struct {
	CharacterID          string                 `json:"characterId"`
	DurationSec          float64                `json:"durationSec"`
	FPS                  int                    `json:"fps"`
	Resolution           Resolution             `json:"resolution"`
	Assets               map[string]string      `json:"assets"`
	LipSyncTimelinePath  string                 `json:"lipSyncTimelinePath"`
	MotionTimelinePath   string                 `json:"motionTimelinePath"`
	AudioAnalysisPath    string                 `json:"audioAnalysisPath"`
	Style                RenderStyle            `json:"style"`
	Renderer             string                 `json:"renderer"`
	CharacterDisplayName string                 `json:"characterDisplayName,omitempty"`
	CharacterAssetRoot   string                 `json:"characterAssetRoot,omitempty"`
	VoiceProfile         VoiceProfile           `json:"voiceProfile,omitempty"`
	HyperGenControl      map[string]interface{} `json:"hypergenControl,omitempty"`
	GenerationMetadata   map[string]interface{} `json:"generationMetadata,omitempty"`
}

type avatarTimelineBundle struct {
	CharacterID        string         `json:"characterId"`
	DurationSec        float64        `json:"durationSec"`
	FPS                int            `json:"fps"`
	LipSyncTimeline    []LipSyncFrame `json:"lipSyncTimeline"`
	MotionTimeline     []MotionEvent  `json:"motionTimeline"`
	AudioAnalysisPath  string         `json:"audioAnalysisPath"`
	LipSyncPath        string         `json:"lipSyncPath"`
	MotionTimelinePath string         `json:"motionTimelinePath"`
	GeneratedAt        string         `json:"generatedAt"`
}

func defaultMotionPolicy() MotionPolicy {
	return MotionPolicy{
		AutoBlink:      true,
		AutoBreath:     true,
		SentenceNod:    true,
		KeywordGesture: true,
	}
}

func defaultRenderStyle() RenderStyle {
	return RenderStyle{
		Position:               "center_bottom",
		Scale:                  1,
		SubtitleEnabled:        true,
		BackgroundEnabled:      true,
		TransparentAvatarVideo: true,
	}
}

func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}
