// GENERATED from OpenAPI spec — do not edit.
// Regenerate: cd cloud-backend && make gen-ts
// Source of truth: internal/core/apispec/cloud_spec.go
//
// This file contains the response/request body types derived from
// the OpenAPI components/schemas. The generic ApiResponse<T> envelope
// lives in types.ts alongside frontend-only UI types.

/**  */
// AIGenerateResponse
export interface AIGenerateResponse {
  code?: number;
  data?: { description?: string; title?: string };
  message?: string;
}

/**  */
// AIPolishResponse
export interface AIPolishResponse {
  code?: number;
  data?: { content?: string; taskId?: string; traceUrl?: string };
  message?: string;
}

/**  */
// AgentReviewDecisionResponse
export interface AgentReviewDecisionResponse {
  code?: number;
  data?: { reviewId?: string; status?: string };
  message?: string;
}

/**  */
// AgentReviewListResponse
export interface AgentReviewListResponse {
  code?: number;
  data?: { reviews?: { blocksDownstream?: boolean; id: string; nodeId: string; reviewArtifactKinds?: string[]; reviewPhase?: string; reviewReason?: string; status: string; stepId?: string; tool?: string }[]; runId?: string };
  message?: string;
}

/**  */
// AgentRunDetailResponse
export interface AgentRunDetailResponse {
  code?: number;
  data?: { run?: { budget?: { maxCostLevel?: string; maxLLMCalls?: number; maxReplans?: number; maxSteps?: number; maxToolCalls?: number }; createdAt: string; domain?: string; id: string; message: string; metadata?: Record<string, Record<string, unknown>>; plan?: { budget?: { maxCostLevel?: string; maxLLMCalls?: number; maxReplans?: number; maxSteps?: number; maxToolCalls?: number }; domain?: string; goal?: string; mode?: string; steps?: { arguments: Record<string, Record<string, unknown>>; dependsOn?: string[]; expectedOutput?: string[]; id: string; intent?: string; produceArtifact?: boolean; tool: string }[]; stopPolicy?: { stopWhenEnough?: boolean } } | null; status: string; taskId?: string; updatedAt: string; userId?: string }; task?: Record<string, unknown> };
  message?: string;
}

/**  */
// AgentRunStartResponse
export interface AgentRunStartResponse {
  code?: number;
  data?: { plan?: { budget: { maxCostLevel?: string; maxLLMCalls?: number; maxReplans?: number; maxSteps?: number; maxToolCalls?: number }; domain: string; goal: string; mode: string; steps: { arguments: Record<string, Record<string, unknown>>; dependsOn?: string[]; expectedOutput?: string[]; id: string; intent?: string; produceArtifact?: boolean; tool: string }[]; stopPolicy: { stopWhenEnough?: boolean } }; runId?: string; status?: string; taskId?: string };
  message?: string;
}

/**  */
// AgentStartRunRequest
export interface AgentStartRunRequest {
  context?: Record<string, Record<string, unknown>>;
  domain?: string;
  message: string;
  mode?: string;
  userId?: string;
}

/**  */
// Artifact
export interface Artifact {
  contentHash: string;
  createdAt: string;
  id: string;
  inlineJson?: string;
  isCurrent: boolean;
  kind: string;
  metadata?: Record<string, Record<string, unknown>>;
  mimeType?: string;
  model?: string;
  name: string;
  parentId?: string;
  projectId: string;
  promptHash?: string;
  provider?: string;
  sizeBytes: number;
  stageName: string;
  storageRef?: string;
  storageType: string;
  unitId?: string;
  version: number;
  workflowRunId?: string;
}

/**  */
// ArtifactContentResponse
export interface ArtifactContentResponse {
  code?: number;
  data?: { artifact?: { contentHash: string; createdAt: string; id: string; inlineJson?: string; isCurrent: boolean; kind: string; metadata?: Record<string, Record<string, unknown>>; mimeType?: string; model?: string; name: string; parentId?: string; projectId: string; promptHash?: string; provider?: string; sizeBytes: number; stageName: string; storageRef?: string; storageType: string; unitId?: string; version: number; workflowRunId?: string }; content?: Record<string, unknown>; mediaUrl?: string; mediaUrls?: string[] };
  message?: string;
}

/**  */
// ArtifactDetailResponse
export interface ArtifactDetailResponse {
  code?: number;
  data?: { artifact?: { contentHash: string; createdAt: string; id: string; inlineJson?: string; isCurrent: boolean; kind: string; metadata?: Record<string, Record<string, unknown>>; mimeType?: string; model?: string; name: string; parentId?: string; projectId: string; promptHash?: string; provider?: string; sizeBytes: number; stageName: string; storageRef?: string; storageType: string; unitId?: string; version: number; workflowRunId?: string } };
  message?: string;
}

