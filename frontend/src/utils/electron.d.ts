export interface CommandResult {
  stdout: string
  stderr: string
  exitCode: number
}

export interface PublishResult {
  platform: string
  success: boolean
  result?: string
  error?: string
}

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
    biaoshuToolsUrl?: string
  }
  executeCommand: (command: string, args?: string[], workDir?: string) => Promise<CommandResult>
  openFileDialog: (options?: Record<string, unknown>) => Promise<string[]>
  readFile: (filePath: string) => Promise<FileReadResult>
  openDirectoryDialog: (options?: Record<string, unknown>) => Promise<string[]>
  saveFileDialog: (options?: Record<string, unknown>) => Promise<string | undefined>
  checkServiceHealth: () => Promise<string>
  getRuntimeConfig: () => Promise<{
    localAgentUrl: string
    cloudApiBase: string
    biaoshuToolsUrl?: string
  }>
  publishToPlatforms: (payload: {
    title: string
    description: string
    images: string[]
    platforms: string[]
  }) => Promise<PublishResult[]>
  openExternal: (url: string) => Promise<void>
  isElectron: boolean
}

declare global {
  interface Window {
    electronAPI?: ElectronAPI
  }
}
