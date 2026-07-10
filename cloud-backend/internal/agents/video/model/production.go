package model

import "strings"

type ExecutionMode string

const (
	ExecutionModeUnknown     ExecutionMode = "unknown"
	ExecutionModeReal        ExecutionMode = "real"
	ExecutionModeFixture     ExecutionMode = "fixture"
	ExecutionModeFallback    ExecutionMode = "fallback"
	ExecutionModePlaceholder ExecutionMode = "placeholder"
)

const (
	ProductionModeStrict = "strict"
	ProductionModeDraft  = "draft"
	ProductionModeTest   = "test"
)

type AssemblyPolicy struct {
	ProductionMode string `json:"productionMode"`
}

func NormalizeShotCandidateExecution(candidate ShotCandidate) ShotCandidate {
	if candidate.ExecutionMode == "" {
		source := strings.ToLower(strings.TrimSpace(candidate.SourceType))
		switch {
		case candidate.IsFallback || strings.Contains(source, "fallback"):
			candidate.ExecutionMode = ExecutionModeFallback
		case strings.Contains(source, "fixture"):
			candidate.ExecutionMode = ExecutionModeFixture
		case strings.Contains(source, "placeholder"):
			candidate.ExecutionMode = ExecutionModePlaceholder
		case isKnownRealArtifactSource(source):
			candidate.ExecutionMode = ExecutionModeReal
			// Legacy candidates predate an explicit eligibility flag. Known real
			// outputs remain readable and eligible after normalization.
			candidate.ProductionEligible = true
		default:
			// Missing or unrecognized provenance must never be promoted to a real
			// production artifact merely because no fallback marker was present.
			candidate.ExecutionMode = ExecutionModeUnknown
		}
	}
	if candidate.ExecutionMode != ExecutionModeReal {
		candidate.ProductionEligible = false
	}
	if candidate.ExecutionMode == ExecutionModeFallback {
		candidate.IsFallback = true
	}
	return candidate
}

func IsStrictProductionMode(mode string) bool {
	normalized := strings.ToLower(strings.TrimSpace(mode))
	return normalized == "" || normalized == ProductionModeStrict
}

func isKnownRealArtifactSource(source string) bool {
	switch source {
	case ArtifactSourceAIGCVideo,
		ArtifactSourceAIGCImage,
		ArtifactSourceHyperFrames,
		ArtifactSourceFFmpegComposite,
		ArtifactSourceUploaded:
		return true
	default:
		return false
	}
}
