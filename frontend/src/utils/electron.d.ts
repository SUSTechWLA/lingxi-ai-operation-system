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
  openExternal: (url: string) => Promise<void>
  isElectron: boolean
}

declare global {
  interface Window {
    electronAPI?: ElectronAPI
  }
}
