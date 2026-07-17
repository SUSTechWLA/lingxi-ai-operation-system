package artifact

// DownstreamStaleArtifactKinds returns the stage_name values of all downstream
// artifacts that must be invalidated when an upstream artifact changes.
//
// The input is a business kind identifier (e.g. "VIDEO_PROPOSAL", "VIDEO_SCRIPT")
// as stored in node.Input["requiredOutputs"]. The returned slice contains
// artifact stage_name values (e.g. "script", "storyboard", "composition")
// that match the artifacts.stage_name column and are used by MarkStaleByStageNames
// to UPDATE the correct rows.
//
// The dependency graph covers the full v1.0-beta pipeline:
//
//	proposal → script → storyboard → composition → reference →
//	continuity → preview → render → quality → package
func DownstreamStaleArtifactKinds(changedKind string) []string {
	// Map business kind identifiers to downstream stage_name values.
	// The stage_names correspond to artifact rows in the DB.
	graph := map[string][]string{
		"VIDEO_PROPOSAL": {
			"script",      // VIDEO_SCRIPT
			"storyboard",  // CARD_PLAN
			"composition", // VIDEO_COMPOSITION_SPEC
			"reference",   // REFERENCE_ASSET_PLAN
			"continuity",  // CONTINUITY_REPORT
			"preview",     // HYPERFRAMES_PROJECT + PREVIEW_SNAPSHOTS
			"render",      // VIDEO
			"quality",     // FFMPEG_PROBE_REPORT + FINAL_REVIEW
			"package",     // PROJECT_PACKAGE
		},
		"VIDEO_SCRIPT": {
			"storyboard",
			"composition",
			"reference",
			"continuity",
			"preview",
			"render",
			"quality",
			"package",
		},
		"CARD_PLAN": {
			"composition",
			"reference",
			"continuity",
			"preview",
			"render",
			"quality",
			"package",
		},
		"VIDEO_COMPOSITION_SPEC": {
			"preview", // HYPERFRAMES_PROJECT + PREVIEW_SNAPSHOTS
			"render",  // VIDEO
			"quality", // FFMPEG_PROBE_REPORT + FINAL_REVIEW
			"package", // PROJECT_PACKAGE
		},
		"REFERENCE_ASSET_PLAN": {
			"continuity",
			"preview",
			"render",
			"quality",
			"package",
		},
		"CONTINUITY_REPORT": {
			"preview",
			"render",
			"quality",
			"package",
		},
		"HYPERFRAMES_PROJECT": {
			"render",
			"quality",
			"package",
		},
		"PREVIEW_SNAPSHOTS": {
			"render",
			"quality",
			"package",
		},
		"VIDEO": {
			"quality",
			"package",
		},
		"FFMPEG_PROBE_REPORT": {
			"quality", // FINAL_REVIEW may reference probe data
			"package",
		},
		"FINAL_REVIEW": {
			"package",
		},
	}

	if downstream, ok := graph[changedKind]; ok {
		// Return a copy to prevent mutation by callers.
		result := make([]string, len(downstream))
		copy(result, downstream)
		return result
	}
	return nil
}

// DownstreamStageNamesForStage returns the stage_name values of all downstream
// artifacts that must be invalidated when the given stage_name changes.
// This is used when the caller already has the artifact's stage_name (from the DB).
//
// The ordered pipeline is:
//
//	proposal → script → storyboard → composition → reference →
//	continuity → preview → render → quality → package
func DownstreamStageNamesForStage(stageName string) []string {
	order := []string{
		"proposal",
		"script",
		"storyboard",
		"composition",
		"reference",
		"continuity",
		"preview",
		"render",
		"quality",
		"package",
	}
	for i, s := range order {
		if s == stageName {
			result := make([]string, len(order)-i-1)
			copy(result, order[i+1:])
			return result
		}
	}
	return nil
}

// DownstreamShotStageNamesForStage returns shot-scoped stage_name values that
// must be invalidated when a shot-scoped artifact changes. Callers should apply
// these to the same Artifact.UnitID as the changed shot artifact.
func DownstreamShotStageNamesForStage(stageName string) []string {
	graph := map[string][]string{
		"shot_unit": {
			"visual_plan",
			"render_strategy",
			"text_layers",
			"keyframe_prompt",
			"keyframe_image",
			"aigc_prompt",
			"html_source",
			"html_preview_video",
			"html_overlay_video",
			"html_overlay_alpha_video",
			"aigc_background_video",
			"aigc_shot_video",
			"composited_shot_video",
		},
		"visual_plan": {
			"render_strategy",
			"text_layers",
			"keyframe_prompt",
			"keyframe_image",
			"aigc_prompt",
			"html_source",
			"html_preview_video",
			"html_overlay_video",
			"html_overlay_alpha_video",
			"aigc_background_video",
			"aigc_shot_video",
			"composited_shot_video",
		},
		"render_strategy": {
			"text_layers",
			"keyframe_prompt",
			"keyframe_image",
			"aigc_prompt",
			"html_source",
			"html_preview_video",
			"html_overlay_video",
			"html_overlay_alpha_video",
			"aigc_background_video",
			"aigc_shot_video",
			"composited_shot_video",
		},
		"text_layers": {
			"html_source",
			"html_preview_video",
			"html_overlay_video",
			"html_overlay_alpha_video",
			"composited_shot_video",
		},
		"aigc_prompt": {
			"aigc_background_video",
			"aigc_shot_video",
			"composited_shot_video",
		},
		"html_source": {
			"html_preview_video",
			"html_overlay_video",
			"html_overlay_alpha_video",
			"composited_shot_video",
		},
		"html_preview_video": {
			"composited_shot_video",
		},
		"html_overlay_video": {
			"composited_shot_video",
		},
		"html_overlay_alpha_video": {
			"composited_shot_video",
		},
		"aigc_background_video": {
			"composited_shot_video",
		},
	}
	if downstream, ok := graph[stageName]; ok {
		result := make([]string, len(downstream))
		copy(result, downstream)
		return result
	}
	return nil
}

// ProjectStageNamesForShotStageChange returns project-scoped artifacts that
// must be invalidated whenever a shot-scoped artifact changes.
func ProjectStageNamesForShotStageChange(stageName string) []string {
	switch stageName {
	case "shot_unit",
		"visual_plan",
		"render_strategy",
		"text_layers",
		"keyframe_prompt",
		"keyframe_image",
		"aigc_prompt",
		"html_source",
		"html_preview_video",
		"html_overlay_video",
		"html_overlay_alpha_video",
		"aigc_background_video",
		"aigc_shot_video",
		"composited_shot_video":
		return []string{"final_video", "publish_package"}
	default:
		return nil
	}
}
