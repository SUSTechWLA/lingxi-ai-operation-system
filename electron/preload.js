const { contextBridge, ipcRenderer } = require('electron')

contextBridge.exposeInMainWorld('electronAPI', {
  // Command execution with user confirmation
  executeCommand: async (command, args, workDir) => {
    const choice = await ipcRenderer.invoke('dialog:confirm', {
      message: `即将执行命令：\n${command} ${(args || []).join(' ')}\n\n是否继续？`,
    })
    if (!choice) throw new Error('用户取消执行')
    return ipcRenderer.invoke('system:execute-command', { command, args, workDir })
  },

  // File dialogs
  openFileDialog: (options) => ipcRenderer.invoke('dialog:open-file', options),
  openDirectoryDialog: (options) => ipcRenderer.invoke('dialog:open-directory', options),
  saveFileDialog: (options) => ipcRenderer.invoke('dialog:save-file', options),

  // Service connectivity
  checkServiceHealth: () => ipcRenderer.invoke('service:health'),

  // Environment flag
  isElectron: true,
})
