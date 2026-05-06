const { contextBridge, ipcRenderer } = require('electron')

contextBridge.exposeInMainWorld('electronAPI', {
  isElectron: true,

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

  publishToPlatforms: (payload) =>
    ipcRenderer.invoke('publish-to-platforms', payload),

  openExternal: (url) =>
    ipcRenderer.invoke('open-external', url),
})
