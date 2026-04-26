export interface CommandResult {
  stdout: string
  stderr: string
  exitCode: number
}

export interface ElectronAPI {
  executeCommand: (command: string, args?: string[], workDir?: string) => Promise<CommandResult>
  openFileDialog: (options?: Record<string, unknown>) => Promise<string[]>
  openDirectoryDialog: (options?: Record<string, unknown>) => Promise<string[]>
  saveFileDialog: (options?: Record<string, unknown>) => Promise<string | undefined>
  checkServiceHealth: () => Promise<string>
  isElectron: boolean
}

declare global {
  interface Window {
    electronAPI?: ElectronAPI
  }
}