/**  */
// ArtifactHistoryResponse
export interface ArtifactHistoryResponse {
  code?: number;
  data?: { history?: { contentHash: string; createdAt: string; id: string; inlineJson?: string; isCurrent: boolean; kind: string; metadata?: Record<string, Record<string, unknown>>; mimeType?: string; model?: string; name: string; parentId?: string; projectId: string; promptHash?: string; provider?: string; sizeBytes: number; stageName: string; storageRef?: string; storageType: string; unitId?: string; version: number; workflowRunId?: string }[] };
  message?: string;
}

/**  */
// ArtifactListResponse
export interface ArtifactListResponse {
  code?: number;
  data?: { artifacts?: { contentHash: string; createdAt: string; id: string; inlineJson?: string; isCurrent: boolean; kind: string; metadata?: Record<string, Record<string, unknown>>; mimeType?: string; model?: string; name: string; parentId?: string; projectId: string; promptHash?: string; provider?: string; sizeBytes: number; stageName: string; storageRef?: string; storageType: string; unitId?: string; version: number; workflowRunId?: string }[] };
  message?: string;
}

/**  */
// AuthLoginRequest
export interface AuthLoginRequest {
  email: string;
  password: string;
}

/**  */
// AuthLogoutRequest
export interface AuthLogoutRequest {
  refresh_token: string;
}

/**  */
// AuthRefreshRequest
export interface AuthRefreshRequest {
  refresh_token: string;
}

/**  */
// AuthRegisterRequest
export interface AuthRegisterRequest {
  email: string;
  nickname?: string;
  password: string;
}

/**  */
// AuthResponse
export interface AuthResponse {
  code?: number;
  data?: { access_token: string; expires_in: number; refresh_token: string; user: { avatarUrl?: string; createdAt: string; email: string; id: string; lastLoginAt?: string | null; nickname?: string; status: string; updatedAt: string } };
  message?: string;
}

/**  */
// BidChapter
export interface BidChapter {
  content?: string;
  createdAt: string;
  id: string;
  nodeId?: string;
  projectId: string;
  reviewComment?: string;
  scoreItems?: string;
  sortOrder: number;
  status: string;
  title: string;
  updatedAt: string;
}

/**  */
// BidChapterResponse
export interface BidChapterResponse {
  code?: number;
  data?: { chapter?: { content?: string; createdAt: string; id: string; nodeId?: string; projectId: string; reviewComment?: string; scoreItems?: string; sortOrder: number; status: string; title: string; updatedAt: string } };
  message?: string;
}

/**  */
// BidExportResponse
export interface BidExportResponse {
  code?: number;
  data?: { format?: string; message?: string };
  message?: string;
}

/**  */
// BidExportStatusResponse
export interface BidExportStatusResponse {
  code?: number;
  data?: { status?: Record<string, unknown> };
  message?: string;
}

/**  */
// BidProgressResponse
export interface BidProgressResponse {
  code?: number;
  data?: { progress?: Record<string, unknown> };
  message?: string;
}

/**  */
// BidProject
export interface BidProject {
  config?: string;
  createdAt: string;
  id: string;
  industry?: string;
  name: string;
  progress: number;
  status: string;
  structure?: string;
  taskId?: string;
  templateId?: string;
  tenderAnalysis?: string;
  tenderFileName?: string;
  tenderFilePath?: string;
  updatedAt: string;
  userId?: string;
}

/**  */
// BidProjectCreateResponse
export interface BidProjectCreateResponse {
  code?: number;
  data?: { project?: { config?: string; createdAt: string; id: string; industry?: string; name: string; progress: number; status: string; structure?: string; taskId?: string; templateId?: string; tenderAnalysis?: string; tenderFileName?: string; tenderFilePath?: string; updatedAt: string; userId?: string } };
  message?: string;
}

/**  */
// BidProjectDetailResponse
export interface BidProjectDetailResponse {
  code?: number;
  data?: { chapters?: Record<string, unknown>[]; project?: { config?: string; createdAt: string; id: string; industry?: string; name: string; progress: number; status: string; structure?: string; taskId?: string; templateId?: string; tenderAnalysis?: string; tenderFileName?: string; tenderFilePath?: string; updatedAt: string; userId?: string } };
  message?: string;
}

/**  */
// BidProjectListResponse
export interface BidProjectListResponse {
  code?: number;
  data?: { items?: { config?: string; createdAt: string; id: string; industry?: string; name: string; progress: number; status: string; structure?: string; taskId?: string; templateId?: string; tenderAnalysis?: string; tenderFileName?: string; tenderFilePath?: string; updatedAt: string; userId?: string }[]; total?: number };
  message?: string;
}

