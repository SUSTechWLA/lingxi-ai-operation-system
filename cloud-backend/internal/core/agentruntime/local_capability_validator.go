package agentruntime

import "github.com/tangying-ai/aios-core/internal/core/worker/tool"

// LocalCapabilityValidator checks whether the local execution plane can satisfy
// the requirements of a plan step before the plan is committed.
//
// Currently a stub — returns nil (no warnings). Future implementation will
// query the runner registry to verify that at least one online runner has the
// required capabilities for tools with executionPlane=local.
type LocalCapabilityValidator struct {
}

// ValidateForLocalExecution checks whether the given step can be executed on
// the local plane. Returns a list of warnings; an empty/nil list means the
// step is compatible with the available local runners.
func (v *LocalCapabilityValidator) ValidateForLocalExecution(step AgentStep, manifest *tool.ToolManifest) []string {
	// Stub: no warnings.
	// Future: check if any online runner can satisfy manifest.LocalRequirements.
	_ = manifest
	_ = step
	return nil
}
