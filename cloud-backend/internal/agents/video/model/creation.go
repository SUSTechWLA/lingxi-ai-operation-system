package model

import "time"

const (
	ReviewModeShotLevel = "shot_level_review"

	RenderStrategyAuto = "auto"

	RenderModeHTMLOnly                = "html_only"
	RenderModeAIGCOnly                = "aigc_only"
	RenderModeHybridAIGCBGHTMLOverlay = "hybrid_aigc_bg_html_overlay"
	RenderModeHTMLPreviewThenAIGC     = "html_preview_then_aigc"
	RenderModeHTMLPreviewThenHybrid   = "html_preview_then_hybrid"

	GenerationModeHTMLOnly                 = "html_only"
	GenerationModeAIGCVideo                = "aigc_video"
	GenerationModeAIGCImageThenHyperFrames = "aigc_image_then_hyperframes"
	GenerationModeHybridAIGCBGHTMLOverlay  = "hybrid_aigc_bg_html_overlay"
	GenerationModeExternalOrUserAsset      = "external_or_user_asset"
	GenerationModePlaceholderPreview       = "placeholder_preview"

	AssetSourceAIGCImage             = "aigc_image"
	AssetSourceAIGCVideo             = "aigc_video"
	AssetSourceHyperFrames           = "hyperframes"
	AssetSourceUserUpload            = "user_upload"
	AssetSourceExternalGeneration    = "external_generation"
	AssetSourceOpenAsset             = "open_asset"
	AssetSourcePlaceholder           = "placeholder"
	ArtifactSourceAIGCVideo          = "aigc_video"
	ArtifactSourceAIGCImage          = "aigc_image"
	ArtifactSourceHyperFrames        = "hyperframes"
	ArtifactSourceFFmpegComposite    = "ffmpeg_composite"
	ArtifactSourceUploaded           = "uploaded"
	ArtifactSourceIPArollVideo       = "ip_aroll_video"
	ArtifactSourceFallbackPreview    = "fallback_preview"
	ArtifactSourceFallbackStoryboard = "fallback_storyboard"

	ReviewStatusPending  = "pending"
	ReviewStatusApproved = "approved"
	ReviewStatusRejected = "rejected"
	ReviewStatusStale    = "stale"

	ShotPlanned              = "PLANNED"
	ShotProductionGenerating = "GENERATING"
	ShotCandidateRendered    = "CANDIDATE_RENDERED"
	ShotQARunning            = "SHOT_QA_RUNNING"
	ShotQAPassed             = "SHOT_QA_PASSED"
	ShotQAFailed             = "SHOT_QA_FAILED"
	ShotHumanReviewRequired  = "HUMAN_REVIEW_REQUIRED"
	ShotAcceptedForAssembly  = "ACCEPTED_FOR_ASSEMBLY"

	CandidateRendered            = "CANDIDATE_RENDERED"
	CandidateShotQARunning       = "SHOT_QA_RUNNING"
	CandidateShotQAPassed        = "SHOT_QA_PASSED"
	CandidateShotQAFailed        = "SHOT_QA_FAILED"
	CandidateHumanReviewRequired = "HUMAN_REVIEW_REQUIRED"
	CandidateAcceptedForAssembly = "ACCEPTED_FOR_ASSEMBLY"

	RepairActionPass = "PASS"
	// RepairActionRerenderHTML is retained as a source-compatible alias; new
	// persisted plans use the canonical layer-scoped RERENDER_TEXT value.
	RepairActionRerenderHTML           = "RERENDER_TEXT"
	RepairActionRecomposite            = "RECOMPOSITE"
	RepairActionPromptPatchRegen       = "PROMPT_PATCH_REGEN"
	RepairActionRegenAIGC              = "REGEN_AIGC"
	RepairActionRegenAIGCWithReference = "REGEN_AIGC_WITH_REFERENCE"
	RepairActionDeferToFinalAssembly   = "DEFER_TO_FINAL_ASSEMBLY"
	RepairActionHumanReview            = "HUMAN_REVIEW"
	RepairActionRegenerateVoice        = "REGENERATE_VOICE"
	RepairActionRealignTimeline        = "REALIGN_TIMELINE"
	RepairActionRegenerateIP           = "REGENERATE_IP"
	RepairActionRelipsyncIP            = "RELIPSYNC_IP"
	RepairActionRerenderText           = RepairActionRerenderHTML
	RepairActionReplaceBroll           = "REPLACE_BROLL"
	RepairActionRegenerateBroll        = "REGENERATE_BROLL"
	RepairActionReencode               = "REENCODE"

	AssemblyStepAllShotsAcceptedGate   = "ALL_SHOTS_ACCEPTED_GATE"
	AssemblyStepNormalizeAcceptedShots = "NORMALIZE_ACCEPTED_SHOTS"
	AssemblyStepFFmpegConcat           = "FFMPEG_CONCAT"
	AssemblyStepGlobalVoiceoverAlign   = "GLOBAL_VOICEOVER_ALIGN"
	AssemblyStepGlobalAudioMix         = "GLOBAL_BGM_MIX_AND_DUCKING"
	AssemblyStepGlobalSubtitleRender   = "GLOBAL_SUBTITLE_RENDER"
	AssemblyStepFinalVideoQA           = "FINAL_VIDEO_QA"
	AssemblyStepExportPublish          = "EXPORT_PUBLISH"
	AssemblyScopeGlobal                = "global"

	VisualChangeLow    = "low"
	VisualChangeMedium = "medium"
	VisualChangeHigh   = "high"

	TextRoleTitle       = "title"
	TextRoleSubtitle    = "subtitle"
	TextRoleKeyword     = "keyword"
	TextRoleLabel       = "label"
	TextRoleUIText      = "ui_text"
	TextRoleCaption     = "caption"
	TextRoleDataText    = "data_text"
	TextRoleButtonText  = "button_text"
	TextRoleFileName    = "file_name"
	TextRoleChatMessage = "chat_message"
	TextRoleCode        = "code"
	TextRoleNumber      = "number"
	TextRoleBrandName   = "brand_name"

	VideoProfileTalkingHead    = "talking_head"
	VideoProfileCinematicStory = "cinematic_story"

	ArtifactKindVideoCreationProfile = "VIDEO_CREATION_PROFILE"
)

