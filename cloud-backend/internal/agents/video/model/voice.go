package model

// NarrationBeat represents a timed segment of narration text.
type NarrationBeat struct {
	ID          string   `json:"id"`
	Index       int      `json:"index"`
	Text        string   `json:"text"`
	DurationSec float64  `json:"durationSec"`
	Tone        string   `json:"tone,omitempty"`
	Emphasis    []string `json:"emphasis,omitempty"`
}

// VisualBeat represents the visual plan for a narration segment.
type VisualBeat struct {
	ID              string         `json:"id"`
	NarrationBeatID string         `json:"narrationBeatId"`
	StartTimeSec    float64        `json:"startTimeSec"`
	DurationSec     float64        `json:"durationSec"`
	Components      []ComponentDSL `json:"components"`
	TransitionIn    string         `json:"transitionIn,omitempty"`
	TransitionOut   string         `json:"transitionOut,omitempty"`
}

// ComponentDSL describes a single visual component.
type ComponentDSL struct {
	Type      string                 `json:"type"`
	Props     map[string]interface{} `json:"props"`
	Animation string                 `json:"animation,omitempty"`
	Position  *ComponentPosition     `json:"position,omitempty"`
}

// ComponentPosition defines position and size.
type ComponentPosition struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// ValidComponentTypes is the whitelist of allowed component types.
var ValidComponentTypes = map[string]bool{
	"TITLE":       true,
	"BULLET_LIST": true,
	"IMAGE_SPLIT": true,
	"KPI_CARD":    true,
	"QUOTE":       true,
	"COMPARISON":  true,
	"CALLOUT":     true,
	"TIMELINE":    true,
	"TEXT_OVERLAY": true,
	"BACKGROUND":  true,
}

// IsValidComponentType checks if a component type is allowed.
func IsValidComponentType(t string) bool {
	return ValidComponentTypes[t]
}
