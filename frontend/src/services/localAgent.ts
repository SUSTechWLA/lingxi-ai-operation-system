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

export interface LocalAgentHealthResponse {
  status: string
  service?: string
  cloudApiBase?: string
  dataDir?: string
  os?: string
  arch?: string
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

export interface LocalDiagnosticsResponse {
  path: string
  createdAt: string
}

export type LocalMCPTransport = 'http' | 'stdio'

export interface LocalMCPProviderConfig {
  id: string
  label: string
  endpoint?: string
  transport?: LocalMCPTransport
  command?: string
  args?: string[]
  env?: Record<string, string>
  workingDir?: string
  toolPrefix?: string
  toolNameMap?: Record<string, string>
  enabled: boolean
}

export interface LocalMCPTool {
  name: string
  description?: string
  inputSchema?: Record<string, unknown>
}

export interface LocalMCPProviderStatus extends LocalMCPProviderConfig {
  reachable: boolean
  error?: string
  tools?: LocalMCPTool[]
}

export interface JiMengSetupStatusResponse {
  dreaminaAvailable: boolean
  dreaminaVersion?: string
  installCommand: string
  installScriptUrl: string
  logDir: string
  mcpProvider?: LocalMCPProviderConfig
  mcpProviders?: LocalMCPProviderStatus[]
  defaultMcpEndpoint: string
  mcpStartCommand: string
}

export interface JiMengInstallCLIResponse {
  status: 'ok' | 'failed'
  command: string
  installScriptUrl: string
  stdout?: string
  stderr?: string
  error?: string
}

export interface MCPToolCallResult {
  content?: Array<{ type: string; text?: string }>
  structuredContent?: Record<string, unknown>
  isError?: boolean
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

export async function fetchLocalAgentHealth(): Promise<LocalAgentHealthResponse> {
  const api = getElectronAPI()
  if (api?.checkServiceHealth) {
    const status = await api.checkServiceHealth()
    return {
      status: status === 'ok' ? 'ok' : 'unhealthy',
      service: 'tangying-local-agent',
      cloudApiBase: api.runtimeConfig?.cloudApiBase,
    }
  }
  const response = await fetch(localAgentUrl('/api/local/health'))
  if (!response.ok) {
    throw new Error(await errorMessage(response, '本地服务未连接'))
  }
  return response.json() as Promise<LocalAgentHealthResponse>
}

export async function openLocalPath(targetPath: string): Promise<boolean> {
  const api = getElectronAPI()
  if (!api?.openPath) return false
  return api.openPath(targetPath)
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
  signal?: AbortSignal
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
    signal: params.signal,
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

export function localArtifactRawUrl(params: {
  projectId: string
  id: string
}): string {
  const path = `/api/local/artifacts/${encodeURIComponent(params.id)}?projectId=${encodeURIComponent(params.projectId)}&raw=1`
  if (shouldUseSameOriginLocalAgentProxy()) return path
  return localAgentUrl(path)
}

export async function createLocalDiagnostics(reason: string): Promise<LocalDiagnosticsResponse> {
  const response = await fetch(localAgentUrl('/api/local/diagnostics'), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ reason }),
  })
  if (!response.ok) {
    throw new Error(await errorMessage(response, '导出诊断包失败'))
  }
  return response.json() as Promise<LocalDiagnosticsResponse>
}

export async function fetchJiMengSetupStatus(): Promise<JiMengSetupStatusResponse> {
  const response = await fetch(localAgentUrl('/api/local/jimeng/setup/status'))
  if (!response.ok) {
    throw new Error(await errorMessage(response, '读取即梦设置失败'))
  }
  return response.json() as Promise<JiMengSetupStatusResponse>
}

export async function installJiMengCLI(): Promise<JiMengInstallCLIResponse> {
  const response = await fetch(localAgentUrl('/api/local/jimeng/setup/install-cli'), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ confirm: true }),
  })
  if (!response.ok) {
    throw new Error(await errorMessage(response, '安装即梦 CLI 失败'))
  }
  return response.json() as Promise<JiMengInstallCLIResponse>
}

export type RegisterJiMengMCPInput = string | Partial<LocalMCPProviderConfig>

export async function registerJiMengMCP(input?: RegisterJiMengMCPInput): Promise<{ status: string; provider: LocalMCPProviderConfig; mcpStartCommand: string }> {
  const payload = typeof input === 'string' ? { endpoint: input } : input || {}
  const response = await fetch(localAgentUrl('/api/local/jimeng/setup/register-mcp'), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
  if (!response.ok) {
    throw new Error(await errorMessage(response, '注册即梦 MCP 失败'))
  }
  return response.json() as Promise<{ status: string; provider: LocalMCPProviderConfig; mcpStartCommand: string }>
}

export async function loginJiMengHeadless(): Promise<MCPToolCallResult> {
  const response = await fetch(localAgentUrl('/api/local/jimeng/setup/login-headless'), {
    method: 'POST',
  })
  if (!response.ok) {
    throw new Error(await errorMessage(response, '获取即梦登录码失败'))
  }
  return response.json() as Promise<MCPToolCallResult>
}

export async function checkJiMengLogin(deviceCode: string, poll = 30): Promise<MCPToolCallResult> {
  const response = await fetch(localAgentUrl('/api/local/jimeng/setup/check-login'), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ device_code: deviceCode, poll }),
  })
  if (!response.ok) {
    throw new Error(await errorMessage(response, '检查即梦登录失败'))
  }
  return response.json() as Promise<MCPToolCallResult>
}

function localAgentUrl(path: string): string {
  return `${getLocalAgentBaseUrl().replace(/\/$/, '')}${path}`
}

function shouldUseSameOriginLocalAgentProxy(): boolean {
  if (typeof window === 'undefined') return false
  const { protocol, hostname, port } = window.location
  return (protocol === 'http:' || protocol === 'https:') &&
    (hostname === '127.0.0.1' || hostname === 'localhost') &&
    port === '3000' &&
    !configuredLocalAgentUrl
}

async function errorMessage(response: Response, fallback: string): Promise<string> {
  try {
    const body = await response.json() as { error?: string }
    return body.error || fallback
  } catch {
    return fallback
  }
}
