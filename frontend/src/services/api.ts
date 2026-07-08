import axios from 'axios'
import { getAuthAccessToken, refreshAuthSession, logout } from './auth'
import { getElectronAPI } from '../utils/electron'
import { readLocalBiaoshuArtifact } from './localAgent'
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
  ArtifactContentResponse,
  ArtifactHistoryResponse,
  AgentStartRunRequest,
  AgentStartRunResponse,
  AgentRun,
  AgentReviewListResponse,
  AgentReviewActionResponse,
} from '../utils/types'

const configuredCloudBase = import.meta.env.VITE_CLOUD_API_BASE || import.meta.env.VITE_API_BASE
const electronCloudBase = typeof window !== 'undefined' ? window.electronAPI?.runtimeConfig?.cloudApiBase : ''
const API_BASE = configuredCloudBase || electronCloudBase || '/api'

const DEFAULT_API_TIMEOUT_MS = 120000
const BIAOSHU_OUTLINE_TIMEOUT_MS = 180000

const api = axios.create({
  baseURL: API_BASE,
  timeout: DEFAULT_API_TIMEOUT_MS,
})

const apiErrorMessage = (error: unknown): string | null => {
  if (!axios.isAxiosError(error)) return null
  const data = error.response?.data
  if (!data || typeof data !== 'object') return null
  const body = data as { message?: unknown; error?: unknown; data?: unknown }
  if (typeof body.message === 'string' && body.message.trim()) return body.message
  if (typeof body.error === 'string' && body.error.trim()) return body.error
  if (body.data && typeof body.data === 'object') {
    const nested = body.data as { message?: unknown; error?: unknown }
    if (typeof nested.message === 'string' && nested.message.trim()) return nested.message
    if (typeof nested.error === 'string' && nested.error.trim()) return nested.error
  }
  return null
}