type VideoCreationProfile struct {
	SchemaVersion          int               `json:"schemaVersion"`
	ProfileID              string            `json:"profileId"`
	SourceRoute            string            `json:"sourceRoute,omitempty"`
	PrimaryArtifact        string            `json:"primaryArtifact"`
	QualityContract        []string          `json:"qualityContract,omitempty"`
	DAGTemplateID          string            `json:"dagTemplateId"`
	ReviewGatePolicy       []string          `json:"reviewGatePolicy,omitempty"`
	ToolBias               map[string]string `json:"toolBias,omitempty"`
	FallbackProfile        string            `json:"fallbackProfile,omitempty"`
	Confidence             float64           `json:"confidence,omitempty"`
	Reason                 string            `json:"reason,omitempty"`
	NeedsUserReview        bool              `json:"needsUserReview,omitempty"`
	RuntimePipelineID      string            `json:"runtimePipelineId"`
	RuntimePipelineVersion string            `json:"runtimePipelineVersion"`
	RuntimePipelineSource  string            `json:"runtimePipelineSource"`
}

type ShotPolicy struct {
	MinDurationSec           int  `json:"minDurationSec"`
	MaxDurationSec           int  `json:"maxDurationSec"`
	PreferDurationSec        int  `json:"preferDurationSec"`
	PreferredMinDurationSec  int  `json:"preferredMinDurationSec"`
	PreferredMaxDurationSec  int  `json:"preferredMaxDurationSec"`
	SplitByScriptSemantics   bool `json:"splitByScriptSemantics"`
	SplitByVisualChange      bool `json:"splitByVisualChange"`
	SingleSceneRequired      bool `json:"singleSceneRequired"`
	LowVisualChangeRequired  bool `json:"lowVisualChangeRequired"`
	AvoidCrossShotDependency bool `json:"avoidCrossShotDependency"`
}

type RenderPreference struct {
	DefaultRenderStrategy string `json:"defaultRenderStrategy"`
	PreferHTMLForText     bool   `json:"preferHTMLForText"`
	PreferHTMLForCharts   bool   `json:"preferHTMLForCharts"`
	PreferHTMLForUI       bool   `json:"preferHTMLForUI"`
	PreferAIGCForPeople   bool   `json:"preferAIGCForPeople"`
	PreferAIGCForScene    bool   `json:"preferAIGCForScene"`
	AllowHybridRender     bool   `json:"allowHybridRender"`
	PreferLowCostPreview  bool   `json:"preferLowCostPreview"`
}

