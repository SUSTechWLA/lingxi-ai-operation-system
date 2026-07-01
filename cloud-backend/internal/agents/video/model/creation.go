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

	AssetSourceAIGCImage          = "aigc_image"
	AssetSourceAIGCVideo          = "aigc_video"
	AssetSourceHyperFrames        = "hyperframes"
	AssetSourceUserUpload         = "user_upload"
	AssetSourceExternalGeneration = "external_generation"
	AssetSourceOpenAsset          = "open_asset"
	AssetSourcePlaceholder        = "placeholder"

	ReviewStatusPending  = "pending"
	ReviewStatusApproved = "approved"
	ReviewStatusRejected = "rejected"
	ReviewStatusStale    = "stale"

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
)

type ShotPolicy struct {
	MinDurationSec           int  `json:"minDurationSec"`
	MaxDurationSec           int  `json:"maxDurationSec"`
	PreferDurationSec        int  `json:"preferDurationSec"`
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
	ID                string            `json:"id"`
	ProjectID         string            `json:"projectId"`
	SequenceIndex     int               `json:"sequenceIndex"`
	Title             string            `json:"title"`
	VideoType         string            `json:"videoType,omitempty"`
	DurationSec       int               `json:"durationSec"`
	SceneID           string            `json:"sceneId,omitempty"`
	SceneSummary      string            `json:"sceneSummary,omitempty"`
	SingleScene       bool              `json:"singleScene"`
	VisualChangeLevel string            `json:"visualChangeLevel"`
	Narration         string            `json:"narration,omitempty"`
	ScreenText        []string          `json:"screenText,omitempty"`
	MainAction        string            `json:"mainAction,omitempty"`
	Camera            string            `json:"camera,omitempty"`
	TransitionIn      string            `json:"transitionIn,omitempty"`
	TransitionOut     string            `json:"transitionOut,omitempty"`
	Continuity        ShotContinuity    `json:"continuity"`
	PromptConstraints PromptConstraints `json:"promptConstraints"`
	VisualPlan        VisualPlan        `json:"visualPlan,omitempty"`
	RenderStrategy    RenderStrategy    `json:"renderStrategy,omitempty"`
	ArtifactRefs      ShotArtifactRefs  `json:"artifactRefs,omitempty"`
	ReviewStatus      string            `json:"reviewStatus"`
	Locked            bool              `json:"locked"`
	Stale             bool              `json:"stale"`
	Version           int               `json:"version"`
	LastRejectReason  string            `json:"lastRejectReason,omitempty"`
	CreatedAt         time.Time         `json:"createdAt"`
	UpdatedAt         time.Time         `json:"updatedAt"`
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
