package service

import (
	"testing"

	"github.com/tangying-ai/aios-core/internal/agents/video/model"
)

func TestValidateComponentDSL_Valid(t *testing.T) {
	beats := []model.VisualBeat{
		{ID: "vb-1", Components: []model.ComponentDSL{{Type: "TITLE"}}},
		{ID: "vb-2", Components: []model.ComponentDSL{{Type: "BULLET_LIST"}, {Type: "QUOTE"}}},
	}
	if err := ValidateComponentDSL(beats); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateComponentDSL_Invalid(t *testing.T) {
	beats := []model.VisualBeat{
		{ID: "vb-1", Components: []model.ComponentDSL{{Type: "UNKNOWN_COMPONENT"}}},
	}
	if err := ValidateComponentDSL(beats); err == nil {
		t.Error("expected error for unknown component type")
	}
}
