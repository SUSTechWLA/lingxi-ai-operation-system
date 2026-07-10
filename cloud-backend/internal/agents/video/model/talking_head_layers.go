package model

import "time"

const ArtifactKindBrollManifest = "BROLL_MANIFEST"

type VisualMode string

const (
	VisualModeIPPrimary              VisualMode = "IP_PRIMARY"
	VisualModeIPWithText             VisualMode = "IP_WITH_TEXT"
	VisualModeIPWithBrollPIP         VisualMode = "IP_WITH_BROLL_PIP"
	VisualModeBrollFullscreen        VisualMode = "BROLL_FULLSCREEN"
	VisualModeAIGCFullscreen         VisualMode = "AIGC_FULLSCREEN"
	VisualModeTextGraphicsFullscreen VisualMode = "TEXT_GRAPHICS_FULLSCREEN"
	VisualModeScreenRecordingFull    VisualMode = "SCREEN_RECORDING_FULLSCREEN"
)

type ShotLayerKind string

const (
	ShotLayerAudio       ShotLayerKind = "audio"
	ShotLayerIP          ShotLayerKind = "ip"
	ShotLayerText        ShotLayerKind = "text"
	ShotLayerBroll       ShotLayerKind = "broll"
	ShotLayerComposition ShotLayerKind = "composition"
	ShotLayerCandidate   ShotLayerKind = "candidate"
	ShotLayerQA          ShotLayerKind = "qa"
	ShotLayerFinal       ShotLayerKind = "final_assembly"
)

const (
	LayerStatusPlanned = "planned"
	LayerStatusCurrent = "current"
	LayerStatusStale   = "stale"
)

type ArtifactDependencyRef struct {
	ArtifactID  string `json:"artifactId,omitempty"`
	Layer       string `json:"layer,omitempty"`
	Revision    string `json:"revision"`
	Fingerprint string `json:"fingerprint,omitempty"`
}

// LayerArtifactState augments the existing artifact references with the
// revision graph needed for precise repair. It does not store media itself.
type LayerArtifactState struct {
	SchemaVersion      int                     `json:"schemaVersion"`
	Layer              ShotLayerKind           `json:"layer"`
	Status             string                  `json:"status"`
	Revision           string                  `json:"revision,omitempty"`
	InputFingerprint   string                  `json:"inputFingerprint,omitempty"`
	OutputFingerprint  string                  `json:"outputFingerprint,omitempty"`
	ArtifactRef        string                  `json:"artifactRef,omitempty"`
	Dependencies       []ArtifactDependencyRef `json:"dependencies,omitempty"`
	Provider           string                  `json:"provider,omitempty"`
	Model              string                  `json:"model,omitempty"`
	ToolVersion        string                  `json:"toolVersion,omitempty"`
	ExecutionMode      ExecutionMode           `json:"executionMode,omitempty"`
	ProductionEligible bool                    `json:"productionEligible"`
	StaleReason        string                  `json:"staleReason,omitempty"`
	CreatedAt          time.Time               `json:"createdAt,omitempty"`
}

type IPAssetPackRef struct {
	ID          string `json:"id"`
	Version     string `json:"version"`
	ContentHash string `json:"contentHash"`
}

type IPAssetPack struct {
	SchemaVersion int `json:"schemaVersion"`
	IPAssetPackRef
	DisplayName          string                 `json:"displayName"`
	ReferenceAssets      []string               `json:"referenceAssets,omitempty"`
	VoiceProfileID       string                 `json:"voiceProfileId,omitempty"`
	VoiceProfileVersion  string                 `json:"voiceProfileVersion,omitempty"`
	LipSyncProfile       string                 `json:"lipSyncProfile,omitempty"`
	BackgroundTemplates  []string               `json:"backgroundTemplates,omitempty"`
	CameraPresets        []string               `json:"cameraPresets,omitempty"`
	PoseLibrary          []string               `json:"poseLibrary,omitempty"`
	ExpressionLibrary    []string               `json:"expressionLibrary,omitempty"`
	GestureLibrary       []string               `json:"gestureLibrary,omitempty"`
	IdleMotionPolicy     string                 `json:"idleMotionPolicy,omitempty"`
	BrandTheme           map[string]interface{} `json:"brandTheme,omitempty"`
	SubtitleSafeZones    []string               `json:"subtitleSafeZones,omitempty"`
	IPPlacementSafeZones []string               `json:"ipPlacementSafeZones,omitempty"`
	ProviderParameters   map[string]interface{} `json:"providerParameters,omitempty"`
	Provenance           ArtifactProvenance     `json:"provenance,omitempty"`
}

