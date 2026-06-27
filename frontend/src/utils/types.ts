// ── Note ────────────────────────────────────────────────────────────────
// api-types.generated.ts is regenerated from the OpenAPI spec via
//   cd ../cloud-backend && make gen-docs
//
// The hand-crafted types below MAY be more precise (enum unions, UI types).
// When the generated file drifts, the CI `make api-docs-check` fails and both
// should be reconciled.
//
// UI-only types: MediaFile, Platform, etc. (no spec equivalent).
// Data types: Artifact, VideoProject, etc. — keep these aligned manually.
// ─────────────────────────────────────────────────────────────────────────

export interface MediaFile {
  file: File
  preview?: string
  name: string
  size: number
}

export interface Platform {
  id: string
  name: string
  icon: string
  enabled: boolean
  status: 'available' | 'developing'
}

export interface ApiResponse<T> {
  code: number
  message: string
  data: T
}

export interface TaskResponse {
  taskId: string
  message: string
}

export interface AIGenerateData {
  title: string
  description: string
  body?: string
  keywords?: string[]
}

export interface AIPolishData {
  content: string
  taskId: string
  traceUrl: string
}

export interface PolishSubmitData {
  taskId: string
  nodeId: string
  message: string
  traceUrl: string
}

export interface PolishQueryData {
  taskId: string
  nodeId: string
  status: string
  content?: string
  error?: string
  traceUrl: string
}

export interface MediaAsset {
  id: string
  userId: string
  originalName: string
  mimeType: string
  size: number
  minioPath: string
  tags: string[]
  embeddingId?: string
  createdAt: string
  updatedAt: string
  url?: string
}

export interface MediaListResponse {
  items: MediaAsset[]
  total: number
  offset: number
  limit: number
}

export interface TraceNode {
  id: string
  taskId: string
  type: string
  name: string
  status: string
  input?: Record<string, unknown>
  output?: Record<string, unknown>
  errorMessage?: string
  retryCount: number
  createdAt: string
}

export interface TraceData {
  task: {
    taskId: string
    status: string
    input?: Record<string, unknown>
    output?: Record<string, unknown>
    createdAt: string
    nodes: TraceNode[]
  }
  contexts: Array<{
    id: number
    contextType: string
    taskId: string
    nodeId: string
    message: string
    metadata?: Record<string, unknown>
    createdAt: string
  }>
}

export type ContentType = 'image' | 'video' | null

export interface SkillStage {
  name: string
  kind?: string
  tool?: string
  instruction: string
  inputSchema?: string
  outputSchema?: string
  input?: Record<string, unknown>
  optional: boolean
  approvalRequired: boolean
  longRunning?: boolean
  heartbeatTimeoutSec?: number
}

export interface SkillRuntimeItem {
  name: string
  version: string
  displayName?: string
  description: string
  category: string
  visibility?: string
  canonicalSkill?: string
  stages: SkillStage[]
  health: 'HEALTHY' | 'UNHEALTHY' | 'DISABLED'
  loadedAt?: string
  loadError?: string
}

export interface SkillCatalogItem {
  name: string
  version: string
  displayName?: string
  description: string
  category: string
  visibility: string
  canonicalSkill?: string
  health: 'HEALTHY' | 'UNHEALTHY' | 'DISABLED'
  stageCount: number
  requiresApproval: boolean
  hasLongRunningStages: boolean
  loadError?: string
}

export interface SkillsResponse {
  skills: SkillRuntimeItem[]
  health: Record<string, string>
}

export interface SkillCatalogResponse {
  skills: SkillCatalogItem[]
  health: Record<string, string>
}

export interface SkillDetailResponse {
  skill: SkillRuntimeItem
}

export interface SkillRouteResponse {
  skill: SkillCatalogItem
  route: string
  deliverable: string
  aspectRatio: string
  targetDurationSec: number
  reasoning: string
  confidence: number
  source: string
}

export interface WorkflowNode {
  id: string
  name: string
  type: string
  input?: Record<string, unknown>
  longRunning?: boolean
  heartbeatTimeoutSec?: number
}

export interface WorkflowEdge {
  from: string
  to: string
}

export interface WorkflowDAG {
  nodes: WorkflowNode[]
  edges: WorkflowEdge[]
}

export interface WorkflowTemplate {
  id: string
  version: string
  name: string
  description?: string
  category?: string
  dag: WorkflowDAG
  createdAt: string
  updatedAt: string
}

export interface WorkflowListResponse {
  templates: WorkflowTemplate[]
}

export type VideoProjectMode = 'aigc_shot' | 'voice_visual'
export type VideoGenerationMode = 'provider_api' | 'manual_import'

export interface VideoProject {
  id: string
  userId: string
  name: string
  description?: string
  mode: VideoProjectMode
  status: 'DRAFT' | 'RUNNING' | 'PAUSED' | 'COMPLETED' | 'ARCHIVED'
  skillName: string
  skillVersion: string
  workflowName: string
  workflowVersion: string
  generationMode: VideoGenerationMode
  aspectRatio?: string
  targetDurationSec?: number
  language?: string
  config?: Record<string, unknown>
  currentRunId?: string
  localPathHint?: string
  createdAt: string
  updatedAt: string
}

export interface VideoProjectListResponse {
  projects: VideoProject[]
  total: number
}

export interface CreateVideoProjectPayload {
  name: string
  description?: string
  mode: VideoProjectMode
  skillName: string
  skillVersion: string
  workflowName: string
  workflowVersion: string
  generationMode: VideoGenerationMode
  aspectRatio?: string
  targetDurationSec?: number
  language?: string
  config?: Record<string, unknown>
}

