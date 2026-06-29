import axios from 'axios'
import { getAuthAccessToken, refreshAuthSession, logout } from './auth'
import {
  ApiResponse,
  TaskResponse,
  AIGenerateData,
  AIPolishData,
  PolishSubmitData,
  PolishQueryData,
  TraceData,
  MediaListResponse,
  MediaAsset,
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

const configuredCloudBase = import.meta.env.VITE_CLOUD_API_BASE || import.meta.env.VITE_API_BASE
const electronCloudBase = typeof window !== 'undefined' ? window.electronAPI?.runtimeConfig?.cloudApiBase : ''
const API_BASE = configuredCloudBase || electronCloudBase || '/api'

const api = axios.create({
  baseURL: API_BASE,
  timeout: 30000,
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
        logout()
        throw refreshError
      }
    }
    throw error
  }
)

export const publishContent = async (
  title: string,
  description: string,
  keywords: string[],
  platforms: string[],
  videoFiles: File[],
  imageFiles: File[],
  contentType?: string,
  coverFile?: File | null,
  signal?: AbortSignal
): Promise<TaskResponse> => {
  const formData = new FormData()
  formData.append('title', title)
  formData.append('description', description)
  formData.append('keywords', JSON.stringify(keywords))
  formData.append('platforms', JSON.stringify(platforms))
  if (contentType) formData.append('content_type', contentType)

  videoFiles.forEach((file) => {
    formData.append('videos', file)
  })

  imageFiles.forEach((file) => {
    formData.append('images', file)
  })

  if (coverFile) {
    formData.append('cover', coverFile)
  }

  const response = await api.post<ApiResponse<TaskResponse>>('/publish', formData, {
    headers: {
      'Content-Type': 'multipart/form-data',
    },
    signal,
  })

  return response.data.data
}

export const aiGenerateContent = async (
  prompt: string,
  signal?: AbortSignal
): Promise<AIGenerateData> => {
  const response = await api.post<ApiResponse<AIGenerateData>>('/ai/generate', { prompt }, { signal })
  return response.data.data
}

export const aiGenerateFromMedia = async (
  prompt: string,
  images: File[],
  videos: File[],
  signal?: AbortSignal
): Promise<AIGenerateData> => {
  const formData = new FormData()
  formData.append('prompt', prompt)
  images.forEach((file) => formData.append('images', file))
  videos.forEach((file) => formData.append('videos', file))

  const response = await api.post<ApiResponse<AIGenerateData>>('/ai/generate-from-media', formData, {
    headers: { 'Content-Type': 'multipart/form-data' },
    signal,
    timeout: 180000, // 180s — video pipeline involves 2 LLM calls (visual analysis + copy generation)
  })
  return response.data.data
}

export const aiPolishText = async (
  text: string,
  type: 'title' | 'description',
  signal?: AbortSignal
): Promise<AIPolishData> => {
  const response = await api.post<ApiResponse<AIPolishData>>('/ai/polish', { text, type }, { signal })
  return response.data.data
}

export const fetchTrace = async (taskId: string): Promise<TraceData> => {
  const response = await api.get<ApiResponse<TraceData>>(`/trace/${taskId}`)
  return response.data.data
}

export const fetchRecentTrace = async (): Promise<TraceData> => {
  const response = await api.get<ApiResponse<TraceData>>('/trace/recent')
  return response.data.data
}

// Media management API
export const fetchMediaList = async (
  offset = 0,
  limit = 20,
  tag?: string
): Promise<MediaListResponse> => {
  const params: Record<string, string | number> = { offset, limit }
  if (tag) params.tag = tag
  const response = await api.get<ApiResponse<MediaListResponse>>('/media/list', { params })
  return response.data.data
}

export const fetchMediaDetail = async (id: string): Promise<MediaAsset> => {
  const response = await api.get<ApiResponse<MediaAsset>>(`/media/${id}`)
  return response.data.data
}

export const updateMediaTags = async (id: string, tags: string[]): Promise<MediaAsset> => {
  const response = await api.put<ApiResponse<MediaAsset>>(`/media/${id}/tags`, { tags })
  return response.data.data
}

export const uploadMedia = async (
  file: File,
  signal?: AbortSignal
): Promise<MediaAsset> => {
  const formData = new FormData()
  formData.append('file', file)
  const response = await api.post<ApiResponse<MediaAsset[]>>('/media/upload', formData, {
    headers: { 'Content-Type': 'multipart/form-data' },
    signal,
  })
  // Backend returns array of assets; we upload one file at a time
  const data = response.data.data
  if (Array.isArray(data) && data.length > 0) {
    return data[0]
  }
  return data as unknown as MediaAsset
}

export const batchProcessMedia = async (
  mediaIds: string[],
  action: 'analyze' | 'generate'
): Promise<{ taskIds: string[] }> => {
  const response = await api.post<ApiResponse<{ taskIds: string[] }>>('/media/batch-process', {
    media_ids: mediaIds,
    action,
  })
  return response.data.data
}

