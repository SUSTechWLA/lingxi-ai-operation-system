package model

// ShotStatus tracks the production state of a shot.
type ShotStatus string

const (
	ShotPending    ShotStatus = "PENDING"
	ShotGenerating ShotStatus = "GENERATING"
	ShotCompleted  ShotStatus = "COMPLETED"
	ShotApproved   ShotStatus = "APPROVED"
	ShotRejected   ShotStatus = "REJECTED"
)

// Script represents the full screenplay output of the script stage.
type Script struct {
	Title      string      `json:"title"`
	Logline    string      `json:"logline"`
	Scenes     []SceneDef  `json:"scenes"`
	Characters []Character `json:"characters"`
	Props      []Prop      `json:"props"`
	StyleNotes string      `json:"styleNotes,omitempty"`
}

// SceneDef describes a scene location and setting.
type SceneDef struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Location    string `json:"location"`
	TimeOfDay   string `json:"timeOfDay,omitempty"`
}

// Character describes a character in the production.
type Character struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Role        string `json:"role"`
	Appearance  string `json:"appearance,omitempty"`
	Age         string `json:"age,omitempty"`
}

// Prop describes a prop used in the production.
type Prop struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	SceneID     string `json:"sceneId,omitempty"`
}

// ShotDefinition describes a single shot in the storyboard.
type ShotDefinition struct {
	ID              string  `json:"id"`
	SceneID         string  `json:"sceneId"`
	ShotNumber      int     `json:"shotNumber"`
	DurationSec     float64 `json:"durationSec"`
	Description     string  `json:"description"`
	CameraMovement  string  `json:"cameraMovement,omitempty"`
	Framing         string  `json:"framing,omitempty"`
	Lighting        string  `json:"lighting,omitempty"`
	KeyframePrompt  string  `json:"keyframePrompt,omitempty"`
	VideoPrompt     string  `json:"videoPrompt,omitempty"`
	Status          ShotStatus `json:"status"`
}

// ShotPackage bundles a shot with its generated assets.
type ShotPackage struct {
	Shot           ShotDefinition `json:"shot"`
	StoryboardURL  string         `json:"storyboardUrl,omitempty"`
	KeyframeURL    string         `json:"keyframeUrl,omitempty"`
	VideoURL       string         `json:"videoUrl,omitempty"`
	GenerationMode string         `json:"generationMode"` // provider_api | manual_import
}

// BoundaryState tracks continuity state between shots.
type BoundaryState struct {
	CameraState    string `json:"cameraState,omitempty"`
	LightingState  string `json:"lightingState,omitempty"`
	BeforeShotID   string `json:"beforeShotId,omitempty"`
	AfterShotID    string `json:"afterShotId,omitempty"`
}
