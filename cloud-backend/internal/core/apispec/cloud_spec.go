package apispec

// BuildCloudSpec constructs the authoritative OpenAPI 3.0 spec for the cloud
// backend. Every API endpoint is listed here — when you add, rename, or remove
// a route, update this file AND the corresponding handler's RegisterRoutes.
//
// Regenerate docs after changes:
//
//	make gen-docs  # overwrites docs/API_REFERENCE.md + frontend types
func BuildCloudSpec() *Spec {
	b := New("Tangying AIOS Cloud API", "0.1.0").
		Server("http://localhost:8080", "Local development server").
		Tag("Health", "Service health and readiness").
		Tag("Auth", "User registration, login, token refresh, and session identity").
		Tag("Publish", "Multi-platform content publishing — frontend-facing").
		Tag("AI", "AI-assisted content generation and polishing").
		Tag("Trace", "Task lifecycle trace/debugging").
		Tag("Orchestrator", "DAG task creation and orchestration").
		Tag("Node", "Node-level operations (reporting, retry, snapshots)").
		Tag("Translate", "Natural language → DAG translation").
		Tag("Context", "Task/node context auditing").
		Tag("Media", "Media asset upload, listing, and tagging").
		Tag("Tools", "Tool registry management").
		Tag("Skills", "AI skill catalog and routing").
		Tag("Skill Capabilities", "Agent capability package catalog").
		Tag("Video Role Agents", "Guided Video Studio role-agent catalog and stage constraints").
		Tag("Agent Runs", "Dynamic agent runtime runs and review gates").
		Tag("Local Runners", "Cloud control-plane protocol for local execution runners").
		Tag("Workflows", "Reusable workflow templates (blueprints)").
		Tag("Video Projects", "Video creation project CRUD").
		Tag("Workflow Runs", "Video workflow run lifecycle").
		Tag("Stages", "Stage-level approval for video pipelines").
		Tag("Artifacts", "Video creation artifacts (JSON, Markdown, media)").
		Tag("Video Project Assistant", "Video-project-scoped assistant for stage explanation and artifact revision guidance")

	// ── Health ──
	b.Route("GET", "/api/health", "Liveness check").
		Tags("Health").
		ResponseJSON("200", "Service is up", "HealthResponse")
	b.Route("GET", "/api/health/ready", "Readiness check with dependencies").
		Tags("Health").
		ResponseJSON("200", "All dependencies healthy", "ReadinessResponse").
		ResponseJSON("503", "One or more dependency down", "ReadinessResponse")

	// ── Auth ──
	b.Route("POST", "/api/auth/register", "Register a user with email and password").
		Tags("Auth").
		BodyJSON("AuthRegisterRequest", "Registration payload", true).
		ResponseJSON("200", "Registered and authenticated", "AuthResponse").
		ResponseJSON("400", "Invalid request", "ErrorResponse").
		ResponseJSON("409", "Email already registered", "ErrorResponse")
	b.Route("POST", "/api/auth/login", "Log in with email and password").
		Tags("Auth").
		BodyJSON("AuthLoginRequest", "Login payload", true).
		ResponseJSON("200", "Authenticated", "AuthResponse").
		ResponseJSON("401", "Invalid credentials", "ErrorResponse")
	b.Route("POST", "/api/auth/refresh", "Rotate refresh token and return new tokens").
		Tags("Auth").
		BodyJSON("AuthRefreshRequest", "Refresh token payload", true).
		ResponseJSON("200", "Refreshed", "AuthResponse").
		ResponseJSON("401", "Invalid refresh token", "ErrorResponse")
	b.Route("POST", "/api/auth/logout", "Revoke the current refresh token").
		Tags("Auth").
		BodyJSON("AuthLogoutRequest", "Refresh token payload", true).
		ResponseJSON("200", "Logged out", "GenericOKResponse").
		ResponseJSON("401", "Missing or invalid access token", "ErrorResponse")
	b.Route("GET", "/api/auth/me", "Get the current authenticated user").
		Tags("Auth").
		ResponseJSON("200", "Current user", "CurrentUserResponse").
		ResponseJSON("401", "Missing or invalid access token", "ErrorResponse")

	// ── Publish ──
	b.Route("POST", "/api/publish", "Submit content for multi-platform publishing").
		Tags("Publish").
		BodyMultipart(map[string]*Schema{
			"title":        StringSchema(),
			"description":  StringSchema(),
			"keywords":     StringSchema(),
			"content_type": StringSchema(),
			"platforms":    StringSchema(),
		}, false).
		ResponseJSON("200", "Task created", "PublishResponse").
		ResponseJSON("400", "Missing required fields", "ErrorResponse")
	b.Route("POST", "/api/ai/generate", "Generate content from text prompt").
		Tags("AI").
		BodyInlineJSON(&Schema{
			Type: "object",
			Properties: map[string]*SchemaRef{
				"prompt": {Schema: StringSchema()},
			},
			Required: []string{"prompt"},
		}, "Content generation prompt", true).
		ResponseJSON("200", "Generated content", "AIGenerateResponse")
	b.Route("POST", "/api/ai/generate-from-media", "Generate content from media files + prompt").
		Tags("AI").
		BodyMultipart(map[string]*Schema{
			"prompt": StringSchema(),
		}, false).
		ResponseJSON("200", "Generated content", "AIGenerateResponse")
	b.Route("POST", "/api/ai/polish", "Polish text via AI (synchronous)").
		Tags("AI").
		BodyInlineJSON(&Schema{
			Type: "object",
			Properties: map[string]*SchemaRef{
				"text": {Schema: StringSchema()},
				"type": {Schema: StringSchema()},
			},
			Required: []string{"text"},
		}, "Text to polish", true).
		ResponseJSON("200", "Polished result", "AIPolishResponse")
	b.Route("POST", "/api/ai/polish/submit", "Submit async polish task").
		Tags("AI").
		BodyInlineJSON(&Schema{
			Type: "object",
			Properties: map[string]*SchemaRef{
				"text": {Schema: StringSchema()},
				"type": {Schema: StringSchema()},
			},
			Required: []string{"text"},
		}, "Text to polish asynchronously", true).
		ResponseJSON("200", "Task submitted", "PolishSubmitResponse")
	b.Route("GET", "/api/ai/polish/result", "Query async polish result").
		Tags("AI").
		QueryParam("taskId", "Task ID from submit response", StringSchema(), true).
		QueryParam("nodeId", "Node ID (optional)", StringSchema(), false).
		ResponseJSON("200", "Polish result or status", "PolishQueryResponse")

	// ── Trace ──
	b.Route("GET", "/api/trace/recent", "Get most recent task trace").
		Tags("Trace").
		ResponseJSON("200", "Trace data", "TraceResponse").
		ResponseJSON("404", "No tasks found", "ErrorResponse")
	b.Route("GET", "/api/trace/:taskId", "Get task trace by ID").
		Tags("Trace").
		PathParam("taskId", "Task identifier", StringSchema()).
		ResponseJSON("200", "Trace data", "TraceResponse").
		ResponseJSON("404", "Task not found", "ErrorResponse")

	// ── Orchestrator / Task ──
	b.Route("POST", "/api/task/create", "Create a new empty task").
		Tags("Orchestrator").
		BodyInlineJSON(ObjectSchema(), "Task metadata (userId, etc.)", false).
		ResponseJSON("200", "Task created", "TaskCreateResponse")
	b.Route("POST", "/api/task/:taskId/dag", "Submit a DAG to a task").
		Tags("Orchestrator").
		PathParam("taskId", "Task identifier", StringSchema()).
		BodyJSON("DAGRequest", "DAG nodes + edges", true).
		ResponseJSON("200", "DAG submitted", "DAGSubmitResponse")
	b.Route("GET", "/api/task/:taskId", "Get task with node details").
		Tags("Orchestrator").
		PathParam("taskId", "Task identifier", StringSchema()).
		ResponseJSON("200", "Task + nodes", "TaskDetailResponse")
	b.Route("GET", "/api/task/:taskId/progress", "Get task execution progress").
		Tags("Orchestrator").
		PathParam("taskId", "Task identifier", StringSchema()).
		ResponseJSON("200", "Task progress", "TaskProgressResponse")
	b.Route("GET", "/api/task/:taskId/context", "Get context records for task").
		Tags("Orchestrator").
		PathParam("taskId", "Task identifier", StringSchema()).
		ResponseJSON("200", "Context records", "ContextListResponse")
	b.Route("POST", "/api/task/:taskId/pause", "Pause a running task").
		Tags("Orchestrator").
		PathParam("taskId", "Task identifier", StringSchema()).
		BodyInlineJSON(&Schema{
			Type: "object",
			Properties: map[string]*SchemaRef{
				"reason": {Schema: StringSchema()},
			},
		}, "Pause reason", false).
		ResponseJSON("200", "Paused", "TaskLifecycleResponse")
	b.Route("POST", "/api/task/:taskId/fail", "Immediately fail a task").
		Tags("Orchestrator").
		PathParam("taskId", "Task identifier", StringSchema()).
		ResponseJSON("200", "Failed", "TaskLifecycleResponse")
	b.Route("POST", "/api/task/:taskId/resume", "Resume a paused task").
		Tags("Orchestrator").
		PathParam("taskId", "Task identifier", StringSchema()).
		ResponseJSON("200", "Resumed", "TaskLifecycleResponse")
	b.Route("GET", "/api/task/:taskId/pause-reason", "Get task pause reason").
		Tags("Orchestrator").
		PathParam("taskId", "Task identifier", StringSchema()).
		ResponseJSON("200", "Pause reason", "PauseReasonResponse")

	// ── Node ──
	b.Route("POST", "/api/node/:nodeId/success", "Report node execution success").
		Tags("Node").
		PathParam("nodeId", "Node identifier", StringSchema()).
		BodyInlineJSON(ObjectSchema(), "Node output data", false).
		ResponseJSON("200", "Recorded", "NodeSuccessResponse")
	b.Route("POST", "/api/node/:nodeId/failure", "Report node execution failure").
		Tags("Node").
		PathParam("nodeId", "Node identifier", StringSchema()).
		BodyInlineJSON(&Schema{
			Type: "object",
			Properties: map[string]*SchemaRef{
				"errorMessage": {Schema: StringSchema()},
			},
		}, "Error details", true).
		ResponseJSON("200", "Recorded", "NodeSuccessResponse")
	b.Route("GET", "/api/node/:nodeId/snapshot/latest", "Get latest node snapshot").
		Tags("Node").
		PathParam("nodeId", "Node identifier", StringSchema()).
		ResponseJSON("200", "Snapshot found", "NodeSnapshotResponse").
		ResponseJSON("404", "No snapshot", "ErrorResponse")
	b.Route("POST", "/api/node/:nodeId/restore", "Restore node from snapshot").
		Tags("Node").
		PathParam("nodeId", "Node identifier", StringSchema()).
		ResponseJSON("200", "Restored", "NodeSnapshotResponse")
	b.Route("POST", "/api/node/:nodeId/retry", "Retry a failed node").
		Tags("Node").
		PathParam("nodeId", "Node identifier", StringSchema()).
		ResponseJSON("200", "Retry initiated", "NodeRetryResponse")
	b.Route("POST", "/api/node", "Submit DAG in one step (create + submit)").
		Tags("Node").
		BodyJSON("DAGRequest", "DAG nodes + edges", true).
		ResponseJSON("200", "DAG submitted", "DAGSubmitResponse")

	// ── Local Runners ──
	b.Route("POST", "/api/local-runners/register", "Register a local execution runner and create a runner session").
		Tags("Local Runners").
		BodyJSON("RegisterRunnerRequest", "Local runner device and capability probe", true).
		ResponseJSON("200", "Runner session", "RegisterRunnerResponse").
		ResponseJSON("400", "Invalid request", "ErrorResponse").
		ResponseJSON("401", "Missing or invalid access token", "ErrorResponse")
	b.Route("POST", "/api/local-runners/:runnerId/heartbeat", "Update local runner heartbeat and resource snapshot").
		Tags("Local Runners").
		PathParam("runnerId", "Runner identifier", StringSchema()).
		BodyJSON("HeartbeatRequest", "Runner heartbeat", true).
		ResponseJSON("200", "Heartbeat accepted", "LocalOKResponse").
		ResponseJSON("400", "Invalid request", "ErrorResponse").
		ResponseJSON("401", "Missing or invalid access token", "ErrorResponse")
	b.Route("GET", "/api/local-runners/:runnerId/jobs/claim", "Claim the next pending local job for a runner").
		Tags("Local Runners").
		PathParam("runnerId", "Runner identifier", StringSchema()).
		ResponseJSON("200", "Claimed job or null", "ClaimJobResponse").
		ResponseJSON("401", "Missing or invalid access token", "ErrorResponse")
	b.Route("POST", "/api/local-jobs/:jobId/progress", "Report local job progress and logs").
		Tags("Local Runners").
		PathParam("jobId", "Local job identifier", StringSchema()).
		BodyJSON("ProgressRequest", "Progress update", true).
		ResponseJSON("200", "Progress accepted", "LocalOKResponse").
		ResponseJSON("400", "Invalid request", "ErrorResponse").
		ResponseJSON("401", "Missing or invalid access token", "ErrorResponse")
	b.Route("POST", "/api/local-jobs/:jobId/complete", "Complete a local job and advance its DAG node").
		Tags("Local Runners").
		PathParam("jobId", "Local job identifier", StringSchema()).
		BodyJSON("CompleteJobRequest", "Local job output", true).
		ResponseJSON("200", "Completion accepted", "LocalOKResponse").
		ResponseJSON("400", "Invalid request", "ErrorResponse").
		ResponseJSON("401", "Missing or invalid access token", "ErrorResponse")
	b.Route("POST", "/api/local-jobs/:jobId/fail", "Fail a local job and advance its DAG node failure path").
		Tags("Local Runners").
		PathParam("jobId", "Local job identifier", StringSchema()).
		BodyJSON("FailJobRequest", "Local job failure details", true).
		ResponseJSON("200", "Failure accepted", "LocalOKResponse").
		ResponseJSON("400", "Invalid request", "ErrorResponse").
		ResponseJSON("401", "Missing or invalid access token", "ErrorResponse")

	// ── Translate ──
	b.Route("POST", "/api/translate", "Translate NL prompt to DAG").
		Tags("Translate").
		BodyInlineJSON(&Schema{
			Type: "object",
			Properties: map[string]*SchemaRef{
				"prompt": {Schema: StringSchema()},
			},
			Required: []string{"prompt"},
		}, "Natural language description", true).
		ResponseJSON("200", "DAG structure", "DAGResponse")
	b.Route("POST", "/api/translate/submit", "Translate NL prompt and submit DAG").
		Tags("Translate").
		BodyInlineJSON(&Schema{
			Type: "object",
			Properties: map[string]*SchemaRef{
				"prompt": {Schema: StringSchema()},
			},
			Required: []string{"prompt"},
		}, "Natural language description", true).
		ResponseJSON("200", "Result", "TranslateSubmitResponse")
	b.Route("GET", "/api/task/:taskId/status", "Query translate task status").
		Tags("Translate").
		PathParam("taskId", "Task identifier", StringSchema()).
		ResponseJSON("200", "Task status", "TaskStatusResponse")

	// ── Context ──
	b.Route("GET", "/api/context/:taskId", "Get context records for task").
		Tags("Context").
		PathParam("taskId", "Task identifier", StringSchema()).
		ResponseJSON("200", "Context records", "ContextListResponse")
	b.Route("GET", "/api/context/:taskId/node/:nodeId/snapshot/latest", "Get node snapshot").
		Tags("Context").
		PathParam("taskId", "Task identifier", StringSchema()).
		PathParam("nodeId", "Node identifier", StringSchema()).
		ResponseJSON("200", "Snapshot found", "NodeSnapshotResponse")
	b.Route("POST", "/api/context/:taskId/node/:nodeId/restore", "Restore node").
		Tags("Context").
		PathParam("taskId", "Task identifier", StringSchema()).
		PathParam("nodeId", "Node identifier", StringSchema()).
		ResponseJSON("200", "Restored", "NodeSnapshotResponse")
	b.Route("POST", "/api/context/record", "Record a context event manually").
		Tags("Context").
		BodyInlineJSON(&Schema{
			Type: "object",
			Properties: map[string]*SchemaRef{
				"taskId":  {Schema: StringSchema()},
				"nodeId":  {Schema: StringSchema()},
				"type":    {Schema: StringSchema()},
				"message": {Schema: StringSchema()},
			},
			Required: []string{"taskId", "type", "message"},
		}, "Context event", true).
		ResponseJSON("200", "Recorded", "GenericOKResponse")

	// ── Media ──
	b.Route("POST", "/api/media/upload", "Upload media files").
		Tags("Media").
		BodyMultipart(map[string]*Schema{
			"userId": StringSchema(),
		}, false).
		ResponseJSON("200", "Uploaded", "MediaListResponse").
		ResponseJSON("400", "No files", "ErrorResponse")
	b.Route("GET", "/api/media/list", "List media assets").
		Tags("Media").
		QueryParam("userId", "User identifier", StringSchema(), false).
		QueryParam("offset", "Pagination offset", IntegerSchema(), false).
		QueryParam("limit", "Page size", IntegerSchema(), false).
		QueryParam("tag", "Filter by tag", StringSchema(), false).
		ResponseJSON("200", "Media list", "MediaListResponse")
	b.Route("GET", "/api/media/:id", "Get media asset by ID").
		Tags("Media").
		PathParam("id", "Media identifier", StringSchema()).
		ResponseJSON("200", "Media asset", "MediaAssetResponse").
		ResponseJSON("404", "Not found", "ErrorResponse")
	b.Route("PUT", "/api/media/:id/tags", "Update media tags").
		Tags("Media").
		PathParam("id", "Media identifier", StringSchema()).
		BodyInlineJSON(&Schema{
			Type: "object",
			Properties: map[string]*SchemaRef{
				"tags": {Schema: ArraySchema(StringSchema())},
			},
			Required: []string{"tags"},
		}, "New tag array", true).
		ResponseJSON("200", "Tags updated", "GenericOKResponse")

	// ── Tools ──
	b.Route("GET", "/api/tools", "List all registered tools").
		Tags("Tools").
		ResponseJSON("200", "Tool manifests", "ToolListResponse")
	b.Route("GET", "/api/tools/:name", "Get tool details").
		Tags("Tools").
		PathParam("name", "Tool name", StringSchema()).
		ResponseJSON("200", "Tool manifest", "ToolDetailResponse").
		ResponseJSON("404", "Tool not found", "ErrorResponse")
	b.Route("POST", "/api/tools/register", "Register an external tool").
		Tags("Tools").
		BodyInlineJSON(ObjectSchema(), "Tool manifest", true).
		ResponseJSON("200", "Registered", "ToolRegisterResponse")
	b.Route("DELETE", "/api/tools/:name", "Deregister an external tool").
		Tags("Tools").
		PathParam("name", "Tool name", StringSchema()).
		ResponseJSON("200", "Deregistered", "GenericOKResponse").
		ResponseJSON("404", "Not found", "ErrorResponse")

	// ── Skills ──
	b.Route("GET", "/api/skills", "List all loaded skills").
		Tags("Skills").
		ResponseJSON("200", "Skills list", "SkillsResponse")
	b.Route("GET", "/api/skills/catalog", "Get skill catalog").
		Tags("Skills").
		QueryParam("includeHidden", "Include hidden skills", BoolSchema(), false).
		ResponseJSON("200", "Catalog", "SkillCatalogResponse")
	b.Route("POST", "/api/skills/route", "Route a brief to a skill").
		Tags("Skills").
		BodyInlineJSON(&Schema{
			Type: "object",
			Properties: map[string]*SchemaRef{
				"brief": {Schema: StringSchema()},
			},
			Required: []string{"brief"},
		}, "Brief description", true).
		ResponseJSON("200", "Route result", "SkillRouteResponse")
	b.Route("GET", "/api/skills/:name/:version", "Get skill detail").
		Tags("Skills").
		PathParam("name", "Skill name", StringSchema()).
		PathParam("version", "Skill version", StringSchema()).
		ResponseJSON("200", "Skill detail", "SkillDetailResponse").
		ResponseJSON("404", "Not found", "ErrorResponse")
	b.Route("POST", "/api/skills/:name/:version/compile", "Compile skill to DAG").
		Tags("Skills").
		PathParam("name", "Skill name", StringSchema()).
		PathParam("version", "Skill version", StringSchema()).
		ResponseJSON("200", "Compiled DAG", "DAGResponse")

	// ── Skill Capabilities ──
	b.Route("GET", "/api/skill-capabilities", "List agent capability packages").
		Tags("Skill Capabilities").
		ResponseJSON("200", "Capability packages", "SkillCapabilityListResponse")
	b.Route("GET", "/api/skill-capabilities/:id", "Get capability package detail").
		Tags("Skill Capabilities").
		PathParam("id", "Capability package identifier", StringSchema()).
		ResponseJSON("200", "Capability package", "SkillCapabilityDetailResponse").
		ResponseJSON("404", "Not found", "ErrorResponse")

	// ── Video Role Agents ──
	b.Route("GET", "/api/video/role-agents", "List Guided Video Studio role agents").
		Tags("Video Role Agents").
		ResponseJSON("200", "Role agents", "VideoRoleAgentListResponse")
	b.Route("GET", "/api/video/role-agents/:roleId", "Get Guided Video Studio role agent detail").
		Tags("Video Role Agents").
		PathParam("roleId", "Role agent identifier", StringSchema()).
		ResponseJSON("200", "Role agent", "VideoRoleAgentDetailResponse").
		ResponseJSON("404", "Not found", "ErrorResponse")

	// ── Dynamic Agent Runs ──
	b.Route("POST", "/api/agent/runs", "Start a dynamic agent run from natural language").
		Tags("Agent Runs").
		BodyJSON("AgentStartRunRequest", "Agent run request", true).
		ResponseJSON("200", "Agent run started", "AgentRunStartResponse").
		ResponseJSON("400", "Invalid plan or request", "ErrorResponse")
	b.Route("GET", "/api/agent/runs/:runId", "Get dynamic agent run state").
		Tags("Agent Runs").
		PathParam("runId", "Agent run identifier", StringSchema()).
		ResponseJSON("200", "Agent run detail", "AgentRunDetailResponse").
		ResponseJSON("404", "Not found", "ErrorResponse")
	b.Route("GET", "/api/agent/runs/:runId/trace", "Get dynamic agent task trace").
		Tags("Agent Runs").
		PathParam("runId", "Agent run identifier", StringSchema()).
		ResponseJSON("200", "Agent task trace", "AgentRunTraceResponse").
		ResponseJSON("404", "Not found", "ErrorResponse")
	b.Route("GET", "/api/agent/runs/:runId/reviews", "List review gates for a dynamic agent run").
		Tags("Agent Runs").
		PathParam("runId", "Agent run identifier", StringSchema()).
		ResponseJSON("200", "Review gates", "AgentReviewListResponse").
		ResponseJSON("404", "Not found", "ErrorResponse")
	b.Route("POST", "/api/agent/runs/:runId/reviews/:reviewId/approve", "Approve a dynamic agent review gate").
		Tags("Agent Runs").
		PathParam("runId", "Agent run identifier", StringSchema()).
		PathParam("reviewId", "Review node identifier", StringSchema()).
		BodyInlineJSON(ObjectSchema(), "Optional review comment", false).
		ResponseJSON("200", "Approved", "AgentReviewDecisionResponse")
	b.Route("POST", "/api/agent/runs/:runId/reviews/:reviewId/reject", "Reject a dynamic agent review gate").
		Tags("Agent Runs").
		PathParam("runId", "Agent run identifier", StringSchema()).
		PathParam("reviewId", "Review node identifier", StringSchema()).
		BodyInlineJSON(ObjectSchema(), "Optional rejection comment", false).
		ResponseJSON("200", "Rejected", "AgentReviewDecisionResponse")

	// ── Workflows ──
	b.Route("GET", "/api/workflows", "List workflow templates").
		Tags("Workflows").
		ResponseJSON("200", "Templates", "WorkflowListResponse")
	b.Route("GET", "/api/workflows/:id", "Get template detail").
		Tags("Workflows").
		PathParam("id", "Template identifier", StringSchema()).
		ResponseJSON("200", "Template", "WorkflowTemplateResponse").
		ResponseJSON("404", "Not found", "ErrorResponse")
	b.Route("POST", "/api/workflows", "Create a workflow template").
		Tags("Workflows").
		BodyInlineJSON(ObjectSchema(), "Template definition", true).
		ResponseJSON("200", "Created", "WorkflowTemplateResponse")
	b.Route("PUT", "/api/workflows/:id", "Update a template").
		Tags("Workflows").
		PathParam("id", "Template identifier", StringSchema()).
		BodyInlineJSON(ObjectSchema(), "Updated fields", true).
		ResponseJSON("200", "Updated", "WorkflowTemplateResponse")
	b.Route("DELETE", "/api/workflows/:id", "Delete a template").
		Tags("Workflows").
		PathParam("id", "Template identifier", StringSchema()).
		ResponseJSON("200", "Deleted", "GenericOKResponse")
	b.Route("POST", "/api/workflows/:id/instantiate", "Instantiate template → task").
		Tags("Workflows").
		PathParam("id", "Template identifier", StringSchema()).
		BodyInlineJSON(ObjectSchema(), "Input overrides", false).
		ResponseJSON("200", "Task created", "InstantiateResponse")

	// ── Video Projects ──
	b.Route("GET", "/api/video-projects", "List video projects").
		Tags("Video Projects").
		ResponseJSON("200", "Projects list", "VideoProjectListResponse")
	b.Route("POST", "/api/video-projects", "Create a video project").
		Tags("Video Projects").
		BodyInlineJSON(ObjectSchema(), "Project definition", true).
		ResponseJSON("200", "Created", "VideoProjectCreateResponse")
	b.Route("GET", "/api/video-projects/:id", "Get project detail").
		Tags("Video Projects").
		PathParam("id", "Project identifier", StringSchema()).
		ResponseJSON("200", "Project", "VideoProjectDetailResponse").
		ResponseJSON("404", "Not found", "ErrorResponse")
	b.Route("PATCH", "/api/video-projects/:id", "Update a project").
		Tags("Video Projects").
		PathParam("id", "Project identifier", StringSchema()).
		BodyInlineJSON(ObjectSchema(), "Updated fields", true).
		ResponseJSON("200", "Updated", "VideoProjectDetailResponse")
	b.Route("DELETE", "/api/video-projects/:id", "Archive a project (soft delete)").
		Tags("Video Projects").
		PathParam("id", "Project identifier", StringSchema()).
		ResponseJSON("200", "Archived", "GenericOKResponse")
	b.Route("POST", "/api/video-projects/:id/assistant/message", "Ask the video-project-scoped assistant").
		Tags("Video Project Assistant").
		PathParam("id", "Project identifier", StringSchema()).
		BodyJSON("VideoAssistantMessageRequest", "Project-scoped assistant question", true).
		ResponseJSON("200", "Assistant answer", "VideoAssistantMessageResponse").
		ResponseJSON("400", "Invalid request", "ErrorResponse").
		ResponseJSON("401", "Missing or invalid access token", "ErrorResponse")
	b.Route("POST", "/api/video-projects/:id/assistant/revise", "Convert assistant feedback into an Artifact revision action").
		Tags("Video Project Assistant").
		PathParam("id", "Project identifier", StringSchema()).
		BodyJSON("VideoAssistantReviseRequest", "Artifact revision instruction", true).
		ResponseJSON("200", "Revision action", "VideoAssistantReviseResponse").
		ResponseJSON("400", "Invalid request", "ErrorResponse").
		ResponseJSON("401", "Missing or invalid access token", "ErrorResponse")
	b.Route("POST", "/api/video-projects/:id/assistant/explain-stage", "Explain a video workflow stage").
		Tags("Video Project Assistant").
		PathParam("id", "Project identifier", StringSchema()).
		BodyJSON("VideoAssistantExplainStageRequest", "Stage to explain", true).
		ResponseJSON("200", "Stage explanation", "VideoAssistantExplainStageResponse").
		ResponseJSON("400", "Invalid request", "ErrorResponse").
		ResponseJSON("401", "Missing or invalid access token", "ErrorResponse")

	// ── Workflow Runs ──
	b.Route("POST", "/api/video-projects/:id/workflow-runs", "Create a workflow run").
		Tags("Workflow Runs").
		PathParam("id", "Project identifier", StringSchema()).
		BodyInlineJSON(&Schema{
			Type: "object",
			Properties: map[string]*SchemaRef{
				"templateId":      {Schema: StringSchema()},
				"templateVersion": {Schema: StringSchema()},
				"input":           {Schema: ObjectSchema()},
			},
			Required: []string{"templateId"},
		}, "Run configuration", true).
		ResponseJSON("200", "Run created", "WorkflowRunCreateResponse")
	b.Route("GET", "/api/video-projects/:id/workflow-runs/:rid", "Get run detail").
		Tags("Workflow Runs").
		PathParam("id", "Project identifier", StringSchema()).
		PathParam("rid", "Run identifier", StringSchema()).
		ResponseJSON("200", "Run detail", "WorkflowRunDetailResponse").
		ResponseJSON("404", "Not found", "ErrorResponse")
	b.Route("POST", "/api/video-projects/:id/workflow-runs/:rid/pause", "Pause a run").
		Tags("Workflow Runs").
		PathParam("id", "Project identifier", StringSchema()).
		PathParam("rid", "Run identifier", StringSchema()).
		ResponseJSON("200", "Paused", "GenericOKResponse")
	b.Route("POST", "/api/video-projects/:id/workflow-runs/:rid/cancel", "Cancel a run").
		Tags("Workflow Runs").
		PathParam("id", "Project identifier", StringSchema()).
		PathParam("rid", "Run identifier", StringSchema()).
		ResponseJSON("200", "Cancelled", "GenericOKResponse")

	// ── Stages ──
	b.Route("POST", "/api/video-projects/:id/stages/:stage/approve", "Approve a stage").
		Tags("Stages").
		PathParam("id", "Project identifier", StringSchema()).
		PathParam("stage", "Stage name", StringSchema()).
		BodyInlineJSON(&Schema{
			Type: "object",
			Properties: map[string]*SchemaRef{
				"runId":   {Schema: StringSchema()},
				"output":  {Schema: ObjectSchema()},
				"comment": {Schema: StringSchema()},
			},
		}, "Approval data", false).
		ResponseJSON("200", "Approved", "StageApprovalResponse").
		ResponseJSON("409", "Stage not ready", "ErrorResponse")

	// ── Artifacts ──
	b.Route("GET", "/api/video-projects/:id/artifacts", "List project artifacts").
		Tags("Artifacts").
		PathParam("id", "Project identifier", StringSchema()).
		ResponseJSON("200", "Artifacts list", "ArtifactListResponse")
	b.Route("GET", "/api/artifacts/:id", "Get artifact by ID").
		Tags("Artifacts").
		PathParam("id", "Artifact identifier", StringSchema()).
		ResponseJSON("200", "Artifact", "ArtifactDetailResponse").
		ResponseJSON("404", "Not found", "ErrorResponse")
	b.Route("GET", "/api/artifacts/:id/content", "Get artifact content").
		Tags("Artifacts").
		PathParam("id", "Artifact identifier", StringSchema()).
		ResponseJSON("200", "Content + metadata", "ArtifactContentResponse").
		ResponseJSON("404", "Not found", "ErrorResponse")
	b.Route("GET", "/api/artifacts/:id/history", "Get artifact revision history").
		Tags("Artifacts").
		PathParam("id", "Artifact identifier", StringSchema()).
		ResponseJSON("200", "Version history", "ArtifactHistoryResponse")
	b.Route("POST", "/api/artifacts/:id/revise", "Create an artifact revision").
		Tags("Artifacts").
		PathParam("id", "Artifact identifier", StringSchema()).
		BodyInlineJSON(&Schema{
			Type: "object",
			Properties: map[string]*SchemaRef{
				"message": {Schema: StringSchema()},
			},
			Required: []string{"message"},
		}, "Revision instruction", true).
		ResponseJSON("200", "Revised", "ArtifactContentResponse")

	// ── Register automatic schemas (derived from real Go types) ──
	registerCloudSchemas(b)

	return b.Build()
}
