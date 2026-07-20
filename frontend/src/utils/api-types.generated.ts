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
  data?: { reviews?: { artifactId?: string; blocksDownstream?: boolean; humanReview?: Record<string, unknown>; id: string; nodeId: string; requiredInputs?: string[]; requiredOutputs?: string[]; reviewArtifactKinds?: string[]; reviewArtifacts?: Record<string, unknown>[]; reviewContent?: string; reviewOutput?: Record<string, unknown>; reviewPhase?: string; reviewReason?: string; roleAgent?: Record<string, unknown>; roleAgentId?: string; sourceNodeId?: string; stage?: string; status: string; stepId?: string; tool?: string }[]; runId?: string };
  message?: string;
}

/**  */
// AgentRunDetailResponse
export interface AgentRunDetailResponse {
  code?: number;
  data?: { run?: { budget?: { maxCostLevel?: string; maxLLMCalls?: number; maxReplans?: number; maxSteps?: number; maxToolCalls?: number }; createdAt: string; domain?: string; id: string; message: string; metadata?: Record<string, unknown>; plan?: { budget: { maxCostLevel?: string; maxLLMCalls?: number; maxReplans?: number; maxSteps?: number; maxToolCalls?: number }; domain: string; goal: string; knowledgePolicy?: { allowedTools?: string[]; blockOnEmptyFacts: boolean; contentType: string; forbiddenCapabilities?: string[]; forbiddenTools?: string[]; freshnessDays?: number; freshnessLevel: string; knowledgeType: string; maxSearchResults?: number; mustCiteFacts: boolean; mustUseFacts: boolean; reason?: string; requiredCapabilities?: string[]; retrievalPolicy: string; searchQueries?: string[] } | null; mode: string; steps: { arguments: Record<string, unknown>; dependsOn?: string[]; expectedOutput?: string[]; id: string; intent?: string; produceArtifact?: boolean; reason?: string; tool: string }[]; stopPolicy: { stopWhenEnough?: boolean }; toolTrace?: { candidateTools?: { costRiskPenalty?: number; hitWhenNotToUse?: string[]; knowledgePolicyReason?: string; matchedCapabilities?: string[]; matchedTags?: string[]; matchedWhenToUse?: string[]; name: string; reason: string; score: number }[]; executedTools?: string[]; guardDecision?: { passed: boolean; warnings?: string[] } | null; knowledgeContext?: { generatedBy?: string[]; itemCount: number; sourceCount: number } | null; plannedTools?: string[] } | null } | null; status: string; taskId?: string; updatedAt: string; userId?: string }; task?: Record<string, unknown> };
  message?: string;
}

/**  */
// AgentRunStartResponse
export interface AgentRunStartResponse {
  code?: number;
  data?: { plan?: { budget: { maxCostLevel?: string; maxLLMCalls?: number; maxReplans?: number; maxSteps?: number; maxToolCalls?: number }; domain: string; goal: string; knowledgePolicy?: { allowedTools?: string[]; blockOnEmptyFacts: boolean; contentType: string; forbiddenCapabilities?: string[]; forbiddenTools?: string[]; freshnessDays?: number; freshnessLevel: string; knowledgeType: string; maxSearchResults?: number; mustCiteFacts: boolean; mustUseFacts: boolean; reason?: string; requiredCapabilities?: string[]; retrievalPolicy: string; searchQueries?: string[] } | null; mode: string; steps: { arguments: Record<string, unknown>; dependsOn?: string[]; expectedOutput?: string[]; id: string; intent?: string; produceArtifact?: boolean; reason?: string; tool: string }[]; stopPolicy: { stopWhenEnough?: boolean }; toolTrace?: { candidateTools?: { costRiskPenalty?: number; hitWhenNotToUse?: string[]; knowledgePolicyReason?: string; matchedCapabilities?: string[]; matchedTags?: string[]; matchedWhenToUse?: string[]; name: string; reason: string; score: number }[]; executedTools?: string[]; guardDecision?: { passed: boolean; warnings?: string[] } | null; knowledgeContext?: { generatedBy?: string[]; itemCount: number; sourceCount: number } | null; plannedTools?: string[] } | null }; runId?: string; status?: string; taskId?: string };
  message?: string;
}

/**  */
// AgentRunTraceResponse
export interface AgentRunTraceResponse {
  code?: number;
  data?: { nodes?: ({ completedAt?: string | null; condition?: string; createdAt: string; currentStep?: string; errorMessage?: string; heartbeatAt?: string | null; heartbeatTimeoutSec?: number; id: string; idempotencyKey?: string; input?: Record<string, unknown>; longRunning: boolean; maxRetry: number; name: string; output?: Record<string, unknown>; priority: number; progress: number; retryCount: number; startedAt?: string | null; status: string; taskId: string; type: string; version: number; workerGroup: string })[]; task?: Record<string, unknown> };
  message?: string;
}

/**  */
// AgentStartRunRequest
export interface AgentStartRunRequest {
  context?: Record<string, unknown>;
  domain?: string;
  maxCostLevel?: string;
  maxRiskLevel?: string;
  message: string;
  mode?: string;
  userId?: string;
}

/**  */
// Artifact
export interface Artifact {
  contentHash: string;
  createdAt: string;
  dependsOn?: string[];
  humanApproved: boolean;
  id: string;
  inlineJson?: string;
  isCurrent: boolean;
  kind: string;
  metadata?: Record<string, unknown>;
  mimeType?: string;
  model?: string;
  name: string;
  parentId?: string;
  producedByNode?: string;
  producedByRole?: string;
  producedByTool?: string;
  projectId: string;
  promptHash?: string;
  provider?: string;
  roleAgentId?: string;
  sizeBytes: number;
  stageName: string;
  status: string;
  storageRef?: string;
  storageType: string;
  taskId?: string;
  unitId?: string;
  updatedAt: string;
  version: number;
  workflowRunId?: string;
}

/**  */
// ArtifactContentResponse
export interface ArtifactContentResponse {
  code?: number;
  data?: { artifact?: { contentHash: string; createdAt: string; dependsOn?: string[]; humanApproved: boolean; id: string; inlineJson?: string; isCurrent: boolean; kind: string; metadata?: Record<string, unknown>; mimeType?: string; model?: string; name: string; parentId?: string; producedByNode?: string; producedByRole?: string; producedByTool?: string; projectId: string; promptHash?: string; provider?: string; roleAgentId?: string; sizeBytes: number; stageName: string; status: string; storageRef?: string; storageType: string; taskId?: string; unitId?: string; updatedAt: string; version: number; workflowRunId?: string }; content?: Record<string, unknown>; mediaUrl?: string; mediaUrls?: string[] };
  message?: string;
}

/**  */
// ArtifactDetailResponse
export interface ArtifactDetailResponse {
  code?: number;
  data?: { artifact?: { contentHash: string; createdAt: string; dependsOn?: string[]; humanApproved: boolean; id: string; inlineJson?: string; isCurrent: boolean; kind: string; metadata?: Record<string, unknown>; mimeType?: string; model?: string; name: string; parentId?: string; producedByNode?: string; producedByRole?: string; producedByTool?: string; projectId: string; promptHash?: string; provider?: string; roleAgentId?: string; sizeBytes: number; stageName: string; status: string; storageRef?: string; storageType: string; taskId?: string; unitId?: string; updatedAt: string; version: number; workflowRunId?: string } };
  message?: string;
}

/**  */
// ArtifactHistoryResponse
export interface ArtifactHistoryResponse {
  code?: number;
  data?: { history?: { contentHash: string; createdAt: string; dependsOn?: string[]; humanApproved: boolean; id: string; inlineJson?: string; isCurrent: boolean; kind: string; metadata?: Record<string, unknown>; mimeType?: string; model?: string; name: string; parentId?: string; producedByNode?: string; producedByRole?: string; producedByTool?: string; projectId: string; promptHash?: string; provider?: string; roleAgentId?: string; sizeBytes: number; stageName: string; status: string; storageRef?: string; storageType: string; taskId?: string; unitId?: string; updatedAt: string; version: number; workflowRunId?: string }[] };
  message?: string;
}

