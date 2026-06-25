package artifact

// DownstreamStaleArtifactKinds returns the beta v1 artifact kinds that must be
// invalidated when an upstream artifact changes.
func DownstreamStaleArtifactKinds(changedKind string) []string {
	order := []string{
		"VIDEO_PROPOSAL",
		"VIDEO_SCRIPT",
		"CARD_PLAN",
		"VIDEO_COMPOSITION_SPEC",
		"REFERENCE_ASSET_PLAN",
		"CONTINUITY_REPORT",
		"HYPERFRAMES_PROJECT",
		"PREVIEW_SNAPSHOTS",
		"VIDEO",
		"FINAL_REVIEW",
		"PROJECT_PACKAGE",
	}
	explicit := map[string][]string{
		"VIDEO_PROPOSAL":         order[1:],
		"VIDEO_SCRIPT":           order[2:],
		"CARD_PLAN":              order[3:],
		"VIDEO_COMPOSITION_SPEC": []string{"HYPERFRAMES_PROJECT", "PREVIEW_SNAPSHOTS", "VIDEO", "FINAL_REVIEW", "PROJECT_PACKAGE"},
		"PREVIEW_SNAPSHOTS":      []string{"VIDEO", "FINAL_REVIEW", "PROJECT_PACKAGE"},
	}
	if downstream, ok := explicit[changedKind]; ok {
		return append([]string{}, downstream...)
	}
	for i, kind := range order {
		if kind == changedKind {
			return append([]string{}, order[i+1:]...)
		}
	}
	return nil
}
