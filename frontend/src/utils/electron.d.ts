export interface FileReadResult {
  data: string      // base64-encoded file content
  mimeType: string
  name: string
  size: number
}

export interface ElectronAPI {
  runtimeConfig: {
    localAgentUrl: string
    cloudApiBase: string
  }
  openFileDialog: (options?: Record<string, unknown>) => Promise<string[]>
  readFile: (filePath: string) => Promise<FileReadResult>
  openDirectoryDialog: (options?: Record<string, unknown>) => Promise<string[]>
  saveFileDialog: (options?: Record<string, unknown>) => Promise<string | undefined>
  checkServiceHealth: () => Promise<string>
  getRuntimeConfig: () => Promise<{
    localAgentUrl: string
    cloudApiBase: string
  }>
  getModelProviderSettingsWithKeys: () => Promise<{
    providers: Partial<Record<string, {
      baseUrl: string
      model: string
      apiKey?: string
      hasApiKey?: boolean
      apiKeyPreview?: string
    }>>
  }>
  openExternal: (url: string) => Promise<void>
  openPath: (targetPath: string) => Promise<boolean>
  isElectron: boolean
}

declare global {
  interface Window {
    electronAPI?: ElectronAPI
  }
}