type AudioLayerPlan struct {
	State                LayerArtifactState `json:"state"`
	AudioMasterRevision  string             `json:"audioMasterRevision"`
	VoiceoverArtifactRef string             `json:"voiceoverArtifactRef,omitempty"`
	VoiceProfileID       string             `json:"voiceProfileId,omitempty"`
	VoiceProfileVersion  string             `json:"voiceProfileVersion,omitempty"`
}

type IPLayerPlan struct {
	State              LayerArtifactState `json:"state"`
	AssetPack          IPAssetPackRef     `json:"assetPack"`
	BackgroundMode     string             `json:"backgroundMode,omitempty"`
	DisplayMode        string             `json:"displayMode,omitempty"`
	BackgroundTemplate string             `json:"backgroundTemplate,omitempty"`
	FramingPreset      string             `json:"framingPreset,omitempty"`
	Pose               string             `json:"pose,omitempty"`
	Expression         string             `json:"expression,omitempty"`
	GestureEvents      []TimelineMarker   `json:"gestureEvents,omitempty"`
	LipSyncRevision    string             `json:"lipSyncRevision,omitempty"`
}

type TextGraphicsLayerPlan struct {
	State            LayerArtifactState `json:"state"`
	TimelineRevision string             `json:"timelineRevision"`
	Renderer         string             `json:"renderer"`
	ProjectRef       string             `json:"projectRef,omitempty"`
	TextLayers       []TextLayerSpec    `json:"textLayers,omitempty"`
}

type BrollLayerPlan struct {
	State       LayerArtifactState   `json:"state"`
	ManifestRef string               `json:"manifestRef,omitempty"`
	Entries     []BrollManifestEntry `json:"entries,omitempty"`
}

type CompositionLayerPlan struct {
	State               LayerArtifactState     `json:"state"`
	BaseArtifactRef     string                 `json:"baseArtifactRef,omitempty"`
	OverlayArtifactRefs []string               `json:"overlayArtifactRefs,omitempty"`
	Assembler           string                 `json:"assembler,omitempty"`
	OutputRequirements  map[string]interface{} `json:"outputRequirements,omitempty"`
}

type TalkingHeadShotLayers struct {
	SchemaVersion        int                   `json:"schemaVersion"`
	TimelineRevision     string                `json:"timelineRevision"`
	VisualMode           VisualMode            `json:"visualMode"`
	VisualModeReason     string                `json:"visualModeReason,omitempty"`
	VisualModeConfidence float64               `json:"visualModeConfidence,omitempty"`
	NeedsHumanReview     bool                  `json:"needsHumanReview,omitempty"`
	Audio                AudioLayerPlan        `json:"audio"`
	IP                   IPLayerPlan           `json:"ip"`
	Text                 TextGraphicsLayerPlan `json:"text"`
	Broll                BrollLayerPlan        `json:"broll"`
	Composition          CompositionLayerPlan  `json:"composition"`
}

type BrollManifest struct {
	SchemaVersion int                  `json:"schemaVersion"`
	Revision      string               `json:"revision"`
	Entries       []BrollManifestEntry `json:"entries"`
}

type BrollManifestEntry struct {
	ID                 string             `json:"id"`
	ShotID             string             `json:"shotId"`
	NarrationText      string             `json:"narrationText,omitempty"`
	SemanticPurpose    string             `json:"semanticPurpose"`
	AssetType          string             `json:"assetType"`
	SourceType         string             `json:"sourceType"`
	SourceURI          string             `json:"sourceUri,omitempty"`
	ArtifactRef        string             `json:"artifactRef,omitempty"`
	LicenseStatus      string             `json:"licenseStatus"`
	UsageStatus        string             `json:"usageStatus,omitempty"`
	Generated          bool               `json:"generated"`
	Provider           string             `json:"provider,omitempty"`
	Model              string             `json:"model,omitempty"`
	Seed               string             `json:"seed,omitempty"`
	StartMs            int64              `json:"startMs"`
	EndMs              int64              `json:"endMs"`
	Fit                string             `json:"fit,omitempty"`
	Placement          string             `json:"placement,omitempty"`
	RelevanceScore     float64            `json:"relevanceScore,omitempty"`
	ReviewStatus       string             `json:"reviewStatus"`
	ReplacementHistory []string           `json:"replacementHistory,omitempty"`
	Provenance         ArtifactProvenance `json:"provenance,omitempty"`
}
