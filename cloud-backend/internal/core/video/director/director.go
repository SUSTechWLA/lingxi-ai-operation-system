// Package director provides stage-specific constraints for Guided Video Studio.
// Each stage (proposal, script, composition, preview, render, package) has a
// Director that declares which tools are allowed and which are forbidden.
// These constraints are enforced by the PlanCompiler and PlanGuard.
package director

import (
	"sort"

	"github.com/tangying-ai/aios-core/internal/core/skillcapability"
	"github.com/tangying-ai/aios-core/internal/core/worker/tool"
)

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
	rolesByID map[string]*RoleAgentDirector
}

// NewRegistry creates an empty Director registry.
func NewRegistry() *Registry {
	return &Registry{
		directors: make(map[string]Director),
		rolesByID: make(map[string]*RoleAgentDirector),
	}
}

// Register adds a Director to the registry.
func (r *Registry) Register(d Director) {
	r.directors[d.StageName()] = d
	if role, ok := d.(*RoleAgentDirector); ok {
		r.rolesByID[role.RoleID()] = role
	}
}

// Get returns the Director for a stage, or nil if not found.
func (r *Registry) Get(stageName string) Director {
	return r.directors[stageName]
}

func (r *Registry) RegisterRoleAgent(agent skillcapability.RoleAgent) {
	r.Register(&RoleAgentDirector{agent: agent})
}

func (r *Registry) RegisterRoleAgents(agents []skillcapability.RoleAgent) {
	for _, agent := range agents {
		r.RegisterRoleAgent(agent)
	}
}

func (r *Registry) GetByID(roleID string) *RoleAgentDirector {
	return r.rolesByID[roleID]
}

func (r *Registry) List() []skillcapability.RoleAgent {
	roles := make([]skillcapability.RoleAgent, 0, len(r.rolesByID))
	for _, role := range r.rolesByID {
		roles = append(roles, role.Agent())
	}
	sort.SliceStable(roles, func(i, j int) bool {
		if roles[i].Stage == roles[j].Stage {
			return roles[i].ID < roles[j].ID
		}
		return roleOrder(roles[i].Stage) < roleOrder(roles[j].Stage)
	})
	return roles
}

// DefaultRegistry returns a Registry pre-populated with the v2.1 role agents
// for the guided image-text video pipeline.
func DefaultRegistry() *Registry {
	r := NewRegistry()
	r.RegisterRoleAgents(DefaultRoleAgents())
	return r
}

type RoleAgentDirector struct {
	agent skillcapability.RoleAgent
}

func (d *RoleAgentDirector) Agent() skillcapability.RoleAgent { return d.agent }

func (d *RoleAgentDirector) Name() string {
	if d.agent.Name != "" {
		return d.agent.Name
	}
	return d.agent.ID
}

func (d *RoleAgentDirector) StageName() string { return d.agent.Stage }

func (d *RoleAgentDirector) AllowedTools() []string { return d.agent.AllowedTools }

func (d *RoleAgentDirector) ForbiddenTools() []string { return d.agent.ForbiddenTools }

func (d *RoleAgentDirector) MaxToolCalls() int { return d.agent.MaxToolCalls }

func (d *RoleAgentDirector) RequiresApproval() bool {
	return d.agent.HumanReview != nil && d.agent.HumanReview.Required
}

func (d *RoleAgentDirector) RoleID() string { return d.agent.ID }

func (d *RoleAgentDirector) DisplayName() string { return d.agent.DisplayName }

func (d *RoleAgentDirector) Goal() string { return d.agent.Goal }

func (d *RoleAgentDirector) RequiredInputs() []string { return d.agent.RequiredInputs }

func (d *RoleAgentDirector) RequiredOutputs() []string { return d.agent.RequiredOutputs }

func (d *RoleAgentDirector) HumanReview() *tool.HumanReview { return d.agent.HumanReview }