export interface WorkflowRun {
  id: string
  projectId: string
  templateId: string
  templateVersion: string
  taskId: string
  status: string
  attempt: number
  input?: Record<string, unknown>
  output?: Record<string, unknown>
  stageStatuses?: Record<string, string>
  traceId?: string
  startedAt?: string
  finishedAt?: string
  createdAt: string
}

export interface CreateWorkflowRunPayload {
  templateId: string
  templateVersion: string
  input: Record<string, unknown>
}

export interface ApproveVideoStagePayload {
  runId?: string
  output?: Record<string, unknown>
  comment?: string
}

export interface ApproveVideoStageResponse {
  nodeId: string
  stage: string
  message: string
}

export type ArtifactKind = 'JSON' | 'MARKDOWN' | 'IMAGE' | 'AUDIO' | 'VIDEO' | 'BUNDLE' | 'LOG'

export interface Artifact {
  id: string
  projectId: string
  workflowRunId?: string
  stageName: string
  unitId?: string
  kind: ArtifactKind
  name: string
  version: number
  parentId?: string
  storageType: string
  storageRef?: string
  inlineJson?: string
  mimeType?: string
  sizeBytes: number
  contentHash: string
  promptHash?: string
  provider?: string
  model?: string
  isCurrent: boolean
  metadata?: Record<string, unknown>
  createdAt: string
}

export interface ArtifactListResponse {
  artifacts: Artifact[]
}

export interface ArtifactContentResponse {
  artifact: Artifact
  content: unknown
  mediaUrl?: string
  mediaUrls?: string[]
}

export interface ArtifactHistoryResponse {
  history: Artifact[]
}

// ── Agent Run Types ─────────────────────────────────────────────────────

export interface AgentBudget {
  maxLLMCalls?: number
  maxToolCalls?: number
  maxSteps?: number
  maxReplans?: number
  maxCostLevel?: string
}

export interface AgentStep {
  id: string
  intent?: string
  tool: string
  arguments: Record<string, unknown>
  dependsOn?: string[]
  expectedOutput?: string[]
  produceArtifact?: boolean
}

export interface AgentPlan {
  goal: string
  domain: string
  mode: string
  steps: AgentStep[]
  budget?: AgentBudget
  stopPolicy?: { stopWhenEnough?: boolean }
}

export interface AgentRun {
  id: string
  taskId?: string
  userId?: string
  domain?: string
  message: string
  plan?: AgentPlan
  status: 'CREATED' | 'RUNNING' | 'FAILED'
  budget?: AgentBudget
  createdAt: string
  updatedAt: string
  metadata?: Record<string, unknown>
}

export interface AgentStartRunRequest {
  message: string
  domain?: string
  context?: Record<string, unknown>
  mode?: string
  userId?: string
}

export interface AgentStartRunResponse {
  runId: string
  taskId: string
  status: string
  plan?: AgentPlan
}

export interface AgentReviewItem {
  id: string
  nodeId: string
  status: 'PENDING' | 'APPROVED' | 'REJECTED'
  stepId?: string
  tool?: string
  stage?: string
  roleAgentId?: string
  roleAgent?: Record<string, unknown>
  humanReview?: RoleHumanReview
  requiredInputs?: string[]
  requiredOutputs?: string[]
  reviewPhase?: string
  reviewReason?: string
  blocksDownstream?: boolean
  reviewArtifactKinds?: string[]
}

export interface AgentReviewListResponse {
  runId: string
  reviews: AgentReviewItem[]
}

export interface AgentReviewActionResponse {
  reviewId: string
  status: 'APPROVED' | 'REJECTED'
}

export interface RoleHumanReview {
  required?: boolean
  title?: string
  gate?: string
  reviewFocus?: string[]
  userActions?: string[]
}

export interface RoleQualityPolicy {
  required?: boolean
  checkerTool?: string
  minScore?: number
  autoRepair?: boolean
  repairTool?: string
  maxRepairAttempts?: number
}

export interface VideoRoleAgent {
  id: string
  name: string
  displayName: string
  stage: string
  goal: string
  requiredInputs?: string[]
  requiredOutputs?: string[]
  allowedTools?: string[]
  forbiddenTools?: string[]
  humanReview?: RoleHumanReview | null
  qualityPolicy?: RoleQualityPolicy | null
  maxToolCalls?: number
}

export interface VideoRoleAgentListResponse {
  roleAgents: VideoRoleAgent[]
}

export interface VideoAssistantAction {
  type: string
  label: string
  method?: string
  path?: string
  body?: Record<string, unknown>
}

export interface VideoAssistantMessageRequest {
  message: string
  stage?: string
  runId?: string
  artifactIds?: string[]
}

export interface VideoAssistantMessageResponse {
  projectId: string
  scope: 'video_project'
  answer: string
  stage?: string
  runId?: string
  suggestedActions: VideoAssistantAction[]
  forbiddenCapabilities: string[]
  referencedArtifactIds?: string[]
}

export interface VideoAssistantReviseRequest {
  artifactId: string
  message: string
  runId?: string
  reviewId?: string
}

export interface VideoAssistantReviseResponse {
  projectId: string
  artifactId: string
  runId?: string
  reviewId?: string
  answer: string
  bypassesArtifact: boolean
  artifactAction: VideoAssistantAction
}

export interface VideoAssistantExplainStageRequest {
  stage: string
}

export interface VideoAssistantExplainStageResponse {
  projectId: string
  stage: string
  displayName: string
  explanation: string
  requiredInputs: string[]
  requiredOutputs: string[]
  reviewFocus: string[]
  nextUserActions: string[]
  betaLimitations: string[]
}
