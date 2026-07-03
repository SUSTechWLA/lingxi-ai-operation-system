import { getElectronAPI } from '../utils/electron'

export type ModelCapability = 'text_to_text' | 'text_to_image' | 'text_to_video'

export interface ModelProviderConfig {
  baseUrl: string
  model: string
  apiKey?: string
  hasApiKey?: boolean
  apiKeyPreview?: string
}

export interface ModelProviderSettingsResponse {
  providers: Partial<Record<ModelCapability, ModelProviderConfig>>
}

export type BiaoshuProjectStatus = 'CREATED' | 'RUNNING' | 'SUCCESS' | 'FAILED' | 'UNKNOWN'

export interface LocalBiaoshuProject {
  runId: string
  projectName: string
  bidFilePath: string
  status: BiaoshuProjectStatus
  createdAt: string
  updatedAt: string
  artifactCount?: number
  validArtifactCount?: number
}

export interface BiaoshuProjectListResponse {
  projects: LocalBiaoshuProject[]
}

export interface BiaoshuProjectUpsertResponse {
  project: LocalBiaoshuProject
  projects: LocalBiaoshuProject[]
}

export interface LocalBiaoshuArtifactContent {
  filePath: string
  format: string
  content: string
  size: number
}

export const DEFAULT_LOCAL_AGENT_URL = 'http://127.0.0.1:18080'
const configuredLocalAgentUrl = import.meta.env.VITE_LOCAL_AGENT_URL || import.meta.env.VITE_LOCAL_BACKEND_URL

export const DEFAULT_MODEL_PROVIDER_SETTINGS: Record<ModelCapability, ModelProviderConfig> = {
  text_to_text: {
    baseUrl: 'https://api.openai.com/v1',
    model: 'gpt-4.1',
    apiKey: '',
  },
  text_to_image: {
    baseUrl: 'https://api.openai.com/v1',
    model: 'gpt-image-1',
    apiKey: '',
  },
  text_to_video: {
    baseUrl: 'https://api.openai.com/v1',
    model: 'sora',
    apiKey: '',
  },
}

export function getLocalAgentBaseUrl(): string {
  const api = getElectronAPI()
  return configuredLocalAgentUrl || api?.runtimeConfig?.localAgentUrl || DEFAULT_LOCAL_AGENT_URL
}

export function mergeModelProviderSettings(
  providers?: Partial<Record<ModelCapability, ModelProviderConfig>>
): Record<ModelCapability, ModelProviderConfig> {
  return {
    text_to_text: { ...DEFAULT_MODEL_PROVIDER_SETTINGS.text_to_text, ...providers?.text_to_text },
    text_to_image: { ...DEFAULT_MODEL_PROVIDER_SETTINGS.text_to_image, ...providers?.text_to_image },
    text_to_video: { ...DEFAULT_MODEL_PROVIDER_SETTINGS.text_to_video, ...providers?.text_to_video },
  }
}

export async function fetchModelProviderSettings(): Promise<ModelProviderSettingsResponse> {
  const response = await fetch(localAgentUrl('/api/local/model-providers'))
  if (!response.ok) {
    throw new Error(await errorMessage(response, '读取模型设置失败'))
  }
  return response.json() as Promise<ModelProviderSettingsResponse>
}

export async function saveModelProviderSettings(
  providers: Record<ModelCapability, ModelProviderConfig>
): Promise<ModelProviderSettingsResponse> {
  const response = await fetch(localAgentUrl('/api/local/model-providers'), {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ providers }),
  })
  if (!response.ok) {
    throw new Error(await errorMessage(response, '保存模型设置失败'))
  }
  return response.json() as Promise<ModelProviderSettingsResponse>
}

export async function fetchBiaoshuProjects(): Promise<BiaoshuProjectListResponse> {
  const response = await fetch(localAgentUrl('/api/local/biaoshu-projects'))
  if (!response.ok) {
    throw new Error(await errorMessage(response, '读取历史标书项目失败'))
  }
  return response.json() as Promise<BiaoshuProjectListResponse>
}