/**  */
// ArtifactListResponse
export interface ArtifactListResponse {
  code?: number;
  data?: { artifacts?: { contentHash: string; createdAt: string; dependsOn?: string[]; humanApproved: boolean; id: string; inlineJson?: string; isCurrent: boolean; kind: string; metadata?: Record<string, unknown>; mimeType?: string; model?: string; name: string; parentId?: string; producedByNode?: string; producedByRole?: string; producedByTool?: string; projectId: string; promptHash?: string; provider?: string; roleAgentId?: string; sizeBytes: number; stageName: string; status: string; storageRef?: string; storageType: string; taskId?: string; unitId?: string; updatedAt: string; version: number; workflowRunId?: string }[] };
  message?: string;
}

/**  */
// ArtifactSelection
export type ArtifactSelection = { endMs?: never; height: number; kind: 'rect'; startMs?: never; width: number; x: number; y: number } | { endMs: number; height?: never; kind: 'time'; startMs: number; width?: never; x?: never; y?: never };

/**  */
// AssemblyValidationResponse
export interface AssemblyValidationResponse {
  code?: number;
  data?: { issues?: { code: string; field?: string; message: string; severity: string }[] };
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
// CandidateAcceptRequest
export interface CandidateAcceptRequest {
  baseVersion: number;
  locks: ('duration' | 'narration' | 'character' | 'wardrobe' | 'scene' | 'camera' | 'first_frame' | 'last_frame' | 'reference_set' | 'accepted_overlay')[];
  scope: 'candidate_accept';
}

/**  */
// CandidateRestoreRequest
export interface CandidateRestoreRequest {
  baseVersion: number;
  locks: ('duration' | 'narration' | 'character' | 'wardrobe' | 'scene' | 'camera' | 'first_frame' | 'last_frame' | 'reference_set' | 'accepted_overlay')[];
  scope: 'candidate_restore';
}

/**  */
// ClaimJobResponse
export interface ClaimJobResponse {
  job: { artifactPolicy?: { location: string; syncFileToCloud: boolean; syncMetadataToCloud: boolean }; attempt?: number; command: string; createdAt?: string; currentStep?: string; diagnostics?: Record<string, unknown>; error?: Record<string, unknown>; errorMessage?: string; idempotencyKey?: string; jobId: string; leaseExpiresAt?: string | null; message?: string; nodeId?: string; output?: Record<string, unknown>; payload: Record<string, unknown>; progress?: number; projectId: string; retryable?: boolean; runnerId?: string; status?: string; taskId?: string; timeoutSec?: number; toolName?: string; updatedAt?: string } | null;
}

/**  */
// CompleteJobRequest
export interface CompleteJobRequest {
  output: Record<string, unknown>;
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
// CreationView
export interface CreationView {
  activeStep: 'requirements' | 'direction' | 'script' | 'shots' | 'preview' | 'delivery';
  activeTasks: CreatorTask[];
  assemblyDirty: boolean;
  project: VideoProject;
  shotSummary: ShotSummary;
  steps: CreatorStep[];
}

/**  */
// CreationViewResponse
export interface CreationViewResponse {
  code: number;
  data: CreationView;
  message: string;
}

/**  */
// CreatorArtifactVersion
export interface CreatorArtifactVersion {
  artifactId: string;
  createdAt: string;
  isCurrent: boolean;
  version: number;
}

/**  */
// CreatorShotUnitResponse
export interface CreatorShotUnitResponse {
  code: number;
  data: { shot: ShotUnit };
  message: string;
}

/**  */
// CreatorStep
export interface CreatorStep {
  allowedActions: string[];
  currentArtifactId?: string;
  currentVersion?: number;
  id: 'requirements' | 'direction' | 'script' | 'shots' | 'preview' | 'delivery';
  label: string;
  reviewId?: string;
  runId?: string;
  state: 'not_started' | 'generating' | 'needs_review' | 'confirmed' | 'needs_attention' | 'failed';
}

/**  */
// CreatorTask
export interface CreatorTask {
  id: string;
  label: string;
  scope: 'requirements' | 'direction' | 'script' | 'shots' | 'preview' | 'delivery';
  shotId?: string;
  status: 'generating' | 'running' | 'processing' | 'queued' | 'dispatching';
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
  nodes: ({ condition?: string; heartbeatTimeoutSec?: number | null; id: string; input?: Record<string, unknown>; longRunning?: boolean; maxRetry?: number | null; name: string; priority?: number | null; type: string; workerGroup?: string })[];
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
// ExternalGenerationResultResponse
export interface ExternalGenerationResultResponse {
  code?: number;
  data?: { artifact?: { contentHash: string; createdAt: string; dependsOn?: string[]; humanApproved: boolean; id: string; inlineJson?: string; isCurrent: boolean; kind: string; metadata?: Record<string, unknown>; mimeType?: string; model?: string; name: string; parentId?: string; producedByNode?: string; producedByRole?: string; producedByTool?: string; projectId: string; promptHash?: string; provider?: string; roleAgentId?: string; sizeBytes: number; stageName: string; status: string; storageRef?: string; storageType: string; taskId?: string; unitId?: string; updatedAt: string; version: number; workflowRunId?: string }; manifest?: { assetId: string; contentHash?: string; description?: string; durationSec?: number; externalPlatform?: string; generationRequestId?: string; mimeType?: string; promptHash?: string; referenceAssetIds?: string[]; relatedShotId?: string; sizeBytes?: number; source: string; storageRef: string; storageType: string; tags?: string[]; type: string } };
  message?: string;
}

/**  */
// FailJobRequest
export interface FailJobRequest {
  diagnostics?: Record<string, unknown>;
  error: Record<string, unknown>;
  retryable: boolean;
  success: boolean;
}

/**  */
// GenerateVideoCreationSpecRequest
export interface GenerateVideoCreationSpecRequest {
  platform?: string;
  sourceMessage: string;
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
  lastError: string | null;
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
// ProjectMaterial
export interface ProjectMaterial {
  contentHash: string;
  kind: 'image' | 'audio' | 'video' | 'document';
  mimeType: string;
  name: string;
  sizeBytes: number;
  storageRef: string;
}

/**  */
// ProjectMaterialManifest
export interface ProjectMaterialManifest {
  materials: ProjectMaterial[];
  relatedProjectId: string;
  schemaVersion: number;
}

/**  */
// ProjectMaterialResponse
export interface ProjectMaterialResponse {
  code: number;
  data: { artifact: { contentHash: string; createdAt: string; dependsOn?: string[]; humanApproved: boolean; id: string; inlineJson?: string; isCurrent: boolean; kind: string; metadata?: Record<string, unknown>; mimeType?: string; model?: string; name: string; parentId?: string; producedByNode?: string; producedByRole?: string; producedByTool?: string; projectId: string; promptHash?: string; provider?: string; roleAgentId?: string; sizeBytes: number; stageName: string; status: string; storageRef?: string; storageType: string; taskId?: string; unitId?: string; updatedAt: string; version: number; workflowRunId?: string }; material: ProjectMaterial };
  message: string;
}

/**  */
// PublishPackageResponse
export interface PublishPackageResponse {
  code?: number;
  data?: { publishPackage?: Record<string, unknown> };
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
// RegenerateShotRequest
export interface RegenerateShotRequest {
  baseVersion: number;
  instruction?: string;
  locks?: string[];
  requestFingerprint: string;
  scope: string;
}

/**  */
// RegisterExternalGenerationResultRequest
export interface RegisterExternalGenerationResultRequest {
  contentHash?: string;
  description?: string;
  durationSec?: number;
  externalPlatform?: string;
  generationRequestId?: string;
  kind: string;
  mimeType?: string;
  promptHash?: string;
  referenceAssetIds?: string[];
  relatedShotId?: string;
  sizeBytes?: number;
  source?: string;
  storageRef: string;
  storageType?: string;
  tags?: string[];
}

/**  */
// RegisterProjectMaterialRequest
export interface RegisterProjectMaterialRequest {
  contentHash: string;
  kind: 'image' | 'audio' | 'video' | 'document';
  mimeType: string;
  name: string;
  sizeBytes: number;
  storageRef: string;
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
// RejectVideoCreationArtifactRequest
export interface RejectVideoCreationArtifactRequest {
  reason: string;
}

/**  */
// RenderStrategy
export interface RenderStrategy {
  aigcInput?: { durationSec?: number; negativePrompt?: string; prompt?: string } | null;
  aigcRequired: boolean;
  compositePlan?: { backgroundArtifactId?: string; outputArtifactType?: string; overlayArtifactId?: string } | null;
  htmlInput?: { durationSec?: number; textLayers?: { animation?: string; background?: string; color?: string; endSec: number; fontSize?: number; fontWeight?: string; id: string; language?: string; mustBeExact: boolean; position?: string; role: string; startSec: number; text: string }[] } | null;
  htmlRequired: boolean;
  mode: string;
  needsCompositing: boolean;
  primaryTool?: string;
  reason?: string;
  secondaryTools?: string[];
  textOverlayNeeded: boolean;
}

/**  */
// RenderStrategyResponse
export interface RenderStrategyResponse {
  code?: number;
  data?: { renderStrategy?: { aigcInput?: { durationSec?: number; negativePrompt?: string; prompt?: string } | null; aigcRequired: boolean; compositePlan?: { backgroundArtifactId?: string; outputArtifactType?: string; overlayArtifactId?: string } | null; htmlInput?: { durationSec?: number; textLayers?: { animation?: string; background?: string; color?: string; endSec: number; fontSize?: number; fontWeight?: string; id: string; language?: string; mustBeExact: boolean; position?: string; role: string; startSec: number; text: string }[] } | null; htmlRequired: boolean; mode: string; needsCompositing: boolean; primaryTool?: string; reason?: string; secondaryTools?: string[]; textOverlayNeeded: boolean } };
  message?: string;
}

/**  */
// ShotCandidate
export interface ShotCandidate {
  artifactRefs?: { aigcBackgroundVideoArtifactId?: string; compositedShotVideoArtifactId?: string; htmlOverlayVideoArtifactId?: string; htmlPreviewVideoArtifactId?: string; htmlSourceArtifactId?: string; keyframeImageArtifactId?: string; keyframePromptArtifactId?: string; renderStrategyArtifactId?: string; subtitleArtifactId?: string; videoClipArtifactId?: string; videoPromptArtifactId?: string; visualPlanArtifactId?: string };
  attemptIndex: number;
  candidateId: string;
  createdAt?: string;
  durationSec: number;
  executionMode?: 'unknown' | 'real' | 'fixture' | 'fallback' | 'placeholder';
  fallbackReason?: string;
  generationPlanRevision?: string;
  inputFingerprint?: string;
  isFallback?: boolean;
  layerRevisions?: Record<string, string>;
  outputFingerprint?: string;
  productionEligible: boolean;
  qaReport?: ShotQAReport | null;
  repairPlan?: { action: string; attemptIndex: number; lockedDimensions?: string[]; nextToolCall?: string; preserve: boolean; promptPatch?: Record<string, unknown>; reason?: string; renderStrategyPatch?: Record<string, unknown>; repairTargets?: string[]; severity?: string; sourceCandidateId?: string; targetShotId?: string } | null;
  schemaVersion?: number;
  shotId: string;
  sourceType?: string;
  stale?: boolean;
  staleReason?: string;
  status: 'CANDIDATE_RENDERED' | 'SHOT_QA_RUNNING' | 'SHOT_QA_PASSED' | 'SHOT_QA_FAILED' | 'HUMAN_REVIEW_REQUIRED' | 'ACCEPTED_FOR_ASSEMBLY' | 'stale';
  timelineRevision?: string;
}

/**  */
// ShotHistoryResponse
export interface ShotHistoryResponse {
  code: number;
  data: { history: ShotRevision[] };
  message: string;
}

/**  */
// ShotImpact
export interface ShotImpact {
  affectedShotIds: string[];
  estimatedDurationSec: number;
  invalidatesFinalAssembly: boolean;
  regeneratesOtherShots: boolean;
  requiresConfirmation: boolean;
  shotId: string;
}

/**  */
// ShotImpactResponse
export interface ShotImpactResponse {
  code: number;
  data: { impact: ShotImpact };
  message: string;
}

/**  */
// ShotListItem
export interface ShotListItem {
  acceptedCandidateId?: string;
  chapter?: string;
  durationSec: number;
  generationStatus: 'PLANNED' | 'GENERATING' | 'CANDIDATE_RENDERED' | 'SHOT_QA_RUNNING' | 'SHOT_QA_PASSED' | 'SHOT_QA_FAILED' | 'HUMAN_REVIEW_REQUIRED' | 'ACCEPTED_FOR_ASSEMBLY' | 'stale' | 'queued' | 'dispatching' | 'running' | 'failed' | 'cancelled';
  id: string;
  qaStatus?: 'PLANNED' | 'GENERATING' | 'CANDIDATE_RENDERED' | 'SHOT_QA_RUNNING' | 'SHOT_QA_PASSED' | 'SHOT_QA_FAILED' | 'HUMAN_REVIEW_REQUIRED' | 'ACCEPTED_FOR_ASSEMBLY' | 'stale';
  reviewStatus: 'pending' | 'approved' | 'rejected' | 'stale';
  sequenceIndex: number;
  thumbnailRef?: string;
  title: string;
  version: number;
}

/**  */
// ShotPageResponse
export interface ShotPageResponse {
  code: number;
  data: { nextCursor: string; shots: ShotListItem[]; total: number };
  message: string;
}

/**  */
// ShotQAReport
export interface ShotQAReport {
  candidateId: string;
  failedDimensions?: string[];
  humanApproved?: boolean;
  overallScore?: number;
  passed: boolean;
  passedDimensions?: string[];
  reportRef?: string;
  scores?: Record<string, number>;
  severity?: string;
  shotId: string;
  status: 'PLANNED' | 'GENERATING' | 'CANDIDATE_RENDERED' | 'SHOT_QA_RUNNING' | 'SHOT_QA_PASSED' | 'SHOT_QA_FAILED' | 'HUMAN_REVIEW_REQUIRED' | 'ACCEPTED_FOR_ASSEMBLY' | 'stale';
  summary?: string;
}

/**  */
// ShotRegenerationRequest
export interface ShotRegenerationRequest {
  baseVersion: number;
  instruction?: string;
  locks: ('duration' | 'narration' | 'character' | 'wardrobe' | 'scene' | 'camera' | 'first_frame' | 'last_frame' | 'reference_set' | 'accepted_overlay')[];
  scope: 'prompt' | 'reference' | 'base_media' | 'overlay' | 'audio_alignment' | 'full_shot';
}

/**  */
// ShotRegenerationResponse
export interface ShotRegenerationResponse {
  code: number;
  data: { shot: ShotUnit; task: ShotRegenerationTask };
  message: string;
}

/**  */
// ShotRegenerationTask
export interface ShotRegenerationTask {
  baseVersion: number;
  createdAt: string;
  dispatchAttempts?: number;
  dispatchLeaseUntil?: string;
  failureReason?: string;
  idempotencyKey: string;
  instruction?: string;
  locks?: ('duration' | 'narration' | 'character' | 'wardrobe' | 'scene' | 'camera' | 'first_frame' | 'last_frame' | 'reference_set' | 'accepted_overlay')[];
  requestFingerprint: string;
  runId?: string;
  scope: 'prompt' | 'reference' | 'base_media' | 'overlay' | 'audio_alignment' | 'full_shot';
  shotId: string;
  status: 'queued' | 'dispatching' | 'running' | 'completed' | 'failed' | 'cancelled';
  taskId: string;
  updatedAt: string;
}

/**  */
// ShotRevision
export interface ShotRevision {
  createdAt: string;
  reason: string;
  revisionId: string;
  shotId: string;
  snapshot: ShotUnit;
  version: number;
}

/**  */
// ShotSummary
export interface ShotSummary {
  awaitingReview: number;
  confirmed: number;
  generating: number;
  needsAction: number;
  total: number;
}

/**  */
// ShotSummaryResponse
export interface ShotSummaryResponse {
  code: number;
  data: { summary: ShotSummary };
  message: string;
}

/**  */
// ShotUnit
export interface ShotUnit {
  acceptedCandidateId?: string;
  action?: string;
  artifactRefs?: { aigcBackgroundVideoArtifactId?: string; compositedShotVideoArtifactId?: string; htmlOverlayVideoArtifactId?: string; htmlPreviewVideoArtifactId?: string; htmlSourceArtifactId?: string; keyframeImageArtifactId?: string; keyframePromptArtifactId?: string; renderStrategyArtifactId?: string; subtitleArtifactId?: string; videoClipArtifactId?: string; videoPromptArtifactId?: string; visualPlanArtifactId?: string };
  camera?: string;
  candidates?: ShotCandidate[];
  continuity: { characters?: string[]; endState?: string; mustMatchNext?: boolean; mustMatchPrevious?: boolean; previousState?: string; props?: string[]; styleTags?: string[] };
  createdAt: string;
  durationMs?: number;
  durationSec: number;
  endMs?: number;
  endSec?: number;
  focalLengthHint?: string;
  framing?: string;
  id: string;
  lastRejectReason?: string;
  locked: boolean;
  mainAction?: string;
  narration?: string;
  projectId: string;
  promptConstraints: { mustAvoid?: string[]; mustInclude?: string[] };
  qaStatus?: 'PLANNED' | 'GENERATING' | 'CANDIDATE_RENDERED' | 'SHOT_QA_RUNNING' | 'SHOT_QA_PASSED' | 'SHOT_QA_FAILED' | 'HUMAN_REVIEW_REQUIRED' | 'ACCEPTED_FOR_ASSEMBLY' | 'stale';
  renderStrategy?: { aigcInput?: { durationSec?: number; negativePrompt?: string; prompt?: string } | null; aigcRequired: boolean; compositePlan?: { backgroundArtifactId?: string; outputArtifactType?: string; overlayArtifactId?: string } | null; htmlInput?: { durationSec?: number; textLayers?: { animation?: string; background?: string; color?: string; endSec: number; fontSize?: number; fontWeight?: string; id: string; language?: string; mustBeExact: boolean; position?: string; role: string; startSec: number; text: string }[] } | null; htmlRequired: boolean; mode: string; needsCompositing: boolean; primaryTool?: string; reason?: string; secondaryTools?: string[]; textOverlayNeeded: boolean };
  repairPlans?: { action: string; attemptIndex: number; lockedDimensions?: string[]; nextToolCall?: string; preserve: boolean; promptPatch?: Record<string, unknown>; reason?: string; renderStrategyPatch?: Record<string, unknown>; repairTargets?: string[]; severity?: string; sourceCandidateId?: string; targetShotId?: string }[];
  reviewStatus: 'pending' | 'approved' | 'rejected' | 'stale';
  scene?: string;
  sceneId?: string;
  sceneSummary?: string;
  schemaVersion?: number;
  screenText?: string[];
  scriptSegmentId?: string;
  sequenceIndex: number;
  shotSize?: string;
  singleScene: boolean;
  stale: boolean;
  startMs?: number;
  startSec?: number;
  subject?: string;
  talkingHeadLayers?: { audio: { audioMasterRevision: string; state: { artifactRef?: string; createdAt?: string; dependencies?: { artifactId?: string; fingerprint?: string; layer?: string; revision: string }[]; executionMode?: string; inputFingerprint?: string; layer: string; model?: string; outputFingerprint?: string; productionEligible: boolean; provider?: string; revision?: string; schemaVersion: number; staleReason?: string; status: string; toolVersion?: string }; voiceProfileId?: string; voiceProfileVersion?: string; voiceoverArtifactRef?: string }; broll: { entries?: { artifactRef?: string; assetType: string; endMs: number; fit?: string; generated: boolean; id: string; licenseStatus: string; model?: string; narrationText?: string; placement?: string; provenance?: { artifactId?: string; candidateId?: string; fallbackReason?: string; generatedAt?: string; inputPromptHash?: string; isFallback: boolean; kind?: string; providerJobId?: string; providerName?: string; shotId?: string; sourceArtifactIds?: string[]; sourceType: string }; provider?: string; relevanceScore?: number; replacementHistory?: string[]; reviewStatus: string; seed?: string; semanticPurpose: string; shotId: string; sourceType: string; sourceUri?: string; startMs: number; usageStatus?: string }[]; manifestRef?: string; state: { artifactRef?: string; createdAt?: string; dependencies?: { artifactId?: string; fingerprint?: string; layer?: string; revision: string }[]; executionMode?: string; inputFingerprint?: string; layer: string; model?: string; outputFingerprint?: string; productionEligible: boolean; provider?: string; revision?: string; schemaVersion: number; staleReason?: string; status: string; toolVersion?: string } }; composition: { assembler?: string; baseArtifactRef?: string; outputRequirements?: Record<string, unknown>; overlayArtifactRefs?: string[]; state: { artifactRef?: string; createdAt?: string; dependencies?: { artifactId?: string; fingerprint?: string; layer?: string; revision: string }[]; executionMode?: string; inputFingerprint?: string; layer: string; model?: string; outputFingerprint?: string; productionEligible: boolean; provider?: string; revision?: string; schemaVersion: number; staleReason?: string; status: string; toolVersion?: string } }; ip: { assetPack: { contentHash: string; id: string; version: string }; backgroundMode?: string; backgroundTemplate?: string; displayMode?: string; expression?: string; framingPreset?: string; gestureEvents?: { atMs: number; endMs?: number; id?: string; timelineRevision: string; type: string; value?: string }[]; lipSyncRevision?: string; pose?: string; state: { artifactRef?: string; createdAt?: string; dependencies?: { artifactId?: string; fingerprint?: string; layer?: string; revision: string }[]; executionMode?: string; inputFingerprint?: string; layer: string; model?: string; outputFingerprint?: string; productionEligible: boolean; provider?: string; revision?: string; schemaVersion: number; staleReason?: string; status: string; toolVersion?: string } }; needsHumanReview?: boolean; schemaVersion: number; text: { projectRef?: string; renderer: string; state: { artifactRef?: string; createdAt?: string; dependencies?: { artifactId?: string; fingerprint?: string; layer?: string; revision: string }[]; executionMode?: string; inputFingerprint?: string; layer: string; model?: string; outputFingerprint?: string; productionEligible: boolean; provider?: string; revision?: string; schemaVersion: number; staleReason?: string; status: string; toolVersion?: string }; textLayers?: { animation?: string; background?: string; color?: string; endSec: number; fontSize?: number; fontWeight?: string; id: string; language?: string; mustBeExact: boolean; position?: string; role: string; startSec: number; text: string }[]; timelineRevision: string }; timelineRevision: string; visualMode: string; visualModeConfidence?: number; visualModeReason?: string } | null;
  timelineRevision?: string;
  title: string;
  transitionIn?: string;
  transitionOut?: string;
  updatedAt: string;
  version: number;
  videoType?: string;
  visualChangeLevel: string;
  visualChangeReason?: string;
  visualPlan?: { background?: { description?: string; requiresAigc?: boolean }; cameraPlan?: { description?: string; movement?: string; requiresAigc?: boolean }; canvas: { aspectRatio: string; durationSec: number; fps: number; height: number; width: number }; characters?: { description?: string; emotion?: string; id: string; motion?: string; requiresAigc?: boolean }[]; constraints?: { mustAvoid?: string[]; mustInclude?: string[] }; dataVisuals?: { description?: string; id: string; type?: string }[]; motionPlan?: { description?: string; requiresAigc?: boolean }; props?: { description?: string; id: string }[]; style?: { description?: string; tags?: string[] }; textLayers?: { animation?: string; background?: string; color?: string; endSec: number; fontSize?: number; fontWeight?: string; id: string; language?: string; mustBeExact: boolean; position?: string; role: string; startSec: number; text: string }[]; transitionIn?: string; transitionOut?: string; uiLayers?: { description?: string; id: string }[] };
}

/**  */
// ShotUnitListResponse
export interface ShotUnitListResponse {
  code?: number;
  data?: { shots?: ({ acceptedCandidateId?: string; action?: string; artifactRefs?: { aigcBackgroundVideoArtifactId?: string; compositedShotVideoArtifactId?: string; htmlOverlayVideoArtifactId?: string; htmlPreviewVideoArtifactId?: string; htmlSourceArtifactId?: string; keyframeImageArtifactId?: string; keyframePromptArtifactId?: string; renderStrategyArtifactId?: string; subtitleArtifactId?: string; videoClipArtifactId?: string; videoPromptArtifactId?: string; visualPlanArtifactId?: string }; camera?: string; candidates?: ({ artifactRefs?: { aigcBackgroundVideoArtifactId?: string; compositedShotVideoArtifactId?: string; htmlOverlayVideoArtifactId?: string; htmlPreviewVideoArtifactId?: string; htmlSourceArtifactId?: string; keyframeImageArtifactId?: string; keyframePromptArtifactId?: string; renderStrategyArtifactId?: string; subtitleArtifactId?: string; videoClipArtifactId?: string; videoPromptArtifactId?: string; visualPlanArtifactId?: string }; attemptIndex: number; candidateId: string; createdAt?: string; durationSec: number; executionMode?: string; fallbackReason?: string; generationPlanRevision?: string; inputFingerprint?: string; isFallback?: boolean; layerRevisions?: Record<string, string>; outputFingerprint?: string; productionEligible: boolean; qaReport?: { candidateId: string; failedDimensions?: string[]; humanApproved?: boolean; overallScore?: number; passed: boolean; passedDimensions?: string[]; reportRef?: string; scores?: Record<string, number>; severity?: string; shotId: string; status: string; summary?: string } | null; repairPlan?: { action: string; attemptIndex: number; lockedDimensions?: string[]; nextToolCall?: string; preserve: boolean; promptPatch?: Record<string, unknown>; reason?: string; renderStrategyPatch?: Record<string, unknown>; repairTargets?: string[]; severity?: string; sourceCandidateId?: string; targetShotId?: string } | null; schemaVersion?: number; shotId: string; sourceType?: string; stale?: boolean; staleReason?: string; status: string; timelineRevision?: string })[]; continuity: { characters?: string[]; endState?: string; mustMatchNext?: boolean; mustMatchPrevious?: boolean; previousState?: string; props?: string[]; styleTags?: string[] }; createdAt: string; durationMs?: number; durationSec: number; endMs?: number; endSec?: number; focalLengthHint?: string; framing?: string; id: string; lastRejectReason?: string; locked: boolean; mainAction?: string; narration?: string; projectId: string; promptConstraints: { mustAvoid?: string[]; mustInclude?: string[] }; qaStatus?: string; renderStrategy?: { aigcInput?: { durationSec?: number; negativePrompt?: string; prompt?: string } | null; aigcRequired: boolean; compositePlan?: { backgroundArtifactId?: string; outputArtifactType?: string; overlayArtifactId?: string } | null; htmlInput?: { durationSec?: number; textLayers?: { animation?: string; background?: string; color?: string; endSec: number; fontSize?: number; fontWeight?: string; id: string; language?: string; mustBeExact: boolean; position?: string; role: string; startSec: number; text: string }[] } | null; htmlRequired: boolean; mode: string; needsCompositing: boolean; primaryTool?: string; reason?: string; secondaryTools?: string[]; textOverlayNeeded: boolean }; repairPlans?: { action: string; attemptIndex: number; lockedDimensions?: string[]; nextToolCall?: string; preserve: boolean; promptPatch?: Record<string, unknown>; reason?: string; renderStrategyPatch?: Record<string, unknown>; repairTargets?: string[]; severity?: string; sourceCandidateId?: string; targetShotId?: string }[]; reviewStatus: string; scene?: string; sceneId?: string; sceneSummary?: string; schemaVersion?: number; screenText?: string[]; scriptSegmentId?: string; sequenceIndex: number; shotSize?: string; singleScene: boolean; stale: boolean; startMs?: number; startSec?: number; subject?: string; talkingHeadLayers?: { audio: { audioMasterRevision: string; state: { artifactRef?: string; createdAt?: string; dependencies?: { artifactId?: string; fingerprint?: string; layer?: string; revision: string }[]; executionMode?: string; inputFingerprint?: string; layer: string; model?: string; outputFingerprint?: string; productionEligible: boolean; provider?: string; revision?: string; schemaVersion: number; staleReason?: string; status: string; toolVersion?: string }; voiceProfileId?: string; voiceProfileVersion?: string; voiceoverArtifactRef?: string }; broll: { entries?: { artifactRef?: string; assetType: string; endMs: number; fit?: string; generated: boolean; id: string; licenseStatus: string; model?: string; narrationText?: string; placement?: string; provenance?: { artifactId?: string; candidateId?: string; fallbackReason?: string; generatedAt?: string; inputPromptHash?: string; isFallback: boolean; kind?: string; providerJobId?: string; providerName?: string; shotId?: string; sourceArtifactIds?: string[]; sourceType: string }; provider?: string; relevanceScore?: number; replacementHistory?: string[]; reviewStatus: string; seed?: string; semanticPurpose: string; shotId: string; sourceType: string; sourceUri?: string; startMs: number; usageStatus?: string }[]; manifestRef?: string; state: { artifactRef?: string; createdAt?: string; dependencies?: { artifactId?: string; fingerprint?: string; layer?: string; revision: string }[]; executionMode?: string; inputFingerprint?: string; layer: string; model?: string; outputFingerprint?: string; productionEligible: boolean; provider?: string; revision?: string; schemaVersion: number; staleReason?: string; status: string; toolVersion?: string } }; composition: { assembler?: string; baseArtifactRef?: string; outputRequirements?: Record<string, unknown>; overlayArtifactRefs?: string[]; state: { artifactRef?: string; createdAt?: string; dependencies?: { artifactId?: string; fingerprint?: string; layer?: string; revision: string }[]; executionMode?: string; inputFingerprint?: string; layer: string; model?: string; outputFingerprint?: string; productionEligible: boolean; provider?: string; revision?: string; schemaVersion: number; staleReason?: string; status: string; toolVersion?: string } }; ip: { assetPack: { contentHash: string; id: string; version: string }; backgroundMode?: string; backgroundTemplate?: string; displayMode?: string; expression?: string; framingPreset?: string; gestureEvents?: { atMs: number; endMs?: number; id?: string; timelineRevision: string; type: string; value?: string }[]; lipSyncRevision?: string; pose?: string; state: { artifactRef?: string; createdAt?: string; dependencies?: { artifactId?: string; fingerprint?: string; layer?: string; revision: string }[]; executionMode?: string; inputFingerprint?: string; layer: string; model?: string; outputFingerprint?: string; productionEligible: boolean; provider?: string; revision?: string; schemaVersion: number; staleReason?: string; status: string; toolVersion?: string } }; needsHumanReview?: boolean; schemaVersion: number; text: { projectRef?: string; renderer: string; state: { artifactRef?: string; createdAt?: string; dependencies?: { artifactId?: string; fingerprint?: string; layer?: string; revision: string }[]; executionMode?: string; inputFingerprint?: string; layer: string; model?: string; outputFingerprint?: string; productionEligible: boolean; provider?: string; revision?: string; schemaVersion: number; staleReason?: string; status: string; toolVersion?: string }; textLayers?: { animation?: string; background?: string; color?: string; endSec: number; fontSize?: number; fontWeight?: string; id: string; language?: string; mustBeExact: boolean; position?: string; role: string; startSec: number; text: string }[]; timelineRevision: string }; timelineRevision: string; visualMode: string; visualModeConfidence?: number; visualModeReason?: string } | null; timelineRevision?: string; title: string; transitionIn?: string; transitionOut?: string; updatedAt: string; version: number; videoType?: string; visualChangeLevel: string; visualChangeReason?: string; visualPlan?: { background?: { description?: string; requiresAigc?: boolean }; cameraPlan?: { description?: string; movement?: string; requiresAigc?: boolean }; canvas: { aspectRatio: string; durationSec: number; fps: number; height: number; width: number }; characters?: { description?: string; emotion?: string; id: string; motion?: string; requiresAigc?: boolean }[]; constraints?: { mustAvoid?: string[]; mustInclude?: string[] }; dataVisuals?: { description?: string; id: string; type?: string }[]; motionPlan?: { description?: string; requiresAigc?: boolean }; props?: { description?: string; id: string }[]; style?: { description?: string; tags?: string[] }; textLayers?: { animation?: string; background?: string; color?: string; endSec: number; fontSize?: number; fontWeight?: string; id: string; language?: string; mustBeExact: boolean; position?: string; role: string; startSec: number; text: string }[]; transitionIn?: string; transitionOut?: string; uiLayers?: { description?: string; id: string }[] } })[] };
  message?: string;
}

/**  */
// ShotUnitResponse
export interface ShotUnitResponse {
  code?: number;
  data?: { shot?: { acceptedCandidateId?: string; action?: string; artifactRefs?: { aigcBackgroundVideoArtifactId?: string; compositedShotVideoArtifactId?: string; htmlOverlayVideoArtifactId?: string; htmlPreviewVideoArtifactId?: string; htmlSourceArtifactId?: string; keyframeImageArtifactId?: string; keyframePromptArtifactId?: string; renderStrategyArtifactId?: string; subtitleArtifactId?: string; videoClipArtifactId?: string; videoPromptArtifactId?: string; visualPlanArtifactId?: string }; camera?: string; candidates?: ({ artifactRefs?: { aigcBackgroundVideoArtifactId?: string; compositedShotVideoArtifactId?: string; htmlOverlayVideoArtifactId?: string; htmlPreviewVideoArtifactId?: string; htmlSourceArtifactId?: string; keyframeImageArtifactId?: string; keyframePromptArtifactId?: string; renderStrategyArtifactId?: string; subtitleArtifactId?: string; videoClipArtifactId?: string; videoPromptArtifactId?: string; visualPlanArtifactId?: string }; attemptIndex: number; candidateId: string; createdAt?: string; durationSec: number; executionMode?: string; fallbackReason?: string; generationPlanRevision?: string; inputFingerprint?: string; isFallback?: boolean; layerRevisions?: Record<string, string>; outputFingerprint?: string; productionEligible: boolean; qaReport?: { candidateId: string; failedDimensions?: string[]; humanApproved?: boolean; overallScore?: number; passed: boolean; passedDimensions?: string[]; reportRef?: string; scores?: Record<string, number>; severity?: string; shotId: string; status: string; summary?: string } | null; repairPlan?: { action: string; attemptIndex: number; lockedDimensions?: string[]; nextToolCall?: string; preserve: boolean; promptPatch?: Record<string, unknown>; reason?: string; renderStrategyPatch?: Record<string, unknown>; repairTargets?: string[]; severity?: string; sourceCandidateId?: string; targetShotId?: string } | null; schemaVersion?: number; shotId: string; sourceType?: string; stale?: boolean; staleReason?: string; status: string; timelineRevision?: string })[]; continuity: { characters?: string[]; endState?: string; mustMatchNext?: boolean; mustMatchPrevious?: boolean; previousState?: string; props?: string[]; styleTags?: string[] }; createdAt: string; durationMs?: number; durationSec: number; endMs?: number; endSec?: number; focalLengthHint?: string; framing?: string; id: string; lastRejectReason?: string; locked: boolean; mainAction?: string; narration?: string; projectId: string; promptConstraints: { mustAvoid?: string[]; mustInclude?: string[] }; qaStatus?: string; renderStrategy?: { aigcInput?: { durationSec?: number; negativePrompt?: string; prompt?: string } | null; aigcRequired: boolean; compositePlan?: { backgroundArtifactId?: string; outputArtifactType?: string; overlayArtifactId?: string } | null; htmlInput?: { durationSec?: number; textLayers?: { animation?: string; background?: string; color?: string; endSec: number; fontSize?: number; fontWeight?: string; id: string; language?: string; mustBeExact: boolean; position?: string; role: string; startSec: number; text: string }[] } | null; htmlRequired: boolean; mode: string; needsCompositing: boolean; primaryTool?: string; reason?: string; secondaryTools?: string[]; textOverlayNeeded: boolean }; repairPlans?: { action: string; attemptIndex: number; lockedDimensions?: string[]; nextToolCall?: string; preserve: boolean; promptPatch?: Record<string, unknown>; reason?: string; renderStrategyPatch?: Record<string, unknown>; repairTargets?: string[]; severity?: string; sourceCandidateId?: string; targetShotId?: string }[]; reviewStatus: string; scene?: string; sceneId?: string; sceneSummary?: string; schemaVersion?: number; screenText?: string[]; scriptSegmentId?: string; sequenceIndex: number; shotSize?: string; singleScene: boolean; stale: boolean; startMs?: number; startSec?: number; subject?: string; talkingHeadLayers?: { audio: { audioMasterRevision: string; state: { artifactRef?: string; createdAt?: string; dependencies?: { artifactId?: string; fingerprint?: string; layer?: string; revision: string }[]; executionMode?: string; inputFingerprint?: string; layer: string; model?: string; outputFingerprint?: string; productionEligible: boolean; provider?: string; revision?: string; schemaVersion: number; staleReason?: string; status: string; toolVersion?: string }; voiceProfileId?: string; voiceProfileVersion?: string; voiceoverArtifactRef?: string }; broll: { entries?: { artifactRef?: string; assetType: string; endMs: number; fit?: string; generated: boolean; id: string; licenseStatus: string; model?: string; narrationText?: string; placement?: string; provenance?: { artifactId?: string; candidateId?: string; fallbackReason?: string; generatedAt?: string; inputPromptHash?: string; isFallback: boolean; kind?: string; providerJobId?: string; providerName?: string; shotId?: string; sourceArtifactIds?: string[]; sourceType: string }; provider?: string; relevanceScore?: number; replacementHistory?: string[]; reviewStatus: string; seed?: string; semanticPurpose: string; shotId: string; sourceType: string; sourceUri?: string; startMs: number; usageStatus?: string }[]; manifestRef?: string; state: { artifactRef?: string; createdAt?: string; dependencies?: { artifactId?: string; fingerprint?: string; layer?: string; revision: string }[]; executionMode?: string; inputFingerprint?: string; layer: string; model?: string; outputFingerprint?: string; productionEligible: boolean; provider?: string; revision?: string; schemaVersion: number; staleReason?: string; status: string; toolVersion?: string } }; composition: { assembler?: string; baseArtifactRef?: string; outputRequirements?: Record<string, unknown>; overlayArtifactRefs?: string[]; state: { artifactRef?: string; createdAt?: string; dependencies?: { artifactId?: string; fingerprint?: string; layer?: string; revision: string }[]; executionMode?: string; inputFingerprint?: string; layer: string; model?: string; outputFingerprint?: string; productionEligible: boolean; provider?: string; revision?: string; schemaVersion: number; staleReason?: string; status: string; toolVersion?: string } }; ip: { assetPack: { contentHash: string; id: string; version: string }; backgroundMode?: string; backgroundTemplate?: string; displayMode?: string; expression?: string; framingPreset?: string; gestureEvents?: { atMs: number; endMs?: number; id?: string; timelineRevision: string; type: string; value?: string }[]; lipSyncRevision?: string; pose?: string; state: { artifactRef?: string; createdAt?: string; dependencies?: { artifactId?: string; fingerprint?: string; layer?: string; revision: string }[]; executionMode?: string; inputFingerprint?: string; layer: string; model?: string; outputFingerprint?: string; productionEligible: boolean; provider?: string; revision?: string; schemaVersion: number; staleReason?: string; status: string; toolVersion?: string } }; needsHumanReview?: boolean; schemaVersion: number; text: { projectRef?: string; renderer: string; state: { artifactRef?: string; createdAt?: string; dependencies?: { artifactId?: string; fingerprint?: string; layer?: string; revision: string }[]; executionMode?: string; inputFingerprint?: string; layer: string; model?: string; outputFingerprint?: string; productionEligible: boolean; provider?: string; revision?: string; schemaVersion: number; staleReason?: string; status: string; toolVersion?: string }; textLayers?: { animation?: string; background?: string; color?: string; endSec: number; fontSize?: number; fontWeight?: string; id: string; language?: string; mustBeExact: boolean; position?: string; role: string; startSec: number; text: string }[]; timelineRevision: string }; timelineRevision: string; visualMode: string; visualModeConfidence?: number; visualModeReason?: string } | null; timelineRevision?: string; title: string; transitionIn?: string; transitionOut?: string; updatedAt: string; version: number; videoType?: string; visualChangeLevel: string; visualChangeReason?: string; visualPlan?: { background?: { description?: string; requiresAigc?: boolean }; cameraPlan?: { description?: string; movement?: string; requiresAigc?: boolean }; canvas: { aspectRatio: string; durationSec: number; fps: number; height: number; width: number }; characters?: { description?: string; emotion?: string; id: string; motion?: string; requiresAigc?: boolean }[]; constraints?: { mustAvoid?: string[]; mustInclude?: string[] }; dataVisuals?: { description?: string; id: string; type?: string }[]; motionPlan?: { description?: string; requiresAigc?: boolean }; props?: { description?: string; id: string }[]; style?: { description?: string; tags?: string[] }; textLayers?: { animation?: string; background?: string; color?: string; endSec: number; fontSize?: number; fontWeight?: string; id: string; language?: string; mustBeExact: boolean; position?: string; role: string; startSec: number; text: string }[]; transitionIn?: string; transitionOut?: string; uiLayers?: { description?: string; id: string }[] } } };
  message?: string;
}

/**  */
// ShotWorkspace
export interface ShotWorkspace {
  history: ShotRevision[];
  impact: ShotImpact;
  shot: ShotUnit;
}

/**  */
// ShotWorkspaceResponse
export interface ShotWorkspaceResponse {
  code: number;
  data: { workspace: ShotWorkspace };
  message: string;
}

/**  */
// SkillCapabilityDetailResponse
export interface SkillCapabilityDetailResponse {
  code?: number;
  data?: { capability?: { activation: { intents?: string[]; keywords?: string[] }; description: string; domain: string; id: string; loadError?: string; name: string; recipe: { mode?: string; path?: string }; resources: { id: string; loadStrategy?: string; path: string; priority?: number; scope?: string; type: string }[]; roleAgents?: ({ allowedTools?: string[]; displayName: string; forbiddenTools?: string[]; goal: string; humanReview?: { gate?: string; required: boolean; reviewFocus?: string[]; title?: string; userActions?: string[] } | null; id: string; maxToolCalls?: number; name: string; qualityPolicy?: { autoRepair?: boolean; checkerTool?: string; maxRepairAttempts?: number; minScore?: number; repairTool?: string; required: boolean } | null; requiredInputs?: string[]; requiredOutputs?: string[]; stage: string })[]; rootPath?: string; status: string; tools: { id: string; manifest: string; prompt?: string }[]; version: string } };
  message?: string;
}

/**  */
// SkillCapabilityListResponse
export interface SkillCapabilityListResponse {
  code?: number;
  data?: { capabilities?: ({ activation: { intents?: string[]; keywords?: string[] }; description: string; domain: string; id: string; loadError?: string; name: string; recipe: { mode?: string; path?: string }; resources: { id: string; loadStrategy?: string; path: string; priority?: number; scope?: string; type: string }[]; roleAgents?: ({ allowedTools?: string[]; displayName: string; forbiddenTools?: string[]; goal: string; humanReview?: { gate?: string; required: boolean; reviewFocus?: string[]; title?: string; userActions?: string[] } | null; id: string; maxToolCalls?: number; name: string; qualityPolicy?: { autoRepair?: boolean; checkerTool?: string; maxRepairAttempts?: number; minScore?: number; repairTool?: string; required: boolean } | null; requiredInputs?: string[]; requiredOutputs?: string[]; stage: string })[]; rootPath?: string; status: string; tools: { id: string; manifest: string; prompt?: string }[]; version: string })[] };
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
// StepConfirmRequest
export interface StepConfirmRequest {
  artifactId: string;
  comment?: string;
  reviewId?: string;
  runId?: string;
}

/**  */
// StepImpact
export interface StepImpact {
  affectedShotIds?: string[];
  affectedStepIds: ('requirements' | 'direction' | 'script' | 'shots' | 'preview' | 'delivery')[];
  requiresConfirmation: boolean;
}

/**  */
// StepImpactResponse
export interface StepImpactResponse {
  code: number;
  data: StepImpact;
  message: string;
}

/**  */
// StepMutationResponse
export interface StepMutationResponse {
  code: number;
  data: { artifact: { contentHash: string; createdAt: string; dependsOn?: string[]; humanApproved: boolean; id: string; inlineJson?: string; isCurrent: boolean; kind: string; metadata?: Record<string, unknown>; mimeType?: string; model?: string; name: string; parentId?: string; producedByNode?: string; producedByRole?: string; producedByTool?: string; projectId: string; promptHash?: string; provider?: string; roleAgentId?: string; sizeBytes: number; stageName: string; status: string; storageRef?: string; storageType: string; taskId?: string; unitId?: string; updatedAt: string; version: number; workflowRunId?: string }; impact: StepImpact; view: CreationView };
  message: string;
}

/**  */
// StepRestoreRequest
export interface StepRestoreRequest {
  baseVersion: number;
  confirmedAffectedShotIds: string[];
  reason?: string;
  reviewId?: string;
  runId?: string;
}

/**  */
// StepRevisionMutationRequest
export type StepRevisionMutationRequest = { artifactId: string; baseVersion: number; confirmedAffectedShotIds: string[]; directContent: string; instruction?: never; mode: 'direct'; reviewId?: string; runId?: string; selection?: ArtifactSelection } | { artifactId: string; baseVersion: number; confirmedAffectedShotIds: string[]; directContent?: never; instruction: string; mode: 'instruction'; reviewId?: string; runId?: string; selection?: ArtifactSelection };

/**  */
// StepRevisionPreviewRequest
export interface StepRevisionPreviewRequest {
  artifactId: string;
  baseVersion: number;
}

/**  */
// StepVersionsResponse
export interface StepVersionsResponse {
  code: number;
  data: { versions: CreatorArtifactVersion[] };
  message: string;
}

/**  */
// TaskCreateResponse
export interface TaskCreateResponse {
  createdAt: string;
  id: string;
  input?: Record<string, unknown>;
  output?: Record<string, unknown>;
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
// TextLayersResponse
export interface TextLayersResponse {
  code?: number;
  data?: { textLayers?: { animation?: string; background?: string; color?: string; endSec: number; fontSize?: number; fontWeight?: string; id: string; language?: string; mustBeExact: boolean; position?: string; role: string; startSec: number; text: string }[] };
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
// VideoAssistantExplainStageRequest
export interface VideoAssistantExplainStageRequest {
  stage: string;
}

/**  */
// VideoAssistantExplainStageResponse
export interface VideoAssistantExplainStageResponse {
  code?: number;
  data?: { betaLimitations: string[]; displayName: string; explanation: string; nextUserActions: string[]; projectId: string; requiredInputs: string[]; requiredOutputs: string[]; reviewFocus: string[]; stage: string };
  message?: string;
}

/**  */
// VideoAssistantMessageRequest
export interface VideoAssistantMessageRequest {
  artifactIds?: string[];
  message: string;
  runId?: string;
  stage?: string;
}

/**  */
// VideoAssistantMessageResponse
export interface VideoAssistantMessageResponse {
  code?: number;
  data?: { answer: string; forbiddenCapabilities: string[]; projectId: string; referencedArtifactIds?: string[]; runId?: string; scope: string; stage?: string; suggestedActions: { body?: Record<string, unknown>; label: string; method?: string; path?: string; type: string }[] };
  message?: string;
}

/**  */
// VideoAssistantReviseRequest
export interface VideoAssistantReviseRequest {
  artifactId: string;
  message: string;
  reviewId?: string;
  runId?: string;
}

/**  */
// VideoAssistantReviseResponse
export interface VideoAssistantReviseResponse {
  code?: number;
  data?: { answer: string; artifactAction: { body?: Record<string, unknown>; label: string; method?: string; path?: string; type: string }; artifactId: string; bypassesArtifact: boolean; projectId: string; reviewId?: string; runId?: string };
  message?: string;
}

/**  */
// VideoCreationSpec
export interface VideoCreationSpec {
  aspectRatio: string;
  audience?: string;
  createdAt: string;
  id: string;
  language: string;
  platform?: string;
  projectId: string;
  renderPreference: { allowHybridRender: boolean; defaultRenderStrategy: string; preferAIGCForPeople: boolean; preferAIGCForScene: boolean; preferHTMLForCharts: boolean; preferHTMLForText: boolean; preferHTMLForUI: boolean; preferLowCostPreview: boolean };
  reviewMode: string;
  shotPolicy: { avoidCrossShotDependency: boolean; lowVisualChangeRequired: boolean; maxDurationSec: number; minDurationSec: number; preferDurationSec: number; preferredMaxDurationSec: number; preferredMinDurationSec: number; singleSceneRequired: boolean; splitByScriptSemantics: boolean; splitByVisualChange: boolean };
  sourceMessage: string;
  status: string;
  targetDurationSec?: number;
  tone?: string;
  topic?: string;
  updatedAt: string;
  videoType?: string;
  visualStyle?: string;
}

/**  */
// VideoCreationSpecResponse
export interface VideoCreationSpecResponse {
  code?: number;
  data?: { spec?: { aspectRatio: string; audience?: string; createdAt: string; id: string; language: string; platform?: string; projectId: string; renderPreference: { allowHybridRender: boolean; defaultRenderStrategy: string; preferAIGCForPeople: boolean; preferAIGCForScene: boolean; preferHTMLForCharts: boolean; preferHTMLForText: boolean; preferHTMLForUI: boolean; preferLowCostPreview: boolean }; reviewMode: string; shotPolicy: { avoidCrossShotDependency: boolean; lowVisualChangeRequired: boolean; maxDurationSec: number; minDurationSec: number; preferDurationSec: number; preferredMaxDurationSec: number; preferredMinDurationSec: number; singleSceneRequired: boolean; splitByScriptSemantics: boolean; splitByVisualChange: boolean }; sourceMessage: string; status: string; targetDurationSec?: number; tone?: string; topic?: string; updatedAt: string; videoType?: string; visualStyle?: string } };
  message?: string;
}

/**  */
// VideoPreflightResponse
export interface VideoPreflightResponse {
  blockers?: { code: string; message: string }[];
  canStart: boolean;
  canonicalProfileId: string;
  capabilityMenu: { compositionRuntime: { hyperframes: { available: boolean; reason?: string } }; localRunner: { available: boolean; reason?: string; runnerId?: string }; localTools: { available: boolean; command: string }[]; warnings: string[] };
  pipeline: string;
  runtimePipelineId: string;
  runtimePipelineSource: string;
  runtimePipelineVersion: string;
  status: string;
}

/**  */
// VideoProject
export interface VideoProject {
  aspectRatio?: string;
  canonicalProfileId?: string;
  config?: Record<string, unknown>;
  configRevision: number;
  createdAt: string;
  currentRunId?: string;
  deletedAt?: string | null;
  description?: string;
  generationMode: 'provider_api' | 'manual_import';
  id: string;
  language?: string;
  localPathHint?: string;
  mode: 'aigc_shot' | 'voice_visual' | 'cinematic_story';
  name: string;
  skillName: string;
  skillVersion: string;
  status: 'DRAFT' | 'RUNNING' | 'PAUSED' | 'COMPLETED' | 'ARCHIVED';
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
  data?: { project?: { aspectRatio?: string; canonicalProfileId?: string; config?: unknown; configRevision: number; createdAt: string; currentRunId?: string; deletedAt?: string | null; description?: string; generationMode: string; id: string; language?: string; localPathHint?: string; mode: string; name: string; skillName: string; skillVersion: string; status: string; targetDurationSec?: number; updatedAt: string; userId: string; workflowName: string; workflowVersion: string } };
  message?: string;
}

/**  */
// VideoProjectDetailResponse
export interface VideoProjectDetailResponse {
  code?: number;
  data?: { project?: { aspectRatio?: string; canonicalProfileId?: string; config?: unknown; configRevision: number; createdAt: string; currentRunId?: string; deletedAt?: string | null; description?: string; generationMode: string; id: string; language?: string; localPathHint?: string; mode: string; name: string; skillName: string; skillVersion: string; status: string; targetDurationSec?: number; updatedAt: string; userId: string; workflowName: string; workflowVersion: string } };
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
// VideoRoleAgentDetailResponse
export interface VideoRoleAgentDetailResponse {
  code?: number;
  data?: { roleAgent?: { allowedTools?: string[]; displayName: string; forbiddenTools?: string[]; goal: string; humanReview?: { gate?: string; required: boolean; reviewFocus?: string[]; title?: string; userActions?: string[] } | null; id: string; maxToolCalls?: number; name: string; qualityPolicy?: { autoRepair?: boolean; checkerTool?: string; maxRepairAttempts?: number; minScore?: number; repairTool?: string; required: boolean } | null; requiredInputs?: string[]; requiredOutputs?: string[]; stage: string } };
  message?: string;
}

/**  */
// VideoRoleAgentListResponse
export interface VideoRoleAgentListResponse {
  code?: number;
  data?: { roleAgents?: ({ allowedTools?: string[]; displayName: string; forbiddenTools?: string[]; goal: string; humanReview?: { gate?: string; required: boolean; reviewFocus?: string[]; title?: string; userActions?: string[] } | null; id: string; maxToolCalls?: number; name: string; qualityPolicy?: { autoRepair?: boolean; checkerTool?: string; maxRepairAttempts?: number; minScore?: number; repairTool?: string; required: boolean } | null; requiredInputs?: string[]; requiredOutputs?: string[]; stage: string })[] };
  message?: string;
}

/**  */
// VisualPlan
export interface VisualPlan {
  background?: { description?: string; requiresAigc?: boolean };
  cameraPlan?: { description?: string; movement?: string; requiresAigc?: boolean };
  canvas: { aspectRatio: string; durationSec: number; fps: number; height: number; width: number };
  characters?: { description?: string; emotion?: string; id: string; motion?: string; requiresAigc?: boolean }[];
  constraints?: { mustAvoid?: string[]; mustInclude?: string[] };
  dataVisuals?: { description?: string; id: string; type?: string }[];
  motionPlan?: { description?: string; requiresAigc?: boolean };
  props?: { description?: string; id: string }[];
  style?: { description?: string; tags?: string[] };
  textLayers?: { animation?: string; background?: string; color?: string; endSec: number; fontSize?: number; fontWeight?: string; id: string; language?: string; mustBeExact: boolean; position?: string; role: string; startSec: number; text: string }[];
  transitionIn?: string;
  transitionOut?: string;
  uiLayers?: { description?: string; id: string }[];
}

/**  */
// VisualPlanResponse
export interface VisualPlanResponse {
  code?: number;
  data?: { visualPlan?: { background?: { description?: string; requiresAigc?: boolean }; cameraPlan?: { description?: string; movement?: string; requiresAigc?: boolean }; canvas: { aspectRatio: string; durationSec: number; fps: number; height: number; width: number }; characters?: { description?: string; emotion?: string; id: string; motion?: string; requiresAigc?: boolean }[]; constraints?: { mustAvoid?: string[]; mustInclude?: string[] }; dataVisuals?: { description?: string; id: string; type?: string }[]; motionPlan?: { description?: string; requiresAigc?: boolean }; props?: { description?: string; id: string }[]; style?: { description?: string; tags?: string[] }; textLayers?: { animation?: string; background?: string; color?: string; endSec: number; fontSize?: number; fontWeight?: string; id: string; language?: string; mustBeExact: boolean; position?: string; role: string; startSec: number; text: string }[]; transitionIn?: string; transitionOut?: string; uiLayers?: { description?: string; id: string }[] } };
  message?: string;
}

/**  */
// WorkflowCheckpointListResponse
export interface WorkflowCheckpointListResponse {
  code?: number;
  data?: { checkpoints?: ({ createdAt: string; id: string; nodeId: string; recoveredAt?: string | null; snapshot?: unknown; stageIndex: number; stageName: string; state: string; taskId: string; workflowRunId: string })[] };
  message?: string;
}

/**  */
// WorkflowCheckpointRecoverResponse
export interface WorkflowCheckpointRecoverResponse {
  code?: number;
  data?: { checkpoint?: { createdAt: string; id: string; nodeId: string; recoveredAt?: string | null; snapshot?: unknown; stageIndex: number; stageName: string; state: string; taskId: string; workflowRunId: string }; message?: string };
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
  dag: unknown;
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
