package apispec

import (
	videoassets "github.com/tangying-ai/aios-core/internal/agents/video/assets"
	videoassistant "github.com/tangying-ai/aios-core/internal/agents/video/assistant"
	videomodel "github.com/tangying-ai/aios-core/internal/agents/video/model"
	videoservice "github.com/tangying-ai/aios-core/internal/agents/video/service"
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
	b.Schema("VideoPreflightResponse", Reflect(localrunner.PreflightResponse{}))

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
	b.Schema("AgentRunTraceResponse", &Schema{
		Type: "object",
		Properties: map[string]*SchemaRef{
			"code":    {Schema: IntegerSchema()},
			"message": {Schema: StringSchema()},
			"data": {Schema: &Schema{
				Type: "object",
				Properties: map[string]*SchemaRef{
					"task":  {Schema: ObjectSchema()},
					"nodes": {Schema: ArraySchema(Reflect(model.Node{}))},
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
	b.Schema("GenerateVideoCreationSpecRequest", Reflect(videoservice.GenerateSpecRequest{}))
	b.Schema("RejectVideoCreationArtifactRequest", Reflect(videoservice.RejectRequest{}))
	b.Schema("RegenerateShotRequest", Reflect(videoservice.RegenerateShotRequest{}))

	// Creator Studio contracts. The stricter mutation schemas deliberately do
	// not reuse the legacy RegenerateShotRequest: the V2 handlers require a
	// positive baseVersion, an exact scope, and an explicit locks array.
	creatorStepID := enumSchema("requirements", "direction", "script", "shots", "preview", "delivery")
	creatorStepState := enumSchema("not_started", "generating", "needs_review", "confirmed", "needs_attention", "failed")
	reviewStatus := enumSchema("pending", "approved", "rejected", "stale")
	shotQAStatus := enumSchema("PLANNED", "GENERATING", "CANDIDATE_RENDERED", "SHOT_QA_RUNNING", "SHOT_QA_PASSED", "SHOT_QA_FAILED", "HUMAN_REVIEW_REQUIRED", "ACCEPTED_FOR_ASSEMBLY", "stale")
	shotGenerationStatus := enumSchema("PLANNED", "GENERATING", "CANDIDATE_RENDERED", "SHOT_QA_RUNNING", "SHOT_QA_PASSED", "SHOT_QA_FAILED", "HUMAN_REVIEW_REQUIRED", "ACCEPTED_FOR_ASSEMBLY", "stale", "queued", "dispatching", "running", "failed", "cancelled")
	candidateStatus := enumSchema("CANDIDATE_RENDERED", "SHOT_QA_RUNNING", "SHOT_QA_PASSED", "SHOT_QA_FAILED", "HUMAN_REVIEW_REQUIRED", "ACCEPTED_FOR_ASSEMBLY", "stale")
	regenerationScope := enumSchema("prompt", "reference", "base_media", "overlay", "audio_alignment", "full_shot")
	regenerationStatus := enumSchema("queued", "dispatching", "running", "completed", "failed", "cancelled")
	shotLocks := enumSchema("duration", "narration", "character", "wardrobe", "scene", "camera", "first_frame", "last_frame", "reference_set", "accepted_overlay")
	positiveVersion := func() *Schema {
		minimum := float64(1)
		return &Schema{Type: "integer", Minimum: &minimum, Description: "Positive current version"}
	}
	requiredEnvelope := func(data *Schema) *Schema {
		return &Schema{Type: "object", Properties: map[string]*SchemaRef{
			"code": {Schema: IntegerSchema()}, "message": {Schema: StringSchema()}, "data": {Schema: data},
		}, Required: []string{"code", "message", "data"}}
	}
	requiredObject := func(properties map[string]*SchemaRef, required ...string) *Schema {
		return &Schema{Type: "object", Properties: properties, Required: required}
	}
	closedObject := func(properties map[string]*SchemaRef, required ...string) *Schema {
		allowed := false
		return &Schema{Type: "object", Properties: properties, Required: required, AdditionalProperties: &AdditionalProperties{Allowed: &allowed}}
	}
	freeFormObject := func(description string) *Schema {
		return &Schema{Type: "object", Description: description, AdditionalProperties: &AdditionalProperties{Schema: &SchemaRef{Schema: &Schema{Description: "arbitrary JSON value"}}}}
	}

	b.Schema("CreatorStep", requiredObject(map[string]*SchemaRef{
		"id": {Schema: creatorStepID}, "label": {Schema: StringSchema()}, "state": {Schema: creatorStepState},
		"currentArtifactId": {Schema: StringSchema()}, "currentVersion": {Schema: IntegerSchema()},
		"reviewId": {Schema: StringSchema()}, "runId": {Schema: StringSchema()},
		"hasHistory": {Schema: BoolSchema()}, "attemptCount": {Schema: IntegerSchema()}, "artifactCount": {Schema: IntegerSchema()},
		"startedAt": {Schema: &Schema{Type: "string", Format: "date-time", Nullable: true}},
		"updatedAt": {Schema: &Schema{Type: "string", Format: "date-time", Nullable: true}}, "isStale": {Schema: BoolSchema()},
		"allowedActions": {Schema: ArraySchema(StringSchema())},
	}, "id", "label", "state", "hasHistory", "attemptCount", "artifactCount", "isStale", "allowedActions"))
	creatorArtifactDescriptor := Reflect(videomodel.CreatorArtifactDescriptor{})
	creatorArtifactDescriptor.Properties["stepId"] = &SchemaRef{Schema: creatorStepID}
	b.Schema("CreatorArtifactDescriptor", creatorArtifactDescriptor)
	creatorProcessEvent := Reflect(videomodel.CreatorProcessEvent{})
	creatorProcessEvent.Properties["stepId"] = &SchemaRef{Schema: creatorStepID}
	creatorProcessEvent.Properties["state"] = &SchemaRef{Schema: enumSchema("started", "generated", "needs_review", "confirmed", "failed", "stale")}
	creatorProcessEvent.Properties["sourceType"] = &SchemaRef{Schema: enumSchema("artifact", "review", "agent_node", "shot", "project")}
	b.Schema("CreatorProcessEvent", creatorProcessEvent)
	creatorTask := Reflect(videomodel.CreatorTask{})
	creatorTask.Properties["scope"] = &SchemaRef{Schema: creatorStepID}
	creatorTask.Properties["status"] = &SchemaRef{Schema: enumSchema(
		string(videomodel.CreatorTaskGenerating), string(videomodel.CreatorTaskRunning), string(videomodel.CreatorTaskProcessing),
		string(videomodel.CreatorTaskQueued), string(videomodel.CreatorTaskDispatching),
	)}
	b.Schema("CreatorTask", creatorTask)
	b.Schema("ShotSummary", Reflect(videomodel.ShotSummary{}))
	shotListItem := Reflect(videomodel.ShotListItem{})
	shotListItem.Properties["reviewStatus"] = &SchemaRef{Schema: reviewStatus}
	shotListItem.Properties["qaStatus"] = &SchemaRef{Schema: shotQAStatus}
	shotListItem.Properties["generationStatus"] = &SchemaRef{Schema: shotGenerationStatus}
	b.Schema("ShotListItem", shotListItem)
	b.Schema("ShotImpact", Reflect(videomodel.ShotImpact{}))
	shotRevision := Reflect(videomodel.ShotRevision{})
	shotRevision.Properties["snapshot"] = &SchemaRef{Schema: RefSchema("ShotUnit")}
	b.Schema("ShotRevision", shotRevision)
	b.Schema("ShotWorkspace", requiredObject(map[string]*SchemaRef{
		"shot": {Schema: RefSchema("ShotUnit")}, "history": {Schema: ArraySchema(RefSchema("ShotRevision"))}, "impact": {Schema: RefSchema("ShotImpact")},
	}, "shot", "history", "impact"))
	b.Schema("CreatorArtifactVersion", Reflect(videomodel.CreatorArtifactVersion{}))
	stepImpact := Reflect(videomodel.StepImpact{})
	stepImpact.Properties["affectedStepIds"] = &SchemaRef{Schema: ArraySchema(creatorStepID)}
	b.Schema("StepImpact", stepImpact)
	materialKind := enumSchema("image", "audio", "video", "document")
	zero := float64(0)
	materialProperties := map[string]*SchemaRef{
		"name": {Schema: StringSchema()}, "kind": {Schema: materialKind},
		"mimeType":    {Schema: &Schema{Type: "string", Description: "Valid media type whose top-level type must match kind"}},
		"sizeBytes":   {Schema: &Schema{Type: "integer", Format: "int64", Minimum: &zero}},
		"storageRef":  {Schema: &Schema{Type: "string", Description: "Canonical local, project-scoped storage reference"}},
		"contentHash": {Schema: &Schema{Type: "string", Pattern: `^sha256:[A-Za-z0-9][A-Za-z0-9._-]*$`}},
	}
	b.Schema("ProjectMaterial", requiredObject(materialProperties, "name", "kind", "mimeType", "sizeBytes", "storageRef", "contentHash"))
	b.Schema("ProjectMaterialManifest", requiredObject(map[string]*SchemaRef{
		"schemaVersion": {Schema: IntegerSchema()}, "relatedProjectId": {Schema: StringSchema()}, "materials": {Schema: ArraySchema(RefSchema("ProjectMaterial"))},
	}, "schemaVersion", "relatedProjectId", "materials"))

	videoProject := Reflect(videomodel.VideoProject{})
	videoProject.Properties["mode"] = &SchemaRef{Schema: enumSchema("aigc_shot", "voice_visual", "cinematic_story")}
	videoProject.Properties["status"] = &SchemaRef{Schema: enumSchema("DRAFT", "RUNNING", "PAUSED", "COMPLETED", "ARCHIVED")}
	videoProject.Properties["generationMode"] = &SchemaRef{Schema: enumSchema("provider_api", "manual_import")}
	videoProject.Properties["config"] = &SchemaRef{Schema: freeFormObject("Project-specific JSON configuration")}
	b.Schema("VideoProject", videoProject)

	shotQAReport := Reflect(videomodel.ShotQAReport{})
	shotQAReport.Properties["status"] = &SchemaRef{Schema: shotQAStatus}
	b.Schema("ShotQAReport", shotQAReport)
	shotCandidate := Reflect(videomodel.ShotCandidate{})
	shotCandidate.Properties["status"] = &SchemaRef{Schema: candidateStatus}
	shotCandidate.Properties["executionMode"] = &SchemaRef{Schema: enumSchema("unknown", "real", "fixture", "fallback", "placeholder")}
	shotCandidate.Properties["qaReport"] = &SchemaRef{Schema: &Schema{Ref: "#/components/schemas/ShotQAReport", Nullable: true}}
	b.Schema("ShotCandidate", shotCandidate)
	shotUnit := Reflect(videomodel.ShotUnit{})
	shotUnit.Properties["reviewStatus"] = &SchemaRef{Schema: reviewStatus}
	shotUnit.Properties["qaStatus"] = &SchemaRef{Schema: shotQAStatus}
	shotUnit.Properties["candidates"] = &SchemaRef{Schema: ArraySchema(RefSchema("ShotCandidate"))}
	b.Schema("ShotUnit", shotUnit)
	shotRegenerationTask := Reflect(videomodel.ShotRegenerationTask{})
	shotRegenerationTask.Properties["scope"] = &SchemaRef{Schema: regenerationScope}
	shotRegenerationTask.Properties["locks"] = &SchemaRef{Schema: ArraySchema(shotLocks)}
	shotRegenerationTask.Properties["status"] = &SchemaRef{Schema: regenerationStatus}
	b.Schema("ShotRegenerationTask", shotRegenerationTask)

	creationView := requiredObject(map[string]*SchemaRef{
		"project": {Schema: RefSchema("VideoProject")}, "activeStep": {Schema: creatorStepID},
		"finalDeliveryArtifactId": {Schema: StringSchema()},
		"steps":                   {Schema: ArraySchema(RefSchema("CreatorStep"))}, "shotSummary": {Schema: RefSchema("ShotSummary")},
		"activeTasks": {Schema: ArraySchema(RefSchema("CreatorTask"))}, "assemblyDirty": {Schema: BoolSchema()},
		"processTimeline": {Schema: ArraySchema(RefSchema("CreatorProcessEvent"))},
		"stepArtifacts":   {Schema: &Schema{Type: "object", AdditionalProperties: &AdditionalProperties{Schema: &SchemaRef{Schema: ArraySchema(RefSchema("CreatorArtifactDescriptor"))}}}},
	}, "project", "activeStep", "steps", "shotSummary", "activeTasks", "assemblyDirty", "processTimeline", "stepArtifacts")
	b.Schema("CreationView", creationView)

	selection := &Schema{OneOf: []*SchemaRef{
		{Schema: closedObject(map[string]*SchemaRef{
			"kind": {Schema: enumSchema("rect")}, "x": {Schema: &Schema{Type: "number"}}, "y": {Schema: &Schema{Type: "number"}},
			"width": {Schema: &Schema{Type: "number"}}, "height": {Schema: &Schema{Type: "number"}},
		}, "kind", "x", "y", "width", "height")},
		{Schema: closedObject(map[string]*SchemaRef{
			"kind": {Schema: enumSchema("time")}, "startMs": {Schema: &Schema{Type: "integer", Format: "int64"}}, "endMs": {Schema: &Schema{Type: "integer", Format: "int64"}},
		}, "kind", "startMs", "endMs")},
		{Schema: closedObject(map[string]*SchemaRef{
			"kind": {Schema: enumSchema("text")}, "start": {Schema: IntegerSchema()}, "end": {Schema: IntegerSchema()}, "text": {Schema: StringSchema()},
			"sourceHash": {Schema: &Schema{Type: "string", Pattern: `^sha256:[0-9a-f]{64}$`}},
		}, "kind", "start", "end", "text", "sourceHash")},
	}}
	b.Schema("ArtifactSelection", selection)
	replacementMaterialIdentity := requiredObject(map[string]*SchemaRef{
		"contentHash": {Schema: &Schema{Type: "string", Pattern: `^sha256:.+$`}},
		"storageRef":  {Schema: &Schema{Type: "string", Pattern: `^local://.+$`}},
		"mimeType":    {Schema: &Schema{Type: "string", Pattern: `^image/.+$`}},
		"sizeBytes":   {Schema: &Schema{Type: "integer", Format: "int64", Minimum: &zero}},
	}, "contentHash", "storageRef", "mimeType", "sizeBytes")
	b.Schema("ReplacementMaterialIdentity", replacementMaterialIdentity)
	b.Schema("StepRevisionPreviewRequest", requiredObject(map[string]*SchemaRef{
		"artifactId": {Schema: StringSchema()}, "baseVersion": {Schema: positiveVersion()},
	}, "artifactId", "baseVersion"))
	mutationProperties := func(mode string) map[string]*SchemaRef {
		return map[string]*SchemaRef{
			"artifactId": {Schema: StringSchema()}, "baseVersion": {Schema: positiveVersion()}, "mode": {Schema: enumSchema(mode)},
			"runId": {Schema: StringSchema()}, "reviewId": {Schema: StringSchema()},
			"confirmedAffectedShotIds": {Schema: ArraySchema(StringSchema())}, "selection": {Schema: &Schema{Ref: "#/components/schemas/ArtifactSelection", Nullable: true}},
		}
	}
	directMutation := mutationProperties("direct")
	directMutation["directContent"] = &SchemaRef{Schema: StringSchema()}
	instructionMutation := mutationProperties("instruction")
	instructionMutation["instruction"] = &SchemaRef{Schema: StringSchema()}
	instructionMutation["modelProviders"] = &SchemaRef{Schema: freeFormObject("Transient desktop model-provider credentials; never persisted")}
	replaceMutation := mutationProperties("replace")
	replaceMutation["replacementMaterial"] = &SchemaRef{Schema: RefSchema("ReplacementMaterialIdentity")}
	replaceSelection := closedObject(
		map[string]*SchemaRef{
			"kind": {Schema: enumSchema("rect")}, "x": {Schema: &Schema{Type: "number"}}, "y": {Schema: &Schema{Type: "number"}},
			"width": {Schema: &Schema{Type: "number"}}, "height": {Schema: &Schema{Type: "number"}},
		},
		"kind", "x", "y", "width", "height",
	)
	replaceSelection.Nullable = true
	replaceMutation["selection"] = &SchemaRef{Schema: replaceSelection}
	b.Schema("StepRevisionMutationRequest", &Schema{OneOf: []*SchemaRef{
		{Schema: closedObject(directMutation, "artifactId", "baseVersion", "mode", "directContent", "confirmedAffectedShotIds")},
		{Schema: closedObject(instructionMutation, "artifactId", "baseVersion", "mode", "instruction", "confirmedAffectedShotIds")},
		{Schema: closedObject(replaceMutation, "artifactId", "baseVersion", "mode", "replacementMaterial", "confirmedAffectedShotIds")},
	}})
	b.Schema("StepConfirmRequest", requiredObject(map[string]*SchemaRef{
		"artifactId": {Schema: StringSchema()}, "runId": {Schema: StringSchema()}, "reviewId": {Schema: StringSchema()}, "comment": {Schema: StringSchema()},
	}, "artifactId"))
	b.Schema("StepRestoreRequest", requiredObject(map[string]*SchemaRef{
		"baseVersion": {Schema: positiveVersion()}, "runId": {Schema: StringSchema()}, "reviewId": {Schema: StringSchema()},
		"reason": {Schema: StringSchema()}, "confirmedAffectedShotIds": {Schema: ArraySchema(StringSchema())},
	}, "baseVersion", "confirmedAffectedShotIds"))
	b.Schema("StepRegenerationRequest", requiredObject(map[string]*SchemaRef{
		"baseArtifactId": {Schema: StringSchema()}, "baseVersion": {Schema: IntegerSchema()},
		"instruction": {Schema: StringSchema()}, "runId": {Schema: StringSchema()}, "reviewId": {Schema: StringSchema()},
		"confirmedAffectedStepIds": {Schema: ArraySchema(creatorStepID)},
	}, "confirmedAffectedStepIds"))
	b.Schema("RegisterProjectMaterialRequest", closedObject(materialProperties,
		"name", "kind", "storageRef", "mimeType", "sizeBytes", "contentHash"))
	b.Schema("ShotRegenerationRequest", requiredObject(map[string]*SchemaRef{
		"baseVersion": {Schema: positiveVersion()}, "scope": {Schema: regenerationScope},
		"locks": {Schema: ArraySchema(shotLocks)}, "instruction": {Schema: StringSchema()},
	}, "baseVersion", "scope", "locks"))
	b.Schema("CandidateAcceptRequest", requiredObject(map[string]*SchemaRef{
		"baseVersion": {Schema: positiveVersion()}, "scope": {Schema: enumSchema("candidate_accept")}, "locks": {Schema: ArraySchema(shotLocks)},
	}, "baseVersion", "scope", "locks"))
	b.Schema("CandidateRestoreRequest", requiredObject(map[string]*SchemaRef{
		"baseVersion": {Schema: positiveVersion()}, "scope": {Schema: enumSchema("candidate_restore")}, "locks": {Schema: ArraySchema(shotLocks)},
	}, "baseVersion", "scope", "locks"))

	b.Schema("CreationViewResponse", requiredEnvelope(RefSchema("CreationView")))
	b.Schema("StepVersionsResponse", requiredEnvelope(requiredObject(map[string]*SchemaRef{
		"versions": {Schema: ArraySchema(RefSchema("CreatorArtifactVersion"))},
	}, "versions")))
	b.Schema("StepImpactResponse", requiredEnvelope(RefSchema("StepImpact")))
	b.Schema("StepMutationResponse", requiredEnvelope(requiredObject(map[string]*SchemaRef{
		"artifact": {Schema: Reflect(artifacts.Artifact{})}, "impact": {Schema: RefSchema("StepImpact")}, "view": {Schema: RefSchema("CreationView")},
	}, "artifact", "impact", "view")))
	b.Schema("StepRegenerationResponse", requiredEnvelope(requiredObject(map[string]*SchemaRef{
		"runId": {Schema: StringSchema()}, "reviewId": {Schema: StringSchema()}, "attempt": {Schema: positiveVersion()},
		"impact": {Schema: RefSchema("StepImpact")}, "view": {Schema: RefSchema("CreationView")},
	}, "runId", "reviewId", "attempt", "impact", "view")))
	b.Schema("ProjectMaterialResponse", requiredEnvelope(requiredObject(map[string]*SchemaRef{
		"material": {Schema: RefSchema("ProjectMaterial")}, "artifact": {Schema: Reflect(artifacts.Artifact{})},
	}, "material", "artifact")))
	b.Schema("ShotPageResponse", requiredEnvelope(requiredObject(map[string]*SchemaRef{
		"shots": {Schema: ArraySchema(RefSchema("ShotListItem"))}, "nextCursor": {Schema: StringSchema()}, "total": {Schema: IntegerSchema()},
	}, "shots", "nextCursor", "total")))
	b.Schema("ShotSummaryResponse", requiredEnvelope(requiredObject(map[string]*SchemaRef{"summary": {Schema: RefSchema("ShotSummary")}}, "summary")))
	b.Schema("ShotWorkspaceResponse", requiredEnvelope(requiredObject(map[string]*SchemaRef{"workspace": {Schema: RefSchema("ShotWorkspace")}}, "workspace")))
	b.Schema("ShotHistoryResponse", requiredEnvelope(requiredObject(map[string]*SchemaRef{"history": {Schema: ArraySchema(RefSchema("ShotRevision"))}}, "history")))
	b.Schema("ShotImpactResponse", requiredEnvelope(requiredObject(map[string]*SchemaRef{"impact": {Schema: RefSchema("ShotImpact")}}, "impact")))
	b.Schema("ShotRegenerationResponse", requiredEnvelope(requiredObject(map[string]*SchemaRef{
		"shot": {Schema: RefSchema("ShotUnit")}, "task": {Schema: RefSchema("ShotRegenerationTask")},
	}, "shot", "task")))
	b.Schema("CreatorShotUnitResponse", requiredEnvelope(requiredObject(map[string]*SchemaRef{
		"shot": {Schema: RefSchema("ShotUnit")},
	}, "shot")))
	b.Schema("VideoCreationSpecResponse", &Schema{
		Type: "object",
		Properties: map[string]*SchemaRef{
			"code":    {Schema: IntegerSchema()},
			"message": {Schema: StringSchema()},
			"data": {Schema: &Schema{
				Type:       "object",
				Properties: map[string]*SchemaRef{"spec": {Schema: Reflect(videomodel.VideoCreationSpec{})}},
			}},
		},
	})
	b.Schema("ShotUnitListResponse", &Schema{
		Type: "object",
		Properties: map[string]*SchemaRef{
			"code":    {Schema: IntegerSchema()},
			"message": {Schema: StringSchema()},
			"data": {Schema: &Schema{
				Type:       "object",
				Properties: map[string]*SchemaRef{"shots": {Schema: ArraySchema(Reflect(videomodel.ShotUnit{}))}},
			}},
		},
	})
	b.Schema("ShotUnitResponse", &Schema{
		Type: "object",
		Properties: map[string]*SchemaRef{
			"code":    {Schema: IntegerSchema()},
			"message": {Schema: StringSchema()},
			"data": {Schema: &Schema{
				Type:       "object",
				Properties: map[string]*SchemaRef{"shot": {Schema: Reflect(videomodel.ShotUnit{})}},
			}},
		},
	})
	b.Schema("VisualPlanResponse", &Schema{
		Type: "object",
		Properties: map[string]*SchemaRef{
			"code":    {Schema: IntegerSchema()},
			"message": {Schema: StringSchema()},
			"data": {Schema: &Schema{
				Type:       "object",
				Properties: map[string]*SchemaRef{"visualPlan": {Schema: Reflect(videomodel.VisualPlan{})}},
			}},
		},
	})
	b.Schema("RenderStrategyResponse", &Schema{
		Type: "object",
		Properties: map[string]*SchemaRef{
			"code":    {Schema: IntegerSchema()},
			"message": {Schema: StringSchema()},
			"data": {Schema: &Schema{
				Type:       "object",
				Properties: map[string]*SchemaRef{"renderStrategy": {Schema: Reflect(videomodel.RenderStrategy{})}},
			}},
		},
	})
	b.Schema("TextLayersResponse", &Schema{
		Type: "object",
		Properties: map[string]*SchemaRef{
			"code":    {Schema: IntegerSchema()},
			"message": {Schema: StringSchema()},
			"data": {Schema: &Schema{
				Type:       "object",
				Properties: map[string]*SchemaRef{"textLayers": {Schema: ArraySchema(Reflect(videomodel.TextLayerSpec{}))}},
			}},
		},
	})
	b.Schema("AssemblyValidationResponse", &Schema{
		Type: "object",
		Properties: map[string]*SchemaRef{
			"code":    {Schema: IntegerSchema()},
			"message": {Schema: StringSchema()},
			"data": {Schema: &Schema{
				Type:       "object",
				Properties: map[string]*SchemaRef{"issues": {Schema: ArraySchema(Reflect(videoservice.ValidationIssue{}))}},
			}},
		},
	})
	b.Schema("AssemblyRebuildResponse", requiredEnvelope(requiredObject(map[string]*SchemaRef{
		"assembly": {Schema: Reflect(videoservice.AssemblyRebuildResult{})},
	}, "assembly")))
	b.Schema("PublishPackageResponse", &Schema{
		Type: "object",
		Properties: map[string]*SchemaRef{
			"code":    {Schema: IntegerSchema()},
			"message": {Schema: StringSchema()},
			"data": {Schema: &Schema{
				Type:       "object",
				Properties: map[string]*SchemaRef{"publishPackage": {Schema: ObjectSchema()}},
			}},
		},
	})

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
	b.Schema("WorkflowCheckpointListResponse", &Schema{
		Type: "object",
		Properties: map[string]*SchemaRef{
			"code":    {Schema: IntegerSchema()},
			"message": {Schema: StringSchema()},
			"data": {Schema: &Schema{
				Type:       "object",
				Properties: map[string]*SchemaRef{"checkpoints": {Schema: ArraySchema(Reflect(workflow.Checkpoint{}))}},
			}},
		},
	})
	b.Schema("WorkflowCheckpointRecoverResponse", &Schema{
		Type: "object",
		Properties: map[string]*SchemaRef{
			"code":    {Schema: IntegerSchema()},
			"message": {Schema: StringSchema()},
			"data": {Schema: &Schema{
				Type: "object",
				Properties: map[string]*SchemaRef{
					"checkpoint": {Schema: Reflect(workflow.Checkpoint{})},
					"message":    {Schema: StringSchema()},
				},
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
	b.Schema("RegisterExternalGenerationResultRequest", Reflect(videoassets.RegisterExternalGenerationResultRequest{}))
	b.Schema("ExternalGenerationResultResponse", &Schema{
		Type: "object",
		Properties: map[string]*SchemaRef{
			"code":    {Schema: IntegerSchema()},
			"message": {Schema: StringSchema()},
			"data": {Schema: &Schema{
				Type: "object",
				Properties: map[string]*SchemaRef{
					"manifest": {Schema: Reflect(videoassets.ManualAssetManifest{})},
					"artifact": {Schema: Reflect(artifacts.Artifact{})},
				},
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
					"artifact":             {Schema: Reflect(artifacts.Artifact{})},
					"content":              {Schema: ObjectSchema()},
					"mediaUrl":             {Schema: StringSchema()},
					"mediaUrls":            {Schema: ArraySchema(StringSchema())},
					"reviewText":           {Schema: StringSchema()},
					"reviewTextSourceHash": {Schema: &Schema{Type: "string", Pattern: `^sha256:[0-9a-f]{64}$`}},
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
	b.Schema("VideoCreationSpec", Reflect(videomodel.VideoCreationSpec{}))
	b.Schema("VisualPlan", Reflect(videomodel.VisualPlan{}))
	b.Schema("RenderStrategy", Reflect(videomodel.RenderStrategy{}))
	b.Schema("Artifact", Reflect(artifacts.Artifact{}))
	b.Schema("WorkflowTemplate", Reflect(workflow.Template{}))

	// Suppress unused import warnings
	_ = model.TaskCreated
	_ = model.NodeCreated
	_ = workflow.Template{}
}

func enumSchema(values ...string) *Schema {
	enums := make([]any, len(values))
	for index, value := range values {
		enums[index] = value
	}
	return &Schema{Type: "string", Enum: enums}
}
