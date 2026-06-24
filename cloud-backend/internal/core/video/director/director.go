// Package director provides stage-specific constraints for Guided Video Studio.
// Each stage (proposal, script, composition, preview, render, package) has a
// Director that declares which tools are allowed and which are forbidden.
// These constraints are enforced by the PlanCompiler and PlanGuard.
package director

// Director defines the constraints for a single video production stage.
type Director interface {
	Name() string
	StageName() string
	AllowedTools() []string
	ForbiddenTools() []string
	MaxToolCalls() int
	RequiresApproval() bool
}

// Registry maps stage names to their Directors.
type Registry struct {
	directors map[string]Director
}

// NewRegistry creates an empty Director registry.
func NewRegistry() *Registry {
	return &Registry{directors: make(map[string]Director)}
}

// Register adds a Director to the registry.
func (r *Registry) Register(d Director) {
	r.directors[d.StageName()] = d
}

// Get returns the Director for a stage, or nil if not found.
func (r *Registry) Get(stageName string) Director {
	return r.directors[stageName]
}

// DefaultRegistry returns a Registry pre-populated with all 6 stage directors
// for the guided image-text video pipeline.
func DefaultRegistry() *Registry {
	r := NewRegistry()
	r.Register(&ProposalDirector{})
	r.Register(&ScriptDirector{})
	r.Register(&CompositionDirector{})
	r.Register(&PreviewDirector{})
	r.Register(&RenderDirector{})
	r.Register(&PackageDirector{})
	return r
}

// --- Proposal Stage ---
// Only generates a creative brief. Must NOT produce scripts, compositions, or call render tools.

type ProposalDirector struct{}

func (d *ProposalDirector) Name() string        { return "proposal-director" }
func (d *ProposalDirector) StageName() string   { return "proposal" }
func (d *ProposalDirector) MaxToolCalls() int   { return 5 }
func (d *ProposalDirector) RequiresApproval() bool { return true }

func (d *ProposalDirector) AllowedTools() []string {
	return []string{
		"knowledge_researcher",
		"capability_preflight",
		"pipeline_selector",
		"proposal_generator",
		"video_script_generator", // for proposal-only output mode
	}
}

func (d *ProposalDirector) ForbiddenTools() []string {
	return []string{
		"hyperframes_project_generator",
		"hyperframes_renderer",
		"hyperframes_linter",
		"hyperframes_snapshot",
		"ffmpeg_probe",
		"ffmpeg_clip_extract",
		"artifact_packager",
		"video_package_exporter",
		"video_composition_builder",
		"shot_splitter",
		"video_prompt_generator",
	}
}

// --- Script Stage ---
// Generates the oral script. Must NOT produce composition specs or call render tools.

type ScriptDirector struct{}

func (d *ScriptDirector) Name() string         { return "script-director" }
func (d *ScriptDirector) StageName() string    { return "script" }
func (d *ScriptDirector) MaxToolCalls() int    { return 3 }
func (d *ScriptDirector) RequiresApproval() bool { return true }

func (d *ScriptDirector) AllowedTools() []string {
	return []string{
		"video_script_generator",
		"script_quality_checker",
		"knowledge_researcher",
		"fact_checker",
	}
}

func (d *ScriptDirector) ForbiddenTools() []string {
	return []string{
		"hyperframes_project_generator",
		"hyperframes_renderer",
		"hyperframes_linter",
		"hyperframes_snapshot",
		"ffmpeg_probe",
		"artifact_packager",
		"video_package_exporter",
		"video_composition_builder",
		"shot_splitter",
		"video_prompt_generator",
	}
}

// --- Composition Stage ---
// Converts the approved script into a VideoCompositionSpec.
// Must NOT call render or project generation tools.

type CompositionDirector struct{}

func (d *CompositionDirector) Name() string         { return "composition-director" }
func (d *CompositionDirector) StageName() string    { return "composition" }
func (d *CompositionDirector) MaxToolCalls() int    { return 3 }
func (d *CompositionDirector) RequiresApproval() bool { return true }

func (d *CompositionDirector) AllowedTools() []string {
	return []string{
		"video_composition_builder",
		"shot_splitter",
		"video_script_generator",
	}
}

func (d *CompositionDirector) ForbiddenTools() []string {
	return []string{
		"hyperframes_project_generator",
		"hyperframes_renderer",
		"hyperframes_linter",
		"hyperframes_snapshot",
		"ffmpeg_probe",
		"artifact_packager",
		"video_package_exporter",
	}
}

// --- Preview Stage ---
// Generates the HyperFrames project and preview snapshots.
// Must NOT call the actual renderer — only project generation and lint.

type PreviewDirector struct{}

func (d *PreviewDirector) Name() string         { return "preview-director" }
func (d *PreviewDirector) StageName() string    { return "preview" }
func (d *PreviewDirector) MaxToolCalls() int    { return 5 }
func (d *PreviewDirector) RequiresApproval() bool { return true }

func (d *PreviewDirector) AllowedTools() []string {
	return []string{
		"hyperframes_project_generator",
		"hyperframes_linter",
		"hyperframes_snapshot",
		"video_composition_builder",
		"capability_preflight",
	}
}

func (d *PreviewDirector) ForbiddenTools() []string {
	return []string{
		"hyperframes_renderer",
		"ffmpeg_probe",
		"artifact_packager",
		"video_package_exporter",
	}
}

// --- Render Stage ---
// Renders the final MP4. Must only render — no project generation, no composition.

type RenderDirector struct{}

func (d *RenderDirector) Name() string         { return "render-director" }
func (d *RenderDirector) StageName() string    { return "render" }
func (d *RenderDirector) MaxToolCalls() int    { return 3 }
func (d *RenderDirector) RequiresApproval() bool { return false }

func (d *RenderDirector) AllowedTools() []string {
	return []string{
		"hyperframes_renderer",
		"ffmpeg_probe",
	}
}

func (d *RenderDirector) ForbiddenTools() []string {
	return []string{
		"hyperframes_project_generator",
		"video_composition_builder",
		"video_script_generator",
		"shot_splitter",
		"video_prompt_generator",
		"knowledge_researcher",
		"fact_checker",
	}
}

// --- Package Stage ---
// Packages the final output. Must only package — no render, no generation.

type PackageDirector struct{}

func (d *PackageDirector) Name() string         { return "package-director" }
func (d *PackageDirector) StageName() string    { return "package" }
func (d *PackageDirector) MaxToolCalls() int    { return 3 }
func (d *PackageDirector) RequiresApproval() bool { return true }

func (d *PackageDirector) AllowedTools() []string {
	return []string{
		"artifact_packager",
		"video_package_exporter",
		"ffmpeg_probe",
		"publish_copy_generator",
	}
}

func (d *PackageDirector) ForbiddenTools() []string {
	return []string{
		"hyperframes_project_generator",
		"hyperframes_renderer",
		"hyperframes_linter",
		"video_composition_builder",
		"video_script_generator",
		"shot_splitter",
		"video_prompt_generator",
		"knowledge_researcher",
	}
}
