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

export interface LocalArtifactUploadResponse {
  id: string
  projectId: string
  storageRef?: string
  mimeType?: string
  contentHash?: string
  sizeBytes?: number
  path: string
  metadataPath: string
  metadata: Record<string, unknown>
}

export interface LocalArtifactFileResponse extends LocalArtifactUploadResponse {
  content?: string
  contentBase64?: string
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
    text_to_text: { ...DEFAULT_MODEL_PROVIDER_SETTINGS.text_to_text, ...providers?.text_to_text, apiKey: '' },
    text_to_image: { ...DEFAULT_MODEL_PROVIDER_SETTINGS.text_to_image, ...providers?.text_to_image, apiKey: '' },
    text_to_video: { ...DEFAULT_MODEL_PROVIDER_SETTINGS.text_to_video, ...providers?.text_to_video, apiKey: '' },
  }
}

export async function fetchModelProviderSettings(): Promise<ModelProviderSettingsResponse> {
  const response = await fetch(localAgentUrl('/api/local/model-providers'))
  if (!response.ok) {
    throw new Error(await errorMessage(response, '读取模型设置失败'))
  }
  return response.json() as Promise<ModelProviderSettingsResponse>
}

export async function fetchModelProviderSettingsWithKeys(): Promise<ModelProviderSettingsResponse> {
  const api = getElectronAPI()
  if (api?.getModelProviderSettingsWithKeys) {
    return api.getModelProviderSettingsWithKeys() as Promise<ModelProviderSettingsResponse>
  }
  const response = await fetch(localAgentUrl('/api/local/model-providers?include_key=true'))
  if (!response.ok) {
    throw new Error(await errorMessage(response, '读取模型密钥设置失败'))
  }
  return response.json() as Promise<ModelProviderSettingsResponse>
}

export async function buildClientModelProvidersForRun(): Promise<Partial<Record<ModelCapability, ModelProviderConfig>> | undefined> {
  try {
    const response = await fetchModelProviderSettingsWithKeys()
    const providers: Partial<Record<ModelCapability, ModelProviderConfig>> = {}
    for (const capability of ['text_to_text', 'text_to_image', 'text_to_video'] as const) {
      const provider = response.providers[capability]
      if (!provider?.apiKey) continue
      providers[capability] = {
        baseUrl: provider.baseUrl,
        model: provider.model,
        apiKey: provider.apiKey,
      }
    }
    return Object.keys(providers).length > 0 ? providers : undefined
  } catch {
    return undefined
  }
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

export async function uploadLocalArtifactFile(params: {
  projectId: string
  id: string
  file: File
  storageRef?: string
  mimeType?: string
  metadata?: Record<string, unknown>
}): Promise<LocalArtifactUploadResponse> {
  const formData = new FormData()
  formData.append('projectId', params.projectId)
  formData.append('id', params.id)
  if (params.storageRef) formData.append('storageRef', params.storageRef)
  if (params.mimeType || params.file.type) formData.append('mimeType', params.mimeType || params.file.type)
  if (params.metadata) formData.append('metadata', JSON.stringify(params.metadata))
  formData.append('file', params.file)

  const response = await fetch(localAgentUrl('/api/local/artifacts'), {
    method: 'POST',
    body: formData,
  })
  if (!response.ok) {
    throw new Error(await errorMessage(response, '上传本地产物失败'))
  }
  return response.json() as Promise<LocalArtifactUploadResponse>
}

export async function fetchLocalArtifactFile(params: {
  projectId: string
  id: string
}): Promise<LocalArtifactFileResponse> {
  const response = await fetch(localAgentUrl(`/api/local/artifacts/${encodeURIComponent(params.id)}?projectId=${encodeURIComponent(params.projectId)}`))
  if (!response.ok) {
    throw new Error(await errorMessage(response, '读取本地产物失败'))
  }
  return response.json() as Promise<LocalArtifactFileResponse>
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