const outlineTimeoutMessage = (error: unknown): string | null => {
  if (!axios.isAxiosError(error)) return null
  if (error.code !== 'ECONNABORTED') return null
  return '生成大纲耗时过长，模型服务未在 180 秒内返回。请稍后重试，或先精简招标文件解析报告后再生成。'
}

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
    const message = apiErrorMessage(error)
    if (message && error instanceof Error) {
      error.message = message
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

// ── Biaoshu Artifact AI Revision ──

export interface ReferenceArtifact {
  kind: string
  name: string
  content: string
}

export interface BiaoshuReviseRequest {
  runId?: string
  artifactKind?: string
  artifactName?: string
  artifactContent: string
  userInstruction: string
  contextMessages?: { role: string; content: string }[]
  referenceArtifacts?: ReferenceArtifact[]
}

export interface BiaoshuReviseResponse {
  revisedContent: string
  summary: string
  model: string
}

export const reviseBiaoshuArtifact = async (
  payload: BiaoshuReviseRequest
): Promise<BiaoshuReviseResponse> => {
  const response = await api.post<ApiResponse<BiaoshuReviseResponse>>('/biaoshu/artifacts/revise', payload)
  return response.data.data
}

// ── Bid Analysis Report Generation ──

export interface GenerateBidAnalysisReportRequest {
  rawTextPath: string
  reportPath: string
  sourceFile?: string
  projectId?: string
  runId?: string
}

export interface GenerateBidAnalysisReportResponse {
  success: boolean
  data?: {
    rawTextPath: string
    reportPath: string
    artifact: Record<string, unknown>
  }
  error?: string
}

export const generateBidAnalysisReport = async (
  payload: GenerateBidAnalysisReportRequest
): Promise<GenerateBidAnalysisReportResponse> => {
  const response = await api.post<GenerateBidAnalysisReportResponse>('/biaoshu/bid-analysis-report/generate', payload)
  return response.data
}

// ── Project Context Questions ──

export type ProjectContextInputType =
  | 'text'
  | 'textarea'
  | 'single_choice'
  | 'multi_choice'
  | 'boolean'

export interface ProjectContextOption {
  value: string
  label: string
  description?: string
}

export interface ProjectContextQuestion {
  id: string
  category: string
  title: string
  prompt: string
  helpText?: string
  inputType: ProjectContextInputType
  required: boolean
  options?: ProjectContextOption[]
}

export interface GenerateProjectContextQuestionsRequest {
  analysisReportPath: string
  sourceFile?: string
}

export interface GenerateProjectContextQuestionsResponse {
  success: boolean
  data?: {
    analysisReportPath: string
    questions: ProjectContextQuestion[]
    markdown: string
  }
  error?: string
}

export const generateProjectContextQuestions = async (
  payload: GenerateProjectContextQuestionsRequest
): Promise<GenerateProjectContextQuestionsResponse> => {
  const response = await api.post<GenerateProjectContextQuestionsResponse>('/biaoshu/project-context/questions', payload)
  return response.data
}

// ── Project Context Report Generation ──

export interface GenerateProjectContextReportRequest {
  analysisReportPath: string
  contextAnswers: string
  contextReportPath: string
  sourceFile?: string
  projectId?: string
  runId?: string
}

export interface GenerateProjectContextReportResponse {
  success: boolean
  data?: {
    analysisReportPath: string
    contextReportPath: string
    artifact: Record<string, unknown>
  }
  error?: string
}

export const generateProjectContextReport = async (
  payload: GenerateProjectContextReportRequest
): Promise<GenerateProjectContextReportResponse> => {
  const response = await api.post<GenerateProjectContextReportResponse>('/biaoshu/project-context/report/generate', payload)
  return response.data
}

// ── Scoring Breakdown Generation ──

export interface GenerateScoringBreakdownRequest {
  analysisReportPath: string
  scoringReportPath: string
  sourceFile?: string
  projectId?: string
  runId?: string
}

export interface GenerateScoringBreakdownResponse {
  success: boolean
  data?: {
    analysisReportPath: string
    scoringReportPath: string
    artifact: Record<string, unknown>
  }
  error?: string
}

export const generateScoringBreakdown = async (
  payload: GenerateScoringBreakdownRequest
): Promise<GenerateScoringBreakdownResponse> => {
  const response = await api.post<GenerateScoringBreakdownResponse>('/biaoshu/scoring-breakdown/generate', payload)
  return response.data
}

// ── Outline Generation ──

export interface GenerateOutlineRequest {
  analysisReportPath: string
  contextReportPath: string
  scoringReportPath?: string
  outlinePath: string
  sourceFile?: string
  projectId?: string
  runId?: string
}

export interface GenerateOutlineResponse {
  success: boolean
  data?: {
    analysisReportPath: string
    contextReportPath: string
    outlinePath: string
    artifact: Record<string, unknown>
  }
  error?: string
}

export const generateOutline = async (
  payload: GenerateOutlineRequest
): Promise<GenerateOutlineResponse> => {
  try {
    const response = await api.post<GenerateOutlineResponse>('/biaoshu/outline/generate', payload, {
      timeout: BIAOSHU_OUTLINE_TIMEOUT_MS,
    })
    return response.data
  } catch (error) {
    const message = outlineTimeoutMessage(error)
    if (message && error instanceof Error) {
      error.message = message
    }
    throw error
  }
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

// ─── Biaoshu Tools ─────────────────────────────────

export interface BiaoshuArtifactContent {
  filePath?: string
  file_path: string
  format: string
  content: string
  size: number
}

const DEFAULT_BIAOSHU_TOOLS_BASE_URL = 'http://127.0.0.1:9001'

function getBiaoshuToolsBaseURL(): string {
  return (
    getElectronAPI()?.runtimeConfig?.biaoshuToolsUrl ||
    import.meta.env.VITE_BIAOSHU_TOOLS_URL ||
    DEFAULT_BIAOSHU_TOOLS_BASE_URL
  ).replace(/\/$/, '')
}

export const readBiaoshuArtifact = async (filePath: string): Promise<BiaoshuArtifactContent> => {
  try {
    const local = await readLocalBiaoshuArtifact(filePath)
    return {
      filePath: local.filePath,
      file_path: local.filePath,
      format: local.format,
      content: local.content,
      size: local.size,
    }
  } catch (localError) {
    try {
      const response = await axios.post(`${getBiaoshuToolsBaseURL()}/tools/read_artifact`, {
        params: { file_path: filePath },
      }, { timeout: 5000 })
      if (!response.data?.success) {
        throw new Error(response.data?.error || '读取产物失败')
      }
      return response.data.data
    } catch (toolError) {
      const localMessage = localError instanceof Error ? localError.message : String(localError)
      const toolMessage = toolError instanceof Error ? toolError.message : String(toolError)
      throw new Error(`读取产物失败：${localMessage}；标书工具服务也不可用：${toolMessage}`)
    }
  }
}