type VideoCreationSpec struct {
	ID                string           `json:"id"`
	ProjectID         string           `json:"projectId"`
	SourceMessage     string           `json:"sourceMessage"`
	Topic             string           `json:"topic,omitempty"`
	Platform          string           `json:"platform,omitempty"`
	VideoType         string           `json:"videoType,omitempty"`
	TargetDurationSec int              `json:"targetDurationSec,omitempty"`
	AspectRatio       string           `json:"aspectRatio"`
	Language          string           `json:"language"`
	Audience          string           `json:"audience,omitempty"`
	Tone              string           `json:"tone,omitempty"`
	VisualStyle       string           `json:"visualStyle,omitempty"`
	ReviewMode        string           `json:"reviewMode"`
	ShotPolicy        ShotPolicy       `json:"shotPolicy"`
	RenderPreference  RenderPreference `json:"renderPreference"`
	Status            string           `json:"status"`
	CreatedAt         time.Time        `json:"createdAt"`
	UpdatedAt         time.Time        `json:"updatedAt"`
}

func NewVideoCreationSpec(projectID, sourceMessage string) *VideoCreationSpec {
	now := time.Now()
	return &VideoCreationSpec{
		ID:               projectID + "-spec",
		ProjectID:        projectID,
		SourceMessage:    sourceMessage,
		AspectRatio:      "16:9",
		Language:         "zh-CN",
		ReviewMode:       ReviewModeShotLevel,
		ShotPolicy:       DefaultShotPolicy(),
		RenderPreference: DefaultRenderPreference(),
		Status:           ReviewStatusPending,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
}

func DefaultShotPolicy() ShotPolicy {
	return ShotPolicy{
		MinDurationSec:           3,
		MaxDurationSec:           15,
		PreferDurationSec:        6,
		PreferredMinDurationSec:  6,
		PreferredMaxDurationSec:  8,
		SplitByScriptSemantics:   true,
		SplitByVisualChange:      true,
		SingleSceneRequired:      true,
		LowVisualChangeRequired:  true,
		AvoidCrossShotDependency: true,
	}
}

func DefaultRenderPreference() RenderPreference {
	return RenderPreference{
		DefaultRenderStrategy: RenderStrategyAuto,
		PreferHTMLForText:     true,
		PreferHTMLForCharts:   true,
		PreferHTMLForUI:       true,
		PreferAIGCForPeople:   true,
		PreferAIGCForScene:    true,
		AllowHybridRender:     true,
		PreferLowCostPreview:  true,
	}
}

type ShotUnit struct {
	SchemaVersion       int                    `json:"schemaVersion,omitempty"`
	ID                  string                 `json:"id"`
	ProjectID           string                 `json:"projectId"`
	SequenceIndex       int                    `json:"sequenceIndex"`
	Title               string                 `json:"title"`
	VideoType           string                 `json:"videoType,omitempty"`
	DurationSec         int                    `json:"durationSec"`
	StartSec            float64                `json:"startSec,omitempty"`
	EndSec              float64                `json:"endSec,omitempty"`
	StartMs             int64                  `json:"startMs,omitempty"`
	EndMs               int64                  `json:"endMs,omitempty"`
	DurationMs          int64                  `json:"durationMs,omitempty"`
	TimelineRevision    string                 `json:"timelineRevision,omitempty"`
	TalkingHeadLayers   *TalkingHeadShotLayers `json:"talkingHeadLayers,omitempty"`
	ScriptSegmentID     string                 `json:"scriptSegmentId,omitempty"`
	SceneID             string                 `json:"sceneId,omitempty"`
	Scene               string                 `json:"scene,omitempty"`
	SceneSummary        string                 `json:"sceneSummary,omitempty"`
	Subject             string                 `json:"subject,omitempty"`
	SingleScene         bool                   `json:"singleScene"`
	VisualChangeLevel   string                 `json:"visualChangeLevel"`
	VisualChangeReason  string                 `json:"visualChangeReason,omitempty"`
	Narration           string                 `json:"narration,omitempty"`
	ScreenText          []string               `json:"screenText,omitempty"`
	MainAction          string                 `json:"mainAction,omitempty"`
	Action              string                 `json:"action,omitempty"`
	Camera              string                 `json:"camera,omitempty"`
	ShotSize            string                 `json:"shotSize,omitempty"`
	Framing             string                 `json:"framing,omitempty"`
	FocalLengthHint     string                 `json:"focalLengthHint,omitempty"`
	TransitionIn        string                 `json:"transitionIn,omitempty"`
	TransitionOut       string                 `json:"transitionOut,omitempty"`
	Continuity          ShotContinuity         `json:"continuity"`
	PromptConstraints   PromptConstraints      `json:"promptConstraints"`
	VisualPlan          VisualPlan             `json:"visualPlan,omitempty"`
	RenderStrategy      RenderStrategy         `json:"renderStrategy,omitempty"`
	ArtifactRefs        ShotArtifactRefs       `json:"artifactRefs,omitempty"`
	QAStatus            string                 `json:"qaStatus,omitempty"`
	AcceptedCandidateID string                 `json:"acceptedCandidateId,omitempty"`
	Candidates          []ShotCandidate        `json:"candidates,omitempty"`
	RepairPlans         []RepairPlan           `json:"repairPlans,omitempty"`
	ReviewStatus        string                 `json:"reviewStatus"`
	Locked              bool                   `json:"locked"`
	Stale               bool                   `json:"stale"`
	Version             int                    `json:"version"`
	LastRejectReason    string                 `json:"lastRejectReason,omitempty"`
	CreatedAt           time.Time              `json:"createdAt"`
	UpdatedAt           time.Time              `json:"updatedAt"`
}

type ShotContinuity struct {
	Characters        []string `json:"characters,omitempty"`
	Props             []string `json:"props,omitempty"`
	StyleTags         []string `json:"styleTags,omitempty"`
	MustMatchPrevious bool     `json:"mustMatchPrevious,omitempty"`
	MustMatchNext     bool     `json:"mustMatchNext,omitempty"`
	PreviousState     string   `json:"previousState,omitempty"`
	EndState          string   `json:"endState,omitempty"`
}

type PromptConstraints struct {
	MustInclude []string `json:"mustInclude,omitempty"`
	MustAvoid   []string `json:"mustAvoid,omitempty"`
}

type ShotArtifactRefs struct {
	VisualPlanArtifactID          string `json:"visualPlanArtifactId,omitempty"`
	RenderStrategyArtifactID      string `json:"renderStrategyArtifactId,omitempty"`
	KeyframePromptArtifactID      string `json:"keyframePromptArtifactId,omitempty"`
	KeyframeImageArtifactID       string `json:"keyframeImageArtifactId,omitempty"`
	VideoPromptArtifactID         string `json:"videoPromptArtifactId,omitempty"`
	AIGCBackgroundVideoArtifactID string `json:"aigcBackgroundVideoArtifactId,omitempty"`
	HTMLSourceArtifactID          string `json:"htmlSourceArtifactId,omitempty"`
	HTMLPreviewVideoArtifactID    string `json:"htmlPreviewVideoArtifactId,omitempty"`
	HTMLOverlayVideoArtifactID    string `json:"htmlOverlayVideoArtifactId,omitempty"`
	CompositedShotVideoArtifactID string `json:"compositedShotVideoArtifactId,omitempty"`
	VideoClipArtifactID           string `json:"videoClipArtifactId,omitempty"`
	SubtitleArtifactID            string `json:"subtitleArtifactId,omitempty"`
}

type ShotCandidate struct {
	SchemaVersion          int               `json:"schemaVersion,omitempty"`
	CandidateID            string            `json:"candidateId"`
	ShotID                 string            `json:"shotId"`
	AttemptIndex           int               `json:"attemptIndex"`
	Status                 string            `json:"status"`
	DurationSec            float64           `json:"durationSec"`
	SourceType             string            `json:"sourceType,omitempty"`
	IsFallback             bool              `json:"isFallback,omitempty"`
	ExecutionMode          ExecutionMode     `json:"executionMode,omitempty"`
	ProductionEligible     bool              `json:"productionEligible"`
	FallbackReason         string            `json:"fallbackReason,omitempty"`
	TimelineRevision       string            `json:"timelineRevision,omitempty"`
	LayerRevisions         map[string]string `json:"layerRevisions,omitempty"`
	GenerationPlanRevision string            `json:"generationPlanRevision,omitempty"`
	InputFingerprint       string            `json:"inputFingerprint,omitempty"`
	OutputFingerprint      string            `json:"outputFingerprint,omitempty"`
	Stale                  bool              `json:"stale,omitempty"`
	StaleReason            string            `json:"staleReason,omitempty"`
	ArtifactRefs           ShotArtifactRefs  `json:"artifactRefs,omitempty"`
	QAReport               *ShotQAReport     `json:"qaReport,omitempty"`
	RepairPlan             *RepairPlan       `json:"repairPlan,omitempty"`
	CreatedAt              time.Time         `json:"createdAt,omitempty"`
}

type ShotQAReport struct {
	ShotID           string         `json:"shotId"`
	CandidateID      string         `json:"candidateId"`
	Status           string         `json:"status"`
	Passed           bool           `json:"passed"`
	HumanApproved    bool           `json:"humanApproved,omitempty"`
	Severity         string         `json:"severity,omitempty"`
	OverallScore     int            `json:"overallScore,omitempty"`
	Scores           map[string]int `json:"scores,omitempty"`
	PassedDimensions []string       `json:"passedDimensions,omitempty"`
	FailedDimensions []string       `json:"failedDimensions,omitempty"`
	ReportRef        string         `json:"reportRef,omitempty"`
	Summary          string         `json:"summary,omitempty"`
}

type RepairPlan struct {
	Action              string                 `json:"action"`
	Reason              string                 `json:"reason,omitempty"`
	Severity            string                 `json:"severity,omitempty"`
	TargetShotID        string                 `json:"targetShotId,omitempty"`
	SourceCandidateID   string                 `json:"sourceCandidateId,omitempty"`
	AttemptIndex        int                    `json:"attemptIndex"`
	Preserve            bool                   `json:"preserve"`
	LockedDimensions    []string               `json:"lockedDimensions,omitempty"`
	RepairTargets       []string               `json:"repairTargets,omitempty"`
	PromptPatch         map[string]interface{} `json:"promptPatch,omitempty"`
	RenderStrategyPatch map[string]interface{} `json:"renderStrategyPatch,omitempty"`
	NextToolCall        string                 `json:"nextToolCall,omitempty"`
}

type ShotRepairPolicy struct {
	MaxRepairAttemptsPerShot           int    `json:"maxRepairAttemptsPerShot"`
	MaxAIGCRegenerationAttemptsPerShot int    `json:"maxAigcRegenerationAttemptsPerShot"`
	PreferLocalRepairBeforeAIGCRegen   bool   `json:"preferLocalRepairBeforeAigcRegen"`
	PreservePassedDimensions           bool   `json:"preservePassedDimensions"`
	ProductionMode                     string `json:"productionMode"`
}

func DefaultShotRepairPolicy() ShotRepairPolicy {
	return ShotRepairPolicy{
		MaxRepairAttemptsPerShot:           3,
		MaxAIGCRegenerationAttemptsPerShot: 2,
		PreferLocalRepairBeforeAIGCRegen:   true,
		PreservePassedDimensions:           true,
		ProductionMode:                     ProductionModeStrict,
	}
}

type VisualPlan struct {
	Canvas        CanvasSpec            `json:"canvas"`
	Background    BackgroundSpec        `json:"background,omitempty"`
	Characters    []CharacterVisualSpec `json:"characters,omitempty"`
	Props         []PropVisualSpec      `json:"props,omitempty"`
	TextLayers    []TextLayerSpec       `json:"textLayers,omitempty"`
	UILayers      []UILayerSpec         `json:"uiLayers,omitempty"`
	DataVisuals   []DataVisualSpec      `json:"dataVisuals,omitempty"`
	MotionPlan    MotionPlan            `json:"motionPlan,omitempty"`
	CameraPlan    CameraPlan            `json:"cameraPlan,omitempty"`
	TransitionIn  string                `json:"transitionIn,omitempty"`
	TransitionOut string                `json:"transitionOut,omitempty"`
	Style         VisualStyleSpec       `json:"style,omitempty"`
	Constraints   VisualConstraints     `json:"constraints,omitempty"`
}

type CanvasSpec struct {
	AspectRatio string `json:"aspectRatio"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	FPS         int    `json:"fps"`
	DurationSec int    `json:"durationSec"`
}

type BackgroundSpec struct {
	Description  string `json:"description,omitempty"`
	RequiresAIGC bool   `json:"requiresAigc,omitempty"`
}

type CharacterVisualSpec struct {
	ID           string `json:"id"`
	Description  string `json:"description,omitempty"`
	Motion       string `json:"motion,omitempty"`
	Emotion      string `json:"emotion,omitempty"`
	RequiresAIGC bool   `json:"requiresAigc,omitempty"`
}

type PropVisualSpec struct {
	ID          string `json:"id"`
	Description string `json:"description,omitempty"`
}

type TextLayerSpec struct {
	ID          string  `json:"id"`
	Text        string  `json:"text"`
	Language    string  `json:"language,omitempty"`
	Role        string  `json:"role"`
	Position    string  `json:"position,omitempty"`
	FontSize    int     `json:"fontSize,omitempty"`
	FontWeight  string  `json:"fontWeight,omitempty"`
	Color       string  `json:"color,omitempty"`
	Background  string  `json:"background,omitempty"`
	StartSec    float64 `json:"startSec"`
	EndSec      float64 `json:"endSec"`
	Animation   string  `json:"animation,omitempty"`
	MustBeExact bool    `json:"mustBeExact"`
}

type UILayerSpec struct {
	ID          string `json:"id"`
	Description string `json:"description,omitempty"`
}

type DataVisualSpec struct {
	ID          string `json:"id"`
	Type        string `json:"type,omitempty"`
	Description string `json:"description,omitempty"`
}

type MotionPlan struct {
	Description  string `json:"description,omitempty"`
	RequiresAIGC bool   `json:"requiresAigc,omitempty"`
}

type CameraPlan struct {
	Description  string `json:"description,omitempty"`
	Movement     string `json:"movement,omitempty"`
	RequiresAIGC bool   `json:"requiresAigc,omitempty"`
}

type VisualStyleSpec struct {
	Description string   `json:"description,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

type VisualConstraints struct {
	MustInclude []string `json:"mustInclude,omitempty"`
	MustAvoid   []string `json:"mustAvoid,omitempty"`
}

type RenderStrategy struct {
	Mode              string         `json:"mode"`
	PrimaryTool       string         `json:"primaryTool,omitempty"`
	SecondaryTools    []string       `json:"secondaryTools,omitempty"`
	Reason            string         `json:"reason,omitempty"`
	AIGCRequired      bool           `json:"aigcRequired"`
	HTMLRequired      bool           `json:"htmlRequired"`
	TextOverlayNeeded bool           `json:"textOverlayNeeded"`
	NeedsCompositing  bool           `json:"needsCompositing"`
	AIGCInput         *AIGCInputSpec `json:"aigcInput,omitempty"`
	HTMLInput         *HTMLInputSpec `json:"htmlInput,omitempty"`
	CompositePlan     *CompositePlan `json:"compositePlan,omitempty"`
}

type ShotGenerationPlan struct {
	ShotID         string                  `json:"shotId"`
	Mode           string                  `json:"mode"`
	PrimaryTool    string                  `json:"primaryTool,omitempty"`
	SecondaryTools []string                `json:"secondaryTools,omitempty"`
	Reason         string                  `json:"reason,omitempty"`
	Confidence     float64                 `json:"confidence,omitempty"`
	RiskLevel      string                  `json:"riskLevel,omitempty"`
	RequiredAssets []ShotAssetNeed         `json:"requiredAssets,omitempty"`
	RenderInputs   map[string]interface{}  `json:"renderInputs,omitempty"`
	FusionPlan     FusionPlan              `json:"fusionPlan"`
	FallbackPlan   *ShotGenerationFallback `json:"fallbackPlan,omitempty"`
	ReviewFocus    []string                `json:"reviewFocus,omitempty"`
}

type ShotGenerationFallback struct {
	Mode   string `json:"mode"`
	Reason string `json:"reason,omitempty"`
}

type ExternalGenerationReference struct {
	Role       string `json:"role,omitempty"`
	StorageRef string `json:"storageRef,omitempty"`
}

type ExternalGenerationDelivery struct {
	DirectAPIEligible    bool                          `json:"directApiEligible"`
	ManualUploadRequired bool                          `json:"manualUploadRequired"`
	ReferenceImages      []ExternalGenerationReference `json:"referenceImages,omitempty"`
	PromptPackage        string                        `json:"promptPackage,omitempty"`
}

type ShotAssetNeed struct {
	ID             string   `json:"id"`
	Kind           string   `json:"kind,omitempty"`
	Role           string   `json:"role,omitempty"`
	Source         string   `json:"source"`
	Required       bool     `json:"required"`
	ApprovalStatus string   `json:"approvalStatus,omitempty"`
	StorageRef     string   `json:"storageRef,omitempty"`
	RelatedShotID  string   `json:"relatedShotId,omitempty"`
	Locks          []string `json:"locks,omitempty"`
}

type FusionPlan struct {
	ShotID             string            `json:"shotId"`
	BaseLayer          FusionLayer       `json:"baseLayer"`
	OverlayLayers      []FusionLayer     `json:"overlayLayers,omitempty"`
	TimedMedia         []TimedMediaLayer `json:"timedMedia,omitempty"`
	Assembler          string            `json:"assembler,omitempty"`
	OutputArtifactKind string            `json:"outputArtifactKind,omitempty"`
}

type FusionLayer struct {
	ID          string  `json:"id"`
	Kind        string  `json:"kind,omitempty"`
	Role        string  `json:"role,omitempty"`
	StorageRef  string  `json:"storageRef,omitempty"`
	StartSec    float64 `json:"startSec,omitempty"`
	DurationSec float64 `json:"durationSec,omitempty"`
}

type TimedMediaLayer struct {
	ID          string  `json:"id"`
	Kind        string  `json:"kind,omitempty"`
	Role        string  `json:"role,omitempty"`
	StorageRef  string  `json:"storageRef,omitempty"`
	StartSec    float64 `json:"startSec"`
	DurationSec float64 `json:"durationSec"`
	TrackIndex  int     `json:"trackIndex"`
	Fit         string  `json:"fit,omitempty"`
	Opacity     float64 `json:"opacity,omitempty"`
}

const (
	ArtifactKindTimeWindowPlan = "TIME_WINDOW_PLAN"
)

type ScriptSpan struct {
	ID       string  `json:"id"`
	StartSec float64 `json:"startSec"`
	EndSec   float64 `json:"endSec"`
	Text     string  `json:"text"`
	Scene    string  `json:"scene,omitempty"`
	Subject  string  `json:"subject,omitempty"`
	Action   string  `json:"action,omitempty"`
	Camera   string  `json:"camera,omitempty"`
	ShotSize string  `json:"shotSize,omitempty"`
	Framing  string  `json:"framing,omitempty"`
	Visual   string  `json:"visual,omitempty"`
	Emotion  string  `json:"emotion,omitempty"`
}

type TimeWindowPlan struct {
	SchemaVersion    int              `json:"schemaVersion,omitempty"`
	ProfileID        string           `json:"profileId"`
	TimelineRevision string           `json:"timelineRevision,omitempty"`
	Windows          []TimeWindowUnit `json:"windows"`
	Warnings         []string         `json:"warnings,omitempty"`
	SplitReport      ShotSplitReport  `json:"splitReport,omitempty"`
}

type TimeWindowUnit struct {
	ID                 string  `json:"id"`
	ShotID             string  `json:"shotId"`
	ParentShotID       string  `json:"parentShotId,omitempty"`
	SequenceIndex      int     `json:"sequenceIndex"`
	StartSec           float64 `json:"startSec"`
	EndSec             float64 `json:"endSec"`
	DurationSec        float64 `json:"durationSec"`
	StartMs            int64   `json:"startMs,omitempty"`
	EndMs              int64   `json:"endMs,omitempty"`
	DurationMs         int64   `json:"durationMs,omitempty"`
	TimelineRevision   string  `json:"timelineRevision,omitempty"`
	ScriptSpanID       string  `json:"scriptSpanId,omitempty"`
	ScriptText         string  `json:"scriptText,omitempty"`
	Subject            string  `json:"subject,omitempty"`
	Action             string  `json:"action,omitempty"`
	Camera             string  `json:"camera,omitempty"`
	ShotSize           string  `json:"shotSize,omitempty"`
	Framing            string  `json:"framing,omitempty"`
	SceneSummary       string  `json:"sceneSummary,omitempty"`
	MainAction         string  `json:"mainAction,omitempty"`
	VisualChangeReason string  `json:"visualChangeReason,omitempty"`
	AIGCEligible       bool    `json:"aigcEligible"`
	RecommendedMode    string  `json:"recommendedMode,omitempty"`
	Reason             string  `json:"reason,omitempty"`
}

type ShotSplitReport struct {
	Policy                 ShotPolicy `json:"policy"`
	SplitByScriptSemantics bool       `json:"splitByScriptSemantics"`
	SplitByVisualChange    bool       `json:"splitByVisualChange"`
	PolicyReasons          []string   `json:"policyReasons,omitempty"`
	MergeCount             int        `json:"mergeCount,omitempty"`
	ForcedSplitCount       int        `json:"forcedSplitCount,omitempty"`
	DurationValidation     []string   `json:"durationValidation,omitempty"`
}

type AcceptedShotRef struct {
	ShotID             string        `json:"shotId"`
	CandidateID        string        `json:"candidateId"`
	DurationSec        float64       `json:"durationSec"`
	SourceType         string        `json:"sourceType,omitempty"`
	IsFallback         bool          `json:"isFallback,omitempty"`
	ExecutionMode      ExecutionMode `json:"executionMode,omitempty"`
	ProductionEligible bool          `json:"productionEligible"`
	ArtifactID         string        `json:"artifactId,omitempty"`
}

type SubtitleCue struct {
	ShotID   string  `json:"shotId,omitempty"`
	StartSec float64 `json:"startSec"`
	EndSec   float64 `json:"endSec"`
	Text     string  `json:"text,omitempty"`
}

type SubtitleTimeline struct {
	Scope string        `json:"scope"`
	Cues  []SubtitleCue `json:"cues,omitempty"`
}

type AudioMixPlan struct {
	Scope          string  `json:"scope"`
	VoiceoverAlign bool    `json:"voiceoverAlign"`
	BGMDucking     bool    `json:"bgmDucking"`
	TargetLUFS     float64 `json:"targetLufs,omitempty"`
}

type FinalQAReport struct {
	Status    string `json:"status"`
	Passed    bool   `json:"passed"`
	ReportRef string `json:"reportRef,omitempty"`
	Summary   string `json:"summary,omitempty"`
}

type FinalAssemblyPlan struct {
	Status             string               `json:"status"`
	Resolution         string               `json:"resolution"`
	FPS                int                  `json:"fps"`
	PixelFormat        string               `json:"pixelFormat"`
	Codec              string               `json:"codec"`
	Steps              []string             `json:"steps"`
	AcceptedShots      []AcceptedShotRef    `json:"acceptedShots"`
	SubtitleTimeline   SubtitleTimeline     `json:"subtitleTimeline"`
	AudioMixPlan       AudioMixPlan         `json:"audioMixPlan"`
	FinalQAReport      *FinalQAReport       `json:"finalQaReport,omitempty"`
	ArtifactProvenance []ArtifactProvenance `json:"artifactProvenance,omitempty"`
}

type ArtifactProvenance struct {
	ArtifactID        string   `json:"artifactId,omitempty"`
	Kind              string   `json:"kind,omitempty"`
	ShotID            string   `json:"shotId,omitempty"`
	CandidateID       string   `json:"candidateId,omitempty"`
	SourceType        string   `json:"sourceType"`
	ProviderName      string   `json:"providerName,omitempty"`
	ProviderJobID     string   `json:"providerJobId,omitempty"`
	FallbackReason    string   `json:"fallbackReason,omitempty"`
	IsFallback        bool     `json:"isFallback"`
	GeneratedAt       string   `json:"generatedAt,omitempty"`
	InputPromptHash   string   `json:"inputPromptHash,omitempty"`
	SourceArtifactIDs []string `json:"sourceArtifactIds,omitempty"`
}

type ProvenanceSummary struct {
	RawShotCandidateCount      int `json:"rawShotCandidateCount"`
	RepairedShotCandidateCount int `json:"repairedShotCandidateCount"`
	AcceptedShotCount          int `json:"acceptedShotCount"`
	NormalizedShotClipCount    int `json:"normalizedShotClipCount"`
	ConcatVideoCount           int `json:"concatVideoCount"`
	FinalAudioMixCount         int `json:"finalAudioMixCount"`
	FinalSubtitleTrackCount    int `json:"finalSubtitleTrackCount"`
	FinalVideoCount            int `json:"finalVideoCount"`
	RealAIGCVideoCount         int `json:"realAigcVideoCount"`
	FallbackCount              int `json:"fallbackCount"`
}

type VideoDiagnosticsSnapshot struct {
	SchemaVersion          int                  `json:"schemaVersion"`
	ShotList               []ShotUnit           `json:"shotList"`
	ShotSplitReport        ShotSplitReport      `json:"shotSplitReport"`
	ShotDurationValidation []string             `json:"shotDurationValidation,omitempty"`
	ShotCandidates         []ShotCandidate      `json:"shotCandidates,omitempty"`
	ShotQAReports          []ShotQAReport       `json:"shotQaReports,omitempty"`
	RepairPlans            []RepairPlan         `json:"repairPlans,omitempty"`
	AcceptedShots          []AcceptedShotRef    `json:"acceptedShots,omitempty"`
	AssemblyPlan           FinalAssemblyPlan    `json:"assemblyPlan"`
	SubtitleTimeline       SubtitleTimeline     `json:"subtitleTimeline"`
	AudioMixPlan           AudioMixPlan         `json:"audioMixPlan"`
	FinalQAReport          FinalQAReport        `json:"finalQaReport"`
	ArtifactManifest       []ArtifactProvenance `json:"artifactManifest,omitempty"`
	ProvenanceSummary      ProvenanceSummary    `json:"provenanceSummary"`
}

type AIGCInputSpec struct {
	Prompt         string `json:"prompt,omitempty"`
	NegativePrompt string `json:"negativePrompt,omitempty"`
	DurationSec    int    `json:"durationSec,omitempty"`
}

type HTMLInputSpec struct {
	DurationSec int             `json:"durationSec,omitempty"`
	TextLayers  []TextLayerSpec `json:"textLayers,omitempty"`
}

type CompositePlan struct {
	BackgroundArtifactID string `json:"backgroundArtifactId,omitempty"`
	OverlayArtifactID    string `json:"overlayArtifactId,omitempty"`
	OutputArtifactType   string `json:"outputArtifactType,omitempty"`
}

type ShotDrivenState struct {
	SchemaVersion int                `json:"schemaVersion"`
	Spec          *VideoCreationSpec `json:"spec,omitempty"`
	Shots         []ShotUnit         `json:"shots,omitempty"`
	UpdatedAt     time.Time          `json:"updatedAt"`
}
