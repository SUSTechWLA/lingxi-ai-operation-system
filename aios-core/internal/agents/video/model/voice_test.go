package model

import "testing"

func TestIsValidComponentType(t *testing.T) {
	if !IsValidComponentType("TITLE") {
		t.Error("TITLE should be valid")
	}
	if !IsValidComponentType("BULLET_LIST") {
		t.Error("BULLET_LIST should be valid")
	}
	if IsValidComponentType("INVALID_TYPE") {
		t.Error("INVALID_TYPE should not be valid")
	}
	if IsValidComponentType("") {
		t.Error("empty string should not be valid")
	}
}

func TestValidComponentTypesCount(t *testing.T) {
	if len(ValidComponentTypes) < 8 {
		t.Errorf("expected at least 8 component types, got %d", len(ValidComponentTypes))
	}
}

func TestComponentDSL(t *testing.T) {
	dsl := ComponentDSL{
		Type:      "TITLE",
		Props:     map[string]interface{}{"text": "Hello", "size": "large"},
		Animation: "fade_in",
	}
	if dsl.Type != "TITLE" {
		t.Errorf("expected TITLE, got %s", dsl.Type)
	}
	if dsl.Props["text"] != "Hello" {
		t.Error("props not set correctly")
	}
}

func TestNarrationBeat(t *testing.T) {
	beat := NarrationBeat{
		ID:          "nb-1",
		Index:       0,
		Text:        "开篇观点",
		DurationSec: 15.0,
		Tone:        "正式",
	}
	if beat.ID == "" {
		t.Error("id should not be empty")
	}
	if beat.DurationSec <= 0 {
		t.Error("duration should be positive")
	}
}

func TestVisualBeat(t *testing.T) {
	beat := VisualBeat{
		ID:              "vb-1",
		NarrationBeatID: "nb-1",
		StartTimeSec:    0,
		DurationSec:     15.0,
		Components: []ComponentDSL{
			{Type: "TITLE", Props: map[string]interface{}{"text": "AIOS"}},
		},
	}
	if len(beat.Components) != 1 {
		t.Errorf("expected 1 component, got %d", len(beat.Components))
	}
	if !IsValidComponentType(beat.Components[0].Type) {
		t.Error("component type should be valid")
	}
}
