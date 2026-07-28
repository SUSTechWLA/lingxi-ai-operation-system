import axios from 'axios'
import { getAuthAccessToken, isDefinitiveAuthFailure, refreshAuthSession, logout } from './auth'
import {
  ApiResponse,
  SkillCatalogResponse,
  SkillDetailResponse,
  SkillRouteResponse,
  SkillsResponse,
  WorkflowListResponse,
  WorkflowTemplate,
  VideoProjectListResponse,
  CreateVideoProjectPayload,
  VideoProject,
  CreateWorkflowRunPayload,
  WorkflowRun,
  ApproveVideoStagePayload,
  ApproveVideoStageResponse,
  ArtifactListResponse,
  ArtifactContentResponse,
  ArtifactHistoryResponse,
  Artifact,
  AgentStartRunRequest,
  AgentStartRunResponse,
  AgentRun,
  AgentReviewListResponse,
  AgentReviewActionResponse,
  VideoRoleAgentListResponse,
  VideoAssistantExplainStageRequest,
  VideoAssistantExplainStageResponse,
  VideoAssistantMessageRequest,
  VideoAssistantMessageResponse,
  VideoAssistantReviseRequest,
  VideoAssistantReviseResponse,
} from '../utils/types'
import { unwrapApiData } from '../utils/apiResponse'

const configuredCloudBase = import.meta.env.VITE_CLOUD_API_BASE || import.meta.env.VITE_API_BASE
const electronCloudBase = typeof window !== 'undefined' ? window.electronAPI?.runtimeConfig?.cloudApiBase : ''
const API_BASE = configuredCloudBase || electronCloudBase || '/api'
const DEFAULT_API_TIMEOUT_MS = 30000
const AGENT_RUN_REQUEST_TIMEOUT_MS = 300000

export const api = axios.create({
  baseURL: API_BASE,
  timeout: DEFAULT_API_TIMEOUT_MS,
})