export const aiPolishSubmit = async (
  text: string,
  type: 'title' | 'description',
  signal?: AbortSignal
): Promise<PolishSubmitData> => {
  const response = await api.post<ApiResponse<PolishSubmitData>>('/ai/polish/submit', { text, type }, { signal })
  return response.data.data
}

export const queryPolishResult = async (
  taskId: string,
  nodeId: string,
  signal?: AbortSignal
): Promise<PolishQueryData> => {
  const response = await api.get<ApiResponse<PolishQueryData>>('/ai/polish/result', {
    params: { taskId, nodeId },
    signal,
  })
  return response.data.data
}

export const failNode = async (
  nodeId: string,
  errorMessage: string,
  signal?: AbortSignal
): Promise<void> => {
  await api.post(`/node/${nodeId}/failure`, { errorMessage }, { signal })
}

export const succeedNode = async (
  nodeId: string,
  output: Record<string, unknown>,
  signal?: AbortSignal
): Promise<void> => {
  await api.post(`/node/${nodeId}/success`, output, { signal })
}

export const failTask = async (
  taskId: string,
  signal?: AbortSignal
): Promise<void> => {
  await api.post(`/task/${taskId}/fail`, {}, { signal })
}

export const recordContextEvent = async (
  taskId: string,
  type: string,
  message: string,
  nodeId?: string
): Promise<void> => {
  await api.post('/context/record', { taskId, nodeId, type, message })
}

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

export const fetchVideoProjects = async (): Promise<VideoProjectListResponse> => {
  const response = await api.get<ApiResponse<VideoProjectListResponse>>('/video-projects')
  return response.data.data
}

export const createVideoProject = async (
  payload: CreateVideoProjectPayload
): Promise<VideoProject> => {
  const response = await api.post<ApiResponse<{ project: VideoProject }>>('/video-projects', payload)
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
  projectId: string
): Promise<ArtifactListResponse> => {
  const response = await api.get<ApiResponse<ArtifactListResponse>>(`/video-projects/${projectId}/artifacts`)
  return response.data.data
}

export const fetchArtifactContent = async (
  artifactId: string
): Promise<ArtifactContentResponse> => {
  const response = await api.get<ApiResponse<ArtifactContentResponse>>(`/artifacts/${artifactId}/content`)
  return response.data.data
}

export const fetchArtifactHistory = async (
  artifactId: string
): Promise<ArtifactHistoryResponse> => {
  const response = await api.get<ApiResponse<ArtifactHistoryResponse>>(`/artifacts/${artifactId}/history`)
  return response.data.data
}

export const reviseArtifact = async (
  artifactId: string,
  message: string
): Promise<ArtifactContentResponse> => {
  const response = await api.post<ApiResponse<ArtifactContentResponse>>(`/artifacts/${artifactId}/revise`, { message })
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

// ── Model Provider Config Sync ──

export interface ModelProviderSyncPayload {
  baseUrl: string
  apiKey?: string
  model: string
}

export const syncModelProviderConfig = async (
  payload: ModelProviderSyncPayload
): Promise<void> => {
  await api.put('/config/model-provider', payload)
}

export const clearModelProviderConfig = async (): Promise<void> => {
  await api.delete('/config/model-provider')
}

// ── Agent Run API (dynamic agent runtime) ──────────────────────────────

export const startAgentRun = async (
  payload: AgentStartRunRequest
): Promise<AgentStartRunResponse> => {
  const response = await api.post<ApiResponse<AgentStartRunResponse>>('/agent/runs', payload)
  return response.data.data
}

export const getAgentRun = async (
  runId: string
): Promise<AgentRun> => {
  const response = await api.get<ApiResponse<{ run: AgentRun; task: unknown }>>(`/agent/runs/${runId}`)
  return response.data.data.run
}

export const getAgentRunTrace = async (
  runId: string
): Promise<unknown> => {
  const response = await api.get<ApiResponse<unknown>>(`/agent/runs/${runId}/trace`)
  return response.data.data
}

export const getAgentRunReviews = async (
  runId: string
): Promise<AgentReviewListResponse> => {
  const response = await api.get<ApiResponse<AgentReviewListResponse>>(`/agent/runs/${runId}/reviews`)
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

export const fetchCheckpoints = async (runId: string): Promise<CheckpointItem[]> => {
  // Uses the video workflow endpoint; project ID is resolved server-side via the run.
  const response = await api.get<ApiResponse<{ checkpoints: CheckpointItem[] }>>(
    `/video-projects/checkpoints`, { params: { runId } }
  )
  return response.data.data?.checkpoints ?? []
}

export const recoverRun = async (runId: string): Promise<{ checkpoint: CheckpointItem; message: string }> => {
  const response = await api.post<ApiResponse<{ checkpoint: CheckpointItem; message: string }>>(
    `/video-projects/checkpoints/recover`, { runId }
  )
  return response.data.data!
}

export interface PreflightResponse {
  pipeline: string
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

export const fetchVideoPreflight = async (pipeline: string = 'wf-guided-image-text-video'): Promise<PreflightResponse> => {
  const response = await api.get<ApiResponse<PreflightResponse>>('/video/preflight', { params: { pipeline } })
  return response.data.data!
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