/**  */
// BidRegenResponse
export interface BidRegenResponse {
  code?: number;
  data?: { chapter_id?: string; message?: string };
  message?: string;
}

/**  */
// BidStartResponse
export interface BidStartResponse {
  code?: number;
  data?: { message?: string; task_id?: string };
  message?: string;
}

/**  */
// BidTemplatesResponse
export interface BidTemplatesResponse {
  code?: number;
  data?: { templates?: Record<string, unknown>[] };
  message?: string;
}

/**  */
// BidTraceResponse
export interface BidTraceResponse {
  code?: number;
  data?: { project_id?: string; task_id?: string; trace_url?: string };
  message?: string;
}

/**  */
// BidUploadResponse
export interface BidUploadResponse {
  code?: number;
  data?: { file_name?: string; file_path?: string; file_size?: number };
  message?: string;
}

/**  */
// ChatResponse
export interface ChatResponse {
  code?: number;
  data?: { fields?: Record<string, unknown>; reply?: string };
  message?: string;
}

/**  */
// ClaimJobResponse
export interface ClaimJobResponse {
  job?: { artifactPolicy?: { location: string; syncFileToCloud: boolean; syncMetadataToCloud: boolean }; attempt?: number; command?: string; createdAt?: string; currentStep?: string; diagnostics?: Record<string, Record<string, unknown>>; error?: Record<string, Record<string, unknown>>; errorMessage?: string; idempotencyKey?: string; jobId?: string; leaseExpiresAt?: string | null; message?: string; nodeId?: string; output?: Record<string, Record<string, unknown>>; payload?: Record<string, Record<string, unknown>>; progress?: number; projectId?: string; retryable?: boolean; runnerId?: string; status?: string; taskId?: string; timeoutSec?: number; toolName?: string; updatedAt?: string } | null;
}

/**  */
// CompleteJobRequest
export interface CompleteJobRequest {
  output: Record<string, Record<string, unknown>>;
  success: boolean;
}

/**  */
// ContextListResponse
export interface ContextListResponse {
  code?: number;
  data?: Record<string, unknown>[];
  message?: string;
}

/**  */
// CreateSessionResponse
export interface CreateSessionResponse {
  code?: number;
  data?: { session_id?: string };
  message?: string;
}

/**  */
// CurrentUserResponse
export interface CurrentUserResponse {
  code?: number;
  data?: { avatarUrl?: string; createdAt: string; email: string; id: string; lastLoginAt?: string | null; nickname?: string; status: string; updatedAt: string };
  message?: string;
}

/**  */
// DAGRequest
export interface DAGRequest {
  edges: { from: string; to: string }[];
  nodes: { condition?: string; heartbeatTimeoutSec?: number | null; id: string; input?: Record<string, Record<string, unknown>>; longRunning?: boolean; maxRetry?: number | null; name: string; priority?: number | null; type: string; workerGroup?: string }[];
}

/**  */
// DAGResponse
export interface DAGResponse {
  code?: number;
  data?: { edges?: Record<string, unknown>[]; nodes?: Record<string, unknown>[] };
  message?: string;
}

/**  */
// DAGSubmitResponse
export interface DAGSubmitResponse {
  code?: number;
  data?: { message?: string; taskId?: string };
  message?: string;
}

/**  */
// ErrorResponse
export interface ErrorResponse {
  code?: number;
  data?: unknown;
  message?: string;
}

/**  */
// FailJobRequest
export interface FailJobRequest {
  diagnostics?: Record<string, Record<string, unknown>>;
  error: Record<string, Record<string, unknown>>;
  retryable: boolean;
  success: boolean;
}

/**  */
// GenericOKResponse
export interface GenericOKResponse {
  code?: number;
  message?: string;
}

/**  */
// HealthResponse
export interface HealthResponse {
  service?: string;
  status?: string;
}

/**  */
// HeartbeatRequest
export interface HeartbeatRequest {
  cpuLoad: number;
  diskFreeMb: number;
  lastError?: string | null;
  memoryUsageMb: number;
  runningJobs: number;
  sessionId: string;
  status: string;
}

/**  */
// InstantiateResponse
export interface InstantiateResponse {
  code?: number;
  data?: { message?: string; task_id?: string; trace_url?: string };
  message?: string;
}

/**  */
// LocalOKResponse
export interface LocalOKResponse {
  ok?: boolean;
}