api.interceptors.request.use((config) => {
  const token = getAuthAccessToken()
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

let refreshPromise: Promise<unknown> | null = null

api.interceptors.response.use(
  (response) => response,
  async (error) => {
    const original = error.config
    if (error.response?.status === 401 && original && !original.__authRetry) {
      original.__authRetry = true
      try {
        refreshPromise = refreshPromise || refreshAuthSession()
        await refreshPromise
        refreshPromise = null
        const token = getAuthAccessToken()
        if (token) original.headers.Authorization = `Bearer ${token}`
        return api(original)
      } catch (refreshError) {
        refreshPromise = null
        if (isDefinitiveAuthFailure(refreshError)) logout()
        throw refreshError
      }
    }
    throw error
  }
)

// Video creator workbench compatibility API
export const fetchSkills = async (): Promise<SkillsResponse> => {
  const response = await api.get<ApiResponse<SkillsResponse>>('/skills')
  return response.data.data
}

export const fetchSkillCatalog = async (): Promise<SkillCatalogResponse> => {
  const response = await api.get<ApiResponse<SkillCatalogResponse>>('/skills/catalog')
  return response.data.data
}

export const fetchSkillDetail = async (
  name: string,
  version: string
): Promise<SkillDetailResponse> => {
  const response = await api.get<ApiResponse<SkillDetailResponse>>(`/skills/${name}/${version}`)
  return response.data.data
}

export const routeSkill = async (brief: string): Promise<SkillRouteResponse> => {
  const response = await api.post<ApiResponse<SkillRouteResponse>>('/skills/route', { brief })
  return response.data.data
}

export const fetchWorkflows = async (): Promise<WorkflowListResponse> => {
  const response = await api.get<ApiResponse<WorkflowListResponse>>('/workflows')
  return response.data.data
}

// fetchWorkflowTemplates returns all workflow templates (GET /api/workflows).
export const fetchWorkflowTemplates = async (): Promise<WorkflowTemplate[]> => {
  const response = await api.get<ApiResponse<{ templates: WorkflowTemplate[] }>>('/workflows')
  return response.data.data?.templates ?? []
}

// instantiateWorkflow creates a new workflow run from a template (POST /api/workflows/:id/instantiate).
export const instantiateWorkflow = async (
  templateId: string,
  overrides?: Record<string, unknown>
): Promise<{ taskId: string }> => {
  const response = await api.post<ApiResponse<{ taskId: string }>>(
    `/workflows/${templateId}/instantiate`,
    { overrides }
  )
  return response.data.data
}

export const fetchVideoProjects = async (signal?: AbortSignal): Promise<VideoProjectListResponse> => {
  const response = await api.get<ApiResponse<VideoProjectListResponse>>('/video-projects', { signal })
  return response.data.data
}

export const createVideoProject = async (
  payload: CreateVideoProjectPayload,
  signal?: AbortSignal,
): Promise<VideoProject> => {
  const response = await api.post<ApiResponse<{ project: VideoProject }>>('/video-projects', payload, { signal })
  return response.data.data.project
}

export const createWorkflowRun = async (
  projectId: string,
  payload: CreateWorkflowRunPayload
): Promise<WorkflowRun> => {
  const response = await api.post<ApiResponse<{ run: WorkflowRun }>>(
    `/video-projects/${projectId}/workflow-runs`,
    payload
  )
  return response.data.data.run
}

export const fetchWorkflowRun = async (
  projectId: string,
  runId: string
): Promise<WorkflowRun> => {
  const response = await api.get<ApiResponse<{ run: WorkflowRun }>>(
    `/video-projects/${projectId}/workflow-runs/${runId}`
  )
  return response.data.data.run
}

export const approveVideoStage = async (
  projectId: string,
  stageName: string,
  payload: ApproveVideoStagePayload
): Promise<ApproveVideoStageResponse> => {
  const response = await api.post<ApiResponse<ApproveVideoStageResponse>>(
    `/video-projects/${projectId}/stages/${stageName}/approve`,
    payload
  )
  return response.data.data
}

export const fetchProjectArtifacts = async (
  projectId: string,
  signal?: AbortSignal,
): Promise<ArtifactListResponse> => {
  const response = await api.get<ApiResponse<ArtifactListResponse>>(`/video-projects/${projectId}/artifacts`, { signal })
  return response.data.data
}

export const fetchProjectArtifactRegistry = async (
  projectId: string,
  signal?: AbortSignal,
): Promise<ArtifactListResponse> => {
  const response = await api.get<ApiResponse<ArtifactListResponse>>(`/video-projects/${projectId}/artifacts`, {
    params: { includeHistory: true },
    signal,
  })
  return response.data.data
}

export const fetchArtifactContent = async (
  artifactId: string,
  signal?: AbortSignal,
): Promise<ArtifactContentResponse> => {
  const response = await api.get<ApiResponse<ArtifactContentResponse>>(`/artifacts/${artifactId}/content`, { signal })
  return response.data.data
}

export const fetchArtifactHistory = async (
  artifactId: string,
  signal?: AbortSignal,
): Promise<ArtifactHistoryResponse> => {
  const response = await api.get<ApiResponse<ArtifactHistoryResponse>>(`/artifacts/${artifactId}/history`, { signal })
  return response.data.data
}

export const reviseArtifact = async (
  artifactId: string,
  message: string,
  modelProviders?: Record<string, unknown>
): Promise<ArtifactContentResponse> => {
  const response = await api.post<ApiResponse<ArtifactContentResponse>>(`/artifacts/${artifactId}/revise`, {
    message,
    ...(modelProviders ? { modelProviders } : {}),
  })
  return response.data.data
}

export interface RegisterExternalGenerationResultPayload {
  kind: 'image' | 'video'
  storageType?: string
  storageRef: string
  mimeType?: string
  sizeBytes?: number
  contentHash?: string
  promptHash?: string
  durationSec?: number
  description?: string
  tags?: string[]
  relatedShotId?: string
  source?: string
  generationRequestId?: string
  externalPlatform?: string
  referenceAssetIds?: string[]
}

export interface ExternalGenerationResultResponse {
  manifest: Record<string, unknown>
  artifact: Artifact
}

export const registerExternalGenerationResult = async (
  projectId: string,
  payload: RegisterExternalGenerationResultPayload
): Promise<ExternalGenerationResultResponse> => {
  const response = await api.post<ApiResponse<ExternalGenerationResultResponse>>(
    `/video-projects/${projectId}/external-generation-results`,
    payload
  )
  return response.data.data
}

// ── Agent Run API (dynamic agent runtime) ──────────────────────────────

export const startAgentRun = async (
  payload: AgentStartRunRequest,
  options: { idempotencyKey?: string; signal?: AbortSignal } = {},
): Promise<AgentStartRunResponse> => {
  const response = await api.post<ApiResponse<AgentStartRunResponse>>('/agent/runs', payload, {
    timeout: AGENT_RUN_REQUEST_TIMEOUT_MS,
    ...(options.idempotencyKey ? { headers: { 'Idempotency-Key': options.idempotencyKey } } : {}),
    signal: options.signal,
  })
  return response.data.data
}

export const getAgentRun = async (
  runId: string,
  signal?: AbortSignal,
): Promise<AgentRun> => {
  const response = await api.get<ApiResponse<{ run: AgentRun; task: unknown }>>(`/agent/runs/${runId}`, { signal })
  return response.data.data.run
}

export async function getTaskDetails(taskId: string, signal?: AbortSignal): Promise<unknown> {
  const response = await api.get<ApiResponse<unknown>>(`/task/${encodeURIComponent(taskId)}`, { signal })
  return response.data.data
}

export async function getTaskContext(taskId: string, signal?: AbortSignal): Promise<unknown[]> {
  const response = await api.get<ApiResponse<unknown[]>>(`/task/${encodeURIComponent(taskId)}/context`, { signal })
  return Array.isArray(response.data.data) ? response.data.data : []
}

export interface FailedAgentNodeCandidate {
  id: string
  status: string
  completedAt?: string | null
}

export interface RetryFailedAgentNodeResult {
  taskId: string
  nodeId: string
}

function completedAtTime(value?: string | null): number {
  if (!value) return 0
  const timestamp = Date.parse(value)
  return Number.isFinite(timestamp) ? timestamp : 0
}

export function selectLatestFailedAgentNode<T extends FailedAgentNodeCandidate>(
  nodes: readonly T[],
): T | undefined {
  return nodes
    .filter((node) => node.status === 'FAILED')
    .slice()
    .sort((left, right) => (
      completedAtTime(right.completedAt) - completedAtTime(left.completedAt) ||
      left.id.localeCompare(right.id)
    ))[0]
}

export const retryLatestFailedAgentNode = async (
  taskId: string,
  expectedNodeId?: string,
): Promise<RetryFailedAgentNodeResult> => {
  const response = await api.get<ApiResponse<{
    nodes?: Array<{ id: string; status: string; completedAt?: string | null }>
  }>>(`/task/${encodeURIComponent(taskId)}`)
  const failedNode = selectLatestFailedAgentNode(response.data.data.nodes ?? [])
  if (!failedNode) throw new Error('没有可重试的失败步骤')
  if (expectedNodeId && failedNode.id !== expectedNodeId) {
    throw new Error(`最新失败节点已从 ${expectedNodeId} 变为 ${failedNode.id}，请刷新快照后确认`)
  }
  await api.post(`/node/${encodeURIComponent(failedNode.id)}/retry`)
  return { taskId, nodeId: failedNode.id }
}

export const getAgentRunTrace = async (
  runId: string,
  signal?: AbortSignal,
): Promise<unknown> => {
  const response = await api.get<ApiResponse<unknown>>(`/agent/runs/${runId}/trace`, { signal })
  return response.data.data
}

export const getAgentRunReviews = async (
  runId: string,
  signal?: AbortSignal,
): Promise<AgentReviewListResponse> => {
  const response = await api.get<ApiResponse<AgentReviewListResponse>>(`/agent/runs/${runId}/reviews`, { signal })
  return response.data.data
}

export const cancelAgentRun = async (
  runId: string,
  projectId?: string,
  reason?: string
): Promise<AgentStartRunResponse> => {
  const response = await api.post<ApiResponse<AgentStartRunResponse>>(
    `/agent/runs/${runId}/cancel`,
    { projectId, reason }
  )
  return response.data.data
}

export const approveAgentReview = async (
  runId: string,
  reviewId: string,
  comment?: string
): Promise<AgentReviewActionResponse> => {
  const response = await api.post<ApiResponse<AgentReviewActionResponse>>(
    `/agent/runs/${runId}/reviews/${reviewId}/approve`,
    { comment }
  )
  return response.data.data
}

export const rejectAgentReview = async (
  runId: string,
  reviewId: string,
  comment?: string
): Promise<AgentReviewActionResponse> => {
  const response = await api.post<ApiResponse<AgentReviewActionResponse>>(
    `/agent/runs/${runId}/reviews/${reviewId}/reject`,
    { comment }
  )
  return response.data.data
}

export const submitEditedArtifact = async (
  runId: string,
  reviewId: string,
  content: unknown,
  comment?: string
): Promise<AgentReviewActionResponse> => {
  const response = await api.post<ApiResponse<AgentReviewActionResponse>>(
    `/agent/runs/${runId}/reviews/${reviewId}/submit-edited`,
    { content, comment }
  )
  return response.data.data
}

export const regenerateAgentStage = async (
  runId: string,
  reviewId: string,
  hint?: string
): Promise<AgentReviewActionResponse> => {
  const response = await api.post<ApiResponse<AgentReviewActionResponse>>(
    `/agent/runs/${runId}/reviews/${reviewId}/regenerate`,
    { hint }
  )
  return response.data.data
}

export interface CheckpointItem {
  id: string
  workflowRunId: string
  taskId: string
  stageName: string
  stageIndex: number
  nodeId: string
  state: 'AWAITING_HUMAN' | 'IN_PROGRESS' | 'COMPLETED'
  createdAt: string
  recoveredAt?: string
}

export const fetchCheckpoints = async (projectId: string, runId: string): Promise<CheckpointItem[]> => {
  const response = await api.get<ApiResponse<{ checkpoints: CheckpointItem[] }>>(
    `/video-projects/${projectId}/workflow-runs/${runId}/checkpoints`
  )
  return response.data.data?.checkpoints ?? []
}

export const recoverRun = async (
  projectId: string,
  runId: string
): Promise<{ checkpoint: CheckpointItem; message: string }> => {
  const response = await api.post<ApiResponse<{ checkpoint: CheckpointItem; message: string }>>(
    `/video-projects/${projectId}/workflow-runs/${runId}/recover`
  )
  return response.data.data!
}

export interface PreflightResponse {
  pipeline: string
  canonicalProfileId: 'talking_head' | 'cinematic_story'
  runtimePipelineId: string
  runtimePipelineVersion: string
  runtimePipelineSource: string
  status: 'passed' | 'blocked'
  canStart: boolean
  capabilityMenu: {
    localRunner: { available: boolean }
    compositionRuntime: { hyperframes: { available: boolean } }
    localTools: { command: string; available: boolean }[]
    warnings: string[]
  }
  blockers?: { code: string; message: string }[]
}

export const fetchVideoPreflight = async (pipeline: string = 'talking_head'): Promise<PreflightResponse> => {
  const response = await api.get<ApiResponse<PreflightResponse>>('/video/preflight', { params: { pipeline } })
  const preflight = unwrapApiData<PreflightResponse>(response.data)
  if (!preflight) throw new Error('invalid video preflight response')
  return preflight
}

export const fetchVideoRoleAgents = async (): Promise<VideoRoleAgentListResponse> => {
  const response = await api.get<ApiResponse<VideoRoleAgentListResponse>>('/video/role-agents')
  return response.data.data
}

export const askVideoProjectAssistant = async (
  projectId: string,
  payload: VideoAssistantMessageRequest
): Promise<VideoAssistantMessageResponse> => {
  const response = await api.post<ApiResponse<VideoAssistantMessageResponse>>(
    `/video-projects/${projectId}/assistant/message`,
    payload
  )
  return response.data.data
}

export const reviseWithVideoProjectAssistant = async (
  projectId: string,
  payload: VideoAssistantReviseRequest
): Promise<VideoAssistantReviseResponse> => {
  const response = await api.post<ApiResponse<VideoAssistantReviseResponse>>(
    `/video-projects/${projectId}/assistant/revise`,
    payload
  )
  return response.data.data
}

export const explainVideoProjectStage = async (
  projectId: string,
  payload: VideoAssistantExplainStageRequest
): Promise<VideoAssistantExplainStageResponse> => {
  const response = await api.post<ApiResponse<VideoAssistantExplainStageResponse>>(
    `/video-projects/${projectId}/assistant/explain-stage`,
    payload
  )
  return response.data.data
}
