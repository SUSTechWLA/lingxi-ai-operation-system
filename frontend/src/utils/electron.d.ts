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

export interface ElectronAPI {
  executeCommand: (command: string, args?: string[], workDir?: string) => Promise<CommandResult>
  openFileDialog: (options?: Record<string, unknown>) => Promise<string[]>
  openDirectoryDialog: (options?: Record<string, unknown>) => Promise<string[]>
  saveFileDialog: (options?: Record<string, unknown>) => Promise<string | undefined>
  checkServiceHealth: () => Promise<string>
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