export async function saveBiaoshuProject(
  project: LocalBiaoshuProject
): Promise<BiaoshuProjectUpsertResponse> {
  const response = await fetch(localAgentUrl(`/api/local/biaoshu-projects/${encodeURIComponent(project.runId)}`), {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(project),
  })
  if (!response.ok) {
    throw new Error(await errorMessage(response, '保存历史标书项目失败'))
  }
  return response.json() as Promise<BiaoshuProjectUpsertResponse>
}

export async function readLocalBiaoshuArtifact(filePath: string): Promise<LocalBiaoshuArtifactContent> {
  const response = await fetch(localAgentUrl('/api/local/biaoshu-artifacts/read'), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ filePath }),
  })
  if (!response.ok) {
    throw new Error(await errorMessage(response, '读取本地产物失败'))
  }
  return response.json() as Promise<LocalBiaoshuArtifactContent>
}

function localAgentUrl(path: string): string {
  return `${getLocalAgentBaseUrl().replace(/\/$/, '')}${path}`
}

async function errorMessage(response: Response, fallback: string): Promise<string> {
  try {
    const body = await response.json() as { error?: string }
    return body.error || fallback
  } catch {
    return fallback
  }
}

// ── Local Agent Health ──

export interface LocalAgentHealth {
  status: string
  service?: string
  version?: string
  dataDir?: string
  os?: string
  arch?: string
}

export async function fetchLocalAgentHealth(): Promise<LocalAgentHealth> {
  const response = await fetch(localAgentUrl('/api/local/health'))
  if (!response.ok) {
    throw new Error(await errorMessage(response, 'Local Agent 连接失败'))
  }
  return response.json() as Promise<LocalAgentHealth>
}

// ── Biaoshu AI Conversation ──

export interface BiaoshuConversationMessage {
  id: string
  role: string
  content: string
  createdAt: string
  metadata?: Record<string, unknown>
}

export interface BiaoshuConversationResponse {
  threadId: string
  messages: BiaoshuConversationMessage[]
}

export interface BiaoshuConversationMessageRequest {
  runId: string
  artifactPath: string
  artifactKind?: string
  role: string
  content: string
  metadata?: Record<string, unknown>
}

export async function fetchBiaoshuConversation(
  runId: string,
  artifactPath: string
): Promise<BiaoshuConversationResponse> {
  const params = new URLSearchParams({ runId, artifactPath })
  const response = await fetch(localAgentUrl(`/api/local/biaoshu-conversations?${params.toString()}`))
  if (!response.ok) {
    throw new Error(await errorMessage(response, '读取会话消息失败'))
  }
  return response.json() as Promise<BiaoshuConversationResponse>
}

export async function sendBiaoshuConversationMessage(
  payload: BiaoshuConversationMessageRequest
): Promise<{ threadId: string; message: BiaoshuConversationMessage }> {
  const response = await fetch(localAgentUrl('/api/local/biaoshu-conversations/messages'), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
  if (!response.ok) {
    throw new Error(await errorMessage(response, '发送消息失败'))
  }
  return response.json() as Promise<{ threadId: string; message: BiaoshuConversationMessage }>
}

// ── Biaoshu Artifact Write-Back ──

export interface BiaoshuArtifactWriteRequest {
  filePath: string
  content: string
  expectedPreviousContent?: string
}

export interface BiaoshuArtifactWriteResponse {
  filePath: string
  written: boolean
}

export async function writeLocalBiaoshuArtifact(
  payload: BiaoshuArtifactWriteRequest
): Promise<BiaoshuArtifactWriteResponse> {
  const response = await fetch(localAgentUrl('/api/local/biaoshu-artifacts/write'), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
  if (!response.ok) {
    throw new Error(await errorMessage(response, '写入产物文件失败'))
  }
  return response.json() as Promise<BiaoshuArtifactWriteResponse>
}