/**  */
// MediaAssetResponse
export interface MediaAssetResponse {
  code?: number;
  data?: { createdAt?: string; id?: string; mimeType?: string; minioPath?: string; originalName?: string; size?: number; tags?: string[]; updatedAt?: string; userId?: string };
  message?: string;
}

/**  */
// MediaListResponse
export interface MediaListResponse {
  code?: number;
  data?: { items?: Record<string, unknown>[]; total?: number };
  message?: string;
}

/**  */
// NodeRetryResponse
export interface NodeRetryResponse {
  code?: number;
  data?: { message?: string; nodeId?: string };
  message?: string;
}

/**  */
// NodeSnapshotResponse
export interface NodeSnapshotResponse {
  code?: number;
  data?: { nodeId?: string; snapshot?: Record<string, unknown> };
  message?: string;
}

/**  */
// NodeSuccessResponse
export interface NodeSuccessResponse {
  code?: number;
  data?: { message?: string };
  message?: string;
}

/**  */
// PauseReasonResponse
export interface PauseReasonResponse {
  code?: number;
  data?: { reason?: string; taskId?: string };
  message?: string;
}

/**  */
// PolishQueryResponse
export interface PolishQueryResponse {
  code?: number;
  data?: { content?: string; nodeId?: string; status?: string; taskId?: string; traceUrl?: string };
  message?: string;
}

/**  */
// PolishSubmitResponse
export interface PolishSubmitResponse {
  code?: number;
  data?: { message?: string; nodeId?: string; taskId?: string; traceUrl?: string };
  message?: string;
}

/**  */
// ProgressRequest
export interface ProgressRequest {
  logs?: string[];
  message: string;
  progress: number;
  status: string;
  step: string;
}

/**  */
// ProgressResponse
export interface ProgressResponse {
  code?: number;
  data?: { status?: string; task_id?: string };
  message?: string;
}

/**  */
// PublishResponse
export interface PublishResponse {
  code?: number;
  data?: { message?: string; taskId?: string };
  message?: string;
}

/**  */
// ReadinessResponse
export interface ReadinessResponse {
  dependencies?: Record<string, unknown>;
  service?: string;
  status?: string;
}

/**  */
// RegisterRunnerRequest
export interface RegisterRunnerRequest {
  capabilities: { available: boolean; command: string; toolName: string; version?: string }[];
  deviceId: string;
  platform: { arch?: string; hostname?: string; os?: string };
  runnerVersion: string;
  userId: string;
  workspaceRoot: string;
}

/**  */
// RegisterRunnerResponse
export interface RegisterRunnerResponse {
  heartbeatIntervalSec: number;
  pollIntervalSec: number;
  runnerId: string;
  sessionId: string;
}

/**  */
// SessionResponse
export interface SessionResponse {
  code?: number;
  data?: { created_at?: string; media_context?: Record<string, unknown>; messages?: Record<string, unknown>[]; session_id?: string; task_ids?: string[]; terminated?: boolean };
  message?: string;
}

/**  */
// SkillCapabilityDetailResponse
export interface SkillCapabilityDetailResponse {
  code?: number;
  data?: { capability?: { activation: { intents?: string[]; keywords?: string[] }; description: string; domain: string; id: string; loadError?: string; name: string; recipe: { mode?: string; path?: string }; resources: { id: string; loadStrategy?: string; path: string; priority?: number; scope?: string; type: string }[]; rootPath?: string; status: string; tools: { id: string; manifest: string; prompt?: string }[]; version: string } };
  message?: string;
}

/**  */
// SkillCapabilityListResponse
export interface SkillCapabilityListResponse {
  code?: number;
  data?: { capabilities?: { activation: { intents?: string[]; keywords?: string[] }; description: string; domain: string; id: string; loadError?: string; name: string; recipe: { mode?: string; path?: string }; resources: { id: string; loadStrategy?: string; path: string; priority?: number; scope?: string; type: string }[]; rootPath?: string; status: string; tools: { id: string; manifest: string; prompt?: string }[]; version: string }[] };
  message?: string;
}

/**  */
// SkillCatalogResponse
export interface SkillCatalogResponse {
  code?: number;
  data?: { health?: Record<string, unknown>; skills?: Record<string, unknown>[] };
  message?: string;
}

/**  */
// SkillDetailResponse
export interface SkillDetailResponse {
  code?: number;
  data?: { skill?: Record<string, unknown> };
  message?: string;
}

/**  */
// SkillRouteResponse
export interface SkillRouteResponse {
  code?: number;
  data?: Record<string, unknown>;
  message?: string;
}

/**  */
// SkillsResponse
export interface SkillsResponse {
  code?: number;
  data?: { health?: Record<string, unknown>; skills?: Record<string, unknown>[] };
  message?: string;
}

