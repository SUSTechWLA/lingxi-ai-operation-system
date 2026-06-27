package apispec

import (
	videoassistant "github.com/tangying-ai/aios-core/internal/agents/video/assistant"
	videomodel "github.com/tangying-ai/aios-core/internal/agents/video/model"
	"github.com/tangying-ai/aios-core/internal/core/agentruntime"
	artifacts "github.com/tangying-ai/aios-core/internal/core/artifact"
	"github.com/tangying-ai/aios-core/internal/core/auth"
	"github.com/tangying-ai/aios-core/internal/core/localrunner"
	"github.com/tangying-ai/aios-core/internal/core/model"
	"github.com/tangying-ai/aios-core/internal/core/skillcapability"
	workflow "github.com/tangying-ai/aios-core/internal/core/workflow"
)

// registerCloudSchemas registers all component schemas (derived from real Go
// model types) and small spec-only response types.
func registerCloudSchemas(b *Builder) {
	// ── Generic / envelope ──
	b.Schema("ErrorResponse", &Schema{
		Type: "object",
		Properties: map[string]*SchemaRef{
			"code":    {Schema: IntegerSchema()},
			"message": {Schema: StringSchema()},
			"data":    {Schema: &Schema{Type: "null"}},
		},
	})
	b.Schema("GenericOKResponse", &Schema{
		Type: "object",
		Properties: map[string]*SchemaRef{
			"code":    {Schema: IntegerSchema()},
			"message": {Schema: StringSchema()},
		},
	})

	// ── Health ──
	b.Schema("HealthResponse", &Schema{
		Type: "object",
		Properties: map[string]*SchemaRef{
			"status":  {Schema: StringSchema()},
			"service": {Schema: StringSchema()},
		},
	})
	b.Schema("ReadinessResponse", &Schema{
		Type: "object",
		Properties: map[string]*SchemaRef{
			"status":       {Schema: StringSchema()},
			"service":      {Schema: StringSchema()},
			"dependencies": {Schema: ObjectSchema()},
		},
	})

	// ── Auth ──
	b.Schema("AuthRegisterRequest", Reflect(auth.RegisterRequest{}))
	b.Schema("AuthLoginRequest", Reflect(auth.LoginRequest{}))
	b.Schema("AuthRefreshRequest", Reflect(auth.RefreshRequest{}))
	b.Schema("AuthLogoutRequest", Reflect(auth.LogoutRequest{}))
	b.Schema("AuthResponse", &Schema{
		Type: "object",
		Properties: map[string]*SchemaRef{
			"code":    {Schema: IntegerSchema()},
			"message": {Schema: StringSchema()},
			"data":    {Schema: Reflect(auth.AuthResponse{})},
		},
	})
	b.Schema("CurrentUserResponse", &Schema{
		Type: "object",
		Properties: map[string]*SchemaRef{
			"code":    {Schema: IntegerSchema()},
			"message": {Schema: StringSchema()},
			"data":    {Schema: Reflect(auth.UserResponse{})},
		},
	})

	// ── Publish / AI ──
	b.Schema("PublishResponse", &Schema{
		Type: "object",
		Properties: map[string]*SchemaRef{
			"code":    {Schema: IntegerSchema()},
			"message": {Schema: StringSchema()},
			"data": {Schema: &Schema{
				Type: "object",
				Properties: map[string]*SchemaRef{
					"taskId":  {Schema: StringSchema()},
					"message": {Schema: StringSchema()},
				},
			}},
		},
	})
	b.Schema("AIGenerateResponse", &Schema{
		Type: "object",
		Properties: map[string]*SchemaRef{
			"code":    {Schema: IntegerSchema()},
			"message": {Schema: StringSchema()},
			"data": {Schema: &Schema{
				Type: "object",
				Properties: map[string]*SchemaRef{
					"title":       {Schema: StringSchema()},
					"description": {Schema: StringSchema()},
				},
			}},
		},
	})
	b.Schema("AIPolishResponse", &Schema{
		Type: "object",
		Properties: map[string]*SchemaRef{
			"code":    {Schema: IntegerSchema()},
			"message": {Schema: StringSchema()},
			"data": {Schema: &Schema{
				Type: "object",
				Properties: map[string]*SchemaRef{
					"content":  {Schema: StringSchema()},
					"taskId":   {Schema: StringSchema()},
					"traceUrl": {Schema: StringSchema()},
				},
			}},
		},
	})
	b.Schema("PolishSubmitResponse", &Schema{
		Type: "object",
		Properties: map[string]*SchemaRef{
			"code":    {Schema: IntegerSchema()},
			"message": {Schema: StringSchema()},
			"data": {Schema: &Schema{
				Type: "object",
				Properties: map[string]*SchemaRef{
					"taskId":   {Schema: StringSchema()},
					"nodeId":   {Schema: StringSchema()},
					"message":  {Schema: StringSchema()},
					"traceUrl": {Schema: StringSchema()},
				},
			}},
		},
	})
	b.Schema("PolishQueryResponse", &Schema{
		Type: "object",
		Properties: map[string]*SchemaRef{
			"code":    {Schema: IntegerSchema()},
			"message": {Schema: StringSchema()},
			"data": {Schema: &Schema{
				Type: "object",
				Properties: map[string]*SchemaRef{
					"taskId":   {Schema: StringSchema()},
					"nodeId":   {Schema: StringSchema()},
					"status":   {Schema: StringSchema()},
					"content":  {Schema: StringSchema()},
					"traceUrl": {Schema: StringSchema()},
				},
			}},
		},
	})

	// ── Trace ──
	b.Schema("TraceResponse", &Schema{
		Type: "object",
		Properties: map[string]*SchemaRef{
			"code":    {Schema: IntegerSchema()},
			"message": {Schema: StringSchema()},
			"data": {Schema: &Schema{
				Type: "object",
				Properties: map[string]*SchemaRef{
					"task":     {Schema: ObjectSchema()},
					"contexts": {Schema: ArraySchema(ObjectSchema())},
				},
			}},
		},
	})

	// ── Orchestrator / Task ──
	b.Schema("TaskCreateResponse", Reflect(model.Task{}))
	b.Schema("DAGRequest", Reflect(model.DAGRequest{}))
	b.Schema("DAGSubmitResponse", &Schema{
		Type: "object",
		Properties: map[string]*SchemaRef{
			"code":    {Schema: IntegerSchema()},
			"message": {Schema: StringSchema()},
			"data": {Schema: &Schema{
				Type: "object",
				Properties: map[string]*SchemaRef{
					"taskId":  {Schema: StringSchema()},
					"message": {Schema: StringSchema()},
				},
			}},
		},
	})
	b.Schema("TaskDetailResponse", &Schema{
		Type: "object",
		Properties: map[string]*SchemaRef{
			"code":    {Schema: IntegerSchema()},
			"message": {Schema: StringSchema()},
			"data":    {Schema: ObjectSchema()},
		},
	})
	b.Schema("TaskProgressResponse", &Schema{
		Type: "object",
		Properties: map[string]*SchemaRef{
			"code":    {Schema: IntegerSchema()},
			"message": {Schema: StringSchema()},
			"data":    {Schema: ObjectSchema()},
		},
	})
	b.Schema("ContextListResponse", &Schema{
		Type: "object",
		Properties: map[string]*SchemaRef{
			"code":    {Schema: IntegerSchema()},
			"message": {Schema: StringSchema()},
			"data":    {Schema: ArraySchema(ObjectSchema())},
		},
	})
	b.Schema("TaskLifecycleResponse", &Schema{
		Type: "object",
		Properties: map[string]*SchemaRef{
			"code":    {Schema: IntegerSchema()},
			"message": {Schema: StringSchema()},
			"data": {Schema: &Schema{
				Type: "object",
				Properties: map[string]*SchemaRef{
					"taskId":  {Schema: StringSchema()},
					"message": {Schema: StringSchema()},
				},
			}},
		},
	})
	b.Schema("PauseReasonResponse", &Schema{
		Type: "object",
		Properties: map[string]*SchemaRef{
			"code":    {Schema: IntegerSchema()},
			"message": {Schema: StringSchema()},
			"data": {Schema: &Schema{
				Type: "object",
				Properties: map[string]*SchemaRef{
					"taskId": {Schema: StringSchema()},
					"reason": {Schema: StringSchema()},
				},
			}},
		},
	})

	// ── Node ──
	b.Schema("NodeSuccessResponse", &Schema{
		Type: "object",
		Properties: map[string]*SchemaRef{
			"code":    {Schema: IntegerSchema()},
			"message": {Schema: StringSchema()},
			"data": {Schema: &Schema{
				Type:       "object",
				Properties: map[string]*SchemaRef{"message": {Schema: StringSchema()}},
			}},
		},
	})
	b.Schema("NodeSnapshotResponse", &Schema{
		Type: "object",
		Properties: map[string]*SchemaRef{
			"code":    {Schema: IntegerSchema()},
			"message": {Schema: StringSchema()},
			"data": {Schema: &Schema{
				Type: "object",
				Properties: map[string]*SchemaRef{
					"nodeId":   {Schema: StringSchema()},
					"snapshot": {Schema: ObjectSchema()},
				},
			}},
		},
	})
	b.Schema("NodeRetryResponse", &Schema{
		Type: "object",
		Properties: map[string]*SchemaRef{
			"code":    {Schema: IntegerSchema()},
			"message": {Schema: StringSchema()},
			"data": {Schema: &Schema{
				Type: "object",
				Properties: map[string]*SchemaRef{
					"nodeId":  {Schema: StringSchema()},
					"message": {Schema: StringSchema()},
				},
			}},
		},
	})

	// ── Local Runners ──
	b.Schema("RegisterRunnerRequest", Reflect(localrunner.RegisterRunnerRequest{}))
	b.Schema("RegisterRunnerResponse", Reflect(localrunner.RegisterRunnerResponse{}))
	b.Schema("HeartbeatRequest", Reflect(localrunner.HeartbeatRequest{}))
	b.Schema("ClaimJobResponse", Reflect(localrunner.ClaimJobResponse{}))
	b.Schema("ProgressRequest", Reflect(localrunner.ProgressRequest{}))
	b.Schema("CompleteJobRequest", Reflect(localrunner.CompleteJobRequest{}))
	b.Schema("FailJobRequest", Reflect(localrunner.FailJobRequest{}))
	b.Schema("LocalOKResponse", &Schema{
		Type: "object",
		Properties: map[string]*SchemaRef{
			"ok": {Schema: BoolSchema()},
		},
	})

	// ── Translate ──
	b.Schema("DAGResponse", &Schema{
		Type: "object",
		Properties: map[string]*SchemaRef{
			"code":    {Schema: IntegerSchema()},
			"message": {Schema: StringSchema()},
			"data": {Schema: &Schema{
				Type: "object",
				Properties: map[string]*SchemaRef{
					"nodes": {Schema: ArraySchema(ObjectSchema())},
					"edges": {Schema: ArraySchema(ObjectSchema())},
				},
			}},
		},
	})
	b.Schema("TranslateSubmitResponse", &Schema{
		Type: "object",
		Properties: map[string]*SchemaRef{
			"code":    {Schema: IntegerSchema()},
			"message": {Schema: StringSchema()},
			"data": {Schema: &Schema{
				Type: "object",
				Properties: map[string]*SchemaRef{
					"taskId":  {Schema: StringSchema()},
					"status":  {Schema: StringSchema()},
					"message": {Schema: StringSchema()},
				},
			}},
		},
	})
	b.Schema("TaskStatusResponse", &Schema{
		Type: "object",
		Properties: map[string]*SchemaRef{
			"code":    {Schema: IntegerSchema()},
			"message": {Schema: StringSchema()},
			"data":    {Schema: ObjectSchema()},
		},
	})

	// ── Media ──
	b.Schema("MediaListResponse", &Schema{
		Type: "object",
		Properties: map[string]*SchemaRef{
			"code":    {Schema: IntegerSchema()},
			"message": {Schema: StringSchema()},
			"data": {Schema: &Schema{
				Type: "object",
				Properties: map[string]*SchemaRef{
					"items": {Schema: ArraySchema(ObjectSchema())},
					"total": {Schema: IntegerSchema()},
				},
			}},
		},
	})
	b.Schema("MediaAssetResponse", &Schema{
		Type: "object",
		Properties: map[string]*SchemaRef{
			"code":    {Schema: IntegerSchema()},
			"message": {Schema: StringSchema()},
			"data": {Schema: &Schema{
				Type: "object",
				Properties: map[string]*SchemaRef{
					"id":           {Schema: StringSchema()},
					"userId":       {Schema: StringSchema()},
					"originalName": {Schema: StringSchema()},
					"mimeType":     {Schema: StringSchema()},
					"size":         {Schema: IntegerSchema()},
					"minioPath":    {Schema: StringSchema()},
					"tags":         {Schema: ArraySchema(StringSchema())},
					"createdAt":    {Schema: StringSchema()},
					"updatedAt":    {Schema: StringSchema()},
				},
			}},
		},
	})

	// ── Tools ──
	b.Schema("ToolListResponse", &Schema{
		Type: "object",
		Properties: map[string]*SchemaRef{
			"code":    {Schema: IntegerSchema()},
			"message": {Schema: StringSchema()},
			"data":    {Schema: ArraySchema(ObjectSchema())},
		},
	})
	b.Schema("ToolDetailResponse", &Schema{
		Type: "object",
		Properties: map[string]*SchemaRef{
			"code":    {Schema: IntegerSchema()},
			"message": {Schema: StringSchema()},
			"data":    {Schema: ObjectSchema()},
		},
	})
	b.Schema("ToolRegisterResponse", &Schema{
		Type: "object",
		Properties: map[string]*SchemaRef{
			"code":    {Schema: IntegerSchema()},
			"message": {Schema: StringSchema()},
			"data": {Schema: &Schema{
				Type: "object",
				Properties: map[string]*SchemaRef{
					"name": {Schema: StringSchema()},
					"type": {Schema: StringSchema()},
				},
			}},
		},
	})

	// ── Skills ──
	b.Schema("SkillsResponse", &Schema{
		Type: "object",
		Properties: map[string]*SchemaRef{
			"code":    {Schema: IntegerSchema()},
			"message": {Schema: StringSchema()},
			"data": {Schema: &Schema{
				Type: "object",
				Properties: map[string]*SchemaRef{
					"skills": {Schema: ArraySchema(ObjectSchema())},
					"health": {Schema: ObjectSchema()},
				},
			}},
		},
	})
	b.Schema("SkillCatalogResponse", &Schema{
		Type: "object",
		Properties: map[string]*SchemaRef{
			"code":    {Schema: IntegerSchema()},
			"message": {Schema: StringSchema()},
			"data": {Schema: &Schema{
				Type: "object",
				Properties: map[string]*SchemaRef{
					"skills": {Schema: ArraySchema(ObjectSchema())},
					"health": {Schema: ObjectSchema()},
				},
			}},
		},
	})
	b.Schema("SkillRouteResponse", &Schema{
		Type: "object",
		Properties: map[string]*SchemaRef{
			"code":    {Schema: IntegerSchema()},
			"message": {Schema: StringSchema()},
			"data":    {Schema: ObjectSchema()},
		},
	})
	b.Schema("SkillDetailResponse", &Schema{
		Type: "object",
		Properties: map[string]*SchemaRef{
			"code":    {Schema: IntegerSchema()},
			"message": {Schema: StringSchema()},
			"data": {Schema: &Schema{
				Type:       "object",
				Properties: map[string]*SchemaRef{"skill": {Schema: ObjectSchema()}},
			}},
		},
	})

	// ── Skill Capabilities ──
	b.Schema("SkillCapabilityListResponse", &Schema{
		Type: "object",
		Properties: map[string]*SchemaRef{
			"code":    {Schema: IntegerSchema()},
			"message": {Schema: StringSchema()},
			"data": {Schema: &Schema{
				Type: "object",
				Properties: map[string]*SchemaRef{
					"capabilities": {Schema: ArraySchema(Reflect(skillcapability.Manifest{}))},
				},
			}},
		},
	})
	b.Schema("SkillCapabilityDetailResponse", &Schema{
		Type: "object",
		Properties: map[string]*SchemaRef{
			"code":    {Schema: IntegerSchema()},
			"message": {Schema: StringSchema()},
			"data": {Schema: &Schema{
				Type: "object",
				Properties: map[string]*SchemaRef{
					"capability": {Schema: Reflect(skillcapability.Manifest{})},
				},
			}},
		},
	})

	// ── Video Role Agents ──
	b.Schema("VideoRoleAgentListResponse", &Schema{
		Type: "object",
		Properties: map[string]*SchemaRef{
			"code":    {Schema: IntegerSchema()},
			"message": {Schema: StringSchema()},
			"data": {Schema: &Schema{
				Type: "object",
				Properties: map[string]*SchemaRef{
					"roleAgents": {Schema: ArraySchema(Reflect(skillcapability.RoleAgent{}))},
				},
			}},
		},
	})
	b.Schema("VideoRoleAgentDetailResponse", &Schema{
		Type: "object",
		Properties: map[string]*SchemaRef{
			"code":    {Schema: IntegerSchema()},
			"message": {Schema: StringSchema()},
			"data": {Schema: &Schema{
				Type: "object",
				Properties: map[string]*SchemaRef{
					"roleAgent": {Schema: Reflect(skillcapability.RoleAgent{})},
				},
			}},
		},
	})

	// ── Dynamic Agent Runs ──
	b.Schema("AgentStartRunRequest", Reflect(agentruntime.StartRunRequest{}))
	b.Schema("AgentRunStartResponse", &Schema{
		Type: "object",
		Properties: map[string]*SchemaRef{
			"code":    {Schema: IntegerSchema()},
			"message": {Schema: StringSchema()},
			"data": {Schema: &Schema{
				Type: "object",
				Properties: map[string]*SchemaRef{
					"runId":  {Schema: StringSchema()},
					"taskId": {Schema: StringSchema()},
					"status": {Schema: StringSchema()},
					"plan":   {Schema: Reflect(agentruntime.AgentPlan{})},
				},
			}},
		},
	})
	b.Schema("AgentRunDetailResponse", &Schema{
		Type: "object",
		Properties: map[string]*SchemaRef{
			"code":    {Schema: IntegerSchema()},
			"message": {Schema: StringSchema()},
			"data": {Schema: &Schema{
				Type: "object",
				Properties: map[string]*SchemaRef{
					"run":  {Schema: Reflect(agentruntime.Run{})},
					"task": {Schema: ObjectSchema()},
				},
			}},
		},
	})
	b.Schema("AgentReviewListResponse", &Schema{
		Type: "object",
		Properties: map[string]*SchemaRef{
			"code":    {Schema: IntegerSchema()},
			"message": {Schema: StringSchema()},
			"data": {Schema: &Schema{
				Type: "object",
				Properties: map[string]*SchemaRef{
					"runId":   {Schema: StringSchema()},
					"reviews": {Schema: ArraySchema(Reflect(agentruntime.Review{}))},
				},
			}},
		},
	})
	b.Schema("AgentReviewDecisionResponse", &Schema{
		Type: "object",
		Properties: map[string]*SchemaRef{
			"code":    {Schema: IntegerSchema()},
			"message": {Schema: StringSchema()},
			"data": {Schema: &Schema{
				Type: "object",
				Properties: map[string]*SchemaRef{
					"reviewId": {Schema: StringSchema()},
					"status":   {Schema: StringSchema()},
				},
			}},
		},
	})

	// ── Workflows ──
	b.Schema("WorkflowListResponse", &Schema{
		Type: "object",
		Properties: map[string]*SchemaRef{
			"code":    {Schema: IntegerSchema()},
			"message": {Schema: StringSchema()},
			"data": {Schema: &Schema{
				Type: "object",
				Properties: map[string]*SchemaRef{
					"templates": {Schema: ArraySchema(ObjectSchema())},
				},
			}},
		},
	})
	b.Schema("WorkflowTemplateResponse", &Schema{
		Type: "object",
		Properties: map[string]*SchemaRef{
			"code":    {Schema: IntegerSchema()},
			"message": {Schema: StringSchema()},
			"data": {Schema: &Schema{
				Type:       "object",
				Properties: map[string]*SchemaRef{"template": {Schema: ObjectSchema()}},
			}},
		},
	})
	b.Schema("InstantiateResponse", &Schema{
		Type: "object",
		Properties: map[string]*SchemaRef{
			"code":    {Schema: IntegerSchema()},
			"message": {Schema: StringSchema()},
			"data": {Schema: &Schema{
				Type: "object",
				Properties: map[string]*SchemaRef{
					"task_id":   {Schema: StringSchema()},
					"message":   {Schema: StringSchema()},
					"trace_url": {Schema: StringSchema()},
				},
			}},
		},
	})

	// ── Video Projects ──
	b.Schema("VideoProjectListResponse", &Schema{
		Type: "object",
		Properties: map[string]*SchemaRef{
			"code":    {Schema: IntegerSchema()},
			"message": {Schema: StringSchema()},
			"data": {Schema: &Schema{
				Type: "object",
				Properties: map[string]*SchemaRef{
					"projects": {Schema: ArraySchema(ObjectSchema())},
					"total":    {Schema: IntegerSchema()},
				},
			}},
		},
	})
	b.Schema("VideoProjectCreateResponse", &Schema{
		Type: "object",
		Properties: map[string]*SchemaRef{
			"code":    {Schema: IntegerSchema()},
			"message": {Schema: StringSchema()},
			"data": {Schema: &Schema{
				Type:       "object",
				Properties: map[string]*SchemaRef{"project": {Schema: Reflect(videomodel.VideoProject{})}},
			}},
		},
	})
	b.Schema("VideoProjectDetailResponse", (&Schema{
		Type: "object",
		Properties: map[string]*SchemaRef{
			"code":    {Schema: IntegerSchema()},
			"message": {Schema: StringSchema()},
			"data": {Schema: &Schema{
				Type:       "object",
				Properties: map[string]*SchemaRef{"project": {Schema: Reflect(videomodel.VideoProject{})}},
			}},
		},
	}))

	// ── Video Project Assistant ──
	b.Schema("VideoAssistantMessageRequest", Reflect(videoassistant.MessageRequest{}))
	b.Schema("VideoAssistantReviseRequest", Reflect(videoassistant.ReviseRequest{}))
	b.Schema("VideoAssistantExplainStageRequest", Reflect(videoassistant.ExplainStageRequest{}))
	b.Schema("VideoAssistantMessageResponse", &Schema{
		Type: "object",
		Properties: map[string]*SchemaRef{
			"code":    {Schema: IntegerSchema()},
			"message": {Schema: StringSchema()},
			"data":    {Schema: Reflect(videoassistant.MessageResponse{})},
		},
	})
	b.Schema("VideoAssistantReviseResponse", &Schema{
		Type: "object",
		Properties: map[string]*SchemaRef{
			"code":    {Schema: IntegerSchema()},
			"message": {Schema: StringSchema()},
			"data":    {Schema: Reflect(videoassistant.ReviseResponse{})},
		},
	})
	b.Schema("VideoAssistantExplainStageResponse", &Schema{
		Type: "object",
		Properties: map[string]*SchemaRef{
			"code":    {Schema: IntegerSchema()},
			"message": {Schema: StringSchema()},
			"data":    {Schema: Reflect(videoassistant.ExplainStageResponse{})},
		},
	})

	// ── Workflow Runs ──
	b.Schema("WorkflowRunCreateResponse", &Schema{
		Type: "object",
		Properties: map[string]*SchemaRef{
			"code":    {Schema: IntegerSchema()},
			"message": {Schema: StringSchema()},
			"data": {Schema: &Schema{
				Type:       "object",
				Properties: map[string]*SchemaRef{"run": {Schema: ObjectSchema()}},
			}},
		},
	})
	b.Schema("WorkflowRunDetailResponse", &Schema{
		Type: "object",
		Properties: map[string]*SchemaRef{
			"code":    {Schema: IntegerSchema()},
			"message": {Schema: StringSchema()},
			"data": {Schema: &Schema{
				Type:       "object",
				Properties: map[string]*SchemaRef{"run": {Schema: ObjectSchema()}},
			}},
		},
	})

	// ── Stages ──
	b.Schema("StageApprovalResponse", &Schema{
		Type: "object",
		Properties: map[string]*SchemaRef{
			"code":    {Schema: IntegerSchema()},
			"message": {Schema: StringSchema()},
			"data": {Schema: &Schema{
				Type: "object",
				Properties: map[string]*SchemaRef{
					"nodeId":  {Schema: StringSchema()},
					"stage":   {Schema: StringSchema()},
					"message": {Schema: StringSchema()},
				},
			}},
		},
	})

	// ── Artifacts ──
	b.Schema("ArtifactListResponse", &Schema{
		Type: "object",
		Properties: map[string]*SchemaRef{
			"code":    {Schema: IntegerSchema()},
			"message": {Schema: StringSchema()},
			"data": {Schema: &Schema{
				Type:       "object",
				Properties: map[string]*SchemaRef{"artifacts": {Schema: ArraySchema(Reflect(artifacts.Artifact{}))}},
			}},
		},
	})
	b.Schema("ArtifactDetailResponse", &Schema{
		Type: "object",
		Properties: map[string]*SchemaRef{
			"code":    {Schema: IntegerSchema()},
			"message": {Schema: StringSchema()},
			"data": {Schema: &Schema{
				Type:       "object",
				Properties: map[string]*SchemaRef{"artifact": {Schema: Reflect(artifacts.Artifact{})}},
			}},
		},
	})
	b.Schema("ArtifactContentResponse", &Schema{
		Type: "object",
		Properties: map[string]*SchemaRef{
			"code":    {Schema: IntegerSchema()},
			"message": {Schema: StringSchema()},
			"data": {Schema: &Schema{
				Type: "object",
				Properties: map[string]*SchemaRef{
					"artifact":  {Schema: Reflect(artifacts.Artifact{})},
					"content":   {Schema: ObjectSchema()},
					"mediaUrl":  {Schema: StringSchema()},
					"mediaUrls": {Schema: ArraySchema(StringSchema())},
				},
			}},
		},
	})
	b.Schema("ArtifactHistoryResponse", &Schema{
		Type: "object",
		Properties: map[string]*SchemaRef{
			"code":    {Schema: IntegerSchema()},
			"message": {Schema: StringSchema()},
			"data": {Schema: &Schema{
				Type:       "object",
				Properties: map[string]*SchemaRef{"history": {Schema: ArraySchema(Reflect(artifacts.Artifact{}))}},
			}},
		},
	})

	// ── Real Go model types (used by multiple responses) ──
	b.Schema("VideoProject", Reflect(videomodel.VideoProject{}))
	b.Schema("Artifact", Reflect(artifacts.Artifact{}))
	b.Schema("WorkflowTemplate", Reflect(workflow.Template{}))

	// Suppress unused import warnings
	_ = model.TaskCreated
	_ = model.NodeCreated
	_ = workflow.Template{}
}