func DefaultRoleAgents() []skillcapability.RoleAgent {
	return []skillcapability.RoleAgent{
		{
			ID:              "creative_director",
			Name:            "CreativeDirectorAgent",
			DisplayName:     "创意总监",
			Stage:           "proposal",
			Goal:            "理解用户需求，确定主题、平台、时长、风格和图文视频创作方向。",
			RequiredInputs:  []string{"USER_REQUEST"},
			RequiredOutputs: []string{"VIDEO_PROPOSAL"},
			AllowedTools:    []string{"pipeline_selector", "capability_preflight", "proposal_generator"},
			ForbiddenTools:  []string{"video_script_generator", "video_composition_builder", "hyperframes_project_generator", "hyperframes_renderer", "artifact_packager"},
			HumanReview:     review("after_artifact", "审核创作方案", []string{"主题是否准确", "目标用户是否明确", "是否适合图文视频第一版能力", "是否继续进入脚本阶段"}),
			MaxToolCalls:    5,
		},
		{
			ID:              "script_writer",
			Name:            "ScriptWriterAgent",
			DisplayName:     "脚本编剧",
			Stage:           "script",
			Goal:            "基于已确认创作方案生成中文图文视频口播脚本。",
			RequiredInputs:  []string{"VIDEO_PROPOSAL"},
			RequiredOutputs: []string{"VIDEO_SCRIPT"},
			AllowedTools:    []string{"video_script_generator", "script_quality_checker", "knowledge_researcher", "fact_checker"},
			ForbiddenTools:  []string{"video_composition_builder", "hyperframes_project_generator", "hyperframes_renderer", "artifact_packager"},
			HumanReview:     review("after_artifact", "审核口播脚本", []string{"开头是否有钩子", "表达是否自然", "观点是否准确", "是否适合后续拆成卡片"}),
			MaxToolCalls:    4,
		},
		{
			ID:              "storyboard_artist",
			Name:            "StoryboardArtistAgent",
			DisplayName:     "分镜/卡片设计师",
			Stage:           "storyboard",
			Goal:            "把已确认脚本拆成图文视频卡片页、字幕节奏和画面段落。",
			RequiredInputs:  []string{"VIDEO_SCRIPT"},
			RequiredOutputs: []string{"CARD_PLAN"},
			AllowedTools:    []string{"card_plan_generator", "caption_splitter"},
			ForbiddenTools:  []string{"hyperframes_project_generator", "hyperframes_renderer", "artifact_packager"},
			HumanReview:     review("after_artifact", "审核卡片/分镜计划", []string{"卡片顺序是否合理", "每页文字是否过长", "字幕拆分是否自然", "重点信息是否突出"}),
			MaxToolCalls:    3,
		},
		{
			ID:              "composition_director",
			Name:            "CompositionDirectorAgent",
			DisplayName:     "图文视频结构导演",
			Stage:           "composition",
			Goal:            "把 CARD_PLAN 转成 VideoCompositionSpec、轨道、时间轴和布局。",
			RequiredInputs:  []string{"VIDEO_SCRIPT", "CARD_PLAN"},
			RequiredOutputs: []string{"VIDEO_COMPOSITION_SPEC"},
			AllowedTools:    []string{"video_composition_builder", "composition_quality_checker"},
			ForbiddenTools:  []string{"hyperframes_renderer", "artifact_packager"},
			HumanReview:     review("after_artifact", "审核视频结构", []string{"卡片顺序是否合理", "时间轴是否完整", "字幕是否覆盖口播", "是否适合 HyperFrames 渲染"}),
			MaxToolCalls:    3,
		},
		{
			ID:              "reference_selector",
			Name:            "ReferenceSelectorAgent",
			DisplayName:     "参考资产选择器",
			Stage:           "reference",
			Goal:            "选择图文视频风格参考、背景策略、图标策略和字体策略。",
			RequiredInputs:  []string{"VIDEO_PROPOSAL", "VIDEO_COMPOSITION_SPEC"},
			RequiredOutputs: []string{"REFERENCE_ASSET_PLAN"},
			AllowedTools:    []string{"reference_asset_planner", "asset_policy_generator"},
			ForbiddenTools:  []string{"hyperframes_renderer", "artifact_packager"},
			MaxToolCalls:    2,
		},
		{
			ID:              "continuity_keeper",
			Name:            "ContinuityKeeperAgent",
			DisplayName:     "连续性管理",
			Stage:           "continuity",
			Goal:            "维护风格、术语和画面一致性，输出下游 stale 与一致性 warning。",
			RequiredInputs:  []string{"VIDEO_PROPOSAL", "VIDEO_SCRIPT", "CARD_PLAN", "VIDEO_COMPOSITION_SPEC"},
			RequiredOutputs: []string{"CONTINUITY_REPORT", "STYLE_PROFILE"},
			AllowedTools:    []string{"continuity_checker", "style_profile_builder", "stale_tracker"},
			ForbiddenTools:  []string{"hyperframes_renderer", "artifact_packager"},
			MaxToolCalls:    3,
		},
		{
			ID:              "preview_director",
			Name:            "PreviewDirectorAgent",
			DisplayName:     "预览导演",
			Stage:           "preview",
			Goal:            "生成 HyperFrames 项目和预览截图，检查可读性并交给用户确认。",
			RequiredInputs:  []string{"VIDEO_COMPOSITION_SPEC", "REFERENCE_ASSET_PLAN", "CONTINUITY_REPORT"},
			RequiredOutputs: []string{"HYPERFRAMES_PROJECT", "PREVIEW_SNAPSHOTS", "PREVIEW_REPORT"},
			AllowedTools:    []string{"hyperframes_project_generator", "hyperframes_snapshot", "preview_quality_checker"},
			ForbiddenTools:  []string{"hyperframes_renderer", "artifact_packager"},
			HumanReview:     review("after_artifact", "审核画面预览", []string{"画面是否可读", "字幕是否溢出", "卡片顺序是否正确", "是否允许最终渲染"}),
			MaxToolCalls:    5,
		},
		{
			ID:              "render_producer",
			Name:            "RenderProducerAgent",
			DisplayName:     "渲染制片",
			Stage:           "render",
			Goal:            "确认预览已通过后创建并跟踪本地 HyperFrames 渲染任务。",
			RequiredInputs:  []string{"PREVIEW_SNAPSHOTS"},
			RequiredOutputs: []string{"VIDEO", "RENDER_REPORT"},
			AllowedTools:    []string{"render_dependency_guard", "hyperframes_renderer", "local_job_status_tracker"},
			ForbiddenTools:  []string{"video_script_generator", "video_composition_builder", "hyperframes_project_generator", "artifact_packager"},
			HumanReview:     review("before_execute", "确认最终渲染", []string{"预览画面是否已确认", "视频结构是否正确", "是否允许开始本地渲染"}),
			MaxToolCalls:    3,
		},
		{
			ID:              "quality_reviewer",
			Name:            "QualityReviewerAgent",
			DisplayName:     "质量审核员",
			Stage:           "quality",
			Goal:            "检查 final.mp4 是否存在、时长和文件信息是否有效、产物是否完整。",
			RequiredInputs:  []string{"VIDEO"},
			RequiredOutputs: []string{"FINAL_REVIEW"},
			AllowedTools:    []string{"ffmpeg_probe", "final_review_generator"},
			ForbiddenTools:  []string{"video_script_generator", "hyperframes_renderer", "artifact_packager"},
			MaxToolCalls:    3,
		},
		{
			ID:              "package_producer",
			Name:            "PackageProducerAgent",
			DisplayName:     "交付制片",
			Stage:           "package",
			Goal:            "打包 final.mp4、spec、manifest、review 和项目交付包。",
			RequiredInputs:  []string{"VIDEO", "FINAL_REVIEW"},
			RequiredOutputs: []string{"PROJECT_PACKAGE"},
			AllowedTools:    []string{"artifact_packager", "package_quality_checker"},
			ForbiddenTools:  []string{"video_script_generator", "video_composition_builder", "hyperframes_renderer"},
			MaxToolCalls:    3,
		},
	}
}