/**  */
// StageApprovalResponse
export interface StageApprovalResponse {
  code?: number;
  data?: { message?: string; nodeId?: string; stage?: string };
  message?: string;
}

/**  */
// TaskCreateResponse
export interface TaskCreateResponse {
  createdAt: string;
  id: string;
  input?: Record<string, Record<string, unknown>>;
  output?: Record<string, Record<string, unknown>>;
  pauseReason?: string;
  status: string;
  userId?: string;
}

/**  */
// TaskDetailResponse
export interface TaskDetailResponse {
  code?: number;
  data?: Record<string, unknown>;
  message?: string;
}

/**  */
// TaskLifecycleResponse
export interface TaskLifecycleResponse {
  code?: number;
  data?: { message?: string; taskId?: string };
  message?: string;
}

/**  */
// TaskProgressResponse
export interface TaskProgressResponse {
  code?: number;
  data?: Record<string, unknown>;
  message?: string;
}

/**  */
// TaskStatusResponse
export interface TaskStatusResponse {
  code?: number;
  data?: Record<string, unknown>;
  message?: string;
}

/**  */
// ToolDetailResponse
export interface ToolDetailResponse {
  code?: number;
  data?: Record<string, unknown>;
  message?: string;
}

/**  */
// ToolListResponse
export interface ToolListResponse {
  code?: number;
  data?: Record<string, unknown>[];
  message?: string;
}

/**  */
// ToolRegisterResponse
export interface ToolRegisterResponse {
  code?: number;
  data?: { name?: string; type?: string };
  message?: string;
}

/**  */
// TraceResponse
export interface TraceResponse {
  code?: number;
  data?: { contexts?: Record<string, unknown>[]; task?: Record<string, unknown> };
  message?: string;
}

/**  */
// TranslateSubmitResponse
export interface TranslateSubmitResponse {
  code?: number;
  data?: { message?: string; status?: string; taskId?: string };
  message?: string;
}

/**  */
// VideoProject
export interface VideoProject {
  aspectRatio?: string;
  config?: string;
  createdAt: string;
  currentRunId?: string;
  deletedAt?: string | null;
  description?: string;
  generationMode: string;
  id: string;
  language?: string;
  localPathHint?: string;
  mode: string;
  name: string;
  skillName: string;
  skillVersion: string;
  status: string;
  targetDurationSec?: number;
  updatedAt: string;
  userId: string;
  workflowName: string;
  workflowVersion: string;
}

/**  */
// VideoProjectCreateResponse
export interface VideoProjectCreateResponse {
  code?: number;
  data?: { project?: { aspectRatio?: string; config?: string; createdAt: string; currentRunId?: string; deletedAt?: string | null; description?: string; generationMode: string; id: string; language?: string; localPathHint?: string; mode: string; name: string; skillName: string; skillVersion: string; status: string; targetDurationSec?: number; updatedAt: string; userId: string; workflowName: string; workflowVersion: string } };
  message?: string;
}

/**  */
// VideoProjectDetailResponse
export interface VideoProjectDetailResponse {
  code?: number;
  data?: { project?: { aspectRatio?: string; config?: string; createdAt: string; currentRunId?: string; deletedAt?: string | null; description?: string; generationMode: string; id: string; language?: string; localPathHint?: string; mode: string; name: string; skillName: string; skillVersion: string; status: string; targetDurationSec?: number; updatedAt: string; userId: string; workflowName: string; workflowVersion: string } };
  message?: string;
}

/**  */
// VideoProjectListResponse
export interface VideoProjectListResponse {
  code?: number;
  data?: { projects?: Record<string, unknown>[]; total?: number };
  message?: string;
}

/**  */
// WorkflowListResponse
export interface WorkflowListResponse {
  code?: number;
  data?: { templates?: Record<string, unknown>[] };
  message?: string;
}

/**  */
// WorkflowRunCreateResponse
export interface WorkflowRunCreateResponse {
  code?: number;
  data?: { run?: Record<string, unknown> };
  message?: string;
}

/**  */
// WorkflowRunDetailResponse
export interface WorkflowRunDetailResponse {
  code?: number;
  data?: { run?: Record<string, unknown> };
  message?: string;
}

/**  */
// WorkflowTemplate
export interface WorkflowTemplate {
  category?: string;
  createdAt: string;
  dag: string;
  description?: string;
  id: string;
  name: string;
  updatedAt: string;
  version: string;
}

/**  */
// WorkflowTemplateResponse
export interface WorkflowTemplateResponse {
  code?: number;
  data?: { template?: Record<string, unknown> };
  message?: string;
}

