const { contextBridge, ipcRenderer } = require('electron')

contextBridge.exposeInMainWorld('electronAPI', {
  isElectron: true,
  runtimeConfig: {
    localAgentUrl: process.env.TANGYING_LOCAL_AGENT_URL || 'http://127.0.0.1:18080',
    cloudApiBase: process.env.TANGYING_CLOUD_API_BASE || process.env.VITE_CLOUD_API_BASE || 'http://localhost:8080/api',
  },

  executeCommand: (command, args, workDir) =>
    ipcRenderer.invoke('execute-command', command, args, workDir),

  openFileDialog: (options) =>
    ipcRenderer.invoke('open-file-dialog', options),

  readFile: (filePath) =>
    ipcRenderer.invoke('read-file', filePath),

  openDirectoryDialog: () =>
    ipcRenderer.invoke('open-directory-dialog'),

  saveFileDialog: (options) =>
    ipcRenderer.invoke('save-file-dialog', options),

  checkServiceHealth: () =>
    ipcRenderer.invoke('check-service-health'),

  getRuntimeConfig: () =>
    ipcRenderer.invoke('get-runtime-config'),

  publishToPlatforms: (payload) =>
    ipcRenderer.invoke('publish-to-platforms', payload),

  openExternal: (url) =>
    ipcRenderer.invoke('open-external', url),
})