func review(gate, title string, focus []string) *tool.HumanReview {
	return &tool.HumanReview{
		Required:    true,
		Gate:        gate,
		Title:       title,
		ReviewFocus: focus,
		UserActions: []string{"approve", "edit", "regenerate", "reject"},
	}
}

func roleOrder(stage string) int {
	switch stage {
	case "proposal":
		return 1
	case "script":
		return 2
	case "storyboard":
		return 3
	case "composition":
		return 4
	case "reference":
		return 5
	case "continuity":
		return 6
	case "preview":
		return 7
	case "render":
		return 8
	case "quality":
		return 9
	case "package":
		return 10
	default:
		return 100
	}
}

// --- Proposal Stage ---
// Only generates a creative brief. Must NOT produce scripts, compositions, or call render tools.

type ProposalDirector struct{}

func (d *ProposalDirector) Name() string           { return "proposal-director" }
func (d *ProposalDirector) StageName() string      { return "proposal" }
func (d *ProposalDirector) MaxToolCalls() int      { return 5 }
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

func (d *ScriptDirector) Name() string           { return "script-director" }
func (d *ScriptDirector) StageName() string      { return "script" }
func (d *ScriptDirector) MaxToolCalls() int      { return 3 }
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

func (d *CompositionDirector) Name() string           { return "composition-director" }
func (d *CompositionDirector) StageName() string      { return "composition" }
func (d *CompositionDirector) MaxToolCalls() int      { return 3 }
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

func (d *PreviewDirector) Name() string           { return "preview-director" }
func (d *PreviewDirector) StageName() string      { return "preview" }
func (d *PreviewDirector) MaxToolCalls() int      { return 5 }
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

func (d *RenderDirector) Name() string           { return "render-director" }
func (d *RenderDirector) StageName() string      { return "render" }
func (d *RenderDirector) MaxToolCalls() int      { return 3 }
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

func (d *PackageDirector) Name() string           { return "package-director" }
func (d *PackageDirector) StageName() string      { return "package" }
func (d *PackageDirector) MaxToolCalls() int      { return 3 }
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
