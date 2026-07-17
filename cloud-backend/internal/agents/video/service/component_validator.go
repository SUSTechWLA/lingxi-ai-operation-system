package service

import (
	"fmt"

	"github.com/tangying-ai/aios-core/internal/agents/video/model"
)

// ValidateComponentDSL checks that all components use only whitelisted types.
func ValidateComponentDSL(beats []model.VisualBeat) error {
	for _, beat := range beats {
		for _, comp := range beat.Components {
			if !model.IsValidComponentType(comp.Type) {
				return fmt.Errorf("visual beat %s: unknown component type %q (allowed: TITLE, BULLET_LIST, IMAGE_SPLIT, KPI_CARD, QUOTE, COMPARISON, CALLOUT, TIMELINE, TEXT_OVERLAY, BACKGROUND)", beat.ID, comp.Type)
			}
		}
	}
	return nil
}
